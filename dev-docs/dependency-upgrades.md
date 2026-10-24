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

# Dependency upgrades

Dependencies are upgraded in **one sweeping pass** rather than through a stream of single-dependency
pull requests. One pass means one round of CRD regeneration, one license-list regeneration, one set
of test-fixture repairs and one changelog entry, instead of repeating all of that per bump.

A release manager should do a sweep before cutting a release.

## The agent skill

The procedure is written up as an [Agent Skill](https://agentskills.io) at
`.agents/skills/dependency-upgrade/SKILL.md`, so an AI coding assistant can carry it out.

`.agents/skills/` is the vendor-neutral convention, read directly by Codex, Cursor, Copilot and
Gemini CLI among others. For those, nothing needs setting up: ask for a dependency upgrade and the
assistant finds the skill from its `description` and `when_to_use` frontmatter.

The skill is instructions, not automation. It contains no scripts; it names the `make` targets to
run, in what order, and what each is for. You can read it and follow it by hand — it is written to
be useful to a human too.

### Claude Code needs a local shim

Claude Code only discovers skills under `.claude/skills/`. It does not scan `.agents/skills/`, and
there is no setting to register another directory, so some local glue is needed until it follows the
convention.

`.claude/` is deliberately **gitignored** in this repository. We do not keep per-skill pointer files
in tree, because that means a hand-maintained duplicate for every skill we ever add, each with a copy
of the frontmatter to keep in sync. Set up whichever of the following suits you; all of them are
local and none are committed.

**Copy on session start (recommended).** Install a `SessionStart` hook that mirrors every skill from
`.agents/skills/` into `.claude/skills/`. Because it copies the real file, the frontmatter and body
are always correct, and because it re-runs every session the copy cannot go stale. Put the hook in
your own `.claude/settings.local.json` (or `~/.claude/settings.json` to get it in every repo):

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          { "type": "command", "command": "${CLAUDE_PROJECT_DIR}/.claude/hooks/sync-agent-skills.sh" }
        ]
      }
    ]
  }
}
```

The script it points at is yours to write and lives outside version control. It needs to do only
this: for each directory under `.agents/skills/` that contains a `SKILL.md`, create the matching
directory under `.claude/skills/` and copy the file in, replacing any previous copy. Keep it fast —
the hook runs on every session start, including after `/clear` and compaction.

**Copy by hand.** The same thing, without the hook: copy
`.agents/skills/<name>/SKILL.md` to `.claude/skills/<name>/SKILL.md` and repeat whenever the skill
changes. Fine if you rarely touch it, easy to forget if you do.

**Point Claude at the directory for one session.** Recent versions accept `--add-dir`, which loads
skills from an extra directory for that session only. Nothing to maintain, nothing persisted.

**Do not use a symlink.** Claude Code's skill discovery and validation do not resolve symlinks even
though execution does, so a symlinked skill can appear to be missing while still working when
invoked by name. Symlinks also do not survive a Windows checkout with `core.symlinks=false`.

### Invoking it

Once Claude Code can see the skill, `/dependency-upgrade` runs it. In any assistant, you can instead
just say what you want — "upgrade all the Go dependencies", "bump controller-runtime" — and let the
`description` and `when_to_use` fields do their job. Or open the file and follow it yourself.

Note that neither this repository's `SKILL.md` nor any copy of it carries an ASF license header, and
neither should: the YAML frontmatter has to be the very first thing in the file or the skill does not
parse. `hack/check_license.sh` only enforces headers on `*.go`, `*.yaml` and `*.sh`, so this is not a
lint failure waiting to happen — but please do not "fix" it by adding one.

## Doing it by hand

The short version, with the detail and the reasoning in the skill:

1. `make list-upgrades` to see what has moved, and decide the scope. Patch and minor bumps are
   always wanted; a new major is a decision, not a default.
2. Delete any `vendor/` directory. The project is not vendored, `vendor/` is not gitignored, and a
   stale copy breaks `make lint` locally while CI stays green.
3. Match your Go toolchain to the `go` directive in `go.mod`. The `Makefile` refuses to run
   otherwise, and a *newer* toolchain fails just as hard as an older one.
4. `make clean`. Tool installs are skipped if the binary already exists, so stale tools in `bin/`
   would otherwise regenerate code with the wrong versions.
5. Edit `go.mod`, plus `versions.props` if the Zookeeper Operator moves.
6. `make mod-tidy`, then `make install-dependencies`, then `make prepare`.
7. `make test` and repair test fixtures. An API bump changes envtest behaviour, so expect fixtures,
   not production code, to need updating.
8. Add `artifacthub.io/changes` entries to **both** `helm/solr-operator/Chart.yaml` and
   `helm/solr/Chart.yaml`.
9. `make lint` (serially, not with `make -j`) and confirm both `git diff --exit-code` and
   `git status --short` are clean.

## What Dependabot owns, and what it does not

Dependabot handles **GitHub Actions only** (`.github/dependabot.yml`): one grouped PR a month across
all workflows. Actions are cheap to review, already pinned to commit SHAs, and need no follow-up
work, so automating them costs nothing.

It deliberately has **no `gomod` entry**, and Dependabot only runs the ecosystems a config lists, so
leaving it out is the enforcement. A Go module bump is not self-contained here: Dependabot cannot run
`make fetch-licenses-list`, so any bump that moves a transitive dependency's license URL fails
`make check-licenses` until the list is regenerated by hand, and it cannot regenerate CRDs, repair
the test fixtures a Kubernetes API bump invalidates, or write a changelog entry. The result would be
a monthly stream of PRs each needing the same manual follow-up. A sweeping upgrade concentrates that
work into one reviewed change, and the skill makes sure the easily forgotten parts — the license list
and the chart changelogs — actually happen.

There is no `docker` entry either. The only Dockerfile pins the Go builder image, which has to move
in lockstep with the `go` directive in `go.mod` and the CI matrix, so it belongs to the sweep.

### Reviewing a Dependabot Actions PR

The ASF allowlists third-party actions **by commit SHA**, in `actions.yml` in the
`apache/infrastructure-actions` repository. A SHA that is not on that list fails the workflow at
startup, before any step runs. Check a proposed SHA there before merging. This has already bitten the
project once: commit `bbb5a3d` had to move `azure/setup-helm` *forward* to an INFRA-approved SHA.

Of the actions in use only `azure/setup-helm` is third-party; the `actions/*` ones are first-party
and not gated. If these PRs turn out to need that check too often, one `ignore:` entry in
`.github/dependabot.yml` takes `azure/setup-helm` back to being a manual bump.

## Finding out what has moved

`make list-upgrades` (or `./hack/list-upgrades.sh` directly, when the Makefile's Go-version gate is in
the way) reports everything in the skill's half of the split: direct Go modules with a newer version
in the same major, modules with a whole new major available, the Makefile's pinned tool versions, and
the envtest Kubernetes assets. It then lists the image tags and version strings that no tool can see,
so they can be checked by hand.

It reads the Go module proxy rather than scraping release pages, writes nothing, and always exits 0.
It is deliberately not part of `make lint` or `make check`: it needs the network, and its output
changes whenever an upstream project cuts a release.

The major-version probe is the part worth knowing about. `go list -u` only reports updates within a
module's current major, because under semantic import versioning a new major is a different module
path and therefore not an update to the current one. Without an explicit probe for the successor
path, a dependency can sit a whole major behind indefinitely without anything saying so.
