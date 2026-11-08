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

When a pull request is opened (or a draft is marked ready for review), the
`PR Triage (LLM)` workflow (`.github/workflows/pr-triage-llm.yaml`) asks a language model,
through the ASF LLM gateway at `llm.apache.org`, a fixed list of questions about the change and
posts **one advisory comment** comparing each answer with what the PR actually contains.
It runs when the PR is opened and each time a draft is marked ready for review (not on later
pushes), never blocks merging, and posts nothing if the model gives no usable answer.

The idea and the script are borrowed from the Apache Solr project
(`dev-tools/scripts/pr-triage-llm.py` and `.github/pr-triage/judgements.json` in apache/solr),
adapted to the conventions of this repository.

## What it checks

The questions live in `.github/pr-triage/judgements.json`. They encode the feedback that
contributor PRs to this repository have historically received from reviewers, which CI does
not catch:

| Judgement | Compared with |
|---|---|
| Does the change warrant a changelog entry, and of which `kind`? | `artifacthub.io/changes` in `helm/solr-operator/Chart.yaml` |
| Same for a change to the `solr` chart | `helm/solr/Chart.yaml` (only asked when that chart changed) |
| Does a new or changed Helm value need a row in the chart README? | `helm/*/README.md` (only asked when values or templates changed) |
| Should the user documentation be updated? | `docs/modules/**` |
| Must users know about this when upgrading the operator? | `docs/modules/upgrade-notes/**` |
| Should the change come with tests? | `*_test.go`, `tests/e2e/**` |
| Is a new CRD option complete: wired through the controllers, kubebuilder markers, exposed in the `solr` chart? | nothing; a `true` verdict is the finding |
| Does the change depend on a Solr or Kubernetes version newer than the oldest supported one? | nothing; a `true` verdict is the finding |
| Does a change that can only be verified against a running cluster describe how it was tested? | nothing; a `true` verdict is the finding |

Files that count as evidence (changelog, docs, tests, READMEs) are hidden from the model, so it
judges what the change *warrants* rather than noticing what the author included. Generated
files (CRDs, deepcopy, `go.sum`, the license list) are listed by name only; `make lint` already
checks that they are current.

The comment has one bullet per judgement. A bullet that agrees with the PR just says what the
PR has (`included`, `not needed`, the changelog `kind`, or `no concern` for the checks that have
nothing to compare with); the model's reasoning is kept in an HTML comment, visible only in the
Markdown source. A bullet that disagrees is marked ⚠️ (or ❔ when the model was unsure or gave no
usable answer) and shows what the model suggests, what the PR has, and why. When every judgement
agrees, the comment collapses to a single line.

The comment also points out PRs not targeting `main`, and PRs from personal forks with
"Allow edits from maintainers" turned off, since maintainers often add the changelog entry
themselves before merging.

## Running it locally

The script needs only Python 3 and `git`. To see the prompt the model would get for the
work on your current branch, without calling anything:

```bash
python3 hack/pr-triage/pr_triage_llm.py --config .github/pr-triage/judgements.json \
    --from-git --dry-run --out /dev/null
```

`--from-git` compares against the `main` or `release-*` branch your branch stems from
(or give it explicitly: `--from-git main`), and includes staged, unstaged and untracked
files. With an API key you can get real verdicts:

```bash
export LLM_API_KEY=...        # a key for llm.apache.org, or any OpenAI-compatible endpoint
export LLM_MODEL=qwen3.8-27b  # and LLM_BASE_URL to use another endpoint
python3 hack/pr-triage/pr_triage_llm.py --config .github/pr-triage/judgements.json \
    --from-git --out result.json
jq -r .comment result.json
```

The offline tests run with:

```bash
python3 -m unittest discover -s hack/pr-triage -p 'test_*.py'
```

## Setup in the repository

- Repository secret `ASF_LLM_API_KEY`: an automation key for `llm.apache.org`, requested
  from ASF Infra. Without it the workflow skips with a notice, so forks are unaffected.
- Optional repository variable `LLM_MODEL` to pick another model offered by the gateway.

The workflow uses `pull_request_target` so that PRs from forks can use the secret. It never
checks out or runs PR code: the PR title, description and diff are fetched through the API
and passed to the model as text, and the job that holds the key has a read-only token while
the job that posts the comment never sees the key.

## Tuning

Edit `.github/pr-triage/judgements.json`; its `_doc` key describes the format. A judgement
can be a yes/no question compared with the presence of evidence files, a choice question
compared with a value extracted from the diff (the changelog `kind`), or a flag with no
evidence at all. `applies_to` restricts a question to PRs that touch certain files. Keep
questions concrete and name the files and conventions involved: the model is small, and
answers only as well as the question.
