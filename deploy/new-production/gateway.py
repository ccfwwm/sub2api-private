#!/usr/bin/env python3
"""Root-owned forced SSH entry; release archives never replace this program."""
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shlex
import subprocess
import sys
import tarfile
import time
import uuid

HOME = Path(__file__).resolve().parent
STATE = Path('/var/lib/codeme-deploy') / HOME.name
TERMINAL = {'succeeded', 'failed', 'failed_rolled_back', 'failed_needs_inspection'}


def read(path):
    return json.loads(Path(path).read_text())


def atomic(path, value):
    path = Path(path)
    temp = path.with_name(path.name + '.tmp')
    with temp.open('w') as f:
        os.chmod(temp, 0o600)
        json.dump(value, f, indent=2)
        f.flush()
        os.fsync(f.fileno())
    os.replace(temp, path)


def run(args, data=None, timeout=3600, cwd=None, stdout=None):
    p = subprocess.run([str(x) for x in args], input=data, stdout=stdout or subprocess.PIPE,
                       stderr=subprocess.PIPE, timeout=timeout, cwd=cwd)
    if p.returncode:
        # This output is only written to root-only systemd job logs.
        sys.stderr.buffer.write(p.stderr[-65536:])
        raise RuntimeError(f'{Path(str(args[0])).name} exited {p.returncode}; inspect host job log')
    return p.stdout


def inspect(name):
    return json.loads(run(['docker', 'inspect', name]))[0]


def extract(archive, destination):
    with tarfile.open(archive, 'r:gz') as tf:
        items = tf.getmembers()
        total = 0
        seen = set()
        for member in items:
            p = PurePosixPath(member.name)
            if (p.is_absolute() or '..' in p.parts or '\\' in member.name or
                    not (member.isfile() or member.isdir()) or
                    any(x in p.parts for x in ('.git', 'node_modules', '.ssh', '.env')) or
                    member.name in seen or member.mode & 0o6000):
                raise ValueError('unsafe or duplicate release member')
            seen.add(member.name)
            total += member.size
        if len(items) > 100000 or total > 2 * 1024**3:
            raise ValueError('expanded release is too large')
        destination.mkdir(mode=0o700)
        tf.extractall(destination, filter='data')


def health(url, attempts=60, host=None):
    for _ in range(attempts):
        headers = ['-H', 'Host: ' + host] if host else []
        p = subprocess.run(['curl', '--noproxy', '*', '-fsS', '--max-time', '5', *headers, url], capture_output=True)
        if p.returncode == 0:
            return p.stdout
        time.sleep(2)
    raise ValueError('application health did not become ready')


def scan(image, receipt, allowed_unfixed=()):
    scanner = '/var/lib/migration-20261117/security/trivy'
    # Update the vulnerability database for each deployment; network/database
    # failures stop preparation instead of treating an absent scan as clean.
    run([scanner, 'image', '--scanners', 'vuln', '--severity', 'HIGH,CRITICAL',
         '--ignorefile', '', '--format', 'json', '--output', receipt, image], timeout=1800)
    findings = [v for result in read(receipt).get('Results', []) for v in result.get('Vulnerabilities', [])]
    rejected = [v for v in findings if v['Severity'] == 'CRITICAL' and
                not ((v['VulnerabilityID'], v['PkgName']) in allowed_unfixed and not v.get('FixedVersion'))]
    if rejected:
        raise ValueError('unaccepted CRITICAL vulnerability; inspect private scan receipt')


def worker(ident):
    import handler
    job_path = STATE / (ident + '.json')
    job = read(job_path)

    def phase(value):
        job.update(phase=value, updated=time.time())
        atomic(job_path, job)

    try:
        with (STATE / 'operations.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            job['status'] = 'running'
            phase('validating')
            source = STATE / (ident + '-source')
            extract(STATE / (ident + '.tar.gz'), source)
            result = handler.deploy(job['revision'], source, job['prepare_only'], phase)
            job.update(status='succeeded', result=result)
            phase('prepared' if job['prepare_only'] else 'online_verified')
    except Exception as error:
        job.update(status=getattr(error, 'deploy_status', 'failed'), error=str(error) if isinstance(error, ValueError) else type(error).__name__)
        atomic(job_path, job)
        raise


def dispatch(words, stream):
    import handler
    if words == ['verify']:
        return {'status': 'succeeded', 'host': '206.82.2.46', 'service': HOME.name, **handler.verify()}
    if len(words) == 2 and words[0] == 'status' and re.fullmatch('[0-9a-f]{32}', words[1]):
        job = read(STATE / (words[1] + '.json'))
        if job['status'] not in TERMINAL:
            active = subprocess.run(['systemctl', 'is-active', '--quiet', 'codeme-deploy-' + job['id']]).returncode == 0
            if not active:
                job.update(status='failed_needs_inspection', error='detached worker stopped; inspect host log')
                atomic(STATE / (job['id'] + '.json'), job)
        return job
    if (len(words) != 3 or words[0] not in ('deploy', 'prepare') or
            not handler.valid_revision(words[1]) or not re.fullmatch('[0-9a-f]{64}', words[2])):
        raise ValueError('allowed: verify; status JOB; deploy|prepare REVISION SHA256 < archive')
    ident = uuid.uuid4().hex
    archive = STATE / (ident + '.tar.gz')
    digest = hashlib.sha256()
    size = 0
    try:
        with archive.open('xb') as out:
            while block := stream.read(1024 * 1024):
                size += len(block)
                if size > 512 * 1024**2:
                    raise ValueError('compressed release is too large')
                digest.update(block)
                out.write(block)
        if digest.hexdigest() != words[2]:
            raise ValueError('release archive checksum mismatch')
        job = dict(id=ident, revision=words[1], prepare_only=words[0] == 'prepare', status='queued', phase='received', bytes=size)
        atomic(STATE / (ident + '.json'), job)
        log = STATE / (ident + '.log')
        log.touch(mode=0o600)
        run(['systemd-run', '--quiet', '--collect', '--unit=codeme-deploy-' + ident,
             '--property=UMask=0077', '--property=Type=exec', '--property=RuntimeMaxSec=10800',
             '--property=StandardOutput=append:' + str(log), '--property=StandardError=append:' + str(log),
             sys.executable, str(HOME / 'gateway.py'), '_worker', ident])
        return job
    except BaseException:
        archive.unlink(missing_ok=True)
        raise


if __name__ == '__main__':
    os.umask(0o077)
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    if len(sys.argv) == 3 and sys.argv[1] == '_worker' and re.fullmatch('[0-9a-f]{32}', sys.argv[2]):
        worker(sys.argv[2])
    else:
        try:
            print(json.dumps(dispatch(shlex.split(os.environ.get('SSH_ORIGINAL_COMMAND', '')), sys.stdin.buffer)))
        except Exception as error:
            print(json.dumps({'status': 'rejected', 'error': str(error) if isinstance(error, ValueError) else type(error).__name__}))
            raise SystemExit(1)
