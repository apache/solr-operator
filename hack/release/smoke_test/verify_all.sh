#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/smoke_test/test_source.sh [-h] -v VERSION -t TAG -l LOCATION

Verify checksums and signatures of all release artifacts.
Check that the docker image contains the necessary LICENSE and NOTICE.

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -t  Tag of the Solr Operator docker image to use
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
EOF
}

OPTIND=1

while getopts hvf: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        t)  TAG=$OPTARG
            ;;
        l)  LOCATION=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  error "Specify a project version through -v, or through the VERSION env var"; exit 1
fi
if [[ -z "${TAG:-}" ]]; then
  error "Specify a docker image tag through -v, or through the TAG env var"; exit 1
fi
if [[ -z "${LOCATION:-}" ]]; then
  error "Specify an base artifact location -l, or through the LOCATION env var"; exit 1
fi

TMP_DIR=$(mktemp -d --tmpdir "solr-operator-smoke-test-source-XXXXXX")

# If LOCATION is not a URL, then get the absolute path
if ! (echo "${LOCATION}" | grep -E "http://"); then
  LOCATION=$(cd "${LOCATION}"; pwd)
fi

echo "Import Solr Keys"
curl -sL0 "https://dist.apache.org/repos/dist/release/solr/KEYS" | gpg2 --import --quiet

echo "Download all artifacts and verify signatures"
# Do all logic in temporary directory
(
  cd "${TMP_DIR}"

  if (echo "${LOCATION}" | grep -E "http"); then
    # Download Source files from the staged location
    wget -r -np -nH -nd --level=1 -P "source" "${LOCATION}/source/"

    # Download Helm files from the staged location
    wget -r -np -nH -nd --level=1 -P "helm" "${LOCATION}/helm/"

    # Download CRD files from the staged location
    wget -r -np -nH -nd --level=1 -P "crds" "${LOCATION}/crds/"
  else
    cp -r "${LOCATION}/"* .
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