#!/usr/bin/env python3
"""Reviewed, additive media migration; ordinary deployment guards stay intact.

REHEARSE only backs up and tests a clone. APPLY requires its sealed successful
receipt, fresh final backup, unchanged runtime/config and exact SQL checksum.
Never rewrites old checksums, restores production data, or drops old tables.
"""
import argparse
import contextlib
try:
    import fcntl
except ImportError:  # Windows can run the pure gate tests, never deployment.
    fcntl = None
import hashlib
import json
import os
from pathlib import Path
import sys
import uuid

from controller import PROTECTED, atomic_json, file_hash, run
from manage import Manager, STATE, read

FILENAME = "243_media_tasks_and_pricing.sql"
UNIFIED_FILES = {
    "244_unified_media_model_pricing.sql": "channel_model_pricing",
    "245_unified_media_account_cost.sql": "channel_account_stats_model_pricing",
}

MONITOR_FILE = '246_channel_monitor_imagevideo.sql'
TASK_INDEX_FILE = '247_media_task_list_indexes.sql'
DOCS_FILE = '248_public_imagevideo_docs.sql'
TEMPLATE_SCHEMA_FILE = '249_media_protocol_templates.sql'
LEGACY_TEMPLATE_FILE = '250_migrate_legacy_media_template_refs.sql'
TEMPLATE_FILES = (TEMPLATE_SCHEMA_FILE, LEGACY_TEMPLATE_FILE)
TEMPLATE_UPGRADE = 'media-protocol-templates'
# Exact reviewed statements, after removing line comments and normalizing space.
# Changing the data migration requires reviewing its expected-account projection
# and this seal together; a checksum supplied by a release is not authorization.
LEGACY_TEMPLATE_BODY_SHA256 = '992c628510b7864cb54cdd16bb3fea41702579b46482bb6ea4321c62278a9475'
MONITOR_PROVIDERS = "'openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'imagevideo'"


def normalized_sql(sql):
    import re
    return ' '.join(re.sub(r'--[^\n]*', '', sql).split())


def template_schema_statements():
    return ("ALTER TABLE media_config_versions DROP CONSTRAINT IF EXISTS media_config_versions_kind_check; "
            "ALTER TABLE media_config_versions ADD CONSTRAINT media_config_versions_kind_check "
            "CHECK(kind IN ('capability','price','template','binding')); "
            "ALTER TABLE media_artifacts ADD COLUMN IF NOT EXISTS filename text NOT NULL DEFAULT '';")


def template_transaction(files):
    import re
    if not files or set(files) - set(TEMPLATE_FILES):
        raise ValueError('outside reviewed media template migration inventory')
    statements = []
    for name in TEMPLATE_FILES:
        if name not in files:
            continue
        sql, checksum = files[name]
        body = normalized_sql(sql)
        if not re.fullmatch(r'[0-9a-f]{64}', checksum):
            raise ValueError('invalid media template migration checksum')
        if name == TEMPLATE_SCHEMA_FILE:
            accepted = body == template_schema_statements()
        else:
            accepted = hashlib.sha256(body.encode()).hexdigest() == LEGACY_TEMPLATE_BODY_SHA256
        if not accepted:
            raise ValueError('outside exact reviewed media template statements')
        statements.append(body + f"\nINSERT INTO schema_migrations(filename,checksum) VALUES('{name}','{checksum}');")
    return ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
            'SELECT pg_advisory_xact_lock(694208311321144027);\n' + '\n'.join(statements) + ' COMMIT;')


def template_transaction_files(transaction):
    import re
    prefix = ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
              'SELECT pg_advisory_xact_lock(694208311321144027);\n')
    if not transaction.startswith(prefix) or not transaction.endswith(' COMMIT;'):
        raise ValueError('invalid media template transaction envelope')
    body = transaction[len(prefix):-len(' COMMIT;')]
    files, cursor = {}, 0
    for match in re.finditer(r"\nINSERT INTO schema_migrations\(filename,checksum\) VALUES\('([^']+)','([0-9a-f]{64})'\);", body):
        name, checksum = match.groups()
        if name in files:
            raise ValueError('duplicate media template migration')
        files[name] = (body[cursor:match.start()].lstrip('\n'), checksum)
        cursor = match.end()
    if cursor != len(body) or template_transaction(files) != transaction:
        raise ValueError('unreviewed media template transaction')
    return files


def validate_template_transaction(transaction):
    try:
        template_transaction_files(transaction)
        return True
    except (ValueError, TypeError):
        return False


def legacy_accounts_fingerprint_sql(sql):
    body = normalized_sql(sql)
    if hashlib.sha256(body.encode()).hexdigest() != LEGACY_TEMPLATE_BODY_SHA256:
        raise ValueError('unreviewed legacy account projection')
    ctes = body.split(' UPDATE accounts a SET extra = ', 1)[0]
    # Only the approved media_models transformation can differ. Populate the
    # original row type to keep every account column and its JSON key order.
    return (ctes + " SELECT count(*)::text || '|' || md5(coalesce(string_agg(h,'' ORDER BY h),'')) "
            "FROM (SELECT md5(row_to_json(t)::text) h FROM (SELECT "
            "(jsonb_populate_record(NULL::accounts, to_jsonb(a) || jsonb_build_object('extra', "
            "CASE WHEN migrated.id IS NULL THEN a.extra ELSE "
            "jsonb_set(COALESCE(a.extra, '{}'::jsonb), '{media_models}', migrated.models, true) END))).* "
            "FROM accounts a LEFT JOIN migrated ON a.id = migrated.id) t) fingerprint_rows;")


def reviewed_template_upgrade(before, target, folder):
    pending = set(target) - set(before)
    if not pending or pending - set(TEMPLATE_FILES) or {k: v for k, v in target.items() if k not in pending} != before:
        raise ValueError('migration inventory differs outside reviewed media template upgrade')
    if LEGACY_TEMPLATE_FILE in pending and TEMPLATE_SCHEMA_FILE not in target:
        raise ValueError('template schema migration is required before account migration')
    if TEMPLATE_SCHEMA_FILE in pending and LEGACY_TEMPLATE_FILE in before:
        raise ValueError('invalid template migration order')
    folder, files = Path(folder), {}
    for name in TEMPLATE_FILES:
        if name not in pending:
            continue
        source = folder / name
        if not source.is_file():
            # Older workers stage only the previous allowlist. Reuse a file
            # solely from the checksum-verified assets of this exact release.
            for candidate in sorted((folder.parent.parent / 'automatic').glob('*-assets')):
                manifest_path = candidate / 'deployment.json'
                if not manifest_path.is_file() or not (candidate / name).is_file():
                    continue
                manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
                if manifest.get('version') == folder.name and manifest.get('migrations') == target:
                    source = candidate / name
                    break
        sql = source.read_text(encoding='utf-8').replace('\r\n', '\n').strip()
        checksum = hashlib.sha256(sql.encode()).hexdigest()
        if checksum != target[name]:
            raise ValueError('media template migration checksum differs')
        files[name] = (sql, checksum)
    return {'filename': TEMPLATE_UPGRADE, 'files': list(files),
            'transaction': template_transaction(files), 'expected_migrations': target}


def task_index_statements():
    return ('CREATE INDEX IF NOT EXISTS media_tasks_created_id ON media_tasks(created_at DESC, id DESC);\n'
            'CREATE INDEX IF NOT EXISTS media_tasks_user_created_id ON media_tasks(user_id, created_at DESC, id DESC);')


def task_index_transaction(sql, checksum):
    import re
    normalized = ' '.join(re.sub(r'--[^\n]*', '', sql).split())
    if normalized != ' '.join(task_index_statements().split()) or not re.fullmatch(r'[0-9a-f]{64}', checksum):
        raise ValueError('outside reviewed media task indexes')
    return ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
            "SELECT pg_advisory_xact_lock(694208311321144027);\n" + task_index_statements() +
            f"\nINSERT INTO schema_migrations(filename,checksum) VALUES('{TASK_INDEX_FILE}','{checksum}'); COMMIT;")


def validate_task_index_transaction(transaction):
    import re
    match = re.search(r"VALUES\('247_media_task_list_indexes.sql','([0-9a-f]{64})'\)", transaction)
    return bool(match and transaction == task_index_transaction(task_index_statements(), match[1]))


def docs_statements():
    return ('CREATE TABLE IF NOT EXISTS public_documents (\n'
            'slug TEXT PRIMARY KEY,\n'
            'published_version BIGINT NOT NULL DEFAULT 0 CHECK (published_version >= 0),\n'
            "published_content JSONB NOT NULL DEFAULT '[]'::jsonb,\n"
            'draft_version BIGINT NOT NULL DEFAULT 0 CHECK (draft_version >= 0),\n'
            "draft_content JSONB NOT NULL DEFAULT '[]'::jsonb,\n"
            'updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n'
            'published_at TIMESTAMPTZ\n'
            ');\n'
            'CREATE TABLE IF NOT EXISTS public_document_versions (\n'
            'id BIGSERIAL PRIMARY KEY,\n'
            'slug TEXT NOT NULL REFERENCES public_documents(slug) ON DELETE CASCADE,\n'
            'version BIGINT NOT NULL,\n'
            "action TEXT NOT NULL CHECK (action IN ('initial', 'draft', 'publish', 'rollback')),\n"
            'content JSONB NOT NULL,\n'
            'actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,\n'
            'created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n'
            'UNIQUE (slug, version, action)\n'
            ');\n'
            'CREATE INDEX IF NOT EXISTS public_document_versions_history ON public_document_versions(slug, created_at DESC, id DESC);')


def docs_transaction(sql, checksum):
    import re
    normalized = ' '.join(re.sub(r'--[^\n]*', '', sql).split())
    expected = ' '.join(docs_statements().split())
    if normalized != expected or not re.fullmatch(r'[0-9a-f]{64}', checksum):
        raise ValueError('outside reviewed public documentation migration')
    return ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
            'SELECT pg_advisory_xact_lock(694208311321144027);\n' + docs_statements() +
            f"\nINSERT INTO schema_migrations(filename,checksum) VALUES('{DOCS_FILE}','{checksum}'); COMMIT;")


def validate_docs_transaction(transaction):
    import re
    match = re.search(r"VALUES\('248_public_imagevideo_docs\.sql','([0-9a-f]{64})'\)", transaction)
    return bool(match and transaction == docs_transaction(docs_statements(), match[1]))


def reviewed_docs_upgrade(before, target, folder):
    if before == target:
        return None
    if DOCS_FILE in before or set(target) - set(before) != {DOCS_FILE} or {k: v for k, v in target.items() if k != DOCS_FILE} != before:
        raise ValueError('migration inventory differs outside reviewed public documentation migration')
    folder = Path(folder)
    source = folder / DOCS_FILE
    if not source.exists():
        for candidate in sorted((folder.parent.parent / 'automatic').glob('*-assets')):
            manifest_path = candidate / 'deployment.json'
            if not manifest_path.is_file() or not (candidate / DOCS_FILE).is_file():
                continue
            manifest = json.loads(manifest_path.read_text())
            if manifest.get('version') == folder.name and manifest.get('migrations') == target:
                source = candidate / DOCS_FILE
                break
    sql = source.read_text().replace('\r\n', '\n').strip()
    checksum = hashlib.sha256(sql.encode()).hexdigest()
    if target[DOCS_FILE] != checksum:
        raise ValueError('public documentation migration checksum differs')
    return {'filename': DOCS_FILE, 'transaction': docs_transaction(sql, checksum), 'expected_migrations': target}


def reviewed_upgrade(before, target, folder):
    if before == target:
        return None
    if (set(target) - set(before)) & set(TEMPLATE_FILES):
        return reviewed_template_upgrade(before, target, folder)
    if set(target) - set(before) != {TASK_INDEX_FILE}:
        if set(target) - set(before) == {MONITOR_FILE}:
            return reviewed_monitor_upgrade(before, target, folder)
        return reviewed_docs_upgrade(before, target, folder)
    if {k: v for k, v in target.items() if k != TASK_INDEX_FILE} != before:
        raise ValueError('migration inventory differs outside reviewed task indexes')
    folder = Path(folder)
    source = folder / TASK_INDEX_FILE
    # Older installed workers download checksummed SQL before installing the new
    # manager, but only stage the monitor asset. Require the exact release here.
    if not source.exists():
        for candidate in sorted((folder.parent.parent / 'automatic').glob('*-assets')):
            manifest_path = candidate / 'deployment.json'
            if not manifest_path.is_file() or not (candidate / TASK_INDEX_FILE).is_file():
                continue
            manifest = json.loads(manifest_path.read_text())
            if manifest.get('version') == folder.name and manifest.get('migrations') == target:
                source = candidate / TASK_INDEX_FILE
                break
    sql = source.read_text().replace('\r\n', '\n').strip()
    checksum = hashlib.sha256(sql.encode()).hexdigest()
    if target[TASK_INDEX_FILE] != checksum:
        raise ValueError('task index migration checksum differs')
    return {'filename': TASK_INDEX_FILE, 'transaction': task_index_transaction(sql, checksum), 'expected_migrations': target}


def monitor_statements():
    statements = []
    for table in ('channel_monitors', 'channel_monitor_request_templates'):
        statements += [f'ALTER TABLE {table} DROP CONSTRAINT IF EXISTS {table}_provider_check;',
                       f'ALTER TABLE {table} ADD CONSTRAINT {table}_provider_check CHECK (provider IN ({MONITOR_PROVIDERS}));']
    table = 'channel_monitor_histories'
    statements += [f'ALTER TABLE {table} DROP CONSTRAINT IF EXISTS {table}_status_check;',
                   f"ALTER TABLE {table} ADD CONSTRAINT {table}_status_check CHECK (status IN ('operational', 'degraded', 'failed', 'error', 'pending'));"]
    return '\n'.join(statements)


def monitor_transaction(sql, checksum):
    """Exact enum expansion only. No table drops, row writes or generic DDL."""
    import re
    normalized = ' '.join(re.sub(r'--[^\n]*', '', sql).split())
    if normalized != ' '.join(monitor_statements().split()) or not re.fullmatch(r'[0-9a-f]{64}', checksum):
        raise ValueError('outside reviewed media monitor enum expansion')
    return ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
            "SELECT pg_advisory_xact_lock(694208311321144027);\n" + monitor_statements() +
            f"\nINSERT INTO schema_migrations(filename,checksum) VALUES('{MONITOR_FILE}','{checksum}'); COMMIT;")


def validate_monitor_transaction(transaction):
    import re
    match = re.search(r"VALUES\('246_channel_monitor_imagevideo.sql','([0-9a-f]{64})'\)", transaction)
    return bool(match and transaction == monitor_transaction(monitor_statements(), match[1]))


def reviewed_monitor_upgrade(before, target, folder):
    if before == target:
        return None
    if MONITOR_FILE in before or set(target) - set(before) != {MONITOR_FILE} or {k: v for k, v in target.items() if k != MONITOR_FILE} != before:
        raise ValueError('migration inventory differs outside reviewed monitor expansion')
    folder = Path(folder)
    source = folder / MONITOR_FILE
    # A previous release's deployment worker installs the new manager but did
    # not stage this new asset. Reuse only its already downloaded immutable
    # release inventory; SQL still must match the target checksum and exact DDL.
    if not source.exists():
        for candidate in sorted((folder.parent.parent / 'automatic').glob('*-assets')):
            manifest_path = candidate / 'deployment.json'
            if not manifest_path.is_file() or not (candidate / MONITOR_FILE).is_file():
                continue
            manifest = json.loads(manifest_path.read_text())
            if manifest.get('version') == folder.name and manifest.get('migrations') == target:
                source = candidate / MONITOR_FILE
                break
    sql = source.read_text().replace('\r\n', '\n').strip()
    checksum = hashlib.sha256(sql.encode()).hexdigest()
    if target[MONITOR_FILE] != checksum:
        raise ValueError('monitor migration checksum differs')
    return {'filename': MONITOR_FILE, 'transaction': monitor_transaction(sql, checksum), 'expected_migrations': target}


def unified_transaction(files):
    """Only the two reviewed nullable JSONB additions; no generic DDL path."""
    import re
    if set(files) != set(UNIFIED_FILES):
        raise ValueError("unexpected unified pricing migration inventory")
    statements = []
    for filename, table in UNIFIED_FILES.items():
        sql, checksum = files[filename]
        expected = "ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS media JSONB;"
        normalized = " ".join(" ".join(line.split()) for line in sql.splitlines() if not line.lstrip().startswith("--"))
        if normalized != expected or not re.fullmatch(r"[0-9a-f]{64}", checksum):
            raise ValueError("outside reviewed unified pricing allowlist")
        statements.append(expected + "\nINSERT INTO schema_migrations(filename,checksum) VALUES('" + filename + "','" + checksum + "');")
    return ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
            "SELECT pg_advisory_xact_lock(694208311321144027);\n" + "\n".join(statements) + " COMMIT;")


def validate_unified_transaction(transaction):
    import re
    checksums = dict(re.findall(r"INSERT INTO schema_migrations\(filename,checksum\) VALUES\('([^']+)','([0-9a-f]{64})'\);", transaction))
    if set(checksums) != set(UNIFIED_FILES):
        return False
    files = {name: ("ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS media JSONB;", checksums[name]) for name, table in UNIFIED_FILES.items()}
    return transaction == unified_transaction(files)


@contextlib.contextmanager
def tracked_operation(manager, action, version):
    job = {"id": uuid.uuid4().hex, "action": "media_migration_" +
           ("rehearsal" if action == "rehearse" else "apply"), "target": version}
    cgroup = Path('/proc/self/cgroup')
    if cgroup.exists():
        for line in cgroup.read_text().splitlines():
            unit = line.rsplit('/', 1)[-1]
            if unit.startswith('sub2api-') and unit.endswith('.service'):
                job['worker_unit'] = unit
                break
    manager.save(job, "queued")
    print("JOB_ID=" + job["id"], flush=True)
    try:
        yield job
    except BaseException as error:
        job["failed_phase"] = job.get("status", "queued")
        job["error_type"] = type(error).__name__
        # Save only structured diagnostics; exception text may include secrets.
        manager.save(job, "failed_needs_inspection")
        raise


def reviewed_transaction(sql, checksum):
    # This module is intentionally an allowlist, not a generic migration runner.
    statements = sql.replace("\r\n", "\n")
    import re
    clean = re.sub(r"--[^\n]*", "", statements)
    clean = re.sub(r"\bON\s+DELETE\s+RESTRICT\b", "", clean, flags=re.I)
    if re.search(r"\b(DROP|DELETE|UPDATE|TRUNCATE|COPY|GRANT|REVOKE)\b", clean, re.I):
        raise ValueError("migration is not additive")
    if "ALTER TABLE user_platform_quotas" not in statements or "CREATE TABLE media_tasks" not in statements:
        raise ValueError("not the reviewed media schema")
    for statement in clean.split(";"):
        statement = statement.strip()
        if statement and not re.match(r"(?:ALTER TABLE user_platform_quotas\s+ADD COLUMN media_|CREATE TABLE media_[a-z_]+\s*\(|CREATE (?:UNIQUE )?INDEX media_[a-z_]+ ON media_[a-z_]+\s*\()", statement, re.I):
            raise ValueError("statement outside reviewed media allowlist")
    if not re.fullmatch(r"[0-9a-f]{64}", checksum):
        raise ValueError("invalid migration checksum")
    return ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
            "SELECT pg_advisory_xact_lock(694208311321144027);\n" + statements +
            "\nINSERT INTO schema_migrations(filename,checksum) VALUES('" + FILENAME + "','" + checksum + "'); COMMIT;")


def validate_legacy_transaction(transaction):
    import re
    prefix = ("BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='60s'; "
              'SELECT pg_advisory_xact_lock(694208311321144027);\n')
    match = re.fullmatch(re.escape(prefix) + r"([\s\S]*)\nINSERT INTO schema_migrations\(filename,checksum\) VALUES\('243_media_tasks_and_pricing\.sql','([0-9a-f]{64})'\); COMMIT;", transaction)
    if not match:
        return False
    try:
        return reviewed_transaction(*match.groups()) == transaction
    except ValueError:
        return False


def execute(args):
    if fcntl is None or os.geteuid() != 0 or args.confirm != args.action.upper():
        raise ValueError("root and explicit operation confirmation required")
    os.umask(0o077)
    m = Manager()
    with tracked_operation(m, args.action, args.version) as job, (STATE / "operations.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        original = m.preflight()
        image, manifest = m.offline(args.version)
        old = m.c.migrations()
        target = manifest["migrations"]
        unified = getattr(args, "schema", "tasks") == "unified-pricing"
        filenames = list(UNIFIED_FILES) if unified else [FILENAME]
        files = {}
        for filename in filenames:
            raw = (STATE / "releases" / args.version / filename).read_bytes()
            normalized = raw.decode("utf-8").replace("\r\n", "\n").strip()
            checksum = hashlib.sha256(normalized.encode()).hexdigest()
            if target.get(filename) != checksum:
                raise ValueError("reviewed migration checksum differs")
            files[filename] = normalized, checksum
        if {k:v for k,v in target.items() if k not in filenames} != {k:v for k,v in old.items() if k not in filenames}:
            raise ValueError("only exact reviewed additive migrations are allowed")
        digest = hashlib.sha256(json.dumps(files,sort_keys=True).encode()).hexdigest() if unified else files[FILENAME][1]
        transaction = unified_transaction(files) if unified else reviewed_transaction(*files[FILENAME])
        receipt_path = STATE / "releases" / args.version / "media-migration-rehearsal.json"
        if args.action == "rehearse":
            if any(filename in old for filename in filenames):
                raise ValueError("already migrated; use ordinary prepare")
            folder, meta = m.backup(job)
            m.save(job, "restoring_and_testing")
            test = {"id":uuid.uuid4().hex,"image":image,"rollback_image":meta["runtime_image"],
                    "version":args.version,"manifest":{"migrations":old},
                    "reviewed_media_migration":{"filename":"unified-media-pricing" if unified else FILENAME,"transaction":transaction,"expected_migrations":target}}
            m.c.rehearse(test,folder)
            (folder / "restored-fingerprints.json").unlink(missing_ok=True)
            if not all(test.get(k) for k in ("restore_verified","rollback_verified","additive_data_verified")):
                raise ValueError("incomplete rehearsal evidence")
            receipt={"version":args.version,"image":image,"checksum":digest,"before":old,"after":target,
                     "backup_id":meta["id"],"config_hashes":meta["compose_hashes"],"binary_sha256":meta["binary_sha256"],
                     "evidence":{k:test[k] for k in ("restore_verified","rollback_verified","additive_data_verified")}}
            atomic_json(receipt_path,receipt)
            atomic_json(receipt_path.with_suffix(".seal.json"),{"sha256":file_hash(receipt_path)})
            m.save(job,"succeeded")
            print(json.dumps({"rehearsed":True,"backup_id":meta["id"],"receipt":str(receipt_path)}))
            return
        receipt=read(receipt_path)
        if file_hash(receipt_path) != read(receipt_path.with_suffix(".seal.json"))["sha256"] or receipt["image"] != image or receipt["checksum"] != digest or receipt["before"] != old or receipt["after"] != target:
            raise ValueError("migration rehearsal receipt differs from current target")
        if m.c.config_hashes()!=receipt["config_hashes"] or run(["docker","exec","sub2api","sha256sum","/app/sub2api"]).decode().split()[0]!=receipt["binary_sha256"]:
            raise ValueError("production changed since rehearsal")
        m.verified_backup(receipt["backup_id"])
        folder,meta=m.backup(job)
        if m.c.inspect()["Id"]!=original["Id"] or m.c.migrations()!=old:
            raise ValueError("production changed during final backup")
        try:
            m.save(job,"draining")
            m.drain(job)
            m.save(job,"applying_reviewed_additive_migration")
            m.c.apply_reviewed_media_migration(transaction)
            if m.c.migrations()!=target:
                raise ValueError("migration inventory mismatch; inspect, do not overwrite data")
            m.save(job,"succeeded")
        finally:
            m.marker.unlink(missing_ok=True)
        print(json.dumps({"applied":True,"backup_id":meta["id"],"ordinary_prepare_required":True}))


def main():
    p=argparse.ArgumentParser()
    p.add_argument("action",choices=["rehearse","apply"])
    p.add_argument("version")
    p.add_argument("--confirm",required=True)
    p.add_argument("--schema",choices=["tasks", "unified-pricing"],default="tasks")
    args=p.parse_args()
    try: execute(args)
    except Exception as error:
        # Root-only logs can contain full diagnostics; the public output cannot
        # include SQL environments or provider credentials.
        print("Media migration gate failed: "+type(error).__name__,file=sys.stderr)
        raise


if __name__=="__main__":main()
