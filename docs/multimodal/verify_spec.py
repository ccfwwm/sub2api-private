"""Verify design files and run real CLI clients against a local HTTP mock.

No production credentials, containers, databases, or billable provider calls.
The Decimal evaluator below verifies spec vectors; it is not the future Go engine.
"""
import copy
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from decimal import Decimal, ROUND_CEILING, ROUND_FLOOR, ROUND_HALF_UP, localcontext
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import jsonschema
import yaml
from openapi_spec_validator import validate as validate_openapi

from generate_spec import MODELS_V, MODELS_Y, SCENARIOS, UNITS

ROOT = Path(__file__).resolve().parent
RESULTS = []


def load(name):
    return json.loads((ROOT / name).read_text(encoding="utf-8"))


def passed(name):
    RESULTS.append(name)
    print("PASS " + name)


def formula(case):
    with localcontext() as ctx:
        ctx.prec = 60
        total = Decimal(0)
        for line in case["lines"]:
            if line["price"] is None:
                raise ValueError("price_not_configured")
            raw = Decimal(line["quantity"])
            mode = line.get("mode", "none")
            if mode != "none":
                step = Decimal(line.get("step", "1"))
                assert step > 0
                raw = (raw / step).to_integral_value(rounding={"ceil": ROUND_CEILING, "floor": ROUND_FLOOR, "half_up": ROUND_HALF_UP}[mode]) * step
            raw = max(raw, Decimal(line.get("minimum", "0")))
            quantity = raw / 60 if line["unit"] == "minute" else raw
            method = line.get("method", "linear")
            if method == "package":
                assert quantity == Decimal(line["package_quantity"])
                amount = Decimal(line["price"])
            elif method == "progressive":
                amount, previous = Decimal(0), Decimal(0)
                for tier in line["tiers"]:
                    end = quantity if tier["up_to"] is None else Decimal(tier["up_to"])
                    assert end >= previous
                    amount += max(Decimal(0), min(quantity, end) - previous) * Decimal(tier["price"])
                    previous = end
                    if end >= quantity:
                        break
                assert previous >= quantity
            else:
                amount = quantity * Decimal(line["price"])
            total += amount
        total *= Decimal(case["multiplier"]) * Decimal(case["time_multiplier"])
        if "cap" in case:
            total = min(total, Decimal(case["cap"]))
        return format(total.quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP), "f")


def verify_files():
    schema, cap, price = load("config.schema.json"), load("model-capabilities.json"), load("price-cards.json")
    validator = jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker())
    validator.check_schema(schema)
    validator.validate(cap)
    validator.validate(price)
    passed("JSON Schema + both template documents")
    profiles = cap["profiles"]
    assert len(profiles) == 38 and len({p["profile_id"] for p in profiles}) == 38
    assert [p["model"] for p in profiles if p["provider"] == "yunfei"] == MODELS_Y
    assert [p["model"] for p in profiles if p["provider"] == "v6k"] == MODELS_V
    assert set(cap["yunfei_scenarios"]) == set(SCENARIOS)
    assert set(SCENARIOS) == {s for p in profiles if p["provider"] == "yunfei" for s in p["scenarios"]}
    assert all(not p["enabled"] and p["validation_status"] == "待渠道验证" for p in profiles)
    for m in ["00sd2.5", "LLseadanco2.5"]:
        p = next(p for p in profiles if p["model"] == m)
        assert p["duration"]["mode"] == "fixed" and p["duration"]["values"] == [30]
    assert next(p for p in profiles if p["model"] == "00sd2.5")["resolution_policy"] == "omit"
    assert next(p for p in profiles if p["model"] == "LLseadanco2.5")["materials"]["image"]["max_count"] == 30
    assert "8:1" in next(p for p in profiles if p["model"] == "gemini-3.1-flash-image-preview")["aspect_ratios"]
    assert "8:1" not in next(p for p in profiles if p["model"] == "gemini-3-pro-image-preview")["aspect_ratios"]
    passed("38 exact IDs + 19 Yunfei sections + fixed30 guards")
    assert len({c["price_id"] for c in price["cards"]}) == len(price["cards"])
    assert {ch["unit"] for c in price["cards"] for r in c["rules"] for ch in r["charges"]} == set(UNITS)
    assert all(c["status"] == "draft" and c["currency"] == "USD" for c in price["cards"])
    assert len([c for c in price["cards"] if not c["test_only"]]) == 38
    assert all(c["rules"][0]["charges"][0]["unit_price"] is None for c in price["cards"] if not c["test_only"])
    # Nullable price and explicit zero are distinct; invalid floats/negative/excess precision fail.
    sample = copy.deepcopy(price)
    component = sample["cards"][0]["rules"][0]["charges"][0]
    for value in ["0", None, "0.000000000001"]:
        component["unit_price"] = value
        validator.validate(sample)
    for value in [0, "-1", "NaN", "Infinity", "0.0000000000001"]:
        component["unit_price"] = value
        assert list(validator.iter_errors(sample)), value
    token_fields = [ch["usage_field"] for c in price["cards"] for r in c["rules"] for ch in r["charges"] if ch["unit"] == "token"]
    assert token_fields == ["image_input_tokens", "image_output_tokens"]
    passed("price templates: all8 units, unknown/free/precision, independent token fields")
    spec = yaml.safe_load((ROOT.parent / "openapi/multimodal-api.yaml").read_text(encoding="utf-8"))
    validate_openapi(spec)
    operations = [op for path in spec["paths"].values() for method, op in path.items() if method in ["get", "post", "put", "head"]]
    assert len({op["operationId"] for op in operations}) == len(operations)
    assert all(op["x-live-provider-verified"] is False for op in operations)
    for p in spec["paths"]:
        if p.startswith("/api/v1/admin"):
            assert all(op["security"] == [{"AdminSession": []}] for op in spec["paths"][p].values())
    assert not spec["paths"]["/v1/videos/{id}/content"]["head"]["responses"]["200"].get("content")
    passed(f"OpenAPI3.1 validator: {len(spec['paths'])} paths/{len(operations)} operations")
    evidence = load("source-evidence.json")
    for source in evidence["sources"]:
        data = {k: v for k, v in source.items() if k != "evidence_sha256"}
        assert hashlib.sha256(json.dumps(data, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest() == source["evidence_sha256"]
    assert [s["section"] for s in evidence["sources"][0]["sections"]] == SCENARIOS
    assert evidence["sources"][2]["document"]["info"]["version"] == "1.1.2"
    passed("source evidence SHA256 + exact sections + reference OpenAPI version")
    vectors = load("billing-vectors.json")
    for case in vectors["cases"]:
        try:
            result = formula(case)
        except ValueError as error:
            assert str(error) == case["expected_error"], case["id"]
        else:
            assert result == case["expected_total"], (case["id"], result)
    for s in vectors["ledger_scenarios"]:
        assert Decimal(s["reserved"]) == sum(Decimal(s[k]) for k in ["accepted", "final", "released"])
    # Rounding must be performed per output unless aggregate explicitly selected.
    per = dict(lines=[dict(unit="second", price="0.1", quantity="6.2", mode="ceil", step="1")], multiplier="1", time_multiplier="1")
    aggregate = copy.deepcopy(per)
    aggregate["lines"][0]["quantity"] = "12.4"
    assert Decimal(formula(per)) * 2 == Decimal("1.4") and Decimal(formula(aggregate)) == Decimal("1.3")
    passed(f"Decimal spec oracle: {len(vectors['cases'])} cases + ledger arithmetic + rounding scope")


class Mock:
    def __init__(self):
        self.mode = "normal"
        self.posts, self.gets, self.content_gets = 0, 0, 0
        self.redirect_hits, self.raw, self.headers = 0, b"", {}
        self.poll_times = []

    def reset(self, mode="normal"):
        self.mode = mode
        self.posts = self.gets = self.content_gets = self.redirect_hits = 0
        self.poll_times = []


MOCK = Mock()
MEDIA = b"\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mock-media-bytes"


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass

    def respond(self, code, data, mime="application/json", **headers):
        data = json.dumps(data).encode() if isinstance(data, dict) else data
        self.send_response(code)
        self.send_header("Content-Type", mime)
        self.send_header("Content-Length", str(len(data)))
        for key, value in headers.items():
            self.send_header(key.replace("_", "-"), value)
        self.end_headers()
        if data:
            self.wfile.write(data)

    def do_POST(self):
        MOCK.posts += 1
        MOCK.raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        MOCK.headers = {k.lower(): v for k, v in self.headers.items()}
        assert self.headers.get("Authorization") == "Bearer local-mock-key"
        if self.path == "/v1/sub2api/estimates":
            self.respond(200, {"maximum_charge": "1.00000000", "locks_price": False})
        elif self.path in ["/v1/images/generations", "/v1/images/edits", "/v1/chat/completions"] or self.path.startswith("/v1beta/models/"):
            self.respond(200, {"data": [{"mock": True}]})
        elif MOCK.mode == "html":
            self.respond(200, b"<html>wrong route</html>", "text/html")
        elif MOCK.mode == "missing_id":
            self.respond(200, {"status": "queued"})
        elif MOCK.mode == "disconnect":
            self.connection.close()
        elif MOCK.mode == "redirect":
            self.respond(302, b"", Location="http://127.0.0.1:" + str(self.server.server_port) + "/cdn")
        elif MOCK.mode == "reject":
            self.respond(400, {"error": {"code": "invalid_parameters"}})
        else:
            code = {"http200": 200, "http201": 201}.get(MOCK.mode, 202)
            status = "failed" if MOCK.mode == "failed" else "queued"
            self.respond(code, {"id": "mock/task", "status": status})

    def do_GET(self):
        if self.path == "/cdn":
            MOCK.redirect_hits += 1
            self.respond(200, MEDIA, "video/mp4")
        elif self.path == "/v1/models":
            self.respond(200, {"object": "list", "data": [{"id": "00sd2.5"}]})
        elif self.path.endswith("/content"):
            MOCK.content_gets += 1
            if MOCK.mode == "download503" and MOCK.content_gets == 1:
                self.respond(503, {})
            elif MOCK.mode == "bad_media":
                self.respond(200, b"not an MP4 document", "video/mp4")
            elif MOCK.mode == "download_redirect":
                self.respond(302, b"", Location="http://127.0.0.1:" + str(self.server.server_port) + "/cdn")
            else:
                self.respond(200, MEDIA, "video/mp4")
        else:
            assert self.path in ["/v1/videos/mock%2Ftask", "/v1/audio/generations/mock%2Ftask"], self.path
            MOCK.gets += 1
            MOCK.poll_times.append(time.monotonic())
            if MOCK.mode == "transient" and MOCK.gets == 1:
                self.respond(503, {})
            elif MOCK.mode == "retry_after" and MOCK.gets == 1:
                self.respond(429, {}, Retry_After="0.08")
            elif MOCK.mode == "unknown" and MOCK.gets == 1:
                self.respond(200, {"id": "mock/task", "status": "unknown"})
            elif MOCK.mode == "poll_failed":
                self.respond(200, {"id": "mock/task", "status": "failed"})
            elif MOCK.mode == "mismatch":
                self.respond(200, {"id": "other", "status": "completed"})
            else:
                self.respond(200, {"id": "mock/task", "status": "completed"})


def verify_clients():
    node = shutil.which("node")
    assert node, "Node >=20 required to verify Node client"
    clients = [[sys.executable, str(ROOT / "examples/media_client.py")], [node, str(ROOT / "examples/media_client.mjs")]]
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    env = {**os.environ, "SUB2API_BASE_URL": f"http://127.0.0.1:{server.server_port}", "SUB2API_API_KEY": "local-mock-key"}
    try:
        with tempfile.TemporaryDirectory(prefix="sub2api-media-spec-") as tmp:
            tmp = Path(tmp)
            request = tmp / "request.json"
            request.write_text('{"model":"00sd2.5","prompt":"科研镜头","seconds":30}', encoding="utf-8")
            counter = 0
            def run(client, *args, success=True):
                result = subprocess.run([*client, *map(str, args)], env=env, capture_output=True, text=True, encoding="utf-8", timeout=15)
                assert (result.returncode == 0) == success, (args, result.stdout, result.stderr)
                assert "local-mock-key" not in result.stdout + result.stderr
                return result
            def paths():
                nonlocal counter
                counter += 1
                return tmp / f"state-{counter}.json", tmp / f"result-{counter}.mp4"
            for ci, client in enumerate(clients):
                label = "Python" if ci == 0 else "Node"
                for mode in ["normal", "http200", "http201"]:
                    MOCK.reset(mode)
                    state, output = paths()
                    run(client, "submit", "--payload", request, "--state", state)
                    assert MOCK.posts == 1
                    run(client, "submit", "--payload", request, "--state", state, success=False)
                    assert MOCK.posts == 1
                    # Both clients read each other's state and preserve encoded task slash.
                    run(clients[1 - ci], "resume", "--state", state, "--output", output, "--poll-interval", "0.02", "--max-wait", "2")
                    assert output.read_bytes() == MEDIA and MOCK.posts == 1
                    assert json.loads(state.read_text())["download"]["sha256"] == hashlib.sha256(MEDIA).hexdigest()
                passed(label + " submit all2xx/dedup + cross-language resume/download")
                for mode in ["html", "missing_id", "disconnect", "redirect", "reject"]:
                    MOCK.reset(mode)
                    state, output = paths()
                    run(client, "submit", "--payload", request, "--state", state, success=False)
                    assert MOCK.posts == 1 and MOCK.redirect_hits == 0
                    saved = json.loads(state.read_text())
                    assert saved["status"] == ("rejected" if mode == "reject" else "unknown")
                    assert "local-mock-key" not in state.read_text()
                    run(client, "resume", "--state", state, "--output", output, success=False)
                    assert MOCK.posts == 1 and MOCK.gets == 0
                passed(label + " ambiguous POST stops; no ID/no retry/no credential redirect")
                MOCK.reset("failed")
                state, output = paths()
                run(client, "submit", "--payload", request, "--state", state, success=False)
                assert json.loads(state.read_text())["status"] == "failed"
                run(client, "resume", "--state", state, "--output", output, success=False)
                assert MOCK.gets == 0 and MOCK.content_gets == 0
                for mode in ["transient", "retry_after", "unknown", "download503", "poll_failed", "bad_media", "download_redirect", "mismatch"]:
                    MOCK.reset()
                    state, output = paths()
                    run(client, "submit", "--payload", request, "--state", state)
                    MOCK.mode = mode
                    okay = mode in ["transient", "retry_after", "unknown", "download503"]
                    run(client, "resume", "--state", state, "--output", output, "--poll-interval", "0.02", "--max-wait", "1", success=okay)
                    assert MOCK.posts == 1 and MOCK.redirect_hits == 0
                    assert output.exists() == okay
                    if mode == "retry_after":
                        assert MOCK.poll_times[1] - MOCK.poll_times[0] >= 0.075
                    if mode == "poll_failed":
                        assert MOCK.content_gets == 0
                    assert not list(tmp.glob(output.name + ".*.part"))
                passed(label + " failed200/transient/unknown/Retry-After/download integrity guards")
                MOCK.reset()
                run(client, "models")
                run(client, "estimate", "--payload", request)
                for endpoint in ["images/generations", "chat/completions", "gemini"]:
                    _, output = paths()
                    args = ["call", "--endpoint", endpoint, "--payload", request, "--output", output]
                    if endpoint == "gemini":
                        args += ["--model", "gemini-3-pro-image-preview"]
                    run(client, *args)
                    assert json.loads(output.read_text())["data"]
                img1, img2 = tmp / "first.png", tmp / "second.png"
                img1.write_bytes(b"\x89PNG\r\n\x1a\nfirst")
                img2.write_bytes(b"\x89PNG\r\n\x1a\nsecond")
                edit = tmp / "edit.json"
                edit.write_text('{"model":"gpt-image-2","prompt":"edit","n":1}', encoding="utf-8")
                _, output = paths()
                run(client, "edit", "--payload", edit, "--image", img1, "--image", img2, "--output", output)
                assert "boundary=" in MOCK.headers.get("content-type", "")
                assert MOCK.raw.count(b'name="image[]"') == 2
                assert MOCK.raw.index(b"first") < MOCK.raw.index(b"second")
                passed(label + " models/estimate/syncJSON/Gemini/multipart image order")
            assert not list(tmp.glob("*.lock")), "client locks not released"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


if __name__ == "__main__":
    verify_files()
    verify_clients()
    print(f"ALL PASSED: {len(RESULTS)} specification/client mock groups. Runtime and provider acceptance are reported separately in VALIDATION.md and REAL_CHANNEL_VALIDATION.md.")
