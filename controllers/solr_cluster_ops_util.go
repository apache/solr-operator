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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

func determineScaleClusterOpLockIfNecessary(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, podList []corev1.Pod, logger logr.Logger) (clusterOpLock string, clusterOpMetadata string, retryLaterDuration time.Duration, err error) {
	desiredPods := int(*instance.Spec.Replicas)
	configuredPods := int(*statefulSet.Spec.Replicas)
	if desiredPods != configuredPods {
		scaleTo := -1
		// Start a scaling operation
		if desiredPods < configuredPods {
			// Scale down!
			// TODO: Check if replicasScaleDown is enabled, if not it's not a clusterOp, just do the patch
			if desiredPods > 0 {
				// We only support one scaling down one pod at-a-time if not scaling down to 0 pods
				scaleTo = configuredPods - 1
			} else if desiredPods == 0 {
				scaleTo = 0
			} else {
				// The cloud is not setup to use managed scale-down
				err = scaleCloudUnmanaged(ctx, r, statefulSet, desiredPods, logger)
			}
		} else if desiredPods > configuredPods {
			// Scale up!
			// TODO: replicasScaleUp is not supported, so do not make a cluserOp out of it, just do the patch
			err = scaleCloudUnmanaged(ctx, r, statefulSet, desiredPods, logger)
		}
		if scaleTo > -1 {
			clusterOpLock = util.ScaleLock
			clusterOpMetadata = strconv.Itoa(scaleTo)
		}
	}
	return
}

func handleLockedClusterOpScale(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, podList []corev1.Pod, logger logr.Logger) (retryLaterDuration time.Duration, err error) {
	if scalingToNodes, hasAnn := statefulSet.Annotations[util.ClusterOpsMetadataAnnotation]; hasAnn {
		if scalingToNodesInt, convErr := strconv.Atoi(scalingToNodes); convErr != nil {
			logger.Error(convErr, "Could not convert statefulSet annotation to int for scale-down-to information", "annotation", util.ClusterOpsMetadataAnnotation, "value", scalingToNodes)
			err = convErr
		} else {
			replicaManagementComplete := false
			if scalingToNodesInt < int(*statefulSet.Spec.Replicas) {
				// Manage scaling down the SolrCloud
				replicaManagementComplete, err = handleManagedCloudScaleDown(ctx, r, instance, statefulSet, scalingToNodesInt, podList, logger)
				// } else if scalingToNodesInt > int(*statefulSet.Spec.Replicas) {
				// TODO: Utilize the scaled-up nodes in the future, however Solr does not currently have APIs for this.
				// TODO: Think about the order of scale-up and restart when individual nodeService IPs are injected into the pods.
				// TODO: Will likely want to do a scale-up of the service first, then do the rolling restart of the cluster, then utilize the node.
			} else {
				// This shouldn't happen. The ScalingToNodesAnnotation is removed when the statefulSet size changes, through a Patch.
				// But if it does happen, we should just remove the annotation and move forward.
				patchedStatefulSet := statefulSet.DeepCopy()
				delete(patchedStatefulSet.Annotations, util.ClusterOpsLockAnnotation)
				delete(patchedStatefulSet.Annotations, util.ClusterOpsMetadataAnnotation)
				if err = r.Patch(ctx, patchedStatefulSet, client.StrategicMergeFrom(statefulSet)); err != nil {
					logger.Error(err, "Error while patching StatefulSet to remove unneeded clusterLockOp annotation for scaling to the current amount of nodes")
				} else {
					statefulSet = patchedStatefulSet
				}
			}

			// Scale down the statefulSet to represent the new number of utilizedPods, if it is lower than the current number of pods
			// Also remove the "scalingToNodes" annotation, as that acts as a lock on the cluster, so that other operations,
			// such as scale-up, pod updates and further scale-down cannot happen at the same time.
			if replicaManagementComplete {
				patchedStatefulSet := statefulSet.DeepCopy()
				patchedStatefulSet.Spec.Replicas = pointer.Int32(int32(scalingToNodesInt))
				patchedStatefulSet.Annotations[util.UtilizedNodesAnnotation] = strconv.Itoa(scalingToNodesInt)
				delete(patchedStatefulSet.Annotations, util.ClusterOpsLockAnnotation)
				delete(patchedStatefulSet.Annotations, util.ClusterOpsMetadataAnnotation)
				if err = r.Patch(ctx, patchedStatefulSet, client.StrategicMergeFrom(statefulSet)); err != nil {
					logger.Error(err, "Error while patching StatefulSet to scale down SolrCloud", "newUtilizedNodes", scalingToNodesInt)
				}

				// TODO: Create event for the CRD.
			} else {
				// Retry after five minutes to check if the replica management commands have been completed
				retryLaterDuration = time.Second * 5
			}
		}
		// If everything succeeded, the statefulSet will have an annotation updated
		// and the reconcile loop will be called again.

		return
	} else {
		err = errors.New("no clusterOpMetadata annotation is present in the statefulSet")
		logger.Error(err, "Cannot perform scaling operation when no scale-to-nodes is provided via the clusterOpMetadata")
		return time.Second * 10, err
	}
}

// handleManagedCloudScaleDown does the logic of a managed and "locked" cloud scale down operation.
// This will likely take many reconcile loops to complete, as it is moving replicas away from the nodes that will be scaled down.
func handleManagedCloudScaleDown(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, scaleDownTo int, podList []corev1.Pod, logger logr.Logger) (replicaManagementComplete bool, err error) {
	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	podStoppedReadinessConditions := map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  ScaleDown,
			message: "Pod is being deleted, traffic to the pod must be stopped",
			status:  false,
		},
	}

	if scaleDownTo == 0 {
		// Delete all collections & data, the user wants no data left if scaling the solrcloud down to 0
		// This is a much different operation to deleting the SolrCloud/StatefulSet all-together
		replicaManagementComplete, err = evictAllPods(ctx, r, instance, podList, podStoppedReadinessConditions, logger)
	} else {
		// Only evict the last pod, even if we are trying to scale down multiple pods.
		// Scale down will happen one pod at a time.
		replicaManagementComplete, err = evictSinglePod(ctx, r, instance, scaleDownTo, podList, podStoppedReadinessConditions, logger)
	}
	// TODO: It would be great to support a multi-node scale down when Solr supports evicting many SolrNodes at once.

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

	// TODO: Get whether the pod has replicas or not
	podHasReplicas := true

	// The pod doesn't exist, we cannot empty it
	if pod == nil {
		return !podHasReplicas, nil
	}

	if updatedPod, e := EnsurePodReadinessConditions(ctx, r, pod, readinessConditions, logger); e != nil {
		err = e
		return
	} else {
		pod = updatedPod
	}

	// Only evict from the pod if it contains replicas in the clusterState
	if err, podIsEmpty = util.EvictReplicasForPodIfNecessary(ctx, instance, pod, podHasReplicas, "scaleDown", logger); err != nil {
		logger.Error(err, "Error while evicting replicas on Pod, when scaling down SolrCloud", "pod", pod.Name)
	}

	return
}
