#!/usr/bin/env python3
"""Prepare native pricing sync, then remove the exact retired container after cutover.

Run prepare before an ordinary managed application upgrade and finalize only
after that upgrade is healthy. Backups and the old directory remain on disk.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import urllib.request

PRODUCTION = Path('/root/sub2api-deploy')
BACKUP = Path('/root/sub2api-ops/state/native-pricing-cutover-20261016')
REMOTE = 'https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json'
HASH = REMOTE.replace('.json', '.sha256')
OVERRIDE = '/app/data/pricing/domestic-pricing-overrides.json'
CONTAINER = 'sub2api-pricing-overlay'


def compose_without_overlay(text):
    replacements = {
        'PRICING_REMOTE_URL=http://pricing-overlay:8090/model-pricing.json': 'PRICING_REMOTE_URL=' + REMOTE,
        'PRICING_HASH_URL=http://pricing-overlay:8090/model-pricing.sha256': 'PRICING_HASH_URL=' + HASH + '\n      - PRICING_OVERRIDE_FILE=' + OVERRIDE,
    }
    for old, new in replacements.items():
        if text.count(old) != 1:
            raise ValueError('unexpected pricing source configuration')
        text = text.replace(old, new)
    text, count = re.subn(r'(?m)^      pricing-overlay:\r?\n        condition: service_healthy\r?\n', '', text)
    if count != 1:
        raise ValueError('unexpected pricing dependency')
    text, count = re.subn(r'(?ms)^  pricing-overlay:\r?\n.*?(?=^  [A-Za-z0-9_-]+:\s*$|^volumes:|^networks:|\Z)', '', text)
    if count != 1 or 'http://pricing-overlay:' in text or 'container_name: ' + CONTAINER in text:
        raise ValueError('unexpected pricing service configuration')
    text = text.replace('      # Global RMB pricing overlay: keeps channel-independent domestic model pricing\n'
                        '      # while preserving automatic updates from the upstream price catalog.',
                        '      # Native catalog sync with existing domestic pricing overrides.')
    return text


def run(args):
    return subprocess.check_output(args, stderr=subprocess.PIPE)


def application():
    return json.loads(run(['docker', 'inspect', 'sub2api']))[0]


def compose_check():
    run(['docker', 'compose', '--project-name', 'sub2api-deploy', '--project-directory', str(PRODUCTION),
         '--env-file', str(PRODUCTION / '.env'), '-f', str(PRODUCTION / 'docker-compose.yml'),
         '-f', str(PRODUCTION / 'docker-compose.override.yml'), '-f', '/root/sub2api-ops/state/active.json',
         'config', '--quiet'])


def prepare():
    if BACKUP.exists():
        raise ValueError('cutover backup already exists; inspect before repeating')
    original = (PRODUCTION / 'docker-compose.yml').read_text()
    updated = compose_without_overlay(original)
    source = PRODUCTION / 'pricing-overlay/domestic-pricing-overrides.json'
    overrides = json.loads(source.read_text())
    if not overrides or not isinstance(overrides, dict):
        raise ValueError('invalid existing overrides')
    # Verify the upstream can be read directly before removing the relay.
    with urllib.request.urlopen(REMOTE, timeout=45) as response:
        catalog = json.load(response)
    with urllib.request.urlopen(HASH, timeout=30) as response:
        if not re.search(r'[0-9a-f]{64}', response.read().decode()):
            raise ValueError('invalid upstream pricing hash')
    if not isinstance(catalog, dict) or not catalog:
        raise ValueError('invalid upstream catalog')
    BACKUP.mkdir(mode=0o700, parents=True)
    for name in ['docker-compose.yml', 'docker-compose.override.yml', '.env']:
        shutil.copyfile(PRODUCTION / name, BACKUP / name)
    shutil.copyfile(source, BACKUP / source.name)
    target = PRODUCTION / 'data/pricing/domestic-pricing-overrides.json'
    target.parent.mkdir(mode=0o755, exist_ok=True)
    if target.exists():
        raise ValueError('native override file already exists; do not overwrite')
    shutil.copyfile(source, target)
    os.chmod(target, 0o644)
    proof = {'overrides': sorted(overrides), 'override_sha256': hashlib.sha256(source.read_bytes()).hexdigest(),
             'remote_url': REMOTE, 'hash_url': HASH, 'override_file': OVERRIDE,
             'compose_before_sha256': hashlib.sha256((PRODUCTION / 'docker-compose.yml').read_bytes()).hexdigest(),
             'phase': 'prepared'}
    try:
        (PRODUCTION / 'docker-compose.yml').write_text(updated)
        compose_check()
    except BaseException:
        shutil.copyfile(BACKUP / 'docker-compose.yml', PRODUCTION / 'docker-compose.yml')
        raise
    (BACKUP / 'receipt.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


def finalize():
    proof = json.loads((BACKUP / 'receipt.json').read_text())
    c = application()
    env = dict(item.split('=', 1) for item in c['Config']['Env'] if '=' in item)
    if env.get('PRICING_REMOTE_URL') != REMOTE or env.get('PRICING_HASH_URL') != HASH or env.get('PRICING_OVERRIDE_FILE') != OVERRIDE:
        raise ValueError('application has not switched to native pricing')
    if c['State'].get('Health', {}).get('Status') != 'healthy':
        raise ValueError('application is not healthy')
    target = PRODUCTION / 'data/pricing/domestic-pricing-overrides.json'
    if hashlib.sha256(target.read_bytes()).hexdigest() != proof['override_sha256']:
        raise ValueError('override content changed')
    compose_check()
    inspect = subprocess.run(['docker', 'inspect', CONTAINER], capture_output=True)
    if inspect.returncode == 0:
        retired = json.loads(inspect.stdout)[0]
        labels = retired['Config'].get('Labels', {})
        if labels.get('com.docker.compose.project') != 'sub2api-deploy' or labels.get('com.docker.compose.service') != 'pricing-overlay':
            raise ValueError('unexpected container owner')
        run(['docker', 'stop', '--time', '15', CONTAINER])
        run(['docker', 'rm', CONTAINER])
    if subprocess.run(['docker', 'inspect', CONTAINER], capture_output=True).returncode == 0:
        raise ValueError('retired container still exists')
    proof['phase'] = 'completed'
    (BACKUP / 'receipt.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'finalize'])
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit('root required')
    os.umask(0o077)
    (prepare if args.action == 'prepare' else finalize)()
