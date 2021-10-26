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

# Managed SolrCloud Rolling Updates
_Since v0.2.7_

Solr Clouds are complex distributed systems, and thus require a more delicate and informed approach to rolling updates.

If the [`Managed` update strategy](solr-cloud-crd.md#update-strategy) is specified in the Solr Cloud CRD, then the Solr Operator will take control over deleting SolrCloud pods when they need to be updated.

The operator will find all pods that have not been updated yet and choose the next set of pods to delete for an update, given the following workflow.

## Pod Update Workflow

The logic goes as follows:

1. Find the pods that are out-of-date
1. Update all out-of-date pods that do not have a started Solr container.
    - This allows for updating a pod that cannot start, even if other pods are not available.
    - This step does not respect the `maxPodsUnavailable` option, because these pods have not even started the Solr process.
1. Retrieve the cluster state of the SolrCloud if there are any `ready` pods.
    - If no pods are ready, then there is no endpoint to retrieve the cluster state from.
1. Sort the pods in order of safety for being restarted. [Sorting order reference](#pod-update-sorting-order)
1. Iterate through the sorted pods, greedily choosing which pods to update. [Selection logic reference](#pod-update-selection-logic)
    - The maximum number of pods that can be updated are determined by starting with `maxPodsUnavailable`,
    then subtracting the number of updated pods that are unavailable as well as the number of not-yet-started, out-of-date pods that were updated in a previous step.
    This check makes sure that any pods taken down during this step do not violate the `maxPodsUnavailable` constraint.
    

### Pod Update Sorting Order

The pods are sorted by the following criteria, in the given order.
If any two pods on a criterion, then the next criteria (in the following order) is used to sort them.

In this context the pods sorted highest are the first chosen to be updated, the pods sorted lowest will be selected last.

1. If the pod is the overseer, it will be sorted lowest.
1. If the pod is not represented in the clusterState, it will be sorted highest.
    - A pod is not in the clusterstate if it does not host any replicas and is not the overseer.
1. Number of leader replicas hosted in the pod, sorted low -> high
1. Number of active or recovering replicas hosted in the pod, sorted low -> high
1. Number of total replicas hosted in the pod, sorted low -> high
1. If the pod is not a liveNode, then it will be sorted lower.
1. Any pods that are equal on the above criteria will be sorted lexicographically.

### Pod Update Selection Logic

Loop over the sorted pods, until the number of pods selected to be updated has reached the maximum.
This maximum is calculated by taking the given, or default, [`maxPodsUnavailable`](solr-cloud-crd.md#update-strategy) and subtracting the number of updated pods that are unavailable or have yet to be re-created.
   - If the pod is the overseer, then all other pods must be updated and available.
   Otherwise, the overseer pod cannot be updated.
   - If the pod contains no replicas, the pod is chosen to be updated.  
   **WARNING**: If you use Solr worker nodes for streaming expressions, you will likely want to set [`maxPodsUnavailable`](solr-cloud-crd.md#update-strategy) to a value you are comfortable with.
   - If Solr Node of the pod is not **`live`**, the pod is chosen to be updated.
   - If all replicas in the pod are in a **`down`** or **`recovery_failed`** state, the pod is chosen to be updated.
   - If the taking down the replicas hosted in the pod would not violate the given [`maxShardReplicasUnavailable`](solr-cloud-crd.md#update-strategy), then the pod can be updated.
   Once a pod with replicas has been chosen to be updated, the replicas hosted in that pod are then considered unavailable for the rest of the selection logic.
        - Some replicas in the shard may already be in a non-active state, or may reside on Solr Nodes that are not "live".
        The `maxShardReplicasUnavailable` calculation will take these replicas into account, as a starting point.
        - If a pod contains non-active replicas, and the pod is chosen to be updated, then the pods that are already non-active will not be double counted for the `maxShardReplicasUnavailable` calculation.

## Triggering a Manual Rolling Restart

Given these complex requirements, `kubectl rollout restart statefulset` will generally not work on a SolrCloud.

One option to trigger a manual restart is to change one of the podOptions annotations. For example you could set this to the date and time of the manual restart.


```yaml
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
spec:
  customSolrKubeOptions:
    podOptions:
      annotations:
        manualrestart: "2021-10-20T08:37:00Z"
```
