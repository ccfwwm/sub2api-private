"""Protocol and failure-boundary tests; run on Linux without Docker mutations."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import types
import unittest
from unittest.mock import patch
import gateway


def archive(path, names):
    with tarfile.open(path, 'w:gz') as tf:
        for name, kind in names:
            info = tarfile.TarInfo(name)
            info.mode = 0o644
            if kind == 'link':
                info.type = tarfile.SYMTYPE
                info.linkname = '/etc/shadow'
                tf.addfile(info)
            else:
                data = b'trusted-source'
                info.size = len(data)
                tf.addfile(info, io.BytesIO(data))


class ProtocolTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def test_release_files_extract_and_unsafe_archives_do_not_escape(self):
        for index, entries in enumerate([[('package.json', 'file')], [('../escape', 'file')], [('linked', 'link')], [('a', 'file'), ('a', 'file')], [('node_modules/pkg', 'file')], [('/etc/cron.d/job', 'file')]]):
            path = self.root / (str(index) + '.tgz')
            archive(path, entries)
            if index == 0:
                gateway.extract(path, self.root / 'ok')
                self.assertEqual((self.root / 'ok/package.json').read_bytes(), b'trusted-source')
            else:
                with self.assertRaises(ValueError):
                    gateway.extract(path, self.root / ('bad-' + str(index)))
        self.assertFalse((self.root.parent / 'escape').exists())

    def test_forced_key_rejects_shell_and_unbounded_arguments(self):
        handler = types.SimpleNamespace(valid_revision=lambda s: s == 'v1.3.25')
        with patch.dict(sys.modules, handler=handler), patch.object(gateway, 'STATE', self.root):
            for words in [['bash'], ['deploy', '../../etc/shadow', '0'*64], ['deploy', 'v1.3.25', '0'*64, '; reboot'], ['status', '../shadow'], ['verify', 'anything']]:
                with self.assertRaises(ValueError):
                    gateway.dispatch(words, io.BytesIO())

    def test_bad_upload_checksum_cannot_create_a_job(self):
        handler = types.SimpleNamespace(valid_revision=lambda _: True)
        with patch.dict(sys.modules, handler=handler), patch.object(gateway, 'STATE', self.root):
            with self.assertRaises(ValueError):
                gateway.dispatch(['deploy', 'a'*40, '0'*64], io.BytesIO(b'bad'))
        self.assertEqual(list(self.root.iterdir()), [])

    def test_durable_worker_records_success_and_rollback_failure(self):
        ident = 'a'*32
        archive(self.root / (ident+'.tar.gz'), [('package.json', 'file')])
        def deploy(revision, source, prepare, phase):
            phase('health_check')
            self.assertTrue((source / 'package.json').is_file())
            return {'production_changed': False}
        handler = types.SimpleNamespace(deploy=deploy)
        with patch.dict(sys.modules, handler=handler), patch.object(gateway, 'STATE', self.root):
            gateway.atomic(self.root / (ident+'.json'), {'id':ident, 'revision':'a'*40, 'prepare_only':False, 'status':'queued'})
            gateway.worker(ident)
            result = gateway.read(self.root / (ident+'.json'))
            self.assertEqual(result['status'], 'succeeded')
            self.assertEqual(result['phase'], 'online_verified')
            self.assertFalse(result['result']['production_changed'])
        other = 'b'*32
        archive(self.root / (other+'.tar.gz'), [('package.json', 'file')])
        def fail(*args):
            error = ValueError('candidate health failed')
            error.deploy_status = 'failed_rolled_back'
            raise error
        handler.deploy = fail
        with patch.dict(sys.modules, handler=handler), patch.object(gateway, 'STATE', self.root):
            gateway.atomic(self.root / (other+'.json'), {'id':other, 'revision':'a'*40, 'prepare_only':False, 'status':'queued'})
            with self.assertRaises(ValueError):
                gateway.worker(other)
            self.assertEqual(gateway.read(self.root / (other+'.json'))['status'], 'failed_rolled_back')

    def test_security_exception_only_allows_accepted_unfixed_cve(self):
        receipt = self.root / 'scan.json'
        allowed = (('CVE-2026-6653', 'libxml2'),)
        for cve, fixed, accepted in [('CVE-2026-6653', '', True), ('CVE-2026-6653', '2.15.1', False), ('CVE-OTHER', '', False)]:
            gateway.atomic(receipt, {'Results':[{'Vulnerabilities':[{'VulnerabilityID':cve,'PkgName':'libxml2','Severity':'CRITICAL','FixedVersion':fixed}]}]})
            with patch.object(gateway, 'run'):
                if accepted:
                    gateway.scan('sha256:abc', receipt, allowed)
                else:
                    with self.assertRaises(ValueError):
                        gateway.scan('sha256:abc', receipt, allowed)


if __name__ == '__main__':
    unittest.main()
