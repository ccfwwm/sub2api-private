import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import check_pnpm_audit_exceptions as checker


def report(inventory=None, format_name="advisories"):
    inventory = {} if inventory is None else inventory
    counts = dict.fromkeys(checker.SEVERITIES, 0)
    for entry in inventory.values():
        counts[entry["severity"]] += 1
    return {format_name: inventory, "metadata": {"vulnerabilities": counts}}


class AuditReportTests(unittest.TestCase):
    def invoke(self, data, exceptions="version: 1\nexceptions:\n", raw=False):
        with tempfile.TemporaryDirectory() as folder:
            audit = Path(folder) / "audit.json"
            audit.write_text(data if raw else json.dumps(data), encoding="utf-8")
            allowed = Path(folder) / "exceptions.yml"
            allowed.write_text(exceptions, encoding="utf-8")
            output, error = io.StringIO(), io.StringIO()
            with patch("sys.argv", ["check", "--audit", str(audit), "--exceptions", str(allowed)]), contextlib.redirect_stdout(output), contextlib.redirect_stderr(error):
                code = checker.main()
            return code, output.getvalue(), error.getvalue()

    def test_valid_clean_reports_pass_both_supported_formats(self):
        for format_name in ("advisories", "vulnerabilities"):
            with self.subTest(format=format_name):
                code, output, error = self.invoke(report(format_name=format_name))
                self.assertEqual(code, 0)
                self.assertIn("validated", output)
                self.assertEqual(error, "")

    def test_endpoint_errors_fail_even_with_an_empty_inventory(self):
        for data in (
            {"error": {"code": "ERR_PNPM_AUDIT_ENDPOINT_NOT_EXISTS", "message": "endpoint unavailable"}},
            {**report(), "error": None},
        ):
            with self.subTest(data=data):
                code, output, error = self.invoke(data)
                self.assertEqual(code, 1)
                self.assertNotIn("validated", output)
                self.assertIn("audit command returned an error", error)

    def test_invalid_json_and_unrecognized_reports_fail(self):
        for data in ("", "{", "not JSON"):
            with self.subTest(raw=data):
                self.assertEqual(self.invoke(data, raw=True)[0], 1)
        for data in (None, [], {}, {"advisories": {}}, {"metadata": report()["metadata"]}):
            with self.subTest(data=data):
                self.assertEqual(self.invoke(data)[0], 1)

    def test_invalid_summary_or_inventory_fails(self):
        for data in (
            {**report(), "advisories": []},
            {**report(), "advisories": {"one": None}},
            {**report(), "metadata": {"vulnerabilities": {"high": 0, "critical": 0}}},
            {**report(), "metadata": {"vulnerabilities": dict.fromkeys(checker.SEVERITIES, False)}},
            {**report(), "metadata": {"vulnerabilities": dict.fromkeys(checker.SEVERITIES, -1)}},
        ):
            with self.subTest(data=data):
                self.assertEqual(self.invoke(data)[0], 1)

    def test_summary_cannot_hide_missing_high_severity_entries(self):
        data = report()
        data["metadata"]["vulnerabilities"]["high"] = 1
        code, _, error = self.invoke(data)
        self.assertEqual(code, 1)
        self.assertIn("does not match", error)

    def test_severity_filtered_inventory_is_rejected_as_incomplete(self):
        data = report({"one": {"module_name": "dependency", "severity": "high", "id": 1}})
        # pnpm --audit-level=high omits lower severities from advisories while
        # retaining them in metadata. The gate requires an unfiltered report.
        data["metadata"]["vulnerabilities"]["moderate"] = 1
        code, _, error = self.invoke(data)
        self.assertEqual(code, 1)
        self.assertIn("does not match", error)

    def test_malformed_advisories_fail_instead_of_being_skipped(self):
        for entry in (
            {"severity": "unknown", "module_name": "dependency", "id": 1},
            {"severity": "high", "id": 1},
            {"severity": "high", "module_name": "dependency"},
        ):
            data = report()
            data["advisories"]["one"] = entry
            with self.subTest(entry=entry):
                self.assertEqual(self.invoke(data)[0], 1)

    def test_modern_high_without_advisory_references_fails(self):
        for via in ([], None, {}, [None], [{}], [""]):
            data = report({"dependency": {"severity": "high", "via": via}}, "vulnerabilities")
            with self.subTest(via=via):
                self.assertEqual(self.invoke(data)[0], 1)

    def test_valid_high_requires_exact_unexpired_exception(self):
        for format_name, entry in (
            ("advisories", {"module_name": "dependency", "severity": "high", "github_advisory_id": "GHSA-test-1234-5678"}),
            ("vulnerabilities", {"severity": "high", "via": [{"github_advisory_id": "GHSA-test-1234-5678"}]}),
        ):
            data = report({"dependency": entry}, format_name)
            allowed = """version: 1
exceptions:
  - package: dependency
    advisory: GHSA-test-1234-5678
    severity: high
    mitigation: fixture only
    expires_on: 2999-01-01
"""
            with self.subTest(format=format_name):
                self.assertEqual(self.invoke(data)[0], 1)
                self.assertEqual(self.invoke(data, allowed)[0], 0)
                self.assertEqual(self.invoke(data, allowed.replace("2999-01-01", "2000-01-01"))[0], 1)
                self.assertEqual(self.invoke(data, allowed.replace("GHSA-test-1234-5678", "GHSA-other-1234-5678"))[0], 1)


if __name__ == "__main__":
    unittest.main()
