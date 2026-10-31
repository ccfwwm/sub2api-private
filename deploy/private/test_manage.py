import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from controller import atomic_json, file_hash
from manage import Manager, ident, archive_application_data, DOCS_PROXY_SNIPPET, LEGACY_DOCS_PROXY_SNIPPET, V1219_DOCS_PROXY_SNIPPET


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

    def test_diagnose_checks_inventory_without_backups_or_cutover(self):
        before = {'old.sql': 'a' * 64}
        target = {**before, '253_scoped_admin_api_keys.sql': 'b' * 64}
        manifest = {'commit': 'c' * 40, 'digest': 'sha256:' + 'd' * 64, 'migrations': target}
        with patch.object(self.manager, 'preflight', return_value={'Image': 'old-image'}), \
                patch.object(self.manager, 'offline', return_value=('new-image', manifest)), \
                patch.object(self.manager.c, 'migrations', return_value=before), \
                patch('media_migrate.reviewed_upgrade', return_value={'filename': 'upstream-v0.2.19'}), \
                patch.object(self.manager, 'backup') as backup, \
                patch.object(self.manager, 'cutover') as cutover:
            result = self.manager.diagnose('v1.3.21')
        self.assertEqual(result['status'], 'ready')
        self.assertEqual(result['pending_migrations'], ['253_scoped_admin_api_keys.sql'])
        self.assertTrue(result['backup_and_restore_still_required'])
        backup.assert_not_called()
        cutover.assert_not_called()

    def test_diagnose_rejects_invalid_version_before_inspection(self):
        with patch.object(self.manager, 'preflight') as preflight:
            with self.assertRaises(ValueError):
                self.manager.diagnose('main; reboot')
        preflight.assert_not_called()

    def test_application_data_archive_uses_fixed_non_recursive_inventory(self):
        source = self.root / 'data'
        (source / 'logs').mkdir(parents=True)
        (source / 'media').mkdir()
        (source / 'logs' / 'rolling.log').write_text('live log')
        (source / 'media' / 'old file.mp4').write_bytes(b'original')
        archive, inventory = self.root / 'data.tar.gz', self.root / 'data.list'
        commands = []

        def fake_run(args, *, stdout=None, **kwargs):
            commands.append(args)
            if args[0] == 'find':
                self.assertEqual(kwargs['cwd'], str(source))
                stdout.write(b'.\0./media\0./media/old file.mp4\0')
                return b''
            self.assertEqual(args[0], 'tar')
            self.assertIn('--no-recursion', args)
            self.assertIn('--null', args)
            self.assertLess(args.index('-C'), args.index('--files-from'))
            listed = Path(args[args.index('--files-from') + 1]).read_bytes().split(b'\0')
            self.assertIn(b'./media/old file.mp4', listed)
            self.assertNotIn(b'./logs/rolling.log', listed)
            (source / 'media' / 'new during archive.mp4').write_bytes(b'new')
            self.assertNotIn(b'./media/new during archive.mp4', listed)
            return b''

        with patch('manage.run', side_effect=fake_run):
            archive_application_data(source, archive, inventory)
        self.assertEqual([command[0] for command in commands], ['find', 'tar'])
        self.assertFalse(inventory.exists())

    def test_application_data_archive_fails_closed_if_tar_fails(self):
        source = self.root / 'data'
        source.mkdir()
        inventory = self.root / 'data.list'

        def fake_run(args, *, stdout=None, **kwargs):
            if args[0] == 'find':
                stdout.write(b'.\0./file\0')
                return b''
            raise RuntimeError('tar failed')

        with patch('manage.run', side_effect=fake_run):
            with self.assertRaisesRegex(RuntimeError, 'tar failed'):
                archive_application_data(source, self.root / 'data.tar.gz', inventory)
        self.assertFalse(inventory.exists())

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
        self.assertEqual(target['services']['docs-web']['container_name'], 'sub2api-docs-web')
        self.assertNotIn('docs-web', self.manager.target('image', {'services': {'sub2api': {}}, 'x-ops': {}})['services'])

    def test_docs_container_resolves_compose_generated_name(self):
        result = type('Result', (), {'returncode': 0, 'stdout': 'sub2api-deploy-docs-web-1\n'})()
        with patch('manage.subprocess.run', return_value=result) as run:
            self.assertEqual(self.manager.docs_container(), 'sub2api-deploy-docs-web-1')
        command = run.call_args.args[0]
        self.assertIn('label=com.docker.compose.project=sub2api-deploy', command)
        self.assertIn('label=com.docker.compose.service=docs-web', command)

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

    def test_docs_proxy_upgrades_legacy_headers_and_preserves_vhost(self):
        vhost = self.root / 'nginx.conf'
        snippet = self.root / 'sub2api-docs-web.conf'
        original = ('server {\n    listen 443 ssl;\n    server_name code.aicodeme.xyz;\n'
                    '    include /etc/nginx/snippets/sub2api-docs-web.conf;\n'
                    '    add_header X-Frame-Options "DENY" always;\n}\n')
        vhost.write_text(original)
        snippet.write_text(LEGACY_DOCS_PROXY_SNIPPET)
        manager = Manager(self.root / 'proxy-state', self.root / 'production',
                          nginx_vhost=vhost, nginx_snippet=snippet)
        with patch('manage.run', return_value=b''):
            manager.sync_docs_proxy(True)
        self.assertEqual(vhost.read_text(), original)
        updated = snippet.read_text()
        self.assertIn('X-Frame-Options "SAMEORIGIN" always', updated)
        self.assertIn('frame-ancestors \'self\'', updated)
        self.assertIn('Strict-Transport-Security "max-age=31536000; includeSubDomains"', updated)
        self.assertIn('X-Content-Type-Options "nosniff"', updated)

    def test_v1219_proxy_upgrade_adds_private_user_playback(self):
        vhost, snippet = self.root / 'nginx.conf', self.root / 'snippet.conf'
        vhost.write_text('server {\n    server_name code.aicodeme.xyz;\n    include /etc/nginx/snippets/sub2api-docs-web.conf;\n}\n')
        snippet.write_text(V1219_DOCS_PROXY_SNIPPET)
        manager = Manager(self.root / 'proxy-state', self.root / 'production', nginx_vhost=vhost, nginx_snippet=snippet)
        with patch('manage.run', return_value=b''):
            manager.sync_docs_proxy(True)
        self.assertEqual(snippet.read_text(), DOCS_PROXY_SNIPPET)
        self.assertIn('location ^~ /api/v1/media/playback/tasks/', snippet.read_text())
        self.assertEqual(snippet.read_text().count('access_log off;'), 2)

    def test_docs_proxy_failed_upgrade_restores_legacy_snippet(self):
        vhost = self.root / 'nginx.conf'
        snippet = self.root / 'sub2api-docs-web.conf'
        original = 'server {\n    listen 443 ssl;\n    server_name code.aicodeme.xyz;\n}\n'
        vhost.write_text(original)
        snippet.write_text(LEGACY_DOCS_PROXY_SNIPPET)
        manager = Manager(self.root / 'proxy-state', self.root / 'production',
                          nginx_vhost=vhost, nginx_snippet=snippet)
        with patch('manage.run', side_effect=RuntimeError('nginx -t failed')):
            with self.assertRaises(RuntimeError):
                manager.sync_docs_proxy(True)
        self.assertEqual(vhost.read_text(), original)
        self.assertEqual(snippet.read_text(), LEGACY_DOCS_PROXY_SNIPPET)

    def test_docs_proxy_rejects_unknown_snippet_before_changing_files(self):
        vhost = self.root / 'nginx.conf'
        snippet = self.root / 'sub2api-docs-web.conf'
        original = 'server {\n    listen 443 ssl;\n    server_name code.aicodeme.xyz;\n}\n'
        vhost.write_text(original)
        snippet.write_text('location /custom/ { return 200; }\n')
        manager = Manager(self.root / 'proxy-state', self.root / 'production',
                          nginx_vhost=vhost, nginx_snippet=snippet)
        with patch('manage.run') as run:
            with self.assertRaisesRegex(ValueError, 'unexpected contents'):
                manager.sync_docs_proxy(True)
        run.assert_not_called()
        self.assertEqual(vhost.read_text(), original)
        self.assertEqual(snippet.read_text(), 'location /custom/ { return 200; }\n')

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

    def test_template_rehearsal_requires_exact_data_projection_and_does_not_claim_accounts_unchanged(self):
        backup = self.root / 'fixture-backup'
        backup.mkdir()
        meta = {'runtime_image': 'captured-old-runtime', 'migrations': {'old.sql': 'a' * 64}}
        reviewed = {'filename': 'media-protocol-templates', 'files': ['249_media_protocol_templates.sql',
                    '250_migrate_legacy_media_template_refs.sql'], 'transaction': 'sealed', 'expected_migrations': {}}
        def report(test, folder):
            test.update(restore_verified=True, rollback_verified=True, additive_data_verified=True)
        with patch.object(self.manager.c, 'rehearse', side_effect=report):
            with self.assertRaisesRegex(ValueError, 'projection was not verified'):
                self.manager.rehearse({'id': 'a' * 32}, backup, meta, 'candidate', 'v1.2.15', reviewed)
        def verified(test, folder):
            report(test, folder)
            test['template_data_verified'] = True
        job = {'id': 'b' * 32}
        with patch.object(self.manager.c, 'rehearse', side_effect=verified):
            self.manager.rehearse(job, backup, meta, 'candidate', 'v1.2.15', reviewed)
        result = job['template_migration']
        self.assertFalse(result['data_unchanged'])
        self.assertTrue(result['exact_account_projection_verified'])
        self.assertTrue(result['task_snapshots_and_other_business_data_unchanged'])
        self.assertEqual(result['files'], reviewed['files'])

    def test_upstream_migration_rehearsal_requires_schema_and_default_evidence(self):
        backup = self.root / 'fixture-backup'
        backup.mkdir()
        meta = {'runtime_image': 'captured-old-runtime', 'migrations': {'old.sql': 'a' * 64}}
        reviewed = {'filename': 'upstream-v0.2.19',
                    'files': ['253_scoped_admin_api_keys.sql', '254_group_image_count_billing.sql'],
                    'transaction': 'sealed', 'expected_migrations': {}}

        def report(test, folder, verified=False):
            test.update(restore_verified=True, rollback_verified=True, additive_data_verified=True)
            if verified:
                test.update(scoped_admin_keys_schema_verified=True, image_count_default_verified=True)

        with patch.object(self.manager.c, 'rehearse', side_effect=report):
            with self.assertRaisesRegex(ValueError, 'evidence is incomplete'):
                self.manager.rehearse({'id': 'a' * 32}, backup, meta, 'candidate', 'v1.3.20', reviewed)
        job = {'id': 'b' * 32}
        with patch.object(self.manager.c, 'rehearse', side_effect=lambda test, folder: report(test, folder, True)):
            self.manager.rehearse(job, backup, meta, 'candidate', 'v1.3.20', reviewed)
        result = job['upstream_migrations']
        self.assertEqual(result['files'], reviewed['files'])
        self.assertTrue(result['scoped_admin_keys_schema_verified'])
        self.assertTrue(result['image_count_default_verified'])
        self.assertTrue(result['billing_and_media_data_unchanged'])

    def test_failed_drain_probe_restores_admission(self):
        with patch.object(self.manager.c, 'inspect', return_value={'Config': {'Env': [
                'IMAGE_DEPLOY_DRAIN_FILE=/run/sub2api-ops/drain']}}), \
             patch.object(self.manager, 'wait_for_idle'), \
             patch('manage.run', side_effect=ValueError('probe failed')):
            with self.assertRaisesRegex(ValueError, 'probe failed'):
                self.manager.drain()
        self.assertFalse(self.manager.marker.exists())

    def test_template_migration_runs_only_after_drain_and_before_application_switch(self):
        events = []
        job = {'id': 'a' * 32}
        reviewed = {'filename': 'media-protocol-templates', 'transaction': 'sealed-template-sql',
                    'expected_migrations': {'new.sql': 'b' * 64}}
        before = {'old.sql': 'a' * 64}
        target = {'services': {'sub2api': {'environment': {}}}}
        state = {'Id': 'fixed', 'Image': 'image', 'State': {'StartedAt': 'fixed'}, 'Config': {}}
        def check(manifest):
            events.append('assert-old' if manifest['migrations'] == before else 'assert-new')
        with patch('manage.inspect', return_value=state), \
             patch.object(self.manager.c, 'inspect', return_value=state), \
             patch.object(self.manager, 'docs_container', return_value=None), \
             patch.object(self.manager, 'drain', side_effect=lambda _: events.append('drain')), \
             patch.object(self.manager.c, 'assert_migrations', side_effect=check), \
             patch.object(self.manager.c, 'apply_reviewed_media_migration', side_effect=lambda _: events.append('apply')), \
             patch.object(self.manager, 'switch', side_effect=lambda _: events.append('switch')), \
             patch.object(self.manager.c, 'wait_command'), \
             patch('manage.run', return_value=b'{"data":{"version":"1.2.15"}}'):
            self.manager.cutover(job, target, {}, 'v1.2.15', reviewed, before)
        self.assertEqual(events, ['drain', 'assert-old', 'apply', 'assert-new', 'switch'])
        self.assertEqual(job['status'], 'succeeded')

    def test_failed_drain_or_immediate_mode_never_applies_template_data_migration(self):
        reviewed = {'filename': 'media-protocol-templates', 'transaction': 'sealed', 'expected_migrations': {}}
        state = {'Id': 'fixed', 'Image': 'image', 'State': {'StartedAt': 'fixed'}, 'Config': {}}
        with patch('manage.inspect', return_value=state), \
             patch.object(self.manager, 'docs_container', return_value=None), \
             patch.object(self.manager, 'drain', side_effect=ValueError('busy')), \
             patch.object(self.manager.c, 'apply_reviewed_media_migration') as apply, \
             patch.object(self.manager, 'switch') as switch:
            with self.assertRaisesRegex(ValueError, 'busy'):
                self.manager.cutover({'id': 'b' * 32}, {}, {}, 'v1.2.15', reviewed, {})
            with self.assertRaisesRegex(ValueError, 'drained'):
                self.manager.cutover({'id': 'c' * 32, 'immediate': True}, {}, {}, 'v1.2.15', reviewed, {})
            apply.assert_not_called()
            switch.assert_not_called()

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
