"""Server-side example. Uses stdlib; never retries a generation POST automatically."""
import argparse
import hashlib
import json
import mimetypes
import os
import sys
import time
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class ApiError(Exception):
    def __init__(self, status=0, retry_after=0, code="transport_or_invalid_response"):
        super().__init__(f"API {code}; HTTP {status}; keep task state, never blindly resubmit")
        self.status, self.retry_after = status, retry_after


def retry_after(value):
    try:
        return max(0, float(value))
    except (TypeError, ValueError):
        try:
            return max(0, (parsedate_to_datetime(value) - datetime.now(timezone.utc)).total_seconds())
        except (TypeError, ValueError, OverflowError):
            return 0


def encode(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def atomic_json(path, value, exclusive=False):
    data = json.dumps(value, ensure_ascii=False, indent=2).encode("utf-8")
    if exclusive:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "wb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        return
    tmp = path.with_name(path.name + "." + uuid.uuid4().hex + ".tmp")
    try:
        with tmp.open("xb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp, path)
    finally:
        tmp.unlink(missing_ok=True)


@contextmanager
def state_lock(path):
    lock = path.with_name(path.name + ".lock")
    fd = os.open(lock, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "w") as f:
            f.write(str(os.getpid()))
        yield
    finally:
        lock.unlink(missing_ok=True)


class Client:
    def __init__(self):
        self.base = os.environ["SUB2API_BASE_URL"].rstrip("/")
        url = urlsplit(self.base)
        if url.scheme not in ("http", "https") or not url.netloc or url.path not in ("", "/") or url.query or url.fragment or url.username or url.password:
            raise ValueError("SUB2API_BASE_URL must be a site origin, without /v1, query or credentials")
        if url.scheme == "http" and url.hostname not in ("localhost", "127.0.0.1", "::1"):
            raise ValueError("Use HTTPS outside localhost")
        self.key = os.environ["SUB2API_API_KEY"]
        if not self.key.strip():
            raise ValueError("Missing API Key")
        self.timeout = 120
        self.opener = build_opener(NoRedirect())

    def response(self, method, path, body=None, extra=None):
        headers = {"Authorization": "Bearer " + self.key, **(extra or {})}
        req = Request(self.base + path, data=body, method=method, headers=headers)
        try:
            r = self.opener.open(req, timeout=self.timeout)
        except HTTPError as e:
            status, delay = e.code, retry_after(e.headers.get("Retry-After"))
            e.close()
            raise ApiError(status, delay, "http_error") from None
        except (URLError, TimeoutError, OSError):
            raise ApiError() from None
        if not 200 <= r.status < 300:
            r.close()
            raise ApiError(r.status)
        return r

    def json(self, method, path, payload=None, extra=None, raw=None):
        headers = {**(extra or {})}
        if raw is None and payload is not None:
            raw = encode(payload)
            headers["Content-Type"] = "application/json"
        with self.response(method, path, raw, headers) as r:
            mime = r.headers.get_content_type()
            if mime != "application/json" and not mime.endswith("+json"):
                raise ApiError(r.status, code="expected_json")
            raw_response = r.read(64 * 1024 * 1024 + 1)
            if len(raw_response) > 64 * 1024 * 1024:
                raise ApiError(r.status, code="json_too_large")
            try:
                value = json.loads(raw_response)
            except (ValueError, UnicodeError):
                raise ApiError(r.status, code="invalid_json") from None
            if not isinstance(value, dict):
                raise ApiError(r.status, code="invalid_object")
            return value

    def download(self, path, output, kind):
        tmp = output.with_name(output.name + "." + uuid.uuid4().hex + ".part")
        limit = 256 * 1024 * 1024
        digest, size = hashlib.sha256(), 0
        try:
            with self.response("GET", path) as r:
                mime = r.headers.get_content_type()
                allowed = {"video": ["video/mp4", "application/octet-stream"], "audio": ["audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav", "audio/mp4", "audio/x-m4a", "application/octet-stream"]}
                if r.status != 200 or mime not in allowed[kind]:
                    raise ApiError(r.status, code="expected_complete_media")
                prefix = r.read(16)
                valid = prefix[4:8] == b"ftyp" if kind == "video" else prefix.startswith((b"ID3", b"RIFF")) or (len(prefix) >= 2 and prefix[0] == 255 and prefix[1] & 224 == 224) or prefix[4:8] == b"ftyp"
                if not valid:
                    raise ApiError(r.status, code="invalid_media_signature")
                with tmp.open("xb") as f:
                    chunk = prefix
                    while chunk:
                        size += len(chunk)
                        if size > limit:
                            raise ApiError(r.status, code="download_limit")
                        digest.update(chunk)
                        f.write(chunk)
                        chunk = r.read(64 * 1024)
                    f.flush()
                    os.fsync(f.fileno())
                length = r.headers.get("Content-Length")
                if length is not None and int(length) != size:
                    raise ApiError(r.status, code="truncated_media")
            os.replace(tmp, output)
            return {"bytes": size, "sha256": digest.hexdigest()}
        finally:
            tmp.unlink(missing_ok=True)


def read_payload(path):
    value = json.loads(Path(path).read_text(encoding="utf-8-sig"))
    if not isinstance(value, dict):
        raise ValueError("Payload must be a JSON object")
    return value


def submit(client, args):
    payload = read_payload(args.payload)
    path = "/v1/videos" if args.kind == "video" else "/v1/audio/generations"
    state_path = Path(args.state)
    with state_lock(state_path):
        state = dict(version=1, base=client.base, kind=args.kind, endpoint=path,
                     request_hash=hashlib.sha256(encode(payload)).hexdigest(), idempotency_key=str(uuid.uuid4()),
                     task_id=None, status="pending_submission")
        atomic_json(state_path, state, exclusive=True)
        try:
            task = client.json("POST", path, payload, {"Idempotency-Key": state["idempotency_key"]})
            if not isinstance(task.get("id"), str) or not task["id"].strip():
                raise ApiError(code="missing_task_id")
            state.update(task_id=task["id"], status=task.get("status", "unknown"))
            if state["status"] not in ["queued", "in_progress", "completed", "failed", "unknown"]:
                state["status"] = "unknown"
            atomic_json(state_path, state)
        except Exception as e:
            state.update(status="rejected" if isinstance(e, ApiError) and 400 <= e.status < 500 and e.status != 408 else "unknown")
            atomic_json(state_path, state)
            raise
        print(json.dumps({"id": state["task_id"], "status": state["status"]}))
        if state["status"] == "failed":
            raise ApiError(code="task_failed")


def resume(client, args):
    state_path = Path(args.state)
    with state_lock(state_path):
        state = json.loads(state_path.read_text(encoding="utf-8"))
        if state.get("version") != 1 or state.get("base") != client.base or state.get("kind") not in ["video", "audio"]:
            raise ValueError("State version/base/kind mismatch")
        if not isinstance(state.get("task_id"), str) or not state["task_id"].strip():
            raise ValueError("No task ID: inspect pending submission using its hash/idempotency key; do not resubmit")
        if state.get("status") == "failed":
            raise ApiError(code="task_failed")
        root = "/v1/videos/" if state["kind"] == "video" else "/v1/audio/generations/"
        path = root + quote(state["task_id"], safe="")
        deadline = time.monotonic() + args.max_wait
        interval, next_allowed = args.poll_interval, time.monotonic()
        while time.monotonic() < deadline:
            if next_allowed > time.monotonic():
                time.sleep(min(60, next_allowed - time.monotonic(), max(0, deadline - time.monotonic())))
                continue
            try:
                task = client.json("GET", path)
                if task.get("id") != state["task_id"]:
                    raise ApiError(code="task_id_mismatch")
                status = task.get("status", "unknown")
                state["status"] = status if status in ["queued", "in_progress", "completed", "failed", "unknown"] else "unknown"
                atomic_json(state_path, state)
                if state["status"] == "failed":
                    raise ApiError(code="task_failed")
                if state["status"] == "completed":
                    result = client.download(path + "/content", Path(args.output), state["kind"])
                    state.update(download=result)
                    atomic_json(state_path, state)
                    print(json.dumps({"id": state["task_id"], "status": "completed", **result}))
                    return
                interval = args.poll_interval
            except ApiError as e:
                if e.status not in (0, 429, 502, 503, 504) or "task_failed" in str(e) or "mismatch" in str(e):
                    raise
                interval = min(60, max(interval * 2, e.retry_after))
                next_allowed = time.monotonic() + max(interval, e.retry_after)
                continue
            next_allowed = time.monotonic() + interval
        print("Polling window ended; task ID preserved. Run resume again; no generation POST was sent.")


def multipart(payload, files):
    boundary = "sub2api-example-" + uuid.uuid4().hex
    chunks = []
    def part(name, value, filename=None, mime=None):
        header = f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"'
        if filename:
            header += f'; filename="{filename}"\r\nContent-Type: {mime}'
        chunks.extend([(header + "\r\n\r\n").encode(), value, b"\r\n"])
    for name, value in payload.items():
        if name not in ["model", "prompt", "n", "size", "quality", "response_format"]:
            raise ValueError("Unsupported edit field")
        part(name, str(value).encode("utf-8"))
    total = 0
    for index, file in enumerate(files):
        file = Path(file)
        total += file.stat().st_size
        if total > 64 * 1024 * 1024:
            raise ValueError("Example upload total exceeds 64 MiB; actual model limits may be lower")
        mime = mimetypes.guess_type(file.name)[0]
        if mime not in ["image/png", "image/jpeg", "image/webp"]:
            raise ValueError("Upload PNG/JPEG/WebP only")
        ext = {"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[mime]
        part("image[]", file.read_bytes(), f"reference-{index}.{ext}", mime)
    chunks.append(f"--{boundary}--\r\n".encode())
    return b"".join(chunks), "multipart/form-data; boundary=" + boundary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subs = parser.add_subparsers(dest="command", required=True)
    subs.add_parser("models")
    estimate = subs.add_parser("estimate")
    estimate.add_argument("--payload", required=True)
    s = subs.add_parser("submit")
    s.add_argument("--kind", choices=["video", "audio"], default="video")
    s.add_argument("--payload", required=True)
    s.add_argument("--state", required=True)
    r = subs.add_parser("resume")
    r.add_argument("--state", required=True)
    r.add_argument("--output", required=True)
    r.add_argument("--max-wait", type=float, default=1200)
    r.add_argument("--poll-interval", type=float, default=20)
    c = subs.add_parser("call")
    c.add_argument("--endpoint", choices=["images/generations", "chat/completions", "gemini"], required=True)
    c.add_argument("--model")
    c.add_argument("--payload", required=True)
    c.add_argument("--output", required=True)
    e = subs.add_parser("edit")
    e.add_argument("--payload", required=True)
    e.add_argument("--image", action="append", required=True)
    e.add_argument("--output", required=True)
    args = parser.parse_args()
    client = Client()
    if args.command == "submit":
        submit(client, args)
    elif args.command == "resume":
        if not 0 < args.poll_interval <= 60 or args.max_wait <= 0:
            raise ValueError("Positive max-wait, poll-interval in (0,60] required")
        resume(client, args)
    elif args.command == "models":
        print(json.dumps(client.json("GET", "/v1/models"), ensure_ascii=False))
    elif args.command == "estimate":
        print(json.dumps(client.json("POST", "/v1/sub2api/estimates", read_payload(args.payload)), ensure_ascii=False))
    else:
        if args.command == "edit":
            body, mime = multipart(read_payload(args.payload), args.image)
            result = client.json("POST", "/v1/images/edits", extra={"Content-Type": mime}, raw=body)
        else:
            if args.endpoint == "gemini" and not args.model:
                raise ValueError("Gemini requires --model")
            path = "/v1beta/models/" + quote(args.model, safe="") + ":generateContent" if args.endpoint == "gemini" else "/v1/" + args.endpoint
            result = client.json("POST", path, read_payload(args.payload))
        atomic_json(Path(args.output), result)
        print("Response saved. Treat returned signed URLs as private; no automatic POST retries.")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        # Do not echo response bodies, URLs with signatures, or environment secrets.
        print(str(exc) if isinstance(exc, ApiError) else "Client stopped (" + type(exc).__name__ + "); inspect local inputs/state, no automatic resubmission.", file=sys.stderr)
        sys.exit(2)
