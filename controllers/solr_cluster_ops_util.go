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
	"errors"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

func determineScaleClusterOpLockIfNecessary(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, podList []corev1.Pod, logger logr.Logger) (clusterLockAcquired bool, retryLaterDuration time.Duration, err error) {
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
				return false, time.Second * 5, nil
			}

			// Managed Scale down!
			originalStatefulSet := statefulSet.DeepCopy()
			statefulSet.Annotations[util.ClusterOpsLockAnnotation] = util.ScaleDownLock
			// The scaleDown metadata is the number of nodes to scale down to.
			// We only support scaling down one pod at-a-time when using a managed scale-down.
			// If the user wishes to scale down by multiple nodes, this ClusterOp will be done once-per-node.
			statefulSet.Annotations[util.ClusterOpsMetadataAnnotation] = strconv.Itoa(configuredPods - 1)
			if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
				logger.Error(err, "Error while patching StatefulSet to start clusterOp", "clusterOp", util.ScaleDownLock, "clusterOpMetadata", configuredPods-1)
			} else {
				clusterLockAcquired = true
			}
		} else if desiredPods > configuredPods && (instance.Spec.Scaling.PopulatePodsOnScaleUp == nil || *instance.Spec.Scaling.PopulatePodsOnScaleUp) {
			if len(podList) < configuredPods {
				// There are not enough pods, the statefulSet controller has yet to create the previously desired pods.
				// Do not start the scale up until these missing pods are created.
				return false, time.Second * 5, nil
			}
			// Managed Scale up!
			originalStatefulSet := statefulSet.DeepCopy()
			statefulSet.Annotations[util.ClusterOpsLockAnnotation] = util.ScaleUpLock
			// The scaleUp metadata is the number of nodes that existed before the scaleUp.
			// This allows the scaleUp operation to know which pods will be empty after the statefulSet is scaledUp.
			statefulSet.Annotations[util.ClusterOpsMetadataAnnotation] = strconv.Itoa(configuredPods)
			// We want to set the number of replicas at the beginning of the scaleUp operation
			statefulSet.Spec.Replicas = pointer.Int32(int32(desiredPods))
			if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
				logger.Error(err, "Error while patching StatefulSet to start clusterOp", "clusterOp", util.ScaleUpLock, "clusterOpMetadata", configuredPods, "newStatefulSetSize", desiredPods)
			} else {
				clusterLockAcquired = true
			}
		} else {
			err = scaleCloudUnmanaged(ctx, r, statefulSet, desiredPods, logger)
		}
	}
	return
}

// handleManagedCloudScaleDown does the logic of a managed and "locked" cloud scale down operation.
// This will likely take many reconcile loops to complete, as it is moving replicas away from the pods that will be scaled down.
func handleManagedCloudScaleDown(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, scaleDownToRaw string, podList []corev1.Pod, logger logr.Logger) (retryLaterDuration time.Duration, err error) {
	var scaleDownTo int
	if scaleDownTo, err = strconv.Atoi(scaleDownToRaw); err != nil {
		logger.Error(err, "Could not convert statefulSet annotation to int for scale-down-to information", "annotation", util.ClusterOpsMetadataAnnotation, "value", scaleDownToRaw)
		return
		// TODO: Create event for the CRD.
	}

	if scaleDownTo >= int(*statefulSet.Spec.Replicas) {
		// This shouldn't happen, but we don't want to be stuck if it does.
		// Just remove the cluster Op, because the cluster has already been scaled down.
		err = clearClusterOp(ctx, r, statefulSet, "statefulSet already scaled-down", logger)
	}

	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	podStoppedReadinessConditions := map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  ScaleDown,
			message: "Pod is being deleted, traffic to the pod must be stopped",
			status:  false,
		},
	}

	// TODO: It would be great to support a multi-node scale down when Solr supports evicting many SolrNodes at once.
	// Only evict the last pod, even if we are trying to scale down multiple pods.
	// Scale down will happen one pod at a time.
	if replicaManagementComplete, evictErr := evictSinglePod(ctx, r, instance, scaleDownTo, podList, podStoppedReadinessConditions, logger); err != nil {
		err = evictErr
	} else if replicaManagementComplete {
		originalStatefulSet := statefulSet.DeepCopy()
		statefulSet.Spec.Replicas = pointer.Int32(int32(scaleDownTo))
		delete(statefulSet.Annotations, util.ClusterOpsLockAnnotation)
		delete(statefulSet.Annotations, util.ClusterOpsMetadataAnnotation)
		if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
			logger.Error(err, "Error while patching StatefulSet to finish the managed SolrCloud scale down clusterOp", "newStatefulSetReplicas", scaleDownTo)
		}

		// TODO: Create event for the CRD.
	} else {
		// Retry after five seconds to check if the replica management commands have been completed
		retryLaterDuration = time.Second * 5
	}
	return
}

// handleManagedCloudScaleUp does the logic of a managed and "locked" cloud scale up operation.
// This will likely take many reconcile loops to complete, as it is moving replicas to the pods that have recently been scaled up.
func handleManagedCloudScaleUp(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, scaleUpFromRaw string, logger logr.Logger) (retryLaterDuration time.Duration, err error) {
	// TODO: Think about bad pod specs, that will never come up healthy. We want to try a rolling restart in between if necessary
	if balanceComplete, balanceErr := util.BalanceReplicasForCluster(ctx, instance, statefulSet, "scaleUp", scaleUpFromRaw, logger); err != nil {
		err = balanceErr
	} else if balanceComplete {
		// Once the replica balancing is complete, finish the cluster operation by deleting the statefulSet annotations
		originalStatefulSet := statefulSet.DeepCopy()
		delete(statefulSet.Annotations, util.ClusterOpsLockAnnotation)
		delete(statefulSet.Annotations, util.ClusterOpsMetadataAnnotation)
		if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
			logger.Error(err, "Error while patching StatefulSet to finish the managed SolrCloud scale up clusterOp")
		}

		// TODO: Create event for the CRD.
	} else {
		// Retry after five seconds to check if the replica management commands have been completed
		retryLaterDuration = time.Second * 5
	}
	return
}

func determineRollingUpdateClusterOpLockIfNecessary(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, outOfDatePods util.OutOfDatePodSegmentation, logger logr.Logger) (clusterLockAcquired bool, retryLaterDuration time.Duration, err error) {
	if instance.Spec.UpdateStrategy.Method == solrv1beta1.ManagedUpdate && !outOfDatePods.IsEmpty() {
		// Managed Rolling Upgrade!
		originalStatefulSet := statefulSet.DeepCopy()
		statefulSet.Annotations[util.ClusterOpsLockAnnotation] = util.UpdateLock
		// No rolling update metadata is currently required
		statefulSet.Annotations[util.ClusterOpsMetadataAnnotation] = ""
		if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
			logger.Error(err, "Error while patching StatefulSet to start clusterOp", "clusterOp", util.UpdateLock, "clusterOpMetadata", "")
		} else {
			clusterLockAcquired = true
		}
	}
	return
}

// handleManagedCloudRollingUpdate does the logic of a managed and "locked" cloud rolling update operation.
// This will take many reconcile loops to complete, as it is deleting pods/moving replicas.
func handleManagedCloudRollingUpdate(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, outOfDatePods util.OutOfDatePodSegmentation, hasReadyPod bool, availableUpdatedPodCount int, logger logr.Logger) (retryLaterDuration time.Duration, err error) {
	// Manage the updating of out-of-spec pods, if the Managed UpdateStrategy has been specified.
	updateLogger := logger.WithName("ManagedUpdateSelector")

	// First check if all pods are up to date. If so the rolling update is complete
	if outOfDatePods.IsEmpty() {
		// Once the rolling update is complete, finish the cluster operation by deleting the statefulSet annotations
		originalStatefulSet := statefulSet.DeepCopy()
		delete(statefulSet.Annotations, util.ClusterOpsLockAnnotation)
		delete(statefulSet.Annotations, util.ClusterOpsMetadataAnnotation)
		if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
			logger.Error(err, "Error while patching StatefulSet to finish the managed SolrCloud rollingUpdate clusterOp")
		}

		// TODO: Create event for the CRD.
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
			retryLaterDurationTemp, errTemp := DeletePodForUpdate(ctx, r, instance, &pod, podsHaveReplicas[pod.Name], updateLogger)

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

// clearClusterOp simply removes any clusterOp for the given statefulSet.
// This should only be used as a "break-glass" scenario. Do not use this to finish off successful clusterOps.
func clearClusterOp(ctx context.Context, r *SolrCloudReconciler, statefulSet *appsv1.StatefulSet, reason string, logger logr.Logger) (err error) {
	logger = logger.WithValues("reason", reason, "clusterOp", statefulSet.Annotations[util.ClusterOpsLockAnnotation], "clusterOpMetadata", statefulSet.Annotations[util.ClusterOpsMetadataAnnotation])
	originalStatefulSet := statefulSet.DeepCopy()
	delete(statefulSet.Annotations, util.ClusterOpsLockAnnotation)
	delete(statefulSet.Annotations, util.ClusterOpsMetadataAnnotation)
	if err = r.Patch(ctx, statefulSet, client.StrategicMergeFrom(originalStatefulSet)); err != nil {
		logger.Error(err, "Error while patching StatefulSet to remove unneeded clusterLockOp annotation")
	} else {
		logger.Error(err, "Removed unneeded clusterLockOp annotation from statefulSet")
	}
	return
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

func evictSinglePod(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, scaleDownTo int, podList []corev1.Pod, readinessConditions map[corev1.PodConditionType]podReadinessConditionChange, logger logr.Logger) (podIsEmpty bool, err error) {
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
		return false, e
	} else {
		podHasReplicas = len(replicas) > 0
	}

	// The pod doesn't exist, we cannot empty it
	if pod == nil {
		return !podHasReplicas, errors.New("Could not find pod " + podName + " when trying to migrate replicas to scale down pod.")
	}

	if updatedPod, e := EnsurePodReadinessConditions(ctx, r, pod, readinessConditions, logger); e != nil {
		err = e
		return
	} else {
		pod = updatedPod
	}

	// Only evict from the pod if it contains replicas in the clusterState
	if e, canDeletePod := util.EvictReplicasForPodIfNecessary(ctx, instance, pod, podHasReplicas, "scaleDown", logger); e != nil {
		err = e
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
