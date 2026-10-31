#!/usr/bin/env python3
"""Summarize privacy-safe JSONL capacity samples without guessing resource limits."""
from __future__ import annotations

import argparse
import datetime as dt
import json
import math
from pathlib import Path


def percentile(values, p):
    values = sorted(value for value in values if isinstance(value, (int, float)))
    if not values:
        return None
    return values[min(len(values) - 1, math.ceil(p * len(values)) - 1)]


def metric_summary(values):
    values = [value for value in values if isinstance(value, (int, float))]
    if not values:
        return {'count': 0, 'min': None, 'p50': None, 'p95': None, 'p99': None, 'max': None}
    return {'count': len(values), 'min': min(values), 'p50': percentile(values, .50),
            'p95': percentile(values, .95), 'p99': percentile(values, .99), 'max': max(values)}


def value_at(row, *path):
    value = row
    for key in path:
        value = value.get(key) if isinstance(value, dict) else None
    return value


def number(row, *path):
    value = value_at(row, *path)
    return value if isinstance(value, (int, float)) else None


def summarize(samples, expected_interval=60, min_days=7):
    timestamps = []
    for row in samples:
        try:
            value = dt.datetime.fromisoformat(row['timestamp'].replace('Z', '+00:00'))
            timestamps.append(value.timestamp())
        except (KeyError, TypeError, ValueError):
            continue
    timestamps.sort()
    duration = max(0, timestamps[-1] - timestamps[0]) if len(timestamps) > 1 else 0
    gaps = [b - a for a, b in zip(timestamps, timestamps[1:])]
    container_names = sorted(
        {item.get('name') or 'unknown' for row in samples for item in row.get('docker', [])} |
        {item.get('name') or 'unknown' for row in samples for item in row.get('container_state', [])} |
        {name for row in samples for name in row.get('oom_events', {})})
    containers = {}
    for name in container_names:
        stats = [item for row in samples for item in row.get('docker', []) if (item.get('name') or 'unknown') == name]
        states = [item for row in samples for item in row.get('container_state', []) if item.get('name') == name]
        containers[name] = {
            'cpu_pct': metric_summary([item.get('cpu_pct') for item in stats]),
            'memory_pct': metric_summary([item.get('memory_pct') for item in stats]),
            'memory_used_bytes': metric_summary([item.get('memory_used_bytes') for item in stats]),
            'restart_count_max': max((item.get('restarts') for item in states
                                      if isinstance(item.get('restarts'), int)), default=None),
            'oom_killed_seen': any(item.get('oom_killed') is True for item in states),
            'oom_events': sum(row.get('oom_events', {}).get(name, 0) for row in samples),
            'unhealthy_samples': sum(item.get('health') == 'unhealthy' for item in states),
        }

    postgres = {
        'connection_pct': metric_summary([
            value * 100 / maximum for row in samples
            if (value := number(row, 'postgres', 'connections')) is not None
            and (maximum := number(row, 'postgres', 'max_connections'))]),
        'waiting_queries': metric_summary([number(row, 'postgres', 'waiting_queries') for row in samples]),
        'lock_waiting_queries': metric_summary([number(row, 'postgres', 'lock_waiting_queries') for row in samples]),
        'io_waiting_queries': metric_summary([number(row, 'postgres', 'io_waiting_queries') for row in samples]),
        'active_connections': metric_summary([number(row, 'postgres', 'active_connections') for row in samples]),
        'slow_active_queries': metric_summary([number(row, 'postgres', 'slow_active_queries') for row in samples]),
        'oldest_active_seconds': metric_summary([number(row, 'postgres', 'oldest_active_seconds') for row in samples]),
        'slow_statement_fingerprints': metric_summary([number(row, 'postgres', 'slow_statement_fingerprints') for row in samples]),
        'slow_statement_execution_calls': metric_summary([number(row, 'postgres', 'slow_exec_calls') for row in samples]),
        'database_size_bytes': metric_summary([number(row, 'postgres', 'database_size_bytes') for row in samples]),
        'pg_stat_statements_available_samples': sum(number(row, 'postgres', 'pg_stat_statements_available') is True for row in samples),
        'pg_stat_statements_unavailable_samples': sum(number(row, 'postgres', 'pg_stat_statements_available') is False for row in samples),
    }
    redis = {
        'used_memory_bytes': metric_summary([number(row, 'redis', 'used_memory') for row in samples]),
        'peak_memory_bytes': metric_summary([number(row, 'redis', 'used_memory_peak') for row in samples]),
        'memory_limit_bytes': metric_summary([number(row, 'redis', 'maxmemory') for row in samples]),
        'connected_clients': metric_summary([number(row, 'redis', 'connected_clients') for row in samples]),
        'blocked_clients': metric_summary([number(row, 'redis', 'blocked_clients') for row in samples]),
        'rejected_connections': metric_summary([number(row, 'redis', 'rejected_connections') for row in samples]),
        'evicted_keys': metric_summary([number(row, 'redis', 'evicted_keys') for row in samples]),
        'probe_latency_ms': metric_summary([number(row, 'redis', 'latency_ms') for row in samples]),
        'maxclients': max((number(row, 'redis', 'maxclients') for row in samples
                           if number(row, 'redis', 'maxclients') is not None), default=None),
        'maxmemory_policy': sorted({value_at(row, 'redis', 'maxmemory_policy') for row in samples
                                    if isinstance(value_at(row, 'redis', 'maxmemory_policy'), str)}),
    }
    filesystems = {}
    for row in samples:
        for path, stats in (row.get('host', {}).get('filesystems') or {}).items():
            filesystems.setdefault(path, []).append((stats or {}).get('used_pct'))
    io_devices = sorted({device for row in samples for device in (row.get('host', {}).get('disk_io') or {})})
    host = {
        'cpu_pct': metric_summary([number(row, 'host', 'cpu_pct') for row in samples]),
        'iowait_pct': metric_summary([number(row, 'host', 'iowait_pct') for row in samples]),
        'memory_used_pct': metric_summary([number(row, 'host', 'memory', 'used_pct') for row in samples]),
        'memory_available_bytes': metric_summary([number(row, 'host', 'memory', 'available_bytes') for row in samples]),
        'filesystems_used_pct': {path: metric_summary(values) for path, values in filesystems.items()},
        'disk_io': {device: {
            key: metric_summary([number(row, 'host', 'disk_io', device, key) for row in samples])
            for key in ('read_mib_s', 'write_mib_s', 'io_busy_pct', 'weighted_io_ms_per_second')
        } for device in io_devices},
    }
    errors = {}
    for row in samples:
        for component, error in (row.get('errors') or {}).items():
            if error:
                errors[component] = errors.get(component, 0) + 1
    report = {
        'sample_count': len(samples),
        'window': {'start_utc': dt.datetime.fromtimestamp(timestamps[0], dt.timezone.utc).isoformat() if timestamps else None,
                   'end_utc': dt.datetime.fromtimestamp(timestamps[-1], dt.timezone.utc).isoformat() if timestamps else None,
                   'observed_days': round(duration / 86400, 3),
                   'expected_interval_seconds': expected_interval,
                   'max_gap_seconds': round(max(gaps), 2) if gaps else None,
                   'gap_samples_over_2x_interval': sum(gap > expected_interval * 2 for gap in gaps)},
        'host': host,
        'application_probe_latency_ms': metric_summary([number(row, 'app', 'latency_ms') for row in samples]),
        'application_probe_failures': sum(number(row, 'app', 'healthy') is False for row in samples),
        'containers': containers,
        'postgres': postgres,
        'redis': redis,
        'collector_errors': errors,
    }
    blockers = []
    if duration < min_days * 86400:
        blockers.append(f'observed window is shorter than {min_days:g} days')
    if report['window']['gap_samples_over_2x_interval']:
        blockers.append('sample gaps exceed twice the requested interval')
    if report['application_probe_failures']:
        blockers.append('application health probe failures occurred')
    if errors:
        blockers.append('one or more data sources had collection errors')
    if not samples:
        blockers.append('no samples were collected')
    report['sizing_gate'] = {
        'ready_for_capacity_review': not blockers,
        'blockers': blockers,
        'decision': 'Use these observations with the 1.5x peak load test. Set host and per-container limits only after restore validation; keep at least 20% host capacity available. Coordinate application pool totals with PostgreSQL max_connections and Redis pool totals with maxclients. This report never proposes resource values.'
    }
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('input', type=Path)
    parser.add_argument('--expected-interval-seconds', type=int, default=60)
    parser.add_argument('--minimum-days', type=float, default=7)
    args = parser.parse_args()
    if args.expected_interval_seconds < 1 or args.minimum_days < 0:
        parser.error('expected interval must be positive and minimum days must be non-negative')
    samples = [json.loads(line) for line in args.input.read_text(encoding='utf-8').splitlines() if line.strip()]
    print(json.dumps(summarize(samples, args.expected_interval_seconds, args.minimum_days),
                     ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
