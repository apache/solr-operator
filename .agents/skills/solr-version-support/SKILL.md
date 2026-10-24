---
name: solr-version-support
description: Adopt a new Apache Solr release in the Solr Operator - bump the default Solr version everywhere it is duplicated, decide whether the release needs operator work, extend the Solr compatibility matrix, and record the change in both Helm charts' artifacthub.io/changes changelog. Nothing in CI checks any of this.
when_to_use: The user asks to support, adopt, test against or default to a new Solr version, or to bump DefaultSolrVersion or the solr chart's appVersion. Also when a new Solr release is out and the operator should follow it.
---

# Adopt a new Solr version

The version string is duplicated across 14 files and **no make target validates it**, so a partial
bump passes CI. The compatibility matrix and the changelog are hand-maintained and are the steps
most often skipped. Read `AGENTS.md` first — where it and this skill disagree, the repo wins.

## Invocation

`/solr-version-support 9.11.0`, or prose ("adopt Solr 9.11"). The target version is the only input.

If it is omitted, do not ask blankly. List what is newer than the current default and offer those:

```bash
grep DefaultSolrVersion api/v1beta1/solrcloud_types.go
curl -s 'https://hub.docker.com/v2/repositories/library/solr/tags?page_size=100' |
  grep -oE '"name":"[0-9]+\.[0-9]+\.[0-9]+"' | cut -d'"' -f4 | sort -V | tail -5
```

Validate before touching anything:

- The tag must exist: `curl -fsS "https://hub.docker.com/v2/repositories/library/solr/tags/<version>/" >/dev/null`
- A target older than the current default is a downgrade. Confirm it is deliberate; do not treat it
  as a typo and silently correct it.
- A **major** Solr bump (9.x → 10.x) is not this skill. Expect removed APIs and config changes;
  that is a deliberate project decision with its own issue and PR.

## Step 0: branch from upstream main

```bash
git remote -v | awk '/apache\/solr-operator(\.git)? \(fetch\)/ {print $1}'
git status --short
git fetch <remote> main
git switch -c solr-<version> <remote>/main
```

The remote is `upstream` in some clones, `apache` or `origin` in others; if the first command
prints nothing, ask. Branch from `<remote>/main`, never local `main`.

## The 14 files

`DefaultSolrVersion` in `api/v1beta1/solrcloud_types.go` is the operator-side default. Every other
site repeats the same string with nothing enforcing agreement:

| File | What |
|---|---|
| `api/v1beta1/solrcloud_types.go` | `DefaultSolrVersion` — the real default |
| `helm/solr/Chart.yaml` | `appVersion`, and the `artifacthub.io/images` example |
| `helm/solr/README.md` | 3 sites, including the values table default |
| `helm/solr-operator/Chart.yaml` | `crdsExamples`, for SolrCloud and SolrPrometheusExporter |
| `example/test_solrcloud.yaml` and 4 more `example/*.yaml` | one `tag:` each |
| `tests/scripts/manage_e2e_tests.sh` | the `-s` default and its help text |
| `hack/release/smoke_test/test_cluster.sh` | the same pair |
| `tests/e2e/suite_test.go` | the e2e default |
| `docs/modules/getting-started/pages/local-tutorial.adoc` | 3 sites in tutorial commands |
| `dev-docs/e2e-testing.md` | 1 example |

`appVersion` means different things in the two charts: in `helm/solr` it is the **Solr** version and
is yours; in `helm/solr-operator` it is the **operator** version and belongs to the release process.
Never touch the latter here.

After editing, `git grep -n '<previous version>'` must return only deliberate historical references
— upgrade notes, and older `artifacthub.io/changes` entries. Review every remaining hit; there is no
check target to catch one you missed.

## Does the release need operator work?

The mechanical bump is not the deliverable. Read the Solr release notes for the span crossed and
check whether anything the operator depends on moved:

- **The admin APIs it calls.** `admin/info/system`, `admin/info/health`, `admin/collections` and the
  v2 `/api/cluster/replicas/balance`, in `controllers/util/solr_api/` and `controllers/util/`.
- **Startup and probes** — `solr_util.go`.
- **security.json handling** — `solr_security_util.go`.
- **TLS** — `solr_tls_util.go`.
- **Rolling updates and scaling**, which read cluster state — `solr_update_util.go`,
  `solr_scale_util.go`.
- New config knobs worth exposing as CRD fields. That is a separate change; see the
  `add-crd-field` skill rather than folding it in here.

Then run the e2e suite against the new image, because **the e2e workflow is permanently disabled in
CI** and proves nothing:

```bash
make e2e-tests                                   # uses the new default
tests/scripts/manage_e2e_tests.sh run-tests -s <version> -i <operator image>
```

Say in the PR that you ran it, and on which Kubernetes version.

## The compatibility matrix

`docs/modules/upgrade-notes/pages/upgrade-notes.adoc` carries a hand-maintained Solr matrix, columns
`7.7` / `8.0`-`8.10` / `8.11` / `9.0`-`9.3` / `9.4`+ with a row per operator version. Nothing
regenerates or verifies it. If the new release falls outside the existing columns, add one; if the
current operator row does not yet cover it, extend it. A matrix that silently stops being true is
worse than no matrix.

## Changelog: both charts

Add an `artifacthub.io/changes` entry to **both** `helm/solr-operator/Chart.yaml` and
`helm/solr/Chart.yaml`. Tight wording, one line per entry, no prose.

This is user-visible and not optional: `helm/solr/values.yaml` ships `tag: ""`, so any SolrCloud
that did not pin its own image follows `DefaultSolrVersion`. **Bumping the default changes the Solr
version those users get on their next operator upgrade.** Say so explicitly.

Add a `fixed` or `security` entry as well if the Solr release addresses a CVE that reaches
operator-managed deployments, and flip `artifacthub.io/containsSecurityUpdates` to `"true"` in that
chart.

## Verify before you commit

1. `make lint` — serially, never `make -j`.
2. `make test`.
3. `git grep -n '<previous version>'`, and account for every hit.
4. `git status --short`, then stage deliberately by naming paths. **Never `git add -A`**. Confirm
   both `git diff --exit-code` and `git status --short` are clean.

## Commit, then ask before pushing

One commit. Imperative subject under ~70 characters, at most a sentence of body.

The fork is the remote whose URL is not `apache/solr-operator`
(`git remote -v | grep -v apache/solr-operator`). **Never push without the user agreeing in this
conversation**, and never push to the apache remote.

PR description, well under a screenful: the version adopted; whether the release needed operator
work and what; what the matrix now says; the changelog entries; and what you ran, including
`make e2e-tests` and the Kubernetes version, since CI's e2e is disabled.

## Out of scope

- **Raising the *minimum* supported Solr version.** A support promise, not a version bump — its own
  PR, timed to a release, with upgrade notes.
- **A major Solr release (10.x).** See Invocation.
- **New CRD fields** for new Solr config. Use the `add-crd-field` skill.
- **`appVersion` in `helm/solr-operator/Chart.yaml`** and the operator image tag — release process,
  via `hack/release/version/propagate_version.sh`.
