"""Non-generating v1.1.2 acceptance checks against loopback staging only.

Credentials arrive through env. Never exports account credentials or raw responses.
"""
import copy
import json
import os
from urllib.error import HTTPError
from urllib.request import Request, urlopen

BASE = "http://127.0.0.1:8085"


def call(path, token, body=None, method=None):
    req = Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                  method=method or ("GET" if body is None else "POST"),
                  headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    try:
        with urlopen(req, timeout=60) as response:
            return response.status, json.load(response)
    except HTTPError as err:
        return err.code, json.load(err)


def main():
    bindings = json.loads(os.environ["MEDIA_STAGING_BINDINGS"])
    jwt = os.environ["MEDIA_STAGING_JWT"]
    label, binding = next((label, b) for label, b in bindings.items() if label == "yunfei:image2 1k")
    group = binding["group_id"]
    account = binding["account_id"]
    key = binding["local_key"]
    code, data = call(f"/api/v1/admin/accounts/{account}", jwt)
    assert code == 200, "staging admin authentication failed"
    capabilities = data["data"]["extra"]["media_models"]
    code, result = call(f"/api/v1/admin/accounts/{account}/models/sync-upstream", jwt, {})
    assert code == 200 and "gpt-image-2" in result["data"]["models"], "upstream sync failed"
    code, data = call(f"/api/v1/admin/accounts/{account}", jwt)
    assert code == 200 and data["data"]["extra"]["media_models"] == capabilities, "sync changed capability configuration"
    code, data = call(f"/api/v1/admin/groups/{group}", jwt)
    assert code == 200
    original = data["data"]["model_pricing"]
    request = {"operation": "image", "request": {"model": "gpt-image-2", "prompt": "test", "size": "1024x1024", "n": 1}}
    channel_id = None
    try:
        code, result = call("/v1/sub2api/estimates", key, request)
        assert code == 200 and result["calculation"]["total"] == "1.00000000", "initial estimate differs"
        configured = copy.deepcopy(original)
        image_price = next(p for p in configured if "gpt-image-2" in p["models"])
        image_price["media"]["rules"][0]["charges"][0]["unit_price"] = "0"
        image_price["media"]["reservation"]["maximum_charge"] = "0"
        code, _ = call(f"/api/v1/admin/groups/{group}", jwt, {"model_pricing": configured}, "PUT")
        assert code == 200, "manual media group pricing did not save"
        code, result = call("/v1/sub2api/estimates", key, request)
        assert code == 200 and result["calculation"]["total"] == "0.00000000", "explicit zero not preserved"
        free_version = result["price_version"]
        code, data = call(f"/api/v1/admin/groups/{group}", jwt)
        saved = next(p for p in data["data"]["model_pricing"] if "gpt-image-2" in p["models"])
        assert saved["media"] == image_price["media"], "pricing roundtrip lost fields"
        channel_price = copy.deepcopy(image_price)
        channel_price["media"]["rules"][0]["charges"][0]["unit_price"] = "2"
        channel_price["media"]["reservation"]["maximum_charge"] = "2"
        cost_price = copy.deepcopy(channel_price)
        cost_price["media"]["rules"][0]["charges"][0]["unit_price"] = "0.25"
        cost_price["media"]["reservation"]["maximum_charge"] = "0.25"
        cost_rule = {"name":"isolated cost", "sort_order":0, "group_ids":[group], "account_ids":[account], "pricing":[cost_price]}
        code, data = call("/api/v1/admin/channels", jwt, {"name":"v112-isolated-price-check", "status":"active", "group_ids":[group], "model_pricing":[channel_price], "account_stats_pricing_rules":[cost_rule]})
        assert code in (200, 201), "channel media pricing did not save"
        channel_id = data["data"]["id"]
        code, result = call("/v1/sub2api/estimates", key, request)
        assert code == 200 and result["calculation"]["total"] == "0.00000000", "channel overrode free group price"
        code, _ = call(f"/api/v1/admin/groups/{group}", jwt, {"model_pricing": []}, "PUT")
        assert code == 200
        code, result = call("/v1/sub2api/estimates", key, request)
        assert code == 200 and result["calculation"]["total"] == "2.00000000", "channel fallback missing"
        assert result["price_version"] != free_version
        code, data = call(f"/api/v1/admin/channels/{channel_id}", jwt)
        assert code == 200 and data["data"]["model_pricing"][0]["media"] == channel_price["media"]
        assert data["data"]["account_stats_pricing_rules"][0]["pricing"][0]["media"] == cost_price["media"], "cost pricing roundtrip lost fields"
        code, _ = call(f"/api/v1/admin/channels/{channel_id}", jwt, method="DELETE")
        assert code == 200
        channel_id = None
        code, _ = call("/v1/sub2api/estimates", key, request)
        assert code == 422, "unpriced generation accepted"
        code, result = call("/v1/models", key)
        assert code == 200 and result["data"] == [], "model discovery leaked unpriced models"
        invalid = copy.deepcopy(original)
        invalid[0]["media"]["rules"][0]["charges"][0]["unit_price"] = None
        code, _ = call(f"/api/v1/admin/groups/{group}", jwt, {"model_pricing":invalid}, "PUT")
        assert code == 400, "unset price allowed"
        print(json.dumps({"suite":"unified-media-staging", "passed":["provider_model_sync_preserves_capabilities", "group_price_roundtrip", "explicit_free", "channel_price_roundtrip", "group_over_channel", "missing_price_rejected", "unpriced_models_hidden", "price_version_change"]}))
    finally:
        call(f"/api/v1/admin/groups/{group}", jwt, {"model_pricing": original}, "PUT")
        if channel_id is not None:
            call(f"/api/v1/admin/channels/{channel_id}", jwt, method="DELETE")


if __name__ == "__main__":
    main()
