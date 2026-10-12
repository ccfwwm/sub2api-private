import unittest
from pathlib import Path
from unittest.mock import Mock
from media_migrate import reviewed_transaction, tracked_operation

class MediaMigrationGateTests(unittest.TestCase):
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
