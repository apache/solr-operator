#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Verify checksums and signatures of all release artifacts
# Download can either be a URL of the base release directory, or a relative or absolute filepath.
# Use:
#   DOWNLOAD="https://dist.apache.org/repos/dist/dev/solr/solr-operator/..." ./hack/release/smoke_test/verify_all.sh
#   ./hack/release/smoke_test/verify_all.sh "https://dist.apache.org/repos/dist/dev/solr/solr-operator/...."
###
if [[ -z "${DOWNLOAD:-}" ]]; then
  if [[ -z "${2:-}" ]]; then
    error "Specify a download location through the second argument, or through the DOWNLOAD env var"
    exit 1
  else
    export DOWNLOAD="$2"
  fi
fi

TMP_DIR=$(mktemp -d --tmpdir "solr-operator-smoke-test-source-XXXXXX")

# If DOWNLOAD is not a URL, then get the absolute path
if ! (echo "${DOWNLOAD}" | grep -E "http://"); then
  DOWNLOAD=$(cd "${DOWNLOAD}"; pwd)
fi

echo "Import Solr Keys"
curl -sL0 "https://dist.apache.org/repos/dist/release/solr/KEYS" | gpg2 --import --quiet

echo "Download all artifacts and verify signatures"
# Do all logic in temporary directory
(
  cd "${TMP_DIR}"

  if (echo "${DOWNLOAD}" | grep -E "http"); then
    # Download Source files from the staged location
    wget -r -np -nH -nd --level=1 -P "source" "${DOWNLOAD}/source/"

    # Download Helm files from the staged location
    wget -r -np -nH -nd --level=1 -P "helm" "${DOWNLOAD}/helm/"

    # Download CRD files from the staged location
    wget -r -np -nH -nd --level=1 -P "crds" "${DOWNLOAD}/crds/"
  else
    cp -r "${DOWNLOAD}/"* .
  fi

  for artifact_directory in $(find * -type d); do
    (
      cd "${artifact_directory}"

      for artifact in $(find * -type f ! \( -name '*.asc' -o -name '*.sha512' \) ); do
        sha512sum -c "${artifact}.sha512" \
          || { echo "Invalid sha512 for ${artifact}. Aborting!"; exit 1; }
        gpg2 --verify "${artifact}.asc" "${artifact}" \
          || { echo "Invalid signature for ${artifact}. Aborting!"; exit 1; }
      done
    )
  done
)

# Delete temporary source download directory
rm -rf "${TMP_DIR}"

echo "Successfully verified all artifacts!"