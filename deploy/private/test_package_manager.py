import base64
from io import BytesIO
from pathlib import Path
import re
import tarfile
import tempfile
import unittest

from package_manager import CAPACITY_MEMBERS, LEGACY_MEMBERS, package


ROOT = Path(__file__).resolve().parents[2]


class ManagerPackageTests(unittest.TestCase):
    def test_archive_keeps_legacy_outer_inventory_and_embeds_capacity_tools(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "server-management.tar.gz"
            package(ROOT, output)
            with tarfile.open(output, "r:gz") as archive:
                members = archive.getmembers()
                self.assertEqual({member.name for member in members}, set(LEGACY_MEMBERS))
                self.assertTrue(all(member.isfile() for member in members))
                installer = archive.extractfile("install-manager.sh").read().decode()

            match = re.search(r'payload = base64\.b64decode\("([A-Za-z0-9+/=]+)"\)', installer)
            self.assertIsNotNone(match)
            with tarfile.open(fileobj=BytesIO(base64.b64decode(match.group(1))), mode="r:gz") as payload:
                self.assertEqual({member.name for member in payload.getmembers()}, set(CAPACITY_MEMBERS))
                for name in CAPACITY_MEMBERS:
                    bundled = payload.extractfile(name).read()
                    source = (ROOT / "deploy" / ("private/" + name if name.endswith(".py") else name)).read_bytes()
                    self.assertEqual(bundled, source)


if __name__ == "__main__":
    unittest.main()
