/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"context"
	"fmt"
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	DefaultMaxPodsUnavailable          = "25%"
	DefaultMaxShardReplicasUnavailable = 1

	SolrScheduledRestartAnnotation = "solr.apache.org/nextScheduledRestart"
)

func ScheduleNextRestart(restartSchedule string, podTemplateAnnotations map[string]string) (nextRestart string, reconcileWaitDuration *time.Duration, err error) {
	return scheduleNextRestartWithTime(restartSchedule, podTemplateAnnotations, time.Now())
}

func scheduleNextRestartWithTime(restartSchedule string, podTemplateAnnotations map[string]string, currentTime time.Time) (nextRestart string, reconcileWaitDuration *time.Duration, err error) {
	lastScheduledTime := currentTime.UTC()
	if restartSchedule == "" {
		return
	}
	scheduledTime, hasScheduled := podTemplateAnnotations[SolrScheduledRestartAnnotation]

	scheduleNextRestart := false

	if hasScheduled {
		parsedScheduledTime, parseErr := time.Parse(time.RFC3339, scheduledTime)
		if parseErr != nil {
			// If the scheduled time cannot be parsed, then go ahead and create a new time.
			scheduleNextRestart = true
		} else {
			parsedScheduledTime = parsedScheduledTime.UTC()
			if parsedScheduledTime.Before(currentTime) {
				// If the already-scheduled time is passed, then schedule a new one.
				scheduleNextRestart = true
				lastScheduledTime = parsedScheduledTime
			} else {
				// If the already-scheduled time is in the future, re-reconcile at that time
				reconcileWaitDurationTmp := parsedScheduledTime.Sub(currentTime)
				reconcileWaitDuration = &reconcileWaitDurationTmp
			}
		}
	} else {
		scheduleNextRestart = true
	}

	if scheduleNextRestart {
		if parsedSchedule, parseErr := cron.ParseStandard(restartSchedule); parseErr != nil {
			err = parseErr
		} else {
			nextRestartTime := parsedSchedule.Next(lastScheduledTime)
			nextRestart = nextRestartTime.Format(time.RFC3339)
			reconcileWaitDurationTmp := nextRestartTime.Sub(currentTime)
			reconcileWaitDuration = &reconcileWaitDurationTmp
		}
	}
	return
}

// DeterminePodsSafeToUpdate takes a list of solr Pods and returns a list of pods that are safe to upgrade now.
// This function MUST be idempotent and return the same list of pods given the same kubernetes/solr state.
//
// NOTE: It is assumed that the list of pods provided are all started.
// If an out of date pod has a solr container that is not started, it should be accounted for in outOfDatePodsNotStartedCount not outOfDatePods.
//
// TODO:
//   - Think about caching this for ~250 ms? Not a huge need to send these requests milliseconds apart.
//   - Might be too much complexity for very little gain.
func DeterminePodsSafeToUpdate(ctx context.Context, cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, readyPods int, availableUpdatedPodCount int, outOfDatePodsNotStartedCount int, logger logr.Logger) (podsToUpdate []corev1.Pod, podsHaveReplicas map[string]bool, retryLater bool) {
	// Before fetching the cluster state, be sure that there is room to update at least 1 pod
	maxPodsUnavailable, unavailableUpdatedPodCount, maxPodsToUpdate := calculateMaxPodsToUpdate(cloud, len(outOfDatePods), outOfDatePodsNotStartedCount, availableUpdatedPodCount)
	if maxPodsToUpdate <= 0 {
		logger.Info("Pod update selection canceled. The number of updated pods unavailable equals or exceeds the calculated maxPodsUnavailable.",
			"unavailableUpdatedPods", unavailableUpdatedPodCount, "outOfDatePodsNotStarted", outOfDatePodsNotStartedCount, "maxPodsUnavailable", maxPodsUnavailable)
	} else {
		clusterResp := &solr_api.SolrClusterStatusResponse{}
		overseerResp := &solr_api.SolrOverseerStatusResponse{}

		if readyPods > 0 {
			queryParams := url.Values{}
			queryParams.Add("action", "CLUSTERSTATUS")
			err := solr_api.CallCollectionsApi(ctx, cloud, queryParams, clusterResp)
			if err == nil {
				if hasError, apiErr := solr_api.CheckForCollectionsApiError("CLUSTERSTATUS", clusterResp.ResponseHeader); hasError {
					err = apiErr
				} else {
					queryParams.Set("action", "OVERSEERSTATUS")
					err = solr_api.CallCollectionsApi(ctx, cloud, queryParams, overseerResp)
					if hasError, apiErr = solr_api.CheckForCollectionsApiError("OVERSEERSTATUS", clusterResp.ResponseHeader); hasError {
						err = apiErr
					}
				}
			} else {
				logger.Error(err, "Error retrieving cluster status, delaying pod update selection")
				// If there is an error fetching the clusterState, retry later.
				retryLater = true
			}
		}
		// If the update logic already wants to retry later, then do not pick any pods
		if !retryLater {
			logger.Info("Pod update selection started.", "outOfDatePods", len(outOfDatePods), "maxPodsUnavailable", maxPodsUnavailable, "unavailableUpdatedPods", unavailableUpdatedPodCount, "outOfDatePodsNotStarted", outOfDatePodsNotStartedCount, "maxPodsToUpdate", maxPodsToUpdate)
			podsToUpdate, podsHaveReplicas = pickPodsToUpdate(cloud, outOfDatePods, clusterResp.ClusterStatus, overseerResp.Leader, maxPodsToUpdate, logger)

			// If there are no pods to upgrade, even though the maxPodsToUpdate is >0, then retry later because the issue stems from cluster state
			// and clusterState changes will not call the reconciler.
			if len(podsToUpdate) == 0 && len(outOfDatePods) > 0 {
				retryLater = true
			}
		}
	}
	return podsToUpdate, podsHaveReplicas, retryLater
}

// calculateMaxPodsToUpdate determines the maximum number of additional pods that can be updated.
func calculateMaxPodsToUpdate(cloud *solr.SolrCloud, outOfDatePodCount int, outOfDatePodsNotStartedCount int, availableUpdatedPodCount int) (maxPodsUnavailable int, unavailableUpdatedPodCount int, maxPodsToUpdate int) {
	totalPods := int(*cloud.Spec.Replicas)
	// In order to calculate the number of updated pods that are unavailable take all pods, take the total pods and subtract those that are available and updated, and those that are not updated.
	unavailableUpdatedPodCount = totalPods - availableUpdatedPodCount - outOfDatePodCount - outOfDatePodsNotStartedCount
	// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes.
	maxPodsUnavailable, _ = ResolveMaxPodsUnavailable(cloud.Spec.UpdateStrategy.ManagedUpdateOptions.MaxPodsUnavailable, totalPods)
	// Subtract:
	//   - unavailableUpdatedPodCount, because those pods are already unavailable
	//   - outOfDatePodsNotStartedCount, because those pods will always be deleted first and affect the number of unavailable pods
	maxPodsToUpdate = maxPodsUnavailable - unavailableUpdatedPodCount - outOfDatePodsNotStartedCount
	return maxPodsUnavailable, unavailableUpdatedPodCount, maxPodsToUpdate
}

func pickPodsToUpdate(cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, clusterStatus solr_api.SolrClusterStatus,
	overseer string, maxPodsToUpdate int, logger logr.Logger) (podsToUpdate []corev1.Pod, podsHaveReplicas map[string]bool) {
	podsHaveReplicas = make(map[string]bool, maxPodsToUpdate)
	nodeContents, totalShardReplicas, shardReplicasNotActive, allManagedPodsLive := findSolrNodeContents(clusterStatus, overseer, GetAllManagedSolrNodeNames(cloud))
	sortNodePodsBySafety(outOfDatePods, nodeContents, cloud)

	updateOptions := cloud.Spec.UpdateStrategy.ManagedUpdateOptions
	var maxShardReplicasUnavailableCache map[string]int
	// In case the user wants all shardReplicas to be unavailable at the same time, populate the cache with the total number of replicas per shard.
	if updateOptions.MaxShardReplicasUnavailable != nil && updateOptions.MaxShardReplicasUnavailable.Type == intstr.Int && updateOptions.MaxShardReplicasUnavailable.IntVal <= int32(0) {
		maxShardReplicasUnavailableCache = totalShardReplicas
	} else {
		maxShardReplicasUnavailableCache = make(map[string]int, len(totalShardReplicas))
	}

	for _, pod := range outOfDatePods {
		isSafeToUpdate := true
		nodeName := SolrNodeName(cloud, pod.Name)
		nodeContent, isInClusterState := nodeContents[nodeName]
		var reason string
		// The overseerLeader can only be upgraded by itself
		if !isInClusterState || !nodeContent.InClusterState() {
			// All pods not in the cluster state are safe to upgrade
			isSafeToUpdate = true
			reason = "Pod not in represented in the cluster state"
		} else {
			// The overseer is a special case
			if nodeContent.overseerLeader {
				// The overseerLeader can only be upgraded by itself
				// We want to update it when it's the last out of date pods and all nodes are "live"
				// But we want to make sure it still follows the same replicasDown rules as the other nodes, so still use that logic
				// This works if there are other solr nodes not managed by this SolrCloud resource, because we just check that this is the last
				// pod managed for this SolrCloud that has not been updated.
				if len(outOfDatePods) == 1 && allManagedPodsLive {
					isSafeToUpdate = true
					reason = "Pod is overseer and all other nodes have been updated."
				} else {
					isSafeToUpdate = false
					reason = "Pod is overseer and must wait for all other pods to be updated and live."
				}
			}
			// Only check the replicaSaftey if the node starts out as isSafeToUpdate, otherwise the check is redundant
			// If the node is not live, then consider it safe to be updated.
			if isSafeToUpdate {
				if !nodeContent.live {
					reason = "Pod's Solr Node is not live, therefore it is safe to take down."
				} else {
					for shard, additionalReplicaCount := range nodeContent.totalReplicasPerShard {
						// If all of the replicas for a shard on the node are down, then this is safe to kill.
						// Currently this logic lets replicas in recovery continue recovery rather than killing them.
						if additionalReplicaCount == nodeContent.downReplicasPerShard[shard] {
							continue
						}

						notActiveReplicaCount, _ := shardReplicasNotActive[shard]

						// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes
						maxShardReplicasDown, _ := ResolveMaxShardReplicasUnavailable(updateOptions.MaxShardReplicasUnavailable, shard, totalShardReplicas, maxShardReplicasUnavailableCache)

						// We have to allow killing of Pods that have multiple replicas of a shard
						// Therefore only check the additional Replica count if some replicas of that shard are already being upgraded
						// Also we only want to check the addition of the active replicas, as the non-active replicas are already included in the check.
						if notActiveReplicaCount > 0 && notActiveReplicaCount+nodeContent.activeReplicasPerShard[shard] > maxShardReplicasDown {
							reason = fmt.Sprintf("Shard %s already has %d replicas not active, taking down %d more would put it over the maximum allowed down: %d", shard, notActiveReplicaCount, nodeContent.activeReplicasPerShard[shard], maxShardReplicasDown)
							isSafeToUpdate = false
							break
						}
					}

					if reason == "" {
						reason = "Pod's replicas are safe to take down, adhering to the minimum active replicas per shard."
					}
				}
			}
		}
		if isSafeToUpdate {
			// Only add future replicas that will be taken down, if the node is "live".
			// If the node is not "live", then the replicas on that node will have already been counted as "not active".
			if isInClusterState && nodeContent.live {
				for shard, additionalReplicaCount := range nodeContent.activeReplicasPerShard {
					shardReplicasNotActive[shard] += additionalReplicaCount
				}
			}
			logger.Info("Pod killed for update.", "pod", pod.Name, "reason", reason)
			podsToUpdate = append(podsToUpdate, pod)
			podsHaveReplicas[pod.Name] = isInClusterState && nodeContent.replicas > 0

			// Stop after the maxBatchNodeUpdate count, if one is provided.
			if maxPodsToUpdate >= 1 && len(podsToUpdate) >= maxPodsToUpdate {
				logger.Info("Pod update selection complete. Maximum number of pods able to be updated reached.", "maxPodsToUpdate", maxPodsToUpdate)
				break
			}
		} else {
			logger.Info("Pod not able to be killed for update.", "pod", pod.Name, "reason", reason)
		}
	}
	return podsToUpdate, podsHaveReplicas
}

func sortNodePodsBySafety(outOfDatePods []corev1.Pod, nodeMap map[string]*SolrNodeContents, solrCloud *solr.SolrCloud) {
	sort.SliceStable(outOfDatePods, func(i, j int) bool {
		// First sort by if the node is in the ClusterState
		nodeI, hasNodeI := nodeMap[SolrNodeName(solrCloud, outOfDatePods[i].Name)]
		if !hasNodeI {
			return true
		} else if nodeI.overseerLeader {
			return false
		}
		nodeJ, hasNodeJ := nodeMap[SolrNodeName(solrCloud, outOfDatePods[j].Name)]
		if !hasNodeJ {
			return false
		} else if nodeJ.overseerLeader {
			return true
		}

		// If the nodes have the same number of replicas, then prioritize if one node is not live.
		if nodeI.live != nodeJ.live {
			return !nodeI.live
		}

		// If both nodes are in the ClusterState and not overseerLeader, then prioritize the one with less leaders.
		if nodeI.leaders != nodeJ.leaders {
			return nodeI.leaders < nodeJ.leaders
		}

		// If the nodes have the same number of leaders, then prioritize by the number of not-down replicas.
		if nodeI.notDownReplicas != nodeJ.notDownReplicas {
			return nodeI.notDownReplicas < nodeJ.notDownReplicas
		}

		// If the nodes have the same number of not-down replicas, then prioritize by the total number of replicas.
		if nodeI.replicas != nodeJ.replicas {
			return nodeI.replicas < nodeJ.replicas
		}

		// Lastly break any ties by a comparison of the name
		return nodeI.nodeName > nodeJ.nodeName
	})
}

// ResolveMaxPodsUnavailable resolves the maximum number of pods that are allowed to be unavailable, when choosing pods to update.
func ResolveMaxPodsUnavailable(maxPodsUnavailable *intstr.IntOrString, desiredPods int) (int, error) {
	if maxPodsUnavailable != nil && maxPodsUnavailable.Type == intstr.Int && maxPodsUnavailable.IntVal <= int32(0) {
		return desiredPods, nil
	}
	podsUnavailable, err := intstr.GetScaledValueFromIntOrPercent(intstr.ValueOrDefault(maxPodsUnavailable, intstr.FromString(DefaultMaxPodsUnavailable)), desiredPods, false)
	if err != nil {
		return 1, err
	}

	if podsUnavailable == 0 {
		// podsUnavailable can never be 0, otherwise pods would never be able to be upgraded.
		podsUnavailable = 1
	}

	return podsUnavailable, nil
}

// ResolveMaxShardReplicasUnavailable resolves the maximum number of replicas that are allowed to be unavailable for a given shard, when choosing pods to update.
func ResolveMaxShardReplicasUnavailable(maxShardReplicasUnavailable *intstr.IntOrString, shard string, totalShardReplicas map[string]int, cache map[string]int) (int, error) {
	maxUnavailable, isCached := cache[shard]
	var err error
	if !isCached {
		maxUnavailable, err = intstr.GetScaledValueFromIntOrPercent(intstr.ValueOrDefault(maxShardReplicasUnavailable, intstr.FromInt(DefaultMaxShardReplicasUnavailable)), totalShardReplicas[shard], false)
		if err != nil {
			maxUnavailable = 1
		}
	} else {
		err = nil
	}

	if maxUnavailable == 0 {
		// podsUnavailable can never be 0, otherwise pods would never be able to be upgraded.
		maxUnavailable = 1
	}

	return maxUnavailable, nil
}

/*
findSolrNodeContents will take a cluster and overseerLeader response from the SolrCloud Collections API, and aggregate the information.
This aggregated info is returned as:
  - A map from Solr nodeName to SolrNodeContents, with the information from the clusterState and overseerLeader
  - A map from unique shard name (collection+shard) to the count of replicas for that shard.
  - A map from unique shard name (collection+shard) to the count of replicas that are not active for that shard.
  - If a node is not live, then all shards that live on that node will be considered "not active"
*/
func findSolrNodeContents(cluster solr_api.SolrClusterStatus, overseerLeader string, managedSolrNodeNames map[string]bool) (nodeContents map[string]*SolrNodeContents, totalShardReplicas map[string]int, shardReplicasNotActive map[string]int, allManagedPodsLive bool) {
	nodeContents = make(map[string]*SolrNodeContents, 0)
	totalShardReplicas = make(map[string]int, 0)
	shardReplicasNotActive = make(map[string]int, 0)
	// Update the info for each "live" node.
	for _, nodeName := range cluster.LiveNodes {
		contents, hasValue := nodeContents[nodeName]
		delete(managedSolrNodeNames, nodeName)
		if !hasValue {
			contents = &SolrNodeContents{
				nodeName:               nodeName,
				leaders:                0,
				replicas:               0,
				totalReplicasPerShard:  map[string]int{},
				activeReplicasPerShard: map[string]int{},
				downReplicasPerShard:   map[string]int{},
				overseerLeader:         false,
				live:                   true,
			}
		} else {
			contents.live = true
		}
		nodeContents[nodeName] = contents
	}
	// Go through the state of each collection getting the count of replicas for each collection/shard living on each node
	for collectionName, collection := range cluster.Collections {
		for shardName, shard := range collection.Shards {
			uniqueShard := collectionName + "|" + shardName
			totalShardReplicas[uniqueShard] = len(shard.Replicas)
			for _, replica := range shard.Replicas {
				contents, hasValue := nodeContents[replica.NodeName]
				if !hasValue {
					contents = &SolrNodeContents{
						nodeName:               replica.NodeName,
						leaders:                0,
						replicas:               0,
						totalReplicasPerShard:  map[string]int{},
						activeReplicasPerShard: map[string]int{},
						downReplicasPerShard:   map[string]int{},
						overseerLeader:         false,
						live:                   false,
					}
				}
				if replica.Leader {
					contents.leaders += 1
				}
				contents.replicas += 1
				contents.totalReplicasPerShard[uniqueShard] += 1

				// A replica can be considered "not active" if it's state is not "active" or the node it lives in is not "live".
				if !(replica.State == solr_api.ReplicaActive && contents.live) {
					shardReplicasNotActive[uniqueShard] += 1
				}
				if replica.State == solr_api.ReplicaActive {
					contents.activeReplicasPerShard[uniqueShard] += 1
				}
				// Keep track of how many of the replicas of this shard are in a down state (down or recovery_failed)
				if replica.State == solr_api.ReplicaDown || replica.State == solr_api.ReplicaRecoveryFailed {
					contents.downReplicasPerShard[uniqueShard] += 1
				} else {
					contents.notDownReplicas += 1
				}

				nodeContents[replica.NodeName] = contents
			}
		}
	}
	// Update the info for the overseerLeader leader.
	if overseerLeader != "" {
		contents, hasValue := nodeContents[overseerLeader]
		if !hasValue {
			contents = &SolrNodeContents{
				nodeName:               overseerLeader,
				leaders:                0,
				totalReplicasPerShard:  map[string]int{},
				activeReplicasPerShard: map[string]int{},
				downReplicasPerShard:   map[string]int{},
				overseerLeader:         true,
				live:                   false,
			}
		} else {
			contents.overseerLeader = true
		}
		nodeContents[overseerLeader] = contents
	}
	return nodeContents, totalShardReplicas, shardReplicasNotActive, len(managedSolrNodeNames) == 0
}

type SolrNodeContents struct {
	// The name of the Solr Node (or pod)
	nodeName string

	// The number of leader replicas that reside in this Solr Node
	leaders int

	// The number of replicas that reside in this Solr Node
	replicas int

	// The number of replicas that reside in this Solr Node, which are not considered "down".
	// These replicas can either be in the "active" or "recovering" state.
	notDownReplicas int

	// The number of total replicas in this Solr Node, grouped by each unique shard (collection+shard)
	// This is NOT the addition of activeReplicasPerShard and downReplicasPerShard, neither of those take into account recovering replicas.
	totalReplicasPerShard map[string]int

	// The number of active replicas in this Solr Node, grouped by each unique shard (collection+shard)
	activeReplicasPerShard map[string]int

	// The number of down (or recovery_failed) replicas in this Solr Node, grouped by each unique shard (collection+shard)
	downReplicasPerShard map[string]int

	// Whether this SolrNode is the overseer leader in the SolrCloud
	overseerLeader bool

	// Whether this SolrNode is registered as a live node in the SolrCloud (Alive and connected to ZK)
	live bool
}

func (nodeContents *SolrNodeContents) InClusterState() bool {
	return nodeContents.overseerLeader || nodeContents.replicas > 0
}

// SolrNodeName takes a cloud and a pod and returns the Solr nodeName for that pod
func SolrNodeName(solrCloud *solr.SolrCloud, podName string) string {
	return fmt.Sprintf("%s:%d_solr", solrCloud.AdvertisedNodeHost(podName), solrCloud.NodePort())
}

func GetAllManagedSolrNodeNames(solrCloud *solr.SolrCloud) map[string]bool {
	podNames := solrCloud.GetAllSolrPodNames()
	allNodeNames := make(map[string]bool, len(podNames))
	for _, podName := range podNames {
		allNodeNames[SolrNodeName(solrCloud, podName)] = true
	}
	return allNodeNames
}

// EvictReplicasForPodIfNecessary takes a solr Pod and migrates all replicas off of that Pod, if the Pod is using ephemeral storage.
// If the pod is using persistent storage, this function is a no-op.
// This function MUST be idempotent and return the same list of pods given the same kubernetes/solr state.
func EvictReplicasForPodIfNecessary(ctx context.Context, solrCloud *solr.SolrCloud, pod *corev1.Pod, logger logr.Logger) (err error, canDeletePod bool) {
	var solrDataVolume *corev1.Volume
	dataVolumeName := solrCloud.DataVolumeName()
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == dataVolumeName {
			solrDataVolume = &volume
			break
		}
	}

	// Only evict if the Data volume is not persistent
	if solrDataVolume != nil && solrDataVolume.VolumeSource.PersistentVolumeClaim == nil {
		// If the Cloud has 1 or zero pods, and this is the "-0" pod, then delete the data since we can't move it anywhere else
		// Otherwise, move the replicas to other pods
		if (solrCloud.Spec.Replicas == nil || *solrCloud.Spec.Replicas < 2) && strings.HasSuffix(pod.Name, "-0") {
			queryParams := url.Values{}
			queryParams.Add("action", "DELETENODE")
			queryParams.Add("node", SolrNodeName(solrCloud, pod.Name))
			// TODO: Figure out a way to do this, since DeleteNode will not delete the last replica of every type...
			canDeletePod = true
		} else {
			requestId := "move-replicas-" + pod.Name

			// First check to see if the Async Replace request has started
			if asyncState, message, asyncErr := solr_api.CheckAsyncRequest(ctx, solrCloud, requestId); asyncErr != nil {
				err = asyncErr
			} else if asyncState == "notfound" {
				// Submit new Replace Node request
				replaceResponse := &solr_api.SolrAsyncResponse{}
				queryParams := url.Values{}
				queryParams.Add("action", "REPLACENODE")
				queryParams.Add("parallel", "true")
				queryParams.Add("sourceNode", SolrNodeName(solrCloud, pod.Name))
				queryParams.Add("async", requestId)
				err = solr_api.CallCollectionsApi(ctx, solrCloud, queryParams, replaceResponse)
				if hasError, apiErr := solr_api.CheckForCollectionsApiError("REPLACENODE", replaceResponse.ResponseHeader); hasError {
					err = apiErr
				}
				if err == nil {
					logger.Info("Migrating all replicas off of pod before deletion.", "requestId", requestId, "pod", pod.Name)
				} else {
					logger.Error(err, "Could not migrate all replicas off of pod before deletion. Will try again later.", "requestId", requestId, "message", message)
				}
			} else {
				logger.Info("Found async status", "requestId", requestId, "state", asyncState)
				// Only continue to delete the pod if the ReplaceNode request is complete and successful
				if asyncState == "completed" {
					canDeletePod = true
					logger.Info("Migration of all replicas off of pod before deletion complete. Pod can now be deleted.", "pod", pod.Name)
				} else if asyncState == "failed" {
					logger.Info("Migration of all replicas off of pod before deletion failed. Will try again.", "pod", pod.Name, "message", message)
				}

				// Delete the async request Id if the async request is successful or failed.
				// If the request failed, this will cause a retry since the next reconcile won't find the async requestId in Solr.
				if asyncState == "completed" || asyncState == "failed" {
					if message, err = solr_api.DeleteAsyncRequest(ctx, solrCloud, requestId); err != nil {
						logger.Error(err, "Could not delete Async request status.", "requestId", requestId, "message", message)
					} else {
						canDeletePod = false
					}
				}
			}
		}
	} else {
		// The pod can be deleted, since it is using persistent data storage
		canDeletePod = true
	}
	return err, canDeletePod
}
