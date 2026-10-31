import hashlib
import json
import unittest
from pathlib import Path
from unittest.mock import Mock
from media_migrate import reviewed_transaction, tracked_operation, unified_transaction, validate_unified_transaction, UNIFIED_FILES
from media_migrate import monitor_transaction, validate_monitor_transaction, reviewed_monitor_upgrade, MONITOR_FILE
from media_migrate import task_index_transaction, validate_task_index_transaction, reviewed_upgrade, TASK_INDEX_FILE
from media_migrate import docs_transaction, validate_docs_transaction, reviewed_docs_upgrade, DOCS_FILE
from media_migrate import (group_availability_transaction, validate_group_availability_transaction,
                           reviewed_group_availability_upgrade, GROUP_AVAILABILITY_FILE, group_availability_statements)
from media_migrate import (TEMPLATE_FILES, TEMPLATE_SCHEMA_FILE, LEGACY_TEMPLATE_FILE, TEMPLATE_UPGRADE,
                          template_transaction, template_transaction_files, validate_template_transaction,
                          legacy_accounts_fingerprint_sql)
from media_migrate import (SCOPED_ADMIN_KEYS_FILE, IMAGE_COUNT_BILLING_FILE, UPSTREAM_V0_2_19_FILES,
                           UPSTREAM_V0_2_19_UPGRADE, upstream_v0_2_19_statements,
                           upstream_v0_2_19_transaction, validate_upstream_v0_2_19_transaction)
import tempfile

class MediaMigrationGateTests(unittest.TestCase):
    def upstream_files(self):
        root = Path(__file__).resolve().parents[2] / 'backend/migrations'
        return {name: ((root / name).read_text(encoding='utf-8').strip(),
                       hashlib.sha256((root / name).read_text(encoding='utf-8').strip().encode()).hexdigest())
                for name in UPSTREAM_V0_2_19_FILES}

    def test_actual_v1319_upgrade_includes_all_seven_upstream_migrations(self):
        self.assertEqual(UPSTREAM_V0_2_19_FILES, (
            '242_group_i18n.sql', '243_gemini_cached_contents.sql',
            '244_gemini_cached_contents_channel_usage.sql',
            '245_gemini_cached_contents_mutation_lease.sql',
            '246_enable_gemini_image_generation_groups.sql',
            '253_scoped_admin_api_keys.sql', '254_group_image_count_billing.sql'))
        files = self.upstream_files()
        with tempfile.TemporaryDirectory() as folder:
            for name, (sql, _) in files.items():
                Path(folder, name).write_text(sql, encoding='utf-8')
            # Preserve the two-file route for databases with the five upstream
            # prerequisites already applied, while rejecting incomplete bundles.
            from media_migrate import UPSTREAM_V0_2_19_CORE_FILES
            before = {name: checksum for name, (_, checksum) in files.items()
                      if name not in UPSTREAM_V0_2_19_CORE_FILES}
            target = {name: checksum for name, (_, checksum) in files.items()}
            reviewed = reviewed_upgrade(before, target, folder)
            self.assertEqual(reviewed['files'], list(UPSTREAM_V0_2_19_CORE_FILES))
            self.assertTrue(validate_upstream_v0_2_19_transaction(reviewed['transaction']))
            for omitted in UPSTREAM_V0_2_19_FILES:
                with self.subTest(omitted=omitted), self.assertRaises(ValueError):
                    reviewed_upgrade({}, {k: v for k, v in target.items() if k != omitted}, folder)

    def test_upstream_v0_2_19_migrations_are_exact_atomic_and_default_image_billing_off(self):
        files = self.upstream_files()
        transaction = upstream_v0_2_19_transaction(files)
        self.assertTrue(validate_upstream_v0_2_19_transaction(transaction))
        self.assertIn('REFERENCES users(id)', transaction)
        self.assertIn('ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ', transaction)
        self.assertIn('image_count_billing BOOLEAN NOT NULL DEFAULT false', transaction)
        self.assertFalse(validate_upstream_v0_2_19_transaction(transaction.replace(' DEFAULT false', ' DEFAULT true')))
        self.assertFalse(validate_upstream_v0_2_19_transaction(transaction.replace(' COMMIT;', ' DROP TABLE users; COMMIT;')))
        expected = upstream_v0_2_19_statements()
        for name, (sql, checksum) in files.items():
            bad = sql + ' DELETE FROM users;'
            modified = dict(files)
            modified[name] = (bad, hashlib.sha256(bad.encode()).hexdigest())
            with self.subTest(name=name), self.assertRaises(ValueError):
                upstream_v0_2_19_transaction(modified)
            self.assertIn(name, expected)

    def test_upstream_v0_2_19_upgrade_preserves_old_checksums_and_requires_both_files(self):
        files = self.upstream_files()
        before = {'247_media_task_list_indexes.sql': 'b' * 64, '252_media_account_concurrency_diagnostics.sql': 'c' * 64}
        target = {**before, **{name: checksum for name, (_, checksum) in files.items()}}
        with tempfile.TemporaryDirectory() as folder:
            for name, (sql, _) in files.items():
                Path(folder, name).write_text(sql, encoding='utf-8')
            reviewed = reviewed_upgrade(before, target, folder)
            self.assertEqual(reviewed['filename'], UPSTREAM_V0_2_19_UPGRADE)
            self.assertEqual(reviewed['files'], list(UPSTREAM_V0_2_19_FILES))
            self.assertEqual(reviewed['expected_migrations'], target)
            self.assertEqual({k: v for k, v in reviewed['expected_migrations'].items() if k in before}, before)
            self.assertIsNone(reviewed_upgrade(target, target, folder))
            for bad in ({**target, '247_media_task_list_indexes.sql': 'd' * 64},
                        {**before, SCOPED_ADMIN_KEYS_FILE: target[SCOPED_ADMIN_KEYS_FILE]},
                        {**target, IMAGE_COUNT_BILLING_FILE: 'e' * 64},
                        {**target, 'unexpected.sql': 'f' * 64}):
                with self.subTest(target=bad), self.assertRaises(ValueError):
                    reviewed_upgrade(before, bad, folder)

    def test_old_worker_loads_upstream_migrations_only_from_matching_release_assets(self):
        files = self.upstream_files()
        before = {'old.sql': 'b' * 64}
        target = {**before, **{name: checksum for name, (_, checksum) in files.items()}}
        with tempfile.TemporaryDirectory() as temporary:
            state = Path(temporary)
            release, assets = state / 'releases/v1.3.20', state / 'automatic/receipt-assets'
            release.mkdir(parents=True)
            assets.mkdir(parents=True)
            for name, (sql, _) in files.items():
                (assets / name).write_text(sql, encoding='utf-8')
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.3.20', 'migrations': target}))
            result = reviewed_upgrade(before, target, release)
            self.assertTrue(validate_upstream_v0_2_19_transaction(result['transaction']))
            (assets / IMAGE_COUNT_BILLING_FILE).write_text(files[IMAGE_COUNT_BILLING_FILE][0] + ' DROP TABLE users;')
            with self.assertRaises(ValueError):
                reviewed_upgrade(before, target, release)
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.3.19', 'migrations': target}))
            with self.assertRaises(FileNotFoundError):
                reviewed_upgrade(before, target, release)

    @staticmethod
    def template_files():
        root = Path(__file__).resolve().parents[2] / 'backend/migrations'
        files = {}
        for name in TEMPLATE_FILES:
            sql = (root / name).read_text(encoding='utf-8').strip()
            files[name] = (sql, hashlib.sha256(sql.encode()).hexdigest())
        return files

    def test_media_template_upgrade_requires_exact_statements_not_release_provided_checksums_alone(self):
        files = self.template_files()
        transaction = template_transaction(files)
        self.assertTrue(validate_template_transaction(transaction))
        self.assertEqual(set(template_transaction_files(transaction)), set(TEMPLATE_FILES))
        self.assertFalse(validate_template_transaction(transaction + 'DROP TABLE users;'))
        self.assertFalse(validate_template_transaction(transaction.replace(' COMMIT;', ' DELETE FROM accounts; COMMIT;')))
        for name, (sql, _) in files.items():
            for bad in (sql + ' UPDATE users SET balance=0;', sql.replace('media_models', 'credentials'),
                        sql.replace("'template','binding'", "'template','binding','unsafe'")):
                if bad == sql:
                    continue
                modified = dict(files)
                modified[name] = (bad, hashlib.sha256(bad.encode()).hexdigest())
                with self.subTest(name=name), self.assertRaises(ValueError):
                    template_transaction(modified)
        projection = legacy_accounts_fingerprint_sql(files[LEGACY_TEMPLATE_FILE][0])
        self.assertIn('jsonb_populate_record(NULL::accounts', projection)
        self.assertIn('LEFT JOIN migrated ON a.id = migrated.id', projection)
        self.assertNotIn('UPDATE accounts', projection)
        with self.assertRaises(ValueError):
            legacy_accounts_fingerprint_sql(files[LEGACY_TEMPLATE_FILE][0] + 'DELETE FROM users;')

    def test_media_template_inventory_preserves_old_checksums_and_supports_partial_and_repeated_upgrade(self):
        files, before = self.template_files(), {'old.sql': 'b' * 64}
        target = {**before, **{name: checksum for name, (_, checksum) in files.items()}}
        with tempfile.TemporaryDirectory() as folder:
            for name, (sql, _) in files.items():
                Path(folder, name).write_text(sql, encoding='utf-8')
            result = reviewed_upgrade(before, target, folder)
            self.assertEqual(result['filename'], TEMPLATE_UPGRADE)
            self.assertEqual(result['files'], list(TEMPLATE_FILES))
            self.assertEqual(result['expected_migrations'], target)
            self.assertIsNone(reviewed_upgrade(target, target, folder))
            schema_only = {**before, TEMPLATE_SCHEMA_FILE: target[TEMPLATE_SCHEMA_FILE]}
            self.assertEqual(reviewed_upgrade(before, schema_only, folder)['files'], [TEMPLATE_SCHEMA_FILE])
            self.assertEqual(reviewed_upgrade(schema_only, target, folder)['files'], [LEGACY_TEMPLATE_FILE])
            for bad in ({**target, 'old.sql': 'c' * 64}, {**target, 'unexpected.sql': 'c' * 64},
                        {**target, LEGACY_TEMPLATE_FILE: 'c' * 64},
                        {**before, LEGACY_TEMPLATE_FILE: target[LEGACY_TEMPLATE_FILE]}):
                with self.subTest(target=bad), self.assertRaises(ValueError):
                    reviewed_upgrade(before, bad, folder)

    def test_old_worker_template_assets_require_matching_version_manifest_and_exact_sql(self):
        files, before = self.template_files(), {'old.sql': 'b' * 64}
        target = {**before, **{name: checksum for name, (_, checksum) in files.items()}}
        with tempfile.TemporaryDirectory() as temporary:
            state = Path(temporary)
            release, assets = state / 'releases/v1.2.15', state / 'automatic/receipt-assets'
            release.mkdir(parents=True)
            assets.mkdir(parents=True)
            for name, (sql, _) in files.items():
                (assets / name).write_text(sql, encoding='utf-8')
            manifest = {'version': 'v1.2.15', 'migrations': target}
            (assets / 'deployment.json').write_text(json.dumps(manifest))
            self.assertTrue(validate_template_transaction(reviewed_upgrade(before, target, release)['transaction']))
            (assets / LEGACY_TEMPLATE_FILE).write_text(files[LEGACY_TEMPLATE_FILE][0] + ' DROP TABLE users;', encoding='utf-8')
            with self.assertRaises(ValueError): reviewed_upgrade(before, target, release)
            manifest['version'] = 'v1.2.14'
            (assets / 'deployment.json').write_text(json.dumps(manifest))
            with self.assertRaises(FileNotFoundError): reviewed_upgrade(before, target, release)

    def test_public_documentation_migration_is_exact_and_additive(self):
        sql = (Path(__file__).resolve().parents[2] / 'backend/migrations' / DOCS_FILE).read_text().strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        transaction = docs_transaction(sql, checksum)
        self.assertTrue(validate_docs_transaction(transaction))
        self.assertFalse(validate_docs_transaction(transaction.replace(' COMMIT;', ' DROP TABLE users; COMMIT;')))
        with tempfile.TemporaryDirectory() as folder:
            Path(folder, DOCS_FILE).write_text(sql)
            before = {'247_media_task_list_indexes.sql': 'b' * 64}
            target = {**before, DOCS_FILE: checksum}
            reviewed = reviewed_docs_upgrade(before, target, folder)
            self.assertEqual(reviewed['transaction'], transaction)
            self.assertEqual(reviewed['expected_migrations'], target)
            self.assertIsNone(reviewed_docs_upgrade(target, target, folder))
            for bad in ({**target, 'unexpected.sql': 'a' * 64}, {**target, DOCS_FILE: 'a' * 64}):
                with self.assertRaises(ValueError): reviewed_docs_upgrade(before, bad, folder)

    def test_group_availability_migration_is_exact_additive_and_defaulted_available(self):
        root = Path(__file__).resolve().parents[2] / 'backend/migrations'
        sql = (root / GROUP_AVAILABILITY_FILE).read_text(encoding='utf-8').strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        transaction = group_availability_transaction(sql, checksum)
        self.assertTrue(validate_group_availability_transaction(transaction))
        self.assertIn("DEFAULT 'available'", transaction)
        self.assertIn("CHECK (media_availability IN ('available', 'unavailable'))", transaction)
        self.assertFalse(validate_group_availability_transaction(transaction.replace(' COMMIT;', ' DROP TABLE groups; COMMIT;')))
        for bad in (sql + ' UPDATE groups SET media_availability=\'unavailable\';',
                    sql.replace("'unavailable'", "'disabled'"), sql.replace('VARCHAR(20)', 'TEXT')):
            with self.subTest(sql=bad), self.assertRaises(ValueError):
                group_availability_transaction(bad, hashlib.sha256(bad.encode()).hexdigest())
        with tempfile.TemporaryDirectory() as folder:
            Path(folder, GROUP_AVAILABILITY_FILE).write_text(sql, encoding='utf-8')
            before = {'old.sql': 'b' * 64}
            target = {**before, GROUP_AVAILABILITY_FILE: checksum}
            reviewed = reviewed_upgrade(before, target, folder)
            self.assertEqual(reviewed['filename'], GROUP_AVAILABILITY_FILE)
            self.assertEqual(reviewed['transaction'], transaction)
            self.assertEqual(reviewed['expected_migrations'], target)
            self.assertIsNone(reviewed_upgrade(target, target, folder))
            for bad in ({**target, 'old.sql': 'a' * 64}, {**target, 'unexpected.sql': 'a' * 64},
                        {**target, GROUP_AVAILABILITY_FILE: 'a' * 64}):
                with self.subTest(target=bad), self.assertRaises(ValueError):
                    reviewed_group_availability_upgrade(before, bad, folder)

    def test_old_worker_stages_media_group_availability_only_for_matching_release(self):
        sql = (Path(__file__).resolve().parents[2] / 'backend/migrations' / GROUP_AVAILABILITY_FILE).read_text().strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        before = {'old.sql': 'b' * 64}
        target = {**before, GROUP_AVAILABILITY_FILE: checksum}
        with tempfile.TemporaryDirectory() as temporary:
            state = Path(temporary)
            release = state / 'releases/v1.3.2'
            release.mkdir(parents=True)
            assets = state / 'automatic/receipt-assets'
            assets.mkdir(parents=True)
            (assets / GROUP_AVAILABILITY_FILE).write_text(sql, encoding='utf-8')
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.3.2', 'migrations': target}))
            self.assertTrue(validate_group_availability_transaction(
                reviewed_upgrade(before, target, release)['transaction']))
            (assets / GROUP_AVAILABILITY_FILE).write_text(sql + ' DROP TABLE users;', encoding='utf-8')
            with self.assertRaises(ValueError): reviewed_upgrade(before, target, release)
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.3.1', 'migrations': target}))
            with self.assertRaises(FileNotFoundError): reviewed_upgrade(before, target, release)

    def test_task_indexes_require_exact_sql_and_preserve_old_migrations(self):
        sql = (Path(__file__).resolve().parents[2] / 'backend/migrations' / TASK_INDEX_FILE).read_text().strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        transaction = task_index_transaction(sql, checksum)
        self.assertTrue(validate_task_index_transaction(transaction))
        self.assertFalse(validate_task_index_transaction(transaction + 'DROP TABLE users;'))
        for bad in (sql + 'DELETE FROM media_tasks;', sql.replace('DESC', 'ASC'), sql.replace('media_tasks(', 'users(')):
            with self.subTest(sql=bad), self.assertRaises(ValueError):
                task_index_transaction(bad, checksum)
        with tempfile.TemporaryDirectory() as folder:
            Path(folder, TASK_INDEX_FILE).write_text(sql)
            before = {'old.sql': 'b' * 64}
            target = {**before, TASK_INDEX_FILE: checksum}
            self.assertEqual(reviewed_upgrade(before, target, folder)['transaction'], transaction)
            self.assertIsNone(reviewed_upgrade(target, target, folder))
            for bad in ({**target, 'old.sql': 'a' * 64}, {**target, 'unexpected.sql': 'a' * 64}, {**target, TASK_INDEX_FILE: 'a' * 64}, {TASK_INDEX_FILE: checksum}):
                with self.assertRaises(ValueError): reviewed_upgrade(before, bad, folder)

    def test_task_index_assets_from_previous_worker_match_release_and_checksum(self):
        sql = (Path(__file__).resolve().parents[2] / 'backend/migrations' / TASK_INDEX_FILE).read_text().strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        before = {'old.sql': 'b' * 64}
        target = {**before, TASK_INDEX_FILE: checksum}
        with tempfile.TemporaryDirectory() as temporary:
            state = Path(temporary)
            release = state / 'releases/v1.2.4'
            release.mkdir(parents=True)
            assets = state / 'automatic/receipt-assets'
            assets.mkdir(parents=True)
            (assets / TASK_INDEX_FILE).write_text(sql)
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.2.4', 'migrations': target}))
            self.assertTrue(validate_task_index_transaction(reviewed_upgrade(before, target, release)['transaction']))
            (assets / TASK_INDEX_FILE).write_text(sql + ' DROP TABLE users;')
            with self.assertRaises(ValueError): reviewed_upgrade(before, target, release)
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.2.3', 'migrations': target}))
            with self.assertRaises(FileNotFoundError): reviewed_upgrade(before, target, release)

    def test_previous_worker_assets_are_accepted_only_for_matching_release_and_exact_sql(self):
        sql = (Path(__file__).resolve().parents[2] / 'backend/migrations' / MONITOR_FILE).read_text().strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        before = {'old.sql': 'b' * 64}
        target = {**before, MONITOR_FILE: checksum}
        with tempfile.TemporaryDirectory() as temporary:
            state = Path(temporary)
            release = state / 'releases/v1.2.2'
            release.mkdir(parents=True)
            assets = state / 'automatic/receipt-assets'
            assets.mkdir(parents=True)
            (assets / MONITOR_FILE).write_text(sql)
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.2.2', 'migrations': target}))
            self.assertTrue(validate_monitor_transaction(reviewed_monitor_upgrade(before, target, release)['transaction']))
            (assets / MONITOR_FILE).write_text(sql + ' DROP TABLE users;')
            with self.assertRaises(ValueError): reviewed_monitor_upgrade(before, target, release)
            (assets / 'deployment.json').write_text(json.dumps({'version': 'v1.2.1', 'migrations': target}))
            with self.assertRaises(FileNotFoundError): reviewed_monitor_upgrade(before, target, release)

    def test_monitor_expansion_requires_exact_sql_inventory_and_checksum(self):
        root = Path(__file__).resolve().parents[2] / 'backend/migrations'
        sql = (root / MONITOR_FILE).read_text().strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        transaction = monitor_transaction(sql, checksum)
        self.assertTrue(validate_monitor_transaction(transaction))
        self.assertFalse(validate_monitor_transaction(transaction.replace(' COMMIT;', ' DROP TABLE accounts; COMMIT;')))
        for bad in (sql + ' UPDATE accounts SET status=\'disabled\';', sql.replace("'pending'", "'unknown'"), sql.replace('DROP CONSTRAINT', 'DROP TABLE')):
            with self.assertRaises(ValueError): monitor_transaction(bad, checksum)
        with tempfile.TemporaryDirectory() as folder:
            Path(folder, MONITOR_FILE).write_text(sql)
            before = {'old.sql': 'b' * 64}
            target = {**before, MONITOR_FILE: checksum}
            self.assertEqual(reviewed_monitor_upgrade(before, target, folder)['transaction'], transaction)
            self.assertIsNone(reviewed_monitor_upgrade(target, target, folder))
            for bad in ({**target, 'unexpected.sql': 'a' * 64}, {**target, 'old.sql': 'a' * 64}, {**target, MONITOR_FILE: 'a' * 64}):
                with self.assertRaises(ValueError): reviewed_monitor_upgrade(before, bad, folder)

    def test_unified_pricing_real_migrations_roundtrip_with_comments(self):
        root = Path(__file__).resolve().parents[2] / 'backend/migrations'
        files = {}
        for name in UNIFIED_FILES:
            sql = (root / name).read_text().replace('\r\n', '\n').strip()
            files[name] = (sql, hashlib.sha256(sql.encode()).hexdigest())
        transaction = unified_transaction(files)
        self.assertTrue(validate_unified_transaction(transaction))
        self.assertNotIn('--', transaction)
        for name, (_, checksum) in files.items():
            self.assertIn("VALUES('" + name + "','" + checksum + "')", transaction)

    def test_unified_pricing_allows_only_exact_nullable_columns(self):
        files={name:("ALTER TABLE "+table+" ADD COLUMN IF NOT EXISTS media JSONB;",'a'*64) for name,table in UNIFIED_FILES.items()}
        transaction=unified_transaction(files)
        self.assertTrue(transaction.startswith('BEGIN;'))
        self.assertEqual(transaction.count('INSERT INTO schema_migrations'),2)
        self.assertTrue(validate_unified_transaction(transaction))
        self.assertFalse(validate_unified_transaction(transaction.replace(' COMMIT;', ' DROP TABLE users; COMMIT;')))
        for suffix in ('DROP TABLE users;', 'UPDATE users SET balance=0;', 'ALTER TABLE users ADD COLUMN media JSONB;'):
            invalid=dict(files)
            name=next(iter(files)); invalid[name]=(files[name][0]+suffix,'a'*64)
            with self.assertRaises(ValueError):unified_transaction(invalid)
    def test_exact_additive_schema_has_atomic_transaction(self):
        sql=(Path(__file__).resolve().parents[2]/'backend/migrations/243_media_tasks_and_pricing.sql').read_text()
        transaction=reviewed_transaction(sql,'a'*64)
        self.assertTrue(transaction.startswith('BEGIN;'))
        self.assertIn("lock_timeout='3s'",transaction)
        self.assertIn('INSERT INTO schema_migrations',transaction)
        self.assertTrue(transaction.endswith('COMMIT;'))
    def test_destructive_or_business_updates_cannot_be_reviewed(self):
        sql=(Path(__file__).resolve().parents[2]/'backend/migrations/243_media_tasks_and_pricing.sql').read_text()
        for extra in ('DROP TABLE users;', 'UPDATE users SET balance=0;', 'DELETE FROM api_keys;', 'TRUNCATE users;', "INSERT INTO users(email) VALUES('bad');", 'ALTER TABLE users ADD COLUMN unsafe text;', 'CREATE TABLE unrelated(id int);'):
            with self.subTest(extra=extra),self.assertRaises(ValueError):reviewed_transaction(sql+extra,'a'*64)

    def test_migration_failure_is_saved_with_phase_and_without_secret_text(self):
        manager = Mock()
        manager.save.side_effect = lambda job, status: job.update(status=status)
        with self.assertRaises(ValueError):
            with tracked_operation(manager, 'apply', 'v1.1.0') as job:
                manager.save(job, 'draining')
                job['backup_id'] = 'b' * 32
                raise ValueError('sensitive credential must not enter job JSON')
        self.assertEqual(job['status'], 'failed_needs_inspection')
        self.assertEqual(job['failed_phase'], 'draining')
        self.assertEqual(job['backup_id'], 'b' * 32)
        self.assertEqual(job['error_type'], 'ValueError')
        self.assertNotIn('sensitive credential', str(job))

    def test_success_is_not_reclassified_as_failure(self):
        manager = Mock()
        manager.save.side_effect = lambda job, status: job.update(status=status)
        with tracked_operation(manager, 'rehearse', 'v1.1.0') as job:
            manager.save(job, 'succeeded')
        self.assertEqual(job['action'], 'media_migration_rehearsal')
        self.assertEqual(job['status'], 'succeeded')

if __name__=='__main__':unittest.main()
