#!/usr/bin/env python3
"""CI-only fresh-stack smoke. Contains no production access or credentials."""
import argparse
import json
import secrets
import subprocess
import time

p = argparse.ArgumentParser()
p.add_argument('--image', required=True)
p.add_argument('--version', required=True)
a = p.parse_args()

def run(args):
    return subprocess.check_output(args, stderr=subprocess.STDOUT).decode()

def wait(args, seconds=180):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            return run(args)
        except subprocess.CalledProcessError:
            time.sleep(2)
    raise RuntimeError('CI image smoke timed out')

names = ['sub2api-ci-app', 'sub2api-ci-redis', 'sub2api-ci-pg']
network = 'sub2api-ci-isolated'
password = secrets.token_hex(24)
try:
    for image in [a.image, 'postgres:18-alpine', 'redis:8-alpine']:
        run(['docker', 'pull', image])
    run(['docker', 'network', 'create', '--internal', network])
    run(['docker', 'run', '-d', '--name', names[2], '--network', network,
         '-e', 'POSTGRES_USER=sub2api', '-e', 'POSTGRES_DB=sub2api', '-e', 'POSTGRES_PASSWORD=' + password,
         'postgres:18-alpine'])
    wait(['docker', 'exec', names[2], 'pg_isready', '-h', '127.0.0.1', '-U', 'sub2api', '-d', 'sub2api'])
    run(['docker', 'run', '-d', '--name', names[1], '--network', network, 'redis:8-alpine'])
    env = {'AUTO_SETUP': 'true', 'DATABASE_HOST': names[2], 'DATABASE_PORT': '5432',
           'DATABASE_USER': 'sub2api', 'DATABASE_DBNAME': 'sub2api', 'DATABASE_PASSWORD': password,
           'DATABASE_SSLMODE': 'disable', 'REDIS_HOST': names[1], 'REDIS_PORT': '6379',
           'JWT_SECRET': secrets.token_hex(32), 'TOTP_ENCRYPTION_KEY': secrets.token_hex(32),
           'ADMIN_EMAIL': 'smoke@example.invalid', 'ADMIN_PASSWORD': secrets.token_hex(20)}
    args = ['docker', 'run', '-d', '--name', names[0], '--network', network]
    for k, v in env.items():
        args += ['-e', k + '=' + v]
    run(args + [a.image])
    wait(['docker', 'exec', names[0], 'wget', '-qO-', 'http://127.0.0.1:8080/health'])
    settings = json.loads(run(['docker', 'exec', names[0], 'wget', '-qO-', 'http://127.0.0.1:8080/api/v1/settings/public']))
    assert settings['data']['version'].lstrip('v') == a.version.lstrip('v'), settings['data']['version']
    page = run(['docker', 'exec', names[0], 'wget', '-qO-', 'http://127.0.0.1:8080/'])
    assert '<html' in page.lower(), 'embedded frontend missing'
    login = json.loads(run(['docker', 'exec', names[0], 'wget', '-qO-',
                           '--header=Content-Type: application/json',
                           '--post-data=' + json.dumps({'email': env['ADMIN_EMAIL'], 'password': env['ADMIN_PASSWORD']}),
                           'http://127.0.0.1:8080/api/v1/auth/login']))
    assert login.get('data', {}).get('access_token'), 'fresh admin login failed'
    print('PASS: image health, database initialization, embedded frontend/version and admin login')
finally:
    for name in names:
        subprocess.run(['docker', 'rm', '-fv', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['docker', 'network', 'rm', network], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
