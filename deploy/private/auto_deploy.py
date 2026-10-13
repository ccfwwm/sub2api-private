#!/usr/bin/env python3
"""Restricted stable-release deployment using the installed production manager."""
from __future__ import annotations
import fcntl
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import time
import uuid
from controller import IMAGE, REPO, VERSION, atomic_json, file_hash, run, validate_manifest
from manage import STATE, Manager, inspect, read

AUTO = STATE / 'automatic'
TERMINAL = {'succeeded', 'failed', 'failed_rolled_back', 'failed_needs_inspection'}

def version_tuple(version):
    if not VERSION.fullmatch(version):
        raise ValueError('stable version required')
    return tuple(map(int, version[1:].split('.')))

def check_order(requested, current):
    if current and version_tuple(requested) < version_tuple(current):
        raise ValueError('older releases cannot replace newer deployments')

def clean_status(job):
    return {k: job[k] for k in ('id', 'version', 'digest', 'commit', 'status', 'phase', 'manager_job', 'backup_id', 'error') if k in job}

def save(job, status=None, phase=None):
    if status: job['status'] = status
    if phase: job['phase'] = phase
    atomic_json(AUTO / (job['id'] + '.json'), job)

def execute_manager(manager_path, job):
    path = STATE / 'jobs' / (job['id'] + '.json')
    atomic_json(path, job)
    code = '''import json,sys
sys.path.insert(0,sys.argv[1])
from manage import Manager
job=json.load(open(sys.argv[2])); manager=Manager()
try:
    manager.execute(job)
except Exception:
    if job.get('status') not in ('failed_rolled_back','failed_needs_inspection'):
        manager.save(job,'failed_needs_inspection')
    raise
'''
    run([sys.executable, '-c', code, str(Path(manager_path).parent), str(path)], timeout=10800)
    return read(path)

def verify_online(version, image_id):
    for base in ['http://127.0.0.1:8081', 'https://code.aicodeme.cn', 'https://code.aicodeme.xyz', 'https://hkcode.aicodeme.xyz']:
        run(['curl','-fsS','--max-time','15',base + '/health'])
        settings = json.loads(run(['curl','-fsS','--max-time','15',base + '/api/v1/settings/public']))
        if settings['data']['version'].lstrip('v') != version.lstrip('v'):
            raise ValueError('online version verification failed')
    if inspect('sub2api')['Image'] != image_id: raise ValueError('online image mismatch')

def release_assets(version, folder):
    release = json.loads(run(['gh', 'api', f'repos/{REPO}/releases/tags/{version}']))
    if release.get('draft') or release.get('prerelease') or release.get('tag_name') != version:
        raise ValueError('only formal published releases may deploy')
    assets = ['deployment.json', 'checksums.txt', 'server-management.tar.gz', 'management-source.txt', 'management-checksums.txt']
    names = [a['name'] for a in release['assets']]
    if any(names.count(n) != 1 for n in assets): raise ValueError('release inventory incomplete')
    run(['gh', 'release', 'download', version, '--repo', REPO, '--dir', str(folder)] + sum([['--pattern', n] for n in assets], []))
    for checksum in ['checksums.txt', 'management-checksums.txt']:
        for line in (folder / checksum).read_text().splitlines():
            digest, name = line.split(None, 1); name = name.lstrip('*')
            if Path(name).name != name or not re.fullmatch(r'[0-9a-f]{64}', digest): raise ValueError('invalid asset checksum')
            if name.endswith('.sql') and not (folder / name).exists():
                run(['gh', 'release', 'download', version, '--repo', REPO, '--dir', str(folder), '--pattern', name])
            if file_hash(folder / name) != digest: raise ValueError('asset checksum mismatch')
    manifest = read(folder / 'deployment.json'); validate_manifest(manifest, version)
    # Require the current-run verification record produced with the release source.
    receipt = manifest.get('channel_verification_record')
    if not isinstance(receipt,dict) or receipt.get('path') != 'docs/multimodal/CHANNEL_VERIFICATION_CURRENT.json' or not re.fullmatch(r'[0-9a-f]{64}',receipt.get('sha256','')): raise ValueError('current release verification record missing')
    source = json.loads(run(['gh','api',f"repos/{REPO}/contents/{receipt['path']}?ref={manifest['commit']}"]))
    import base64,hashlib
    evidence=base64.b64decode(source['content'])
    if hashlib.sha256(evidence).hexdigest()!=receipt['sha256']:raise ValueError('verification record checksum mismatch')
    parsed=json.loads(evidence)
    if parsed.get('version')!=version or parsed.get('status')!='recorded':raise ValueError('verification record version mismatch')
    revision = json.loads(run(['gh', 'api', f'repos/{REPO}/commits/{version}']))['sha']
    comparison = json.loads(run(['gh', 'api', f'repos/{REPO}/compare/{revision}...main']))
    if revision != manifest['commit'] or comparison['status'] not in ('ahead', 'identical'):
        raise ValueError('release source is not on main')
    if (folder / 'management-source.txt').read_text().strip() != revision:
        raise ValueError('manager source differs from release source')
    return manifest

def worker(job):
    log = AUTO / (job['id'] + '.log')
    try:
        with (STATE / 'operations.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            current = json.loads(run(['curl', '-fsS', '--max-time', '10', 'http://127.0.0.1:8081/api/v1/settings/public']))['data']['version']
            current = 'v' + current.lstrip('v'); check_order(job['version'], current)
            folder = AUTO / (job['id'] + '-assets'); folder.mkdir(mode=0o700)
            save(job, 'running', 'validating_release')
            manifest = release_assets(job['version'], folder)
            job.update(digest=manifest['digest'], commit=manifest['commit']); save(job)
            image = validate_manifest(manifest, job['version'])
            save(job, phase='pulling_fixed_digest'); run(['docker', 'pull', image])
            identity = inspect(image)
            labels = identity['Config'].get('Labels', {})
            if identity['Os'] != 'linux' or identity['Architecture'] != 'amd64' or labels.get('org.opencontainers.image.revision') != manifest['commit'] or labels.get('org.opencontainers.image.version') != job['version']:
                raise ValueError('image source or architecture mismatch')
            if current == job['version']:
                verify_online(job['version'], identity['Id'])
                save(job, 'succeeded', 'already_deployed'); return
            staged = STATE / 'releases' / job['version']; staged.mkdir(exist_ok=True, mode=0o700)
            if (staged / 'deployment.json').exists() and read(staged / 'deployment.json')['digest'] != manifest['digest']:
                raise ValueError('immutable version digest changed')
            run(['docker', 'tag', image, IMAGE + ':' + job['version']])
            atomic_json(staged / 'deployment.json', manifest); atomic_json(staged / 'image-inspect.json', [identity])
            # Install checksum-verified release management sources without executing
            # archive paths or commands supplied through the SSH request.
            import tarfile
            tooling = folder / 'manager'; tooling.mkdir(mode=0o700)
            allowed = {'manage.py','controller.py','media_migrate.py','install-manager.sh','SERVER-MANAGEMENT.md','auto_deploy.py'}
            with tarfile.open(folder / 'server-management.tar.gz') as archive:
                if any(m.name not in allowed or not m.isfile() for m in archive.getmembers()): raise ValueError('invalid management archive')
                archive.extractall(tooling, filter='data')
            run(['sh', str(tooling / 'install-manager.sh')])
            # Run the actual newly installed manager under the same operations lock.
            launcher = Path('/root/sub2api-manage').read_text()
            manager_path = re.search(r'(/root/sub2api-ops/releases/[0-9a-f]+/manage.py)', launcher).group(1)
            job['manager_job'] = uuid.uuid4().hex
            manager_job = {'id': job['manager_job'], 'action':'upgrade', 'target':job['version'], 'immediate':True}
            save(job, phase='backup_restore_and_cutover')
            # Import the verified installed module in an isolated Python process;
            # execute directly because this worker already owns operations.lock.
            outcome = execute_manager(manager_path, manager_job)
            job['backup_id'] = outcome.get('backup_id')
            if outcome['status'] != 'succeeded': raise ValueError('manager did not complete')
            try:
                verify_online(job['version'], identity['Id'])
            except Exception:
                save(job, phase='rolling_back_online_verification')
                rescue = {'id':uuid.uuid4().hex,'action':'rescue','target':manager_job['id']}
                try:
                    execute_manager(manager_path, rescue)
                    save(job, 'failed_rolled_back')
                except Exception:
                    save(job, 'failed_needs_inspection')
                raise
            atomic_json(AUTO / 'deployed.json', {'version':job['version'], 'digest':job['digest'], 'commit':job['commit'], 'job':job['id']})
            save(job, 'succeeded', 'online_verified')
    except Exception as error:
        # Raw subprocess output is never returned to GitHub.
        if job.get('status') not in ('failed_rolled_back','failed_needs_inspection'):
            manager_result = read(STATE / 'jobs' / (job['manager_job'] + '.json')) if job.get('manager_job') else {}
            job['backup_id'] = manager_result.get('backup_id')
            save(job, manager_result.get('status') if manager_result.get('status') in TERMINAL else 'failed', phase='inspect_host_log')
        job['error'] = str(error) if isinstance(error, ValueError) else type(error).__name__; save(job)
        raise

def dispatch(words):
    if len(words) != 2 or words[0] not in ('deploy','status'): raise ValueError('allowed commands: deploy stable-version; status job-id')
    AUTO.mkdir(parents=True, exist_ok=True, mode=0o700)
    if words[0] == 'status':
        if not re.fullmatch(r'[0-9a-f]{32}', words[1]): raise ValueError('invalid job id')
        job = read(AUTO / (words[1] + '.json'))
        if job['status'] not in TERMINAL:
            state = subprocess.run(['systemctl','is-active','sub2api-auto-' + job['id']], capture_output=True, text=True)
            if state.returncode:
                job.update(status='failed', error='worker not running; inspect host log')
                save(job)
        return clean_status(job)
    version_tuple(words[1])
    with (AUTO / 'dispatch.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        check_order(words[1], read(AUTO / 'deployed.json')['version'] if (AUTO / 'deployed.json').exists() else '')
        for path in AUTO.glob('*.json'):
            job = read(path)
            if 'id' in job and job.get('version') == words[1] and job.get('status') not in TERMINAL:
                return clean_status(job)
        job={'id':uuid.uuid4().hex,'version':words[1],'status':'queued'}; save(job)
        log=AUTO / (job['id'] + '.log'); log.touch(mode=0o600)
        subprocess.run(['systemd-run','--unit=sub2api-auto-' + job['id'],'--collect','--property=Type=exec','--property=UMask=0077','--setenv=HOME=/root','--setenv=XDG_CONFIG_HOME=/root/.config','--property=StandardOutput=append:' + str(log),'--property=StandardError=append:' + str(log),sys.executable,str(Path(__file__).resolve()),'_worker',job['id']],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        return clean_status(job)

if __name__ == '__main__':
    os.umask(0o077)
    if os.geteuid() != 0: raise SystemExit('root required')
    if len(sys.argv) == 3 and sys.argv[1] == '_worker': worker(read(AUTO / (sys.argv[2] + '.json')))
    else:
        try: print(json.dumps(dispatch(shlex.split(os.environ.get('SSH_ORIGINAL_COMMAND', '')))))
        except Exception as error: print(json.dumps({'status':'rejected','error':str(error) if isinstance(error,ValueError) else type(error).__name__})); raise SystemExit(1)
