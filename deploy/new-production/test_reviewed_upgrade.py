"""Fail-closed migration authorization and nontransactional index boundaries."""
from pathlib import Path
import tempfile
import unittest
import reviewed_upgrade as upgrade


class ReviewedUpgradeTests(unittest.TestCase):
    def setUp(self):
        self.sql = Path(__file__).resolve().parents[2] / 'backend/migrations'
        self.before = {'001_init.sql': 'a' * 64}
        self.target = {**self.before, **upgrade.SEALED}

    def test_only_exact_reviewed_sources_and_historical_inventory_are_accepted(self):
        files = upgrade.validate(upgrade.VERSION, self.before, self.target, self.sql)
        self.assertEqual(len(files), 6)
        for version, before, target in [
            ('v1.3.27', self.before, self.target),
            (upgrade.VERSION, self.before, {**self.target, '263_evil.sql': 'a' * 64}),
            (upgrade.VERSION, self.before, {**self.target, '001_init.sql': 'b' * 64}),
            (upgrade.VERSION, self.before, {k: v for k, v in self.target.items() if k != '001_init.sql'}),
        ]:
            with self.assertRaises(ValueError):
                upgrade.validate(version, before, target, self.sql)

    def test_tampered_source_is_rejected_even_with_a_trusted_manifest(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in upgrade.SEALED:
                (root / name).write_bytes((self.sql / name).read_bytes())
            (root / next(iter(upgrade.SEALED))).write_text('DROP TABLE users;')
            with self.assertRaises(ValueError):
                upgrade.validate(upgrade.VERSION, self.before, self.target, root)

    def test_completed_prefix_can_resume_without_reapplying_it(self):
        first = next(iter(upgrade.SEALED))
        files = upgrade.validate(upgrade.VERSION, {**self.before, first: upgrade.SEALED[first]}, self.target, self.sql)
        self.assertEqual(len(files), 5)
        self.assertNotIn(first, [item[0] for item in files])

    def test_failed_index_is_repaired_and_receipt_waits_for_valid_index(self):
        files = upgrade.validate(upgrade.VERSION, self.before, self.target, self.sql)[:1]
        calls = []
        answers = iter(['false', 'true'])
        upgrade.apply(files, lambda settings, sql: calls.append((settings, sql)), lambda _: next(answers), 'sub2api_app')
        self.assertIn('DROP INDEX CONCURRENTLY', calls[0][1])
        self.assertNotIn('BEGIN;', calls[1][1])
        self.assertIn('INSERT INTO schema_migrations', calls[2][1])
        calls.clear()
        answers = iter(['missing', 'false'])
        with self.assertRaises(ValueError):
            upgrade.apply(files, lambda settings, sql: calls.append((settings, sql)), lambda _: next(answers), 'sub2api_app')
        self.assertFalse(any('INSERT INTO schema_migrations' in sql for _, sql in calls))

    def test_invalid_application_role_cannot_reach_sql(self):
        with self.assertRaises(ValueError):
            upgrade.apply([], lambda *args: self.fail('executed SQL'), lambda _: '', 'app; DROP TABLE users;')


if __name__ == '__main__':
    unittest.main()
