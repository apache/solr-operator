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

# Agent Guide for solr-operator

This file is read by automated agents (security scanners, code
analyzers, AI assistants) operating on this repository.

## Skills

Reusable procedures for this repository are published as [Agent Skills](https://agentskills.io) in
`.agents/skills/`, the vendor-neutral location that most assistants read. Before doing one of these
tasks by hand, read its `SKILL.md`:

| Skill | Use it for |
|---|---|
| `dependency-upgrade` | Upgrading Go modules and pinned tool versions in one sweeping pass, including CRD/RBAC regeneration, the dependency license list, and the Helm chart changelogs. |

To see what has moved before starting an upgrade, run `make list-upgrades`. It reports the Go
modules, pinned tool versions and envtest assets that the skill owns, and lists the image tags no
tool can check. GitHub Actions are Dependabot's (`.github/dependabot.yml`), not the skill's.

Most assistants read `.agents/skills/` directly. Claude Code does not, and needs a small local shim
that is deliberately not kept in this repository - `dev-docs/dependency-upgrades.md` explains the
options, including a session-start hook that mirrors the skills automatically.

## Security

Security model: [SECURITY.md](./SECURITY.md)

Agents that scan this repository should consult `SECURITY.md` and the
threat model it links before reporting issues.

This repo is the Kubernetes operator; its threat model is distinct from the Apache Solr search server (cross-referenced within).
