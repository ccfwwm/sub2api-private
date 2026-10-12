"""Durable, budgeted single-generation pilot through isolated Sub2API.

Keys enter via env. One submission only; persist the result before polling.
Never retries an uncertain submission, and records actual provider quota deltas.
"""
import argparse
import hashlib
import json
import os
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def token_usage(root, provider, label):
    """Read only this audit key's accounting, never return key material."""
    receipts = json.loads((root / "test-key-receipts.json").read_text(encoding="utf-8"))
    record = next(r for r in receipts if provider + ":" + r["group"] == label)
    from probe_media_providers import request
    base, user = ("https://img.yunfei.best", 20) if provider == "yunfei" else ("https://www.v6k.top", 79)
    code, data = request(base, "/api/token/" + str(record["token_id"]), os.environ["MEDIA_" + provider.upper() + "_ACCESS"], user)
    if code != 200 or data.get("success") is not True:
        raise RuntimeError("Cannot read dedicated test key accounting")
    token = data["data"]
    return {"used_quota": token["used_quota"], "remaining_quota": token["remain_quota"], "observed_at": time.time()}


def call(base, path, key, body=None, method=None, headers=None):
    request = Request(base + path, data=None if body is None else (body if isinstance(body, bytes) else json.dumps(body).encode()),
                      method=method or ("GET" if body is None else "POST"),
                      headers={"Authorization": "Bearer " + key, "Content-Type": "application/json", **(headers or {})})
    try:
        with urlopen(request, timeout=195) as response:
            raw = response.read()
            try:
                result = json.loads(raw)
            except (ValueError, UnicodeDecodeError):
                if response.headers.get("Content-Type", "").startswith("text/event-stream"):
                    events = [json.loads(line[6:]) for line in raw.decode().splitlines() if line.startswith("data: ") and line[6:] != "[DONE]"]
                    result = {"id": next((event.get("id") for event in events if event.get("id")), None), "sse_events": len(events), "sse_done": "data: [DONE]" in raw.decode()}
                else:
                    result = {"binary_bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}
            return response.status, result, dict(response.headers)
    except HTTPError as error:
        try:
            result = json.load(error)
        except Exception:
            result = {"error": "Non-JSON error"}
        return error.code, result, dict(error.headers)
    except (URLError, TimeoutError):
        return 0, {"error": "Transport uncertain; never resubmit"}, {}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--provider", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--case", required=True)
    parser.add_argument("--operation", default="video")
    parser.add_argument("--seconds", type=int)
    parser.add_argument("--resolution")
    parser.add_argument("--group")
    parser.add_argument("--quality")
    parser.add_argument("--size", default="1024x1024")
    parser.add_argument("--payload-file", type=Path)
    parser.add_argument("--multipart-image", type=Path, action="append", default=[])
    parser.add_argument("--maximum-cost", type=str, required=True)
    args = parser.parse_args()
    from decimal import Decimal
    root = Path(os.environ["MEDIA_TEST_OUTPUT"])
    bindings = json.loads(os.environ["MEDIA_STAGING_BINDINGS"])
    ledger_path = root / "real-test-ledger.json"
    ledger = json.loads(ledger_path.read_text(encoding="utf-8")) if ledger_path.exists() else {"budget_usd": "50", "cases": {}}
    outstanding = sum(Decimal(value["maximum_cost_usd"]) for value in ledger["cases"].values())
    existing = ledger["cases"].get(args.case)
    if existing and existing["state"] == "submitting" and not existing.get("task_id"):
        existing["state"] = "unknown"
        ledger_path.write_text(json.dumps(ledger, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"case": args.case, "state": "unknown", "message": "Interrupted submission requires reconciliation; no retry"}))
        return
    if existing and existing["state"] in ("completed", "failed", "unknown", "rejected"):
        print(json.dumps({"case": args.case, "state": existing["state"], "message": "Durable result exists; no new submission"}))
        return
    if not existing and outstanding + Decimal(args.maximum_cost) > Decimal(ledger["budget_usd"]):
        raise SystemExit("Budget ceiling reached")
    matches = [(label, binding) for label, binding in bindings.items() if label.startswith(args.provider + ":") and args.model in binding["models"] and (not args.group or label.endswith(":" + args.group))]
    if not matches:
        raise SystemExit("Model not in authorized inference discovery")
    label, binding = matches[0]
    key = binding["local_key"]
    payload = {"model": args.model, "prompt": "A quiet blue mountain lake at sunrise, gentle clouds, scientific nature documentary, no people, no text"}
    if args.seconds:
        payload["seconds"] = args.seconds
    if args.resolution:
        payload["resolution"] = args.resolution
    if args.operation == "image":
        payload.update(n=1, size=args.size, response_format="url")
        if args.quality: payload["quality"] = args.quality
    if args.operation == "gemini_image":
        payload = {"model":args.model,"contents":[{"role":"user","parts":[{"text":payload["prompt"]}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"],"imageConfig":{"imageSize":"1K","aspectRatio":"1:1"}}}
    if args.operation == "chat_music":
        payload = {"model": args.model, "messages": [{"role": "user", "content": "A short peaceful instrumental piano study, no lyrics"}], "stream": False}
    if args.payload_file:
        payload = json.loads(args.payload_file.read_text(encoding="utf-8"))
    base = "http://127.0.0.1:8085"
    def summarized(value):
        if isinstance(value, dict):
            return {k: summarized(v) for k, v in value.items()}
        if isinstance(value, list):
            return [summarized(v) for v in value]
        if isinstance(value, str) and (len(value) > 1000 or value.startswith("data:")):
            return {"length": len(value), "sha256": hashlib.sha256(value.encode()).hexdigest()}
        return value

    case = existing or {"provider": args.provider, "model": args.model, "group": label, "maximum_cost_usd": args.maximum_cost,
                        "state": "prepared", "payload": summarized(payload), "created_at": time.time()}
    ledger["cases"][args.case] = case

    def save():
        temp = ledger_path.with_suffix(".tmp")
        temp.write_text(json.dumps(ledger, ensure_ascii=False, indent=2), encoding="utf-8")
        temp.replace(ledger_path)

    save()
    if not case.get("task_id"):
        case["quota_before"] = token_usage(root, args.provider, label)
        save()
        estimate_payload = dict(payload)
        if args.multipart_image:
            import base64
            estimate_payload["images"] = ["data:image/png;base64," + base64.b64encode(image.read_bytes()).decode() for image in args.multipart_image]
        code, estimate, _ = call(base, "/v1/sub2api/estimates", key, {"operation": args.operation, "request": estimate_payload})
        case["estimate_http"] = code
        case["estimate"] = estimate
        if code != 200:
            case["state"] = "rejected"
            save()
            print(json.dumps({"case": args.case, "state": case["state"], "http": code, "error": estimate.get("error")}, ensure_ascii=True))
            return
        path = {"video": "/v1/videos", "image": "/v1/images/generations", "image_edit": "/v1/images/edits", "audio": "/v1/audio/generations", "music": "/v1/music/generations", "chat_music": "/v1/chat/completions", "gemini_image":"/v1beta/models/"+args.model+":generateContent"}[args.operation]
        case["state"] = "submitting"
        save()
        submit_body = payload
        submit_headers = {"Idempotency-Key": "audit-20261012:" + args.case}
        if args.multipart_image:
            import uuid
            boundary = "media-audit-" + uuid.uuid4().hex
            parts = []
            for field, value in payload.items():
                raw = json.dumps(value) if isinstance(value, (list, dict)) else str(value)
                parts.append((f'--{boundary}\r\nContent-Disposition: form-data; name="{field}"\r\n\r\n{raw}\r\n').encode())
            for image in args.multipart_image:
                parts.append((f'--{boundary}\r\nContent-Disposition: form-data; name="image"; filename="reference.png"\r\nContent-Type: image/png\r\n\r\n').encode() + image.read_bytes() + b'\r\n')
            submit_body = b''.join(parts) + f'--{boundary}--\r\n'.encode()
            submit_headers["Content-Type"] = "multipart/form-data; boundary=" + boundary
        code, result, _ = call(base, path, key, submit_body, headers=submit_headers)
        case["submit_http"] = code
        # Result may contain huge Base64. Retain identity and billing, not payload bytes.
        candidate = result.get("id") or result.get("task_id")
        case["task_id"] = candidate if isinstance(candidate,str) and candidate.startswith("mt_") else None
        case["state"] = "polling" if case["task_id"] else ("unknown" if code == 0 or code >= 500 else "rejected")
        case["submit_summary"] = {k: result.get(k) for k in ("id", "status", "error", "billing") if k in result}
        if "sse_events" in result:
            case["sse"] = {k: result[k] for k in ("sse_events", "sse_done")}
        save()
        print(json.dumps({"case": args.case, "submitted": bool(case["task_id"]), "http": code, "id": case["task_id"], "state": case["state"]}, ensure_ascii=True), flush=True)
    if not case.get("task_id"):
        return
    stop_at=time.monotonic()+1200
    while time.monotonic()<stop_at:
        code, task, _ = call(base, "/v1/videos/" + case["task_id"], key)
        if code == 200:
            case["task"] = task
            state = task.get("status")
            settlement = task.get("billing", {}).get("state")
            if state in ("failed", "unknown") or settlement in ("settled", "released"):
                case["state"] = state
                case["quota_after"] = token_usage(root, args.provider, label)
                before, after = case["quota_before"], case["quota_after"]
                case["observed_cost_usd"] = str(Decimal(after["used_quota"] - before["used_quota"]) / 500000)
                case["cost_status"] = "dedicated_key_quota_delta"
                if state == "completed":
                    status, download, headers = call(base, "/v1/videos/" + case["task_id"] + "/content", key)
                    case["download"] = {"http": status, "mime": headers.get("Content-Type"), **download}
                    status, _, headers = call(base, "/v1/videos/" + case["task_id"] + "/content", key, method="HEAD")
                    case["head"] = {"http": status, "length": headers.get("Content-Length")}
                    status, ranged, _ = call(base, "/v1/videos/" + case["task_id"] + "/content", key, headers={"Range": "bytes=0-1023"})
                    case["range"] = {"http": status, **ranged}
                save()
                print(json.dumps({"case": args.case, "state": case["state"], "settlement": settlement,
                                  "download_http": case.get("download", {}).get("http"), "head_http": case.get("head", {}).get("http"),
                                  "range_http": case.get("range", {}).get("http"), "quantities": task.get("billing", {}).get("quantities")}, ensure_ascii=True), flush=True)
                return
        case["last_poll_http"] = code
        case["last_poll_at"] = time.time()
        save()
        time.sleep(10)
    print(json.dumps({"case":args.case,"state":"polling","message":"Observation window ended; task remains durable and may be resumed"}),flush=True)


if __name__ == "__main__":
    main()
