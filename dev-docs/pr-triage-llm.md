<!--
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
-->

# LLM-assisted PR triage

When a PR is opened, or a draft is marked ready for review, the `PR Triage (LLM)` workflow
asks a model through the ASF gateway (`llm.apache.org`) the questions in
`.github/pr-triage/judgements.json`: does this change warrant a changelog entry (and of which
`kind`), a chart README row, user documentation, upgrade notes, tests; is a new CRD option
complete; does it depend on a Solr or Kubernetes version newer than the oldest supported one;
does a change that needs a running cluster say how it was tested? It posts one advisory comment
comparing each verdict with what the PR contains, one bullet per judgement, and never blocks
merging. Files that count as evidence (changelog, docs, tests, READMEs) are hidden from the model
so it judges what the change *warrants*, not what the author included. Borrowed from
`dev-tools/scripts/pr-triage-llm.py` in apache/solr.

## Running it locally

```bash
# The prompt for the work on your branch, without calling the model
python3 hack/pr-triage/pr_triage_llm.py --config .github/pr-triage/judgements.json --from-git --dry-run --out /dev/null

# Real verdicts (LLM_BASE_URL selects another OpenAI-compatible endpoint; --debug shows the raw reply)
export LLM_API_KEY=... LLM_MODEL=qwen3.8-27b
python3 hack/pr-triage/pr_triage_llm.py --config .github/pr-triage/judgements.json --from-git --out result.json
jq -r .comment result.json

# Offline tests
python3 -m unittest discover -s hack/pr-triage -p 'test_*.py'
```

## Setup and tuning

Repository secret `ASF_LLM_API_KEY` (an automation key for `llm.apache.org`, from ASF Infra;
without it the workflow skips) and, optionally, repository variable `LLM_MODEL`.

The questions and their evidence globs live in `judgements.json`; its `_doc` key describes the
format. Keep questions concrete and name the files and conventions involved: the model is small
and answers only as well as the question.
