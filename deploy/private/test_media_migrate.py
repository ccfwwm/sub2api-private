import hashlib
import json
import unittest
from pathlib import Path
from unittest.mock import Mock
from media_migrate import reviewed_transaction, tracked_operation, unified_transaction, validate_unified_transaction, UNIFIED_FILES
from media_migrate import monitor_transaction, validate_monitor_transaction, reviewed_monitor_upgrade, MONITOR_FILE
import tempfile

class MediaMigrationGateTests(unittest.TestCase):
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
