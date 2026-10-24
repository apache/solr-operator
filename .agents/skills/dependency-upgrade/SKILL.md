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

Read `AGENTS.md` at the repo root, and whatever it links, before you start. Where it and this skill
disagree, the repository wins.

## Who owns what

Not every pinned version in this repo is yours to move. Three populations, three owners:

| Tier | What | Owner |
|---|---|---|
| A | GitHub Actions | **Dependabot** (`.github/dependabot.yml`). Not yours. Leave its PRs alone and do not bump `uses:` SHAs during a sweep. |
| B | Go modules, the Makefile's pinned tools, the envtest assets | **This skill.** `make list-upgrades` detects them. |
| C | Base images, kind/Kubernetes versions in the test scripts, the Zookeeper server image, the docs matrices | **This skill**, but no tool sees them. Walk the checklist `make list-upgrades` prints at the end. |

Out of scope entirely: `DefaultSolrVersion`, chart `appVersion`, and the operator's own image tag. Those
belong to the release process (`hack/release/version/propagate_version.sh`), not to a dependency sweep.
So is the declared minimum Kubernetes version — see below; it is a support promise, not a dependency.

## Step 0: branch off the latest upstream main

Do this before the environment checks below, and before editing anything. A sweep touches generated
files across the whole tree, so rebasing one afterwards is far more painful than starting from the
right base.

1. **Find the upstream remote by URL, not by name.** It is `upstream` in some clones, `apache` or
   `origin` in others:

   ```bash
   git remote -v | awk '/apache\/solr-operator(\.git)? \(fetch\)/ {print $1}'
   ```

   If that prints nothing, ask which remote tracks `apache/solr-operator` rather than guessing.

2. **Confirm the working tree is clean.** `git status --short`. The sweep's own diff is large
   enough that unrelated edits become unreviewable. Untracked scratch files are usually fine;
   uncommitted changes to tracked files are not, so stop and ask.

3. **Fetch, and branch from the fetched ref — never from local `main`:**

   ```bash
   git fetch <remote> main
   git switch -c dependency-upgrade-<yyyy-mm> <remote>/main
   ```

   Branching from `<remote>/main` means a stale or diverged local `main` cannot leak in. Do not
   `git pull`, and do not commit to `main`.

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

Run `make list-upgrades` first. It reports, from the Go module proxy, the direct dependencies with a
newer version in the same major, the ones with a whole new major available, the Makefile's pinned
tool versions, and the envtest assets — then prints the Tier C pins it cannot see.

Run it as `./hack/list-upgrades.sh` when the Makefile's Go-version gate is in your way, which it will
be whenever your toolchain does not already match `go.mod`. The script sets `-mod=readonly` itself,
so it works before you have dealt with a stale `vendor/` and cannot rewrite `go.mod` or `go.sum` as
a side effect. It writes nothing, always exits 0, and prints every section even when one of them
fails — so a half-edited `go.mod` gets you a real error message for that section and the hand-check
list for the rest, rather than silence.

It is worth knowing why the detector needs its own major-version probe: `go list -u`, which is the
obvious thing to reach for, only reports updates *within* each module's current major. Under semantic
import versioning a new major is a different module path (`/v2` → `/v3`), so it is not an update to
the current one and can never appear there. Nothing about a stale major shows up until you go looking
for the successor path.

**Take every patch and minor bump without asking.** Wanting them is what asking for the sweep meant.
Do not interview the user about them, do not present them for approval one by one, and do not hold
one back because its release notes are long.

Separate direct dependencies from indirect ones, and treat these as linked sets that must move
together, because bumping one alone produces a tidy that resolves but a build that fails:

- **The Kubernetes set.** `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`, and the
  `k8s.io/*` indirect entries (`apiextensions-apiserver`, `apiserver`, `cli-runtime`,
  `component-base`, `kubectl`) all sit on one version. `helm.sh/helm/v3` pulls several of them, so
  Helm and the `k8s.io/*` set must be bumped in lockstep.
- **`sigs.k8s.io/controller-runtime`**, which also decides which `setup-envtest` release branch the
  Makefile derives.
- **`github.com/onsi/ginkgo/v2`**, which the Makefile derives the installed ginkgo version from.
- **`github.com/pravega/zookeeper-operator`**, which is special — see the next section.

For each candidate, read its release notes for the span you are crossing.

### A new major version is the user's call

Never fold a major-version bump into the sweep on your own, and never take the minor silently
either — an available major that nobody mentions is how a project drifts years behind. Instead cost
it out quickly, then put the choice to the user.

Spend a few minutes per module, not an hour:

1. Read the migration or breaking-changes notes **for the major boundary only**, not every release
   in the span.
2. Count the blast radius: `grep -rln '<module path>' --include='*.go' .` — a module used behind one
   adapter is a different proposition from one threaded through the controllers.
3. Check what it drags with it. A `controller-runtime` major usually forces the Kubernetes set, and
   `helm.sh/helm` pulls `k8s.io/*`, so one major can turn into four.
4. Check whether it is user-visible — CRD schema, generated RBAC, minimum Kubernetes version — which
   means upgrade notes and a changelog entry beyond the routine one.
5. Check whether the old major is still maintained. If it is EOL, staying put is itself the risk.

Then present, in a few lines per module: current version → available major, which breaking changes
actually touch this codebase, roughly how many files, what else the bump forces, and a
recommendation. Ask whether to take the major now, take the minor and leave the major for a
dedicated PR, or hold the module where it is.

When it is a genuine toss-up, recommend the minor now and a separate PR for the major: a module-path
change is hard to review and harder to revert when it is buried in thousands of lines of regenerated
CRDs. Recommend doing it now when the migration is small and mechanical, when the old major is
unmaintained, or when a bump the user already wants requires it anyway. Say which of those applies.

If a major is taken, give it its own commit within the branch and its own paragraph in the PR
description.

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

## The minimum Kubernetes version is not a dependency

The `k8s.io/*` libraries are an ordinary dependency, but the *declared minimum supported
Kubernetes* is a support promise: it lives in `kubeVersion` in both charts and in the compatibility
matrix in `docs/modules/upgrade-notes/pages/upgrade-notes.adoc`, Helm enforces it at install time,
and raising it stops existing users from upgrading. Nothing links the two — no make target, script
or Go code reads `kubeVersion` — so a library bump cannot raise the floor by accident, but it does
widen the gap between what the operator is built against and what it still claims to support.
`make list-upgrades` prints declared, built and tested side by side; report that gap, never close
it. **Never edit `kubeVersion` or the matrix during a sweep.** Raising the floor is its own PR,
timed to a release, with an upgrade-notes entry and a `changed` entry in both charts. The one
exception is a bump that *forces* the floor — new libraries dropping an API the operator uses, or
CRD schema older API servers reject — in which case stop, because the supported range is now the
decision to make first.

## Tier C: the pins nothing watches

`make list-upgrades` prints these at the end because it cannot check them against anything upstream.
They are image tags and version strings scattered through the Makefile, the test scripts and the
docs. It does locate them for you, by pattern rather than by line number, and prints each current
value — so if it reports `PATTERN NOT FOUND` or `MISSING FILE`, a pin has moved or been renamed and
the table in the script needs updating. Walk the list during a sweep and bump what is clearly stale;
none of them has to move every time.

Two that need care rather than a bump:

- The `golang` builder image in `build/Dockerfile` is not independent. It has to move together with
  the `go` directive in `go.mod` and the `go-version` matrix in the CI workflows, because the
  Makefile's version gate rejects a mismatch in either direction.
- `artifacthub/ah` in the Makefile is **untagged**, so `make lint` pulls `:latest` and the check is
  not reproducible between runs. Pinning it is a worthwhile change, but a deliberate one of its own.

Several of these pins have already drifted apart — kind and the Kubernetes version differ between
the e2e script and the smoke-test script, and `hack/test-upgrade.sh` still names an old Zookeeper
Operator. Do not fold a drift cleanup into a dependency sweep. File it separately, or the sweep's
diff stops being reviewable.

## Do the upgrade

1. Run `make list-upgrades` and decide the scope, including any major-version interview.
2. Edit `go.mod` to the target versions, and `versions.props` if the Zookeeper Operator moves.
3. `make mod-tidy`.
4. If the Zookeeper Operator moved: run `hack/zk-operator/update-crd.sh`, update the files listed
   above, then `make check-zk-op-version` until it passes.
5. `make install-dependencies`, which now installs the tools at the new pinned and derived versions.
6. `make prepare`. This is the one command that makes a PR pass lint: it runs `fmt`, `generate`,
   `manifests`, `fetch-licenses-list` and `mod-tidy` in the right order.
7. `make test`, then repair fixtures as below.
8. Add the changelog entries. See the dedicated section.
9. `make lint`, then `make test` once more.

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
  permissions, a bundled Zookeeper Operator bump, a changed default. A new minimum Kubernetes
  version belongs here too, but should never be reaching this point from a sweep — see above.
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

## Verify before you commit

Run, in order, and treat any failure as unfinished work:

1. `make lint` — serially. Covers zk version consistency, tidy, formatting, licenses, manifests,
   generated code, charts and docs.
2. `make test`.
3. `git status --short`, and stage deliberately — name the paths, never `git add -A`. A sweep's tree
   often still holds an untracked `vendor/` (6500+ files) or scratch notes, and `-A` sweeps those
   straight into the commit. Then confirm **both** `git diff --exit-code` and `git status --short`
   are clean. CI's `check-git` only checks the former, so an untracked generated file passes CI and
   still breaks the next person.

Optionally `make smoke-test` if the change could affect packaging. It needs Docker, kind and network,
and it unpacks the source release and runs `make check` again inside it.

Know what CI will and will not tell you. The build-and-check workflow runs on every PR. The
smoke-test workflow is skipped for doc-only paths but does run for `go.mod`, `go.sum`, `Makefile`,
the license CSV and `helm/`. **The e2e workflow is permanently disabled** and reports as skipped, so
it is not evidence of anything — if the change could plausibly affect runtime behaviour, run
`make e2e-tests` locally and say in the PR that you did.

## Commit, then ask before pushing

**Commit.** One commit, or a small number of logically separate ones, keeping generated output in
the same commit as the change that generated it so a reviewer can regenerate and compare. Keep the
message to a line or two: an imperative subject of about 70 characters or less, in the style of the
existing log (`Upgrade almost all operator dependencies`), and at most a sentence of body if
something genuinely needs explaining. The detail belongs in the PR description, not the commit.

**Identify the user's fork, and ask before pushing.** The fork is the remote whose URL is
`<user>/solr-operator` rather than `apache/solr-operator`:

```bash
git remote -v | grep -v apache/solr-operator
```

Several candidates, or none, means ask which to use. **Never push without the user agreeing in this
conversation**, and never push to the apache remote at all. After pushing, propose the PR
description below and let the user open the PR unless they ask you to.

### What the PR description should say

Short and concise. A reviewer cannot meaningfully read thousands of lines of regenerated CRDs, so
the description's job is to say what to check instead of the diff. Aim for well under a screenful:

- One sentence of summary: which dependency sets moved, and why now — routine sweep, pre-release, or
  a CVE.
- A short bullet list of the sets and the version spans crossed (the Kubernetes set,
  controller-runtime, Helm, Zookeeper Operator), not one bullet per module.
- Everything that is not mechanical: any major-version bump and what it required; each test fixture
  that changed, and whether it was a stale fixture or a real behaviour change; any RBAC, minimum
  Kubernetes version, or default that moved.
- One line naming the changelog entries added, so they can be checked without hunting for them.
- What you ran: `make lint`, `make test`, and `make e2e-tests` if you ran it — CI's e2e workflow is
  disabled, so say so explicitly.

Leave out anything visible from the file list, and do not paste regenerated diffs. Write each
paragraph and bullet as a single unwrapped line; GitHub reflows them to the reader's width.
