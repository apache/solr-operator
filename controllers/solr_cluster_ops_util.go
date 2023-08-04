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

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

// SolrClusterOp contains metadata for cluster operations performed on SolrClouds.
type SolrClusterOp struct {
	// The type of Cluster Operation
	Operation SolrClusterOperationType `json:"operation"`

	// Time that the Cluster Operation was started or re-started
	LastStartTime metav1.Time `json:"lastStartTime"`

	// Time that the Cluster Operation was started or re-started
	Metadata string `json:"metadata"`
}
type SolrClusterOperationType string

const (
	ScaleDownLock SolrClusterOperationType = "ScalingDown"
	ScaleUpLock   SolrClusterOperationType = "ScalingUp"
	UpdateLock    SolrClusterOperationType = "RollingUpdate"
)

func clearClusterOpLock(statefulSet *appsv1.StatefulSet) {
	delete(statefulSet.Annotations, util.ClusterOpsLockAnnotation)
}

func setClusterOpLock(statefulSet *appsv1.StatefulSet, op SolrClusterOp) error {
	bytes, err := json.Marshal(op)
	if err != nil {
		return err
	}
	statefulSet.Annotations[util.ClusterOpsLockAnnotation] = string(bytes)
	return nil
}

func setClusterOpRetryQueue(statefulSet *appsv1.StatefulSet, queue []SolrClusterOp) error {
	if len(queue) > 0 {
		bytes, err := json.Marshal(queue)
		if err != nil {
			return err
		}
		statefulSet.Annotations[util.ClusterOpsRetryQueueAnnotation] = string(bytes)
	} else {
		delete(statefulSet.Annotations, util.ClusterOpsRetryQueueAnnotation)
	}
	return nil
}

func GetCurrentClusterOp(statefulSet *appsv1.StatefulSet) (clusterOp *SolrClusterOp, err error) {
	if op, hasOp := statefulSet.Annotations[util.ClusterOpsLockAnnotation]; hasOp {
		clusterOp = &SolrClusterOp{}
		err = json.Unmarshal([]byte(op), clusterOp)
	}
	return
}

func GetClusterOpRetryQueue(statefulSet *appsv1.StatefulSet) (clusterOpQueue []SolrClusterOp, err error) {
	if op, hasOp := statefulSet.Annotations[util.ClusterOpsRetryQueueAnnotation]; hasOp {
		err = json.Unmarshal([]byte(op), &clusterOpQueue)
	}
	return
}

func enqueueCurrentClusterOpForRetry(statefulSet *appsv1.StatefulSet) (hasOp bool, err error) {
	clusterOp, err := GetCurrentClusterOp(statefulSet)
	if err != nil || clusterOp == nil {
		return false, err
	}
	clusterOpRetryQueue, err := GetClusterOpRetryQueue(statefulSet)
	if err != nil {
		return true, err
	}
	clusterOpRetryQueue = append(clusterOpRetryQueue, *clusterOp)
	clearClusterOpLock(statefulSet)
	return true, setClusterOpRetryQueue(statefulSet, clusterOpRetryQueue)
}

func retryNextQueuedClusterOp(statefulSet *appsv1.StatefulSet) (hasOp bool, err error) {
	clusterOpRetryQueue, err := GetClusterOpRetryQueue(statefulSet)
	if err != nil {
		return hasOp, err
	}
	hasOp = len(clusterOpRetryQueue) > 0
	if len(clusterOpRetryQueue) > 0 {
		nextOp := clusterOpRetryQueue[0]
		nextOp.LastStartTime = metav1.Now()
		err = setClusterOpLock(statefulSet, nextOp)
		if err != nil {
			return hasOp, err
		}
		err = setClusterOpRetryQueue(statefulSet, clusterOpRetryQueue[1:])
	}
	return hasOp, err
}

func retryNextQueuedClusterOpWithQueue(statefulSet *appsv1.StatefulSet, clusterOpQueue []SolrClusterOp) (hasOp bool, err error) {
	if err != nil {
		return hasOp, err
	}
	hasOp = len(clusterOpQueue) > 0
	if len(clusterOpQueue) > 0 {
		nextOp := clusterOpQueue[0]
		nextOp.LastStartTime = metav1.Now()
		err = setClusterOpLock(statefulSet, nextOp)
		if err != nil {
			return hasOp, err
		}
		err = setClusterOpRetryQueue(statefulSet, clusterOpQueue[1:])
	}
	return hasOp, err
}

func determineScaleClusterOpLockIfNecessary(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, scaleDownOpIsQueued bool, podList []corev1.Pod, logger logr.Logger) (clusterOp *SolrClusterOp, retryLaterDuration time.Duration, err error) {
	desiredPods := int(*instance.Spec.Replicas)
	configuredPods := int(*statefulSet.Spec.Replicas)
	if desiredPods != configuredPods {
		// We do not do a "managed" scale-to-zero operation.
		// Only do a managed scale down if the desiredPods is positive.
		// The VacatePodsOnScaleDown option is enabled by default, so treat "nil" like "true"
		if desiredPods < configuredPods && desiredPods > 0 &&
			(instance.Spec.Scaling.VacatePodsOnScaleDown == nil || *instance.Spec.Scaling.VacatePodsOnScaleDown) {
			if len(podList) > configuredPods {
				// There are too many pods, the statefulSet controller has yet to delete unwanted pods.
				// Do not start the scale down until these extra pods are deleted.
				return nil, time.Second * 5, nil
			}
			clusterOp = &SolrClusterOp{
				Operation: ScaleDownLock,
				Metadata:  strconv.Itoa(configuredPods - 1),
			}
		} else if desiredPods > configuredPods && (instance.Spec.Scaling.PopulatePodsOnScaleUp == nil || *instance.Spec.Scaling.PopulatePodsOnScaleUp) {
			if len(podList) < configuredPods {
				// There are not enough pods, the statefulSet controller has yet to create the previously desired pods.
				// Do not start the scale up until these missing pods are created.
				return nil, time.Second * 5, nil
			}
			clusterOp = &SolrClusterOp{
				Operation: ScaleUpLock,
				Metadata:  strconv.Itoa(desiredPods),
			}
		} else {
			err = scaleCloudUnmanaged(ctx, r, statefulSet, desiredPods, logger)
		}
	} else if scaleDownOpIsQueued {
		// If the statefulSet and the solrCloud have the same number of pods configured, and the queued operation is a scaleDown,
		// that means the scaleDown was reverted. So there's no reason to change the number of pods.
		// However, a Replica Balancing should be done just in case, so do a ScaleUp, but don't change the number of pods.
		clusterOp = &SolrClusterOp{
			Operation: ScaleUpLock,
			Metadata:  strconv.Itoa(desiredPods),
		}
	}
	return
}

// handleManagedCloudScaleDown does the logic of a managed and "locked" cloud scale down operation.
// This will likely take many reconcile loops to complete, as it is moving replicas away from the pods that will be scaled down.
func handleManagedCloudScaleDown(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, clusterOp *SolrClusterOp, podList []corev1.Pod, logger logr.Logger) (operationComplete bool, requestInProgress bool, retryLaterDuration time.Duration, err error) {
	var scaleDownTo int
	if scaleDownTo, err = strconv.Atoi(clusterOp.Metadata); err != nil {
		logger.Error(err, "Could not convert ScaleDown metadata to int, as it represents the number of nodes to scale to", "metadata", clusterOp.Metadata)
		return
		// TODO: Create event for the CRD.
	}

	if len(podList) <= scaleDownTo {
		// The number of pods is less than we are trying to scaleDown to, so we are done
		return true, false, 0, nil
	}
	if int(*statefulSet.Spec.Replicas) <= scaleDownTo {
		// We've done everything we need to do at this point. We just need to wait until the pods are deleted to be "done".
		// So return and wait for the next reconcile loop, whenever it happens
		return false, false, time.Second, nil
	}
	// TODO: It would be great to support a multi-node scale down when Solr supports evicting many SolrNodes at once.
	if int(*statefulSet.Spec.Replicas) > scaleDownTo+1 {
		// This shouldn't happen, but we don't want to be stuck if it does.
		// Just remove the cluster Op, because the cluster is bigger than it should be.
		// We will retry the whole thing again, with the right metadata this time
		operationComplete = true
		return true, false, time.Second, nil
	}

	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	podStoppedReadinessConditions := map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  ScaleDown,
			message: "Pod is being deleted, traffic to the pod must be stopped",
			status:  false,
		},
	}

	// Only evict the last pod, even if we are trying to scale down multiple pods.
	// Scale down will happen one pod at a time.
	var replicaManagementComplete bool
	if replicaManagementComplete, requestInProgress, err = evictSinglePod(ctx, r, instance, scaleDownTo, podList, podStoppedReadinessConditions, logger); err == nil {
		if replicaManagementComplete {
			originalStatefulSet := statefulSet.DeepCopy()
			statefulSet.Spec.Replicas = pointer.Int32(int32(scaleDownTo))
			if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
				logger.Error(err, "Error while patching StatefulSet to scale down pods after eviction", "newStatefulSetReplicas", scaleDownTo)
			}
			// Return and wait for the pods to be created, which will call another reconcile
			retryLaterDuration = 0
		} else {
			// Retry after five seconds to check if the replica management commands have been completed
			retryLaterDuration = time.Second * 5
		}
	}
	return
}

// handleManagedCloudScaleUp does the logic of a managed and "locked" cloud scale up operation.
// This will likely take many reconcile loops to complete, as it is moving replicas to the pods that have recently been scaled up.
func handleManagedCloudScaleUp(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, clusterOp *SolrClusterOp, podList []corev1.Pod, logger logr.Logger) (operationComplete bool, requestInProgress bool, retryLaterDuration time.Duration, err error) {
	desiredPods, err := strconv.Atoi(clusterOp.Metadata)
	if err != nil {
		logger.Error(err, "Could not convert ScaleUp metadata to int, as it represents the number of nodes to scale to", "metadata", clusterOp.Metadata)
		return
	}
	configuredPods := int(*statefulSet.Spec.Replicas)
	if configuredPods < desiredPods {
		// The first thing to do is increase the number of pods the statefulSet is running
		originalStatefulSet := statefulSet.DeepCopy()
		statefulSet.Spec.Replicas = pointer.Int32(int32(desiredPods))

		err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet))
		if err != nil {
			logger.Error(err, "Error while patching StatefulSet to increase the number of pods for the ScaleUp")
		}
		// Return and wait for the pods to be created, which will call another reconcile
		return false, false, 0, err
	} else {
		// Before doing anything to the pod, make sure that the pods do not have a stopped readiness condition
		readinessConditions := map[corev1.PodConditionType]podReadinessConditionChange{
			util.SolrIsNotStoppedReadinessCondition: {
				reason:  PodStarted,
				message: "Pod is not being deleted, traffic to the pod must be started",
				status:  true,
			},
		}
		for _, pod := range podList {
			if updatedPod, e := EnsurePodReadinessConditions(ctx, r, &pod, readinessConditions, logger); e != nil {
				err = e
				return
			} else {
				pod = *updatedPod
			}
		}
		if operationComplete, requestInProgress, err = util.BalanceReplicasForCluster(ctx, instance, statefulSet, "scaleUp", clusterOp.Metadata, logger); !operationComplete && err == nil {
			// Retry after five seconds to check if the replica management commands have been completed
			retryLaterDuration = time.Second * 5
		}
	}
	return
}

func determineRollingUpdateClusterOpLockIfNecessary(instance *solrv1beta1.SolrCloud, outOfDatePods util.OutOfDatePodSegmentation) (clusterOp *SolrClusterOp, retryLaterDuration time.Duration, err error) {
	if instance.Spec.UpdateStrategy.Method == solrv1beta1.ManagedUpdate && !outOfDatePods.IsEmpty() {
		clusterOp = &SolrClusterOp{
			Operation: UpdateLock,
		}
	}
	return
}

// handleManagedCloudRollingUpdate does the logic of a managed and "locked" cloud rolling update operation.
// This will take many reconcile loops to complete, as it is deleting pods/moving replicas.
func handleManagedCloudRollingUpdate(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, outOfDatePods util.OutOfDatePodSegmentation, hasReadyPod bool, availableUpdatedPodCount int, logger logr.Logger) (operationComplete bool, requestInProgress bool, retryLaterDuration time.Duration, err error) {
	// Manage the updating of out-of-spec pods, if the Managed UpdateStrategy has been specified.
	updateLogger := logger.WithName("ManagedUpdateSelector")

	// First check if all pods are up to date and ready. If so the rolling update is complete
	configuredPods := int(*statefulSet.Spec.Replicas)
	if configuredPods == availableUpdatedPodCount {
		// The configured number of pods are all healthy and up to date. The operation is complete
		operationComplete = true
		return
	} else if outOfDatePods.IsEmpty() {
		// Just return and wait for the updated pods to come up healthy, these will call new reconciles, so there is nothing for us to do
		return
	} else {
		// The out of date pods that have not been started, should all be updated immediately.
		// There is no use "safely" updating pods which have not been started yet.
		podsToUpdate := append([]corev1.Pod{}, outOfDatePods.NotStarted...)
		for _, pod := range outOfDatePods.NotStarted {
			updateLogger.Info("Pod killed for update.", "pod", pod.Name, "reason", "The solr container in the pod has not yet started, thus it is safe to update.")
		}

		// Pick which pods should be deleted for an update.
		// Don't exit on an error, which would only occur because of an HTTP Exception. Requeue later instead.
		additionalPodsToUpdate, podsHaveReplicas, retryLater, clusterStateError :=
			util.DeterminePodsSafeToUpdate(ctx, instance, int(*statefulSet.Spec.Replicas), outOfDatePods, hasReadyPod, availableUpdatedPodCount, updateLogger)
		// If we do not have the clusterState, it's not safe to update pods that are running
		if clusterStateError != nil {
			retryLater = true
		} else {
			podsToUpdate = append(podsToUpdate, outOfDatePods.ScheduledForDeletion...)
			podsToUpdate = append(podsToUpdate, additionalPodsToUpdate...)
		}

		// Only actually delete a running pod if it has been evicted, or doesn't need eviction (persistent storage)
		for _, pod := range podsToUpdate {
			retryLaterDurationTemp, inProgTmp, errTemp := DeletePodForUpdate(ctx, r, instance, &pod, podsHaveReplicas[pod.Name], updateLogger)
			requestInProgress = requestInProgress || inProgTmp

			// Use the retryLaterDuration of the pod that requires a retry the soonest (smallest duration > 0)
			if retryLaterDurationTemp > 0 && (retryLaterDurationTemp < retryLaterDuration || retryLaterDuration == 0) {
				retryLaterDuration = retryLaterDurationTemp
			}
			if errTemp != nil {
				err = errTemp
			}
		}
		if retryLater && retryLaterDuration == 0 {
			retryLaterDuration = time.Second * 10
		}
	}
	return
}

// clearClusterOpLockWithPatch simply removes any clusterOp for the given statefulSet.
func clearClusterOpLockWithPatch(ctx context.Context, r *SolrCloudReconciler, statefulSet *appsv1.StatefulSet, reason string, logger logr.Logger) (err error) {
	originalStatefulSet := statefulSet.DeepCopy()
	clearClusterOpLock(statefulSet)
	if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
		logger.Error(err, "Error while patching StatefulSet to remove unneeded clusterOpLock annotation", "reason", reason)
	} else {
		logger.Info("Removed unneeded clusterOpLock annotation from statefulSet", "reason", reason)
	}
	return
}

// enqueueCurrentClusterOpForRetryWithPatch adds the current clusterOp to the clusterOpRetryQueue, and clears the current cluster Op.
// This method will send the StatefulSet patch to the API Server.
func enqueueCurrentClusterOpForRetryWithPatch(ctx context.Context, r *SolrCloudReconciler, statefulSet *appsv1.StatefulSet, reason string, logger logr.Logger) (err error) {
	originalStatefulSet := statefulSet.DeepCopy()
	hasOp, err := enqueueCurrentClusterOpForRetry(statefulSet)
	if hasOp && err == nil {
		err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet))
	}
	if err != nil {
		logger.Error(err, "Error while patching StatefulSet to enqueue clusterOp for retry", "reason", reason)
	} else if hasOp {
		logger.Info("Enqueued current clusterOp to continue later", "reason", reason)
	}
	return err
}

// retryNextQueuedClusterOpWithPatch removes the first clusterOp from the clusterOpRetryQueue, and sets it as the current cluster Op.
// This method will send the StatefulSet patch to the API Server.
func retryNextQueuedClusterOpWithPatch(ctx context.Context, r *SolrCloudReconciler, statefulSet *appsv1.StatefulSet, clusterOpQueue []SolrClusterOp, logger logr.Logger) (err error) {
	originalStatefulSet := statefulSet.DeepCopy()
	hasOp, err := retryNextQueuedClusterOpWithQueue(statefulSet, clusterOpQueue)
	if hasOp && err == nil {
		err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet))
	}
	if err != nil {
		logger.Error(err, "Error while patching StatefulSet to retry next queued clusterOp")
	} else if hasOp {
		logger.Info("Retrying next queued clusterOp")
	}
	return err
}

// scaleCloudUnmanaged does simple scaling of a SolrCloud without moving replicas.
// This is not a "locked" cluster operation, and does not block other cluster operations from taking place.
func scaleCloudUnmanaged(ctx context.Context, r *SolrCloudReconciler, statefulSet *appsv1.StatefulSet, scaleTo int, logger logr.Logger) (err error) {
	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	patchedStatefulSet := statefulSet.DeepCopy()
	patchedStatefulSet.Spec.Replicas = pointer.Int32(int32(scaleTo))
	if err = r.Patch(ctx, patchedStatefulSet, client.StrategicMergeFrom(statefulSet)); err != nil {
		logger.Error(err, "Error while patching StatefulSet to scale SolrCloud.", "fromNodes", *statefulSet.Spec.Replicas, "toNodes", scaleTo)
	}
	return err
}

// This is currently not used, use in the future if we want to delete all data when scaling down to zero
func evictAllPods(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, podList []corev1.Pod, readinessConditions map[corev1.PodConditionType]podReadinessConditionChange, logger logr.Logger) (podsAreEmpty bool, err error) {
	// If there are no pods, we can't empty them. Just return true
	if len(podList) == 0 {
		return true, nil
	}

	for i, pod := range podList {
		if updatedPod, e := EnsurePodReadinessConditions(ctx, r, &pod, readinessConditions, logger); e != nil {
			err = e
			return
		} else {
			podList[i] = *updatedPod
		}
	}

	// Delete all collections & data, the user wants no data left if scaling the solrcloud down to 0
	// This is a much different operation to deleting the SolrCloud/StatefulSet all-together
	// TODO: Implement delete all collections. Currently just leave the data
	//if err, podsAreEmpty = util.DeleteAllCollectionsIfNecessary(ctx, instance, "scaleDown", logger); err != nil {
	//	logger.Error(err, "Error while evicting all collections in SolrCloud, when scaling down SolrCloud to 0 pods")
	//}
	podsAreEmpty = true

	return
}

func evictSinglePod(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, scaleDownTo int, podList []corev1.Pod, readinessConditions map[corev1.PodConditionType]podReadinessConditionChange, logger logr.Logger) (podIsEmpty bool, requestInProgress bool, err error) {
	var pod *corev1.Pod
	podName := instance.GetSolrPodName(scaleDownTo)
	for _, p := range podList {
		if p.Name == podName {
			pod = &p
			break
		}
	}

	podHasReplicas := true
	if replicas, e := getReplicasForPod(ctx, instance, podName, logger); e != nil {
		return false, false, e
	} else {
		podHasReplicas = len(replicas) > 0
	}

	// The pod doesn't exist, we cannot empty it
	if pod == nil {
		return !podHasReplicas, false, errors.New("Could not find pod " + podName + " when trying to migrate replicas to scale down pod.")
	}

	if updatedPod, e := EnsurePodReadinessConditions(ctx, r, pod, readinessConditions, logger); e != nil {
		err = e
		return
	} else {
		pod = updatedPod
	}

	// Only evict from the pod if it contains replicas in the clusterState
	var canDeletePod bool
	if err, canDeletePod, requestInProgress = util.EvictReplicasForPodIfNecessary(ctx, instance, pod, podHasReplicas, "scaleDown", logger); err != nil {
		logger.Error(err, "Error while evicting replicas on Pod, when scaling down SolrCloud", "pod", pod.Name)
	} else if canDeletePod {
		// The pod previously had replicas, so loop back in the next reconcile to make sure that the pod doesn't
		// have replicas anymore even if the previous evict command was successful.
		// If there are still replicas, it will start the eviction process again
		podIsEmpty = !podHasReplicas
	}

	return
}

func getReplicasForPod(ctx context.Context, cloud *solrv1beta1.SolrCloud, podName string, logger logr.Logger) (replicas []string, err error) {
	clusterResp := &solr_api.SolrClusterStatusResponse{}
	queryParams := url.Values{}
	queryParams.Add("action", "CLUSTERSTATUS")
	err = solr_api.CallCollectionsApi(ctx, cloud, queryParams, clusterResp)
	if _, apiError := solr_api.CheckForCollectionsApiError("CLUSTERSTATUS", clusterResp.ResponseHeader, clusterResp.Error); apiError != nil {
		err = apiError
	}
	podNodeName := util.SolrNodeName(cloud, podName)
	if err == nil {
		for _, colState := range clusterResp.ClusterStatus.Collections {
			for _, shardState := range colState.Shards {
				for replica, replicaState := range shardState.Replicas {
					if replicaState.NodeName == podNodeName {
						replicas = append(replicas, replica)
					}
				}
			}
		}
	} else {
		logger.Error(err, "Error retrieving cluster status, cannot determine if pod has replicas")
	}
	return
}
