"""Opt-in SQL gate rehearsal on a dedicated database in an existing test PG18.

Set MEDIA_MIGRATION_TEST_PG_CONTAINER to a disposable local PostgreSQL container.
The test creates and drops only its own randomly named database, never app data.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import unittest
import uuid

from controller import Controller
from media_migrate import (TEMPLATE_FILES, LEGACY_TEMPLATE_FILE, template_transaction,
                          legacy_accounts_fingerprint_sql, reviewed_upgrade)


@unittest.skipUnless(os.environ.get('MEDIA_MIGRATION_TEST_PG_CONTAINER'), 'dedicated PostgreSQL fixture not selected')
class TemplateMigrationSQLTests(unittest.TestCase):
    def test_exact_account_projection_idempotence_and_immutable_task_data(self):
        container = os.environ['MEDIA_MIGRATION_TEST_PG_CONTAINER']
        database = 'media_migration_gate_' + uuid.uuid4().hex
        self.assertRegex(database, r'^media_migration_gate_[0-9a-f]{32}$')
        subprocess.run(['docker', 'exec', container, 'createdb', '-U', 'postgres', database], check=True, capture_output=True)
        def sql(query):
            result = subprocess.run(['docker', 'exec', '-i', container, 'psql', '-X', '-qAt',
                                     '-v', 'ON_ERROR_STOP=1', '-U', 'postgres', '-d', database],
                                    input=query.encode(), capture_output=True, check=True)
            return result.stdout.decode().strip()
        try:
            self.assertEqual(sql("SELECT current_setting('server_version_num')::int / 10000;"), '18')
            sql("""
                CREATE TABLE accounts(id bigint PRIMARY KEY, platform text, extra jsonb,
                    credentials jsonb NOT NULL DEFAULT '{"preserve":"fixture-secret"}', enabled boolean DEFAULT true);
                CREATE TABLE schema_migrations(filename text PRIMARY KEY, checksum text NOT NULL);
                CREATE TABLE media_config_versions(kind text CHECK(kind IN ('capability','price')), config jsonb);
                CREATE TABLE media_artifacts(id text PRIMARY KEY, locator text NOT NULL);
                CREATE TABLE media_tasks(id text PRIMARY KEY, snapshot jsonb, upstream_id text);
                INSERT INTO media_tasks VALUES('old-task','{"upstream":"old-config","template_ref":{"id":"old","version":7}}','original-id');
                INSERT INTO media_artifacts VALUES('old-file','immutable-locator');
                INSERT INTO media_config_versions VALUES('capability','{"original":"price-and-capability-data"}');
            """)
            models = [
                {'id': 'one', 'request_format': 'v6k_url', 'media_type': 'video', 'operations': ['video'], 'enabled': True},
                {'id': 'two', 'request_format': 'yunfei_multimodal', 'media_type': 'video', 'operations': ['video'], 'enabled': True},
                {'id': 'three', 'adapter': 'yunfei_grok', 'media_type': 'video', 'operations': ['video'], 'enabled': True},
                {'id': 'four', 'adapter': 'custom', 'media_type': 'video', 'operations': ['video'], 'enabled': True},
                {'id': 'five', 'template_ref': {'id': 'admin-copy', 'version': 12}, 'enabled': False},
                {'id': 'six', 'template_ref': {'id': 'broken', 'version': 0}, 'enabled': True},
                {'id': 'seven', 'request_format': 'unknown', 'adapter': 'yunfei_grok', 'media_type': 'video', 'operations': ['video'], 'enabled': True},
                {'id': 'eight', 'request_format': 'v6k_url', 'media_type': 'image', 'operations': ['image'], 'enabled': True},
            ]
            extra = {'media_models': models, 'other': {'keep': 'original-value'}}
            literal = json.dumps(extra, ensure_ascii=False).replace("'", "''")
            sql(f"INSERT INTO accounts(id,platform,extra) VALUES(1,'imagevideo','{literal}'),(2,'openai','{literal}'),(3,'imagevideo',NULL),(4,'imagevideo','{{\"media_models\":[]}}');")
            root = Path(__file__).resolve().parents[2] / 'backend/migrations'
            files = {name: ((root / name).read_text(encoding='utf-8').strip(), '') for name in TEMPLATE_FILES}
            files = {name: (body, hashlib.sha256(body.encode()).hexdigest()) for name, (body, _) in files.items()}
            expected = sql(legacy_accounts_fingerprint_sql(files[LEGACY_TEMPLATE_FILE][0]))
            preserved = {table: sql(Controller.fingerprint_sql(table)) for table in ('media_tasks', 'media_config_versions')}
            non_media = sql('SELECT md5(row_to_json(a)::text) FROM accounts a WHERE id=2;')
            sql(template_transaction(files))
            self.assertEqual(sql(Controller.fingerprint_sql('accounts')), expected)
            self.assertEqual(sql(legacy_accounts_fingerprint_sql(files[LEGACY_TEMPLATE_FILE][0])), expected)
            self.assertEqual(sql('SELECT md5(row_to_json(a)::text) FROM accounts a WHERE id=2;'), non_media)
            for table, before in preserved.items():
                self.assertEqual(sql(Controller.fingerprint_sql(table)), before)
            self.assertEqual(sql("SELECT id || '|' || locator || '|' || filename FROM media_artifacts;"), 'old-file|immutable-locator|')
            migrated = json.loads(sql("SELECT extra->'media_models' FROM accounts WHERE id=1;"))
            self.assertEqual([item['id'] for item in migrated], [item['id'] for item in models])
            self.assertEqual(migrated[0]['template_ref'], {'id': 'video-arrays', 'version': 1})
            self.assertEqual(migrated[1]['template_ref'], {'id': 'video-reference-arrays', 'version': 1})
            self.assertEqual(migrated[2]['template_ref'], {'id': 'video-single-image', 'version': 1})
            self.assertEqual(migrated[4], models[4])
            for index in (3, 5, 6, 7):
                self.assertFalse(migrated[index]['enabled'])
                self.assertEqual(migrated[index]['validation_status'], '待配置协议模板')
            sql(files[LEGACY_TEMPLATE_FILE][0])
            self.assertEqual(sql(Controller.fingerprint_sql('accounts')), expected)
            target = {name: checksum for name, (_, checksum) in files.items()}
            self.assertIsNone(reviewed_upgrade(target, target, root))
            with self.assertRaises(subprocess.CalledProcessError):
                sql(template_transaction(files))
            self.assertEqual(sql(Controller.fingerprint_sql('accounts')), expected)
        finally:
            subprocess.run(['docker', 'exec', container, 'dropdb', '-U', 'postgres', database],
                           check=True, capture_output=True)


if __name__ == '__main__':
    unittest.main()
