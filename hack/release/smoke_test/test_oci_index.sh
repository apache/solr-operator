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

show_help() {
cat << EOF
Usage: ./hack/release/smoke_test/test_oci_index.sh [-h] [-k] [-v VERSION] [-H HELM_VERSIONS] [-p PUSH_HELM_VERSION]

Test that a classic Helm repository index.yaml, whose entries point at oci:// urls, works for the
ways users consume the Solr Operator Helm charts: 'helm repo add', search, show, pull, provenance
verification, install-style chart resolution and 'helm dependency update'.

Everything runs in Docker containers on a private network, so nothing is bound on the host:
  - a TLS-enabled 'registry:2', standing in for ghcr.io. TLS matters: Helm only speaks plain HTTP
    to a registry when given --plain-http, which cannot be passed through an index entry.
  - a static file server for the index.yaml, standing in for https://solr.apache.org/charts
  - one container per Helm version under test, using the official Helm release binaries

The charts are packaged from this checkout and signed with a throwaway GPG key, pushed with their
.prov files (as the publish-helm-oci.yaml workflow does), and indexed with
hack/release/upload/update_helm_index.sh.

    -h  Display this help and exit
    -v  Chart version to package and test (Optional, defaults to the project version, without any
        prerelease suffix, so that the default 'helm search' behaviour is exercised)
    -H  Space-separated Helm versions to test (Optional, defaults to "${DEFAULT_HELM_VERSIONS}")
    -p  Helm version used to push the charts (Optional, defaults to ${DEFAULT_PUSH_HELM_VERSION},
        the version publish-helm-oci.yaml pins)
    -k  Keep the containers, network and working directory after the run, for debugging

Requires: docker, openssl, gpg, curl, helm (to package the charts) and bin/yq ('make yq').
EOF
}

# 3.8 is the first Helm release with OCI support out of experimental. 4.3.1 matches the
# version publish-helm-oci.yaml pins. Pass -H to test others, e.g. the latest 3.x.
DEFAULT_HELM_VERSIONS="3.8.2 3.19.0 4.3.1"
DEFAULT_PUSH_HELM_VERSION="4.3.1"

OPTIND=1
while getopts hkv:H:p: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        k)  KEEP=true
            ;;
        v)  VERSION=$OPTARG
            ;;
        H)  HELM_VERSIONS=$OPTARG
            ;;
        p)  PUSH_HELM_VERSION=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${REPO_ROOT}"

HELM_VERSIONS="${HELM_VERSIONS:-${DEFAULT_HELM_VERSIONS}}"
PUSH_HELM_VERSION="${PUSH_HELM_VERSION:-${DEFAULT_PUSH_HELM_VERSION}}"
if [[ -z "${VERSION:-}" ]]; then
  VERSION="$(make -s version)"
  VERSION="${VERSION%%-*}"
fi
CHART_VERSION="${VERSION#v}"

for tool in docker openssl gpg curl helm; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "${tool} is required to run this test" >&2 && exit 1
  fi
done

RUN_ID="solr-oci-index-$$"
NETWORK="${RUN_ID}"
REGISTRY_CONTAINER="${RUN_ID}-registry"
CHARTS_CONTAINER="${RUN_ID}-charts"
# Containers reach each other by these network aliases.
REGISTRY_HOST="registry"
CHARTS_HOST="charts"
OCI_BASE="oci://${REGISTRY_HOST}:5000/helm"
REPO_URL="http://${CHARTS_HOST}:8080"
CONTAINER_IMAGE="alpine:3"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/solr-operator-oci-index-test-XXXXXXXX")"
# Docker Desktop shares /var/folders and /private, but not every realpath form of TMPDIR.
WORK_DIR="$(cd "${WORK_DIR}" && pwd -P)"
HELM_BIN_CACHE="${HELM_BIN_CACHE:-${TMPDIR:-/tmp}/solr-operator-helm-binaries}"

cleanup() {
  if [[ -n "${GPG_HOME:-}" ]]; then
    GNUPGHOME="${GPG_HOME}" gpgconf --kill gpg-agent >/dev/null 2>&1 || true
    rm -rf "${GPG_HOME}"
  fi
  if [[ "${KEEP:-}" == "true" ]]; then
    echo "Kept the network ${NETWORK}, containers ${REGISTRY_CONTAINER} and ${CHARTS_CONTAINER}, and ${WORK_DIR}"
    return
  fi
  docker rm -f "${REGISTRY_CONTAINER}" "${CHARTS_CONTAINER}" >/dev/null 2>&1 || true
  docker network rm "${NETWORK}" >/dev/null 2>&1 || true
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

echo "Testing an index of oci:// chart urls, for chart version ${CHART_VERSION}, with Helm ${HELM_VERSIONS}"

# --- Helm binaries, for the architecture of the Docker engine (not the host) ----------------

case "$(docker info --format '{{.Architecture}}')" in
  x86_64|amd64)  HELM_ARCH=amd64 ;;
  aarch64|arm64) HELM_ARCH=arm64 ;;
  *) echo "Unsupported Docker architecture: $(docker info --format '{{.Architecture}}')" >&2 && exit 1 ;;
esac

fetch_helm() {
  local version="$1"
  local dir="${HELM_BIN_CACHE}/v${version}-linux-${HELM_ARCH}"
  if [[ ! -x "${dir}/helm" ]]; then
    local tarball="helm-v${version}-linux-${HELM_ARCH}.tar.gz"
    mkdir -p "${dir}"
    echo "Downloading Helm v${version} (linux/${HELM_ARCH})" >&2
    curl --fail --show-error --silent --location -o "${dir}/${tarball}" "https://get.helm.sh/${tarball}"
    expected="$(curl --fail --show-error --silent --location "https://get.helm.sh/${tarball}.sha256sum" | awk '{print $1}')"
    actual="$(openssl dgst -sha256 "${dir}/${tarball}" | awk '{print $NF}')"
    if [[ "${expected}" != "${actual}" ]]; then
      rm -rf "${dir}"
      echo "Checksum mismatch for ${tarball}" >&2 && exit 1
    fi
    tar -xzf "${dir}/${tarball}" -C "${dir}" --strip-components=1 "linux-${HELM_ARCH}/helm"
    rm "${dir}/${tarball}"
  fi
  echo "${dir}/helm"
}

PUSH_HELM="$(fetch_helm "${PUSH_HELM_VERSION}")"
for v in ${HELM_VERSIONS}; do
  fetch_helm "${v}" >/dev/null
done

# --- TLS for the registry ----------------------------------------------------------------------

TLS_DIR="${WORK_DIR}/tls"
mkdir -p "${TLS_DIR}"
(
  cd "${TLS_DIR}"
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=Solr OCI index test CA" \
    -keyout ca.key -out ca.crt 2>/dev/null
  openssl req -newkey rsa:2048 -nodes -subj "/CN=${REGISTRY_HOST}" \
    -keyout registry.key -out registry.csr 2>/dev/null
  printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\n' "${REGISTRY_HOST}" > registry.ext
  openssl x509 -req -in registry.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 1 \
    -extfile registry.ext -out registry.crt 2>/dev/null
  chmod 644 ./*
)

# --- Throwaway signing key -----------------------------------------------------------------------

GPG_DIR="${WORK_DIR}/gpg"
mkdir -p "${GPG_DIR}"
# The gpg-agent socket lives in GNUPGHOME, and a unix socket path is limited to ~104 bytes. The
# macOS TMPDIR (/var/folders/...) is too long for that, so the keyring home goes under /tmp.
GPG_HOME="$(mktemp -d /tmp/solr-gpg-XXXXXX)"
chmod 700 "${GPG_HOME}"
GPG_UID="Solr OCI Index Test <oci-index-test@example.invalid>"
GNUPGHOME="${GPG_HOME}" gpg --batch --quiet --pinentry-mode loopback --passphrase '' \
  --quick-gen-key "${GPG_UID}" rsa2048 sign 1d
# Helm reads the legacy keyring formats, as build_helm.sh and publish-helm-oci.yaml also rely on.
GNUPGHOME="${GPG_HOME}" gpg --batch --export-secret-keys > "${GPG_DIR}/secring.gpg"
GNUPGHOME="${GPG_HOME}" gpg --batch --export > "${GPG_DIR}/pubring.gpg"
GNUPGHOME="${GPG_HOME}" gpgconf --kill gpg-agent >/dev/null 2>&1 || true
rm -rf "${GPG_HOME}"

# --- Package and sign the charts -----------------------------------------------------------------

CHARTS_DIR="${WORK_DIR}/charts"
mkdir -p "${CHARTS_DIR}"
echo "Packaging and signing the charts at version ${CHART_VERSION}"
SIGNING=(--sign --key "Solr OCI Index Test" --keyring "${GPG_DIR}/secring.gpg")
# This adds the pravega repo the zookeeper-operator dependency comes from, if it is missing.
make -s helm-dependency-build >/dev/null
helm package helm/solr-operator --app-version "v${CHART_VERSION}" --version "${CHART_VERSION}" -d "${CHARTS_DIR}" "${SIGNING[@]}" >/dev/null
helm package helm/solr --version "${CHART_VERSION}" -d "${CHARTS_DIR}" "${SIGNING[@]}" >/dev/null
ls "${CHARTS_DIR}"

# --- Registry and index server -------------------------------------------------------------------

docker network create "${NETWORK}" >/dev/null
docker run -d --name "${REGISTRY_CONTAINER}" --network "${NETWORK}" --network-alias "${REGISTRY_HOST}" \
  -v "${TLS_DIR}:/certs:ro" \
  -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/registry.crt \
  -e REGISTRY_HTTP_TLS_KEY=/certs/registry.key \
  registry:2 >/dev/null

SITE_DIR="${WORK_DIR}/site"
mkdir -p "${SITE_DIR}"
docker run -d --name "${CHARTS_CONTAINER}" --network "${NETWORK}" --network-alias "${CHARTS_HOST}" \
  -v "${SITE_DIR}:/srv:ro" \
  busybox:stable httpd -f -p 8080 -h /srv >/dev/null

# Runs a command with the given Helm binary, in a container on the test network.
in_helm_container() {
  local helm_bin="$1"; shift
  docker run --rm --network "${NETWORK}" \
    -v "${helm_bin}:/usr/local/bin/helm:ro" \
    -v "${WORK_DIR}:/work:ro" \
    -e SSL_CERT_FILE=/work/tls/ca.crt \
    -e HELM_CONFIG_HOME=/tmp/helm/config \
    -e HELM_CACHE_HOME=/tmp/helm/cache \
    -e HELM_DATA_HOME=/tmp/helm/data \
    "$@"
}

# --- Push, the way publish-helm-oci.yaml does: the .prov next to each chart is uploaded too ------

echo "Pushing the charts to ${OCI_BASE} with Helm v${PUSH_HELM_VERSION}"
pushed=false
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if in_helm_container "${PUSH_HELM}" "${CONTAINER_IMAGE}" sh -c "
      set -e
      helm push /work/charts/solr-operator-${CHART_VERSION}.tgz ${OCI_BASE}
      helm push /work/charts/solr-${CHART_VERSION}.tgz ${OCI_BASE}
    "; then
    pushed=true
    break
  fi
  echo "Push attempt ${attempt} failed, the registry may still be starting. Retrying."
  sleep 2
done
if [[ "${pushed}" != "true" ]]; then
  echo "Could not push the charts to the test registry" >&2 && exit 1
fi

# --- Generate the index, exactly as a release would ----------------------------------------------

./hack/release/upload/update_helm_index.sh -n -c -d "${CHARTS_DIR}" -o "${OCI_BASE}" -i "${SITE_DIR}/index.yaml"

# --- The checks, run once per Helm version -------------------------------------------------------

cat > "${WORK_DIR}/checks.sh" << 'EOF'
#!/bin/sh
# Each check prints 'RESULT|<name>|PASS' or 'RESULT|<name>|FAIL', followed by its output on failure.
V="$1"
REPO_URL="$2"
OCI_BASE="$3"
KEYRING=/work/gpg/pubring.gpg
# 'helm pull -d' does not create its destination directory.
mkdir -p /tmp/direct /tmp/pull /tmp/pull-verify

check() {
  name="$1"; shift
  if "$@" > /tmp/check.log 2>&1; then
    echo "RESULT|${name}|PASS"
  else
    echo "RESULT|${name}|FAIL"
    tail -n 5 /tmp/check.log | sed 's/^/    /'
  fi
}

umbrella() {
  dir="$1"; repository="$2"
  mkdir -p "${dir}"
  cat > "${dir}/Chart.yaml" << CHART
apiVersion: v2
name: umbrella
version: 0.0.1
dependencies:
  - name: solr
    version: "${V}"
    repository: "${repository}"
CHART
  helm dependency update "${dir}" && test -f "${dir}/charts/solr-${V}.tgz" \
    && cmp "${dir}/charts/solr-${V}.tgz" "/work/charts/solr-${V}.tgz"
}

helm version --short

# Control: the registry and TLS setup work without the index, so a failure below is the index's.
check "control: helm pull oci:// directly" \
  helm pull "${OCI_BASE}/solr-operator" --version "${V}" -d /tmp/direct

check "helm repo add" helm repo add apache-solr "${REPO_URL}"
check "helm repo update" helm repo update
check "helm search repo" sh -c "helm search repo apache-solr/solr-operator --versions | grep -q '${V}'"
check "helm show chart" helm show chart apache-solr/solr-operator --version "${V}"
check "helm pull (byte-identical)" sh -c "
  helm pull apache-solr/solr-operator --version '${V}' -d /tmp/pull &&
  cmp '/tmp/pull/solr-operator-${V}.tgz' '/work/charts/solr-operator-${V}.tgz'"
check "helm pull --verify" \
  helm pull apache-solr/solr-operator --version "${V}" --verify --keyring "${KEYRING}" -d /tmp/pull-verify
# 'helm template' resolves the chart through the same code path as 'helm install', without
# needing a cluster.
check "install path: solr-operator" helm template t apache-solr/solr-operator --version "${V}"
check "install path: solr" helm template t apache-solr/solr --version "${V}"
check "install path: --verify" \
  helm template t apache-solr/solr-operator --version "${V}" --verify --keyring "${KEYRING}"
check "dependency update: repository url" umbrella /tmp/umbrella-url "${REPO_URL}"
check "dependency update: @repo alias" umbrella /tmp/umbrella-alias "@apache-solr"
EOF

RESULTS_FILE="${WORK_DIR}/results.txt"
: > "${RESULTS_FILE}"
for v in ${HELM_VERSIONS}; do
  echo ""
  echo "=== Helm v${v} ==="
  output="$(in_helm_container "$(fetch_helm "${v}")" "${CONTAINER_IMAGE}" \
    sh /work/checks.sh "${CHART_VERSION}" "${REPO_URL}" "${OCI_BASE}" 2>&1 || true)"
  echo "${output}" | sed -n 's/^RESULT|\(.*\)|\(.*\)$/  \2  \1/p; /^    /p; /^v[0-9]/p'
  echo "${output}" | sed -n "s/^RESULT|\(.*\)|\(.*\)$/v${v}|\1|\2/p" >> "${RESULTS_FILE}"
done

echo ""
echo "=== Summary ==="
failures="$(grep -c '|FAIL$' "${RESULTS_FILE}" || true)"
total="$(wc -l < "${RESULTS_FILE}" | tr -d ' ')"
grep '|FAIL$' "${RESULTS_FILE}" | sed 's/^\(.*\)|FAIL$/  FAIL  \1/' || true
expected=$(( $(grep -c '^check "' "${WORK_DIR}/checks.sh") * $(echo ${HELM_VERSIONS} | wc -w) ))
if [[ "${total}" -ne "${expected}" ]]; then
  echo "Only ${total} of ${expected} checks reported a result" >&2
  exit 1
fi
echo "${total} checks across Helm ${HELM_VERSIONS}: $((total - failures)) passed, ${failures} failed"
if [[ "${failures}" -gt 0 ]]; then
  exit 1
fi
