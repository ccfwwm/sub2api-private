#!/usr/bin/env python3
"""Opt-in, two-phase Sub2API image deployment. Never installed by release CI.

Only root-owned configuration can enable mutations. A preparation job performs
backup/restore/smoke testing; a separate explicit request switches production.
"""
from __future__ import annotations

import argparse
import contextlib
import datetime as dt
import hashlib
import http.client
import http.server
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import socketserver
import subprocess
import threading
import time
import urllib.request
import urllib.parse
import uuid

REPO = "ccfwwm/sub2api-private"
IMAGE = "ghcr.io/ccfwwm/sub2api-private"
DOCS_IMAGE = "ghcr.io/ccfwwm/sub2api-private-docs"
CONFIG = Path("/etc/sub2api-deployer/config.json")
SOCKET = "/run/sub2api-deployer/controller.sock"
VERSION = re.compile(r"v(0|[1-9][0-9]*)\.[0-9]+\.[0-9]+\Z")
JOB = re.compile(r"[0-9a-f]{32}\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
PROTECTED = ("users", "accounts", "api_keys", "groups", "settings", "user_subscriptions",
             "payment_orders", "channel_model_pricing", "channel_account_stats_model_pricing", "security_secrets")
COMMAND_ERROR_LOG = None


def file_hash(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def atomic_json(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    tmp = path.with_suffix(".tmp")
    with tmp.open("w", encoding="utf-8") as f:
        os.chmod(tmp, 0o600)
        json.dump(data, f, indent=2)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, path)


def run(args, *, stdin=None, stdout=None, timeout=1800):
    # Command output can contain environment secrets; never include it in errors.
    result = subprocess.run(args, input=stdin, stdout=stdout or subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout, check=False)
    if result.returncode:
        if COMMAND_ERROR_LOG:
            # Diagnostics stay on the host in a root-only file; never return raw
            # command output to the browser or GitHub SSH client.
            with open(COMMAND_ERROR_LOG, "ab") as log:
                os.chmod(COMMAND_ERROR_LOG, 0o600)
                log.write((now() + " " + args[0] + " failed\n").encode())
                log.write(result.stderr[-65536:])
        raise RuntimeError(f"{args[0]} failed (exit {result.returncode}); inspect restricted host logs")
    return result.stdout


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, url):
        new = super().redirect_request(req, fp, code, msg, headers, url)
        parsed = urllib.parse.urlsplit(url)
        if parsed.scheme != "https" or parsed.hostname not in (
                "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"):
            raise RuntimeError("untrusted GitHub asset redirect")
        if parsed.hostname != "api.github.com":
            new.remove_header("Authorization")
        return new


def github(path, token, binary=False):
    if not path.startswith(f"repos/{REPO}/"):
        raise ValueError("repository not allowed")
    req = urllib.request.Request("https://api.github.com/" + path, headers={
        "Authorization": "Bearer " + token,
        "Accept": "application/octet-stream" if binary else "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "sub2api-private-deployer"})
    with urllib.request.build_opener(SafeRedirect()).open(req, timeout=30) as response:
        data = response.read(2 * 1024 * 1024 + 1)
    if len(data) > 2 * 1024 * 1024:
        raise ValueError("release metadata too large")
    return json.loads(data)


def validate_manifest(manifest, version):
    if not VERSION.fullmatch(version):
        raise ValueError("expected private release vMAJOR.MINOR.PATCH")
    if (manifest.get("repository") != REPO or manifest.get("image") != IMAGE or
            manifest.get("version") != version or manifest.get("platform") != "linux/amd64" or
            not DIGEST.fullmatch(manifest.get("digest", "")) or
            not re.fullmatch(r"[0-9a-f]{40}", manifest.get("commit", ""))):
        raise ValueError("release manifest identity mismatch")
    migrations = manifest.get("migrations")
    if not isinstance(migrations, dict) or not migrations:
        raise ValueError("missing migration inventory")
    for name, digest in migrations.items():
        if not re.fullmatch(r"[A-Za-z0-9_]+\.sql", name) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError("invalid migration inventory")
    # v1.2.7 introduced the independently replaceable documentation image.
    # Keep older release manifests readable so historical rollback remains
    # possible, while making the new release fail closed if either identity is
    # missing or malformed.
    if tuple(map(int, version[1:].split('.'))) >= (1, 2, 7):
        if manifest.get("docs_image") != DOCS_IMAGE or not DIGEST.fullmatch(manifest.get("docs_digest", "")):
            raise ValueError("documentation image identity missing")
    elif manifest.get("docs_image") is not None or manifest.get("docs_digest") is not None:
        if manifest.get("docs_image") != DOCS_IMAGE or not DIGEST.fullmatch(manifest.get("docs_digest", "")):
            raise ValueError("invalid documentation image identity")
    return f"{IMAGE}@{manifest['digest']}"


def manifest_images(manifest, version):
    """Return the immutable application and optional docs image references."""
    application = validate_manifest(manifest, version)
    docs = None
    if manifest.get("docs_digest"):
        docs = f"{manifest.get('docs_image', DOCS_IMAGE)}@{manifest['docs_digest']}"
    return application, docs


def validate_request(request):
    if set(request) - {"action", "version", "job_id", "confirm"}:
        raise ValueError("unknown deployment fields")
    action = request.get("action")
    if action not in ("prepare", "deploy", "rollback") or request.get("confirm") is not True:
        raise ValueError("explicit action confirmation is required")
    if action == "prepare":
        if not isinstance(request.get("version"), str) or not VERSION.fullmatch(request["version"]):
            raise ValueError("invalid release version")
    elif not isinstance(request.get("job_id"), str) or not JOB.fullmatch(request["job_id"]):
        raise ValueError("invalid preparation job")
    return action


class Controller:
    def __init__(self, config):
        global COMMAND_ERROR_LOG
        self.config = config
        self.state = Path(config["state_dir"])
        self.root = Path(config["production_dir"])
        self.state.mkdir(parents=True, exist_ok=True, mode=0o700)
        COMMAND_ERROR_LOG = self.state / "command-errors.log"
        self.guard = threading.Lock()

    def compose(self, override=None):
        args = ["docker", "compose", "--project-name", "sub2api-deploy", "--project-directory", str(self.root),
                "--env-file", str(self.root / ".env")]
        for name in ("docker-compose.yml", "docker-compose.override.yml"):
            path = self.root / name
            if path.exists():
                args += ["-f", str(path)]
        if override:
            args += ["-f", str(override)]
        return args

    def save(self, job, status=None):
        if status:
            job["status"] = status
        job["updated_at"] = now()
        atomic_json(self.state / "jobs" / (job["id"] + ".json"), job)
        atomic_json(self.state / "latest.json", {"id": job["id"]})

    def load(self, job_id):
        if not JOB.fullmatch(job_id):
            raise ValueError("invalid job")
        return json.loads((self.state / "jobs" / (job_id + ".json")).read_text())

    def status(self):
        path = self.state / "latest.json"
        job = self.load(json.loads(path.read_text())["id"]) if path.exists() else None
        return {"enabled": self.config.get("enabled") is True, "repository": REPO,
                "image": IMAGE, "job": job}

    def submit(self, request):
        action = validate_request(request)
        if self.config.get("enabled") is not True:
            raise ValueError("production deployment is disabled by the operator")
        if not self.guard.acquire(blocking=False):
            raise ValueError("another deployment operation is running")
        lock = None
        try:
            import fcntl
            lock = (self.state / "deployment.lock").open("a")
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            if action == "prepare":
                job = {"id": uuid.uuid4().hex, "version": request["version"], "created_at": now()}
            else:
                job = self.load(request["job_id"])
                allowed = ("ready",) if action == "deploy" else ("succeeded",)
                if job["status"] not in allowed:
                    raise ValueError("job is not eligible for this operation")
                if action == "deploy" and time.time() - job["prepared_epoch"] > 3600:
                    raise ValueError("preparation expired; prepare a fresh backup first")
            self.save(job, "queued_" + action)
            thread = threading.Thread(target=self.worker, args=(action, job, lock), daemon=False)
            thread.start()
            return {"job": job}
        except Exception:
            if lock:
                lock.close()
            self.guard.release()
            raise

    def worker(self, action, job, lock):
        try:
            getattr(self, action)(job)
        except Exception as exc:
            job["error"] = str(exc) if isinstance(exc, ValueError) else "operation failed; inspect the restricted host journal"
            self.save(job, "failed")
            # Only exception type, never subprocess output/credentials.
            print(f"job {job['id']} failed: {type(exc).__name__}", flush=True)
        finally:
            lock.close()
            self.guard.release()

    def manifest(self, version):
        token = Path(self.config["github_token_file"]).read_text().strip()
        release = github(f"repos/{REPO}/releases/tags/{version}", token)
        if release.get("draft") or release.get("prerelease") or release.get("tag_name") != version:
            raise ValueError("only published stable releases may be deployed")
        assets = [a for a in release.get("assets", []) if a["name"] == "deployment.json"]
        if len(assets) != 1:
            raise ValueError("release has no unique deployment manifest")
        manifest = github(f"repos/{REPO}/releases/assets/{int(assets[0]['id'])}", token, binary=True)
        validate_manifest(manifest, version)
        return manifest

    def inspect(self):
        return json.loads(run(["docker", "inspect", "sub2api"]))[0]

    def sql(self, query, container=None):
        container = container or self.config.get("database_container", "sub2api-postgres")
        return run(["docker", "exec", "-i", container, "sh", "-c",
                    'PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=30000 -c timezone=UTC" '
                    'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],
                   stdin=query.encode()).decode().strip()

    def migrations(self, container=None):
        rows = self.sql("SELECT filename || '|' || checksum FROM schema_migrations ORDER BY filename;", container)
        return dict(line.split("|") for line in rows.splitlines())

    def apply_reviewed_media_migration(self, transaction, container=None):
        # Kept separate from the read-only SQL helper and ordinary prepare.
        from media_migrate import (validate_unified_transaction, validate_monitor_transaction,
                                   validate_task_index_transaction, validate_docs_transaction)
        legacy = "243_media_tasks_and_pricing.sql" in transaction
        reviewed = (legacy or validate_unified_transaction(transaction) or
                    validate_monitor_transaction(transaction) or
                    validate_task_index_transaction(transaction) or
                    validate_docs_transaction(transaction))
        if not transaction.startswith("BEGIN; SET LOCAL lock_timeout='3s';") or not transaction.endswith("COMMIT;") or not reviewed:
            raise ValueError("reviewed atomic media transaction required")
        container = container or self.config.get("database_container", "sub2api-postgres")
        run(["docker", "exec", "-i", container, "sh", "-c",
             'PGOPTIONS="-c default_transaction_read_only=off -c statement_timeout=60000 -c timezone=UTC" '
             'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'], stdin=transaction.encode())

    def assert_migrations(self, manifest):
        # Deliberately reject even additive migrations in the ordinary update path.
        # A separate reviewed migration procedure is needed when this changes.
        if self.migrations() != manifest["migrations"]:
            raise ValueError("database migrations differ; production is unchanged; separate migration review required")

    def config_hashes(self):
        return {name: hashlib.sha256((self.root / name).read_bytes()).hexdigest()
                for name in (".env", "docker-compose.yml", "docker-compose.override.yml")
                if (self.root / name).exists()}

    def prepare(self, job):
        self.save(job, "validating")
        credential = Path(self.config.get("release_token_file", "/etc/sub2api-deployer/release-token"))
        if not credential.is_file():
            raise ValueError("application release credential file is not installed")
        mode = credential.stat()
        if mode.st_uid != 0 or mode.st_gid != self.config.get("socket_gid", 1000) or not mode.st_mode & 0o040 or mode.st_mode & 0o007:
            raise ValueError("release credential must be root-owned, app-group-readable and not world-readable")
        manifest = self.manifest(job["version"])
        image, docs_image = manifest_images(manifest, job["version"])
        self.assert_migrations(manifest)
        original = self.inspect()
        if docs_image and subprocess.run(["docker", "inspect", "sub2api-docs-web"], capture_output=True).returncode == 0:
            docs_original = json.loads(run(["docker", "inspect", "sub2api-docs-web"]))[0]
            job["rollback_docs_image"] = docs_original["Config"].get("Image", docs_original["Image"])
        job.update(image=image, manifest=manifest, original_container=original["Id"],
                   original_started=original["State"]["StartedAt"], config_hashes=self.config_hashes())
        run(["docker", "pull", image])
        labels = json.loads(run(["docker", "image", "inspect", image]))[0]["Config"].get("Labels", {})
        if labels.get("org.opencontainers.image.revision") != manifest["commit"]:
            raise ValueError("image revision differs from release manifest")
        if docs_image:
            run(["docker", "pull", docs_image])
            docs_labels = json.loads(run(["docker", "image", "inspect", docs_image]))[0]["Config"].get("Labels", {})
            if docs_labels.get("org.opencontainers.image.revision") != manifest["commit"]:
                raise ValueError("documentation image revision differs from release manifest")
            job["docs_image"] = docs_image
        self.save(job, "backing_up")
        backup = self.state / "backups" / job["id"]
        backup.mkdir(parents=True, mode=0o700)
        # This captures changed runtime binaries without pausing production. Config
        # and secrets remain root-only locally; this image is NEVER pushed.
        rollback_tag = "sub2api-local-rollback:" + job["id"]
        run(["docker", "commit", "--pause=false", "sub2api", rollback_tag])
            # Keep the local rollback tag as the Compose image reference.  The
            # inspect Image ID (sha256:...) is useful for verification but is
            # not a portable Compose image reference.
        job["rollback_image"] = rollback_tag
        job["original_binary_sha256"] = run(["docker", "exec", "sub2api", "sha256sum", "/app/sub2api"]).decode().split()[0]
        captured = run(["docker", "run", "--rm", "--network", "none", "--entrypoint", "sha256sum",
                        job["rollback_image"], "/app/sub2api"]).decode().split()[0]
        if captured != job["original_binary_sha256"]:
            raise ValueError("runtime changed during rollback capture")
        self.backup_database(backup)
        # Archive application files and configuration only; never copy live PGDATA.
        run(["tar", "-czf", str(backup / "application.tar.gz"), "-C", str(self.root),
             ".env", "docker-compose.yml", "docker-compose.override.yml", "data", "pricing-overlay"])
        # Fetch a consistent Redis RDB through replication (Redis may fork a
        # snapshot worker). No service restart; the backup stays on this host.
        with (backup / "redis.rdb").open("wb") as f:
            run(["docker", "exec", "sub2api-redis", "sh", "-c",
                 'redis-cli --rdb /tmp/sub2api-deployment.rdb >/dev/null 2>&1 && '
                 'cat /tmp/sub2api-deployment.rdb && rm /tmp/sub2api-deployment.rdb'], stdout=f)
        self.save(job, "restoring_and_testing")
        self.rehearse(job, backup)
        checksums = {p.name: file_hash(p)
                     for p in backup.iterdir() if p.is_file()}
        atomic_json(backup / "checksums.json", checksums)
        job["prepared_epoch"] = time.time()
        self.save(job, "ready")

    def backup_database(self, backup):
        # Hold one exported MVCC snapshot for both the dump and protected-table
        # fingerprints. Concurrent production writes do not invalidate comparison.
        container = self.config.get("database_container", "sub2api-postgres")
        process = subprocess.Popen(["docker", "exec", "-i", container, "sh", "-c",
                    'PGOPTIONS="-c statement_timeout=30000 -c idle_in_transaction_session_timeout=3600000" '
                    'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],
                    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            process.stdin.write("BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL TIME ZONE 'UTC'; SELECT pg_export_snapshot();\n")
            process.stdin.flush()
            snapshot = process.stdout.readline().strip()
            if not re.fullmatch(r"[0-9A-Fa-f-]+", snapshot):
                raise RuntimeError("cannot export backup snapshot")
            expected = {}
            for table in PROTECTED:
                process.stdin.write(self.fingerprint_sql(table) + "\n")
                process.stdin.flush()
                fingerprint = process.stdout.readline().strip()
                if not re.fullmatch(r"\d+\|[0-9a-f]{32}", fingerprint):
                    raise RuntimeError("cannot fingerprint protected table")
                expected[table] = fingerprint
            with (backup / "database.dump").open("wb") as f:
                run(["docker", "exec", container, "sh", "-c",
                     'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc '
                     '--exclude-table-data=public.ops_system_logs --snapshot="$1"', "sh", snapshot], stdout=f)
            atomic_json(backup / "protected-snapshot.json", expected)
        finally:
            with contextlib.suppress(BrokenPipeError):
                try:
                    process.communicate("ROLLBACK;\n", timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.communicate()
            if process.poll() is None:
                process.kill()
                process.wait()
        with (backup / "globals.sql").open("wb") as f:
            run(["docker", "exec", container, "sh", "-c",
                 'pg_dumpall -U "$POSTGRES_USER" --globals-only'], stdout=f)

    @staticmethod
    def fingerprint_sql(table):
        if table not in PROTECTED + ('media_tasks', 'media_ledger_entries', 'media_cost_entries', 'media_artifacts'):
            raise ValueError("table not allowed")
        return (f'SELECT count(*)::text || \'|\' || md5(coalesce(string_agg(h, \'\' ORDER BY h), \'\')) '
                f'FROM (SELECT md5(row_to_json(t)::text) h FROM "{table}" t) fingerprint_rows;')

    def rehearse(self, job, backup):
        prefix = "sub2api-rehearsal-" + job["id"][:12]
        network = prefix + "-net"
        pg, redis, app = prefix + "-pg", prefix + "-redis", prefix + "-app"
        password = secrets.token_hex(24)
        rehearsal = self.state / "rehearsals" / job["id"]
        rehearsal.mkdir(parents=True, mode=0o700)
        # An internal network has no default external route. No production networks,
        # ports, bind mounts or credentials are passed into the cloned application.
        run(["docker", "network", "create", "--internal", network])
        try:
            run(["docker", "run", "-d", "--name", pg, "--network", network, "--network-alias", "postgres",
                 "--cpus=1", "--memory=2g", "-v", str(rehearsal / "postgres") + ":/var/lib/postgresql",
                 "-e", "POSTGRES_USER=sub2api", "-e", "POSTGRES_DB=sub2api",
                 "-e", "POSTGRES_PASSWORD=" + password, "postgres:18-alpine"])
            self.wait_command(["docker", "exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "sub2api", "-d", "sub2api"], 90)
            with (backup / "database.dump").open("rb") as f:
                proc = subprocess.run(["docker", "exec", "-i", pg, "pg_restore", "-U", "sub2api", "-d", "sub2api",
                                       "--exit-on-error", "--no-owner", "--no-acl"], stdin=f,
                                      stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, timeout=3600)
                if proc.returncode:
                    raise RuntimeError("isolated database restore failed")
            if self.migrations(pg) != job["manifest"]["migrations"]:
                raise ValueError("restored migrations differ")
            # Record protected-table counts before the app starts; the dump itself
            # is a PostgreSQL consistent snapshot, so live concurrent counts need
            # not equal restored counts.
            restored = {t: self.sql(self.fingerprint_sql(t), pg) for t in PROTECTED}
            expected = json.loads((backup / "protected-snapshot.json").read_text())
            if restored != expected:
                raise ValueError("restored protected-table fingerprints do not match backup snapshot")
            atomic_json(backup / "restored-fingerprints.json", restored)
            # Only the separately reviewed media migration rehearsal supplies
            # this field. Ordinary prepare continues to require exact schemas.
            reviewed = job.get("reviewed_media_migration")
            if reviewed:
                if reviewed["filename"] not in ("243_media_tasks_and_pricing.sql", "unified-media-pricing", "246_channel_monitor_imagevideo.sql", "247_media_task_list_indexes.sql", "248_public_imagevideo_docs.sql"):
                    raise ValueError("unreviewed migration")
                before_columns = self.sql("SELECT string_agg(quote_ident(column_name),',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='public' AND table_name='user_platform_quotas';", pg)
                quota_sql = "SELECT count(*)::text || '|' || md5(coalesce(string_agg(md5(row_to_json(t)::text),'' ORDER BY id),'')) FROM (SELECT " + before_columns + " FROM user_platform_quotas ORDER BY id) t;"
                quota_before = self.sql(quota_sql, pg)
                # Compare the original column projection when adding nullable
                # pricing fields; row_to_json(*) necessarily changes shape.
                comparison = {t: self.fingerprint_sql(t) for t in PROTECTED}
                monitor_before = {}
                if reviewed['filename'] == '246_channel_monitor_imagevideo.sql':
                    for table in ('channel_monitors', 'channel_monitor_request_templates', 'channel_monitor_histories'):
                        query = "SELECT count(*)::text || '|' || md5(coalesce(string_agg(md5(row_to_json(t)::text),'' ORDER BY id),'')) FROM " + table + " t;"
                        monitor_before[query] = self.sql(query, pg)
                if reviewed['filename'] == '247_media_task_list_indexes.sql':
                    for table in ('media_tasks', 'media_ledger_entries', 'media_cost_entries', 'media_artifacts'):
                        query = self.fingerprint_sql(table)
                        monitor_before[query] = self.sql(query, pg)
                if reviewed["filename"] == "unified-media-pricing":
                    for table in ("channel_model_pricing", "channel_account_stats_model_pricing"):
                        columns = self.sql("SELECT string_agg(quote_ident(column_name),',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='public' AND table_name='" + table + "';", pg)
                        comparison[table] = "SELECT count(*)::text || '|' || md5(coalesce(string_agg(h,'' ORDER BY h),'')) FROM (SELECT md5(row_to_json(t)::text) h FROM (SELECT " + columns + " FROM " + table + ") t) fingerprint_rows;"
                self.apply_reviewed_media_migration(reviewed["transaction"], pg)
                if self.migrations(pg) != reviewed["expected_migrations"]:
                    raise ValueError("reviewed migration inventory mismatch")
                if {t: self.sql(comparison[t], pg) for t in PROTECTED} != restored or self.sql(quota_sql, pg) != quota_before:
                    raise ValueError("reviewed additive migration changed existing business data")
                if any(self.sql(query, pg) != value for query, value in monitor_before.items()):
                    raise ValueError('reviewed migration changed existing media or monitor data')
                job["additive_data_verified"] = True
            run(["docker", "run", "-d", "--name", redis, "--network", network, "--network-alias", "redis",
                 "--cpus=0.5", "--memory=512m", "redis:8-alpine"])
            original_env = dict(v.split("=", 1) for v in self.inspect()["Config"]["Env"] if "=" in v)
            env = {"AUTO_SETUP": "true", "DATABASE_HOST": pg, "DATABASE_PORT": "5432",
                   "DATABASE_USER": "sub2api", "DATABASE_DBNAME": "sub2api", "DATABASE_PASSWORD": password,
                   "DATABASE_SSLMODE": "disable", "REDIS_HOST": redis, "REDIS_PORT": "6379", "REDIS_PASSWORD": "",
                   "JWT_SECRET": original_env["JWT_SECRET"], "TOTP_ENCRYPTION_KEY": original_env["TOTP_ENCRYPTION_KEY"],
                   "SERVER_HOST": "0.0.0.0", "SERVER_PORT": "8080", "RUN_MODE": "standard"}
            args = ["docker", "run", "-d", "--name", app, "--network", network, "--cpus=2", "--memory=2g"]
            for k, v in env.items():
                args += ["-e", k + "=" + v]
            run(args + [job["image"]])
            self.wait_command(["docker", "exec", app, "wget", "-qO-", "http://127.0.0.1:8080/health"], 180)
            # Logger versions may be written to stderr. Inspect build labels and
            # the public settings version instead of relying on stdout alone.
            settings = json.loads(run(["docker", "exec", app, "wget", "-qO-", "http://127.0.0.1:8080/api/v1/settings/public"]))
            if settings.get("data", {}).get("version", "").lstrip("v") != job["version"][1:]:
                raise ValueError("smoke-test version mismatch")
            run(["docker", "rm", "-f", app])
            # Prove the exact captured runtime also starts against the same schema.
            run(args + [job["rollback_image"]])
            self.wait_command(["docker", "exec", app, "wget", "-qO-", "http://127.0.0.1:8080/health"], 180)
            job["restore_verified"] = True
            job["rollback_verified"] = True
        finally:
            for name in (app, redis, pg):
                subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            subprocess.run(["docker", "network", "rm", network], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            # Remove only this job's disposable restored database after containers
            # are gone. Backups and all production bind mounts remain untouched.
            if rehearsal.resolve().parent == (self.state / "rehearsals").resolve():
                remaining = subprocess.run(["docker", "inspect", pg], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                if remaining.returncode != 0:
                    shutil.rmtree(rehearsal)

    @staticmethod
    def wait_command(args, seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            try:
                run(args, timeout=10)
                return
            except (RuntimeError, subprocess.TimeoutExpired):
                time.sleep(2)
        raise RuntimeError("health check timed out")

    def override(self, image, docs_image=None):
        # JSON is valid YAML; preserve all production service settings by layering.
        path = self.state / "active-image.json"
        services = {"sub2api": {"image": image, "pull_policy": "never",
                    "stop_grace_period": "130s", "environment": {
                        "IMAGE_DEPLOY_SOCKET": SOCKET, "IMAGE_DEPLOY_DRAIN_FILE": "/run/sub2api-deployer/drain",
                        "SERVER_SHUTDOWN_TIMEOUT_SECONDS": "120",
                        "UPDATE_GITHUB_TOKEN_FILE": "/run/secrets/sub2api-release-token"},
                    "volumes": ["/run/sub2api-deployer:/run/sub2api-deployer:ro",
                                str(self.config.get("release_token_file", "/etc/sub2api-deployer/release-token")) + ":/run/secrets/sub2api-release-token:ro"]}}
        if docs_image:
            services["docs-web"] = {"image": docs_image, "pull_policy": "never",
                                    "restart": "unless-stopped",
                                    "ports": ["127.0.0.1:8082:8080"],
                                    "healthcheck": {"test": ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8080/health"]}}
        atomic_json(path, {"services": services})
        return path

    def switch(self, image, docs_image=None):
        path = self.override(image, docs_image)
        services = ["sub2api"] + (["docs-web"] if docs_image else [])
        run(self.compose(path) + ["up", "-d", "--no-deps", "--pull", "never", *services], timeout=300)
        # A rescue target can intentionally have no docs image (for example,
        # the first release before docs-web existed).  Remove only the named
        # docs container so a failed dual-container cutover cannot leave the
        # newer documentation image serving traffic.
        if not docs_image:
            subprocess.run(["docker", "rm", "-f", "sub2api-docs-web"],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.wait_command(["curl", "-fsS", "--max-time", "5", "http://127.0.0.1:8081/health"], 180)
        current = self.inspect()
        expected_id = json.loads(run(["docker", "image", "inspect", image]))[0]["Id"]
        if current["Image"] != expected_id:
            raise ValueError("running container image does not match requested digest")
        if docs_image:
            docs = json.loads(run(["docker", "inspect", "sub2api-docs-web"]))[0]
            expected_docs_id = json.loads(run(["docker", "image", "inspect", docs_image]))[0]["Id"]
            if docs["Image"] != expected_docs_id or not docs["State"].get("Running"):
                raise ValueError("running documentation image does not match requested digest")
            self.wait_command(["docker", "exec", "sub2api-docs-web", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://127.0.0.1:8080/health"], 60)
            html = run(["curl", "-fsS", "--max-time", "10", "http://127.0.0.1:8082/docs/imagevideo"]).decode(errors="replace")
            if "词元互联图片、视频与音乐接口文档" not in html:
                raise ValueError("documentation route content check failed")
        if self.config.get("manage_docs_proxy", True):
            from manage import Manager
            Manager(self.state, self.root, docs_proxy_enabled=self.config.get("docs_proxy_enabled", True)).sync_docs_proxy(bool(docs_image))

    def deploy(self, job):
        self.save(job, "checking_cutover")
        if not job.get("restore_verified") or not job.get("rollback_verified"):
            raise ValueError("restore and rollback rehearsal are required")
        backup = self.state / "backups" / job["id"]
        expected = json.loads((backup / "checksums.json").read_text())
        for name, digest in expected.items():
            if Path(name).name != name or file_hash(backup / name) != digest:
                raise ValueError("preparation backup integrity check failed")
        original = self.inspect()
        if (original["Id"] != job["original_container"] or original["State"]["StartedAt"] != job["original_started"]
                or self.config_hashes() != job["config_hashes"]):
            raise ValueError("production changed after preparation; prepare again")
        actual = run(["docker", "exec", "sub2api", "sha256sum", "/app/sub2api"]).decode().split()[0]
        if actual != job["original_binary_sha256"]:
            raise ValueError("production binary changed after preparation")
        self.assert_migrations(job["manifest"])
        # Fresh online snapshot before every cutover; never restore it automatically.
        self.save(job, "final_backup")
        final = self.state / "backups" / (job["id"] + "-cutover")
        final.mkdir(mode=0o700)
        self.backup_database(final)
        self.save(job, "draining")
        marker = Path("/run/sub2api-deployer/drain")
        marker.touch(mode=0o644)
        os.chmod(marker, 0o644)
        try:
            # Captured rollback images use a local image ID but still support
            # draining once the controller's environment has been installed.
            if "IMAGE_DEPLOY_DRAIN_FILE=/run/sub2api-deployer/drain" in original["Config"].get("Env", []):
                deadline = time.monotonic() + 120
                while True:
                    status = json.loads(run(["docker", "exec", "sub2api", "wget", "-qO-",
                                             "http://127.0.0.1:8080/internal/deployment/status"]))
                    if status.get("draining") and status.get("active") == 0:
                        break
                    if time.monotonic() >= deadline:
                        raise ValueError("active requests did not drain; retry during a quieter window")
                    time.sleep(1)
            self.save(job, "switching")
            self.switch(job["image"], job.get("docs_image"))
            marker.unlink(missing_ok=True)
            # The application caches the drain marker briefly. Health bypasses
            # draining, so wait for normal admission before verifying settings.
            self.wait_command(["curl", "-fsS", "--max-time", "5",
                               "http://127.0.0.1:8081/api/v1/settings/public"], 30)
            settings = json.loads(run(["curl", "-fsS", "--max-time", "10",
                                      "http://127.0.0.1:8081/api/v1/settings/public"]))
            if settings.get("data", {}).get("version", "").lstrip("v") != job["version"][1:]:
                raise ValueError("production version mismatch")
            for url in self.config["health_urls"]:
                run(["curl", "-fsS", "--max-time", "15", url])
            self.save(job, "succeeded")
        except Exception:
            if job["status"] == "draining":
                raise
            self.save(job, "rolling_back")
            self.switch(job["rollback_image"], job.get("rollback_docs_image"))
            self.save(job, "rolled_back")
        finally:
            marker.unlink(missing_ok=True)

    def rollback(self, job):
        self.assert_migrations(job["manifest"])
        if self.inspect()["Config"]["Image"] != job["image"]:
            raise ValueError("production no longer runs this job's image")
        self.save(job, "final_backup")
        backup = self.state / "backups" / (job["id"] + "-manual-rollback")
        backup.mkdir(mode=0o700)
        self.backup_database(backup)
        marker = Path("/run/sub2api-deployer/drain")
        marker.touch(mode=0o644)
        os.chmod(marker, 0o644)
        try:
            self.save(job, "draining")
            deadline = time.monotonic() + 120
            while True:
                status = json.loads(run(["docker", "exec", "sub2api", "wget", "-qO-",
                                         "http://127.0.0.1:8080/internal/deployment/status"]))
                if status.get("draining") and status.get("active") == 0:
                    break
                if time.monotonic() >= deadline:
                    raise ValueError("active requests did not drain; rollback aborted")
                time.sleep(1)
            self.save(job, "rolling_back")
            try:
                self.switch(job["rollback_image"], job.get("rollback_docs_image"))
            except Exception:
                # A failed manual rollback should attempt to restore the image
                # that was healthy when the operator requested rollback.
                self.switch(job["image"], job.get("docs_image"))
                raise
            self.save(job, "rolled_back")
        finally:
            marker.unlink(missing_ok=True)


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, status, result):
        data = json.dumps(result).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path != "/deployment":
            return self.reply(404, {"error": "not found"})
        self.reply(200, self.server.controller.status())

    def do_POST(self):
        if self.path != "/deployment":
            return self.reply(404, {"error": "not found"})
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if size < 1 or size > 4096:
                raise ValueError("invalid request length")
            data = json.loads(self.rfile.read(size))
            if not isinstance(data, dict):
                raise ValueError("expected an object")
            result = self.server.controller.submit(data)
            self.reply(202, result)
        except (ValueError, FileNotFoundError, BlockingIOError) as exc:
            self.reply(409, {"error": str(exc) if isinstance(exc, ValueError) else "job unavailable or controller busy"})


class Server(socketserver.ThreadingMixIn, getattr(socketserver, "UnixStreamServer", socketserver.TCPServer)):
    daemon_threads = True


def client(request=None):
    class Connection(http.client.HTTPConnection):
        def connect(self):
            self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self.sock.settimeout(15)
            self.sock.connect(SOCKET)
    conn = Connection("localhost")
    conn.request("POST" if request else "GET", "/deployment", json.dumps(request) if request else None,
                 {"Content-Type": "application/json"})
    response = conn.getresponse()
    result = response.read().decode()
    print(result)
    if response.status >= 400:
        raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("serve", "status", "request", "ssh"))
    parser.add_argument("--action", choices=("prepare", "deploy", "rollback"))
    parser.add_argument("--version", default="")
    parser.add_argument("--job-id", default="")
    parser.add_argument("--confirm", action="store_true")
    args = parser.parse_args()
    if args.command == "serve":
        if os.geteuid() != 0 or CONFIG.stat().st_uid != 0 or CONFIG.stat().st_mode & 0o022:
            raise SystemExit("root-owned non-writable configuration required")
        controller = Controller(json.loads(CONFIG.read_text()))
        Path(SOCKET).parent.mkdir(mode=0o750, parents=True, exist_ok=True)
        os.chown(Path(SOCKET).parent, 0, int(controller.config["socket_gid"]))
        with contextlib.suppress(FileNotFoundError):
            os.unlink(SOCKET)
        with Server(SOCKET, Handler) as server:
            server.controller = controller
            os.chown(SOCKET, 0, int(controller.config["socket_gid"]))
            os.chmod(SOCKET, 0o660)
            # Never resume an interrupted switch automatically after a host crash.
            for path in (controller.state / "jobs").glob("*.json"):
                job = json.loads(path.read_text())
                if job["status"] not in ("ready", "succeeded", "failed", "rolled_back", "interrupted"):
                    controller.save(job, "interrupted")
            server.serve_forever()
    elif args.command == "status":
        client()
    elif args.command == "ssh":
        # authorized_keys forced command; never execute SSH_ORIGINAL_COMMAND.
        words = os.environ.get("SSH_ORIGINAL_COMMAND", "").split()
        if words == ["status"]:
            client()
        elif len(words) == 3 and words[2] == "CONFIRM" and words[0] in ("prepare", "deploy", "rollback"):
            req = {"action": words[0], "confirm": True}
            req["version" if words[0] == "prepare" else "job_id"] = words[1]
            validate_request(req)
            client(req)
        else:
            raise SystemExit("restricted deployment command rejected")
    else:
        req = {"action": args.action, "version": args.version, "job_id": args.job_id, "confirm": args.confirm}
        validate_request(req)
        client(req)


if __name__ == "__main__":
    main()
