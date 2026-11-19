"""Sealed, additive v1.3.26 migration set for the migrated production host.

This module is installed separately by the operator. Release archives cannot
change these hashes or authorize arbitrary SQL. Ordinary upgrades remain strict.
"""
import hashlib
import re

VERSION = 'v1.3.26'
SEALED = {
    '257_add_usage_logs_api_key_cost_index_notx.sql': '94f124c3e50405f7b75746885111a4f662c331985a697de46fe946a17ff1f7d9',
    '258_mcp_oauth.sql': '751a1dcee6108afcecbd580b6e3140cca3480853fecd2174861b02f3a71e54a1',
    '259_openai_cache_write_ttl.sql': '532551ce7aee86b2ee0a78d397d422424ceed4051e31d5f542277c80c21b8b98',
    '260_group_force_openai_ultrafast.sql': 'd78e605d4fc2038c57f620dff398022823a850ea8994b129b50774ac49e20af1',
    '261_group_usage_rollup_nonblocking_build.sql': '5b6e9b57f9b354a96c6870185c034db8adc7def944efcddf654e39cd97770718',
    '262_channel_pricing_ultrafast_multiplier.sql': '458752b3523254ee60ab4a821261ab64df9216a6f208c1960e7b816f8c7cfd1d',
}


def validate(version, before, target, folder):
    if before == target:
        return []
    if version != VERSION:
        raise ValueError('new migrations require an explicitly reviewed release')
    missing = set(target) - set(before)
    if not missing or not missing <= SEALED.keys():
        raise ValueError('outside sealed migration inventory')
    if any(target.get(k) != v for k, v in before.items()):
        raise ValueError('historical migrations changed or disappeared')
    if any(target.get(k) != v for k, v in SEALED.items()):
        raise ValueError('release differs from sealed migration checksums')
    files = []
    for name, digest in SEALED.items():
        sql = (folder / name).read_text(encoding='utf8').replace('\r\n', '\n').strip()
        if hashlib.sha256(sql.encode()).hexdigest() != digest:
            raise ValueError('sealed SQL source checksum mismatch')
        if name in missing:
            files.append((name, digest, sql))
    return files


def apply(files, execute, query, app_role):
    """Execute with a host administrator; query returns one scalar string."""
    if not re.fullmatch('[a-z_][a-z0-9_]*', app_role):
        raise ValueError('invalid application role')
    for name, digest, sql in files:
        if name not in SEALED or SEALED[name] != digest or hashlib.sha256(sql.encode()).hexdigest() != digest:
            raise ValueError('unsealed migration')
        record = f"INSERT INTO schema_migrations(filename,checksum) VALUES('{name}','{digest}');"
        if name.endswith('_notx.sql'):
            # Concurrent index DDL cannot share a transaction with its receipt.
            # Inspect invalid remnants before IF NOT EXISTS can hide a failure.
            name_index = 'idx_usage_logs_api_key_created_cost'
            state = query("SELECT coalesce((SELECT indisvalid::text FROM pg_index WHERE indexrelid=to_regclass('public." + name_index + "')),'missing');")
            if state in ('false', 'f'):
                execute('SET lock_timeout=\'5s\'; SET statement_timeout=\'30min\';', 'DROP INDEX CONCURRENTLY IF EXISTS ' + name_index + ';')
            execute("SET lock_timeout='5s'; SET statement_timeout='30min';", sql)
            if query("SELECT indisvalid::text FROM pg_index WHERE indexrelid=to_regclass('public." + name_index + "');") not in ('true', 't'):
                raise ValueError('concurrent index is not valid')
            execute('', record)
        else:
            ownership = ''
            if name == '258_mcp_oauth.sql':
                ownership = '\n'.join(f'ALTER TABLE {table} OWNER TO "{app_role}";' for table in ('mcp_oauth_grants', 'mcp_oauth_codes', 'mcp_oauth_credentials'))
            execute('', "BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s';\n" + sql + '\n' + ownership + '\n' + record + '\nCOMMIT;')
