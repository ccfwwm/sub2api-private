#!/usr/bin/env python3
"""Exercise prepare, restore, cutover and rollback on a disposable CI fixture.

This program deliberately refuses to run outside GitHub Actions. It has no SSH
client or production credentials. Container names match the controller contract
only on a fresh hosted runner.
"""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import uuid

from controller import Controller, run


def main():
    if os.environ.get('GITHUB_ACTIONS') != 'true' or os.environ.get('RUNNER_ENVIRONMENT') != 'github-hosted':
        raise SystemExit('Disposable GitHub-hosted CI runner required')
    parser = argparse.ArgumentParser()
    parser.add_argument('--manifest', required=True)
    args = parser.parse_args()
    manifest = json.loads(Path(args.manifest).read_text())
    image = manifest['image'] + '@' + manifest['digest']
    for name in ['sub2api', 'sub2api-postgres', 'sub2api-redis']:
        if subprocess.run(['docker', 'inspect', name], capture_output=True).returncode == 0:
            raise SystemExit('Refusing to touch existing container ' + name)
    with tempfile.TemporaryDirectory(prefix='sub2api-fixture-') as tmp:
        root = Path(tmp)
        prod = root / 'production'
        prod.mkdir()
        for name in ['data', 'pricing-overlay']:
            (prod / name).mkdir()
        (prod / '.env').write_text('')
        (prod / 'docker-compose.override.yml').write_text('{}')
        release_token = root / 'release-token'
        release_token.write_text('')
        os.chown(release_token, 0, 1000)
        release_token.chmod(0o440)
        # Allow the non-root app to read the bind-mounted empty CI token.
        root.chmod(0o755)
        password = secrets.token_hex(24)
        base = 'sub2api-fixture-original:local'
        run(['docker', 'tag', image, base])
        env = {'AUTO_SETUP': 'true', 'DATABASE_HOST': 'postgres', 'DATABASE_PORT': '5432',
               'DATABASE_USER': 'sub2api', 'DATABASE_DBNAME': 'sub2api', 'DATABASE_PASSWORD': password,
               'DATABASE_SSLMODE': 'disable', 'REDIS_HOST': 'redis', 'REDIS_PORT': '6379',
               'JWT_SECRET': secrets.token_hex(32), 'TOTP_ENCRYPTION_KEY': secrets.token_hex(32),
               'ADMIN_EMAIL': 'fixture@example.invalid', 'ADMIN_PASSWORD': secrets.token_hex(20)}
        compose = {'services': {
            'postgres': {'image': 'postgres:18-alpine', 'container_name': 'sub2api-postgres',
                         'environment': {'POSTGRES_USER': 'sub2api', 'POSTGRES_DB': 'sub2api', 'POSTGRES_PASSWORD': password, 'TZ': 'Asia/Shanghai'}},
            'redis': {'image': 'redis:8-alpine', 'container_name': 'sub2api-redis'},
            'sub2api': {'image': base, 'container_name': 'sub2api', 'environment': env,
                        'ports': ['127.0.0.1:8081:8080'], 'volumes': [str(prod / 'data') + ':/app/data']}},
            'networks': {'default': {}}}
        (prod / 'docker-compose.yml').write_text(json.dumps(compose))
        c = Controller({'enabled': True, 'production_dir': str(prod), 'state_dir': str(root / 'state'),
                        'release_token_file': str(release_token), 'health_urls': []})
        c.manifest = lambda version: manifest
        try:
            run(c.compose() + ['up', '-d', 'postgres', 'redis'])
            c.wait_command(['docker', 'exec', 'sub2api-postgres', 'pg_isready', '-h', '127.0.0.1', '-U', 'sub2api', '-d', 'sub2api'], 90)
            run(c.compose() + ['up', '-d', '--no-deps', 'sub2api'])
            c.wait_command(['curl', '-fsS', '--max-time', '5', 'http://127.0.0.1:8081/health'], 180)
            # Mark the current container's writable layer to prove rollback captures
            # the runtime, not merely the original base image.
            run(['docker', 'exec', '-u', '0', 'sub2api', 'touch', '/app/fixture-runtime-marker'])
            dependencies = {n: json.loads(run(['docker', 'inspect', n]))[0]['State']['StartedAt']
                            for n in ['sub2api-postgres', 'sub2api-redis']}
            job = {'id': uuid.uuid4().hex, 'version': manifest['version']}
            c.prepare(job)
            assert job['status'] == 'ready'
            assert c.inspect()['Id'] == job['original_container'], 'prepare replaced application'
            c.deploy(job)
            assert job['status'] == 'succeeded', job['status']
            c.rollback(job)
            assert job['status'] == 'rolled_back'
            run(['docker', 'exec', 'sub2api', 'test', '-f', '/app/fixture-runtime-marker'])
            for n, started in dependencies.items():
                assert json.loads(run(['docker', 'inspect', n]))[0]['State']['StartedAt'] == started
            print('PASS: consistent backup, protected fingerprints, isolated restore, manual cutover and exact-runtime rollback; database/Redis not restarted')
        except Exception:
            # All accounts and passwords in this runner are disposable fixtures.
            # Preserve diagnostics before destroying the temporary environment.
            for name in ['sub2api', 'sub2api-postgres', 'sub2api-redis']:
                subprocess.run(['docker', 'logs', '--tail', '35', name], check=False)
                subprocess.run(['docker', 'inspect', '--format', '{{json .State}}', name], check=False)
            errors = c.state / 'command-errors.log'
            if errors.exists():
                print(errors.read_text(errors='replace')[-6000:])
            raise
        finally:
            run(c.compose() + ['down', '--volumes'])  # CI fixture only, never production controller.
            Path('/run/sub2api-deployer/drain').unlink(missing_ok=True)


if __name__ == '__main__':
    main()
