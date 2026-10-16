#!/usr/bin/env python3
"""Enable compression only in the two existing Sub2API HTTPS virtual hosts."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from manage import Manager

INCLUDE = 'include /etc/nginx/snippets/sub2api-web-performance.conf;'
SNIPPET = '''# Page/static compression. Existing API locations keep gzip off.
gzip on;
gzip_vary on;
gzip_proxied any;
gzip_comp_level 5;
gzip_min_length 1024;
gzip_types text/plain text/css text/javascript application/javascript application/json application/xml image/svg+xml application/manifest+json;
'''
TARGETS = {'sub2api-code-xyz-https.conf': 'code.aicodeme.xyz',
           'sub2api-cn.conf': 'code.aicodeme.cn'}


def add_include(content, hostname):
    offset = 0
    while offset < len(content):
        start, end, insertion = Manager._nginx_server_block(content[offset:], hostname)
        start, end, insertion = start + offset, end + offset, insertion + offset
        block = content[start:end]
        if re.search(r'(?m)^\s*listen\s+443\b', block):
            if INCLUDE in block:
                return content
            return content[:insertion] + '\n    ' + INCLUDE + content[insertion:]
        offset = end
    raise ValueError('expected existing HTTPS virtual host: ' + hostname)


def apply():
    if os.geteuid() != 0:
        raise ValueError('root required')
    directory = Path('/etc/nginx/conf.d')
    snippet = Path('/etc/nginx/snippets/sub2api-web-performance.conf')
    originals = {directory / name: (directory / name).read_bytes() for name in TARGETS}
    originals[snippet] = snippet.read_bytes() if snippet.exists() else None
    if originals[snippet] not in (None, SNIPPET.encode()):
        raise ValueError('unexpected existing performance snippet')
    changes = {directory / name: add_include(originals[directory / name].decode(), host).encode()
               for name, host in TARGETS.items()}
    changes[snippet] = SNIPPET.encode()
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    backup = Path('/root/sub2api-ops/state') / ('web-performance-' + stamp)
    backup.mkdir(mode=0o700)
    for file, body in originals.items():
        if body is not None:
            (backup / file.name).write_bytes(body)
    try:
        for file, body in changes.items():
            file.write_bytes(body)
        subprocess.run(['nginx', '-t'], check=True)
        subprocess.run(['systemctl', 'reload', 'nginx'], check=True)
    except BaseException:
        for file, body in originals.items():
            if body is None:
                file.unlink(missing_ok=True)
            else:
                file.write_bytes(body)
        subprocess.run(['nginx', '-t'], check=True)
        subprocess.run(['systemctl', 'reload', 'nginx'], check=True)
        raise
    receipt = {'backup': str(backup), 'files': {str(p): hashlib.sha256(b).hexdigest()
               for p, b in changes.items()}, 'reload': True, 'application_restarted': False}
    (backup / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print(json.dumps(receipt))


if __name__ == '__main__':
    apply()
