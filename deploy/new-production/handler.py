"""Deploy formal Sub2API releases onto the hardened migrated application stack."""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import time
from gateway import HOME, STATE, atomic, health, inspect, read, run, scan

COMPOSE = Path('/srv/migration-20261117/sub2api/compose.production.json')
APP = 'migration-sub-app-production'
DOCS = 'migration-sub-docs-production'
PG = 'migration-sub-postgres'
REDIS = 'migration-sub-redis'


def valid_revision(value):
    return re.fullmatch(r'v(0|[1-9][0-9]*)\.[0-9]+\.[0-9]+', value)


def compose():
    return ['docker', 'compose', '-f', str(COMPOSE), '--profile', 'cutover-required']


def verify():
    config = read(COMPOSE)
    for service, name in [('app', APP), ('docs', DOCS)]:
        info = inspect(name)
        spec = config['services'][service]
        assert info['State']['Running'] and info['HostConfig']['ReadonlyRootfs']
        assert info['Config']['User'] == '1000:1000'
        assert 'ALL' in info['HostConfig']['CapDrop']
        assert inspect(spec['image'])['Id'] == info['Image']
    version = json.loads(health('http://127.0.0.1:8081/api/v1/settings/public'))['data']['version']
    health('http://127.0.0.1:8082/health')
    return {'version': 'v' + version.lstrip('v'), 'app_image': inspect(APP)['Image'], 'docs_image': inspect(DOCS)['Image'], 'data_preserved': True}


def controller():
    from controller import Controller, PROTECTED

    def command(shell, interactive=False):
        return ['docker', 'exec', *(['-i'] if interactive else []), '--env-file',
                '/var/lib/migration-20261117/sub2api/admin.env', PG, 'sh', '-c', shell]

    class MigratedController(Controller):
        def sql(self, query, container=None):
            if container and container != PG:
                return super().sql(query, container)
            return run(command('PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=30000 -c timezone=UTC" '
                               'psql -X -qAt -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$PGDATABASE"', True),
                       data=query.encode()).decode().strip()

        def backup_database(self, backup):
            # The application role was rotated after migration. Both the snapshot
            # holder and pg_dump must use the separate host-only administrator.
            process = subprocess.Popen(command('PGOPTIONS="-c statement_timeout=30000 -c idle_in_transaction_session_timeout=3600000" '
                                               'psql -X -qAt -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$PGDATABASE"', True),
                                       stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                process.stdin.write("BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL TIME ZONE 'UTC'; SELECT pg_export_snapshot();\n")
                process.stdin.flush()
                snapshot = process.stdout.readline().strip()
                if not re.fullmatch('[0-9A-Fa-f-]+', snapshot):
                    raise ValueError('cannot export backup snapshot')
                expected = {}
                for table in PROTECTED:
                    process.stdin.write(self.fingerprint_sql(table) + '\n')
                    process.stdin.flush()
                    value = process.stdout.readline().strip()
                    if not re.fullmatch(r'\d+\|[0-9a-f]{32}', value):
                        raise ValueError('cannot fingerprint protected table')
                    expected[table] = value
                with (backup / 'database.dump').open('wb') as out:
                    run(command('pg_dump -U "$PGUSER" -d "$PGDATABASE" -Fc '
                                '--exclude-table-data=public.ops_system_logs --snapshot="$1"') + ['sh', snapshot], stdout=out)
                atomic(backup / 'protected-snapshot.json', expected)
            finally:
                with contextlib.suppress(BrokenPipeError):
                    try:
                        process.communicate('ROLLBACK;\n', timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.communicate()
                if process.poll() is None:
                    process.kill()
                    process.wait()
            with (backup / 'globals.sql').open('wb') as out:
                run(command('pg_dumpall -U "$PGUSER" --globals-only'), stdout=out)

    return MigratedController({'production_dir': str(COMPOSE.parent), 'state_dir': str(STATE), 'database_container': PG, 'health_urls': []})


def execute_reviewed(files, c, container=None):
    from reviewed_upgrade import apply
    app_role = 'sub2api'
    if container:
        prefix = ['docker', 'exec', container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'sub2api', '-d', 'sub2api']
    else:
        env = dict(x.split('=', 1) for x in inspect(APP)['Config']['Env'] if '=' in x)
        app_role = env['DATABASE_USER']
        # Administrator credentials remain in the host-only env file. Never
        # deserialize or print them in the release job.
        prefix = ['docker', 'exec', '--env-file', '/var/lib/migration-20261117/sub2api/admin.env', PG,
                  'sh', '-c', 'exec psql -X -qAt -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$PGDATABASE" "$@"', 'sh']

    def execute(settings, sql):
        args = prefix + (['-c', settings] if settings else []) + ['-c', sql]
        run(args, timeout=1900)

    apply(files, execute, lambda sql: c.sql(sql, container), app_role)


def deploy(version, source, prepare_only, phase):
    from controller import validate_manifest, manifest_images, PROTECTED
    manifest = read(source / 'deployment.json')
    validate_manifest(manifest, version)
    current = verify()
    if tuple(map(int, version[1:].split('.'))) < tuple(map(int, current['version'][1:].split('.'))):
        raise ValueError('automatic downgrade refused')
    if version == current['version'] and not prepare_only:
        return {**current, 'already_deployed': True}
    c = controller()
    from reviewed_upgrade import validate as validate_upgrade
    before_migrations = c.migrations()
    reviewed = validate_upgrade(version, before_migrations, manifest['migrations'], source / 'migrations')
    for name, digest in manifest['migrations'].items():
        sql = (source / 'migrations' / name).read_text().replace('\r\n', '\n').strip().encode()
        if hashlib.sha256(sql).hexdigest() != digest:
            raise ValueError('immutable migration source checksum mismatch')
    app_ref, docs_ref = manifest_images(manifest, version)
    if not docs_ref:
        raise ValueError('current production requires a documentation image')
    phase('building_hardened_release')
    for ref in [app_ref, docs_ref]:
        run(['docker', 'pull', ref])
        info = inspect(ref)
        if info['Config'].get('Labels', {}).get('org.opencontainers.image.revision') != manifest['commit']:
            raise ValueError('release image revision differs')
    dockerfile = (HOME / 'Dockerfile.app').read_text().replace('RELEASE_IMAGE', app_ref).replace('RELEASE_COMMIT', manifest['commit'])
    (source / 'Dockerfile.app').write_text(dockerfile)
    (source / 'Dockerfile.docs').write_text((HOME / 'Dockerfile.docs').read_text().replace('RELEASE_IMAGE', docs_ref))
    app_image = 'codeme-sub2api:' + version
    docs_image = 'codeme-sub2api-docs:' + version
    for name, tag in [('app', app_image), ('docs', docs_image)]:
        run(['docker', 'build', '--pull', '-t', tag, '-f', source / ('Dockerfile.' + name), source])
        scan(inspect(tag)['Id'], source / ('scan-' + name + '.json'))
    phase('backup_and_restore_rehearsal')
    backup = source / 'backup'
    backup.mkdir()
    c.backup_database(backup)
    shutil.copy2(COMPOSE, backup / 'compose.json')
    # No runtime layer is copied from the compromised machine. Exact current
    # application image IDs remain available locally for application rollback.
    atomic(backup / 'images.json', current)
    suffix = secrets.token_hex(5)
    pg = 'codeme-sub-rehearsal-' + suffix
    candidate = pg + '-app'
    redis = pg + '-redis'
    pg_image = inspect(PG)['Image']
    try:
        run(['docker', 'run', '-d', '--name', pg, '--network', 'none', '-e', 'POSTGRES_HOST_AUTH_METHOD=trust', '-e', 'POSTGRES_USER=sub2api', '-e', 'POSTGRES_DB=sub2api', pg_image])
        for _ in range(90):
            if subprocess.run(['docker', 'exec', pg, 'pg_isready', '-U', 'sub2api', '-d', 'sub2api'], capture_output=True).returncode == 0:
                break
            time.sleep(1)
        with (backup / 'database.dump').open('rb') as f:
            p = subprocess.run(['docker', 'exec', '-i', pg, 'pg_restore', '-U', 'sub2api', '-d', 'sub2api', '--no-owner', '--no-privileges', '--exit-on-error'], stdin=f, capture_output=True, timeout=1800)
            if p.returncode:
                raise ValueError('isolated PostgreSQL restore rehearsal failed')
        expected = read(backup / 'protected-snapshot.json')
        actual = {table: c.sql(c.fingerprint_sql(table), pg) for table in PROTECTED}
        if actual != expected:
            raise ValueError('isolated restore protected-table fingerprints differ')
        phase('reviewed_migration_rehearsal')
        execute_reviewed(reviewed, c, pg)
        if c.migrations(pg) != manifest['migrations']:
            raise ValueError('reviewed clone migration inventory differs')
        probe_data = source / 'probe-data'
        probe_data.mkdir(mode=0o700)
        os.chown(probe_data, 1000, 1000)
        env = dict(x.split('=', 1) for x in inspect(APP)['Config']['Env'] if '=' in x)
        password = secrets.token_hex(24)
        env.update(DATABASE_HOST='127.0.0.1', DATABASE_PORT='5432', DATABASE_USER='sub2api', DATABASE_DBNAME='sub2api', DATABASE_PASSWORD='', DATABASE_SSLMODE='disable', REDIS_HOST='127.0.0.1', REDIS_PORT='6379', REDIS_PASSWORD=password, AUTO_SETUP='true', IMAGE_DEPLOY_DRAIN_FILE='/app/data/probe-drain')
        (source / 'probe.env').write_text(''.join(k+'='+v+'\n' for k,v in env.items() if '\n' not in v))
        run(['docker', 'run', '-d', '--name', redis, '--network', 'container:' + pg, inspect(REDIS)['Image'], 'redis-server', '--save', '', '--requirepass', password])
        for image in [current['app_image'], app_image]:
            run(['docker', 'run', '-d', '--name', candidate, '--network', 'container:' + pg, '--user', '1000:1000', '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--tmpfs', '/tmp:rw,nosuid,nodev,size=256m', '--env-file', source / 'probe.env', '-v', str(probe_data)+':/app/data', image])
            for _ in range(90):
                if subprocess.run(['docker', 'exec', candidate, 'wget', '-q', '-T', '2', '-O', '/dev/null', 'http://127.0.0.1:8080/health'], capture_output=True).returncode == 0:
                    break
                time.sleep(2)
            else:
                raise ValueError('new or rollback application failed against the migrated clone')
            run(['docker', 'exec', candidate, 'wget', '-q', '-T', '5', '-O', '/dev/null', 'http://127.0.0.1:8080/api/v1/settings/public'])
            run(['docker', 'stop', '--time', '120', candidate])
            run(['docker', 'rm', candidate])
        if c.migrations(pg) != manifest['migrations']:
            raise ValueError('candidate changed the reviewed migration inventory')
        if c.migrations() != before_migrations:
            raise ValueError('production migration inventory drifted during rehearsal')
    finally:
        for name in (candidate, redis, pg):
            subprocess.run(['docker', 'rm', '-fv', name], capture_output=True)
    if prepare_only:
        return {'version': version, 'backup': str(backup), 'restore_verified': True, 'production_changed': False}
    if reviewed:
        phase('reviewed_production_migrations')
        fresh_backup = source / 'pre-migration-backup'
        fresh_backup.mkdir(mode=0o700)
        c.backup_database(fresh_backup)
        if c.migrations() != before_migrations:
            raise ValueError('production migration inventory changed before execution')
        execute_reviewed(reviewed, c)
        atomic(source / 'migration-receipt.json', {'version': version, 'files': [f[0] for f in reviewed],
                                                 'before': before_migrations, 'after': c.migrations()})
    c.assert_migrations(manifest)
    phase('waiting_for_idle')
    info = inspect(APP)
    env = dict(x.split('=', 1) for x in info['Config']['Env'] if '=' in x)
    if env.get('IMAGE_DEPLOY_DRAIN_FILE') != '/app/data/image-worker-drain':
        raise ValueError('running application lacks the expected drain marker')
    marker = COMPOSE.parent / 'data/image-worker-drain'
    if marker.exists():
        raise ValueError('existing drain marker requires inspection')
    deadline = time.monotonic() + 900
    while time.monotonic() < deadline:
        status = json.loads(run(['docker', 'exec', APP, 'wget', '-qO-', 'http://127.0.0.1:8080/internal/deployment/status']))
        if not status.get('draining') and status.get('active') == 0:
            break
        time.sleep(2)
    else:
        raise ValueError('no idle window; production remains unchanged')
    original = COMPOSE.read_bytes()
    target = read(COMPOSE)
    target['services']['app']['image'] = inspect(app_image)['Id']
    target['services']['docs']['image'] = inspect(docs_image)['Id']
    target['services']['app'].setdefault('environment', {})['SERVER_SHUTDOWN_TIMEOUT_SECONDS'] = '120'
    target['services']['app']['stop_grace_period'] = '130s'
    # Compose remains host-owned: ports, raw env files, mounts, networks and
    # non-root/read-only/capability restrictions survive every release.
    phase('application_cutover')
    try:
        marker.touch(mode=0o644)
        marker.chmod(0o644)
        for _ in range(120):
            status = json.loads(run(['docker', 'exec', APP, 'wget', '-qO-', 'http://127.0.0.1:8080/internal/deployment/status']))
            if status.get('draining') is True and status.get('active') == 0:
                break
            time.sleep(1)
        else:
            raise ValueError('requests did not drain; application cutover cancelled')
        atomic(COMPOSE, target)
        run(compose() + ['up', '-d', '--no-deps', '--pull', 'never', 'app', 'docs'])
        health('http://127.0.0.1:8081/health')
        marker.unlink(missing_ok=True)
        result = verify()
        if result['version'] != version:
            raise ValueError('online version mismatch')
        return {**result, 'backup': str(backup), 'restore_verified': True}
    except Exception as error:
        phase('application_rollback')
        COMPOSE.write_bytes(original)
        try:
            run(compose() + ['up', '-d', '--no-deps', '--pull', 'never', 'app', 'docs'])
            health('http://127.0.0.1:8081/health')
            marker.unlink(missing_ok=True)
            verify()
            error.deploy_status = 'failed_rolled_back'
        except Exception:
            error.deploy_status = 'failed_needs_inspection'
        raise
    finally:
        marker.unlink(missing_ok=True)
