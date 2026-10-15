import hashlib
import json
from pathlib import Path
import runpy
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


class ReleaseManifestTests(unittest.TestCase):
    def test_verification_uses_committed_bytes_instead_of_checkout_line_endings(self):
        script = Path(__file__).with_name('release_manifest.py')
        root = script.parents[2]
        blob = subprocess.check_output(
            ['git', 'show', 'HEAD:docs/multimodal/CHANNEL_VERIFICATION_CURRENT.json'], cwd=root)
        record = json.loads(blob)
        checkout = root / 'docs/multimodal/CHANNEL_VERIFICATION_CURRENT.json'
        original_read = Path.read_text

        def read_checkout(path, *args, **kwargs):
            if path == checkout:
                raise AssertionError('Verification must read the committed Git blob')
            return original_read(path, *args, **kwargs)

        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'deployment.json'
            args = [str(script), '--version', record['version'], '--digest', 'sha256:' + 'b' * 64,
                    '--docs-digest', 'sha256:' + 'c' * 64, '--output', str(output)]
            with patch.object(sys, 'argv', args), patch.object(Path, 'read_text', read_checkout):
                runpy.run_path(str(script), run_name='__main__')
            manifest = json.loads(output.read_text())
        self.assertEqual(manifest['channel_verification_record']['sha256'], hashlib.sha256(blob).hexdigest())
        self.assertNotEqual(hashlib.sha256(blob.replace(b'\r\n', b'\n').replace(b'\n', b'\r\n')).hexdigest(),
                            manifest['channel_verification_record']['sha256'])


if __name__ == '__main__':
    unittest.main()
