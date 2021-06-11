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

# ********   WARNING   ******
# Only run this script on a clean testing cluster. It will test the upgrade from v0.2.8 -> v0.3.0
# ***************************

# Clear your kube environment
kubectl delete crds zookeeperclusters.zookeeper.pravega.io solrbackups.solr.apache.org solrclouds.solr.apache.org solrprometheusexporters.solr.apache.org \
    solrclouds.solr.bloomberg.com solrprometheusexporters.solr.bloomberg.com solrbackups.solr.bloomberg.com solrcollections.solr.bloomberg.com solrcollectionaliases.solr.bloomberg.com; \
    kubectl delete pvc --all; helm delete solr-operator apache
helm ls -a --all-namespaces | awk 'NR > 1 { print  "-n "$2, $1}' | xargs -L1 helm delete

helm repo add apache-solr https://solr.apache.org/charts && helm repo update

####
##  Setup an environment using the v0.2.6 Solr Operator
####

# Install the Zookeeper Operator
kubectl apply -f https://apache.github.io/solr-operator/example/dependencies/zk_operator.yaml

# Install a past version of the Solr Operator
helm install solr-operator apache-solr/solr-operator --version 0.2.6 --set ingressBaseDomain=localhost.com
kubectl rollout status deployment/solr-operator

# Install an out-of-date Solr Cloud
cat <<EOF | kubectl apply -f -
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCloud
metadata:
  name: example
spec:
  dataPvcSpec:
    resources:
      requests:
        storage: "5Gi"
  replicas: 3
  solrImage:
    tag: 8.5.0
  solrJavaMem: "-Xms1g -Xmx3g"
  customSolrKubeOptions:
    podOptions:
      resources:
        limits:
          memory: "1G"
        requests:
          cpu: "65m"
          memory: "156Mi"
  zookeeperRef:
    provided:
      chroot: "/this/will/be/auto/created"
      zookeeper:
        persistence:
          spec:
            storageClassName: "hostpath"
            resources:
              requests:
                storage: "5Gi"
        replicas: 1
        zookeeperPodPolicy:
          resources:
            limits:
              memory: "1G"
            requests:
              cpu: "65m"
              memory: "156Mi"
EOF

# Wait for solrcloud to be ready
sleep 60
kubectl rollout status statefulset/example-solrcloud

# Install a SolrCollection and PrometheusExporter
cat <<EOF | kubectl apply -f -
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCollection
metadata:
  name: example-collection-1
spec:
  solrCloud: example
  collection: example-collection
  routerName: compositeId
  autoAddReplicas: false
  numShards: 2
  replicationFactor: 1
  maxShardsPerNode: 1
  collectionConfigName: "_default"
---
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrPrometheusExporter
metadata:
  name: example
spec:
  solrReference:
    cloud:
      name: "example"
  numThreads: 4
  image:
    tag: 8.5.0
EOF

# Wait for solrmetrics to be ready
sleep 5
kubectl rollout status deployment/example-solr-metrics

####
##  Upgrade the Solr Operator to v0.2.8
####
helm upgrade solr-operator apache-solr/solr-operator --version 0.2.8 --set ingressBaseDomain=localhost.com

kubectl replace -f https://solr.apache.org/operator/downloads/crds/v0.2.8/all-with-dependencies.yaml



# Wait until everything is up to date and the cluster is calm
kubectl get solrcloud -w

####

####
##  Install the Apache Solr Operator separately
####
# Build the Apache release (TODO: Change to release when it is cut)
make docker-build
helm dependency build helm/solr-operator

# Uninstall Old ZK Operator
echo "Deleting all Zookeeper Operator resources, except for CRDs" && \
  kubectl delete deployment zk-operator && \
  kubectl delete clusterrolebinding zookeeper-operator-cluster-role-binding && \
  kubectl delete clusterrole zookeeper-operator && \
  kubectl delete serviceaccount zookeeper-operator

# Install new ZookeeperCluster CRD
kubectl replace -f https://raw.githubusercontent.com/pravega/zookeeper-operator/v0.2.9/deploy/crds/zookeeper.pravega.io_zookeeperclusters_crd.yaml

helm install apache helm/solr-operator --set image.tag=latest --set image.pullPolicy=Never

####
##  Convert BB Solr resources to Apache Solr resources
####
kubectl get solrclouds.solr.bloomberg.com --all-namespaces -o yaml | \
  sed "s#solr.bloomberg.com#solr.apache.org#g" | \
  yq eval 'del(.items.[].metadata.annotations."kubectl.kubernetes.io/last-applied-configuration", .items.[].metadata.managedFields, .items.[].metadata.resourceVersion, .items.[].metadata.creationTimestamp, .items.[].metadata.generation, .items.[].metadata.selfLink, .items.[].metadata.uid, .items.[].spec.solrPodPolicy, .items.[].spec.zookeeperRef.provided.image.tag, .items.[].status)' - \
  | kubectl apply -f -
kubectl get solrprometheusexporters.solr.bloomberg.com --all-namespaces -o yaml | \
  sed "s#solr.bloomberg.com#solr.apache.org#g" | \
  yq eval 'del(.items.[].metadata.annotations."kubectl.kubernetes.io/last-applied-configuration", .items.[].metadata.managedFields, .items.[].metadata.resourceVersion, .items.[].metadata.creationTimestamp, .items.[].metadata.generation, .items.[].metadata.selfLink, .items.[].metadata.uid, .items.[].spec.podPolicy, .items.[].status)' - \
  | kubectl apply -f -
kubectl get solrbackups.solr.bloomberg.com --all-namespaces -o yaml | \
  sed "s#solr.bloomberg.com#solr.apache.org#g" | \
  yq eval 'del(.items.[].metadata.annotations."kubectl.kubernetes.io/last-applied-configuration", .items.[].metadata.managedFields, .items.[].metadata.resourceVersion, .items.[].metadata.creationTimestamp, .items.[].metadata.generation, .items.[].metadata.selfLink, .items.[].metadata.uid, .items.[].status)' - \
  | kubectl apply -f -


####
##  Uninstall the Bloomberg Solr Operator and resources
####

helm delete solr-operator

# Remove finalizers for Collections and CollectionAliases
kubectl get solrclouds.solr.bloomberg.com -o name | sed -e 's/.*\///g' | xargs -I {} kubectl patch solrclouds.solr.bloomberg.com {} --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers/0"}]'
kubectl get solrcollections.solr.bloomberg.com -o name | sed -e 's/.*\///g' | xargs -I {} kubectl patch solrcollections.solr.bloomberg.com {} --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers/0"}]'
kubectl get solrcollectionaliases.solr.bloomberg.com -o name | sed -e 's/.*\///g' | xargs -I {} kubectl patch solrcollectionaliases.solr.bloomberg.com {} --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers/0"}]'

# Uninstall the Bloomberg resources
kubectl delete solrclouds.solr.bloomberg.com --all --all-namespaces
kubectl delete solrprometheusexporters.solr.bloomberg.com --all --all-namespaces
kubectl delete solrbackups.solr.bloomberg.com --all --all-namespaces
kubectl delete solrcollections.solr.bloomberg.com --all --all-namespaces
kubectl delete solrcollectionaliases.solr.bloomberg.com --all --all-namespaces

# Remove Bloomberg CRDs
kubectl delete crd solrclouds.solr.bloomberg.com solrprometheusexporters.solr.bloomberg.com solrbackups.solr.bloomberg.com solrcollections.solr.bloomberg.com solrcollectionaliases.solr.bloomberg.com

# Watch Solr upgrade for v0.3.0
kubectl get solr -w
