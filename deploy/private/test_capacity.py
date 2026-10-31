import json
from pathlib import Path
import shlex
import unittest
from unittest.mock import patch

from capacity_collect import parse_cpu_stat, parse_docker_stats, parse_meminfo, parse_redis_info, postgres_probe
from capacity_report import summarize


class CapacityCollectorTests(unittest.TestCase):
    def test_redis_capacity_overlay_requires_an_explicit_client_budget(self):
        overlay = Path(__file__).resolve().parents[1] / 'docker-compose.capacity.yml'
        text = overlay.read_text(encoding='utf-8')
        self.assertIn('--maxclients ${CAPACITY_REDIS_MAXCLIENTS:?', text)
        self.assertIn('CAPACITY_REDIS_POOL_SIZE', text)

    def test_host_and_container_parsers_keep_units_explicit(self):
        self.assertEqual(parse_cpu_stat('cpu  10 2 3 40 5 1 0 0 0 0\n'), (61, 45, 1))
        memory = parse_meminfo('MemTotal:       1024 kB\nMemAvailable:    256 kB\n')
        self.assertEqual(memory['total_bytes'], 1024 * 1024)
        self.assertEqual(memory['used_pct'], 75.0)
        stats = parse_docker_stats('{"Name":"sub2api","CPUPerc":"12.50%","MemUsage":"256MiB / 1GiB","PIDs":"7"}')
        self.assertEqual(stats[0]['memory_used_bytes'], 256 * 1024 * 1024)
        self.assertEqual(stats[0]['memory_pct'], 25.0)

    def test_redis_parser_excludes_unbounded_text(self):
        result = parse_redis_info('# Memory\nused_memory:123\nused_memory_peak:456\nconnected_clients:7\n')
        self.assertEqual(result, {'used_memory': 123, 'used_memory_peak': 456, 'connected_clients': 7})

    def test_report_blocks_short_or_incomplete_window_and_does_not_size(self):
        sample = {
            'timestamp': '2026-10-01T00:00:00+00:00',
            'host': {'cpu_pct': 10, 'iowait_pct': 1, 'memory': {'used_pct': 20, 'available_bytes': 100}, 'filesystems': {}, 'disk_io': {}},
            'app': {'latency_ms': 4, 'healthy': True}, 'docker': [], 'container_state': [],
            'postgres': {'connections': 2, 'max_connections': 10, 'waiting_connections': 0, 'active_connections': 1,
                         'slow_active_queries': 0, 'oldest_active_seconds': 0, 'pg_stat_statements_available': False},
            'redis': {'used_memory': 1, 'used_memory_peak': 1, 'maxmemory': 10, 'connected_clients': 1,
                      'blocked_clients': 0, 'rejected_connections': 0, 'evicted_keys': 0, 'latency_ms': 1,
                      'maxclients': 10, 'maxmemory_policy': 'noeviction'},
            'errors': {},
        }
        report = summarize([sample])
        self.assertFalse(report['sizing_gate']['ready_for_capacity_review'])
        self.assertTrue(report['sizing_gate']['blockers'])
        self.assertNotIn('recommended_cpu', json.dumps(report))
        sample['app']['healthy'] = False
        health_failure = summarize([sample], min_days=0)
        self.assertFalse(health_failure['sizing_gate']['ready_for_capacity_review'])
        self.assertIn('application health probe failures occurred', health_failure['sizing_gate']['blockers'])

    def test_postgres_probe_keeps_credentials_out_of_command_and_query_text(self):
        core = {'pg_stat_statements_available': False, 'connections': 3, 'max_connections': 10}
        with patch('capacity_collect.run', return_value=(json.dumps(core), None)) as command:
            result, error = postgres_probe('postgres')
        self.assertIsNone(error)
        self.assertEqual(result['connections'], 3)
        shell_command = command.call_args.args[0][-1]
        self.assertIn('PGPASSWORD="${POSTGRES_PASSWORD:-}"', shell_command)
        self.assertNotIn('do-not-record', shell_command)
        query = shlex.split(shell_command)[-1]
        self.assertIn('waiting_queries', query)
        self.assertIn('query_start', query)
        self.assertIn('\n', query)
        self.assertNotIn(r'\n', query)

    def test_postgres_slow_query_probe_returns_fingerprints_without_query_text(self):
        core = {'pg_stat_statements_available': True, 'connections': 3, 'max_connections': 10}
        slow = {'slow_statement_fingerprints': 2, 'slow_exec_calls': 9}
        with patch('capacity_collect.run', side_effect=[(json.dumps(core), None), (json.dumps(slow), None)]) as command:
            result, error = postgres_probe('postgres')
        self.assertIsNone(error)
        self.assertEqual(result['slow_exec_calls'], 9)
        self.assertEqual(command.call_count, 2)
        self.assertIn('mean_exec_time', command.call_args_list[1].args[0][-1])


if __name__ == '__main__':
    unittest.main()
