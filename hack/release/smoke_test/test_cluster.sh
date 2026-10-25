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
Usage: ./hack/release/smoke_test/test_cluster.sh [-h] [-i IMAGE] [-k KUBERNETES_VERSION] [-t SOLR_IMAGE] [-g GPG_KEY] [-o OCI_REGISTRY] -v VERSION -l LOCATION

Test the release candidate in a Kind cluster

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
    -i  Solr Operator docker image to use (Optional, defaults to apache/solr-operator:<version>)
    -g  GPG Key (fingerprint) used to sign the artifacts (Optional, if not provided then the helm chart will not be verified)
    -k  Kubernetes Version to test with (full tag, e.g. v1.26.6) (Optional, defaults to a compatible version)
    -t  Full solr image, or image tag (for the official Solr image), to test with (e.g. apache/solr-nightly:9.0.0, 9.10.0). (Optional, defaults to a compatible version)
    -o  OCI registry to install the Helm charts from, e.g. oci://ghcr.io/apache/solr-operator/helm (Optional)
        Use this after a release has been published, to smoke test the published OCI artifacts.
        The CRDs are still taken from LOCATION, since they are not part of the OCI artifacts.
EOF
}

OPTIND=1
while getopts hv:i:l:g:k:t:o: opt; do
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
        o)  OCI_REGISTRY=$OPTARG
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
  KUBERNETES_VERSION="v1.26.6"
fi
if [[ -z "${SOLR_IMAGE:-}" ]]; then
  SOLR_IMAGE="${SOLR_VERSION:-9.10.0}"
fi
if [[ "${SOLR_IMAGE}" != *":"* ]]; then
  SOLR_IMAGE="solr:${SOLR_IMAGE}"
fi

export LOCATION="$LOCATION"
export VERSION="$VERSION"

# An OCI registry is never a helm repo, so there is nothing to add or remove for it.
function add_solr_helm_repo() {
  if [[ -z "${OCI_REGISTRY:-}" ]] && (echo "${LOCATION}" | grep "http"); then
    helm repo add --force-update "apache-solr-test-${VERSION}" "${LOCATION}/helm-charts"
  fi
}

function remove_solr_helm_repo() {
  if [[ -z "${OCI_REGISTRY:-}" ]] && (echo "${LOCATION}" | grep "http"); then
    helm repo remove "apache-solr-test-${VERSION}"
  fi
}

# Wait until a backgrounded 'kubectl port-forward' is accepting connections.
#
# A fixed sleep races with the port-forward: on a loaded machine the forward can print
# "Forwarding from ..." only after the first request has already been refused, and with
# 'set -e' that aborts the whole smoke test. Any HTTP response means the forward is up,
# so a non-zero curl exit (7 for a refused connection) is what we retry on.
function wait_for_port_forward() {
  local port="$1"
  local timeout=60
  local deadline=$((SECONDS + timeout))
  local probe
  while (( SECONDS < deadline )); do
    # Bound each probe by the time actually left. A port that is listening but not yet
    # proxying accepts the connection and then hangs, so a fixed --max-time per attempt
    # would let an attempt count overrun the deadline several times over. Never pass 0,
    # which curl reads as "no timeout at all".
    probe=$((deadline - SECONDS))
    if (( probe > 2 )); then
      probe=2
    fi
    if curl --silent --max-time "${probe}" --output /dev/null "http://127.0.0.1:${port}/"; then
      return 0
    fi
    sleep 1
  done
  echo "Timed out after ${timeout}s waiting for the port-forward on localhost:${port} to accept connections" >&2
  return 1
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

# Decide how the Solr Operator image gets into the Kind cluster.
#
# When LOCATION is a URL the image was just pulled from a registry, so the Kind nodes can
# pull it themselves. When LOCATION is a local path the image is typically a local build that
# exists nowhere else, so it has to be loaded into the nodes and must never be pulled.
#
# Only locally built images are loaded. Loading a registry image fails with Docker's
# containerd image store (the default since Docker Engine 29): 'kind load docker-image'
# exports the full multi-platform index, but only the host platform's content was pulled,
# so the import dies with "content digest sha256:...: not found".
# See https://github.com/kubernetes-sigs/kind/issues/3795
if (echo "${LOCATION}" | grep -q "http"); then
  LOAD_OPERATOR_IMAGE=false
  OPERATOR_PULL_POLICY="IfNotPresent"
else
  LOAD_OPERATOR_IMAGE=true
  OPERATOR_PULL_POLICY="Never"
fi

# The charts published to an OCI registry are the same artifacts that were staged at
# LOCATION, so only where the charts are pulled from changes. CRDs still come from LOCATION.
# Unlike a helm repo, an OCI reference has no index, so the version must be given explicitly.
CHART_VERSION_ARG=()
if [[ -n "${OCI_REGISTRY:-}" ]]; then
  OP_HELM_CHART="${OCI_REGISTRY}/solr-operator"
  SOLR_HELM_CHART="${OCI_REGISTRY}/solr"
  CHART_VERSION_ARG=(--version "${VERSION#v}")
fi

if ! (which kind); then
  echo "Install Kind (Kubernetes in Docker)"
  GO111MODULE="on" go install sigs.k8s.io/kind@v0.20.0
fi

CLUSTER_NAME="solr-operator-${VERSION}-rc"
KUBE_CONTEXT="kind-${CLUSTER_NAME}"

if (kind get clusters | grep "${CLUSTER_NAME}"); then
  printf "Delete cluster, so the test starts with a clean slate.\n\n"
  kind delete clusters "${CLUSTER_NAME}"
fi

echo "Create test Kubernetes ${KUBERNETES_VERSION} cluster in Kind. This will allow us to test the CRDs, Helm chart and the Docker image."
kind create cluster --name "${CLUSTER_NAME}" --image "kindest/node:${KUBERNETES_VERSION}"

# Load the Solr Operator image into the cluster, but only when it is a local build.
# The Solr image is never loaded: it always comes from a registry, and the Kind nodes pull it.
if [[ "${LOAD_OPERATOR_IMAGE}" = true ]]; then
  if ! (docker image inspect "${IMAGE}" &>/dev/null); then
    echo "Solr Operator image \"${IMAGE}\" was not found locally. Build it locally before running this smoke test with a local artifact location." >&2
    exit 1
  fi
  printf "\nLoading local Solr Operator image \"${IMAGE}\" into the Kind cluster.\n\n"
  if ! kind load docker-image --name "${CLUSTER_NAME}" "${IMAGE}"; then
    cat >&2 <<ERR

Failed to load "${IMAGE}" into Kind. If the error above mentions a missing content digest,
Docker's containerd image store exported a multi-platform index whose content is not all
present locally. Only locally built, single-platform images can be loaded this way.
See https://github.com/kubernetes-sigs/kind/issues/3795
ERR
    exit 1
  fi
else
  printf "\nThe Kind nodes will pull the Solr Operator image \"${IMAGE}\" from its registry.\n\n"
fi
printf "The Kind nodes will pull the Solr image \"${SOLR_IMAGE}\" from its registry.\n\n"

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
helm install --kube-context "${KUBE_CONTEXT}" ${VERIFY_OR_NOT} solr-operator "${OP_HELM_CHART}" ${CHART_VERSION_ARG[@]+"${CHART_VERSION_ARG[@]}"} \
    --set-string image.tag="${IMAGE##*:}" \
    --set image.repository="${IMAGE%%:*}" \
    --set image.pullPolicy="${OPERATOR_PULL_POLICY}"

printf "\nInstall a test Solr Cluster\n"
helm install --kube-context "${KUBE_CONTEXT}" ${VERIFY_OR_NOT} example "${SOLR_HELM_CHART}" ${CHART_VERSION_ARG[@]+"${CHART_VERSION_ARG[@]}"} \
    --set replicas=2 \
    --set image.repository="${SOLR_IMAGE%%:*}" \
    --set-string image.tag="${SOLR_IMAGE##*:}" \
    --set solrOptions.javaMemory="-Xms700m -Xmx700m" \
    --set podOptions.resources.limits.cpu="500m" \
    --set podOptions.resources.limits.memory="1Gi" \
    --set podOptions.resources.requests.cpu="500m" \
    --set podOptions.resources.requests.memory="1Gi" \
    --set zk.provided.persistence.spec.resources.requests.storage="5Gi" \
    --set zk.provided.replicas=1 \
    --set "backupRepositories[0].name=local" \
    --set "backupRepositories[0].volume.source.hostPath.path=/tmp/backup"

# If LOCATION is a URL, then remove the helm repo after use
remove_solr_helm_repo

# Wait for solrcloud to be ready
printf '\nWait for all 2 Solr nodes to become ready.\n\n'
grep -q "2              2       2            2" <(exec kubectl get solrcloud example -w); kill $!

# Expose the common Solr service to localhost
kubectl port-forward service/example-solrcloud-common 18983:80 || true &
wait_for_port_forward 18983

printf "\nCheck the admin URL to make sure it works\n"
curl --silent "http://localhost:18983/solr/admin/info/health" | grep '"status":0' > /dev/null

printf "\nCreating a test collection\n"
curl --silent "http://localhost:18983/solr/admin/collections?action=CREATE&name=smoke-test&replicationFactor=1&numShards=2" | grep '"status":0' > /dev/null

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
helm upgrade --kube-context "${KUBE_CONTEXT}" ${VERIFY_OR_NOT} example "${SOLR_HELM_CHART}" ${CHART_VERSION_ARG[@]+"${CHART_VERSION_ARG[@]}"} --reuse-values  \
    --set-string podOptions.annotations.restart="true"
printf '\nWait for the rolling restart to begin.\n\n'
grep -q "2              [[:digit:]]       [[:digit:]]            0" <(exec kubectl get solrcloud example -w); kill $!
remove_solr_helm_repo

printf '\nWait 5 minutes for all 2 Solr nodes to become ready.\n\n'
grep -q "2              2       2            2" <(exec kubectl get solrcloud example -w --request-timeout 300); kill $!

# Need a new port-forward, since the last one will have broken due to all pods restarting
kubectl port-forward service/example-solrcloud-common 28983:80 || true &
wait_for_port_forward 28983

printf "\nQuery the test collection, test for 0 docs\n\n"
curl --silent "http://localhost:28983/solr/smoke-test/select" | grep '\"numFound\":0' > /dev/null

echo "Delete test Kind Kubernetes cluster."
kind delete clusters "${CLUSTER_NAME}"

printf "\n********************\nLocal end-to-end cluster test successfully run!\n\n"
