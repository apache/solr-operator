<!--
    Licensed to the Apache Software Foundation (ASF) under one or more
    contributor license agreements.  See the NOTICE file distributed with
    this work for additional information regarding copyright ownership.
    The ASF licenses this file to You under the Apache License, Version 2.0
    the "License"); you may not use this file except in compliance with
    the License.  You may obtain a copy of the License at

        http://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.
 -->

# Upgrading the Solr Operator from Bloomberg to Apache

Please read the guide fully before attempting to upgrade your system.

## Guide

There are many ways of managing what runs in your Kubernetes cluster.
Therefore, we cannot give one definitive way of upgrading the Solr Operator and the Solr resources in your Cluster.

Below are guides based on how to upgrade, depending on the type of management that you use.
Please look through all the instructions to see what will work best for your setup.

## If you manage your cluster declaratively

If you have an external system managing your cluster (e.g. Ansible, Helm, etc), then the steps you take to upgrade the Solr Operator may be very different.

1. Make sure your system is installing the `v0.3.0` version of the Solr Operator, through the [Helm chart](https://artifacthub.io/packages/helm/apache-solr/solr-operator), or other means.
1. Make sure your system is [installing the `v0.2.9` version of the Zookeeper Operator](#zookeeper-operator-upgrade).
1. Make sure the Solr resources you are installing are using the new `solr.apache.org` group and not `solr.bloomberg.com`.
1. Check if the Solr resources you are managing use any fields deprecated in `v0.2.x` versions, you can check for these in the [Upgrade Notes](upgrade-notes.md#upgrade-warnings-and-notes).
    - If your resources contain these fields, replace them with their new locations.
    Otherwise, your `kubectl apply` commands will fail when using the `solr.apache.org` CRDs.

Once you have made sure that your declarative system is checked for the above points, you can run it to install the new version of the Solr Operator and all of your new CRDs.

Now you need to delete all the old `solr.bloomberg.com` resources & operator.

1. Remove the `v0.2.x` version of the Solr Operator that is running, unless you merely upgraded your existing deployment to `v0.3.0`.
1. [Remove any finalizers](#remove-solr-resource-finalizers) that the Solr Resources may contain, in all namespaces that you run Solr resources in.
1. [Remove the `solr.bloomberg.com` CRDs](#remove-bloomberg-solr-crds), as they are no longer needed when running the `v0.3.0` version of the Solr Operator.

## If you manually manage your cluster

If you manually manage your cluster, then the instructions below will allow you to upgrade in-place.

1. Make sure you have the latest Apache Solr Operator helm charts (If you use helm to deploy the Solr Operator)
   ```bash
    helm repo add apache-solr https://solr.apache.org/charts
    helm repo update
    ```
1. Upgrade to `v0.2.8`, if you are running a version lower.
   - If your Solr Operator deployment is managed with a Helm chart, merely upgrade your helm chart to version `0.2.8`
1. Install the v0.2.8 Solr Operator CRDs
   ```bash
    kubectl replace -f "https://solr.apache.org/operator/downloads/crds/v0.2.8/all.yaml"
    ```
   _The Upgrade Notes page says to always upgrade CRDs before upgrading the operator, however for this single upgrade case this order is safer._
1. _If you are using the ZK Operator_  
   [Remove the outdated Zookeeper Operator deployment and other resources](#removing-the-old-zookeeper-operator-resources).
1. Install the Apache Solr CRDs
   ```bash
    kubectl create -f "https://solr.apache.org/operator/downloads/crds/v0.3.0/all.yaml"
    ```
   _If you are using the ZK Operator_
   ```bash
    kubectl create -f "https://solr.apache.org/operator/downloads/crds/v0.3.0/all-with-dependencies.yaml" || \
      kubectl replace -f "https://solr.apache.org/operator/downloads/crds/v0.3.0/all-with-dependencies.yaml"
    ```
1. Install the Apache Solr Operator
   ```bash
    helm install apache apache-solr/solr-operator --version 0.3.0
    ```
1. Convert `solr.bloomberg.com` resources into `solr.apache.org` resources:
   ```bash
   # First make sure that "yq" is installed
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
   ```
1. Delete the `v0.2.8` Solr Operator deployment:
   ```bash
   # If solr-operator is the name of your v0.2.8 Solr Operator deployment
   helm delete solr-operator 
   ```
1. [Remove any finalizers](#remove-solr-resource-finalizers) that the Solr Resources may contain, in all namespaces that you run Solr resources in.
1. [Remove the `solr.bloomberg.com` CRDs](#remove-bloomberg-solr-crds), as they are no longer needed when running the `v0.3.0` version of the Solr Operator.
1. Test to make sure that your Solr resources exist as apache resources now and your `StatefulSets`/`Deployments` are still running correctly.

## Other Considerations

### Zookeeper Operator Upgrade

The [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) version that the Solr Operator requires has changed between `v0.2.6` and `v0.3.0`.
Therefore, the Zookeeper Operator needs to be upgraded to `v0.2.9` when the Solr Operator is upgraded to `v0.3.0`.

#### Removing the old Zookeeper Operator resources
If you use the Solr Operator [Helm chart](https://artifacthub.io/packages/helm/apache-solr/solr-operator), then the correct version of the Zookeeper Operator will be deployed when upgrading.
However, you will need to pre-emptively delete some Kubernetes resources so that they can be managed by Helm.

_Only use the below `kubectl` commands if you installed the Zookeeper Operator using the URL provided in the Solr Operator repository._  
If you already have a `v0.2.9` Zookeeper Operator running, ignore this and pass the following options when installing the Solr Operator:
`--set zookeeper-operator.install=false --set zookeeper-operator.use=true`

```bash
kubectl delete deployment zk-operator
kubectl delete clusterrolebinding zookeeper-operator-cluster-role-binding
kubectl delete clusterrole zookeeper-operator
kubectl delete serviceaccount zookeeper-operator
```

You will then be able to install the Solr Operator & dependency CRDs, and the Solr Operator Helm Chart.


### Remove Solr Resource Finalizers

It is necessary to remove any finalizers that the old `solr.bloomberg.com` resources may have, otherwise they will not be able to be deleted.
You should do this for all namespaces that you are running solr resources in.

**Only run these after stopping any `v0.2.x` Solr Operator that may be running in the Kubernetes cluster.**

```bash
kubectl get solrcollections.solr.bloomberg.com -o name | sed -e 's/.*\///g' | xargs -I {} kubectl patch solrcollections.solr.bloomberg.com {} --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers/0"}]'
kubectl get solrcollectionaliases.solr.bloomberg.com -o name | sed -e 's/.*\///g' | xargs -I {} kubectl patch solrcollectionaliases.solr.bloomberg.com {} --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers/0"}]'
kubectl get solrclouds.solr.bloomberg.com -o name | sed -e 's/.*\///g' | xargs -I {} kubectl patch solrclouds.solr.bloomberg.com {} --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers/0"}]'
```

### Remove Bloomberg Solr CRDs

After you have [removed all finalizers on Solr resources](#remove-solr-resource-finalizers), you can remove all `solr.bloomberg.com` CRDs.

If you don't want to disrupt any Solr service that is running, make sure that all Solr resources have been upgraded from `solr.bloomberg.com` CRDs to `solr.apache.org` CRDs.
This command will delete all `solr.bloomberg.com` resources, which means that only resources that exist as `solr.apache.org` will continue to run afterwards.

```bash
kubectl delete crd solrclouds.solr.bloomberg.com solrprometheusexporters.solr.bloomberg.com solrbackups.solr.bloomberg.com solrcollections.solr.bloomberg.com solrcollectionaliases.solr.bloomberg.com
```
