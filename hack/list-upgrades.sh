#!/usr/bin/env bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

# Report which dependencies have newer versions available. Read-only: this
# script changes nothing, and always exits 0, so it can never fail a build.
#
# It covers what can be detected reproducibly from the Go module proxy, which
# is the tier the dependency-upgrade skill owns. GitHub Actions are Dependabot's
# (see .github/dependabot.yml) and are not reported here. The pins that no tool
# can see are listed at the end as a reminder to check them by hand.
#
# Runnable either as "make list-upgrades" or directly as ./hack/list-upgrades.sh.
# The direct form matters: the Makefile refuses to run at all when the installed
# Go toolchain does not match the "go" directive in go.mod, which is exactly the
# situation you are in when you are about to bump it.

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_PATH="${PROJECT_DIR}/hack/$(basename "${BASH_SOURCE[0]}")"
cd "${PROJECT_DIR}"

# A stale, untracked vendor/ directory switches Go to -mod=vendor and makes
# every command below fail with "inconsistent vendoring". Detection has to work
# before that has been cleaned up, so override it for this script only.
# readonly rather than mod: it escapes vendor mode just the same, but also
# forbids Go from writing go.mod or go.sum, which "-mod=mod" does as a side
# effect of resolving the upgrade candidates.
export GOFLAGS="-mod=readonly"
export GO111MODULE=on

# Internal re-entry used to run the major-version probes in parallel through
# xargs. Not part of the interface.
if [[ "${1:-}" == "--probe-major" ]]; then
  probe_path="$2"
  probe_current="$3"

  # Split a module path into its base and its current major. Paths without a
  # /vN suffix are major 0 or 1, whose successor is /v2 either way.
  if [[ "${probe_path}" =~ ^(.*)/v([0-9]+)$ ]]; then
    probe_base="${BASH_REMATCH[1]}"
    probe_major="${BASH_REMATCH[2]}"
  else
    probe_base="${probe_path}"
    probe_major=1
  fi

  found_path=""
  found_version=""
  # Probing stops at the first major that does not exist. A project that
  # skipped a major number outright would be missed, which is rare enough to
  # live with and cheaper than probing a fixed range every time.
  for ((n = probe_major + 1; n <= probe_major + 5; n++)); do
    if ! versions="$(go list -m -versions "${probe_base}/v${n}" 2>/dev/null)"; then
      break
    fi
    # Field 1 is the module path; the rest are versions, oldest first. Ignore
    # prereleases, so an -alpha or -rc tag is never reported as available.
    stable="$(tr ' ' '\n' <<<"${versions}" | tail -n +2 | awk '!/-/' | tail -1)"
    if [[ -z "${stable}" ]]; then
      break
    fi
    found_path="${probe_base}/v${n}"
    found_version="${stable}"
  done

  if [[ -n "${found_path}" ]]; then
    printf '  %-50s %-28s -> %s %s\n' "${probe_base}" "${probe_current}" "${found_version}" "(${found_path})"
  fi
  exit 0
fi

# Print the newest non-prerelease version of a module path, or nothing. Walks
# up the path until it finds a real module, because the Makefile installs some
# tools by package path (.../cmd/controller-gen) rather than by module path.
latest_stable() {
  local path="$1"
  local versions stable
  while [[ "${path}" == *"/"*"/"* || "${path}" == *"/"* ]]; do
    if versions="$(go list -m -versions "${path}" 2>/dev/null)"; then
      stable="$(tr ' ' '\n' <<<"${versions}" | tail -n +2 | awk '!/-/' | tail -1)"
      if [[ -n "${stable}" ]]; then
        printf '%s' "${stable}"
        return 0
      fi
    fi
    # No versions at this path: it is a package inside a module, so go up one.
    local parent="${path%/*}"
    if [[ "${parent}" == "${path}" || "${parent}" != *"/"* ]]; then
      return 0
    fi
    path="${parent}"
  done
}

printf '\n=== Go modules (direct, within the current major) ===\n\n'
module_updates="$(go list -m -u -f '{{if and (not .Indirect) .Update}}  {{printf "%-50s %-28s -> %s" .Path .Version .Update.Version}}{{end}}' all 2>/dev/null | grep . || true)"
if [[ -n "${module_updates}" ]]; then
  printf '%s\n' "${module_updates}"
else
  printf '  Everything is current.\n'
fi

printf '\n=== New MAJOR versions available (each one needs a decision) ===\n\n'
# "go list -u" can never report these: under semantic import versioning a new
# major is a different module path, so it is not an update to the current one.
# Probe the direct dependencies and the installed tools for a successor path.
probe_input="$(
  go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}} {{.Version}}{{end}}' all 2>/dev/null |
    # Keep only complete "path version" pairs. xargs -n 2 below splits on
    # whitespace, so a line missing its version would consume the next line's
    # path as its version and misalign every probe that follows.
    awk 'NF == 2' |
    # Pseudo-versions mean the module publishes no tags, so /vN is meaningless.
    awk '$2 !~ /^v0\.0\.0-[0-9]{14}-/'
)"
# The tools the Makefile installs are modules too, and are just as capable of
# having moved a major (kustomize/v4 -> /v5, for instance).
tool_lines="$(grep -E '\$\(call go-get-tool,' Makefile || true)"
while IFS= read -r line; do
  [[ -z "${line}" ]] && continue
  # $(call go-get-tool,$(BINARY),some.host/mod/path@$(SOME_VERSION))
  if [[ "${line}" =~ go-get-tool,[^,]*,([^@]+)@\$\(([A-Z0-9_]+)\)\)[[:space:]]*$ ]]; then
    tool_path="${BASH_REMATCH[1]}"
    tool_var="${BASH_REMATCH[2]}"
    tool_def="$(grep -E "^${tool_var} *[:?]?= *" Makefile | head -1 || true)"
    # Versions derived from go.mod with $(shell ...) follow their module
    # automatically and are already covered by the section above.
    [[ "${tool_def}" == *'$(shell'* ]] && continue
    tool_version="${tool_def#*=}"
    [[ -z "${tool_version// /}" ]] && continue
    probe_input+=$'\n'"${tool_path} ${tool_version// /}"
  fi
done <<<"${tool_lines}"

major_output="$(printf '%s\n' "${probe_input}" | awk 'NF == 2' | xargs -P 8 -n 2 "${SCRIPT_PATH}" --probe-major | sort || true)"
if [[ -n "${major_output}" ]]; then
  printf '%s\n' "${major_output}"
  printf '\n  A new major is never taken automatically. Assess the migration cost\n'
  printf '  and put the choice to the user - see the skill for what to present.\n'
else
  printf '  None.\n'
fi

printf '\n=== Makefile tool pins ===\n\n'
while IFS= read -r line; do
  [[ -z "${line}" ]] && continue
  if [[ "${line}" =~ go-get-tool,[^,]*,([^@]+)@\$\(([A-Z0-9_]+)\)\)[[:space:]]*$ ]]; then
    tool_path="${BASH_REMATCH[1]}"
    tool_var="${BASH_REMATCH[2]}"
    tool_def="$(grep -E "^${tool_var} *[:?]?= *" Makefile | head -1 || true)"
    if [[ "${tool_def}" == *'$(shell'* ]]; then
      printf '  %-26s %-12s derived from go.mod, nothing to do\n' "${tool_var}" ""
      continue
    fi
    tool_version="${tool_def#*=}"
    tool_version="${tool_version// /}"
    tool_latest="$(latest_stable "${tool_path}")"
    if [[ -z "${tool_latest}" ]]; then
      printf '  %-26s %-12s (could not resolve %s)\n' "${tool_var}" "${tool_version}" "${tool_path}"
    elif [[ "${tool_latest}" == "${tool_version}" ]]; then
      printf '  %-26s %-12s current\n' "${tool_var}" "${tool_version}"
    else
      printf '  %-26s %-12s -> %s\n' "${tool_var}" "${tool_version}" "${tool_latest}"
    fi
  fi
done <<<"${tool_lines}"

printf '\n=== envtest Kubernetes assets ===\n\n'
ENVTEST_PINNED="$(grep -E '^ENVTEST_K8S_VERSION' Makefile | head -1 | sed -E 's/.*= *//' | tr -d ' ')"
SETUP_ENVTEST="${PROJECT_DIR}/bin/setup-envtest"
if [[ -x "${SETUP_ENVTEST}" ]]; then
  # Asset versions are plain x.y.z, and the list is not ordered newest-first.
  envtest_latest="$("${SETUP_ENVTEST}" list 2>/dev/null |
    grep -o -E '[0-9]+\.[0-9]+\.[0-9]+' | sort -t. -k1,1n -k2,2n -k3,3n -u | tail -1 || true)"
  if [[ -n "${envtest_latest}" && "${envtest_latest}" != "${ENVTEST_PINNED}" ]]; then
    printf '  %-26s %-12s -> %s\n' "ENVTEST_K8S_VERSION" "${ENVTEST_PINNED}" "${envtest_latest}"
  else
    printf '  %-26s %-12s current\n' "ENVTEST_K8S_VERSION" "${ENVTEST_PINNED}"
  fi
else
  printf '  %-26s %-12s (run "make install-dependencies" to check for newer assets)\n' \
    "ENVTEST_K8S_VERSION" "${ENVTEST_PINNED}"
fi

printf '\n=== Not detectable automatically - check these by hand ===\n\n'

# Image tags and version strings that no tool watches. Located by pattern at
# run time rather than by line number, so an edit above them cannot silently
# make this list point at the wrong line. A pattern that stops matching is
# reported, because that means the pin moved or the file was renamed and this
# table needs updating.
#
# Format: description|file|extended regex
while IFS='|' read -r pin_desc pin_file pin_re; do
  [[ -z "${pin_desc}" ]] && continue
  if [[ ! -f "${pin_file}" ]]; then
    printf '  %-46s %s\n' "MISSING FILE - update this list" "${pin_file}"
    continue
  fi
  pin_match="$(grep -nE "${pin_re}" "${pin_file}" 2>/dev/null | head -1 || true)"
  if [[ -z "${pin_match}" ]]; then
    printf '  %-46s %s\n' "PATTERN NOT FOUND - update this list" "${pin_file}"
    continue
  fi
  pin_line="${pin_match%%:*}"
  # Strip the line number, then leading and trailing whitespace.
  pin_text="${pin_match#*:}"
  pin_text="$(printf '%s' "${pin_text}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
  # Some matches are whole command lines; keep the column readable.
  if ((${#pin_text} > 72)); then
    pin_text="${pin_text:0:69}..."
  fi
  printf '  %-46s %s\n' "${pin_file}:${pin_line}" "${pin_text}"
  printf '  %-46s %s\n' "" "${pin_desc}"
done <<'PINS'
must move together with go.mod and the CI matrix|build/Dockerfile|^FROM golang:
a floating tag, no digest|build/Dockerfile|^FROM gcr.io/distroless
the docs toolchain|Makefile|^ANTORA_IMAGE
untagged, so "make lint" pulls :latest|Makefile|artifacthub/ah
the Zookeeper server image|tests/scripts/manage_e2e_tests.sh|pravega/zookeeper:
Kubernetes for the e2e kind cluster|tests/scripts/manage_e2e_tests.sh|KUBERNETES_VERSION="v[0-9]
Kubernetes for the smoke-test kind cluster|hack/release/smoke_test/test_cluster.sh|KUBERNETES_VERSION="v[0-9]
a second kind pin, independent of the Makefile|hack/release/smoke_test/test_cluster.sh|sigs.k8s.io/kind@
left over from kubebuilder scaffolding|config/default/manager_auth_proxy_patch.yaml|kube-rbac-proxy:
PINS

for pin_file in hack/release/wizard/requirements.txt docs/modules/upgrade-notes/pages/upgrade-notes.adoc; do
  if [[ -f "${pin_file}" ]]; then
    printf '  %-46s %s\n' "${pin_file}" "several pins, read the file"
  else
    printf '  %-46s %s\n' "MISSING FILE - update this list" "${pin_file}"
  fi
done

cat <<'EOF'

  GitHub Actions are Dependabot's, not this script's. See .github/dependabot.yml.
  The Solr and operator image versions belong to the release process, not to a
  dependency sweep.

EOF

exit 0
