import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from controller import atomic_json, file_hash
from manage import Manager, ident


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


if __name__ == '__main__':
    unittest.main()
