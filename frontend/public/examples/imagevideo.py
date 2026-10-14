"""Python 3.10+; only standard library. Submit once, resume by saved task ID."""
import argparse
import json
import mimetypes
import os
import time
import uuid
from pathlib import Path
from urllib.parse import quote, urljoin, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


class Client:
    def __init__(self, base=None, key=None):
        self.base = (base or os.getenv("MEDIA_BASE_URL", "https://code.aicodeme.xyz")).rstrip("/")
        self.key = key or os.environ["MEDIA_API_KEY"]
        u = urlsplit(self.base)
        if u.scheme not in ("http", "https") or not u.netloc or u.path or u.query or u.fragment or u.username:
            raise ValueError("Base URL must be a site origin without /v1")
        if u.scheme == "http" and u.hostname not in ("localhost", "127.0.0.1", "::1"):
            raise ValueError("Use HTTPS")
        self.opener = build_opener(NoRedirect())

    def request(self, method, path, payload=None, extra=None):
        url = urljoin(self.base + "/", path)
        if urlsplit(url).netloc != urlsplit(self.base).netloc or urlsplit(url).scheme != urlsplit(self.base).scheme:
            raise ValueError("Artifact URL must belong to the API origin")
        headers = {"Authorization": "Bearer " + self.key, **(extra or {})}
        body = None
        if payload is not None:
            if isinstance(payload, bytes):
                body = payload
            else:
                body = json.dumps(payload, ensure_ascii=False).encode()
                headers["Content-Type"] = "application/json"
        return self.opener.open(Request(url, data=body, headers=headers, method=method), timeout=200)

    def json(self, method, path, payload=None, extra=None):
        with self.request(method, path, payload, extra) as r:
            return json.load(r)

    def download(self, artifact, directory, index):
        if not artifact.get("available", True):
            raise ValueError("Artifact unavailable")
        with self.request("GET", artifact["content_url"]) as r:
            mime = r.headers.get_content_type()
            extensions = {"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "video/mp4": ".mp4", "audio/mpeg": ".mp3", "audio/mp3": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav", "audio/mp4": ".m4a", "audio/ogg": ".ogg", "audio/flac": ".flac", "application/octet-stream": ".bin"}
            if mime not in extensions:
                raise ValueError("Unexpected artifact MIME: " + mime)
            path = directory / (str(index) + extensions[mime])
            tmp = path.with_suffix(path.suffix + ".part")
            try:
                with tmp.open("wb") as f:
                    while chunk := r.read(65536):
                        f.write(chunk)
                tmp.replace(path)
            finally:
                tmp.unlink(missing_ok=True)
            return str(path)


def save(path, state, exclusive=False):
    if exclusive:
        with path.open("x", encoding="utf-8") as f:
            json.dump(state, f, ensure_ascii=False, indent=2)
    else:
        tmp = path.with_suffix(path.suffix + ".tmp")
        tmp.write_text(json.dumps(state, ensure_ascii=False, indent=2), encoding="utf-8")
        tmp.replace(path)


def edit_body(payload, images):
    boundary = "media-" + uuid.uuid4().hex
    chunks = []
    for name, value in payload.items():
        value = value if isinstance(value, str) else json.dumps(value)
        chunks.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
    for name in images:
        path = Path(name)
        mime = mimetypes.guess_type(path.name)[0]
        if mime not in ("image/png", "image/jpeg", "image/webp"):
            raise ValueError("Use PNG, JPEG or WebP references")
        chunks.extend([f'--{boundary}\r\nContent-Disposition: form-data; name="image[]"; filename="reference"\r\nContent-Type: {mime}\r\n\r\n'.encode(), path.read_bytes(), b"\r\n"])
    chunks.append(f"--{boundary}--\r\n".encode())
    return b"".join(chunks), "multipart/form-data; boundary=" + boundary


def submit(client, endpoint, payload, state_path, images=None):
    extra = {}
    if images:
        if endpoint != "/v1/images/edits" or len(images) > 2:
            raise ValueError("Image uploads require /v1/images/edits and 1-2 files")
        payload, extra["Content-Type"] = edit_body(payload, images)
    state = {"base": client.base, "idempotency_key": str(uuid.uuid4()), "task_id": None, "status": "submitting"}
    save(state_path, state, exclusive=True)
    # Never retry POST automatically, even if the connection times out.
    result = client.json("POST", endpoint, payload, {"Idempotency-Key": state["idempotency_key"], **extra})
    state.update(task_id=result.get("task_id") or result.get("id"), status=result.get("status", "submitted"))
    save(state_path, state)
    if not isinstance(state["task_id"], str) or not state["task_id"].strip():
        raise ValueError("Missing task ID; preserve state and reconcile, do not resubmit")
    return state


def resume(client, state_path, directory, interval=20, max_wait=3600):
    state = json.loads(state_path.read_text(encoding="utf-8"))
    if state["base"] != client.base or not state.get("task_id"):
        raise ValueError("State origin mismatch or no task ID; reconcile before another submission")
    deadline = time.monotonic() + max_wait
    path = "/v1/tasks/" + quote(state["task_id"], safe="")
    while time.monotonic() < deadline:
        task = client.json("GET", path)
        if task.get("id") != state["task_id"]:
            raise ValueError("Task ID mismatch")
        state["status"] = task["status"]
        save(state_path, state)
        if task["status"] == "completed":
            artifacts = client.json("GET", path + "/artifacts")["artifacts"]
            if not artifacts:
                raise ValueError("Completed task has no artifacts yet; retain ID and reconcile delivery")
            directory.mkdir(parents=True, exist_ok=True)
            files = [client.download(a, directory, i) for i, a in enumerate(artifacts)]
            state["files"] = files
            save(state_path, state)
            return files
        if task["status"] not in ("queued", "in_progress"):
            raise ValueError("Task " + task["status"] + "; retain ID for reconciliation")
        time.sleep(interval)
    raise TimeoutError("Polling ended; run resume again using the same state file")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("command", choices=("submit", "resume"))
    p.add_argument("--endpoint", default="/v1/videos")
    p.add_argument("--payload", help="UTF-8 JSON file; required for submit")
    p.add_argument("--image", action="append", help="Local GPT edit reference; repeat for two files")
    p.add_argument("--state", default="media-task.json")
    p.add_argument("--output", default="media-output")
    p.add_argument("--interval", type=float, default=20)
    p.add_argument("--max-wait", type=float, default=3600)
    args = p.parse_args()
    if args.interval <= 0 or args.max_wait <= 0:
        p.error("interval and max-wait must be positive")
    client = Client()
    if args.command == "submit":
        if not args.payload:
            p.error("submit requires --payload")
        payload = json.loads(Path(args.payload).read_text(encoding="utf-8-sig"))
        print(submit(client, args.endpoint, payload, Path(args.state), args.image))
    else:
        print(resume(client, Path(args.state), Path(args.output), args.interval, args.max_wait))


if __name__ == "__main__":
    main()
