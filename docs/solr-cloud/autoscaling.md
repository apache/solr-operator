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

# SolrCloud Scaling
_Since v0.8.0_

Solr Clouds are complex distributed systems, and thus require additional help when trying to scale up or down.

Scaling/Autoscaling can mean different things in different situations, and this is true even within the `SolrCloud.spec.autoscaling` section.
- Replicas can be moved when new nodes are added or when nodes need to be taken down
- Nodes can be added/removed if more or less resources are desired.

The following sections describes all the features that the Solr Operator currently supports to aid in scaling & autoscaling SolrClouds.

## Configuration

The `autoscaling` section in the SolrCloud CRD can be configured in the following ways

```yaml
spec:
  autoscaling:
    vacatePodsOnScaleDown: true # Default: true
```

## Replica Movement

Solr can be scaled up & down either manually or by `HorizontalPodAutoscaler`'s, however no matter how the `SolrCloud.Spec.Replicas` value
changes, the Solr Operator must implement this change the same way.

For now Replicas are not scaled up and down themselves, they are just moved to utilize new Solr pods or vacate soon-to-be-deleted Solr pods.

### Solr Pod Scale-Down

When the desired number of Solr Pods that should be run `SolrCloud.Spec.Replicas` is decreased,
the `SolrCloud.spec.autoscaling.vacatePodsOnScaleDown` option determines whether the Solr Operator should move replicas
off of the pods that are about to be deleted.

When a StatefulSet, which the Solr Operator uses to run Solr pods, has its size decreased by `x` pods, it's the last
`x` pods that are deleted. So if a StatefulSet `tmp` has size 4, it will have pods `tmp-0`, `tmp-1`, `tmp-2` and `tmp-3`.
If that `tmp` then is scaled down to size 2, then pods `tmp-2` and `tmp-3` will be deleted because they are `tmp`'s last pods numerically.

If Solr has replicas placed on the pods that will be deleted as a part of the scale-down, then it has a problem.
Solr will expect that these replicas will eventually come back online, because they are a part of the clusterState.
However, the Solr Operator has no expectations for these replicas to come back, because the cloud has been scaled down.
Therefore, the safest option is to move the replicas off of these pods before the scale-down operation occurs.

If `autoscaling.vacatePodsOnScaleDown` option is not enabled, then whenever the `SolrCloud.Spec.Replicas` is decreased,
that change will be reflected in the StatefulSet immediately.
Pods will be deleted even if replicas live on those pods.

If `autoscaling.vacatePodsOnScaleDown` option is enabled, which it is by default, then the following steps occur:
1. Acquire a cluster-ops lock on the SolrCloud. (This means other cluster operations, such as a rolling restart, cannot occur during the scale down operation)
1. Scale down the last pod.
   1. Mark the pod as "notReady" so that traffic is diverted away from this pod.
   1. Check to see if the last pod has any replicas.
   1. If so, start an asynchronous command to remove replicas from this pod.
   1. Check if the async command completed, if not then loop back until the command is finished.
   1. If the command succeeded, continue, if not go back to step #2.3.
   1. Scale down the StatefulSet by 1. This will delete the pod that was just vacated.
1. If the StatefulSet size == the desired SolrCloud size, continue, otherwise go back to step #2.
1. Give up the cluster-ops lock on the SolrCloud. The scale-down operation is complete.

Because of the available Solr APIs, the statefulSet can only be scaled down 1 pod at-a-time,
this is why the Scale down step is repeated until the statefulSet size reaches the desired size.
