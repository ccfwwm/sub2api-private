"""Seed a disposable local staging DB. Never accepts a remote database target.

No generation calls. Credentials arrive in process env and are not logged.
"""
import json
import os
from pathlib import Path
import psycopg
from psycopg.types.json import Jsonb


def main():
    root = Path(os.environ["MEDIA_TEST_OUTPUT"])
    models = json.loads((root / "inference-models.json").read_text(encoding="utf-8"))
    keys = json.loads(os.environ["MEDIA_INFERENCE_KEYS"])
    profiles = json.loads(Path("docs/multimodal/model-capabilities.json").read_text(encoding="utf-8"))["profiles"]
    with psycopg.connect("host=127.0.0.1 port=15438 dbname=media_staging user=postgres password=media-isolated-test-only") as conn:
        cur = conn.cursor()
        cur.execute("SELECT id FROM users WHERE email=%s", ("media-staging@example.invalid",))
        user = cur.fetchone()[0]
        cur.execute("UPDATE users SET balance=1000 WHERE id=%s", (user,))
        bindings, metadata = {}, []
        for entry in models:
            provider, group = entry["provider"], entry["group"]
            label = provider + ":" + group
            if label not in keys or entry["http"] != 200:
                continue
            name = "media-audit:" + label
            cur.execute("SELECT id FROM groups WHERE name=%s", (name,))
            row = cur.fetchone()
            if row:
                group_id = row[0]
            else:
                cur.execute("INSERT INTO groups(name,platform) VALUES(%s,'openai') RETURNING id", (name,))
                group_id = cur.fetchone()[0]
            cur.execute("INSERT INTO user_allowed_groups(user_id,group_id) VALUES(%s,%s) ON CONFLICT DO NOTHING", (user, group_id))
            cur.execute("SELECT id FROM accounts WHERE name=%s", (name,))
            row = cur.fetchone()
            if row:
                account_id = row[0]
            else:
                cur.execute("INSERT INTO accounts(name,platform,type,credentials,schedulable) VALUES(%s,'openai','apikey',%s,true) RETURNING id",
                            (name, Jsonb({"base_url": "https://img.yunfei.best" if provider == "yunfei" else "https://www.v6k.top", "api_key": keys[label]})))
                account_id = cur.fetchone()[0]
                cur.execute("INSERT INTO account_groups(account_id,group_id) VALUES(%s,%s)", (account_id, group_id))
            cur.execute("SELECT id,key FROM api_keys WHERE user_id=%s AND group_id=%s AND name='media-audit'", (user, group_id))
            row = cur.fetchone()
            if row:
                key_id, local_key = row
            else:
                import secrets
                local_key = "sk-media-audit-" + secrets.token_hex(24)
                cur.execute("INSERT INTO api_keys(user_id,key,name,group_id) VALUES(%s,%s,'media-audit',%s) RETURNING id", (user, local_key, group_id))
                key_id = cur.fetchone()[0]
            bindings[label] = {"group_id": group_id, "account_id": account_id, "local_key": local_key, "key_id": key_id,
                               "models": entry["models"]}
        for profile in profiles:
            matches = [(label, b) for label, b in bindings.items() if label.startswith(profile["provider"] + ":") and profile["model"] in b["models"]]
            # Prefer the lowest image group for the first pilot; other image groups
            # can use another isolated profile ID after the basic path passes.
            if not matches:
                metadata.append({"model": profile["model"], "provider": profile["provider"], "state": "not_in_authorized_model_discovery"})
                continue
            label, binding = matches[0]
            profile["account_ids"] = [binding["account_id"]]
            profile["group_ids"] = [binding["group_id"]]
            profile["enabled"] = True
            profile["status"] = "enabled"
            profile["version"] = "1"
            profile["verification_evidence"] = "isolated staging pilot only; not verified and never export as published production config"
            profile["limits"] = {"max_request_bytes": 100 * 1024 * 1024, "max_concurrent_tasks": 2, "observation_seconds": 7200}
            for material in profile["materials"].values():
                if material["max_count"] is None:
                    material["max_count"] = 4  # Narrow staging pilot, not a supplier claim.
                if material["max_file_bytes"] is None:
                    material["max_file_bytes"] = 30 * 1024 * 1024
                if material["max_duration_seconds"] is None:
                    material["max_duration_seconds"] = "30"
            if profile["duration"]["mode"] == "unknown":
                profile["duration"].update(mode="fixed", values=[5], default=5)
            if profile["adapter"] == "yunfei_gemini_native":
                profile["materials"]["image"]["formats"] = ["data_url", "public_url_string"]
            profile["usage_fields"] = {"provider_tokens.total_tokens": "usage.total_tokens"}
            cur.execute("INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES('capability',%s,1,'enabled',%s,%s) ON CONFLICT DO NOTHING", (profile["profile_id"], Jsonb(profile), user))
            cur.execute("INSERT INTO media_config_heads(kind,id,version,published_version) VALUES('capability',%s,1,1) ON CONFLICT DO NOTHING", (profile["profile_id"],))
            for operation in profile["operations"]:
                price_id = "audit-price:" + profile["profile_id"] + ":" + operation
                card = {"price_id": price_id, "version": "1", "purpose": "sale", "status": "enabled", "currency": "USD",
                        "test_only": True, "media_type": profile["media_type"], "operation": operation,
                        "model_match": {"kind": "exact", "value": profile["model"]}, "scope": {"kind": "group", "reference": str(binding["group_id"])},
                        "rules": [{"priority": 100, "when": {}, "charges": [{"name": "audit", "unit": "request", "unit_price": "1",
                        "quantity_source": "task_count", "trigger": "success", "refundable": True, "method": "linear",
                        "rounding": {"mode": "none", "step": "1", "minimum": "0", "scope": "aggregate"}}]}],
                        "reservation": {"maximum_charge": "1", "max_quantities": {}}, "success_policy": "any_output", "multiplier_policy": "effective_media_then_time"}
                cur.execute("INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES('price',%s,1,'enabled',%s,%s) ON CONFLICT DO NOTHING", (price_id, Jsonb(card), user))
                cur.execute("INSERT INTO media_config_heads(kind,id,version,published_version) VALUES('price',%s,1,1) ON CONFLICT DO NOTHING", (price_id,))
            metadata.append({"model": profile["model"], "provider": profile["provider"], "group": label, "state": "staging_only_unverified"})
        (root / "staging-profiles.json").write_text(json.dumps(metadata, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(bindings, ensure_ascii=True))  # Captured and DPAPI-encrypted by caller.


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # Never echo SQL parameters or provider credentials from driver errors.
        raise SystemExit("Isolated seed failed: " + type(error).__name__) from None
