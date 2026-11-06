import hashlib
from pathlib import Path
import tempfile
import unittest

from media_migrate import (MONITOR_POLICY_FILE, reviewed_upgrade,
                           validate_monitor_policy_transaction, monitor_policy_transaction)


class MonitorPolicyMigrationTests(unittest.TestCase):
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
