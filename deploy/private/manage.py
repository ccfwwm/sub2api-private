#!/usr/bin/env python3
"""Root-only, detached, auditable operations for this Sub2API deployment."""
import argparse
import contextlib
import copy
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import tarfile
import time
import traceback
import uuid

from controller import Controller, IMAGE, VERSION, PROTECTED, atomic_json, file_hash, run, validate_manifest

BASE = Path('/root/sub2api-ops')
STATE = BASE / 'state'
PRODUCTION = Path('/root/sub2api-deploy')
ACTIVE = STATE / 'active.json'
IDENT = re.compile(r'[0-9a-f]{32}\Z')


def read(path):
    return json.loads(Path(path).read_text())


def ident(value):
    if not IDENT.fullmatch(value):
        raise ValueError('invalid job/backup ID')
    return value


def inspect(name):
    return json.loads(run(['docker', 'inspect', name]))[0]


class Manager:
    def __init__(self, state=STATE, production=PRODUCTION):
        self.state, self.production = Path(state), Path(production)
        self.state.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.active_path = self.state / 'active.json'
        self.drain_dir = self.state / 'runtime'
        self.drain_dir.mkdir(exist_ok=True, mode=0o755)
        os.chmod(self.drain_dir, 0o755)
        self.marker = self.drain_dir / 'drain'
        self.c = Controller({'production_dir': str(self.production), 'state_dir': str(self.state), 'health_urls': []})

    def active(self):
        return read(self.active_path) if self.active_path.exists() else {
            'services': {'sub2api': {}},
            'x-ops': {'postgres': 'sub2api-postgres', 'redis': 'sub2api-redis'}}

    def refresh(self):
        self.c.config['database_container'] = self.active()['x-ops']['postgres']

    def save(self, job, status):
        job.update(status=status, updated_at=datetime.datetime.now(datetime.timezone.utc).isoformat())
        atomic_json(self.state / 'jobs' / (job['id'] + '.json'), job)
        print(status, flush=True)

    def preflight(self):
        self.refresh()
        original = self.c.inspect()
        labels = original['Config']['Labels']
        if labels.get('com.docker.compose.project') != 'sub2api-deploy':
            raise ValueError('unexpected Compose project')
        if not original['State']['Running']:
            raise ValueError('application is not running; inspect before operating')
        if (Path('/var/lib/sub2api-deployer/active-image.json').exists() or
                Path('/run/sub2api-deployer/controller.sock').exists()):
            raise ValueError('another deployment controller is installed; do not mix controllers')
        db = inspect(self.c.config['database_container'])
        env = dict(x.split('=', 1) for x in original['Config']['Env'])
        db_env = dict(x.split('=', 1) for x in db['Config']['Env'])
        if env.get('DATABASE_DBNAME') != db_env.get('POSTGRES_DB'):
            raise ValueError('database name differs from managed backup target')
        expected_host = self.active()['x-ops']['postgres']
        if env.get('DATABASE_HOST') not in ('postgres', expected_host):
            raise ValueError('application uses an unexpected database host')
        if env.get('REDIS_HOST') not in ('redis', self.active()['x-ops']['redis']):
            raise ValueError('application uses an unexpected Redis host')
        if not env.get('JWT_SECRET') or not env.get('TOTP_ENCRYPTION_KEY'):
            raise ValueError('encryption keys must exist before backup')
        size = int(self.c.sql('SELECT pg_database_size(current_database());'))
        usage = shutil.disk_usage(self.state)
        # Production keeps a conservative 4x database-size safety margin. The
        # disposable GitHub rehearsal has no production data and may run on the
        # hosted runner's smaller ephemeral disk; its workflow opts into a
        # separately bounded requirement after cleaning runner caches.
        ci_rehearsal = (os.environ.get('GITHUB_ACTIONS') == 'true' and
                        os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted')
        if ci_rehearsal:
            minimum = int(os.environ.get('SUB2API_REHEARSAL_MIN_FREE_BYTES', str(4 * 1024**3)))
            multiplier = int(os.environ.get('SUB2API_REHEARSAL_DISK_MULTIPLIER', '2'))
        else:
            minimum = 10 * 1024**3
            multiplier = 4
        required = max(minimum, size * multiplier)
        if usage.free < required:
            raise ValueError(f'insufficient space for backup and isolated restore: free={usage.free} required={required} db={size}')
        return original

    def backup(self, job):
        original = self.preflight()
        self.save(job, 'backing_up')
        bid = uuid.uuid4().hex
        folder = self.state / 'backups' / bid
        folder.mkdir(parents=True, mode=0o700)
        job['backup_id'] = bid
        self.save(job, 'backing_up')
        tag = 'sub2api-ops-rollback:' + bid
        run(['docker', 'commit', '--pause=false', 'sub2api', tag])
        binary = run(['docker', 'exec', 'sub2api', 'sha256sum', '/app/sub2api']).split()[0]
        captured = run(['docker', 'run', '--rm', '--network', 'none', '--entrypoint', 'sha256sum', tag, '/app/sub2api']).split()[0]
        if binary != captured:
            raise ValueError('runtime changed during capture')
        with (folder / 'runtime-image.tar').open('wb') as out:
            run(['docker', 'image', 'save', tag], stdout=out)
        self.c.backup_database(folder)
        data = [m['Source'] for m in original['Mounts'] if m['Destination'] == '/app/data']
        if len(data) != 1:
            raise ValueError('expected one application data mount')
        # Live rolling logs are not restore inputs and change during tar reads.
        run(['tar', '--exclude=./logs', '-czf', str(folder / 'application-data.tar.gz'), '-C', data[0], '.'])
        run(['tar', '-czf', str(folder / 'configuration.tar.gz'), '-C', str(self.production),
             '.env', 'docker-compose.yml', 'docker-compose.override.yml', 'pricing-overlay'])
        redis = self.active()['x-ops']['redis']
        rdb = '/tmp/sub2api-ops-' + bid + '.rdb'
        redis_password = dict(x.split('=', 1) for x in original['Config']['Env']).get('REDIS_PASSWORD', '')
        redis_exec = ['docker', 'exec']
        probe = subprocess.run(['docker', 'exec', redis, 'sh', '-c', 'unset REDISCLI_AUTH; redis-cli ping'], capture_output=True, text=True)
        authenticated = probe.returncode != 0 or probe.stdout.strip() != 'PONG'
        if authenticated:
            if not redis_password:
                raise ValueError('Redis requires authentication but no backup credential is configured')
            redis_exec += ['-e', 'REDISCLI_AUTH=' + redis_password]
        with (folder / 'redis.rdb').open('wb') as out:
            run(redis_exec + [redis, 'sh', '-c',
                 ('' if authenticated else 'unset REDISCLI_AUTH; ') +
                 'redis-cli --rdb "$1" >/dev/null && cat "$1" && rm "$1"', 'sh', rdb], stdout=out)
        metadata = {'id': bid, 'created_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    'runtime_image': tag, 'binary_sha256': binary.decode(), 'migrations': self.c.migrations(),
                    'active': self.active(), 'application': original,
                    'compose_hashes': self.c.config_hashes()}
        atomic_json(folder / 'metadata.json', metadata)
        atomic_json(folder / 'checksums.json', {p.name: file_hash(p) for p in folder.iterdir() if p.is_file()})
        if self.c.inspect()['Id'] != original['Id']:
            raise ValueError('application changed during backup')
        print('BACKUP_ID=' + bid, flush=True)
        return folder, metadata

    def verified_backup(self, bid):
        folder = self.state / 'backups' / ident(bid)
        checks = read(folder / 'checksums.json')
        required = {'database.dump', 'globals.sql', 'protected-snapshot.json', 'runtime-image.tar',
                    'application-data.tar.gz', 'configuration.tar.gz', 'redis.rdb', 'metadata.json'}
        if set(checks) != required:
            raise ValueError('backup is incomplete')
        for name, digest in checks.items():
            if file_hash(folder / name) != digest:
                raise ValueError('backup checksum mismatch: ' + name)
        return folder, read(folder / 'metadata.json')

    def offline(self, version):
        if not VERSION.fullmatch(version):
            raise ValueError('expected private vMAJOR.MINOR.PATCH')
        folder = self.state / 'releases' / version
        manifest, identity = read(folder / 'deployment.json'), read(folder / 'image-inspect.json')[0]
        validate_manifest(manifest, version)
        image = IMAGE + ':' + version
        actual = inspect(image)
        if (actual['RootFS'] != identity['RootFS'] or actual['Os'] != 'linux' or actual['Architecture'] != 'amd64' or
                actual['Config'].get('Labels', {}).get('org.opencontainers.image.revision') != manifest['commit']):
            raise ValueError('staged image identity mismatch')
        # Compare original config bytes across classic/containerd image stores.
        expected = identity['Id'].split(':')[1]
        found = False
        proc = subprocess.Popen(['docker', 'image', 'save', image], stdout=subprocess.PIPE)
        with tarfile.open(fileobj=proc.stdout, mode='r|') as archive:
            for member in archive:
                if member.isfile() and member.name in (expected + '.json', 'blobs/sha256/' + expected):
                    if member.size > 1024 * 1024:
                        raise ValueError('invalid image config size')
                    found = hashlib.sha256(archive.extractfile(member).read()).hexdigest() == expected
        proc.stdout.read()
        proc.stdout.close()
        if proc.wait() or not found:
            raise ValueError('staged image config mismatch')
        return actual['Id'], manifest

    def load_runtime(self, folder, meta):
        run(['docker', 'image', 'load', '-i', str(folder / 'runtime-image.tar')])
        image = inspect(meta['runtime_image'])['Id']
        actual = run(['docker', 'run', '--rm', '--network', 'none', '--entrypoint', 'sha256sum', image, '/app/sub2api']).decode().split()[0]
        if actual != meta['binary_sha256']:
            raise ValueError('backup runtime mismatch')
        return image

    def rehearse(self, job, folder, meta, image, version='', reviewed=None):
        self.save(job, 'restoring_and_testing')
        # Base controller verifies a real pg_restore plus protected-table hashes.
        test = {'id': uuid.uuid4().hex, 'image': image, 'rollback_image': meta['runtime_image'],
                'version': version or 'v' + self.image_version(image),
                'manifest': {'migrations': meta['migrations']}}
        if reviewed:
            test['reviewed_media_migration'] = reviewed
        self.c.rehearse(test, folder)
        if reviewed:
            if not all(test.get(k) for k in ('restore_verified', 'rollback_verified', 'additive_data_verified')):
                raise ValueError('incomplete monitor migration rehearsal')
            job['monitor_migration'] = {'filename': reviewed['filename'], 'restore_verified': True,
                                        'rollback_verified': True, 'data_unchanged': True}
        # The controller adds an evidence file; retain the original backup seal.
        (folder / 'restored-fingerprints.json').unlink(missing_ok=True)

    @staticmethod
    def image_version(image):
        result = subprocess.run(['docker', 'run', '--rm', '--network', 'none', '--entrypoint',
                                 '/app/sub2api', image, '-version'], capture_output=True, text=True, check=True)
        match = re.search(r'Sub2API v?(\d+\.\d+\.\d+)', result.stdout + result.stderr)
        if not match:
            raise ValueError('cannot determine captured runtime version')
        return match[1]

    def target(self, image, base=None):
        target = copy.deepcopy(base or self.active())
        service = target['services']['sub2api']
        service.update(image=image, pull_policy='never', stop_grace_period='130s')
        service.setdefault('environment', {}).update(IMAGE_DEPLOY_DRAIN_FILE='/run/sub2api-ops/drain',
                                                    SERVER_SHUTDOWN_TIMEOUT_SECONDS='120')
        volumes = service.setdefault('volumes', [])
        mount = str(self.drain_dir) + ':/run/sub2api-ops:ro'
        if mount not in volumes:
            volumes.append(mount)
        return target

    def deployment_status(self):
        status = json.loads(run(['docker', 'exec', 'sub2api', 'wget', '-qO-',
                                 'http://127.0.0.1:8080/internal/deployment/status'], timeout=10))
        if (not isinstance(status, dict) or type(status.get('active')) is not int or
                status['active'] < 0 or type(status.get('draining')) is not bool):
            raise ValueError('invalid deployment status; operation cancelled')
        return {key: status[key] for key in ('draining', 'active')}

    def wait_for_idle(self, timeout=900):
        if self.marker.exists():
            raise ValueError('maintenance marker already exists; inspect before operating')
        deadline = time.monotonic() + timeout
        report_at = 0
        last = {}
        while time.monotonic() < deadline:
            last = self.deployment_status()
            if last['draining']:
                raise ValueError('runtime is already draining; inspect before operating')
            if last['active'] == 0:
                return
            if time.monotonic() >= report_at:
                print('WAITING_FOR_IDLE=' + json.dumps(last), flush=True)
                report_at = time.monotonic() + 30
            time.sleep(2)
        raise ValueError('no idle window; production remains open; last_status=' + json.dumps(last))

    def drain(self, job=None):
        env = dict(x.split('=', 1) for x in self.c.inspect()['Config']['Env'])
        if env.get('IMAGE_DEPLOY_DRAIN_FILE') != '/run/sub2api-ops/drain':
            self.marker.touch(mode=0o644)
            os.chmod(self.marker, 0o644)
            print('First cutover: current official runtime cannot drain; existing requests may be interrupted.', flush=True)
            return
        if job is not None:
            self.save(job, 'waiting_for_idle')
        # Observe a natural idle window while admission remains open. The
        # subsequent maintenance gate still handles requests racing this probe.
        self.wait_for_idle()
        if job is not None:
            self.save(job, 'draining')
        self.marker.touch(mode=0o644)
        os.chmod(self.marker, 0o644)
        deadline = time.monotonic() + 120
        report_at = 0
        last = {}
        try:
            while time.monotonic() < deadline:
                last = self.deployment_status()
                if last['draining'] and last['active'] == 0:
                    return
                if time.monotonic() >= report_at:
                    print('DRAIN_STATUS=' + json.dumps(last), flush=True)
                    report_at = time.monotonic() + 15
                time.sleep(1)
            raise ValueError('active requests did not drain; cutover cancelled; last_status=' + json.dumps(last))
        except BaseException:
            # Failed probes and interrupted waits must immediately restore
            # admission even when drain is called outside the cutover wrapper.
            self.marker.unlink(missing_ok=True)
            raise

    def switch(self, target):
        atomic_json(self.active_path, target)
        run(self.c.compose(self.active_path) + ['up', '-d', '--no-deps', '--pull', 'never', 'sub2api'], timeout=300)
        self.c.wait_command(['curl', '-fsS', '--max-time', '5', 'http://127.0.0.1:8081/health'], 180)
        if self.c.inspect()['Config']['Image'] != target['services']['sub2api']['image']:
            raise ValueError('running image differs from requested image')

    def cutover(self, job, target, rescue, version=''):
        self.save(job, 'preparing_cutover')
        # Record the rescue target before setting a persistent drain marker, so
        # a host failure during drain can also be recovered from the CLI.
        atomic_json(self.state / 'latest-cutover.json', {'id': job['id']})
        switched = False
        before = {n: (inspect(n)['Id'], inspect(n)['State']['StartedAt'])
                  for n in ['sub2api-postgres', 'sub2api-redis', 'sub2api-pricing-overlay']}
        try:
            if job.get('immediate'):
                if self.marker.exists():
                    raise ValueError('maintenance marker exists; inspect before immediate cutover')
                job['cutover_mode'] = 'immediate'
                # The operator authorized interruption of in-flight requests.
                target['services']['sub2api']['stop_grace_period'] = '1s'
                target['services']['sub2api']['environment']['SERVER_SHUTDOWN_TIMEOUT_SECONDS'] = '1'
            else:
                self.drain(job)
            self.save(job, 'switching')
            switched = True
            self.switch(target)
            self.marker.unlink(missing_ok=True)
            self.c.wait_command(['curl', '-fsS', '--max-time', '5', 'http://127.0.0.1:8081/api/v1/settings/public'], 30)
            settings = json.loads(run(['curl', '-fsS', '--max-time', '10', 'http://127.0.0.1:8081/api/v1/settings/public']))
            if version and settings['data']['version'].lstrip('v') != version.lstrip('v'):
                raise ValueError('running version mismatch')
            for n, state in before.items():
                if state != (inspect(n)['Id'], inspect(n)['State']['StartedAt']):
                    raise ValueError('database/Redis container changed unexpectedly')
            job['protected_containers'] = {n: {'id': state[0], 'started_at': state[1]} for n, state in before.items()}
            job['active_image'] = self.c.inspect()['Image']
            self.save(job, 'succeeded')
        except Exception:
            if switched:
                self.save(job, 'rolling_back_after_error')
                self.switch(rescue)
                self.save(job, 'failed_rolled_back')
            raise
        finally:
            self.marker.unlink(missing_ok=True)

    def recover_stack(self, job, folder, meta, image):
        """Create new PG/Redis/data without modifying the original stores."""
        prefix = 'sub2api-recovery-' + job['id'][:12]
        network, pg, redis = prefix + '-net', prefix + '-pg', prefix + '-redis'
        root = self.state / 'recoveries' / job['id']
        root.mkdir(parents=True, mode=0o700)
        job.update(recovery_directory=str(root), recovery_postgres=pg, recovery_redis=redis)
        self.save(job, 'restoring_to_new_stores')
        run(['docker', 'network', 'create', '--internal', network])
        password = secrets.token_hex(32)
        run(['docker', 'run', '-d', '--name', pg, '--restart', 'unless-stopped', '--network', network, '--network-alias', 'postgres',
             '--cpus=2', '--memory=3g', '-v', str(root / 'postgres') + ':/var/lib/postgresql',
             '-e', 'POSTGRES_USER=sub2api', '-e', 'POSTGRES_DB=sub2api', '-e', 'POSTGRES_PASSWORD=' + password, 'postgres:18-alpine'])
        self.c.wait_command(['docker', 'exec', pg, 'pg_isready', '-h', '127.0.0.1', '-U', 'sub2api'], 90)
        with (folder / 'database.dump').open('rb') as src:
            proc = subprocess.run(['docker', 'exec', '-i', pg, 'pg_restore', '-U', 'sub2api', '-d', 'sub2api',
                                   '--exit-on-error', '--no-owner', '--no-acl'], stdin=src, capture_output=True, timeout=3600)
        if proc.returncode:
            raise ValueError('restored database rejected dump; original data untouched')
        if {t: self.c.sql(self.c.fingerprint_sql(t), pg) for t in PROTECTED} != read(folder / 'protected-snapshot.json'):
            raise ValueError('restored database fingerprint mismatch')
        (root / 'redis').mkdir(mode=0o700)
        shutil.copyfile(folder / 'redis.rdb', root / 'redis' / 'dump.rdb')
        redis_password = secrets.token_hex(32)
        redis_config = root / 'redis.conf'
        redis_config.write_text('appendonly yes\nrequirepass ' + redis_password + '\n')
        os.chmod(redis_config, 0o644)  # containing host directory stays root-only
        run(['docker', 'run', '-d', '--name', redis, '--restart', 'unless-stopped', '--network', network, '--network-alias', 'redis',
             '--cpus=1', '--memory=1g', '-v', str(root / 'redis') + ':/data',
             '-v', str(redis_config) + ':/etc/redis-recovery.conf:ro', 'redis:8-alpine',
             'redis-server', '/etc/redis-recovery.conf'])
        self.c.wait_command(['docker', 'exec', '-e', 'REDISCLI_AUTH=' + redis_password, redis, 'redis-cli', 'ping'], 60)
        data = root / 'data'
        data.mkdir(mode=0o700)
        with tarfile.open(folder / 'application-data.tar.gz') as archive:
            archive.extractall(data, filter='data')
        env = dict(x.split('=', 1) for x in meta['application']['Config']['Env'])
        env.update(DATABASE_HOST=pg, DATABASE_PORT='5432', DATABASE_USER='sub2api', DATABASE_DBNAME='sub2api',
                   DATABASE_PASSWORD=password, DATABASE_SSLMODE='disable', REDIS_HOST=redis, REDIS_PORT='6379',
                   REDIS_PASSWORD=redis_password, IMAGE_DEPLOY_DRAIN_FILE='', AUTO_SETUP='true')
        envfile = root / 'app.env'
        if any('\n' in v or '\r' in v for v in env.values()):
            raise ValueError('multiline environment unsupported')
        envfile.write_text(''.join(k + '=' + v + '\n' for k, v in env.items()))
        os.chmod(envfile, 0o600)
        probe = prefix + '-probe'
        try:
            run(['docker', 'run', '-d', '--name', probe, '--network', network,
                 '--cpus=2', '--memory=2g', '--env-file', str(envfile), '-v', str(data) + ':/app/data', image])
            self.c.wait_command(['docker', 'exec', probe, 'wget', '-qO-', 'http://127.0.0.1:8080/health'], 180)
        finally:
            subprocess.run(['docker', 'rm', '-f', probe], capture_output=True)
        base_compose = json.loads(run(self.c.compose() + ['config', '--format', 'json']))
        base_networks = base_compose['services']['sub2api'].get('networks', {'default': {}})
        if isinstance(base_networks, list):
            base_networks = {name: {} for name in base_networks}
        networks = copy.deepcopy(base_networks)
        networks[network] = {}
        target = self.target(image, {'services': {'sub2api': {'environment': env,
                            'volumes': [str(data) + ':/app/data'], 'networks': networks}},
                            'networks': {network: {'external': True, 'name': network}},
                            'x-ops': {'postgres': pg, 'redis': redis}})
        return target

    def execute(self, job):
        action, arg = job['action'], job.get('target', '')
        if action == 'rescue':
            if read(self.state / 'latest-cutover.json')['id'] != ident(arg):
                raise ValueError('rescue only accepts the most recent cutover job')
            rescue = read(self.state / 'jobs' / (arg + '-rescue.json'))
            self.save(job, 'recovering_previous_application')
            try:
                self.switch(rescue)
            finally:
                self.marker.unlink(missing_ok=True)
            self.save(job, 'succeeded')
            return
        if action == 'backup':
            self.backup(job)
            self.save(job, 'succeeded')
            return
        if action == 'verify':
            folder, meta = self.verified_backup(arg)
            image = self.load_runtime(folder, meta)
            self.rehearse(job, folder, meta, image)
            self.save(job, 'succeeded')
            return
        original = self.preflight()
        reviewed = None
        if action in ('prepare', 'upgrade'):
            image, manifest = self.offline(arg)
            from media_migrate import reviewed_monitor_upgrade
            reviewed = reviewed_monitor_upgrade(self.c.migrations(), manifest['migrations'], self.state / 'releases' / arg)
        else:
            source, old = self.verified_backup(arg)
            image = self.load_runtime(source, old)
            if action == 'rollback' and self.c.migrations() != old['migrations']:
                raise ValueError('schema differs; application-only rollback refused')
        folder, meta = self.backup(job)
        rescue = self.target(inspect(meta['runtime_image'])['Id'])
        atomic_json(self.state / 'jobs' / (job['id'] + '-rescue.json'), rescue)
        if action in ('prepare', 'upgrade'):
            self.rehearse(job, folder, meta, image, arg, reviewed=reviewed)
            if action == 'prepare':
                job['prepared_only'] = True
                self.save(job, 'succeeded')
                return
            target = self.target(image)
        elif action == 'rollback':
            self.rehearse(job, folder, meta, image)
            target = self.target(image)
        elif action == 'restore':
            target = self.recover_stack(job, source, old, image)
        else:
            raise ValueError('unknown action')
        if self.c.inspect()['Id'] != original['Id'] or self.c.config_hashes() != meta['compose_hashes']:
            raise ValueError('production changed during preparation')
        if run(['docker', 'exec', 'sub2api', 'sha256sum', '/app/sub2api']).decode().split()[0] != meta['binary_sha256']:
            raise ValueError('production runtime changed during preparation')
        self.c.assert_migrations({'migrations': meta['migrations']})
        if reviewed:
            self.save(job, 'applying_reviewed_monitor_enum_expansion')
            self.c.apply_reviewed_media_migration(reviewed['transaction'])
            self.c.assert_migrations(manifest)
        self.cutover(job, target, rescue, arg if action == 'upgrade' else '')


def main():
    parser = argparse.ArgumentParser(description='Sub2API 离线生产管理。任务由 systemd 后台执行，SSH 断开不影响。')
    parser.add_argument('action', choices=['status', 'logs', 'list', 'backup', 'verify', 'prepare', 'upgrade', 'rollback', 'restore', 'rescue', '_worker'])
    parser.add_argument('target', nargs='?', default='')
    parser.add_argument('--immediate', action='store_true', help='Upgrade application immediately without waiting for idle or draining')
    parser.add_argument('--confirm', default='', help='写操作须与动作同名大写：BACKUP / VERIFY / UPGRADE / ROLLBACK / RESTORE')
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit('root required')
    if args.action in ('status', 'list'):
        jobs = sorted((STATE / 'jobs').glob('*.json'), key=lambda p: p.stat().st_mtime, reverse=True) if STATE.exists() else []
        if args.target:
            jobs = [STATE / 'jobs' / (ident(args.target) + '.json')]
        for p in jobs[:20]:
            if p.name.endswith('-rescue.json'):
                continue
            job = read(p)
            if job.get('status') not in ('succeeded', 'failed_needs_inspection', 'failed_rolled_back'):
                unit = subprocess.run(['systemctl', 'is-active', job.get('worker_unit', 'sub2api-ops-' + job['id'])], capture_output=True, text=True)
                job['worker_state'] = unit.stdout.strip()
                if unit.returncode:
                    job['attention'] = 'worker is not running; inspect logs, do not assume completion'
            print(json.dumps(job, ensure_ascii=False))
        if args.action == 'status':
            run_status = subprocess.run(['docker', 'ps', '--filter', 'name=sub2api', '--format', '{{.Names}} {{.Status}}'])
            raise SystemExit(run_status.returncode)
        return
    if args.action == 'logs':
        target = ident(args.target) if args.target else read(STATE / 'latest.json')['id']
        os.execvp('tail', ['tail', '-n', '80', '-F', str(STATE / 'logs' / (target + '.log'))])
    os.umask(0o077)
    manager = Manager()
    if args.action == '_worker':
        import fcntl
        job = read(STATE / 'jobs' / (ident(args.target) + '.json'))
        try:
            with (STATE / 'operations.lock').open('a') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                manager.execute(job)
        except Exception:
            traceback.print_exc()  # root-only log; never copy logs to GitHub
            if job.get('status') != 'failed_rolled_back':
                manager.save(job, 'failed_needs_inspection')
            raise SystemExit(1)
        return
    if args.confirm != args.action.upper():
        raise SystemExit('未执行。备份/验证增加磁盘与 CPU 负载；升级/回滚短暂停服；恢复会让应用使用备份时的数据。确认后加 --confirm ' + args.action.upper())
    if args.action in ('prepare', 'upgrade'):
        if not VERSION.fullmatch(args.target):
            raise SystemExit('upgrade requires vMAJOR.MINOR.PATCH')
    elif args.action != 'backup':
        ident(args.target)
    if args.immediate and args.action != 'upgrade':
        raise SystemExit('--immediate is only valid with upgrade')
    job = {'id': uuid.uuid4().hex, 'action': args.action, 'target': args.target, 'immediate': args.immediate}
    (STATE / 'logs').mkdir(exist_ok=True, mode=0o700)
    log = STATE / 'logs' / (job['id'] + '.log')
    log.touch(mode=0o600)
    manager.save(job, 'queued')
    atomic_json(STATE / 'latest.json', {'id': job['id']})
    subprocess.run(['systemd-run', '--unit=sub2api-ops-' + job['id'], '--collect',
                    '--property=Type=exec', '--property=UMask=0077', '--property=Nice=10',
                    '--setenv=HOME=/root', '--setenv=XDG_CONFIG_HOME=/root/.config',
                    '--property=StandardOutput=append:' + str(log), '--property=StandardError=append:' + str(log),
                    sys.executable, str(Path(__file__).resolve()), '_worker', job['id']], check=True)
    print('JOB_ID=' + job['id'] + '\n查看：/root/sub2api-manage status\n日志：/root/sub2api-manage logs ' + job['id'])


if __name__ == '__main__':
    main()

