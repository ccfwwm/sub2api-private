"""Provision limited keys for this authorized audit, without exposing secrets.

Output key material through stdout ONLY for the PowerShell caller to encrypt;
status messages go to stderr. Never run directly in a logged terminal.
"""
import json
import os
import sys
import time
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import Request, urlopen


def main():
    root = Path(os.environ["MEDIA_TEST_OUTPUT"])
    discovery = json.loads((root / "provider-discovery.json").read_text(encoding="utf-8"))
    secrets, receipts = {}, []
    # Combined key quotas = USD 37.50, below the authorized USD 50 ceiling.
    allocations = {"yunfei": [("image2 1k", 3), ("image2 2k 4k", 3),
                               ("\u9999\u8549\u751f\u56fe", 3), ("grok\u89c6\u9891", 6),
                               ("Gemini\u89c6\u9891", 7), ("\u97f3\u9891", 3)],
                   "v6k": [("default", 12.5)]}
    for provider, groups in allocations.items():
        p = discovery[provider]
        token = os.environ["MEDIA_" + provider.upper() + "_ACCESS"]
        headers = {"Authorization": "Bearer " + token, "New-Api-User": str(p["user"]["id"]), "Content-Type": "application/json"}

        def call(path, method="GET", body=None):
            req = Request(p["base"] + path, headers=headers, method=method,
                          data=None if body is None else json.dumps(body).encode())
            try:
                with urlopen(req, timeout=25) as response:
                    return json.load(response)
            except HTTPError as error:
                return {"success": False, "message": "HTTP " + str(error.code)}

        for index, (group, dollars) in enumerate(groups):
            name = "sub2api-audit-20261012-" + str(index)
            result = call("/api/token/search?keyword=" + quote(name) + "&size=100")
            data = result.get("data", [])
            records = data if isinstance(data, list) else data.get("items", [])
            exact = [record for record in records if record.get("name") == name]
            if not exact:
                result = call("/api/token/", "POST", {"name": name, "remain_quota": int(dollars * 500000),
                    "expired_time": int(time.time()) + 14 * 86400, "unlimited_quota": False,
                    "model_limits_enabled": False, "model_limits": "", "group": group, "status": 1})
                if result.get("success") is not True:
                    print(json.dumps({"provider": provider, "group": group, "created": False,
                                      "message": str(result.get("message", "Unknown error")).replace(token, "[REDACTED]")[:180]}), file=sys.stderr)
                    continue
                result = call("/api/token/search?keyword=" + quote(name) + "&size=100")
                data = result.get("data", [])
                records = data if isinstance(data, list) else data.get("items", [])
                exact = [record for record in records if record.get("name") == name]
            if len(exact) != 1:
                print(json.dumps({"provider": provider, "group": group, "error": "Cannot identify unique test token"}), file=sys.stderr)
                continue
            record = exact[0]
            result = call("/api/token/" + str(record["id"]) + "/key", "POST", {})
            key = result.get("data", {}).get("key") if isinstance(result.get("data"), dict) else result.get("data")
            if not key and record.get("key") and "*" not in record["key"]:
                key = record["key"]
            if not key:
                print(json.dumps({"provider": provider, "group": group, "error": "Token key retrieval unavailable"}), file=sys.stderr)
                continue
            if not key.startswith("sk-"):
                key = "sk-" + key
            secrets[provider + ":" + group] = key
            receipts.append({"provider": provider, "group": group, "token_id": record["id"], "budget_usd": str(dollars),
                             "used_quota": record.get("used_quota", 0), "remaining_quota": record.get("remain_quota")})
            print(json.dumps({"provider": provider, "group": group, "test_key_ready": True, "limit_usd": dollars}), file=sys.stderr)
    (root / "test-key-receipts.json").write_text(json.dumps(receipts, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(secrets, ensure_ascii=True))


if __name__ == "__main__":
    main()
