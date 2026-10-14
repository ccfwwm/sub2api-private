"""Offline contract checks for the downloadable clients. Never calls a provider."""
import json
from email.parser import BytesParser
from email.policy import default
import os
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CLIENTS = ROOT / "frontend/public/examples"


class PublicClientsTest(unittest.TestCase):
    def run_case(self, language, scenario, mime="audio/wav", extension=".wav"):
        calls = []
        polls = 0

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def respond(self, status, body, content_type="application/json"):
                raw = json.dumps(body).encode() if content_type == "application/json" else body
                self.send_response(status)
                self.send_header("Content-Type", content_type)
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def do_POST(self):
                calls.append(("POST", self.path, self.headers.get("Authorization")))
                raw = self.rfile.read(int(self.headers["Content-Length"]))
                if scenario == "multipart":
                    message = BytesParser(policy=default).parsebytes(("Content-Type: " + self.headers["Content-Type"] + "\r\nMIME-Version: 1.0\r\n\r\n").encode() + raw)
                    parts = list(message.iter_parts())
                    refs = [p for p in parts if p.get_param("name", header="Content-Disposition") == "image[]"]
                    self.server.test.assertEqual(len(refs), 2)
                    for ref in refs:
                        self.server.test.assertEqual(ref.get_content_type(), "image/png")
                        self.server.test.assertEqual(ref.get_payload(decode=True), b"PNG-REFERENCE")
                    payload = {p.get_param("name", header="Content-Disposition"): p.get_payload(decode=True).decode() for p in parts if not p.get_filename()}
                    self.server.test.assertEqual(self.path, "/v1/images/edits")
                else:
                    payload = json.loads(raw)
                self.server.test.assertEqual(payload["model"], "lyria-3.5")
                self.server.test.assertTrue(self.headers.get("Idempotency-Key"))
                if scenario == "sync":
                    self.respond(200, {"task_id": "task-local", "data": []})
                else:
                    self.respond(202, {"id": "task-local", "status": "queued"})

            def do_GET(self):
                nonlocal polls
                calls.append(("GET", self.path, self.headers.get("Authorization")))
                if self.path.endswith("/content"):
                    self.respond(200, b"RIFF\x10\x00\x00\x00WAVEfmt MOCK", mime)
                elif self.path.endswith("/artifacts"):
                    path = "/v1/tasks/task-local/artifacts/0/content"
                    if scenario == "foreign":
                        path = "https://other.invalid/content"
                    self.respond(200, {"task_id": "task-local", "artifacts": [{"key": "0", "content_url": path, "mime_type": mime, "available": True}]})
                else:
                    polls += 1
                    status = "unknown" if scenario == "unknown" else "in_progress" if polls == 1 else "completed"
                    self.respond(200, {"id": "task-local", "status": status})

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        server.test = self
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            with tempfile.TemporaryDirectory(prefix="media-clients-") as directory:
                folder = Path(directory)
                payload = folder / "request.json"
                payload.write_text(json.dumps({"model": "lyria-3.5", "prompt": "local mock", "async": scenario != "sync"}), encoding="utf-8")
                state = folder / "task.json"
                output = folder / "output"
                env = dict(os.environ, MEDIA_API_KEY="local-test-key", MEDIA_BASE_URL=f"http://127.0.0.1:{server.server_port}")
                command = [sys.executable, str(CLIENTS / "imagevideo.py")] if language == "python" else ["node", str(CLIENTS / "imagevideo.mjs")]
                submit = [*command, "submit", "--endpoint", "/v1/audio/generations", "--payload", str(payload), "--state", str(state)]
                if scenario == "multipart":
                    reference = folder / "reference.png"
                    reference.write_bytes(b"PNG-REFERENCE")
                    submit = [*command, "submit", "--endpoint", "/v1/images/edits", "--payload", str(payload), "--state", str(state), "--image", str(reference), "--image", str(reference)]
                result = subprocess.run(submit, env=env, capture_output=True, text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                # Existing state prevents another POST, including after restart.
                repeated = subprocess.run(submit, env=env, capture_output=True, text=True, timeout=15)
                self.assertNotEqual(repeated.returncode, 0)
                result = subprocess.run([*command, "resume", "--state", str(state), "--output", str(output), "--interval", "0.001", "--max-wait", "5"], env=env, capture_output=True, text=True, timeout=15)
                if scenario in ("unknown", "foreign"):
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse(list(output.glob("*")))
                else:
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual((output / ("0" + extension)).read_bytes(), b"RIFF\x10\x00\x00\x00WAVEfmt MOCK")
                self.assertEqual(json.loads(state.read_text())["task_id"], "task-local")
                self.assertEqual(sum(method == "POST" for method, _, _ in calls), 1)
                self.assertTrue(all(auth == "Bearer local-test-key" for _, _, auth in calls))
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=5)

    def test_async_sync_and_sync_fallback_resume(self):
        for language in ("python", "javascript"):
            for scenario, mime, extension in (("async", "audio/wav", ".wav"), ("sync", "image/png", ".png"), ("sync_fallback", "audio/mp4", ".m4a")):
                with self.subTest(language=language, scenario=scenario):
                    self.run_case(language, scenario, mime, extension)

    def test_unknown_stops_without_resubmission(self):
        for language in ("python", "javascript"):
            with self.subTest(language=language):
                self.run_case(language, "unknown")

    def test_multipart_edit_preserves_two_files_and_resumes(self):
        for language in ("python", "javascript"):
            with self.subTest(language=language):
                self.run_case(language, "multipart", "image/png", ".png")

    def test_never_sends_key_to_foreign_artifact_origin(self):
        for language in ("python", "javascript"):
            with self.subTest(language=language):
                self.run_case(language, "foreign")


if __name__ == "__main__":
    unittest.main(verbosity=2)
