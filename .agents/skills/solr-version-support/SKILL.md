---
name: solr-version-support
description: Adopt a new Apache Solr release in the Solr Operator - bump the default Solr version everywhere it is duplicated, decide whether the release needs operator work, extend the Solr compatibility matrix, and record the change in both Helm charts' artifacthub.io/changes changelog. Covers a new Solr major version too, where support is gated on the major rather than carried by a version string. Sizes each piece of work and stops, filing issues instead of a PR, when a release needs more than one pass. Nothing in CI checks any of this.
when_to_use: The user asks to support, adopt, test against or default to a new Solr version or a new Solr major version, or to bump DefaultSolrVersion or the solr chart's appVersion. Also when a new Solr release is out and the operator should follow it.
---

# Adopt a new Solr version

The version string is duplicated across 14 files and **no make target validates it**, so a partial
bump passes CI. The compatibility matrix and the changelog are hand-maintained and are the steps
most often skipped. Read `AGENTS.md` first — where it and this skill disagree, the repo wins.

## Invocation

`/solr-version-support <version>`, or prose ("adopt Solr 9.11"). The version is the only input.

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
- A **major** Solr release (9.x → 10.x) takes a different path through this skill — see "A new
  Solr major version". Do not treat it as a bigger patch bump.

## Step 0: branch from upstream main

```bash
git remote -v | awk '/apache\/solr-operator(\.git)? \(fetch\)/ {print $1}'
git status --short
git fetch <remote> main
git switch -c solr-<version> <remote>/main
```

The remote is `upstream` in some clones, `apache` or `origin` in others; if the first command
prints nothing, ask. Branch from `<remote>/main`, never local `main`.

## Do the work

1. Validate the target version. A major takes the other path — see "A new Solr major version".
2. Read what changed across the whole span crossed, then size every change it implies.
   **Stop at L**, and follow the halt rules below.
3. Edit the 15 version sites.
4. Do any operator work sized M or below.
5. `make test`, and repair fixtures.
6. `make e2e-tests` against the new default; CI's e2e is disabled, so this is the only evidence.
7. Extend the compatibility matrix.
8. Add the changelog entries to both charts.
9. `make lint`, re-grep the previous version, and verify.

The sections below are reference for each step, not an order to read them in.

## Size the work before editing anything

Size every operator change the release implies, separately. The release's size is the largest of
them.

| Size | Shape |
|---|---|
| XS | Version strings only, no operator change — the routine patch release |
| S | A default, env var or flag the operator already sets; one or two files, no CRD change |
| M | One new CRD field end to end — ~12-15 files including both charts, docs and tests |
| L | Behaviour gated across several subsystems, or a removal the operator must work around — Solr 10's `solr.xml`, env vars, CLI and probes together |
| XL | A removed feature with no in-place replacement, or anything needing new API design or a migration path for running clusters — `solr-exporter` dropped in Solr 10 (#820) |

**Attempt XS, S and M. Stop at L.** State the size and the reasoning for each item before you start
editing, so the cut line is visible rather than implied.

### A minor release with an L or XL item

Take the minor anyway. Do the XS/S/M work, land the version bump, and file one issue per L/XL item
for what is not natively supported. Say in the PR which features are unsupported and link the
issues — otherwise the matrix and the changelog imply more than the operator delivers.

### A major release with an L or XL item

Halt. Do not open a PR, do not part-gate the major. The outcome is one issue per item, each
solvable on its own; the skill is re-run once they are closed. A half-gated major is worse than an
unstarted one, because the operator then claims support it does not have.

### Filing those issues

Never file without approval. Present each draft one at a time — title, the Solr change, the
operator touchpoints, the size and why — and wait for an explicit go. Search for an existing open
issue first; a duplicate is worse than a missing one.

## The 15 version sites

`DefaultSolrVersion` in `api/v1beta1/solrcloud_types.go` is the operator-side default. Every other
site repeats it with nothing enforcing agreement. Eleven rows, fifteen files:

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
| `docs/modules/getting-started/pages/local-tutorial.adoc` | Keep the install comment and image tag on an older compatible version; set the upgrade tag to the target. The initial version is a deliberate historical reference for the final grep. |
| `dev-docs/e2e-testing.md` | 1 example |
| `.github/workflows/e2e-tests.yaml` | the `solr-version` matrix — **major only** (`'9'`), so the grep below cannot see it |

`appVersion` means different things in the two charts: in `helm/solr` it is the **Solr** version and
is yours; in `helm/solr-operator` it is the **operator** version and belongs to the release process.
Never touch the latter here.

After editing, `git grep -n '<previous version>'` must return only deliberate historical references
— upgrade notes, and older `artifacthub.io/changes` entries. Review every remaining hit; there is no
check target to catch one you missed. The grep is not sufficient on its own: the e2e workflow pins
the major alone, so check that file by hand every time.

## Does the release need operator work?

The mechanical bump is not the deliverable.

### Read the whole span, not just the target

Adopting 9.11.0 off a 9.10.0 default means reading **9.10.1 and 9.11.0**. The intermediate patch is
usually where the security fixes sit — every CVE a 9.11.0 adoption picks up was fixed in 9.10.1 — so
reading only the target gets the changelog's security entry wrong.

Do not go looking for those fixes in `CHANGELOG.md`. A vulnerability is disclosed *after* the
release that fixes it, so a release's section normally carries no `### Security` subsection even
when it does fix something, and an absent one tells you nothing either way.

### Where to read it

Three sources, and they are not interchangeable:

| Source | What it gives |
|---|---|
| `solr/solr-ref-guide/modules/upgrade-notes/pages/major-changes-in-solr-<major>.adoc` in `apache/solr` git | **Start here.** One `== Solr <x>.<y>` section per release — patch releases get one when they need it (`== Solr 9.8.1`) — written for upgraders: removals, deprecations, renames, and what to do about each. Its `=== Rolling Upgrades` section is where cross-version constraints land, and those are usually operator work: the Solr 8→9 PKI three-restart sequence is the shape of the thing |
| `https://raw.githubusercontent.com/apache/solr/main/CHANGELOG.md` | Every release in one markdown file, `[9.11.0] - 2026-10-04` headings over `### Added` / `Changed` / `Fixed` / `Deprecated` / `Removed` / `Security` / `Other`. Greppable, and far easier than the per-release `Changes.html`. Complete, but it is a commit list — it says a thing changed, not what an upgrader must do. Fetch it **raw**: at ~1.5 MB it exceeds the GitHub contents API's 1 MB cap and that endpoint returns empty content |
| `https://solr.apache.org/feeds/solr/security.atom.xml` | **The source for CVEs.** One `<entry>` per advisory — a date, a title carrying the CVE ID, and a link to the advisory itself — so it is far easier to line up against a release date than the HTML. `https://solr.apache.org/security-news.html` is the same set rendered, and each entry carries the affected range, the fixed-in version and the precise conditions for being impacted. Read those conditions: CVE-2026-22022 only bites a `RuleBasedAuthorizationPlugin` permission list that omits the `all` predefined permission, which the operator's generated `security.json` defines, so operator defaults are not affected. Fetch and grep these rather than trusting a summary — the index page and the per-advisory entries disagree about which title goes with which ID, and not every advisory has a CVE in its title |

**Advisory timing does not track the release**, so never infer the mapping — each advisory states
its own affected range and fixed-in version, and that is the only thing to trust. CVE-2026-22022
and CVE-2026-22444 were published the day 9.10.1 shipped; CVE-2025-66516 six weeks before it, with
an interim config mitigation; the JWT `blockUnknown` advisory months before the 9.11.0 release that
changed the default. The feed is usually current within a couple of weeks of a release, which means
**in the first days after a Solr release it cannot yet show everything that release fixes**. Say
that in the PR rather than implying the release carries no security fixes, and look again before the
operator release that ships the bump.

A CVE can also be superseded: CVE-2025-66516 supersedes CVE-2025-54988, and `CHANGELOG.md` records
that fix under the **older** ID. Searching the changelog for the ID an advisory gave you can
therefore come up empty. Trust the advisory for the ID, the changelog for the release it landed in.

### What to check against

Then check whether anything the operator depends on moved:

- **The admin APIs it calls.** `admin/info/system`, `admin/info/health`, `admin/collections` and the
  v2 `/api/cluster/replicas/balance`, in `controllers/util/solr_api/` and `controllers/util/`.
- **Startup and probes** — `solr_util.go`.
- **security.json handling** — `solr_security_util.go`.
- **TLS** — `solr_tls_util.go`.
- **Rolling updates and scaling**, which read cluster state — `solr_update_util.go`,
  `solr_scale_util.go`.
- New config knobs worth exposing as CRD fields. In scope at size M — deliver the field end to
  end. (A dedicated `add-crd-field` skill is proposed in #867 but does not exist yet; until it
  does, follow an existing field such as `enableServiceLinks` in #790 for the file set.)

Within a major, differences are usually additive and need no gating. Across a major, they do — see
below.

Then run the e2e suite against the new image, because **the e2e workflow is permanently disabled in
CI** and proves nothing:

```bash
make e2e-tests                                             # after the bump: picks up the new default
tests/scripts/manage_e2e_tests.sh run-tests -s <version>   # before it: pins the version explicitly
```

The script's Kubernetes default has drifted from the smoke test's, so state which you used; `-k`
overrides it. Say in the PR that you ran it, and on which Kubernetes version.

## A new Solr major version

Supporting a new major is **not** the same decision as making it the default, and the two should
not land together. Support means the operator can run it; the default stays on the proven major
until it is. Work out which you are being asked for, and say so.

A major removes and renames things, so the operator needs version-conditional code rather than a
new version string. The mechanism arrived in #827 (`solr-10-compat`): `SolrCloud.SolrMajorVersion()
(int, error)` reads the major from the image tag, behaviour differences are gated on it, and a
major the operator does not support is **not reconciled** — it gets an `UnsupportedSolrVersion`
warning event. Re-read the current code before relying on these names; #827 was unmerged when this
was written.

Consequences worth knowing:

- **The image tag must start with the Solr version**, and only its first number is read, so a
  bare major works: `10`, `10.0.0`, `<x>.<y>.<z>-slim` and `10.0.0@sha256:…` are all fine. This is
  why `.github/workflows/e2e-tests.yaml` can pin `solr:9`. `latest`, or a date-based tag like
  `2026.10.1`, is not, and stops reconciliation.
- Where differences land, from the Solr 10 work: `solr.xml` (`DefaultSolrXMLForSolr10`), env var
  names (`SOLR_PORT_LISTEN`, `SOLR_HOST_ADVERTISE`), module paths under `sharedLib`, CLI commands
  (`solr cluster --property` replacing the removed `zkcli.sh`), and probe invocation
  (`solr api --solr-url`, `--credentials`).
- A major can remove a feature outright — Solr 10 dropped `solr-exporter`, so the Prometheus
  exporter has no Solr 10 support (#820). Say so rather than quietly leaving it broken.

For each removal or rename in the Solr upgrade notes, find the operator touchpoint and size it. If
everything comes in at M or below: gate each on the major, unit-test **both** majors, run e2e
against both, add a matrix column, write upgrade notes, and give the major its own PR. If anything
is L or XL — which a major usually has — halt and file issues instead.

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
operator-managed deployments, and flip `artifacthub.io/containsSecurityUpdates` to `"true"` in the
chart that carries it.

**Nothing propagates between the two charts.** They are separate Artifact Hub packages: a `security`
entry and a `"true"` flag on `helm/solr` produce no security notice whatsoever on
`helm/solr-operator`. Decide for each chart on its own terms, by asking whose users the claim is
about. For a default-version bump the Solr chart is the one making a security claim, because it
ships the Solr version; the operator chart's own code carries no fix, so its flag normally stays
`"false"`. Operator-chart readers still deserve the information — put the CVE list in that chart's
`changed` entry instead, and do not leave a `kind: security` entry sitting under a `"false"` flag.

## Verify before you commit

1. `make lint` — serially, never `make -j`.
2. `make test`.
3. `git grep -n '<previous version>'`, and account for every hit.
4. `git status --short`, then stage deliberately by naming paths. **Never `git add -A`**. Confirm
   both `git diff --exit-code` and `git status --short` are clean.

## Commit, then ask before pushing

One commit. Imperative subject under ~70 characters, at most a sentence of body.

The fork is the remote whose URL is not `apache/solr-operator`:

```bash
git remote -v | grep -v apache/solr-operator | awk '{print $1}' | sort -u
```

Clones often hold other contributors' forks too, so more than one result — or none — means ask
which to use rather than taking the first. **Never push without the user agreeing in this
conversation**, and never push to the apache remote.

PR description, well under a screenful: the version adopted; whether the release needed operator
work and what; what the matrix now says; the changelog entries; and what you ran, including
`make e2e-tests` and the Kubernetes version, since CI's e2e is disabled.

## Out of scope

- **Raising the *minimum* supported Solr version.** A support promise, not a version bump — its own
  PR, timed to a release, with upgrade notes.
- **Making a new major the default** in the same change that adds support for it. Two decisions,
  two PRs.
- **`appVersion` in `helm/solr-operator/Chart.yaml`** and the operator image tag — release process,
  via `hack/release/version/propagate_version.sh`.
