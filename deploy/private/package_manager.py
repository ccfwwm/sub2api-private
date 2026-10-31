#!/usr/bin/env python3
"""Package the manager for both current and legacy production bootstrappers."""
import argparse
import base64
from io import BytesIO
from pathlib import Path
import tarfile


LEGACY_MEMBERS = (
    "manage.py",
    "controller.py",
    "media_migrate.py",
    "install-manager.sh",
    "SERVER-MANAGEMENT.md",
    "auto_deploy.py",
)
CAPACITY_MEMBERS = (
    "capacity_collect.py",
    "capacity_report.py",
    "docker-compose.capacity.yml",
)
CAPACITY_SOURCE_PATHS = (
    "private/capacity_collect.py",
    "private/capacity_report.py",
    "docker-compose.capacity.yml",
)
INSTALL_MARKER = "release_id=$(cat "


def archive_bytes(root, names):
    output = BytesIO()
    with tarfile.open(fileobj=output, mode="w:gz") as archive:
        for name in names:
            path = root / name
            info = archive.gettarinfo(str(path), arcname=Path(name).name)
            info.mtime = 0
            with path.open("rb") as source:
                archive.addfile(info, source)
    return output.getvalue()


def install_script(root):
    source = (root / "deploy/private/install-manager.sh").read_text(encoding="utf-8")
    if source.count(INSTALL_MARKER) != 1:
        raise ValueError("manager installer packaging marker mismatch")
    payload = base64.b64encode(archive_bytes(root / "deploy", CAPACITY_SOURCE_PATHS)).decode("ascii")
    block = f'''# Restore capacity tooling covered by this installer's release checksum.
python3 - <<'PY'
import base64, io, tarfile
from pathlib import Path
payload = base64.b64decode("{payload}")
allowed = set({CAPACITY_MEMBERS!r})
with tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz") as archive:
    members = archive.getmembers()
    if {{member.name for member in members}} != allowed or any(not member.isfile() for member in members):
        raise SystemExit("embedded capacity tool inventory mismatch")
    for member in members:
        destination = Path(member.name)
        destination.write_bytes(archive.extractfile(member).read())
        destination.chmod(0o600)
PY
'''
    return source.replace(INSTALL_MARKER, block + INSTALL_MARKER)


def package(root, output):
    root = Path(root)
    output = Path(output)
    output.parent.mkdir(parents=True, exist_ok=True)
    contents = {name: (root / "deploy/private" / name).read_bytes() for name in LEGACY_MEMBERS}
    contents["install-manager.sh"] = install_script(root).encode("utf-8")
    with tarfile.open(output, mode="w:gz") as archive:
        for name in LEGACY_MEMBERS:
            data = contents[name]
            info = tarfile.TarInfo(name)
            info.size = len(data)
            info.mode = 0o600
            info.mtime = 0
            archive.addfile(info, BytesIO(data))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    package(args.root, args.output)
