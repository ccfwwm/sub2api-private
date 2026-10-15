import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from controller import atomic_json, file_hash
from manage import Manager, ident, DOCS_PROXY_SNIPPET


class ManagementSafetyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.manager = Manager(self.root / 'state', self.root / 'production')

    def test_ids_reject_paths_and_commands(self):
        for value in ['../backup', 'id;reboot', '/etc', 'latest', 'a' * 33]:
            with self.assertRaises(ValueError):
                ident(value)

    def test_target_preserves_recovered_stores(self):
        previous = {'services': {'sub2api': {'environment': {'DATABASE_HOST': 'recovered-pg'},
                      'volumes': ['/recovered/data:/app/data'], 'networks': {'recovery': {}}}},
                    'networks': {'recovery': {'external': True}},
                    'x-ops': {'postgres': 'recovered-pg', 'redis': 'recovered-redis'}}
        original = json.loads(json.dumps(previous))
        target = self.manager.target('sha256:' + 'a' * 64, previous)
        self.assertEqual(previous, original)
        self.assertEqual(target['x-ops'], original['x-ops'])
        self.assertEqual(target['services']['sub2api']['environment']['DATABASE_HOST'], 'recovered-pg')
        self.assertIn('/recovered/data:/app/data', target['services']['sub2api']['volumes'])
        self.assertEqual(list(target['services']), ['sub2api'])

    def test_target_can_restore_docs_service_and_legacy_target_removes_it(self):
        target = self.manager.target('image', {'services': {'sub2api': {}}, 'x-ops': {}}, 'ghcr.io/example/docs@sha256:' + 'a' * 64)
        self.assertEqual(target['services']['docs-web']['image'], 'ghcr.io/example/docs@sha256:' + 'a' * 64)
        self.assertNotIn('docs-web', self.manager.target('image', {'services': {'sub2api': {}}, 'x-ops': {}})['services'])

    def test_docs_proxy_insertion_is_idempotent(self):
        vhost = self.root / 'nginx.conf'
        snippet = self.root / 'snippets' / 'sub2api-docs-web.conf'
        vhost.write_text('server {\n    listen 443 ssl;\n    server_name code.aicodeme.xyz;\n    location / { proxy_pass http://127.0.0.1:8081; }\n}\n')
        manager = Manager(self.root / 'proxy-state', self.root / 'production', docs_proxy_enabled=True,
                          nginx_vhost=vhost, nginx_snippet=snippet)
        with patch('manage.run', return_value=b'') as run:
            manager.sync_docs_proxy(True)
            manager.sync_docs_proxy(True)
        content = vhost.read_text()
        self.assertEqual(content.count('include /etc/nginx/snippets/sub2api-docs-web.conf;'), 1)
        self.assertEqual(snippet.read_text(), DOCS_PROXY_SNIPPET)
        self.assertGreaterEqual(run.call_count, 2)

    def test_docs_proxy_restores_files_when_nginx_validation_fails(self):
        vhost = self.root / 'nginx.conf'
        snippet = self.root / 'snippets' / 'sub2api-docs-web.conf'
        original = 'server {\n    listen 443 ssl;\n    server_name code.aicodeme.xyz;\n}\n'
        vhost.write_text(original)
        manager = Manager(self.root / 'proxy-state', self.root / 'production', docs_proxy_enabled=True,
                          nginx_vhost=vhost, nginx_snippet=snippet)
        with patch('manage.run', side_effect=RuntimeError('nginx -t failed')):
            with self.assertRaisesRegex(RuntimeError, 'nginx -t failed'):
                manager.sync_docs_proxy(True)
        self.assertEqual(vhost.read_text(), original)
        self.assertFalse(snippet.exists())

    def test_backup_tampering_prevents_restore(self):
        folder = self.manager.state / 'backups' / ('a' * 32)
        folder.mkdir(parents=True)
        names = ['database.dump', 'globals.sql', 'protected-snapshot.json', 'runtime-image.tar',
                 'application-data.tar.gz', 'configuration.tar.gz', 'redis.rdb', 'metadata.json']
        for name in names:
            (folder / name).write_text('{}')
        atomic_json(folder / 'checksums.json', {n: file_hash(folder / n) for n in names})
        (folder / 'database.dump').write_text('corrupted')
        with patch('manage.run') as run:
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                self.manager.verified_backup('a' * 32)
            run.assert_not_called()

    def test_old_rescue_job_cannot_replace_newer_deployment(self):
        atomic_json(self.manager.state / 'latest-cutover.json', {'id': 'b' * 32})
        with patch.object(self.manager, 'switch') as switch:
            with self.assertRaisesRegex(ValueError, 'most recent'):
                self.manager.execute({'id': 'c' * 32, 'action': 'rescue', 'target': 'a' * 32})
            switch.assert_not_called()

    def test_backup_tracks_recovered_database(self):
        atomic_json(self.manager.active_path, {'x-ops': {'postgres': 'recovered-pg', 'redis': 'recovered-redis'}})
        self.manager.refresh()
        with patch('controller.run', return_value=b'1') as run:
            self.manager.c.sql('SELECT 1;')
        self.assertEqual(run.call_args.args[0][3], 'recovered-pg')

    def test_prepare_never_enters_cutover(self):
        job = {'id': 'a' * 32, 'action': 'prepare', 'target': 'v1.0.1'}
        with patch.object(self.manager, 'preflight', return_value={'Id': 'old'}), \
             patch.object(self.manager, 'offline', return_value=('image', {'migrations': {}})), \
             patch.object(self.manager.c, 'migrations', return_value={}), \
             patch.object(self.manager.c, 'assert_migrations'), \
             patch.object(self.manager, 'backup', return_value=(self.root, {'runtime_image': 'old'})), \
             patch('manage.inspect', return_value={'Id': 'old'}), \
             patch.object(self.manager, 'rehearse'), \
             patch.object(self.manager, 'cutover') as cutover:
            self.manager.execute(job)
        self.assertTrue(job['prepared_only'])
        self.assertEqual(job['status'], 'succeeded')
        cutover.assert_not_called()

    def test_failed_drain_probe_restores_admission(self):
        with patch.object(self.manager.c, 'inspect', return_value={'Config': {'Env': [
                'IMAGE_DEPLOY_DRAIN_FILE=/run/sub2api-ops/drain']}}), \
             patch.object(self.manager, 'wait_for_idle'), \
             patch('manage.run', side_effect=ValueError('probe failed')):
            with self.assertRaisesRegex(ValueError, 'probe failed'):
                self.manager.drain()
        self.assertFalse(self.manager.marker.exists())

    def test_drain_timeout_requires_zero_active_and_restores_admission(self):
        with patch.object(self.manager.c, 'inspect', return_value={'Config': {'Env': [
                'IMAGE_DEPLOY_DRAIN_FILE=/run/sub2api-ops/drain']}}), \
             patch.object(self.manager, 'wait_for_idle'), \
             patch('manage.run', return_value=b'{"active":1,"draining":true}'), \
             patch('manage.time.monotonic', side_effect=[0, 1, 2, 3, 121]), \
             patch('manage.time.sleep'):
            with self.assertRaisesRegex(ValueError, 'active requests did not drain'):
                self.manager.drain()
        self.assertFalse(self.manager.marker.exists())

    def test_successful_drain_retains_marker_until_caller_finishes(self):
        with patch.object(self.manager.c, 'inspect', return_value={'Config': {'Env': [
                'IMAGE_DEPLOY_DRAIN_FILE=/run/sub2api-ops/drain']}}), \
             patch.object(self.manager, 'wait_for_idle'), \
             patch('manage.run', return_value=b'{"active":0,"draining":true}'):
            self.manager.drain()
        self.assertTrue(self.manager.marker.exists())

    def test_idle_wait_keeps_admission_open(self):
        statuses = iter([{'active': 2, 'draining': False}, {'active': 0, 'draining': False}])
        def probe():
            self.assertFalse(self.manager.marker.exists())
            return next(statuses)
        with patch.object(self.manager, 'deployment_status', side_effect=probe), \
             patch('manage.time.sleep'):
            self.manager.wait_for_idle()
        self.assertFalse(self.manager.marker.exists())

    def test_busy_idle_wait_times_out_without_maintenance(self):
        with patch.object(self.manager, 'deployment_status', return_value={'active': 1, 'draining': False}), \
             patch('manage.time.monotonic', side_effect=[0, 1, 2, 3, 901]), \
             patch('manage.time.sleep'):
            with self.assertRaisesRegex(ValueError, 'production remains open'):
                self.manager.wait_for_idle()
        self.assertFalse(self.manager.marker.exists())

    def test_malformed_deployment_status_cannot_authorize_cutover(self):
        for status in (None, [], {}, {'active': -1, 'draining': False},
                       {'active': False, 'draining': True}, {'active': 0, 'draining': 'false'},
                       {'active': '0', 'draining': False}):
            with self.subTest(status=status), patch('manage.run', return_value=json.dumps(status).encode()):
                with self.assertRaisesRegex(ValueError, 'invalid deployment status'):
                    self.manager.deployment_status()

    def test_existing_maintenance_prevents_idle_wait(self):
        self.manager.marker.touch()
        with patch.object(self.manager, 'deployment_status') as probe:
            with self.assertRaisesRegex(ValueError, 'maintenance marker already exists'):
                self.manager.wait_for_idle()
        probe.assert_not_called()
        self.assertTrue(self.manager.marker.exists())

    def test_existing_runtime_drain_cannot_be_treated_as_idle(self):
        with patch.object(self.manager, 'deployment_status', return_value={'active': 0, 'draining': True}):
            with self.assertRaisesRegex(ValueError, 'already draining'):
                self.manager.wait_for_idle()
        self.assertFalse(self.manager.marker.exists())

    def test_idle_wait_failure_never_enters_maintenance(self):
        with patch.object(self.manager.c, 'inspect', return_value={'Config': {'Env': [
                'IMAGE_DEPLOY_DRAIN_FILE=/run/sub2api-ops/drain']}}), \
             patch.object(self.manager, 'wait_for_idle', side_effect=ValueError('no idle window')):
            with self.assertRaisesRegex(ValueError, 'no idle window'):
                self.manager.drain()
        self.assertFalse(self.manager.marker.exists())


if __name__ == '__main__':
    unittest.main()
