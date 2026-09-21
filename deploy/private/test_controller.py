import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.request

from controller import Controller, IMAGE, REPO, SafeRedirect, atomic_json, validate_manifest, validate_request


class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.config = {'enabled': False, 'state_dir': self.tmp.name,
                       'production_dir': self.tmp.name, 'health_urls': []}
        self.controller = Controller(self.config)
        self.manifest = {'repository': REPO, 'image': IMAGE, 'platform': 'linux/amd64',
                         'version': 'v1.0.0', 'digest': 'sha256:' + 'a' * 64,
                         'commit': 'b' * 40, 'migrations': {'001_init.sql': 'c' * 64}}

    def test_default_disabled_does_not_touch_docker(self):
        with patch('controller.run') as command:
            with self.assertRaisesRegex(ValueError, 'disabled'):
                self.controller.submit({'action': 'prepare', 'version': 'v1.0.0', 'confirm': True})
            command.assert_not_called()

    def test_no_commands_urls_or_unconfirmed_requests(self):
        for value in ['v1.0.0;id', '../v1.0.0', 'latest', 'https://evil/image', 'v1.0.0-rc1']:
            with self.assertRaises(ValueError):
                validate_request({'action': 'prepare', 'version': value, 'confirm': True})
        for req in [{'action': 'prepare', 'version': 'v1.0.0'},
                    {'action': 'deploy', 'job_id': '../test', 'confirm': True},
                    {'action': 'prepare', 'version': 'v1.0.0', 'confirm': True, 'command': 'id'}]:
            with self.assertRaises(ValueError):
                validate_request(req)

    def test_manifest_is_bound_to_private_repo_digest_and_version(self):
        self.assertEqual(validate_manifest(self.manifest, 'v1.0.0'), IMAGE + '@sha256:' + 'a' * 64)
        for key, bad in [('repository', 'Wei-Shaw/sub2api'), ('image', 'evil/image'),
                         ('digest', 'latest'), ('commit', 'main'), ('platform', 'linux/arm64'),
                         ('version', 'v2.0.0'), ('migrations', {})]:
            value = copy.deepcopy(self.manifest)
            value[key] = bad
            with self.assertRaises(ValueError):
                validate_manifest(value, 'v1.0.0')

    def test_no_implicit_schema_upgrade(self):
        with patch.object(self.controller, 'migrations', return_value={'old.sql': 'c' * 64}):
            with self.assertRaisesRegex(ValueError, 'separate migration'):
                self.controller.assert_migrations(self.manifest)

    def test_credentials_stripped_on_asset_redirect(self):
        req = urllib.request.Request('https://api.github.com/repos/' + REPO + '/releases/assets/1',
                                     headers={'Authorization': 'Bearer test-token'})
        redirected = SafeRedirect().redirect_request(req, None, 302, '', {},
                      'https://release-assets.githubusercontent.com/asset')
        self.assertIsNone(redirected.get_header('Authorization'))
        for url in ['http://api.github.com/a', 'https://evil.example/a',
                    'https://release-assets.githubusercontent.com.evil.example/a']:
            with self.assertRaises(RuntimeError):
                SafeRedirect().redirect_request(req, None, 302, '', {}, url)

    def test_compose_changes_only_application(self):
        Path(self.tmp.name, 'docker-compose.yml').touch()
        Path(self.tmp.name, 'docker-compose.override.yml').touch()
        overlay = self.controller.override(IMAGE + '@sha256:' + 'a' * 64)
        data = json.loads(overlay.read_text())
        self.assertEqual(list(data['services']), ['sub2api'])
        self.assertEqual(data['services']['sub2api']['pull_policy'], 'never')
        self.assertNotIn('/var/run/docker.sock', overlay.read_text())
        with patch('controller.run', side_effect=[b'', b'[ {"Id":"expected"} ]']) as command, \
             patch.object(self.controller, 'wait_command'), \
             patch.object(self.controller, 'inspect', return_value={'Image': 'expected'}):
            self.controller.switch(IMAGE + '@sha256:' + 'a' * 64)
        invocation = command.call_args_list[0].args[0]
        self.assertEqual(invocation[-6:], ['up', '-d', '--no-deps', '--pull', 'never', 'sub2api'])
        self.assertNotIn('down', invocation)

    def test_stale_production_aborts_before_backup_or_switch(self):
        job = {'id': 'a' * 32, 'version': 'v1.0.0', 'restore_verified': True, 'rollback_verified': True,
               'original_container': 'old', 'original_started': 'old'}
        with patch.object(self.controller, 'inspect', return_value={'Id': 'changed', 'State': {'StartedAt': 'new'}}), \
             patch('controller.run') as command:
            with self.assertRaisesRegex(ValueError, 'changed after preparation'):
                self.controller.deploy(job)
            command.assert_not_called()

    def test_atomic_job_status_survives_reopen(self):
        job = {'id': 'a' * 32, 'version': 'v1.0.0'}
        self.controller.save(job, 'ready')
        reopened = Controller(self.config)
        self.assertEqual(reopened.status()['job']['status'], 'ready')


if __name__ == '__main__':
    unittest.main()
