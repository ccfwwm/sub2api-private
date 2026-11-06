import hashlib
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from controller import Controller, atomic_json

from media_migrate import (MONITOR_POLICY_FILE, reviewed_upgrade,
                           validate_monitor_policy_transaction, monitor_policy_transaction)


class MonitorPolicyMigrationTests(unittest.TestCase):
    def test_policy_migration_reaches_reviewed_restore_application(self):
        # Exercise the rehearsal dispatch boundary; transaction validation alone
        # cannot catch a new migration omitted from its separate allowlist.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            controller = Controller({'state_dir': str(root), 'production_dir': str(root)})
            (root / 'database.dump').write_bytes(b'fixture')
            atomic_json(root / 'protected-snapshot.json', {})
            job = {'id': 'a' * 32, 'manifest': {'migrations': {'old.sql': 'sealed'}},
                   'reviewed_media_migration': {'filename': MONITOR_POLICY_FILE, 'transaction': 'reviewed-fixture'}}
            class ApplicationReached(Exception):
                pass
            with patch('controller.run'), patch('controller.subprocess.run') as restore, \
                 patch('controller.PROTECTED', ()), patch.object(controller, 'wait_command'), \
                 patch.object(controller, 'migrations', return_value={'old.sql': 'sealed'}), \
                 patch.object(controller, 'sql', return_value='old-column-fingerprint'), \
                 patch.object(controller, 'apply_reviewed_media_migration', side_effect=ApplicationReached) as apply:
                restore.return_value.returncode = 0
                with self.assertRaises(ApplicationReached):
                    controller.rehearse(job, root)
                apply.assert_called_once_with('reviewed-fixture', 'sub2api-rehearsal-' + 'a' * 12 + '-pg')

    def test_exact_migration_and_inventory(self):
        source = Path(__file__).resolve().parents[2] / 'backend' / 'migrations' / MONITOR_POLICY_FILE
        sql = source.read_text(encoding='utf-8').strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / MONITOR_POLICY_FILE).write_text(sql, encoding='utf-8')
            before = {'old.sql': 'immutable'}
            result = reviewed_upgrade(before, {**before, MONITOR_POLICY_FILE: checksum}, Path(tmp))
            self.assertTrue(validate_monitor_policy_transaction(result['transaction']))
            self.assertFalse(validate_monitor_policy_transaction(result['transaction'].replace('CREATE TABLE', 'DROP TABLE')))
            with self.assertRaises(ValueError):
                reviewed_upgrade(before, {'old.sql': 'changed', MONITOR_POLICY_FILE: checksum}, Path(tmp))
            with self.assertRaises(ValueError):
                monitor_policy_transaction(sql + '\nDELETE FROM channel_monitors;', checksum)


if __name__ == '__main__':
    unittest.main()
