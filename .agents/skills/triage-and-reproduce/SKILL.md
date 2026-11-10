---
name: triage-and-reproduce
description: Triage an incoming Solr Operator bug report - classify it as an operator, Helm chart, Solr or configuration problem, reproduce it at the cheapest level that can show it, and propose one routed outcome (issue type, labels, reviewers, draft comment). Proposes only; never writes to GitHub on its own.
when_to_use: The user asks to triage, investigate or reproduce a bug report in this repository, asks whether a reported problem is an operator bug, or asks where an issue belongs. Not for feature requests, and not for security reports - those follow SECURITY.md.
---

# Triage a bug report

Classify before reproducing, reproduce at the cheapest level that can show the bug, then propose one
outcome. Most reports are decided without ever starting a cluster. Read `AGENTS.md` first - where it
and this skill disagree, the repo wins.

## Before you start

- **Bugs only.** A feature request: say so and stop.
- **Security reports go elsewhere:** `SECURITY.md` for the ASF process, `THREAT_MODEL.md` §13 for
  dispositions and §11a for recurring false positives. Do not triage them here.
- **Nothing reaches GitHub without an explicit go in the conversation.** No comment, label, type,
  close or push. Produce one proposal with the exact draft text and wait.
- A fix is a separate task. This skill ends at the proposal.

## 1. Pull the facts

```bash
gh issue view <issue> -R apache/solr-operator --comments
```

Five facts decide almost every report. Anything missing that would change the answer makes the
info request the outcome, not a guess:

| Fact | Why it decides things |
|---|---|
| Operator version (chart or image tag) | The behaviour may already have changed; check the current code before trusting the report's version. |
| Kubernetes version | Pod, PVC and securityContext behaviour differs by version. |
| Solr image tag | Solr 8/9/10 differ in `bin/solr` flags, `solr.xml` and the metrics endpoint. |
| The `SolrCloud`/`SolrBackup`/`SolrPrometheusExporter` spec | Without it you cannot generate the objects to compare against. |
| `dataStorage` (ephemeral vs persistent) and `updateStrategy` | Decides whether replica eviction and re-balancing are expected at all. |

For "wrong output" reports, also ask for the generated object (`kubectl get statefulset <name> -o yaml`).

## 2. Classify before reproducing

| Class | Signal | Goes to |
|---|---|---|
| Operator bug | Generated objects do not match the spec, or the reconcile loop misbehaves (rolling update, scaling, backup). | This repo. |
| Chart bug | The CRD field works, but a Helm value never reaches it - usually `helm/solr/templates/_custom_option_helpers.tpl`. | This repo, `helm` label. |
| Solr bug | Reproducible against the same image outside Kubernetes (`docker run --rm solr:<tag> …`). | `apache/solr`. |
| Configuration or docs | Objects are correct and Solr behaves as configured. The largest class; the answer is a docs link. | Close with the link. |

The discriminator: **if the generated Kubernetes objects match the spec and Solr still misbehaves, it
is not an operator bug.** `THREAT_MODEL.md` §1-§3 draws the same line - this repo is the operator,
not the search server.

## 3. Reproduce at the cheapest level that can show it

Start at the top. Only go down a level when the level above cannot show the reported symptom.

| Level | How | Cost |
|---|---|---|
| Generated output | A throwaway Go test calling the generator directly (`GenerateStatefulSet`, `GenerateSolrXMLStringForCloud`, `GenerateCommonService`, …) and printing the result as YAML. Delete it afterwards. | seconds |
| Chart output | `make helm-dependency-build` once, then `helm template helm/solr --set <the reporter's values>`. | seconds |
| Reconcile behaviour | `make unit-tests` - envtest boots a real API server with the real CRDs, so controller behaviour is testable without a cluster. | ~2 min |
| Runtime behaviour | `make e2e-tests`, narrowed with `TEST_FILES` and `TEST_FILTER`. See `dev-docs/e2e-testing.md`. | ~10 min |
| Release packaging | `make smoke-test` - only for packaging, chart install or release-artifact reports. | ~40 min |

Pin the reporter's versions. The harnesses default differently, and reproducing on the wrong one
wastes a cycle:

| Harness | Kubernetes default | Solr default | Override |
|---|---|---|---|
| `tests/scripts/manage_e2e_tests.sh` | `v1.33.7` | `solr:9.11.0` | `-k`, `-s`, `-i` |
| `hack/release/smoke_test/test_cluster.sh` | `v1.26.6` | the release's default | `-k`, `-t` |

Three traps worth knowing before you spend time:

- Every spec uses focused Ginkgo nodes (`FDescribe`, `FContext`, `FIt`), so running
  `go test ./controllers/...` by hand fails on programmatic focus. Use `make unit-tests`, or export
  `GINKGO_EDITOR_INTEGRATION=true`.
- The e2e script needs the repo's pinned `bin/kind`; an older `kind` on `PATH` cannot create the
  node images it asks for.
- CI's PR e2e job runs only `Label("pr")` specs, on one Kubernetes and Solr version, so a green PR is
  no evidence about the rest. The full suite is a manual `workflow_dispatch` on `e2e-tests.yaml` with
  an empty label filter, which also takes a Solr and Kubernetes version.

## 4. Propose one outcome

| Outcome | The proposal contains |
|---|---|
| Reproduced | Class, the exact reproduction command, issue type and area labels, suggested reviewers, and a short comment stating what reproduces on which versions. |
| Not reproducible | A draft comment asking for precisely the facts missing from step 1 - nothing else. |
| Duplicate | The issue it duplicates, a draft close comment, and the `duplicate` label. |
| Not an operator bug | Where it belongs, and a draft comment saying so with the docs link or tracker. |

Type and labels - the repo uses GitHub issue **types** (`Bug`, `Feature`, `Task`); there is no `bug`
label:

```bash
gh issue edit <issue> -R apache/solr-operator --type Bug --add-label cloud
```

Area labels already in use: `backup`, `restore`, `cloud`, `helm`, `metrics`, `networking`,
`security`, `storage`, `zookeeper`, `autoscaling`, `controller operations`, `custom kube options`,
`docker`, `documentation`, `testing`.

Suggest reviewers from recent activity on the touched paths, never from a hardcoded list:

```bash
git log --since=12.months --format='%an' -- <path> | sort | uniq -c | sort -rn | head
```
