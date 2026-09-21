#!/usr/bin/env python3
"""Build a release inventory from the exact checked-out commit."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--version', required=True)
parser.add_argument('--digest', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
metadata = json.loads((root / 'deploy/private-release.json').read_text())
manifest = {
    'schema': 1, 'repository': metadata['repository'], 'image': metadata['image'],
    'platform': metadata['platform'], 'version': args.version, 'digest': args.digest,
    'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root).decode().strip(),
    'upstream_commit': metadata['upstream_commit'], 'upstream_version': metadata['upstream_version'],
    'migrations': {p.name: hashlib.sha256(p.read_text(encoding='utf-8').strip().encode()).hexdigest()
                   for p in sorted((root / 'backend/migrations').glob('*.sql'))}
}
from controller import validate_manifest
validate_manifest(manifest, args.version)
Path(args.output).write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
