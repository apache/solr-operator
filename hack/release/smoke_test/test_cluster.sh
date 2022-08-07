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
Usage: ./hack/release/smoke_test/test_cluster.sh [-h] [-i IMAGE] [-k KUBERNETES_VERSION] [-t SOLR_IMAGE] [-g GPG_KEY] -v VERSION -l LOCATION

Test the release candidate in a Kind cluster

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
    -i  Solr Operator docker image to use (Optional, defaults to apache/solr-operator:<version>)
    -g  GPG Key (fingerprint) used to sign the artifacts (Optional, if not provided then the helm chart will not be verified)
    -k  Kubernetes Version to test with (full tag, e.g. v1.21.2) (Optional, defaults to a compatible version)
    -t  Full solr image, or image tag (for the official Solr image), to test with (e.g. apache/solr-nightly:9.0.0, 8.11). (Optional, defaults to a compatible version)
EOF
}

OPTIND=1
while getopts hv:i:l:g:k:t: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        i)  IMAGE=$OPTARG
            ;;
        l)  LOCATION=$OPTARG
            ;;
        g)  GPG_KEY=$OPTARG
            ;;
        k)  KUBERNETES_VERSION=$OPTARG
            ;;
        t)  SOLR_IMAGE=$OPTARG
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
if [[ -z "${LOCATION:-}" ]]; then
  echo "Specify an base artifact location -l, or through the LOCATION env var" >&2 && exit 1
fi
if [[ -z "${KUBERNETES_VERSION:-}" ]]; then
  KUBERNETES_VERSION="v1.21.2"
fi
if [[ -z "${SOLR_IMAGE:-}" ]]; then
  SOLR_IMAGE="${SOLR_VERSION:-9.0}"
fi
if [[ "${SOLR_IMAGE}" != *":"* ]]; then
  SOLR_IMAGE="solr:${SOLR_IMAGE}"
fi

export LOCATION="$LOCATION"
export VERSION="$VERSION"

function add_solr_helm_repo() {
  if (echo "${LOCATION}" | grep "http"); then
    helm repo add --force-update "apache-solr-test-${VERSION}" "${LOCATION}/helm-charts"
  fi
}

function remove_solr_helm_repo() {
  if (echo "${LOCATION}" | grep "http"); then
    helm repo remove "apache-solr-test-${VERSION}"
  fi
}

# If LOCATION is not a URL, then get the absolute path
if ! (echo "${LOCATION}" | grep "http"); then
  LOCATION=$(cd "${LOCATION}"; pwd)
  LOCATION=${LOCATION%%/}

  OP_HELM_CHART="${LOCATION}/helm-charts/solr-operator-${VERSION#v}.tgz"
  SOLR_HELM_CHART="${LOCATION}/helm-charts/solr-${VERSION#v}.tgz"
else
  # If LOCATION is a URL, then we want to make sure we have the up-to-date docker image.
  docker pull "${IMAGE}"

  OP_HELM_CHART="apache-solr-test-${VERSION}/solr-operator"
  SOLR_HELM_CHART="apache-solr-test-${VERSION}/solr"
fi

if ! (which kind); then
  echo "Install Kind (Kubernetes in Docker)"
  GO111MODULE="on" go install sigs.k8s.io/kind@v0.11.1
fi

CLUSTER_NAME="solr-operator-${VERSION}-rc"
KUBE_CONTEXT="kind-${CLUSTER_NAME}"

if (kind get clusters | grep "${CLUSTER_NAME}"); then
  printf "Delete cluster, so the test starts with a clean slate.\n\n"
  kind delete clusters "${CLUSTER_NAME}"
fi

echo "Create test Kubernetes ${KUBERNETES_VERSION} cluster in Kind. This will allow us to test the CRDs, Helm chart and the Docker image."
kind create cluster --name "${CLUSTER_NAME}" --image "kindest/node:${KUBERNETES_VERSION}"

# Load the docker images into the cluster
if (docker image inspect "${IMAGE}" &>/dev/null); then
  kind load docker-image --name "${CLUSTER_NAME}" "${IMAGE}"
  printf "\nUsing local version of Solr Operator image \"${IMAGE}\".\nIf you want to use an updated version of this image, run \"docker pull ${SOLR_IMAGE}\" before running the smoke test again.\n\n"
else
  printf "\nUsing the remote Solr Operator image \"${IMAGE}\", since it was not found in the local Docker image list.\n\n"
fi
if (docker image inspect "${SOLR_IMAGE}" &>/dev/null); then
  kind load docker-image --name "${CLUSTER_NAME}" "${SOLR_IMAGE}"
  printf "\nUsing local version of Solr image \"${SOLR_IMAGE}\".\nIf you want to use an updated version of this image, run \"docker pull ${SOLR_IMAGE}\" before running the smoke test again.\n\n"
else
  printf "\nUsing the remote Solr image \"${SOLR_IMAGE}\", since it was not found in the local Docker image list.\n\n"
fi

# Add a temporary directory for backups
docker exec "${CLUSTER_NAME}-control-plane" bash -c "mkdir -p /tmp/backup"

echo "Import Solr Keys"
curl -sL0 "https://dist.apache.org/repos/dist/release/solr/KEYS" | gpg --import --quiet

# First generate the old-style public key ring, if it doesn't already exist and contain the information we want.
# Only do this if a GPG Key was provided
VERIFY_OR_NOT=""
if [[ -n "${GPG_KEY:-}" ]]; then
  VERIFY_OR_NOT="--verify"
  if ! (gpg --no-default-keyring --keyring=~/.gnupg/pubring.gpg --list-keys "${GPG_KEY}"); then
    gpg --export >~/.gnupg/pubring.gpg
  fi
fi

add_solr_helm_repo

# Install the Solr Operator
kubectl create -f "${LOCATION}/crds/all-with-dependencies.yaml" || kubectl replace -f "${LOCATION}/crds/all-with-dependencies.yaml"
helm install --kube-context "${KUBE_CONTEXT}" ${VERIFY_OR_NOT} solr-operator "${OP_HELM_CHART}" \
    --set-string image.tag="${IMAGE##*:}" \
    --set image.repository="${IMAGE%%:*}" \
    --set image.pullPolicy="Never"

printf "\nInstall a test Solr Cluster\n"
helm install --kube-context "${KUBE_CONTEXT}" ${VERIFY_OR_NOT} example "${SOLR_HELM_CHART}" \
    --set replicas=3 \
    --set image.repository="${SOLR_IMAGE%%:*}" \
    --set-string image.tag="${SOLR_IMAGE##*:}" \
    --set solrOptions.javaMemory="-Xms1g -Xmx3g" \
    --set podOptions.resources.limits.memory="1G" \
    --set podOptions.resources.requests.cpu="300m" \
    --set podOptions.resources.requests.memory="512Mi" \
    --set zk.provided.persistence.spec.resources.requests.storage="5Gi" \
    --set zk.provided.replicas=1 \
    --set "backupRepositories[0].name=local" \
    --set "backupRepositories[0].volume.source.hostPath.path=/tmp/backup"

# If LOCATION is a URL, then remove the helm repo after use
remove_solr_helm_repo

# Wait for solrcloud to be ready
printf '\nWait for all 3 Solr nodes to become ready.\n\n'
grep -q "3              3       3            3" <(exec kubectl get solrcloud example -w); kill $!

# Expose the common Solr service to localhost
kubectl port-forward service/example-solrcloud-common 18983:80 || true &
sleep 2

printf "\nCheck the admin URL to make sure it works\n"
curl --silent "http://localhost:18983/solr/admin/info/system" | grep '"status":0' > /dev/null

printf "\nCreating a test collection\n"
curl --silent "http://localhost:18983/solr/admin/collections?action=CREATE&name=smoke-test&replicationFactor=2&numShards=1" | grep '"status":0' > /dev/null

printf "\nQuery the test collection, test for 0 docs\n"
curl --silent "http://localhost:18983/solr/smoke-test/select" | grep '\"numFound\":0' > /dev/null

printf "\nCreate a Solr Backup to take local backups of the test collection\n"
cat <<EOF | kubectl apply -f -
apiVersion: solr.apache.org/v1beta1
kind: SolrBackup
metadata:
  name: ex-back
spec:
  solrCloud: example
  collections:
    - smoke-test
  location: test-dir/
  repositoryName: local
  recurrence:
    schedule: "@every 10s"
    maxSaved: 3
EOF

printf "\nCreate a Solr Prometheus Exporter to expose metrics for the Solr Cloud\n"
cat <<EOF | kubectl apply -f -
apiVersion: solr.apache.org/v1beta1
kind: SolrPrometheusExporter
metadata:
  name: example
spec:
  solrReference:
    cloud:
      name: "example"
  numThreads: 4
EOF

printf "\nWait for the Solr Prometheus Exporter to be ready\n"
sleep 5
kubectl rollout status deployment/example-solr-metrics

# Expose the Solr Prometheus Exporter service to localhost
kubectl port-forward service/example-solr-metrics 18984:80 || true &
sleep 15

printf "\nQuery the prometheus exporter, test for 'http://example-solrcloud-*.example-solrcloud-headless.default:8983/solr' (internal) URL being scraped.\n"
curl --silent "http://localhost:18984/metrics" | grep 'http://example-solrcloud-.*.example-solrcloud-headless.default:8983/solr' > /dev/null

printf "\nWait 22 seconds, so that more backups can be taken.\n"
sleep 22

printf "\nList the backups, and make sure that >= 3 have been taken (should be four), but only 3 are saved.\n"
BACKUP_RESP=$(curl --silent -L "http://localhost:18983/solr/admin/collections?action=LISTBACKUP&name=ex-back-smoke-test&repository=local&collection=smoke-test&location=/var/solr/data/backup-restore/local/test-dir")
SAVED_BACKUPS=$(echo "${BACKUP_RESP}" | jq --raw-output '.backups | length')
if [[ "${SAVED_BACKUPS}" != "3" ]]; then
    echo "Wrong number of saved backups, should be 3, found ${SAVED_BACKUPS}" >&2
    exit 1
fi
LAST_BACKUP_ID=$(echo "${BACKUP_RESP}" | jq --raw-output '.backups[-1].backupId')
if (( "${LAST_BACKUP_ID}" < 4 )); then
    echo "The last backup id must be > 3, since we should have taken at least 4 backups. Last backup id found: ${LAST_BACKUP_ID}" >&2
    exit 1
fi

printf "\nStop recurring backup\n"
cat <<EOF | kubectl apply -f -
apiVersion: solr.apache.org/v1beta1
kind: SolrBackup
metadata:
  name: ex-back
spec:
  solrCloud: example
  collections:
    - smoke-test
  location: test-dir/
  repositoryName: local
  recurrence:
    schedule: "@every 10s"
    maxSaved: 3
    disabled: true
EOF
sleep 5
LAST_BACKUP_ID=$(curl --silent -L "http://localhost:18983/solr/admin/collections?action=LISTBACKUP&name=ex-back-smoke-test&repository=local&collection=smoke-test&location=/var/solr/data/backup-restore/local/test-dir" | jq --raw-output '.backups[-1].backupId')

printf "\nWait to make sure more backups are not taken\n"
sleep 15
FOUND_BACKUP_ID=$(curl --silent -L "http://localhost:18983/solr/admin/collections?action=LISTBACKUP&name=ex-back-smoke-test&repository=local&collection=smoke-test&location=/var/solr/data/backup-restore/local/test-dir" | jq --raw-output '.backups[-1].backupId')
if (( "${FOUND_BACKUP_ID}" != "${LAST_BACKUP_ID}" )); then
    echo "The another backup has been taken since recurrence was stopped. Last backupId should be '${LAST_BACKUP_ID}', but instead found '${FOUND_BACKUP_ID}'." >&2
    exit 1
fi


printf "\nDo a rolling restart and make sure the cluster is healthy afterwards\n"
add_solr_helm_repo
helm upgrade --kube-context "${KUBE_CONTEXT}" ${VERIFY_OR_NOT} example "${SOLR_HELM_CHART}" --reuse-values  \
    --set-string podOptions.annotations.restart="true"
printf '\nWait for the rolling restart to begin.\n\n'
grep -q "3              [[:digit:]]       [[:digit:]]            0" <(exec kubectl get solrcloud example -w); kill $!
remove_solr_helm_repo

printf '\nWait 5 minutes for all 3 Solr nodes to become ready.\n\n'
grep -q "3              3       3            3" <(exec kubectl get solrcloud example -w --request-timeout 300); kill $!

# Need a new port-forward, since the last one will have broken due to all pods restarting
kubectl port-forward service/example-solrcloud-common 28983:80 || true &
sleep 2

printf "\nQuery the test collection, test for 0 docs\n\n"
curl --silent "http://localhost:28983/solr/smoke-test/select" | grep '\"numFound\":0' > /dev/null

echo "Delete test Kind Kubernetes cluster."
kind delete clusters "${CLUSTER_NAME}"

printf "\n********************\nLocal end-to-end cluster test successfully run!\n\n"
