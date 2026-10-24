---
name: dependency-upgrade
description: Perform a sweeping dependency upgrade of the Solr Operator - bump Go modules and pinned tool versions together, regenerate CRDs, RBAC, deepcopy code and the license list, fix the test fixtures that k8s API bumps break, and record the user-visible effects in both Helm charts' artifacthub.io/changes changelog. Use this instead of accepting many small Dependabot PRs, and especially before cutting a release.
when_to_use: The user asks to upgrade dependencies, bump Go modules, update k8s/controller-runtime/Helm libraries, refresh the dependency license list, handle a major-version or breaking dependency change, or prepare dependencies ahead of a release. Also when a dependency PR fails "make check-licenses" or "make check-git".
---

# Sweeping dependency upgrade

Upgrade all dependencies in one reviewed pass, rather than merging a stream of single-dependency
PRs. One pass means one round of CRD regeneration, one license-list regeneration, one set of test
fixture repairs, and one changelog entry, instead of repeating all of that per bump.

**Do not skip the changelog.** Users read `artifacthub.io/changes` on Artifact Hub. An upgrade that
changes RBAC, the minimum Kubernetes version, or the bundled Zookeeper Operator is user-visible and
must say so. This is the step most often forgotten, and the main reason this skill exists.

## Before you touch anything

Work through all five. Each one silently ruins the run if skipped.

1. **Delete `vendor/` if it exists.** The project is not vendored and `vendor/` is not in
   `.gitignore`, so a stale copy lingers in many working trees. It breaks three things at once: Go
   silently switches to `-mod=vendor` and fails with `inconsistent vendoring`; `hack/check_license.sh`
   has no `vendor/` exclusion so `make lint` reports thousands of third-party files as missing the
   ASF header and exits 255; and `make check-git` only inspects tracked files, so CI stays green and
   gives you no warning. Confirm with `git status --short` that `vendor/` is gone, and check
   `go env GOFLAGS` for a stray `-mod=vendor`.

2. **Match the Go toolchain to `go.mod`.** `Makefile` fails at parse time, blocking *every* target
   including `make help`, unless the `major.minor` from the `go` directive in `go.mod` is a substring
   of `go version`. The gate is two-sided: a *newer* toolchain fails exactly as hard as an older one.
   Either install the matching toolchain, or set `GOTOOLCHAIN=go1.<minor>.0` for the session.

3. **Run `make clean`.** Tool installation uses a bare file-existence check, so anything already in
   `bin/` is kept no matter which version the Makefile now pins. Without a clean you will regenerate
   CRDs with the *old* `controller-gen` and produce a diff that CI cannot reproduce.

4. **Confirm Docker is running and you have network.** `make lint` needs Docker for `check-helm` and
   `check-docs`; `make test` downloads envtest assets on first run; `mod-tidy` and
   `fetch-licenses-list` both reach the network.

5. **Check `helm` and `yq` are on `PATH`.** `make install-dependencies` does *not* install either,
   but `check-helm`, `check-zk-op-version` and the version scripts need them.

## Decide the scope

Read `go.mod` first. Separate direct dependencies from indirect ones, and treat these as linked sets
that must move together, because bumping one alone produces a tidy that resolves but a build that
fails:

- **The Kubernetes set.** `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`, and the
  `k8s.io/*` indirect entries (`apiextensions-apiserver`, `apiserver`, `cli-runtime`,
  `component-base`, `kubectl`) all sit on one version. `helm.sh/helm/v3` pulls several of them, so
  Helm and the `k8s.io/*` set must be bumped in lockstep.
- **`sigs.k8s.io/controller-runtime`**, which also decides which `setup-envtest` release branch the
  Makefile derives.
- **`github.com/onsi/ginkgo/v2`**, which the Makefile derives the installed ginkgo version from.
- **`github.com/pravega/zookeeper-operator`**, which is special — see the next section.

For each candidate, read its release notes for the span you are crossing. Major-version bumps
(`/v2` → `/v3`) change the module path and are a separate, deliberate change; do not fold one into a
routine sweep.

## The Zookeeper Operator is pinned in a dozen places

`versions.props` is the single source of truth, and `make check-zk-op-version` enforces consistency
across most, but not all, of the copies. If you move it, update every one of these and then run that
target:

`versions.props`, `go.mod`, `config/dependencies/zookeeper_cluster_crd.yaml` (both the `tree/`
comment and the `operator.zookeeper.pravega.io/version` annotation), `helm/solr-operator/Chart.yaml`
(the `dependencies:` entry), `helm/solr-operator/README.md`, `NOTICE`, and
`docs/modules/development/pages/development.adoc`.

Two traps. First, the **`v` prefix is inconsistent on purpose**: `versions.props`, `go.mod`, the CRD
file, `NOTICE` and the chart README use `v0.0.0`, while `Chart.yaml` and `development.adoc` use bare
`0.0.0`. The check script enforces both forms and prints a hint when you get it wrong. Second, the
ZookeeperCluster CRD is **not** regenerated by any make target — run `hack/zk-operator/update-crd.sh`
by hand, which needs network.

Also update `tests/scripts/manage_e2e_tests.sh`, which pins the Zookeeper *server* image
independently and is not covered by the check script.

## Do the upgrade

1. Edit `go.mod` to the target versions, and `versions.props` if the Zookeeper Operator moves.
2. `make mod-tidy`.
3. If the Zookeeper Operator moved: run `hack/zk-operator/update-crd.sh`, update the files listed
   above, then `make check-zk-op-version` until it passes.
4. `make install-dependencies`, which now installs the tools at the new pinned and derived versions.
5. `make prepare`. This is the one command that makes a PR pass lint: it runs `fmt`, `generate`,
   `manifests`, `fetch-licenses-list` and `mod-tidy` in the right order.
6. `make test`, then repair fixtures as below.
7. Add the changelog entries. See the dedicated section.
8. `make lint`, then `make test` once more.

### Regenerate, never hand-edit

`make manifests` writes to `config/` **and** `helm/`, from one `controller-gen` run, transforming on
the way. Never hand-edit `helm/solr-operator/crds/crds.yaml` (a concatenation of the three
`config/crd/bases` files with a header prepended) or `helm/solr-operator/templates/role.yaml`
(`config/rbac/role.yaml` wrapped in Helm conditionals with names templated). `make check-manifests`
diffs both trees recursively, so a partial regeneration fails.

Expect a **controller-gen bump alone to rewrite every CRD**, because the annotation script stamps a
version line into each one. Diffs of several thousand lines across the CRD files are normal for a
sweep and are not a reason to doubt the result.

### Run the lint targets serially

`check-mod`, `check-manifests` and `check-generated` each start by deleting a shared scratch
directory, so `make -j` makes them destroy each other's work. `check-mod` is the fragile one: it
moves `go.mod`/`go.sum` aside, re-tidies in place, then moves the originals back. Interrupt it
between those steps and your working tree keeps the tidied files while the originals sit in the
scratch directory that `make clean` then deletes. If you must interrupt, check `git diff go.mod`
before doing anything else.

### Regenerate the license list on the right platform

`fetch-licenses-list` is prefixed with `GOOS=linux GOARCH=amd64` deliberately: `go-licenses` only
walks the current platform's build graph, so regenerating on macOS or ARM silently omits Linux-only
dependencies and CI's byte-exact diff then fails even though you "just regenerated". Always go
through the make target, never call `go-licenses` directly.

Two distinct failures come out of `make check-licenses`. A `<`/`>` diff means the CSV is stale —
regenerate it. A complaint about a disallowed license means a new transitive dependency arrived
under a license outside the allow-list, and **there is no override**: either find a version that
avoids that dependency, or stop and raise it with the PMC. Do not edit the allow-list to make a build
pass. Do not re-sort the CSV; it is in the generator's native order.

## Fixing what a Kubernetes bump breaks

The controller tests boot a real API server through envtest and load the real CRDs, so an API bump
changes behaviour rather than just signatures. Expect to repair test fixtures, not production code.
Past sweeps needed a toleration operator changed from `Exists` to `Equal`, and mandatory storage
resource requests added to PVC templates.

Work one failure at a time and ask, for each, whether it is a fixture that encoded an old default or
a real behavioural regression. If it is the latter, fix the controller and note it in the changelog.

Note that all controller and e2e specs use focused Ginkgo nodes (`FDescribe`, `FContext`, `FIt`), so
`GINKGO_EDITOR_INTEGRATION=true` is load-bearing. `make test` sets it; running `go test ./controllers/...`
by hand without it fails confusingly on programmatic focus.

If a dependency changed generated RBAC, that is user-visible on upgrade and **must** get a changelog
entry.

## Record the change in both charts

Add entries to `artifacthub.io/changes` in **both** `helm/solr-operator/Chart.yaml` and
`helm/solr/Chart.yaml`. The annotation is a YAML block scalar holding a list. `kind` accepts exactly
`added`, `changed`, `deprecated`, `removed`, `fixed` and `security`. Follow the entries already in
the file, including the `links:` convention of a `Github Issue` and/or `Github PR` pair.

Write for a user deciding whether to upgrade, not for a reviewer reading the diff:

- Do not list library bumps individually. One `changed` entry summarising the sweep is enough.
- Do write a separate entry for anything that changes behaviour or required action: new RBAC
  permissions, a new minimum Kubernetes version, a bundled Zookeeper Operator bump, a changed
  default.
- If a CVE is addressed, use `kind: security` **and** flip `artifacthub.io/containsSecurityUpdates`
  to `"true"` in that chart.
- A sweep with no user-visible effect still gets one `changed` entry. Silence is not an option.

Malformed changelog YAML fails `make check-helm`, so `make lint` is the check.

If the bundled Zookeeper Operator or the supported Kubernetes range moved, also add an entry to
`docs/modules/upgrade-notes/pages/upgrade-notes.adoc`. Its compatibility matrices are hand-maintained
and nothing regenerates them.

Be aware of release-time interaction: the release process wipes `artifacthub.io/changes` back to a
placeholder just after a release branch is cut. An entry added to `main` after that point lands in
the *next* release's changelog. If the upgrade belongs to the release being cut, it must merge before
that step.

## Verify before pushing

Run, in order, and treat any failure as unfinished work:

1. `make lint` — serially. Covers zk version consistency, tidy, formatting, licenses, manifests,
   generated code, charts and docs.
2. `make test`.
3. `git add -A`, then confirm **both** `git diff --exit-code` and `git status --short` are clean.
   CI's `check-git` only checks the former, so an untracked generated file passes CI and still breaks
   the next person.

Optionally `make smoke-test` if the change could affect packaging. It needs Docker, kind and network,
and it unpacks the source release and runs `make check` again inside it.

Know what CI will and will not tell you. The build-and-check workflow runs on every PR. The
smoke-test workflow is skipped for doc-only paths but does run for `go.mod`, `go.sum`, `Makefile`,
the license CSV and `helm/`. **The e2e workflow is permanently disabled** and reports as skipped, so
it is not evidence of anything — if the change could plausibly affect runtime behaviour, run
`make e2e-tests` locally and say in the PR that you did.

## Opening the PR

One commit, or a small number of logically separate ones. Keep generated output in the same commit as
the change that generated it, so a reviewer can regenerate and compare.

In the description, state: which dependency sets moved and why; any major-version bump and what it
required; which test fixtures changed and whether each was a stale fixture or a real behaviour change;
and what the changelog entries say. Reviewers cannot meaningfully read thousands of lines of
regenerated CRDs, so tell them explicitly what to check instead of asking them to read the diff.
