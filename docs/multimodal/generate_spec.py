"""Regenerate design artifacts only. No network, runtime config, or production writes."""
import copy
import json
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parent
DRAFT = "1.0.0-draft"
SOURCE_DATE = "2026-10-12"
YUNFEI = "https://doc.yunfei.best/"
V6K = "https://www.v6k.top/docs#video"
SCENARIOS = ["gpt-text", "gpt-edit", "banana-text", "banana-edit", "grok-video-submit",
             "grok-video-image", "grok-video-result", "omni-video-submit", "omni-video-image",
             "omni-video-result", "veo-text", "veo-image", "veo-frames", "veo-ref", "audio-chat",
             "audio-submit", "audio-compat", "audio-models", "audio-endpoints"]
MODELS_Y = ["gpt-image-2", "gemini-3-pro-image-preview", "gemini-3.1-flash-image-preview",
            "grok-imagine-video", "grok-imagine-video-1.5", "omni-flash", "veo-3.1-fast",
            "veo-3.1-lite", "veo-3.1-quality", "veo-3.1-fast-generate-preview",
            "veo-3.1-generate-preview", "veo-3.1-generate-preview-ref", "lyria-3.5", "lyria-3-pro"]
MODELS_V = ["00sd2.0", "00sd2.5", "XXsedanco2.0", "XXsedanco2.0mini", "XXsedanco2.0S",
            "XXsedanco2.5S", "wan3.0", "seedance-mini", "seadanco-oplu-480", "seadanco-oplu-720",
            "sedanco-2.5-AM", "dola2.5", "video-QY", "doubao-seedance-2-0-65Z",
            "doubao-seedance-2-0-fast-65Z", "doubao-seedance-2-0-mini-65Z", "doubao-seedance-2-5/65Z",
            "JSadanco2.0", "LLseadanco2.5", "F720seadanco2.5", "F480seadanco2.5",
            "FF720wan3.0", "FF1080wan3.0", "MinimaxH3-2K"]


def obj(properties, required=(), **extra):
    return {"type": "object", "properties": properties, "required": list(required),
            "additionalProperties": False, **extra}


def arr(items):
    return {"type": "array", "items": items}


def enum(*values):
    return {"type": "string", "enum": list(values)}


def ref(name):
    return {"$ref": "#/$defs/" + name}


TEXT = {"type": "string", "minLength": 1}
DECIMAL = {"type": "string", "pattern": r"^(0|[1-9][0-9]*)(\.[0-9]+)?$"}
PRICE = {"type": ["string", "null"], "pattern": r"^(0|[1-9][0-9]*)(\.[0-9]{1,12})?$"}
NULL_INT = {"type": ["integer", "null"], "minimum": 0}
NULL_LIST = {"type": ["array", "null"], "items": TEXT, "uniqueItems": True}
UNITS = ["request", "output", "second", "minute", "token", "input_image", "input_video_second", "input_audio_second"]
QUANTITIES = ["task_count", "successful_outputs", "request_duration", "fixed_duration", "artifact_duration",
              "provider_duration", "provider_tokens", "accepted_input_images", "accepted_input_video_duration",
              "accepted_input_audio_duration"]

DEFS = {
    "Duration": obj({"mode": enum("enum", "range", "fixed", "hint", "unknown", "not_applicable"),
                     "values": arr({"type": "integer", "minimum": 1}), "min": NULL_INT, "max": NULL_INT,
                     "default": NULL_INT, "required": {"type": "boolean"},
                     "scenarios": arr({"type": "object"})}, ["mode", "values", "min", "max", "default", "required", "scenarios"]),
    "Material": obj({"formats": arr(TEXT), "max_count": NULL_INT, "max_file_bytes": NULL_INT,
                     "max_duration_seconds": {"type": ["string", "null"], "pattern": DECIMAL["pattern"]}},
                    ["formats", "max_count", "max_file_bytes", "max_duration_seconds"]),
    "Capability": obj({
        "profile_id": TEXT, "version": TEXT, "model": TEXT, "upstream_model": TEXT,
        "provider": enum("yunfei", "v6k"), "adapter": TEXT, "media_type": enum("image", "video", "audio"),
        "enabled": {"const": False}, "status": {"const": "draft"}, "validation_status": {"const": "待渠道验证"},
        "source": TEXT, "scenarios": arr(TEXT), "operations": arr(TEXT), "formats": arr(TEXT),
        "duration": ref("Duration"), "resolutions": NULL_LIST, "sizes": NULL_LIST, "aspect_ratios": NULL_LIST,
        "qualities": NULL_LIST, "resolution_policy": enum("enum", "omit", "unknown", "native"),
        "materials": obj({k: ref("Material") for k in ["image", "video", "audio"]}, ["image", "video", "audio"]),
        "field_mapping": {"type": "object", "additionalProperties": TEXT},
        "constraints": arr({"type": "object"}), "unverified": arr(TEXT),
        "delivery": obj({"poll_seconds": {"type": "integer", "minimum": 1}, "head": {"type": ["boolean", "null"]},
                         "range": {"type": ["boolean", "null"]}, "refresh": {"type": ["boolean", "null"]}},
                        ["poll_seconds", "head", "range", "refresh"]),
        # max_concurrent_tasks remains a legacy snapshot field. Media runtime
        # admission is owned by the bound account's concurrency setting.
        "limits": obj({"max_request_bytes": NULL_INT, "max_concurrent_tasks": NULL_INT,
                       "observation_seconds": NULL_INT}, ["max_request_bytes", "observation_seconds"]),
        "account_ids": arr({"type":"integer","minimum":1}), "group_ids": arr({"type":"integer","minimum":1}),
        "channel_id":{"type":"integer","minimum":1}, "price_templates":arr(TEXT), "cost_templates":arr(TEXT),
        "music_rate_override":PRICE, "verification_evidence":TEXT, "artifact_hosts":arr(TEXT),
        "usage_fields":{"type":"object","additionalProperties":{"type":"string","pattern":r"^(usage|usageMetadata|billing_usage)\.[A-Za-z_][A-Za-z0-9_.]*$"}}
    }, ["profile_id", "version", "model", "upstream_model", "provider", "adapter", "media_type", "enabled", "status",
        "validation_status", "source", "scenarios", "operations", "formats", "duration", "resolutions", "aspect_ratios",
        "qualities", "resolution_policy", "materials", "field_mapping", "constraints", "unverified", "delivery", "limits"]),
    "Rounding": obj({"mode": enum("none", "ceil", "floor", "half_up"), "step": DECIMAL, "minimum": DECIMAL,
                     "scope": enum("per_output", "aggregate")}, ["mode", "step", "minimum", "scope"]),
    "Charge": obj({"name": TEXT, "unit": enum(*UNITS), "unit_price": PRICE,
                   "quantity_source": enum(*QUANTITIES), "fallback_source": {"type": ["string", "null"], "enum": QUANTITIES + [None]},
                   "usage_field": {"type": ["string", "null"]},
                   "trigger": enum("success", "accepted"), "refundable": {"type": "boolean"},
                   "rounding": ref("Rounding"), "method": enum("linear", "package", "progressive"),
                   "package_quantity": {"type": ["string", "null"], "pattern": DECIMAL["pattern"]},
                   "tiers": arr(obj({"up_to": {"type": ["string", "null"], "pattern": DECIMAL["pattern"]},
                                     "unit_price": PRICE}, ["up_to", "unit_price"]))},
                  ["name", "unit", "unit_price", "quantity_source", "fallback_source", "usage_field", "trigger", "refundable",
                   "rounding", "method", "package_quantity", "tiers"]),
    "PriceCard": obj({
        "price_id": TEXT, "version": TEXT, "purpose": enum("sale", "cost"), "status": enum("draft", "enabled", "disabled"),
        "test_only": {"type": "boolean"}, "currency": {"const": "USD"}, "media_type": enum("image", "video", "audio"),
        "operation": TEXT, "model_match": obj({"kind": enum("exact", "alias", "prefix"), "value": TEXT}, ["kind", "value"]),
        "scope": obj({"kind": enum("group", "channel", "linked_template", "account", "account_group", "supplier_template"),
                      "reference": TEXT}, ["kind", "reference"]),
        "effective_at": {"type": ["string", "null"], "format": "date-time"},
        "rules": arr(obj({"priority": {"type": "integer", "minimum": 0},
                          "when": {"type": "object", "additionalProperties": {"type": ["string", "boolean"]}},
                          "charges": arr(ref("Charge"))}, ["priority", "when", "charges"])),
        "reservation": obj({"maximum_charge": {"type": ["string", "null"], "pattern": DECIMAL["pattern"]},
                            "max_quantities": {"type": "object", "additionalProperties": DECIMAL},
                            "overrun": {"const": "cap_and_alert"}}, ["maximum_charge", "max_quantities", "overrun"]),
        "success_policy": enum("at_least_one", "all_outputs_success"),
        "multiplier_policy": enum("effective_media_then_time", "account_cost_once"),
        "notes": TEXT
    }, ["price_id", "version", "purpose", "status", "test_only", "currency", "media_type", "operation", "model_match",
        "scope", "effective_at", "rules", "reservation", "success_policy", "multiplier_policy", "notes"]),
}


def duration(mode="unknown", values=None, low=None, high=None, default=None, required=False, scenarios=None):
    return dict(mode=mode, values=values or [], min=low, max=high, default=default, required=required, scenarios=scenarios or [])


def profile(provider, model, media, adapter):
    p = dict(profile_id=provider + ":" + model, version="draft-v1", provider=provider, model=model, upstream_model=model,
             adapter=adapter, media_type=media, enabled=False, status="draft", validation_status="待渠道验证",
             source=YUNFEI if provider == "yunfei" else V6K, scenarios=[], operations=[media], formats=["json"],
             duration=duration(), resolutions=None, aspect_ratios=None, qualities=None, resolution_policy="unknown",
             materials={k: dict(formats=[], max_count=0, max_file_bytes=None, max_duration_seconds=None) for k in ["image", "video", "audio"]},
             field_mapping={}, constraints=[], unverified=["max_request_bytes", "max_concurrent_tasks", "observation_seconds", "real_generation"],
             delivery=dict(poll_seconds=10 if provider == "yunfei" else 20, head=None, range=None, refresh=None),
             limits=dict(max_request_bytes=None, max_concurrent_tasks=None, observation_seconds=None))
    return p


def material(p, kind, formats, count=None, size=None):
    p["materials"][kind].update(formats=formats, max_count=count, max_file_bytes=size)
    if count is None:
        p["unverified"].append(kind + ".max_count")
    if size is None:
        p["unverified"].append(kind + ".max_file_bytes")


def capabilities():
    result = []
    for m in MODELS_Y:
        media = "image" if m.startswith(("gpt-", "gemini-")) else "audio" if m.startswith("lyria") else "video"
        adapter = "yunfei_gpt_images" if m.startswith("gpt") else "yunfei_gemini_native" if m.startswith("gemini") else "yunfei_lyria" if media == "audio" else "yunfei_grok" if m.startswith("grok") else "yunfei_veo_preview" if "generate-preview" in m else "yunfei_omni"
        p = profile("yunfei", m, media, adapter)
        if media == "image":
            p.update(duration=duration("not_applicable"), resolutions=["1K", "2K", "4K"], resolution_policy="native")
            p["constraints"].append({"n": 1, "depends_on_authorized_group": True})
            if m.startswith("gpt"):
                p.update(scenarios=["gpt-text", "gpt-edit"], formats=["json", "multipart"], operations=["image", "image_edit"], qualities=["medium", "high"])
                material(p, "image", ["file_png", "file_jpeg", "file_webp"])
                p["field_mapping"] = {"image": "multipart:image[]", "size": "size", "quality": "quality"}
                p["unverified"].append("size_pixel_enum_by_group")
            else:
                p.update(scenarios=["banana-text", "banana-edit"], operations=["gemini_image"])
                p["aspect_ratios"] = ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "5:4", "4:5", "21:9"] + (["8:1", "4:1", "1:4", "1:8"] if "flash" in m else [])
                material(p, "image", ["inline_data", "file_data"])
                p["field_mapping"] = {"image": "contents[].parts[]", "size": "generationConfig.imageConfig.imageSize"}
                p["constraints"].append({"automatic_aspect_ratio": "omit", "TEXT_url_output": "provider_extension"})
                p["unverified"].append("channel_validation_of_source_declared_aspect_ratios")
        elif media == "audio":
            p.update(scenarios=["audio-chat", "audio-submit", "audio-compat", "audio-models", "audio-endpoints"],
                     operations=["audio", "music", "chat_music"], duration=duration("hint", low=5, high=300), resolution_policy="native")
            p["constraints"] += [{"source_dedicated_endpoint_status": 404}, {"prompt_max": 10000, "title_max": 160,
                                  "lyrics_max": 20000, "n": 1, "bpm_min": 30, "bpm_max": 300,
                                  "seed_min": 0, "seed_max": 2147483647, "instrumental_ignores_lyrics": True,
                                  "audio_formats": ["mp3", "wav", "m4a"], "b64_max_bytes_source": "30 MB"},
                                 {"duration_hint_is_not_output_cap": True}]
            p["unverified"] += ["dedicated_endpoints", "sse", "chat_extension_parameters", "maximum_actual_duration"]
        elif m.startswith("grok"):
            p.update(scenarios=["grok-video-submit", "grok-video-image", "grok-video-result"], formats=["json", "multipart"],
                     duration=duration("range", low=1, high=15, default=8), resolutions=["480p", "720p"] + (["1080p"] if m.endswith("1.5") else []),
                     aspect_ratios=["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"], resolution_policy="enum")
            material(p, "image", ["url_object", "data_url_in_url_object", "base64_in_url_object", "multipart_input_reference"], 1)
            p["field_mapping"] = {"seconds": "duration", "image": "image.url", "resolution": "resolution"}
            p["constraints"].append({"image_aspect_ratio": "follow_reference", "video_edit_extension_source_status": 503})
        elif "generate-preview" in m:
            p.update(scenarios=["veo-text", "veo-ref"] if m.endswith("-ref") else ["veo-text", "veo-image", "veo-frames"],
                     duration=duration("enum", values=[4, 6, 8], required=True), resolutions=["720p"],
                     aspect_ratios=["16:9", "9:16"], resolution_policy="enum")
            material(p, "image", ["url_string", "data_url", "base64"], None if m.endswith("-ref") else 2)
            p["field_mapping"] = {"seconds": "duration", "image": "image_url", "images": "image_urls"}
            if m.endswith("-ref"):
                p["duration"]["scenarios"].append({"with_images": True, "fixed": 8, "aspect_ratio": "16:9"})
            else:
                p["constraints"].append({"image_urls_roles": ["first_frame", "last_frame"], "image_urls_exact_count": 2})
        else:
            p.update(scenarios=["omni-video-submit", "omni-video-image", "omni-video-result"], resolutions=["720p"],
                     aspect_ratios=["16:9"], resolution_policy="enum", duration=duration("enum", values=[4, 6, 8, 10], default=10) if m == "omni-flash" else duration("fixed", values=[8], default=8))
            material(p, "image", ["url_string", "data_url", "base64"])
            p["field_mapping"] = {"seconds": "duration", "image": "image", "images": "images"}
            p["constraints"].append({"seconds_upstream_type": "string", "unsupported": ["negative_prompt", "generate_audio"], "output": "1280x720"})
        result.append(p)
    for i, m in enumerate(MODELS_V):
        p = profile("v6k", m, "video", "v6k_video_gateway")
        p["scenarios"] = ["video-submit", "video-query", "video-download"]
        p["field_mapping"] = {"duration": "seconds", "aspect_ratio": "ratio"}
        if i < 2:
            p.update(duration=duration("enum", values=[5, 10, 15], default=5) if i == 0 else duration("fixed", values=[30], default=30), resolution_policy="omit")
            material(p, "image", ["public_url_object"], 10)
        elif 2 <= i <= 11:
            p["duration"]["required"] = True
            for k in ["image", "video", "audio"]:
                material(p, k, ["public_url_string", "public_url_object"])
            p["unverified"] += ["duration_enum", "resolution_enum", "allowed_material_types"]
        elif m == "video-QY":
            material(p, "video", ["public_url_string", "public_url_object"])
            p["constraints"].append({"required_reference": "video"})
            p["unverified"] += ["duration_enum", "resolution_enum"]
        elif m.startswith("doubao-"):
            for k in ["image", "video", "audio"]:
                material(p, k, ["public_url_string", "public_url_object", "metadata.content"])
            p["constraints"].append({"metadata_content": "native_validated_no_meter_override"})
            p["unverified"] += ["duration_enum", "resolution_enum", "allowed_material_types"]
        elif m == "JSadanco2.0":
            material(p, "image", ["public_url_string", "public_url_object", "data_url"], 9)
            p["unverified"] += ["duration_enum", "resolution_enum"]
        elif m == "LLseadanco2.5":
            p.update(duration=duration("fixed", values=[30], default=30), resolutions=["720p"], resolution_policy="enum", formats=["json", "multipart"])
            material(p, "image", ["public_url_string", "public_url_object", "base64", "data_url", "file"], 30, 30 * 1024 * 1024)
            p["constraints"].append({"image_fetch_timeout_seconds": 60, "multipart_fields": ["image", "images", "input_reference"]})
        elif m == "MinimaxH3-2K":
            p.update(duration=duration("range", low=4, high=15, default=15), resolutions=["2K", "768p"], resolution_policy="enum", formats=["json", "multipart"])
            for k, count in [("image", 9), ("video", 3), ("audio", 3)]:
                material(p, k, ["public_url_string", "public_url_object", "file"], count)
            p["unverified"] += ["input_video_duration_cap", "input_audio_duration_cap"]
        else:
            for k in ["image", "video", "audio"]:
                material(p, k, ["public_url_string", "public_url_object"])
            p["field_mapping"].update({"reference_image_urls": "images", "reference_video_urls": "videos", "reference_audio_urls": "audios"})
            p["unverified"] += ["duration_enum", "resolution_enum", "allowed_material_types"]
        result.append(p)
    return result


def charge(name, unit, price, source, **kwargs):
    return dict(name=name, unit=unit, unit_price=price, quantity_source=source, fallback_source=None, usage_field=None,
                trigger="success", refundable=True, rounding=dict(mode="none", step="1", minimum="0", scope="per_output"),
                method="linear", package_quantity=None, tiers=[], **kwargs)


def card(name, model, media, charges, purpose="sale", conditions=None):
    return dict(price_id=name, version="test-v1", purpose=purpose, status="draft", test_only=True, currency="USD",
                media_type=media, operation=media, model_match=dict(kind="exact", value=model),
                scope=dict(kind="linked_template" if purpose == "sale" else "supplier_template", reference="REPLACE_WITH_EXPLICIT_BINDING"),
                effective_at=None, rules=[dict(priority=0, when=conditions or {}, charges=charges)],
                reservation=dict(maximum_charge=None, max_quantities={}, overrun="cap_and_alert"),
                success_policy="at_least_one", multiplier_policy="effective_media_then_time" if purpose == "sale" else "account_cost_once",
                notes="测试价/开发模板，不可发布到生产；必须补真实价格、能力、绑定、预留和验收。")


def prices(profiles):
    cards = []
    for p in profiles:
        c = card("unconfigured:" + p["profile_id"], p["model"], p["media_type"], [charge("unconfigured", "request", None, "task_count")])
        c["test_only"] = False
        c["version"] = "draft-v1"
        c["notes"] = "未配置价格占位，不可启用；来源渠道须由管理员明确绑定，不能当默认价。"
        cards.append(c)
    cards += [card("sd25-request", "00sd2.5", "video", [charge("generation", "request", "1", "task_count")]),
              card("sd25-second", "00sd2.5", "video", [charge("duration", "second", "0.04", "fixed_duration")]),
              card("sd25-cost", "00sd2.5", "video", [charge("supplier", "second", "0.02", "fixed_duration")], "cost"),
              card("ll25-package", "LLseadanco2.5", "video", [charge("package", "second", "1", "fixed_duration")], conditions={"resolution": "720p"}),
              card("gpt-output", "gpt-image-2", "image", [charge("image", "output", "0.1", "successful_outputs")], conditions={"size_tier": "1K", "quality": "medium"}),
              card("image-token", "gpt-image-2", "image", [charge("input", "token", "0.000001", "provider_tokens"), charge("output", "token", "0.000004", "provider_tokens")]),
              card("music-minute", "lyria-3.5", "audio", [charge("audio", "minute", "0.6", "artifact_duration")]),
              card("composite", "LLseadanco2.5", "video", [charge("base", "request", "0.2", "task_count"), charge("duration", "second", "0.03", "fixed_duration"), charge("reference", "input_image", "0.05", "accepted_input_images")]),
              card("progressive", "00sd2.5", "video", [charge("progressive_duration", "second", "0", "fixed_duration")]),
              card("input-media", "MinimaxH3-2K", "video", [charge("generation", "second", "0.04", "request_duration"), charge("video_ref", "input_video_second", "0.01", "accepted_input_video_duration"), charge("audio_ref", "input_audio_second", "0.005", "accepted_input_audio_duration")], conditions={"resolution": "2K"}),
              card("accepted-base", "00sd2.5", "video", [charge("accepted_base", "request", "0.2", "task_count"), charge("generation", "request", "0.8", "task_count")])]
    by_id = {c["price_id"]: c for c in cards}
    by_id["ll25-package"]["rules"][0]["charges"][0].update(method="package", package_quantity="30")
    by_id["music-minute"]["rules"][0]["charges"][0]["rounding"].update(mode="ceil", step="60")
    by_id["progressive"]["rules"][0]["charges"][0].update(method="progressive", tiers=[dict(up_to="10", unit_price="0.04"), dict(up_to=None, unit_price="0.02")])
    by_id["accepted-base"]["rules"][0]["charges"][0].update(trigger="accepted", refundable=False)
    # Token quantities must refer to distinct adapter-whitelisted usage counters.
    by_id["image-token"]["rules"][0]["when"] = {"usage_profile": "image_input_and_output_tokens"}
    by_id["image-token"]["rules"][0]["charges"][0]["usage_field"] = "image_input_tokens"
    by_id["image-token"]["rules"][0]["charges"][1]["usage_field"] = "image_output_tokens"
    for c in cards:
        if c["test_only"]:
            c["reservation"]["maximum_charge"] = "10"
    return cards


def vectors():
    def line(unit, price, quantity, **kwargs):
        return dict(unit=unit, price=price, quantity=quantity, **kwargs)
    cases = [
        ("sd25-request", [line("request", "1", "1")], "1.00000000"),
        ("sd25-second", [line("second", "0.04", "30")], "1.20000000"),
        ("grok-720", [line("second", "0.02", "6")], "0.12000000"),
        ("grok-1080", [line("second", "0.04", "6")], "0.24000000"),
        ("ceil-second", [line("second", "0.1", "6.2", mode="ceil", step="1")], "0.70000000"),
        ("music-minute", [line("minute", "0.6", "61", mode="ceil", step="60")], "1.20000000"),
        ("minute-unrounded", [line("minute", "0.6", "61")], "0.61000000"),
        ("composite", [line("request", "0.2", "1"), line("second", "0.03", "30"), line("input_image", "0.05", "2")], "1.20000000"),
        ("free-tier", [line("request", "0", "1")], "0.00000000"),
        ("missing-tier", [line("request", None, "1")], None),
        ("supplier-cost", [line("second", "0.02", "30")], "0.60000000"),
        ("outputs", [line("output", "0.1", "2")], "0.20000000"),
        ("tokens", [line("token", "0.000001", "2000")], "0.00200000"),
        ("input-media", [line("input_video_second", "0.01", "5"), line("input_audio_second", "0.005", "6")], "0.08000000"),
        ("floor", [line("second", "0.1", "6.9", mode="floor", step="1")], "0.60000000"),
        ("half-up", [line("second", "0.1", "6.5", mode="half_up", step="1")], "0.70000000"),
        ("minimum", [line("second", "0.1", "1.2", mode="ceil", step="1", minimum="5")], "0.50000000"),
        ("round-once", [line("request", "0.000000004", "1"), line("request", "0.000000004", "1")], "0.00000001"),
        ("package", [line("second", "1", "30", method="package", package_quantity="30")], "1.00000000"),
        ("progressive", [line("second", "0", "30", method="progressive", tiers=[{"up_to": "10", "price": "0.04"}, {"up_to": None, "price": "0.02"}])], "0.80000000"),
    ]
    result = [dict(id=i, lines=ls, multiplier="1", time_multiplier="1", expected_total=total, expected_error="price_not_configured" if total is None else None) for i, ls, total in cases]
    result.append(dict(id="media-multiplier-once", lines=[line("request", "1.2", "1")], multiplier="2", time_multiplier="1.5", expected_total="3.60000000", expected_error=None))
    result.append(dict(id="cap", lines=[line("request", "1.2", "1")], multiplier="1", time_multiplier="1", cap="1", expected_total="1.00000000", expected_error=None))
    return dict(version=DRAFT, test_only=True, note="规范算术样本，非运行时账务验证", cases=result,
                ledger_scenarios=[dict(id="default_failure", reserved="1", accepted="0", final="0", released="1"),
                                  dict(id="accepted_failure", reserved="1", accepted="0.2", final="0", released="0.8"),
                                  dict(id="success", reserved="2", accepted="0.2", final="1", released="0.8")],
                runtime_required=["concurrent_reservation", "20_idempotent_retries", "lease_expiry_no_resubmit", "redis_down_recovery", "settlement_release_race", "refund_dedup", "snapshot_after_price_change", "quota_window_capture", "cost_retained_after_refund"])


def to_openapi_schema(s):
    if isinstance(s, list):
        return [to_openapi_schema(x) for x in s]
    if isinstance(s, dict):
        return {k: (v.replace("#/$defs/", "#/components/schemas/") if k == "$ref" else to_openapi_schema(v)) for k, v in s.items()}
    return s


def openapi():
    schemas = {k: to_openapi_schema(v) for k, v in DEFS.items()}
    # Runtime API schemas allow enabled published versions. The export templates stay draft-only.
    schemas["Capability"]["properties"]["enabled"] = {"type": "boolean"}
    schemas["Capability"]["properties"]["status"] = enum("draft", "enabled", "disabled")
    schemas["Capability"]["properties"]["validation_status"] = enum("源码已有基础", "参考服务声明验证", "待开发", "待渠道验证", "本项目实测通过")
    def sr(name):
        return {"$ref": "#/components/schemas/" + name}
    schemas.update({
        "Error": obj({"error": obj({"code": TEXT, "message": TEXT, "type": TEXT, "param": {"type": ["string", "null"]}}, ["type", "message"]),
                      "request_id": TEXT, "task_id": TEXT}, ["error"]),
        "AdminError": obj({"code": {"type": "integer"}, "message": TEXT, "reason": {"type": "string"}, "metadata": {"type": "object"}}, ["code", "message"]),
        "ConfigRecord": obj({"id": TEXT, "version": {"type": "integer"}, "published_version": {"type": ["integer", "null"]}, "status": TEXT, "config": {"type": "object"}}, ["id", "version", "published_version", "status", "config"]),
        "TaskRecord": obj({"task": sr("Task"), "user_id": {"type": "integer"}, "account_id": {"type": "integer"}, "cost_status": enum("unknown", "estimated", "verified"), "cost_amount": {"type": ["string", "null"]}, "margin_amount": {"type": ["string", "null"]}}, ["task"]),
        "Calculation": obj({"currency": {"const": "USD"}, "items": arr({"type": "object"}), "total": DECIMAL,
                            "uncapped": DECIMAL, "missing_usage": arr(TEXT)}, ["currency", "items", "total", "uncapped"]),
        "Billing": obj({"currency": {"const": "USD"}, "reserved": DECIMAL, "maximum_charge": DECIMAL,
                        "settled": DECIMAL, "refunded": DECIMAL, "accepted": DECIMAL, "released": DECIMAL, "price_version": TEXT,
                        "multiplier": DECIMAL, "time_multiplier": DECIMAL, "version": {"type": "integer"},
                        "quantities": {"type": "object", "additionalProperties": arr(DECIMAL)}, "calculation": sr("Calculation"),
                        "state": enum("held", "pending_usage", "settled", "released", "manual_review")}, ["currency", "price_version", "maximum_charge", "state"]),
        "Artifact": obj({"key": TEXT, "type": enum("image", "video", "audio"), "mime_type": TEXT, "size_bytes": NULL_INT,
                         "duration_seconds": {"type": ["string", "null"], "pattern": DECIMAL["pattern"]}, "content_url": TEXT,
                         "available": {"type": "boolean"}}, ["key", "type", "mime_type", "content_url", "available"]),
        "Task": obj({"id": TEXT, "object": enum("video", "audio.generation", "image.generation"), "model": TEXT,
                     "status": enum("queued", "in_progress", "completed", "failed", "unknown"),
                     "progress": {"type": "number", "minimum": 0, "maximum": 100},
                     "created_at": {"type": "integer"}, "completed_at": {"type": ["integer", "null"]},
                     "seconds": {"type": ["string", "null"]}, "billing": sr("Billing"), "data": arr(sr("Artifact")),
                     "error": {"oneOf": [sr("Error"), {"type": "null"}]}, "diagnostic": TEXT}, ["id", "object", "model", "status"]),
        "Reference": {"oneOf": [TEXT, obj({"url": TEXT, "duration_seconds": DECIMAL}, ["url"])]},
        "VideoRequest": obj({"model": TEXT, "prompt": {"type": "string"},
            "seconds": {"oneOf": [{"type": "integer", "minimum": 1}, {"type": "string", "pattern": "^[1-9][0-9]*$"}]},
            "duration": {"oneOf": [{"type": "integer", "minimum": 1}, {"type": "string", "pattern": "^[1-9][0-9]*$"}]},
            "ratio": TEXT, "aspect_ratio": TEXT, "resolution": TEXT, "size": TEXT,
            "image": sr("Reference"), "image_url": TEXT, "image_urls": arr(TEXT),
            "images": arr(sr("Reference")), "videos": arr(sr("Reference")), "audios": arr(sr("Reference")),
            "reference_image_urls": arr(sr("Reference")), "reference_video_urls": arr(sr("Reference")), "reference_audio_urls": arr(sr("Reference")),
            "input_reference": TEXT, "generate_audio": {"type": "boolean"}, "generateAudio": {"type": "boolean"},
            "negative_prompt": {"type": "string"}, "negativePrompt": {"type": "string"}, "metadata": {"type": "object"}}, ["model"]),
        "ImageRequest": obj({"model": TEXT, "prompt": TEXT, "size": TEXT, "quality": TEXT,
                             "n": {"type": "integer", "minimum": 1}, "response_format": enum("url", "b64_json")}, ["model", "prompt"]),
        "ImageResponse": {"type": "object", "properties": {"created": {"type": "integer"}, "data": arr({"type": "object", "properties": {"url": TEXT, "b64_json": TEXT}})}},
        "GeminiRequest": obj({"contents": arr({"type": "object"}), "generationConfig": {"type": "object"}}, ["contents"]),
        "NativeResponse": {"type": "object", "additionalProperties": True},
        "ChatRequest": {"type": "object", "required": ["model", "messages"], "properties": {"model": TEXT, "messages": {"type": "array", "minItems": 1, "items": {"type": "object"}}, "stream": {"type": "boolean"}}, "additionalProperties": True},
        "MusicRequest": obj({"model": TEXT, "prompt": {"type": "string", "minLength": 1, "maxLength": 10000},
                             "title": {"type": "string", "maxLength": 160}, "instrumental": {"type": "boolean", "default": False},
                             "lyrics": {"type": "string", "maxLength": 20000}, "duration": {"type": "integer", "minimum": 5, "maximum": 300},
                             "bpm": {"type": "integer", "minimum": 30, "maximum": 300}, "seed": {"type": "integer", "minimum": 0, "maximum": 2147483647},
                             "n": {"type": "integer", "const": 1}, "async": {"type": "boolean", "default": True},
                             "audio_format": enum("mp3", "wav", "m4a"), "response_format": enum("url", "b64_json")}, ["model", "prompt"]),
        "EstimateRequest": obj({"operation": enum("video", "audio", "music", "image", "image_edit", "gemini_image", "chat_music"),
                                "request": {"type": "object"}}, ["operation", "request"]),
        "Estimate": obj({"currency": {"const": "USD"}, "price_version": TEXT, "maximum_charge": DECIMAL,
                         "calculation": sr("Calculation"), "locks_price": {"const": False}},
                        ["currency", "price_version", "maximum_charge", "calculation", "locks_price"]),
        "Models": obj({"object": {"const": "list"}, "data": arr({"type": "object", "required": ["id"], "properties": {"id": TEXT, "object": {"const": "model"}, "capabilities": {"type": "object"}}})}, ["object", "data"]),
        "Artifacts": obj({"task_id": TEXT, "artifacts": arr(sr("Artifact"))}, ["task_id", "artifacts"]),
        "VersionRequest": obj({"expected_version": {"type": "integer"}, "reason": TEXT}, ["expected_version", "reason"]),
        "PublicMediaCapability": obj({
            "profile_id": {"type": "string", "pattern": "^mp_[0-9a-f]{64}$"}, "version": TEXT, "model": TEXT,
            "media_type": enum("image", "video", "audio"), "operations": arr(TEXT),
            "reference_field": TEXT, "reference_shape": TEXT,
            "duration": obj({"mode": TEXT, "values": {"type": ["array", "null"], "items": {"type": "integer"}},
                             "min": NULL_INT, "max": NULL_INT, "default": NULL_INT, "required": {"type": "boolean"}, "scenarios": {"type": "null"}}),
            **{name: {"type": ["array", "null"], "items": TEXT} for name in ["sizes", "resolutions", "aspect_ratios", "qualities", "formats"]},
            "materials": {"type": "object", "additionalProperties": obj({"formats": {"type": ["array", "null"], "items": TEXT},
                "max_count": NULL_INT, "max_file_bytes": NULL_INT, "max_duration_seconds": {"type": ["string", "null"]}})},
            "poll_seconds": {"type": "integer"}, "max_request_bytes": NULL_INT, "observation_seconds": NULL_INT
        }, ["profile_id", "version", "model", "media_type", "operations"]),
        "PublicMediaCatalog": obj({"group_id": {"type": "integer", "minimum": 1}, "group_name": TEXT,
            "platform": {"const": "imagevideo"}, "models": arr(sr("PublicMediaCapability"))}, ["group_id", "group_name", "platform", "models"]),
        "SimulationRequest": obj({"operation": TEXT, "request": {"type": "object"}, "capability": sr("Capability"),
                                  "price": sr("PriceCard"), "cost": sr("PriceCard"), "identity": {"type": "object"},
                                  "cost_multiplier": DECIMAL, "observed_quantities": {"type": "object", "additionalProperties": arr(DECIMAL)}},
                                 ["operation", "request", "capability", "price"]),
        "Simulation": {"type": "object", "properties": {"quote": sr("Calculation"), "maximum_charge": DECIMAL,
                        "matched_rule": {"type": "object"}, "conditions": {"type": "object"}, "settlement": sr("Calculation"),
                        "estimated_cost": sr("Calculation"), "estimated_margin": {"type": "string"}}, "additionalProperties": True},
        "ReconcileRequest": obj({"expected_version": {"type": "integer"}, "decision": enum("query", "failed"),
                                 "evidence": TEXT, "upstream_id": TEXT}, ["expected_version", "decision", "evidence"]),
        "RefundRequest": obj({"expected_version": {"type": "integer"}, "amount": DECIMAL,
                              "reason": TEXT}, ["expected_version", "amount", "reason"]),
    })
    for name in ["VideoRequest", "ImageRequest", "GeminiRequest", "MusicRequest", "ChatRequest"]:
        schemas[name]["properties"]["public_profile_id"] = {"type": "string", "pattern": "^mp_[0-9a-f]{64}$", "description": "可选；精确选择登录能力目录中的配置。"}
    api = dict(openapi="3.1.0", info=dict(title="Sub2API 多模态 API 实施规格（非已部署承诺）", version=DRAFT,
              description="v0.2.15基础上的媒体运行时接口。模型受能力和发布价卡门控，模板默认关闭；实测范围见REAL_CHANNEL_VALIDATION.md。接口存在不表示所有模型验证通过。"),
              servers=[{"url": "https://your-sub2api.example", "description": "替换为自己的站点根；paths已包含/v1"}],
              security=[{"DownstreamKey": []}], paths={},
              components=dict(securitySchemes={"DownstreamKey": {"type": "http", "scheme": "bearer"},
                                               "GeminiKey": {"type": "apiKey", "in": "header", "name": "x-goog-api-key"},
                                               "UserSession": {"type": "http", "scheme": "bearer", "description": "现有用户登录JWT，不是下游API Key。"},
                                               "AdminSession": {"type": "http", "scheme": "bearer", "description": "现有管理员认证token，非下游API Key；沿用权限及敏感操作验证。"}}, schemas=schemas))
    def response(name, desc="成功", media="application/json"):
        return {"description": desc, "content": {media: {"schema": sr(name)}}}
    errors = {str(code): response("Error", desc) for code, desc in [(400, "参数或别名冲突"), (401, "认证失败"), (403, "权限不足"), (404, "接口/任务不存在或无归属权限"), (409, "幂等/版本冲突或制品未就绪"), (422, "未配置能力/价格或预留无上限"), (429, "限流"), (502, "上游异常/提交结果待确认"), (503, "功能关闭/繁忙"), (504, "超时；不自动重提") ]}
    def add(path, method, oid, summary, schema=None, status="proposed", responses=None, description="", admin=False):
        op = dict(operationId=oid, summary=summary, description=description or "具体模型能力、价卡与渠道验证完成后才开放。",
                  tags=["Admin" if admin else "Media"], **{"x-implementation-status": status, "x-live-provider-verified": False},
                  responses={**copy.deepcopy(errors), **(responses or {"200": response("NativeResponse")})})
        params = [{"name": n, "in": "path", "required": True, "schema": TEXT} for n in ["id", "key", "model"] if "{" + n + "}" in path]
        if params:
            op["parameters"] = params
        if schema:
            op["requestBody"] = {"required": True, "content": {"application/json": {"schema": sr(schema)}}}
        if admin:
            op["security"] = [{"AdminSession": []}]
        api["paths"].setdefault(path, {})[method] = op
        return op
    add("/v1/models", "get", "listModels", "当前Key模型发现", status="existing-foundation", responses={"200": response("Models")})
    add("/v1/tasks/{id}", "get", "getOwnedMediaTask", "统一查询当前Key所属媒体任务；不重新提交或收费", responses={"200": response("Task")})
    add("/v1/images/generations", "post", "createImage", "同步图像生成", "ImageRequest", "existing-foundation", {"200": response("ImageResponse")})
    op = add("/v1/images/edits", "post", "editImage", "同步图像编辑", status="existing-foundation", responses={"200": response("ImageResponse")})
    edit = copy.deepcopy(schemas["ImageRequest"])
    edit["properties"].update({"image": {"type": "string", "format": "binary"}, "image[]": arr({"type": "string", "format": "binary"}), "mask": {"type": "string", "format": "binary", "description": "可选的单张PNG mask，仅支持GPT图片编辑。"}})
    edit["anyOf"] = [{"required": ["image"]}, {"required": ["image[]"]}]
    op["requestBody"] = {"required": True, "content": {"multipart/form-data": {"schema": edit}}}
    op = add("/v1beta/models/{model}:generateContent", "post", "geminiImage", "Gemini原生图像", "GeminiRequest", "existing-foundation", {"200": response("NativeResponse")})
    op["security"] = [{"DownstreamKey": []}, {"GeminiKey": []}]
    op = add("/v1/videos", "post", "createVideo", "通用异步视频", "VideoRequest", "existing-foundation-extension-proposed", {"202": response("Task", "新通用任务已落库；保留本地ID"), "200": response("Task", "已有路径保留响应约定")}, "旧Grok路径保留；新通用任务202本地ID。未知受理结果进入unknown；同幂等键不同参数409；不自动重提。")
    op["parameters"] = [{"name": "Idempotency-Key", "in": "header", "schema": TEXT, "description": "建议必传；服务端以Key+规范化operation+请求hash约束。"}]
    multi = copy.deepcopy(schemas["VideoRequest"])
    multi["properties"].update({k: {"type": "string", "format": "binary"} for k in ["image", "input_reference", "video", "audio"]})
    multi["properties"]["images"] = arr({"type": "string", "format": "binary"})
    op["requestBody"]["content"]["multipart/form-data"] = {"schema": multi, "encoding": {"metadata": {"contentType": "application/json"}}, "x-model-gated": True}
    op["responses"]["402"] = response("Error", "余额/额度不足，未生成")
    add("/v1/videos/{id}", "get", "getVideo", "查询视频（不收费）", status="existing-foundation-extension-proposed", responses={"200": response("Task")})
    def content(path, oid, status="proposed", head=False):
        responses = {"200": {"description": "完整媒体字节；HEAD无响应体"}, "206": {"description": "支持Range时返回部分字节"}, "304": {"description": "支持条件请求时未修改"}, "410": response("Error", "制品过期且不可刷新"), "416": {"description": "范围无效；Content-Range bytes */N"}}
        for code in ["200", "206"]:
            responses[code]["headers"] = {"Content-Type": {"schema": TEXT}, "Content-Length": {"schema": {"type": "integer"}}, "ETag": {"schema": TEXT}, "Accept-Ranges": {"schema": TEXT}, "Content-Range": {"schema": TEXT}}
            if not head:
                responses[code]["content"] = {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}
        op = add(path, "head" if head else "get", oid, "读取制品头部" if head else "制品下载（不收费）", status=status, responses=responses,
                 description="HEAD/Range/刷新按模型及渠道能力开放，不伪造206。不发送供应商Key给媒体域名，相对地址使用绑定账号base解析；下载不重新生成。")
        op.setdefault("parameters", []).extend([{"name": "Range", "in": "header", "schema": TEXT}, {"name": "If-None-Match", "in": "header", "schema": TEXT}])
    content("/v1/videos/{id}/content", "downloadVideo", "existing-foundation-extension-proposed")
    content("/v1/videos/{id}/content", "headVideo", head=True)
    add("/v1/tasks/{id}/artifacts", "get", "listArtifacts", "列出任务制品", responses={"200": response("Artifacts")})
    content("/v1/tasks/{id}/artifacts/{key}/content", "downloadArtifact")
    op = add("/v1/chat/completions", "post", "chatOrMusic", "聊天/已声明的音乐协议", "ChatRequest", "existing-foundation-music-proposed", {"200": response("NativeResponse")}, "现有聊天保留。Lyria非流式保留choices与data；SSE待渠道验证且默认关闭；TEXT进度不等于实时音频。")
    op["responses"]["200"]["content"]["text/event-stream"] = {"schema": {"type": "string"}, "x-status": "unverified-disabled"}
    for path, oid in [("/v1/audio/generations", "createMusic"), ("/v1/music/generations", "createMusicAlias")]:
        op = add(path, "post", oid, "音乐生成（来源专用路径未开放）", "MusicRequest", responses={"202": response("Task", "async=true本地任务"), "200": response("Task", "async=false同任务等待完成")}, description="两个入口规范化为同一operation进行幂等去重；来源404不能当作已可用。duration是提示不是输出上限。")
        op["parameters"] = [{"name": "Idempotency-Key", "in": "header", "schema": TEXT}]
    add("/v1/audio/generations/{id}", "get", "getMusic", "音乐任务查询", responses={"200": response("Task")})
    content("/v1/audio/generations/{id}/content", "downloadMusic")
    add("/v1/sub2api/estimates", "post", "estimateMedia", "只估价（无上游/资金副作用）", "EstimateRequest", responses={"200": response("Estimate")}, description="身份/分组来自Key；只返回自己的售价，不含供应商成本；不锁价，不冻结不扣费不提交供应商。")
    for kind, schema, name in [("capability","Capability","Capability"),("price","PriceCard","PriceCard")]:
        path="/api/v1/admin/media/configs/"+kind
        op=add(path,"get","list"+name,"列表；id查询参数读取指定配置历史",admin=True)
        op["parameters"]=[{"name":"id","in":"query","schema":TEXT}]
        op=add(path,"post","save"+name,"保存草稿或发布版本（敏感操作认证）",admin=True)
        op["requestBody"]={"required":True,"content":{"application/json":{"schema":obj({"id":TEXT,"expected_version":{"type":"integer","minimum":0},"publish":{"type":"boolean"},"config":sr(schema)},["id","expected_version","publish","config"])}}}
        op=add(path+"/disable","post","disable"+name,"停用并新增版本（敏感操作认证）",admin=True)
        op["requestBody"]={"required":True,"content":{"application/json":{"schema":obj({"id":TEXT,"expected_version":{"type":"integer","minimum":1}},["id","expected_version"])}}}
    add("/api/v1/admin/media/simulate", "post", "simulateMedia", "管理员计费模拟（无副作用）", "SimulationRequest", responses={"200": response("Simulation")}, admin=True)
    op = add("/api/v1/admin/media/tasks", "get", "listMediaTasks", "任务核对队列（分页）", admin=True)
    op["parameters"]=[{"name":"before","in":"query","schema":TEXT},{"name":"user_id","in":"query","schema":{"type":"integer"}},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":100}}]
    add("/api/v1/admin/media/tasks/{id}/reconcile", "post", "reconcileMediaTask", "带证据核对任务，禁止盲目重提", "ReconcileRequest", admin=True)
    op = add("/api/v1/admin/media/tasks/{id}/refund", "post", "refundMediaTask", "追加幂等退款，不覆写原账单", "RefundRequest", admin=True)
    op.setdefault("parameters", []).append({"name": "Idempotency-Key", "in": "header", "required": True, "schema": TEXT})
    op=add("/api/v1/admin/media/tasks/{id}/cost","post","verifyMediaCost","核对供应商成本账单；不改用户扣款",admin=True)
    op["requestBody"]={"required":True,"content":{"application/json":{"schema":obj({"amount":DECIMAL,"evidence":TEXT,"expected_version":{"type":"integer"}},["amount","evidence","expected_version"])}}}
    op["parameters"].append({"name":"Idempotency-Key","in":"header","required":True,"schema":TEXT})
    add("/api/v1/media/tasks","get","listOwnMediaTasks","当前用户任务和收费记录",admin=True)
    op = add("/api/v1/media/capabilities", "get", "listOwnMediaCapabilities", "登录用户可绑定分组的公开媒体能力；不创建Key或扣费")
    op["security"] = [{"UserSession": []}]
    op["parameters"] = [{"name": "group_id", "in": "query", "required": True, "schema": {"type": "integer", "minimum": 1}}]
    op["description"] = "仅返回启用、白名单允许且有有效售价的能力。每个参数配置独立返回不可混合；不返回供应商凭据、账户绑定、成本或内部地址。生成和估价请求可携带public_profile_id精确选择此配置。"
    # Management and user JSON use the existing code/message/data envelope;
    # gateway responses stay native. Errors can come from either media guards
    # or existing auth/step-up middleware.
    for url, path in api["paths"].items():
        if not url.startswith("/api/v1/"):
            continue
        for method, operation in path.items():
            if "/configs/" in url:
                payload = arr(sr("ConfigRecord")) if method == "get" else sr("ConfigRecord")
            elif url.endswith("/simulate"):
                payload = sr("Simulation")
            elif url.endswith("/tasks"):
                payload = arr(sr("TaskRecord"))
            elif url.endswith("/capabilities"):
                payload = sr("PublicMediaCatalog")
            else:
                payload = obj({"ok": {"type": "boolean"}}, ["ok"])
            operation["responses"]["200"] = {"description": "成功；现有管理/用户响应信封", "content": {"application/json": {"schema": obj({"code": {"const": 0}, "message": TEXT, "data": payload}, ["code", "message", "data"])}}}
            for code, response_value in operation["responses"].items():
                if int(code) >= 400:
                    response_value["content"]["application/json"]["schema"] = {"oneOf": [sr("Error"), sr("AdminError")]}
    for path in api["paths"].values():
        for operation in path.values(): operation["x-implementation-status"]="implemented-model-gated"
    api["paths"]["/v1/chat/completions"]["post"]["responses"]["200"]["content"]["text/event-stream"]["x-status"]="implemented-completion-chunks-not-audio-stream"
    return api


def write_json(name, data):
    (ROOT / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main():
    profiles = capabilities()
    schema = {"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://your-sub2api.example/spec/multimodal-config.json", "$defs": DEFS,
              **obj({"schema_version": {"const": DRAFT}, "kind": enum("capabilities", "price_cards"), "source_date": {"const": SOURCE_DATE},
                     "note": TEXT, "yunfei_scenarios": arr(TEXT), "profiles": arr(ref("Capability")), "cards": arr(ref("PriceCard"))},
                    ["schema_version", "kind", "source_date", "note"]),
              "oneOf": [{"properties": {"kind": {"const": "capabilities"}}, "required": ["profiles", "yunfei_scenarios"]},
                        {"properties": {"kind": {"const": "price_cards"}}, "required": ["cards"]}]}
    write_json("config.schema.json", schema)
    write_json("model-capabilities.json", dict(schema_version=DRAFT, kind="capabilities", source_date=SOURCE_DATE,
               note="来源声明的配置模板，全部关闭。null不是无限制；通过后台导入编辑，补齐绑定/约束/价格与实测后方可发布。",
               yunfei_scenarios=SCENARIOS, profiles=profiles))
    write_json("price-cards.json", dict(schema_version=DRAFT, kind="price_cards", source_date=SOURCE_DATE,
               note="未配置卡不可启用；test_only卡仅用于测试，不是正式报价；通过后台逐卡导入并填写正式价格。", cards=prices(profiles)))
    write_json("billing-vectors.json", vectors())
    target = ROOT.parent / "openapi" / "multimodal-api.yaml"
    target.parent.mkdir(exist_ok=True)
    target.write_text(yaml.safe_dump(openapi(), allow_unicode=True, sort_keys=False), encoding="utf-8")
    print(f"Generated draft: {len(profiles)} models; {len(SCENARIOS)} Yunfei sections; no runtime changes.")


if __name__ == "__main__":
    main()
