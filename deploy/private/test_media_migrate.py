import unittest
from pathlib import Path
from media_migrate import reviewed_transaction

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

if __name__=='__main__':unittest.main()
