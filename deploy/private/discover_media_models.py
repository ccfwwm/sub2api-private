"""Non-billable discovery with dedicated, quota-limited inference keys."""
import json
import os
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

root = Path(os.environ["MEDIA_TEST_OUTPUT"])
keys = json.loads(os.environ["MEDIA_INFERENCE_KEYS"])
results = []
for label, key in keys.items():
    provider, group = label.split(":", 1)
    base = "https://img.yunfei.best" if provider == "yunfei" else "https://www.v6k.top"
    request = Request(base + "/v1/models", headers={"Authorization": "Bearer " + key})
    try:
        with urlopen(request, timeout=30) as response:
            data = json.load(response)
            entry = {"provider": provider, "group": group, "http": response.status,
                     "models": [m["id"] for m in data.get("data", [])]}
    except HTTPError as error:
        entry = {"provider": provider, "group": group, "http": error.code, "models": []}
    results.append(entry)
    print(json.dumps(entry, ensure_ascii=True))
(root / "inference-models.json").write_text(json.dumps(results, ensure_ascii=False, indent=2), encoding="utf-8")
