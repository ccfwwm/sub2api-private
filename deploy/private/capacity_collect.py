#!/usr/bin/env python3
"""Collect host and single-host Sub2API capacity samples as privacy-safe JSONL."""
from __future__ import annotations

import argparse
import datetime as dt
import json
from pathlib import Path
import re
import shlex
import shutil
import signal
import subprocess
import sys
import time
from urllib.error import URLError
from urllib.request import urlopen


_STOP = False


def stop(_signum, _frame):
    global _STOP
    _STOP = True


def run(command, timeout=10):
    try:
        result = subprocess.run(command, check=True, text=True, capture_output=True, timeout=timeout)
        return result.stdout, None
    except (OSError, subprocess.SubprocessError) as exc:
        return None, type(exc).__name__


def read_text(path):
    try:
        return Path(path).read_text(encoding='utf-8')
    except OSError:
        return None


def parse_cpu_stat(text):
    if not text:
        return None
    first = next((line for line in text.splitlines() if line.startswith('cpu ')), None)
    if not first:
        return None
    fields = [int(value) for value in first.split()[1:]]
    if len(fields) < 4:
        return None
    idle = fields[3] + (fields[4] if len(fields) > 4 else 0)
    iowait = fields[5] if len(fields) > 5 else 0
    return sum(fields), idle, iowait


def parse_meminfo(text):
    if not text:
        return None
    values = {}
    for line in text.splitlines():
        match = re.match(r'^(MemTotal|MemAvailable):\s+(\d+)\s+kB$', line)
        if match:
            values[match.group(1)] = int(match.group(2)) * 1024
    total, available = values.get('MemTotal'), values.get('MemAvailable')
    if not total or available is None:
        return None
    return {'total_bytes': total, 'available_bytes': available,
            'used_pct': round((total - available) * 100 / total, 2)}


def parse_diskstats(text):
    result = {}
    if not text:
        return result
    for line in text.splitlines():
        fields = line.split()
        if len(fields) < 14:
            continue
        name = fields[2]
        if name.startswith(('loop', 'ram', 'zram')):
            continue
        try:
            result[name] = {
                'reads_completed': int(fields[3]),
                'sectors_read': int(fields[5]),
                'read_ms': int(fields[6]),
                'writes_completed': int(fields[7]),
                'sectors_written': int(fields[9]),
                'write_ms': int(fields[10]),
                'io_in_progress': int(fields[11]),
                'io_ms': int(fields[12]),
                'weighted_io_ms': int(fields[13]),
            }
        except ValueError:
            continue
    return result


def _bytes(value):
    match = re.fullmatch(r'\s*([\d.]+)\s*([KMGT]i?B|B)?\s*', value, re.I)
    if not match:
        return None
    number, unit = float(match.group(1)), (match.group(2) or 'B').upper()
    powers = {'B': 0, 'KB': 1, 'KIB': 1, 'MB': 2, 'MIB': 2,
              'GB': 3, 'GIB': 3, 'TB': 4, 'TIB': 4}
    if unit not in powers:
        return None
    return int(number * (1024 ** powers[unit]))


def parse_docker_stats(text):
    rows = []
    for line in (text or '').splitlines():
        try:
            row = json.loads(line)
            memory = row.get('MemUsage', '').split('/', 1)
            used = _bytes(memory[0]) if memory else None
            limit = _bytes(memory[1]) if len(memory) == 2 else None
            cpu = float(row.get('CPUPerc', '').strip().rstrip('%'))
            rows.append({'name': row.get('Name') or row.get('Container'), 'cpu_pct': cpu,
                         'memory_used_bytes': used, 'memory_limit_bytes': limit,
                         'memory_pct': round(used * 100 / limit, 2) if used is not None and limit else None,
                         'pids': int(row['PIDs']) if str(row.get('PIDs', '')).isdigit() else None})
        except (ValueError, TypeError, json.JSONDecodeError):
            continue
    return rows


def _json_command(command):
    output, error = run(command)
    if error:
        return None, error
    try:
        return json.loads(output.strip()), None
    except (ValueError, TypeError):
        return None, 'InvalidOutput'


def app_probe(url):
    started = time.monotonic()
    try:
        with urlopen(url, timeout=5) as response:
            return {'status': response.status, 'latency_ms': round((time.monotonic() - started) * 1000, 2),
                    'healthy': 200 <= response.status < 400}
    except (OSError, URLError, TimeoutError) as exc:
        return {'status': None, 'latency_ms': round((time.monotonic() - started) * 1000, 2),
                'healthy': False, 'error': type(exc).__name__}


def postgres_probe(container):
    sql = """SELECT json_build_object(
      'max_connections', current_setting('max_connections')::int,
      'database_size_bytes', pg_database_size(current_database()),
      'connections', (SELECT count(*) FROM pg_stat_activity),
      'active_connections', (SELECT count(*) FROM pg_stat_activity WHERE state = 'active'),
      'waiting_queries', (SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND wait_event IS NOT NULL),
      'lock_waiting_queries', (SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND wait_event_type = 'Lock'),
      'io_waiting_queries', (SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND wait_event_type = 'IO'),
      'oldest_active_seconds', (SELECT COALESCE(max(extract(epoch FROM now()-query_start)), 0)::int FROM pg_stat_activity WHERE state = 'active'),
      'slow_active_queries', (SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND query_start < now() - interval '1 second'),
      'pg_stat_statements_available', to_regclass('pg_stat_statements') IS NOT NULL
    )::text;"""
    output, error = _postgres_query(container, sql)
    if error:
        return None, error
    try:
        result = json.loads(output.strip())
    except (ValueError, TypeError):
        return None, 'InvalidOutput'
    if result.get('pg_stat_statements_available'):
        slow_sql = ("SELECT json_build_object('slow_statement_fingerprints', "
                    "count(*) FILTER (WHERE mean_exec_time >= 1000 OR max_exec_time >= 5000), "
                    "'slow_exec_calls', COALESCE(sum(calls) FILTER "
                    "(WHERE mean_exec_time >= 1000 OR max_exec_time >= 5000), 0))::text "
                    'FROM pg_stat_statements;')
        slow_output, slow_error = _postgres_query(container, slow_sql)
        if not slow_error:
            try:
                result.update(json.loads(slow_output.strip()))
            except (ValueError, TypeError):
                result['slow_statement_stats_error'] = 'InvalidOutput'
        else:
            result['slow_statement_stats_error'] = slow_error
    else:
        result['slow_statement_fingerprints'] = None
        result['slow_exec_calls'] = None
    return result, None


def _postgres_query(container, sql):
    shell = ('PGPASSWORD="${POSTGRES_PASSWORD:-}" psql -X -U "${POSTGRES_USER:-sub2api}" '
             '-d "${POSTGRES_DB:-sub2api}" -Atqc ' + shlex.quote(sql))
    return run(['docker', 'exec', container, 'sh', '-lc', shell])


def redis_probe(container):
    started = time.monotonic()
    output, error = run(['docker', 'exec', container, 'redis-cli', '--raw', 'INFO', 'ALL'])
    latency = round((time.monotonic() - started) * 1000, 2)
    if error:
        return None, error
    result = parse_redis_info(output)
    maxclients_output, maxclients_error = run(['docker', 'exec', container, 'redis-cli', '--raw', 'CONFIG', 'GET', 'maxclients'])
    policy_output, policy_error = run(['docker', 'exec', container, 'redis-cli', '--raw', 'CONFIG', 'GET', 'maxmemory-policy'])
    maxmemory_output, maxmemory_error = run(['docker', 'exec', container, 'redis-cli', '--raw', 'CONFIG', 'GET', 'maxmemory'])
    config_errors = [name for name, error in (('maxclients', maxclients_error),
                                               ('maxmemory_policy', policy_error),
                                               ('maxmemory', maxmemory_error)) if error]
    result.update({'maxclients': _second_line(maxclients_output) if not maxclients_error else None,
                   'maxmemory_policy': _second_line(policy_output) if not policy_error else None,
                   'maxmemory': _second_line(maxmemory_output) if not maxmemory_error else result.get('maxmemory'),
                   'latency_ms': latency, 'configuration_errors': config_errors})
    return result, 'ConfigurationCommandFailed' if config_errors else None


def parse_redis_info(text):
    result = {}
    for line in (text or '').splitlines():
        if not line or line.startswith('#') or ':' not in line:
            continue
        key, value = line.split(':', 1)
        if key == 'maxmemory_policy':
            result[key] = value
        elif key in {'used_memory', 'used_memory_peak', 'used_memory_rss', 'maxmemory',
                   'connected_clients', 'blocked_clients', 'maxclients', 'evicted_keys',
                   'rejected_connections', 'total_connections_received'}:
            try:
                result[key] = int(value)
            except ValueError:
                pass
    return result


def _second_line(text):
    values = [line.strip() for line in (text or '').splitlines() if line.strip()]
    value = values[1] if len(values) > 1 else None
    return int(value) if value and value.isdigit() else value


def container_inspect(containers):
    if not containers:
        return [], 'NoContainers'
    rows, error = _json_command(['docker', 'inspect', *containers])
    if error:
        return [], error
    result = []
    for row in rows:
        state = row.get('State') or {}
        health = (state.get('Health') or {}).get('Status')
        result.append({'name': row.get('Name', '').lstrip('/'),
                       'restarts': row.get('RestartCount'), 'oom_killed': state.get('OOMKilled'),
                       'running': state.get('Running'), 'health': health})
    return result, None


class Collector:
    def __init__(self, health_url, containers, data_paths):
        self.health_url, self.containers, self.data_paths = health_url, containers, data_paths
        self.previous_cpu = None
        self.previous_disks = None
        self.previous_mono = None
        self.last_event_cursor = None

    def host_sample(self, now):
        cpu, memory, diskstats = parse_cpu_stat(read_text('/proc/stat')), parse_meminfo(read_text('/proc/meminfo')), parse_diskstats(read_text('/proc/diskstats'))
        disks, elapsed = {}, None
        if self.previous_mono is not None:
            elapsed = max(now - self.previous_mono, 0.001)
        if cpu and self.previous_cpu:
            total, idle = cpu[0] - self.previous_cpu[0], cpu[1] - self.previous_cpu[1]
            cpu_pct = round((total - idle) * 100 / total, 2) if total > 0 else None
            iowait_pct = round((cpu[2] - self.previous_cpu[2]) * 100 / total, 2) if total > 0 else None
        else:
            cpu_pct, iowait_pct = None, None
        if elapsed and self.previous_disks is not None:
            for name, current in diskstats.items():
                previous = self.previous_disks.get(name)
                if not previous:
                    continue
                delta = {key: max(current[key] - previous[key], 0) for key in current}
                disks[name] = {'read_mib_s': round(delta['sectors_read'] * 512 / elapsed / (1024 ** 2), 3),
                               'write_mib_s': round(delta['sectors_written'] * 512 / elapsed / (1024 ** 2), 3),
                               'io_busy_pct': round(delta['io_ms'] * 100 / (elapsed * 1000), 2),
                               'weighted_io_ms_per_second': round(delta['weighted_io_ms'] / elapsed, 2),
                               'io_in_progress': current['io_in_progress']}
        filesystem = {}
        for path in self.data_paths:
            try:
                usage = shutil.disk_usage(path)
                filesystem[path] = {'total_bytes': usage.total, 'used_bytes': usage.used,
                                    'free_bytes': usage.free, 'used_pct': round(usage.used * 100 / usage.total, 2)}
            except OSError:
                filesystem[path] = None
        self.previous_cpu, self.previous_disks, self.previous_mono = cpu, diskstats, now
        return {'cpu_pct': cpu_pct, 'iowait_pct': iowait_pct, 'memory': memory,
                'disk_io': disks, 'filesystems': filesystem}

    def sample(self):
        now = time.monotonic()
        wall_now = time.time()
        names = list(self.containers.values())
        stats, stats_error = run(['docker', 'stats', '--no-stream', '--format', '{{json .}}', *names])
        container_stats = parse_docker_stats(stats)
        containers, inspect_error = container_inspect(names)
        since = int(self.last_event_cursor if self.last_event_cursor is not None else wall_now - 60)
        events_output, events_error = run(['docker', 'events', f'--since={since}', f'--until={int(wall_now)}',
                                           '--filter', 'type=container', '--filter', 'event=oom',
                                           '--format', '{{json .}}'])
        oom_events = {}
        if not events_error:
            for line in (events_output or '').splitlines():
                try:
                    event = json.loads(line)
                    name = ((event.get('Actor') or {}).get('Attributes') or {}).get('name')
                    if name:
                        oom_events[name] = oom_events.get(name, 0) + 1
                except (ValueError, TypeError):
                    continue
        self.last_event_cursor = wall_now
        postgres, postgres_error = postgres_probe(self.containers['postgres'])
        redis, redis_error = redis_probe(self.containers['redis'])
        host = self.host_sample(now)
        return {'timestamp': dt.datetime.now(dt.timezone.utc).isoformat(timespec='seconds'),
                'host': host, 'app': app_probe(self.health_url), 'docker': container_stats,
                'container_state': containers, 'postgres': postgres, 'redis': redis,
                'oom_events': oom_events,
                'errors': {key: value for key, value in {
                    'docker_stats': stats_error, 'container_inspect': inspect_error,
                    'docker_oom_events': events_error,
                    'postgres': postgres_error, 'redis': redis_error,
                }.items() if value}}


def arguments(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True, help='append-only JSONL destination')
    parser.add_argument('--interval-seconds', type=int, default=60)
    parser.add_argument('--duration-seconds', type=int, default=7 * 24 * 60 * 60)
    parser.add_argument('--health-url', default='http://127.0.0.1:8080/health')
    parser.add_argument('--app-container', default='sub2api')
    parser.add_argument('--docs-container', default='sub2api-docs-web')
    parser.add_argument('--postgres-container', default='sub2api-postgres')
    parser.add_argument('--redis-container', default='sub2api-redis')
    parser.add_argument('--data-path', action='append', dest='data_paths')
    args = parser.parse_args(argv)
    if args.interval_seconds < 10 or args.duration_seconds < 0:
        parser.error('interval must be >= 10 seconds and duration must be >= 0')
    if not args.data_paths:
        args.data_paths = ['/']
        docker_root, error = run(['docker', 'info', '--format', '{{.DockerRootDir}}'])
        if not error and docker_root.strip() not in args.data_paths:
            args.data_paths.append(docker_root.strip())
    return args


def main(argv=None):
    args = arguments(argv)
    collector = Collector(args.health_url,
                          {'app': args.app_container, 'docs': args.docs_container,
                           'postgres': args.postgres_container,
                           'redis': args.redis_container}, args.data_paths)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    deadline = time.monotonic() + args.duration_seconds
    with args.output.open('a', encoding='utf-8', buffering=1) as output:
        while not _STOP:
            row = collector.sample()
            output.write(json.dumps(row, ensure_ascii=True, separators=(',', ':')) + '\n')
            output.flush()
            if time.monotonic() >= deadline:
                break
            time.sleep(min(args.interval_seconds, max(deadline - time.monotonic(), 0)))
    return 0


if __name__ == '__main__':
    sys.exit(main())
