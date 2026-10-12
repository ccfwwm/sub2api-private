"""Read-only New API discovery; credentials enter only through process env.

Never logs token records, Authorization headers, or unfiltered user records.
"""
import json
import os
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def request(base, path, token=None, user=None):
    headers = {"Accept": "application/json", "User-Agent": "Sub2API-compatibility-audit/1"}
    if token:
        headers["Authorization"] = "Bearer " + token
    if user:
        headers["New-Api-User"] = str(user)
    try:
        with urlopen(Request(base + path, headers=headers), timeout=30) as response:
            return response.status, json.load(response)
    except HTTPError as error:
        try:
            data = json.load(error)
        except Exception:
            data = {"message": "Non-JSON error response"}
        return error.code, data
    except (URLError, TimeoutError, ValueError):
        return 0, {"message": "Connection or response decoding failed"}


def main():
    inventory = {}
    for name, base, env, default_id in [
        ("yunfei", "https://img.yunfei.best", "MEDIA_YUNFEI_ACCESS", None),
        ("v6k", "https://www.v6k.top", "MEDIA_V6K_ACCESS", 79),
    ]:
        token = os.environ[env]
        code, data = request(base, "/api/user/self", token, default_id)
        user = data.get("data") or {}
        user_id = user.get("id", default_id)
        if data.get("success") is not True and user_id:
            code, data = request(base, "/api/user/self", token, user_id)
            user = data.get("data") or {}
        info = {"base": base, "auth_http": code, "authenticated": data.get("success") is True,
                "user": {key: user.get(key) for key in ("id", "username", "group", "quota", "used_quota")}}
        if not info["authenticated"]:
            msg = str(data.get("message", "Authentication failed"))
            info["message"] = msg.replace(token, "[REDACTED]")[:180]
        else:
            for path, label in [("/api/status", "status"), ("/api/pricing", "pricing"),
                                ("/v1/models", "models"), ("/api/token/?p=0&size=10", "token_inventory")]:
                status, body = request(base, path, token, user_id)
                entry = {"http": status, "success": body.get("success"), "keys": list(body)[:12]}
                raw = body.get("data")
                if label == "status" and isinstance(raw, dict):
                    entry["data"] = {k: raw[k] for k in ("quota_per_unit", "display_in_currency", "usd_exchange_rate", "price", "currency", "version") if k in raw}
                elif label == "token_inventory":
                    records = raw if isinstance(raw, list) else (raw or {}).get("items", [])
                    entry["tokens"] = [{k: item.get(k) for k in ("id", "name", "status", "remain_quota", "used_quota", "unlimited_quota", "group", "models", "models_enabled")} for item in records]
                elif label == "models" and isinstance(raw, list):
                    entry["models"] = [i.get("id") for i in raw if isinstance(i, dict)]
                elif label == "pricing":
                    entry["data"] = raw
                    for k in ("group_ratio", "usable_group", "auto_groups", "supported_endpoint"):
                        if k in body:
                            entry[k] = body[k]
                if "message" in body:
                    entry["message"] = str(body["message"]).replace(token, "[REDACTED]")[:180]
                info[label] = entry
        inventory[name] = info
        print(json.dumps({"provider": name, "http": info["auth_http"], "authenticated": info["authenticated"],
                          "user_id": info["user"].get("id"), "quota": info["user"].get("quota"),
                          "discovery": {k: {"http": v["http"], "records": len(v.get("models", v.get("tokens", v.get("data", [])))) if isinstance(v.get("models", v.get("tokens", v.get("data", []))), list) else None} for k, v in info.items() if isinstance(v, dict) and "http" in v},
                          "message": info.get("message")}, ensure_ascii=False))
    output = Path(os.environ["MEDIA_TEST_OUTPUT"]) / "provider-discovery.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(inventory, ensure_ascii=False, indent=2), encoding="utf-8")


if __name__ == "__main__":
    main()
