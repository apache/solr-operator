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

1. Delete any `vendor/` directory. The project is not vendored, `vendor/` is not gitignored, and a
   stale copy breaks `make lint` locally while CI stays green.
2. Match your Go toolchain to the `go` directive in `go.mod`. The `Makefile` refuses to run
   otherwise, and a *newer* toolchain fails just as hard as an older one.
3. `make clean`. Tool installs are skipped if the binary already exists, so stale tools in `bin/`
   would otherwise regenerate code with the wrong versions.
4. Edit `go.mod`, plus `versions.props` if the Zookeeper Operator moves.
5. `make mod-tidy`, then `make install-dependencies`, then `make prepare`.
6. `make test` and repair test fixtures. An API bump changes envtest behaviour, so expect fixtures,
   not production code, to need updating.
7. Add `artifacthub.io/changes` entries to **both** `helm/solr-operator/Chart.yaml` and
   `helm/solr/Chart.yaml`.
8. `make lint` (serially, not with `make -j`) and confirm both `git diff --exit-code` and
   `git status --short` are clean.

## Why not Dependabot

Dependabot was evaluated and set aside for the routine case. It cannot run
`make fetch-licenses-list`, so any bump that moves a transitive dependency's license URL fails
`make check-licenses` until the list is regenerated by hand. It also cannot regenerate CRDs, repair
test fixtures, or write a changelog entry. The result is a monthly stream of PRs that each need the
same manual follow-up.

A sweeping upgrade concentrates that work into one reviewed change, and the skill makes sure the
easily forgotten parts — the license list and the chart changelogs — actually happen.
