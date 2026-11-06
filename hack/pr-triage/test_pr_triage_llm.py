# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Offline regression tests. No API calls, credentials or checkout of PR code.

Run with: python3 -m unittest discover -s hack/pr-triage -p 'test_*.py'
"""

import contextlib
import copy
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import urllib.error
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import pr_triage_llm as TRIAGE  # noqa: E402

ROOT = Path(__file__).resolve().parents[2]
CONFIG = ROOT / ".github/pr-triage/judgements.json"


def changed_file(name, status="modified", patch="@@ -1 +1 @@\n-old\n+new"):
    return {"filename": name, "status": status, "patch": patch, "additions": 1, "deletions": 1}


PRODUCTION = changed_file("controllers/util/solr_util.go")


MODEL_ENV = {"LLM_BASE_URL": "https://example.invalid/v1", "LLM_MODEL": "test", "LLM_API_KEY": "fake"}


def load_config():
    return json.loads(CONFIG.read_text(encoding="utf-8"))


def judgement(cfg, judgement_id):
    return next(j for j in cfg["judgements"] if j["id"] == judgement_id)


class TriageTest(unittest.TestCase):
    def setUp(self):
        self.cfg = {
            "min_confidence": "medium",
            "project": {"maintainers": "Solr Operator maintainers"},
            "judgements": [{
                "id": "needs_docs", "title": "Documentation",
                "question": "Does this warrant documentation?",
                "evidence_files": ["docs/modules/**"],
            }],
        }
        self.pr = {
            "title": "Fix pod startup", "body": "A description.",
            "base": {"ref": "main", "sha": "b" * 40, "repo": {"full_name": "apache/solr-operator"}},
            "head": {"sha": "a" * 40, "repo": {
                "full_name": "contributor/solr-operator", "owner": {"type": "User"},
            }},
            "maintainer_can_modify": True,
        }
        self.answers = {"needs_docs": {
            "verdict": True, "confidence": "high", "reason": "Changes a documented option.",
        }}

    def run_main(self, files, answers=None, dry_run=False):
        # Judgeable files are needed for the model to be asked at all.
        files = [PRODUCTION, *files]
        return self.run_main_exact(files, answers, dry_run)

    def run_main_exact(self, files, answers=None, dry_run=False):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            (temp / "config.json").write_text(json.dumps(self.cfg), encoding="utf-8")
            (temp / "pr.json").write_text(json.dumps(self.pr), encoding="utf-8")
            (temp / "files.json").write_text(json.dumps(files), encoding="utf-8")
            args = ["pr_triage_llm.py", "--config", str(temp / "config.json"),
                    "--pr", str(temp / "pr.json"), "--files", str(temp / "files.json"),
                    "--out", str(temp / "result.json")]
            if dry_run:
                args.append("--dry-run")
            content = json.dumps(self.answers) if answers is None else answers
            output = io.StringIO()
            with mock.patch("sys.argv", args), contextlib.redirect_stdout(output):
                with mock.patch.object(TRIAGE, "call_model", return_value=(content, {})) as model:
                    TRIAGE.main()
            result_path = temp / "result.json"
            result = json.loads(result_path.read_text(encoding="utf-8")) if result_path.exists() else None
            return result, model, output.getvalue()

    def prompt(self, model):
        return str(model.call_args[0][0])

    def test_deleted_evidence_is_missing(self):
        result, _, _ = self.run_main([changed_file("docs/modules/solr-cloud/pages/tls.adoc", "removed")])
        row = result["results"]["needs_docs"]
        self.assertFalse(row["included"])
        self.assertTrue(row["mismatch"])
        self.assertEqual(1, result["mismatches"])
        self.assertIn("missing", result["comment"])

    def test_present_evidence_statuses(self):
        for status in ("added", "modified", "renamed", "copied"):
            with self.subTest(status=status):
                result, _, _ = self.run_main([changed_file("docs/modules/solr-cloud/pages/tls.adoc", status)])
                self.assertTrue(result["results"]["needs_docs"]["included"])
                self.assertEqual(0, result["mismatches"])

    def test_missing_answer_is_unknown(self):
        result, _, _ = self.run_main([], '{"unrelated": {}}')
        row = result["results"]["needs_docs"]
        self.assertIsNone(row["warranted"])
        self.assertIsNone(row["mismatch"])
        self.assertIn("unknown", result["comment"])
        self.assertIn("human review", result["comment"])

    def test_invalid_answer_fields_are_unknown(self):
        for patch in ({"verdict": "maybe"}, {"verdict": 0}, {"confidence": "certain"},
                      {"confidence": []}, {"reason": []}, {"reason": "  "}):
            with self.subTest(patch=patch):
                answer = dict(self.answers["needs_docs"], **patch)
                result, _, _ = self.run_main([], json.dumps({"needs_docs": answer}))
                self.assertIsNone(result["results"]["needs_docs"]["warranted"])

    def test_textual_boolean_verdicts_are_accepted(self):
        self.cfg["judgements"] = [judgement(load_config(), "needs_tests")]
        for text, expected in (("true", True), (" False ", False), ("Yes", True), ("no", False)):
            with self.subTest(text=text):
                answer = {"verdict": text, "confidence": "high", "reason": "Because."}
                result, _, _ = self.run_main([], json.dumps({"needs_tests": answer}))
                self.assertIs(expected, result["results"]["needs_tests"]["warranted"])

    def test_malformed_answer_is_unknown(self):
        for content in ('{"needs_docs":true}', '{"needs_docs":{}}'):
            with self.subTest(content=content):
                result, _, _ = self.run_main([], content)
                self.assertIsNone(result["results"]["needs_docs"]["warranted"])

    def test_unusable_reply_posts_nothing(self):
        for content in ("not JSON", "[]", "null", "{}", '{"needs_docs":{},"needs_docs":{}}'):
            with self.subTest(content=content):
                result, _, output = self.run_main([], content)
                self.assertIsNone(result)
                self.assertIn("::warning::", output)

    def test_unreachable_model_posts_nothing(self):
        failures = (urllib.error.URLError("down"), TimeoutError(), ValueError("bad json"))
        for failure in failures:
            with self.subTest(failure=failure), mock.patch.dict("os.environ", MODEL_ENV):
                with mock.patch.object(TRIAGE.urllib.request, "urlopen", side_effect=failure):
                    with self.assertRaises(TRIAGE.NoAnswer):
                        TRIAGE.call_model([], {})

    def test_error_shaped_or_truncated_response_is_no_answer(self):
        for body in ({"error": "overloaded"}, {"choices": []},
                     {"choices": [{"message": {"content": "{"}, "finish_reason": "length"}]},
                     {"choices": [{"message": {"content": " "}}]}):
            with self.subTest(body=body), mock.patch.dict("os.environ", MODEL_ENV):
                with mock.patch.object(TRIAGE.urllib.request, "urlopen", return_value=io.StringIO(json.dumps(body))):
                    with self.assertRaises(TRIAGE.NoAnswer):
                        TRIAGE.call_model([], {})

    def test_only_evidence_files_changed_posts_nothing(self):
        self.cfg["judgements"][0]["evidence_files"] += ["**/*_test.go"]
        files = [changed_file("docs/modules/solr-cloud/pages/tls.adoc"), changed_file("controllers/solrcloud_controller_test.go")]
        result, model, _ = self.run_main_exact(files)
        self.assertIsNone(result)
        model.assert_not_called()

    def test_comment_does_not_emit_active_model_markup(self):
        self.answers["needs_docs"]["reason"] = (
            '<img src="x"> [click](https://example.org) ![image](x) '
            '@someone @org/team `code` **bold** | cell\nnew\rline &lt;b&gt;'
        )
        result, _, _ = self.run_main([])
        reason = result["results"]["needs_docs"]["reason"]
        for active in ("<img", "[click]", "![image]", "@someone", "@org/team", "`", "**", "|", "\n", "\r", "https://"):
            self.assertNotIn(active, reason)
        self.assertIn("&#60;img", reason)
        self.assertIn("&#64;​someone", reason)
        self.assertIn("&#124;", reason)
        self.assertIn(reason, result["comment"])

    def test_reason_is_bounded_and_cannot_reintroduce_entities(self):
        self.answers["needs_docs"]["reason"] = "&lt;script&gt; #123 www.example.org‮" + "x" * 1000
        result, _, _ = self.run_main([])
        reason = result["results"]["needs_docs"]["reason"]
        self.assertIn("&#38;lt", reason)
        self.assertIn("&#35;​123", reason)
        self.assertNotIn("www.example.org", reason)
        self.assertNotIn("‮", reason)
        self.assertLess(len(reason), 1000)

    def test_fenced_json_is_accepted(self):
        content = "```json\n" + json.dumps(self.answers) + "\n```"
        result, _, _ = self.run_main([], content)
        self.assertTrue(result["results"]["needs_docs"]["warranted"])

    def test_unknown_answer_does_not_suppress_valid_answers(self):
        self.cfg["judgements"].append({"id": "other", "title": "Other", "question": "Other?"})
        result, _, _ = self.run_main([])
        self.assertEqual(1, result["unanswered"])
        self.assertEqual(1, result["mismatches"])
        self.assertTrue(result["results"]["needs_docs"]["warranted"])

    def test_reverse_mismatch_and_low_confidence(self):
        answer = self.answers["needs_docs"]
        answer.update(verdict=False, confidence="low")
        result, _, _ = self.run_main([changed_file("docs/modules/solr-cloud/pages/tls.adoc")])
        self.assertTrue(result["results"]["needs_docs"]["mismatch"])
        self.assertEqual(0, result["mismatches"])
        self.assertIn("❔", result["comment"])
        self.assertIn("probably not needed", result["comment"])

    def test_non_main_notice_precedes_model_table(self):
        self.pr["base"]["ref"] = "release-0.9"
        result, _, _ = self.run_main([])
        self.assertIn("Prefer PRs against `main`", result["comment"])
        self.assertLess(result["comment"].index("Prefer PRs"), result["comment"].index("Warranted?"))

    def test_main_does_not_warn(self):
        result, _, _ = self.run_main([])
        self.assertNotIn("Prefer PRs", result["comment"])

    def test_personal_fork_edits_disabled(self):
        self.pr["maintainer_can_modify"] = False
        result, _, _ = self.run_main([])
        self.assertIn("Consider enabling", result["comment"])
        self.assertIn("so Solr Operator maintainers can contribute", result["comment"])
        self.assertIn("(optional)", result["comment"])

    def test_access_notice_exceptions(self):
        for case in ("allowed", "same_repo", "org", "missing_repo", "unknown"):
            with self.subTest(case=case):
                pr = copy.deepcopy(self.pr)
                pr["maintainer_can_modify"] = False
                if case == "allowed":
                    pr["maintainer_can_modify"] = True
                elif case == "same_repo":
                    pr["head"]["repo"]["full_name"] = "apache/solr-operator"
                elif case == "org":
                    pr["head"]["repo"]["owner"]["type"] = "Organization"
                elif case == "missing_repo":
                    pr["head"]["repo"] = None
                else:
                    pr.pop("maintainer_can_modify")
                comment = "\n".join(TRIAGE.metadata_notices(pr, self.cfg))
                self.assertNotIn("Consider enabling", comment)
                if case == "org":
                    self.assertIn("unavailable", comment)

    def test_metadata_is_escaped(self):
        self.pr["base"]["ref"] = "branch/@someone/[click]"
        result, _, _ = self.run_main([])
        self.assertNotIn("@someone", result["comment"])
        self.assertNotIn("[click]", result["comment"])

    def test_hidden_evidence_is_absent_from_prompt(self):
        files = [changed_file("docs/modules/solr-cloud/pages/secret.adoc", patch="HIDDEN EVIDENCE"),
                 changed_file("controllers/solrcloud_controller.go", patch="PRODUCTION CHANGE")]
        _, model, _ = self.run_main(files)
        messages = self.prompt(model)
        self.assertNotIn("HIDDEN EVIDENCE", messages)
        self.assertNotIn("secret.adoc", messages)
        self.assertIn("PRODUCTION CHANGE", messages)

    def test_dry_run_never_calls_model(self):
        result, model, output = self.run_main([], dry_run=True)
        model.assert_not_called()
        self.assertIsNone(result)
        self.assertIn("Does this warrant", output)

    def test_incomplete_files_refused_before_model(self):
        self.pr["changed_files"] = 3001
        result, model, output = self.run_main([])
        self.assertIsNone(result)
        model.assert_not_called()
        self.assertIn("Incomplete changed-file list", output)

    def test_skip_when_included_does_not_ask_the_model(self):
        self.cfg["judgements"].append({
            "id": "needs_issue", "title": "Issue", "question": "Issue?",
            "evidence_text": r"#[1-9][0-9]*\b", "skip_when_included": True})
        self.pr["body"] = "Resolves #123"
        result, model, _ = self.run_main([])
        self.assertNotIn("needs_issue", self.prompt(model))
        self.assertEqual(0, result["unanswered"])
        self.assertIn("Already present; not judged.", result["comment"])
        self.pr["body"] = "No issue."
        result, model, _ = self.run_main([])
        self.assertIn("needs_issue", self.prompt(model))
        self.assertEqual(1, result["unanswered"])

    def test_evidence_text_matches_title_or_body(self):
        j = {"id": "x", "title": "x", "question": "x", "evidence_text": r"\bFixes #[0-9]+"}
        self.assertTrue(TRIAGE.evidence_included({"title": "Fixes #12", "body": None}, [], j))
        self.assertTrue(TRIAGE.evidence_included({"title": "t", "body": "Fixes #12"}, [], j))
        self.assertFalse(TRIAGE.evidence_included({"title": "t", "body": "nothing"}, [], j))

    def test_all_satisfied_skips_the_model(self):
        self.cfg["judgements"] = [{
            "id": "needs_issue", "title": "Issue", "question": "Issue?",
            "evidence_title": r"#[1-9][0-9]*\b", "skip_when_included": True}]
        self.pr["title"] = "Fix #123"
        result, model, _ = self.run_main([])
        self.assertIsNone(result)
        model.assert_not_called()

    def test_applies_to_restricts_the_question_and_the_row(self):
        self.cfg["judgements"].append({
            "id": "needs_helm_readme", "title": "Helm README", "question": "README row?",
            "applies_to": ["helm/*/values.yaml"], "evidence_files": ["helm/*/README.md"]})
        self.answers["needs_helm_readme"] = {"verdict": True, "confidence": "high", "reason": "New value."}
        result, model, _ = self.run_main([])
        self.assertNotIn("needs_helm_readme", self.prompt(model))
        self.assertNotIn("needs_helm_readme", result["results"])
        self.assertNotIn("Helm README", result["comment"])
        self.assertEqual(0, result["unanswered"])
        result, model, _ = self.run_main([changed_file("helm/solr/values.yaml")])
        self.assertIn("needs_helm_readme", self.prompt(model))
        self.assertTrue(result["results"]["needs_helm_readme"]["mismatch"])
        self.assertIn("Helm README", result["comment"])

    def test_applies_to_ignores_hidden_evidence_files(self):
        # Only the evidence (README) changed: there is no visible chart change to judge.
        self.cfg["judgements"] = [{
            "id": "needs_helm_readme", "title": "Helm README", "question": "README row?",
            "applies_to": ["helm/**"], "evidence_files": ["helm/*/README.md"]}]
        result, model, output = self.run_main([changed_file("helm/solr/README.md")])
        self.assertIsNone(result)
        model.assert_not_called()
        self.assertIn("nothing to ask", output)

    def test_flag_judgement_true_is_a_mismatch(self):
        self.cfg["judgements"] = [{"id": "version_compat", "title": "Compat", "question": "Compat?"}]
        self.answers = {"version_compat": {"verdict": True, "confidence": "high", "reason": "Uses solr zk flags."}}
        result, _, _ = self.run_main([])
        row = result["results"]["version_compat"]
        self.assertIsNone(row["included"])
        self.assertTrue(row["mismatch"])
        self.assertEqual(1, result["mismatches"])
        self.assertIn("please check", result["comment"])
        self.assertIn("| yes | — |", result["comment"])

    def test_flag_judgement_false_agrees(self):
        self.cfg["judgements"] = [{"id": "version_compat", "title": "Compat", "question": "Compat?"}]
        self.answers = {"version_compat": {"verdict": False, "confidence": "medium", "reason": "Neutral."}}
        result, _, _ = self.run_main([])
        self.assertFalse(result["results"]["version_compat"]["mismatch"])
        self.assertEqual(0, result["mismatches"])
        self.assertIn("✅ All 1 checks agree", result["comment"])

    def test_all_green_collapses_to_one_line(self):
        self.cfg["judgements"].append({"id": "version_compat", "title": "Compat", "question": "Compat?"})
        self.answers["version_compat"] = {"verdict": False, "confidence": "medium", "reason": "Neutral."}
        result, _, _ = self.run_main([changed_file("docs/modules/solr-cloud/pages/tls.adoc")])
        comment = result["comment"]
        self.assertIn("✅ All 2 checks agree with what this PR includes (Documentation, Compat).", comment)
        self.assertNotIn("| Warranted?", comment)
        self.assertNotIn("Neutral", comment)
        self.assertIn("Assessed commit", comment)
        self.assertEqual(2, len(result["results"]))  # the verdicts are still in the JSON

    def test_table_is_kept_unless_every_row_agrees(self):
        self.cfg["judgements"].append({"id": "version_compat", "title": "Compat", "question": "Compat?"})
        self.answers["version_compat"] = {"verdict": True, "confidence": "low", "reason": "Maybe."}
        result, _, _ = self.run_main([changed_file("docs/modules/solr-cloud/pages/tls.adoc")])
        self.assertEqual(0, result["mismatches"])
        self.assertIn("| Warranted?", result["comment"])
        self.assertIn("❔ Compat", result["comment"])
        self.assertIn("✅ Documentation", result["comment"])

    def test_flag_judgement_low_confidence_is_uncertain(self):
        self.cfg["judgements"] = [{"id": "version_compat", "title": "Compat", "question": "Compat?"}]
        self.answers = {"version_compat": {"verdict": True, "confidence": "low", "reason": "Maybe."}}
        result, _, _ = self.run_main([])
        self.assertEqual(0, result["mismatches"])
        self.assertIn("❔ Compat", result["comment"])

    def changelog_kind(self, suggested, entry_kind=None, chart="solr-operator"):
        cfg = load_config()
        judgement_id = "needs_changelog" if chart == "solr-operator" else "needs_solr_chart_changelog"
        self.cfg["judgements"] = [judgement(cfg, judgement_id)]
        self.answers = {judgement_id: {
            "verdict": suggested, "confidence": "high", "reason": "Because."}}
        files = []
        if chart != "solr-operator":
            files.append(changed_file("helm/solr/values.yaml"))
        if entry_kind:
            patch = (f"@@ -40,0 +41,6 @@\n+    - kind: {entry_kind}\n+      description: Did a thing\n"
                     "+      links:\n+        - name: Github PR\n+          url: https://github.com/apache/solr-operator/pull/1")
            files.append(changed_file(f"helm/{chart}/Chart.yaml", "modified", patch))
        result, model, _ = self.run_main(files)
        return result, model, judgement_id

    def test_changelog_kind_differing_from_entry_is_highlighted(self):
        result, model, jid = self.changelog_kind("added", "fixed")
        row = result["results"][jid]
        self.assertTrue(row["mismatch"])
        self.assertEqual(1, result["mismatches"])
        self.assertIn("differs from the entry", result["comment"])
        self.assertIn("| added | fixed |", result["comment"])
        prompt = self.prompt(model)
        self.assertIn("exactly one of: added, changed, fixed", prompt)
        self.assertNotIn("kind: fixed", prompt)

    def test_changelog_kind_agreeing_with_entry(self):
        result, _, jid = self.changelog_kind("fixed", "fixed")
        self.assertFalse(result["results"][jid]["mismatch"])
        self.assertEqual(0, result["mismatches"])

    def test_changelog_kind_without_entry_is_missing(self):
        result, _, jid = self.changelog_kind("changed")
        self.assertTrue(result["results"][jid]["mismatch"])
        self.assertIn("| changed | no entry |", result["comment"])
        self.assertIn("missing", result["comment"])

    def test_changelog_not_worthy(self):
        result, _, jid = self.changelog_kind("(not-worthy)")
        self.assertFalse(result["results"][jid]["mismatch"])
        self.assertEqual(0, result["mismatches"])
        result, _, jid = self.changelog_kind("(not-worthy)", "security")
        self.assertTrue(result["results"][jid]["mismatch"])
        self.assertIn("probably not needed", result["comment"])

    def test_changelog_kind_outside_choices_is_unknown(self):
        result, _, jid = self.changelog_kind("bugfix", "fixed")
        self.assertEqual(1, result["unanswered"])
        self.assertIsNone(result["results"][jid]["warranted"])

    def test_solr_chart_changelog_only_when_chart_changed(self):
        self.cfg["judgements"] = [judgement(load_config(), "needs_solr_chart_changelog")]
        result, model, output = self.run_main([])
        self.assertIsNone(result)
        model.assert_not_called()
        self.assertIn("nothing to ask", output)
        result, _, jid = self.changelog_kind("added", "added", chart="solr")
        self.assertFalse(result["results"][jid]["mismatch"])

    def test_crd_kind_lines_are_not_changelog_entries(self):
        # `- kind: SolrCloud` under artifacthub.io/crds must not count as a changelog kind.
        spec = judgement(load_config(), "needs_changelog")["evidence_value"]
        patch = "@@ -1,0 +2,3 @@\n+    - kind: SolrCloud\n+      version: v1beta1\n+    - kind: fixed\n"
        values = TRIAGE.actual_values([changed_file("helm/solr-operator/Chart.yaml", "modified", patch)], spec)
        self.assertEqual(["fixed"], values)

    def test_changelog_description_is_input_to_every_judgement(self):
        self.cfg["summary_from"] = load_config()["summary_from"]
        patch = ("@@ -40,0 +41,4 @@\n+    - kind: fixed\n+      description: SOLR_TOOL_OPTS is now set for the init container\n"
                 "+      links:\n+        - name: Github PR\n")
        _, model, _ = self.run_main([changed_file("helm/solr-operator/Chart.yaml", "modified", patch)])
        prompt = self.prompt(model)
        self.assertIn("Changelog entry description: SOLR_TOOL_OPTS is now set for the init container", prompt)
        self.assertNotIn("kind: fixed", prompt)

    def test_chart_description_is_not_a_changelog_summary(self):
        self.cfg["summary_from"] = load_config()["summary_from"]
        patch = "@@ -17 +17 @@\n-description: The Solr Operator enables easy management\n+description: The Solr Operator manages Solr\n"
        _, model, _ = self.run_main([changed_file("helm/solr-operator/Chart.yaml", "modified", patch)])
        self.assertNotIn("Changelog entry description:", self.prompt(model))

    def test_no_changelog_description_without_entry(self):
        self.cfg["summary_from"] = load_config()["summary_from"]
        _, model, _ = self.run_main([])
        self.assertNotIn("Changelog entry description:", self.prompt(model))

    def test_project_context_is_in_the_system_prompt(self):
        self.cfg["project"] = load_config()["project"]
        _, model, _ = self.run_main([])
        system = model.call_args[0][0][0]
        self.assertEqual("system", system["role"])
        self.assertIn("Apache Solr Operator", system["content"])
        self.assertIn("api/v1beta1", system["content"])

    def test_exact_revision_is_reported(self):
        result, _, _ = self.run_main([])
        self.assertIn("Assessed commit `" + "a" * 40 + "`", result["comment"])

    def test_configured_evidence_families(self):
        cfg = load_config()
        samples = {
            "needs_docs": "docs/modules/solr-cloud/pages/solr-cloud-crd.adoc",
            "needs_upgrade_notes": "docs/modules/upgrade-notes/pages/upgrade-notes.adoc",
            "needs_tests": "controllers/solrcloud_controller_test.go",
            "needs_helm_readme": "helm/solr/README.md",
        }
        for j in cfg["judgements"]:
            if j["id"] in samples:
                with self.subTest(judgement=j["id"]):
                    f = changed_file(samples[j["id"]])
                    self.assertTrue(TRIAGE.evidence_included({}, [f], j))
                    f["status"] = "removed"
                    self.assertFalse(TRIAGE.evidence_included({}, [f], j))
        docs = judgement(cfg, "needs_docs")
        self.assertFalse(TRIAGE.evidence_included({}, [changed_file(samples["needs_upgrade_notes"])], docs))
        tests = judgement(cfg, "needs_tests")
        for name in ("tests/e2e/solrcloud_tls_test.go", "controllers/util/solr_util_test.go", "hack/pr-triage/test_pr_triage_llm.py"):
            self.assertTrue(TRIAGE.evidence_included({}, [changed_file(name)], tests))
        self.assertFalse(TRIAGE.evidence_included({}, [changed_file("controllers/util/solr_util.go")], tests))

    def test_configured_applies_to(self):
        cfg = load_config()
        api = [changed_file("api/v1beta1/solrcloud_types.go")]
        go = [changed_file("controllers/util/solr_util.go")]
        for jid, files, expected in (
            ("api_change_complete", api, True), ("api_change_complete", go, False),
            ("needs_helm_readme", [changed_file("helm/solr-operator/templates/deployment.yaml")], True),
            ("needs_helm_readme", [changed_file("helm/solr-operator/crds/crds.yaml")], False),
            ("needs_solr_chart_changelog", [changed_file("helm/solr/templates/solrcloud.yaml")], True),
            ("needs_solr_chart_changelog", [changed_file("helm/solr-operator/values.yaml")], False),
            ("needs_changelog", go, True), ("version_compat", go, True),
        ):
            with self.subTest(judgement=jid, files=[f["filename"] for f in files]):
                self.assertIs(expected, TRIAGE.applies(judgement(cfg, jid), files))

    def test_configured_flags_have_no_evidence(self):
        cfg = load_config()
        for jid in ("api_change_complete", "version_compat", "testing_notes"):
            self.assertIsNone(TRIAGE.evidence_included({}, [PRODUCTION], judgement(cfg, jid)))

    def test_config_is_well_formed(self):
        cfg = load_config()
        ids = [j["id"] for j in cfg["judgements"]]
        self.assertEqual(len(ids), len(set(ids)))
        for j in cfg["judgements"]:
            with self.subTest(judgement=j["id"]):
                self.assertTrue(j["title"] and j["question"])
                if j.get("choices"):
                    self.assertIn(j["absent_choice"], j["choices"])
                    self.assertIn("evidence_value", j)
                    TRIAGE.re.compile(j["evidence_value"]["pattern"])
        TRIAGE.re.compile(cfg["summary_from"]["pattern"])

    def test_diff_budget_prioritises_production_and_excludes_patches(self):
        go = changed_file("controllers/solrcloud_controller.go", patch="GO")
        readme = changed_file("README.md", patch="README")
        lock = changed_file("go.sum", patch="LOCK")
        binary = changed_file("image.png", patch=None)
        budget = len("--- controllers/solrcloud_controller.go (modified)\nGO\n")
        diff, omitted = TRIAGE.build_diff([readme, lock, binary, go], {
            "max_diff_chars": budget, "exclude_patches": ["go.sum"], "show_first": ["controllers/**"],
        })
        self.assertIn("GO", diff)
        self.assertNotIn("README", diff)
        self.assertCountEqual([readme["filename"], lock["filename"], binary["filename"]], omitted)
        self.assertLessEqual(len(diff), budget)

    def test_configured_exclusions_hide_generated_files(self):
        cfg = load_config()
        for name in ("go.sum", "dependency_licenses.csv", "api/v1beta1/zz_generated.deepcopy.go",
                     "config/crd/bases/solr.apache.org_solrclouds.yaml", "helm/solr-operator/crds/crds.yaml"):
            with self.subTest(name=name):
                _, omitted = TRIAGE.build_diff([changed_file(name, patch="X")], cfg)
                self.assertEqual([name], omitted)
        diff, omitted = TRIAGE.build_diff([changed_file("config/rbac/role.yaml", patch="RBAC")], cfg)
        self.assertIn("RBAC", diff)

    def test_model_request_is_mocked_at_http_boundary(self):
        response = io.StringIO(json.dumps({"choices": [{"message": {"content": "{}"}}]}))
        with mock.patch.dict("os.environ", MODEL_ENV), mock.patch.object(TRIAGE.urllib.request, "urlopen", return_value=response) as http:
            content, _ = TRIAGE.call_model([], {})
        self.assertEqual("{}", content)
        request = http.call_args[0][0]
        self.assertEqual("https://example.invalid/v1/chat/completions", request.full_url)
        self.assertEqual({"type": "json_object"}, json.loads(request.data)["response_format"])

    def test_response_header_says_checked_once(self):
        result, _, _ = self.run_main([])
        self.assertIn("not updated on later pushes", result["comment"])

    def test_verdicts_file_replaces_model(self):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            (temp / "config.json").write_text(json.dumps(self.cfg), encoding="utf-8")
            (temp / "v.json").write_text(json.dumps(self.answers), encoding="utf-8")
            args = ["pr_triage_llm.py", "--config", str(temp / "config.json"),
                    "--verdicts", str(temp / "v.json"), "--out", str(temp / "out.json"),
                    "--from-git", "main"]
            with mock.patch("sys.argv", args), contextlib.redirect_stdout(io.StringIO()):
                with mock.patch.object(TRIAGE, "pr_from_git", return_value=(self.pr, [PRODUCTION])):
                    with mock.patch.object(TRIAGE, "call_model") as model:
                        TRIAGE.main()
            model.assert_not_called()
            result = json.loads((temp / "out.json").read_text(encoding="utf-8"))
            self.assertTrue(result["results"]["needs_docs"]["warranted"])


class FromGitTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.repo = Path(self.directory.name)
        self.git("init", "-q", "-b", "main")
        self.write("controllers/old.go", "package controllers\n")
        self.write("controllers/gone.go", "package controllers\n")
        self.commit("base")

    def git(self, *args):
        env = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@e", GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@e")
        subprocess.run(["git", *args], cwd=self.repo, env=env, check=True, capture_output=True)

    def write(self, name, text):
        path = self.repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)

    @contextlib.contextmanager
    def in_repo(self):
        cwd = os.getcwd()
        os.chdir(self.repo)
        try:
            yield
        finally:
            os.chdir(cwd)

    def test_commits_staged_unstaged_and_untracked(self):
        self.git("switch", "-q", "-c", "feature")
        self.write("controllers/old.go", "package controllers\n// changed\n")
        self.commit("Change old\n\nBecause reasons.")
        self.git("rm", "-q", "controllers/gone.go")
        self.write("controllers/staged.go", "package controllers\n")
        self.git("add", "controllers/staged.go")
        self.write("helm/solr-operator/Chart.yaml", "annotations: {}\n")
        with self.in_repo():
            pr, files = TRIAGE.pr_from_git("main")
        self.assertEqual("Change old", pr["title"])
        self.assertIn("Because reasons.", pr["body"])
        self.assertEqual(
            {"controllers/old.go": "modified", "controllers/gone.go": "removed",
             "controllers/staged.go": "added", "helm/solr-operator/Chart.yaml": "added"},
            {f["filename"]: f["status"] for f in files})
        old = next(f for f in files if f["filename"] == "controllers/old.go")
        self.assertTrue(old["patch"].startswith("@@"))
        self.assertEqual(1, old["additions"])

    def test_detects_the_branch_it_stemmed_from(self):
        self.git("switch", "-q", "-c", "release-0.9")
        self.write("controllers/nine.go", "package controllers\n")
        self.commit("on 0.9")
        self.git("switch", "-q", "-c", "feature")
        self.write("controllers/feature.go", "package controllers\n")
        self.commit("feature work")
        self.git("switch", "-q", "main")
        self.write("controllers/main.go", "package controllers\n")
        self.commit("main moves on")
        self.git("switch", "-q", "feature")
        with self.in_repo():
            self.assertEqual("release-0.9", TRIAGE.detect_base())
            self.git("switch", "-q", "-c", "from-main", "main")
            self.assertEqual("main", TRIAGE.detect_base())

    def test_no_commits_uses_branch_name(self):
        self.git("switch", "-q", "-c", "my-branch")
        self.write("controllers/new.go", "package controllers\n")
        with self.in_repo():
            pr, files = TRIAGE.pr_from_git("main")
        self.assertEqual("my-branch", pr["title"])
        self.assertEqual(["controllers/new.go"], [f["filename"] for f in files])


if __name__ == "__main__":
    unittest.main()
