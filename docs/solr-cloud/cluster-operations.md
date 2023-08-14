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

# Cluster Operations
_Since v0.8.0_

Solr Clouds are complex distributed systems, and thus any operations that deal with data availability should be handled with care.

## Cluster Operation Locks

Since cluster operations deal with Solr's index data (either the availability of it, or moving it), its safest to only allow one operation to take place at a time.
That is why these operations must first obtain a lock on the SolrCloud before execution can be started.

### Lockable Operations

- [Managed Rolling Updates](managed-updates.md)
- [Scaling Down with Replica Migrations](scaling.md#solr-pod-scale-down)
- [Scaling Up with Replica Migrations](scaling.md#solr-pod-scale-up)

### How is the Lock Implemented?

The lock is implemented as an annotation on the SolrCloud's `StatefulSet`.
The cluster operation retry queue is also implemented as an annotation.
These locks can be viewed at the following annotation keys:

- `solr.apache.org/clusterOpsLock` - The cluster operation that currently holds a lock on the SolrCloud and is executing.
- `solr.apache.org/clusterOpsRetryQueue` - The queue of cluster operations that timed out and will be retried in order after the `clusterOpsLock` is given up.


### Avoiding Deadlocks

If all cluster operations executed without any issues, there would be no need to worry about deadlocks.
Cluster operations give up the lock when the operation is complete, and then other operations that have been waiting can proceed.
Unfortunately, these cluster operations can and will fail for a number of reasons:

- Replicas have no other pod to be placed when moving off of a node. (Due to the [Replica Placement Plugin](https://solr.apache.org/guide/solr/latest/configuration-guide/replica-placement-plugins.html) used)
- There are insufficient resources to create new Solr Pods.
- The Solr Pod Template has an error and new Solr Pods cannot be started successfully.

If this is the case, then we need to be able to stop the locked cluster operation if it hasn't succeeded in a certain time period.
The cluster operation can only be stopped if there is no background task (async request) being executed in the Solr Cluster.
Once cluster operation reaches a point at which it can stop, and the locking-timeout has been exceeded or an error was found, the cluster operation is _paused_, and added to a queue to retry later.
The _timeout_ is different per-operation:
- Scaling (Up or Down): **1 minute**
- Rolling restarts: **10 minutes**

Immediately afterwards, the Solr Operator sees if there are any other operations that need to take place while before the queued cluster operation is re-started.
This allows for users to make changes to fix the reason why the cluster operation was failing.
Examples:

- **If there are insufficient resources to create new Solr Pods** \
  The user can decrease the resource requirements in the Pod Template. \
  This will create a `Rolling Update` cluster operation that will run once the `Scale Up` is paused. \
  The `Scale Up` will be dequeued when the `Rolling Update` is complete, and can now complete because there are more available resources in the Kubernetes Cluster.

- **Scale Down is failing because a replica from the scaled-down pod has nowhere to be moved to** \
  The user can see this error in the logs, and know that the scale down won't work for their use case. \
  Instead they will have to scale the SolrCloud to the number of pods that the `StatefulSet` is currently running. \
  Once the `Scale Down` is paused, it will be replaced by a `Scale Up` operation to current number of running pods. \
  This doesn't actually increase the number of pods, but it will issue a command to Solr to balance replicas across all pods, to make sure the cluster is well-balanced after the failed `ScaleDown`.

If a queued operation is going to be retried, the Solr Operator first makes sure that its values are still valid.
For the `Scale Down` example above, when the Solr Operator tries to restart the queued `Scale Down` operation, it sees that the `SolrCloud.Spec.Replicas` is no longer lower than the current number of Solr Pods.
Therefore, the `Scale Down` does not need to be retried, and a "fake" `Scale Up` needs to take place.

### In the case of an emergency

When all else fails, and you need to stop a cluster operation, you can remove the lock annotation from the `StatefulSet` manually.

Edit the StatefulSet (e.g. `kubectl edit statefulset <name>`) and remove the cluster operation lock annotation: `solr.apache.org/clusterOpsLock`

This can be done via the following command:

```bash
$ kubectl annotate statefulset ${statefulSetName} solr.apache.org/clusterOpsLock-
```

This will only remove the current running cluster operation, if other cluster operations have been queued, they will be retried once the lock annotation is removed.
Also if the operation still needs to occur to put the SolrCloud in its expected state, then the operation will be retried once a lock can be acquired.
The only way to have the cluster operation not run again is to put the SolrCloud back to its previous state (for scaling, set `SolrCloud.Spec.replicas` to the value found in `StatefulSet.Spec.replicas`).
If the SolrCloud requires a rolling restart, it cannot be "put back to its previous state". The only way to move forward is to either delete the `StatefulSet` (a very dangerous operation), or find a way to allow the `RollingUpdate` operation to succeed.