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
        path = STATE / "releases" / args.version / FILENAME
        raw = path.read_bytes()
        normalized = raw.decode("utf-8").replace("\r\n", "\n").strip()
        # Match release_manifest.py and the Go migration runner's trimmed SQL.
        digest = hashlib.sha256(normalized.encode()).hexdigest()
        if target.get(FILENAME) != digest or {k:v for k,v in target.items() if k != FILENAME} != {k:v for k,v in old.items() if k != FILENAME}:
            raise ValueError("only the exact reviewed additive migration is allowed")
        transaction = reviewed_transaction(normalized, digest)
        receipt_path = STATE / "releases" / args.version / "media-migration-rehearsal.json"
        if args.action == "rehearse":
            if FILENAME in old:
                raise ValueError("already migrated; use ordinary prepare")
            folder, meta = m.backup(job)
            m.save(job, "restoring_and_testing")
            test = {"id":uuid.uuid4().hex,"image":image,"rollback_image":meta["runtime_image"],
                    "version":args.version,"manifest":{"migrations":old},
                    "reviewed_media_migration":{"filename":FILENAME,"transaction":transaction,"expected_migrations":target}}
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
    args=p.parse_args()
    try: execute(args)
    except Exception as error:
        # Root-only logs can contain full diagnostics; the public output cannot
        # include SQL environments or provider credentials.
        print("Media migration gate failed: "+type(error).__name__,file=sys.stderr)
        raise


if __name__=="__main__":main()
