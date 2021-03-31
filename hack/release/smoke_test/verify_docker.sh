#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/smoke_test/verify_docker.sh [-h] [-p] [-i IMAGE] [-g GIT_SHA] -v VERSION

Verify the Solr Operator Docker image.

    -h  Display this help and exit
    -p  Pull Docker image before verifying (Optional, defaults to false)
    -v  Version of the Solr Operator
    -g  GitSHA of the last commit for this version of Solr (Optional, check will not happen if not provided)
    -i  Name of the docker image to verify (Optional, defaults to apache/solr-operator:<version>)
EOF
}

OPTIND=1
while getopts hpv:i:g: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        g)  GIT_SHA=$OPTARG
            ;;
        i)  IMAGE=$OPTARG
            ;;
        p)  PULL_FIRST=true
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  echo "Specify a project version through -v, or through the VERSION env var" >&2 && exit 1
fi
if [[ -z "${IMAGE:-}" ]]; then
  IMAGE="apache/solr-operator:${VERSION}"
fi

# Pull the image, if requested
if [[ ${PULL_FIRST:-} ]]; then
  docker pull "apache/solr-operator:${IMAGE}"
fi

echo "Verify the Docker image ${IMAGE}"

# Check for the LICENSE and NOTICE info in the image
docker run --rm --entrypoint='sh' "${IMAGE}" -c "ls /etc/licenses/LICENSE ; ls /etc/licenses/NOTICE; ls /etc/licenses/dependencies/*"

# Check for Version and other information
docker run --rm -it --entrypoint='sh' "${IMAGE}" -c "/solr-operator || true" | grep "solr-operator Version: ${VERSION}" \
  || {
     echo "Could not find correct Version in Operator startup logs: ${VERSION}" >&2;
     exit 1
    }
if [[ -n "${GIT_SHA:-}" ]]; then
  docker run --rm -it --entrypoint='sh' "${IMAGE}" -c "/solr-operator || true" | grep "solr-operator Git SHA: ${GIT_SHA}" \
    || {
     echo "Could not find correct Git SHA in Operator startup logs: ${GIT_SHA}" >&2;
     exit 1
    }
fi

echo "Docker verification successful!"