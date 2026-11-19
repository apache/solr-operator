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
Usage: ./hack/release/upload/update_helm_index.sh [-h] [-g GPG_KEY] [-n] [-c] [-o OCI_BASE] [-v VERSION] (-r RELEASE_URL | -d CHARTS_DIR) -i INDEX_FILE

Add (or replace) Helm chart versions in a Helm repository index.yaml, whose entries point at
the OCI registry the charts were published to, instead of at HTTPS tarballs.

The index is served from https://solr.apache.org/charts, and is checked into the solr-site
repository. Point -i at the index.yaml in a solr-site checkout. Nothing is committed or uploaded.

An entry that already exists for the same chart and version is replaced in place, keeping its
original 'created' timestamp. Its digest must not change, since the OCI copy is required to be
byte-identical to the released chart. This makes it safe to re-run, and is what backfills older
releases once they have been published to the OCI registry.

    -h  Display this help and exit
    -i  The index.yaml to update.
    -c  Create the index if it does not exist. Without this, a missing index is an error, so that a
        wrong path can not silently produce an index that has lost every earlier release.
    -r  URL of the released Solr Operator artifacts, on dist.apache.org or archive.apache.org
        (do not include the 'helm-charts' suffix). The charts are downloaded, and their
        checksums and GPG signatures verified. The Solr KEYS must already be imported into gpg.
    -d  A local directory of chart tarballs to index instead of -r. Nothing is verified.
        Meant for testing.
    -v  Version of the release, e.g. v0.10.0 (Optional, defaults to the last path element of -r)
    -o  OCI registry the charts were pushed to (Optional, defaults to ${DEFAULT_OCI_BASE})
    -g  GPG Key to sign the new index file (Optional, defaults to the gpg default key)
    -n  Do not sign the index, or write its checksum. Meant for testing.
EOF
}

DEFAULT_OCI_BASE="oci://ghcr.io/apache/solr-operator/helm"
CHART_NAMES=(solr-operator solr)

OPTIND=1
while getopts hi:r:d:v:o:g:nc opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        i)  INDEX_FILE=$OPTARG
            ;;
        r)  RELEASE_URL=$OPTARG
            ;;
        d)  CHARTS_DIR=$OPTARG
            ;;
        v)  VERSION=$OPTARG
            ;;
        o)  OCI_BASE=$OPTARG
            ;;
        g)  GPG_KEY=$OPTARG
            ;;
        n)  NO_SIGN=true
            ;;
        c)  CREATE=true
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${INDEX_FILE:-}" ]]; then
  echo "Specify the index.yaml to update via -i, or through the INDEX_FILE env var" >&2 && exit 1
fi
if [[ ! -f "${INDEX_FILE}" && "${CREATE:-}" != "true" ]]; then
  echo "The index ${INDEX_FILE} does not exist. Pass -c to create a new one." >&2 && exit 1
fi
if [[ -z "${RELEASE_URL:-}" && -z "${CHARTS_DIR:-}" ]]; then
  echo "Specify either a release url via -r, or a local charts directory via -d" >&2 && exit 1
fi
if [[ -n "${RELEASE_URL:-}" && -n "${CHARTS_DIR:-}" ]]; then
  echo "Specify only one of -r and -d" >&2 && exit 1
fi
OCI_BASE="${OCI_BASE:-${DEFAULT_OCI_BASE}}"
OCI_BASE="${OCI_BASE%/}"
if [[ "${OCI_BASE}" != oci://* ]]; then
  echo "The OCI registry must start with oci:// : ${OCI_BASE}" >&2 && exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
YAML_HEADER_FILE="${REPO_ROOT}/hack/headers/header.yaml.txt"
YQ="${YQ:-${REPO_ROOT}/bin/yq}"
if [[ ! -x "${YQ}" ]]; then
  echo "yq was not found at ${YQ}. Run 'make yq' first, or set the YQ env var." >&2 && exit 1
fi
HELM="${HELM:-helm}"

sha512() {
  if command -v sha512sum >/dev/null 2>&1; then
    sha512sum "$@"
  else
    shasum -a 512 "$@"
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/solr-operator-helm-index-XXXXXXXX")
trap 'rm -rf "${TMP_DIR}"' EXIT
mkdir -p "${TMP_DIR}/charts"

if [[ -n "${RELEASE_URL:-}" ]]; then
  RELEASE_URL="${RELEASE_URL%/}"
  VERSION="${VERSION:-$(basename "${RELEASE_URL}")}"
  CHART_VERSION="${VERSION#v}"
  echo "Downloading and verifying the ${VERSION} Helm charts from ${RELEASE_URL}/helm-charts"
  (
    cd "${TMP_DIR}/charts"
    for name in "${CHART_NAMES[@]}"; do
      for suffix in "" .sha512 .asc; do
        curl --fail --show-error --silent --location --retry 3 --retry-delay 5 \
          -O "${RELEASE_URL}/helm-charts/${name}-${CHART_VERSION}.tgz${suffix}"
      done
      sha512 -c "${name}-${CHART_VERSION}.tgz.sha512"
      if ! gpg --verify "${name}-${CHART_VERSION}.tgz.asc" "${name}-${CHART_VERSION}.tgz"; then
        echo "GPG verification failed for ${name}-${CHART_VERSION}.tgz. If the key is missing, import the Solr KEYS first:" >&2
        echo "  curl -sL https://dist.apache.org/repos/dist/release/solr/KEYS | gpg --import" >&2
        exit 1
      fi
      rm "${name}-${CHART_VERSION}.tgz.sha512" "${name}-${CHART_VERSION}.tgz.asc"
    done
  )
else
  shopt -s nullglob
  tarballs=("${CHARTS_DIR}"/*.tgz)
  shopt -u nullglob
  if [[ ${#tarballs[@]} -eq 0 ]]; then
    echo "No chart tarballs found in ${CHARTS_DIR}" >&2 && exit 1
  fi
  cp "${tarballs[@]}" "${TMP_DIR}/charts/"
fi

# 'helm repo index' produces the exact metadata Helm expects for each chart version (the
# Chart.yaml fields, digest and created timestamp). Only its urls are wrong for us, since it
# can only produce HTTP urls. Helm resolves an oci:// url with a tag through its OCI getter.
NEW_INDEX="${TMP_DIR}/charts/index.yaml"
"${HELM}" repo index "${TMP_DIR}/charts"
OCI_BASE="${OCI_BASE}" "${YQ}" -i \
  '.entries[][] |= (.urls = [strenv(OCI_BASE) + "/" + .name + ":" + .version])' "${NEW_INDEX}"

# Guard against indexing a chart whose digest does not match the tarball that was verified.
while IFS=$'\t' read -r name version digest; do
  actual="$(sha256_of "${TMP_DIR}/charts/${name}-${version}.tgz")"
  if [[ "${actual}" != "${digest}" ]]; then
    echo "Digest mismatch for ${name}-${version}.tgz: index has ${digest}, file is ${actual}" >&2 && exit 1
  fi
done < <("${YQ}" '.entries[][] | [.name, .version, .digest] | @tsv' "${NEW_INDEX}")

# Start from the existing index, dropping its comments (the license header is re-added below).
WORK_INDEX="${TMP_DIR}/index.yaml"
if [[ -f "${INDEX_FILE}" ]]; then
  "${YQ}" '... comments=""' "${INDEX_FILE}" > "${WORK_INDEX}"
else
  echo "Creating a new index at ${INDEX_FILE}"
  printf 'apiVersion: v1\nentries: {}\n' > "${WORK_INDEX}"
fi

while IFS=$'\t' read -r name version; do
  export N="${name}" V="${version}" NEW="${NEW_INDEX}"
  existing="$("${YQ}" '[.entries[strenv(N)][]? | select(.version == strenv(V))] | length' "${WORK_INDEX}")"
  if [[ "${existing}" -gt 0 ]]; then
    old_digest="$("${YQ}" '.entries[strenv(N)][] | select(.version == strenv(V)) | .digest' "${WORK_INDEX}")"
    new_digest="$("${YQ}" '.entries[strenv(N)][] | select(.version == strenv(V)) | .digest' "${NEW_INDEX}")"
    if [[ "${old_digest}" != "${new_digest}" ]]; then
      echo "Refusing to replace ${name} ${version}: the indexed digest ${old_digest} differs from the chart's ${new_digest}." >&2
      echo "The OCI copy must be byte-identical to the released chart." >&2
      exit 1
    fi
    echo "Replacing ${name} ${version}, keeping its original created timestamp"
    "${YQ}" -i '
      (.entries[strenv(N)][] | select(.version == strenv(V))) |=
        (load(strenv(NEW)).entries[strenv(N)][] | select(.version == strenv(V))) * {"created": .created}
    ' "${WORK_INDEX}"
  else
    echo "Adding ${name} ${version}"
    # New releases are prepended, so the index stays newest-first. Helm sorts on load anyway.
    "${YQ}" -i '
      .entries[strenv(N)] = (
        [load(strenv(NEW)).entries[strenv(N)][] | select(.version == strenv(V))] + (.entries[strenv(N)] // [])
      )
    ' "${WORK_INDEX}"
  fi
  unset N V NEW
done < <("${YQ}" '.entries[][] | [.name, .version] | @tsv' "${NEW_INDEX}")

GENERATED="$(date -u +%Y-%m-%dT%H:%M:%S.000000000Z)" "${YQ}" -i '.generated = strenv(GENERATED)' "${WORK_INDEX}"

mkdir -p "$(dirname "${INDEX_FILE}")"
{
  cat "${YAML_HEADER_FILE}"
  printf "\n\n"
  cat "${WORK_INDEX}"
} > "${INDEX_FILE}"
echo "Wrote ${INDEX_FILE}"

if [[ "${NO_SIGN:-}" != "true" ]]; then
  GPG_USER=()
  if [[ -n "${GPG_KEY:-}" ]]; then
    GPG_USER=(-u "${GPG_KEY}")
  fi
  (
    cd "$(dirname "${INDEX_FILE}")"
    index_name="$(basename "${INDEX_FILE}")"
    rm -f "${index_name}.asc"
    gpg ${GPG_USER[@]+"${GPG_USER[@]}"} --pinentry-mode loopback -ab "${index_name}"
    sha512 -b "${index_name}" > "${index_name}.sha512"
  )
  echo "Signed ${INDEX_FILE}, and wrote its checksum"
fi
