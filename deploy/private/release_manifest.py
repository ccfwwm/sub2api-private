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
parser.add_argument('--docs-digest', default='')
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
if args.docs_digest:
    manifest['docs_image'] = 'ghcr.io/ccfwwm/sub2api-private-docs'
    manifest['docs_digest'] = args.docs_digest
record_path = root / 'docs/multimodal/CHANNEL_VERIFICATION_CURRENT.json'
record = json.loads(record_path.read_text(encoding='utf-8'))
if record.get('version') != args.version or record.get('status') != 'recorded' or not record.get('cases'):
    raise SystemExit('Record current real-channel verification before publishing')
if not any(case.get('verification') in ('real_generation', 'real_connection') and case.get('status') == 'completed' for case in record['cases']):
    raise SystemExit('At least one completed real-channel verification is required')
manifest['channel_verification_record'] = {'path':'docs/multimodal/CHANNEL_VERIFICATION_CURRENT.json', 'sha256':hashlib.sha256(record_path.read_bytes()).hexdigest(), 'verified_at':record['verified_at']}
from controller import validate_manifest
validate_manifest(manifest, args.version)
Path(args.output).write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
