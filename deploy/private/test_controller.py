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

    def test_capacity_compose_overlay_is_applied_after_active_compose_state(self):
        active = Path(self.tmp.name) / 'active.json'
        capacity = Path(self.tmp.name) / 'capacity-compose.yml'
        active.touch()
        capacity.touch()
        args = self.controller.compose(active)
        self.assertLess(args.index(str(active)), args.index(str(capacity)))
        self.assertEqual(args[-2:], ['-f', str(capacity)])

    def test_no_commands_urls_or_unconfirmed_requests(self):
        for value in ['v1.0.0;id', '../v1.0.0', 'latest', 'https://evil/image', 'v1.0.0-rc1']:
            with self.assertRaises(ValueError):
                validate_request({'action': 'prepare', 'version': value, 'confirm': True})
        for req in [{'action': 'prepare', 'version': 'v1.0.0'},
                    {'action': 'prepare', 'version': 1, 'confirm': True},
                    {'action': 'deploy', 'job_id': None, 'confirm': True},
                    {'action': 'deploy', 'job_id': '../test', 'confirm': True},
                    {'action': 'prepare', 'version': 'v1.0.0', 'confirm': True, 'command': 'id'}]:
            with self.assertRaises(ValueError):
                validate_request(req)

    def test_upstream_aligned_and_legacy_stable_versions(self):
        for version in ['v0.2.15', 'v1.0.6']:
            validate_request({'action': 'prepare', 'version': version, 'confirm': True})
            manifest = dict(self.manifest, version=version)
            self.assertEqual(validate_manifest(manifest, version), IMAGE + '@sha256:' + 'a' * 64)
        for version in ['v00.2.15', 'v0.2.15-rc1', 'v0.2.15;id', '../v0.2.15']:
            with self.assertRaises(ValueError):
                validate_request({'action': 'prepare', 'version': version, 'confirm': True})

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

    def test_reviewed_template_gate_cannot_be_bypassed_by_mentioning_legacy_filename(self):
        transaction = ("BEGIN; SET LOCAL lock_timeout='3s'; "
                       "SELECT '243_media_tasks_and_pricing.sql'; DELETE FROM accounts; COMMIT;")
        with patch('controller.run') as command:
            with self.assertRaisesRegex(ValueError, 'reviewed atomic'):
                self.controller.apply_reviewed_media_migration(transaction)
            command.assert_not_called()

    def test_group_availability_migration_uses_exact_reviewed_transaction(self):
        from media_migrate import group_availability_transaction
        migration = Path(__file__).resolve().parents[2] / 'backend/migrations/251_media_group_availability.sql'
        sql = migration.read_text(encoding='utf-8').strip()
        transaction = group_availability_transaction(sql, __import__('hashlib').sha256(sql.encode()).hexdigest())
        with patch('controller.run') as command:
            self.controller.apply_reviewed_media_migration(transaction)
            command.assert_called_once()
        for bad in (transaction.replace("DEFAULT 'available'", "DEFAULT 'unavailable'"),
                    transaction.replace(' COMMIT;', ' DROP TABLE groups; COMMIT;')):
            with patch('controller.run') as command, self.assertRaisesRegex(ValueError, 'reviewed atomic'):
                self.controller.apply_reviewed_media_migration(bad)
                command.assert_not_called()

    def test_upstream_v0_2_19_migration_uses_exact_reviewed_transaction(self):
        import hashlib
        from media_migrate import upstream_v0_2_19_transaction, UPSTREAM_V0_2_19_FILES
        root = Path(__file__).resolve().parents[2] / 'backend/migrations'
        files = {}
        for name in UPSTREAM_V0_2_19_FILES:
            sql = (root / name).read_text(encoding='utf-8').strip()
            files[name] = (sql, hashlib.sha256(sql.encode()).hexdigest())
        transaction = upstream_v0_2_19_transaction(files)
        with patch('controller.run') as command:
            self.controller.apply_reviewed_media_migration(transaction)
            command.assert_called_once()
        for bad in (transaction.replace('DEFAULT false', 'DEFAULT true'),
                    transaction.replace(' COMMIT;', ' DROP TABLE users; COMMIT;')):
            with patch('controller.run') as command, self.assertRaisesRegex(ValueError, 'reviewed atomic'):
                self.controller.apply_reviewed_media_migration(bad)
                command.assert_not_called()

    def test_task_index_rehearsal_can_fingerprint_media_but_not_arbitrary_tables(self):
        for table in ('billing_usage_entries', 'user_affiliate_ledger', 'media_tasks',
                      'media_ledger_entries', 'media_cost_entries', 'media_artifacts'):
            self.assertIn('"' + table + '"', self.controller.fingerprint_sql(table))
        with self.assertRaises(ValueError): self.controller.fingerprint_sql('unreviewed_table')

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

    def test_docs_image_is_optional_for_legacy_manifests(self):
        legacy = self.controller.override(IMAGE + '@sha256:' + 'a' * 64)
        self.assertNotIn('docs-web', json.loads(legacy.read_text())['services'])

    def test_docs_override_has_stable_container_name(self):
        overlay = self.controller.override(
            IMAGE + '@sha256:' + 'a' * 64,
            'ghcr.io/example/docs@sha256:' + 'b' * 64)
        service = json.loads(overlay.read_text())['services']['docs-web']
        self.assertEqual(service['container_name'], 'sub2api-docs-web')

    def test_v127_manifest_requires_docs_image_digest(self):
        manifest = dict(self.manifest, version='v1.2.7')
        with self.assertRaisesRegex(ValueError, 'documentation image identity'):
            validate_manifest(manifest, 'v1.2.7')

    def test_stale_production_aborts_before_backup_or_switch(self):
        job = {'id': 'a' * 32, 'version': 'v1.0.0', 'restore_verified': True, 'rollback_verified': True,
               'original_container': 'old', 'original_started': 'old'}
        backup = Path(self.tmp.name) / 'backups' / job['id']
        backup.mkdir(parents=True)
        atomic_json(backup / 'checksums.json', {})
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
