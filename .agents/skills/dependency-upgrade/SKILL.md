---
name: dependency-upgrade
description: Perform a sweeping dependency upgrade of the Solr Operator - bump Go modules and pinned tool versions together, regenerate CRDs, RBAC, deepcopy code and the license list, fix the test fixtures that k8s API bumps break, and record the user-visible effects in both Helm charts' artifacthub.io/changes changelog. Use this instead of accepting many small Dependabot PRs, and especially before cutting a release.
when_to_use: The user asks to upgrade dependencies, bump Go modules, update k8s/controller-runtime/Helm libraries, refresh the dependency license list, handle a major-version or breaking dependency change, or prepare dependencies ahead of a release. Also when a dependency PR fails "make check-licenses" or "make check-git".
---

# Sweeping dependency upgrade

Upgrade everything in one reviewed pass: one CRD regeneration, one license-list regeneration, one
set of fixture repairs, one changelog entry. The changelog is the step most often forgotten and the
main reason this skill exists. Read `AGENTS.md` first — where it and this skill disagree, the repo
wins.

## Who owns what

| Tier | What | Owner |
|---|---|---|
| A | GitHub Actions | Dependabot (`.github/dependabot.yml`). Not yours: leave its PRs alone, never bump a `uses:` SHA during a sweep. |
| B | Go modules, Makefile tool pins, envtest assets | This skill. `make list-upgrades` detects them. |
| C | Base images, kind/Kubernetes versions in the test scripts, the Zookeeper server image, docs matrices | This skill, but nothing upstream to check against. `make list-upgrades` locates them by pattern and prints current values; bump what is clearly stale. `PATTERN NOT FOUND` or `MISSING FILE` means the table in the script needs updating. |

Out of scope: `DefaultSolrVersion`, chart `appVersion`, the operator's image tag — all release
process (`hack/release/version/propagate_version.sh`). Also the declared minimum Kubernetes
version, below.

## Step 0: branch from upstream main

```bash
git remote -v | awk '/apache\/solr-operator(\.git)? \(fetch\)/ {print $1}'
git status --short
git fetch <remote> main
git switch -c dependency-upgrade-<yyyy-mm> <remote>/main
```

The remote is `upstream` in some clones, `apache` or `origin` in others; if the first command prints
nothing, ask rather than guess. Stop and ask if tracked files are modified (untracked scratch files
are fine). Branch from `<remote>/main`, never local `main`. Do not `git pull`. Do not commit to
`main`.

## Preflight

All five. Each one silently ruins the run.

1. **Delete `vendor/`.** Not vendored, not gitignored, so stale copies linger. Breaks three things:
   Go switches to `-mod=vendor` (`inconsistent vendoring`); `hack/check_license.sh` has no `vendor/`
   exclusion so `make lint` exits 255 on thousands of third-party files; `make check-git` sees only
   tracked files, so CI stays green and warns you of nothing. Also check `go env GOFLAGS`.
2. **Match the Go toolchain to `go.mod`.** The Makefile fails at parse time, blocking *every*
   target including `make help`, unless `major.minor` from the `go` directive is a substring of
   `go version`. Newer fails as hard as older. Install it, or set `GOTOOLCHAIN=go1.<minor>.0`.
3. **`make clean`.** Tool installation is a bare file-existence check, so stale binaries in `bin/`
   survive a version bump and regenerate CRDs with the old `controller-gen`.
4. **Docker running, network available.** `make lint` needs Docker (`check-helm`, `check-docs`),
   `make test` downloads envtest assets, `mod-tidy` and `fetch-licenses-list` reach the network.
5. **`helm` and `yq` on `PATH`.** `make install-dependencies` installs neither.

## Decide the scope

Run `make list-upgrades`, or `./hack/list-upgrades.sh` when the Go-version gate blocks make. It
writes nothing, exits 0, and prints every section even when one of them fails.

**Take every patch and minor bump without asking.** No interview, no per-module approval, and never
hold one back because its release notes are long.

### Resolve the Go toolchain first

Decide the target Go version before judging any other candidate. A dependency's newest release may
require a newer Go than `go.mod` pins; taking it anyway silently pulls a second toolchain, and the
Makefile's gate then rejects the mismatch. `make list-upgrades` marks those candidates
`[needs Go X.Y]` — compared against the *current* directive, so if you raise `go` in `go.mod`,
re-run it and judge everything else against the new floor.

Raising Go is its own decision: it moves `build/Dockerfile` and the `go-version` matrix in CI, and
forces every contributor to update. If Go stays put, take the newest dependency version that still
builds on it rather than skipping the module — e.g. kustomize v5.8.3 needs Go 1.27, while v5.8.0
needs only 1.24.

### Linked sets

These move together — bumping one alone produces a tidy that resolves and a build that fails:

- **The Kubernetes set:** `k8s.io/api`, `apimachinery`, `client-go`, plus the indirect `k8s.io/*`
  entries (`apiextensions-apiserver`, `apiserver`, `cli-runtime`, `component-base`, `kubectl`) — all
  on one version. `helm.sh/helm/v3` pulls several, so Helm moves in lockstep.
- **`sigs.k8s.io/controller-runtime`** — also decides the `setup-envtest` release branch.
- **`github.com/onsi/ginkgo/v2`** — the Makefile derives the installed ginkgo version from it.
- **`github.com/pravega/zookeeper-operator`** — see below.

Read each candidate's release notes for the span crossed.

### A new major is the user's call

Never fold one in unilaterally, and never take the minor without saying the major exists. Cost it
out in minutes:

1. Migration notes for the major boundary only, not every release in the span.
2. Blast radius: `grep -rln '<module path>' --include='*.go' .`
3. What it drags along: a `controller-runtime` major usually forces the Kubernetes set, and
   `helm.sh/helm` pulls `k8s.io/*`, so one major becomes four.
4. User-visible effects: CRD schema, generated RBAC, minimum Kubernetes version.
5. Whether the old major is still maintained. EOL means staying put is the risk.

Present per module: current → available major, which breaking changes touch this codebase, file
count, knock-on bumps, recommendation. Offer three options — take it now, take the minor and leave
the major to a dedicated PR, or hold the module.

On a toss-up recommend the minor plus a separate PR — a module-path change is unreviewable buried
in regenerated CRDs. Recommend taking it now when the migration is mechanical, the old major is
unmaintained, or a wanted bump requires it; say which. A major taken gets its own commit and its own
PR paragraph.

## The Zookeeper Operator is pinned in a dozen places

`versions.props` is the source of truth and `make check-zk-op-version` enforces most, not all, of
the copies. If it moves, update every one of `versions.props`, `go.mod`,
`config/dependencies/zookeeper_cluster_crd.yaml` (both the `tree/` comment and the
`operator.zookeeper.pravega.io/version` annotation), `helm/solr-operator/Chart.yaml` (the
`dependencies:` entry), `helm/solr-operator/README.md`, `NOTICE` and
`docs/modules/development/pages/development.adoc`, then run that target.

- The **`v` prefix is inconsistent on purpose**: `v0.0.0` in `versions.props`, `go.mod`, the CRD
  file, `NOTICE` and the chart README; bare `0.0.0` in `Chart.yaml` and `development.adoc`. The
  check enforces both forms and hints when you get it wrong.
- The ZookeeperCluster CRD is regenerated by **no** make target. Run `hack/zk-operator/update-crd.sh`
  by hand; it needs network.
- `tests/scripts/manage_e2e_tests.sh` pins the Zookeeper *server* image separately, unchecked.

## The minimum Kubernetes version is not a dependency

The `k8s.io/*` libraries are an ordinary dependency, but the declared minimum supported Kubernetes
is a support promise: `kubeVersion` in both charts plus the matrix in
`docs/modules/upgrade-notes/pages/upgrade-notes.adoc`, enforced by Helm at install time, so raising
it stops existing users from upgrading. Nothing links the two, so a library bump cannot raise the
floor by accident — but it widens the gap between what the operator is built against and what it
claims to support. `make list-upgrades` prints declared, built and tested side by side: report that
gap, never close it. **Never edit `kubeVersion` or the matrix during a sweep.** Raising the floor is
its own PR, timed to a release, with an upgrade-notes entry and a `changed` entry in both charts. The
exception is a bump that *forces* the floor — new libraries dropping an API the operator uses, or CRD
schema older API servers reject — and then you stop, because the supported range is the first
decision.

## Tier C notes

Two that need care rather than a bump:

- The `golang` builder image in `build/Dockerfile` moves together with the `go` directive in
  `go.mod` and the `go-version` matrix in CI, because the Makefile's gate rejects a mismatch either
  way.
- `artifacthub/ah` in the Makefile is **untagged**, so `make lint` pulls `:latest`. Pinning it is
  worthwhile but a deliberate change of its own.

Some Tier C pins have already drifted (kind and Kubernetes differ between the e2e and smoke-test
scripts; `hack/test-upgrade.sh` names an old Zookeeper Operator). Do not fold a drift cleanup into a
sweep — file it separately, or the diff stops being reviewable.

## Do the upgrade

1. `make list-upgrades`. Settle the Go toolchain target first, then the rest of the scope,
   including any major-version interview.
2. Edit `go.mod`, and `versions.props` if the Zookeeper Operator moves.
3. `make mod-tidy`.
4. If the Zookeeper Operator moved: `hack/zk-operator/update-crd.sh`, update the files listed above,
   then `make check-zk-op-version` until it passes.
5. `make install-dependencies` — installs the tools at the new pinned and derived versions.
6. `make prepare` — the one command that makes a PR pass lint. Runs `fmt`, `generate`, `manifests`,
   `fetch-licenses-list` and `mod-tidy` in the right order.
7. `make test`, then repair fixtures.
8. Add the changelog entries.
9. `make lint`, then `make test` again.

### Regenerate, never hand-edit

`make manifests` writes `config/` **and** `helm/` from one `controller-gen` run, transforming on the
way, and `make check-manifests` diffs both trees recursively — a partial regeneration fails. Never
hand-edit `helm/solr-operator/crds/crds.yaml` (the three `config/crd/bases` files concatenated under
a header) or `helm/solr-operator/templates/role.yaml` (`config/rbac/role.yaml` in Helm conditionals
with templated names). A `controller-gen` bump alone rewrites every CRD, because the annotation
script stamps a version line into each; several thousand changed lines is normal.

### Lint targets run serially

`check-mod`, `check-manifests` and `check-generated` each delete a shared scratch directory, so
`make -j` has them destroy each other's work. `check-mod` also moves `go.mod`/`go.sum` aside,
re-tidies, then moves the originals back — interrupt it mid-way and the tree keeps the tidied files
while the originals sit in a scratch directory `make clean` deletes. After interrupting, check
`git diff go.mod` first.

### The license list is platform-sensitive

`fetch-licenses-list` is prefixed `GOOS=linux GOARCH=amd64` deliberately: `go-licenses` walks only
the current platform's build graph, so regenerating on macOS or ARM silently omits Linux-only
dependencies and CI's byte-exact diff then fails. Always use the make target, never `go-licenses`
directly. Do not re-sort the CSV; it is in the generator's native order.

`make check-licenses` fails two ways. A `<`/`>` diff means the CSV is stale — regenerate it. A
disallowed-license complaint means a new transitive dependency landed under a license outside the
allow-list, and **there is no override**: find a version that avoids it, or stop and raise it with
the PMC. Never edit the allow-list to make a build pass.

## Fixing what a Kubernetes bump breaks

The controller tests boot a real API server through envtest and load the real CRDs, so an API bump
changes behaviour, not just signatures. Expect to repair test fixtures, not production code — past
sweeps needed a toleration operator changed from `Exists` to `Equal`, and storage resource requests
added to PVC templates. One failure at a time, and for each ask whether it is a fixture encoding an
old default or a real regression. A regression means fixing the controller and a changelog entry; so
does any change to generated RBAC, which is user-visible.

All controller and e2e specs use focused Ginkgo nodes (`FDescribe`, `FContext`, `FIt`), so
`GINKGO_EDITOR_INTEGRATION=true` is load-bearing. `make test` sets it; `go test ./controllers/...`
by hand without it fails confusingly on programmatic focus.

## Changelog: both charts

Add entries to `artifacthub.io/changes` in **both** `helm/solr-operator/Chart.yaml` and
`helm/solr/Chart.yaml` — a YAML block scalar holding a list. `kind` accepts exactly `added`,
`changed`, `deprecated`, `removed`, `fixed`, `security`. Follow the existing entries, including the
`links:` convention of a `Github Issue` and/or `Github PR` pair. Malformed YAML fails
`make check-helm`.

Write for a user deciding whether to upgrade, not a reviewer reading the diff:

- Tight wording. One line per entry, no prose, no restating the diff.
- Never list library bumps individually. One `changed` entry summarising the sweep.
- A separate entry for anything changing behaviour or requiring action: new RBAC permissions, a
  bundled Zookeeper Operator bump, a changed default.
- A CVE fix uses `kind: security` **and** flips `artifacthub.io/containsSecurityUpdates` to `"true"`
  in that chart.
- A sweep with no user-visible effect still gets one `changed` entry.

If the bundled Zookeeper Operator moved, also add an entry to
`docs/modules/upgrade-notes/pages/upgrade-notes.adoc`; its matrices are hand-maintained.

Release-time interaction: the release process wipes `artifacthub.io/changes` to a placeholder just
after a release branch is cut, so an entry added to `main` after that lands in the *next* release.
An upgrade belonging to the release being cut must merge before that step.

## Verify before you commit

1. `make lint`, serially. Covers zk version consistency, tidy, formatting, licenses, manifests,
   generated code, charts, docs.
2. `make test`.
3. `git status --short`, then stage deliberately by naming paths. **Never `git add -A`** — the tree
   often still holds an untracked `vendor/` (6500+ files) or scratch notes. Confirm **both**
   `git diff --exit-code` and `git status --short` are clean; CI's `check-git` only checks the
   former, so an untracked generated file passes CI and breaks the next person.

Treat any failure as unfinished work. Add `make smoke-test` if packaging could be affected; it needs
Docker, kind and network, unpacks the source release and runs `make check` inside it.

CI coverage: build-and-check runs on every PR; smoke-test skips doc-only paths but runs for
`go.mod`, `go.sum`, `Makefile`, the license CSV and `helm/`. **The e2e workflow is permanently
disabled** and reports as skipped, so it is evidence of nothing — if runtime behaviour could be
affected, run `make e2e-tests` locally and say so in the PR.

## Commit, then ask before pushing

One commit, or a few logically separate ones, with generated output in the same commit as the change
that generated it. Message is a line or two: imperative subject under ~70 characters matching the
existing log (`Upgrade almost all operator dependencies`), at most a sentence of body. Detail belongs
in the PR description.

The fork is the remote whose URL is not `apache/solr-operator`:

```bash
git remote -v | grep -v apache/solr-operator
```

Several candidates or none means ask which. **Never push without the user agreeing in this
conversation**, and never push to the apache remote. After pushing, propose the PR description and
let the user open the PR unless they ask you to.

### PR description

A reviewer cannot read thousands of lines of regenerated CRDs, so the description says what to check
instead. Well under a screenful:

- One sentence: which dependency sets moved, and why now — routine sweep, pre-release, CVE.
- The sets and version spans crossed, not one bullet per module.
- Everything not mechanical: each major bump and what it required; each changed fixture, and whether
  it was stale or a real behaviour change; any RBAC or default that moved.
- One line naming the changelog entries added.
- What you ran: `make lint`, `make test`, and `make e2e-tests` if you did — CI's e2e is disabled.

Leave out anything visible from the file list, and never paste regenerated diffs. Write each
paragraph and bullet as one unwrapped line; GitHub reflows them.
