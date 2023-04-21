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
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

type podReadinessConditionChange struct {
	// If this is provided, the change will only be made if the condition currently uses this reason
	matchPreviousReason *PodConditionChangeReason
	reason              PodConditionChangeReason
	message             string
	status              bool
}

// PodConditionChangeReason describes the reason why a Pod is being stopped.
type PodConditionChangeReason string

const (
	PodStarted       PodConditionChangeReason = "PodStarted"
	PodUpdate        PodConditionChangeReason = "PodUpdate"
	EvictingReplicas PodConditionChangeReason = "EvictingReplicas"
	ScaleDown        PodConditionChangeReason = "ScaleDown"
)

func DeletePodForUpdate(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, pod *corev1.Pod, podHasReplicas bool, logger logr.Logger) (requeueAfterDuration time.Duration, err error) {
	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	ps := PodStarted
	podStoppedReadinessConditions := map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  PodUpdate,
			message: "Pod is being deleted, traffic to the pod must be stopped",
			status:  false,
		},
		util.SolrReplicasNotEvictedReadinessCondition: {
			// Only set this condition if the condition hasn't been changed since pod start
			// We do not want to over-write future states later down the eviction pipeline
			matchPreviousReason: &ps,
			reason:              EvictingReplicas,
			message:             "Pod is being deleted, ephemeral data must be evicted",
			status:              false,
		},
	}
	if updatedPod, e := EnsurePodReadinessConditions(ctx, r, pod, podStoppedReadinessConditions, logger); e != nil {
		err = e
		return
	} else {
		pod = updatedPod
	}

	// If the pod needs to be drained of replicas (i.e. upgrading a pod with ephemeral storage), do that before deleting the pod
	deletePod := false
	if PodConditionEquals(pod, util.SolrReplicasNotEvictedReadinessCondition, EvictingReplicas) {
		// Only evict pods that contain replicas in the clusterState
		if evictError, canDeletePod := util.EvictReplicasForPodIfNecessary(ctx, instance, pod, podHasReplicas, "podUpdate", logger); evictError != nil {
			err = evictError
			logger.Error(err, "Error while evicting replicas on pod", "pod", pod.Name)
		} else if canDeletePod {
			deletePod = true
		} else {
			// Try again in 5 seconds if we cannot delete a pod.
			requeueAfterDuration = time.Second * 5
		}
	} else {
		// If a pod has no replicas, then update it when asked to
		deletePod = true
	}

	// Delete the pod
	if deletePod {
		err = r.Delete(ctx, pod, client.Preconditions{
			UID: &pod.UID,
		})
		if err != nil {
			logger.Error(err, "Error while killing solr pod for update", "pod", pod.Name)
		}

		// TODO: Create event for the CRD.
	}

	return
}

func ScaleDownPods(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, scaleDownTo int, podList []corev1.Pod, logger logr.Logger) (requeueAfterDuration time.Duration, err error) {
	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	podStoppedReadinessConditions := map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  ScaleDown,
			message: "Pod is being deleted, traffic to the pod must be stopped",
			status:  false,
		},
	}

	utilizedPods := *statefulSet.Spec.Replicas
	if scaleDownTo == 0 {
		// Delete all collections & data, the user wants no data left if scaling the solrcloud down to 0
		// This is a much different operation to deleting the SolrCloud/StatefulSet all-together
		if podsAreEmpty, evictError := EvictAllPods(ctx, r, instance, podList, podStoppedReadinessConditions, logger); evictError != nil {
			err = evictError
		} else if podsAreEmpty {
			// There is no data, so there should be no utilized nodes
			utilizedPods = 0
		} else {
			// Try again in 5 seconds if we cannot delete a pod.
			requeueAfterDuration = time.Second * 5
		}
	} else {
		// Only evict the last pod, even if we are trying to scale down multiple pods.
		// Scale down will happen one pod at a time.
		if podIsEmpty, evictError := EvictSinglePod(ctx, r, instance, scaleDownTo, podList, podStoppedReadinessConditions, logger); evictError != nil {
			err = evictError
		} else if podIsEmpty {
			// We have only evicted data from one pod (solr node)
			utilizedPods = int32(scaleDownTo)
		} else {
			// Try again in 5 seconds if we cannot delete a pod.
			requeueAfterDuration = time.Second * 5
		}
	}
	// TODO: It would be great to support a multi-node scale down when Solr supports evicting many SolrNodes at once.

	// Scale down the statefulSet to represent the new number of utilizedPods, if it is lower than the current number of pods
	// Also remove the "scalingToNodes" annotation, as that acts as a lock on the cluster, so that other operations,
	// such as scale-up, pod updates and further scale-down cannot happen at the same time.
	if utilizedPods < *statefulSet.Spec.Replicas {
		patchedStatefulSet := statefulSet.DeepCopy()
		patchedStatefulSet.Spec.Replicas = pointer.Int32(utilizedPods)
		patchedStatefulSet.Labels[util.UtilizedNodesAnnotation] = strconv.Itoa(int(utilizedPods))
		delete(patchedStatefulSet.Labels, util.ScalingToNodesAnnotation)
		if err = r.Patch(ctx, patchedStatefulSet, client.StrategicMergeFrom(statefulSet)); err != nil {
			logger.Error(err, "Error while patching StatefulSet to scale down SolrCloud", "newUtilizedNodes", utilizedPods)
		}

		// TODO: Create event for the CRD.
	}

	return
}

func EvictAllPods(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, podList []corev1.Pod, readinessConditions map[corev1.PodConditionType]podReadinessConditionChange, logger logr.Logger) (podsAreEmpty bool, err error) {
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

func EvictSinglePod(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, scaleDownTo int, podList []corev1.Pod, readinessConditions map[corev1.PodConditionType]podReadinessConditionChange, logger logr.Logger) (podIsEmpty bool, err error) {
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

func EnsurePodReadinessConditions(ctx context.Context, r *SolrCloudReconciler, pod *corev1.Pod, ensureConditions map[corev1.PodConditionType]podReadinessConditionChange, logger logr.Logger) (updatedPod *corev1.Pod, err error) {
	updatedPod = pod.DeepCopy()

	needsUpdate := false

	for conditionType, readinessCondition := range ensureConditions {
		podHasCondition := false
		for _, gate := range pod.Spec.ReadinessGates {
			if gate.ConditionType == conditionType {
				podHasCondition = true
			}
		}
		if podHasCondition {
			needsUpdate = EnsurePodReadinessCondition(updatedPod, conditionType, readinessCondition) || needsUpdate
		}
	}

	if needsUpdate {
		if err = r.Status().Patch(ctx, updatedPod, client.MergeFrom(pod)); err != nil {
			logger.Error(err, "Could not patch readiness condition(s) for pod to stop traffic", "pod", pod.Name)
			updatedPod = pod

			// TODO: Create event for the CRD.
		}
	} else {
		updatedPod = pod
	}

	return
}

var (
	initialSolrPodReadinessConditions = map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  PodStarted,
			message: "Pod has not yet been stopped",
			status:  true,
		},
		util.SolrReplicasNotEvictedReadinessCondition: {
			reason:  PodStarted,
			message: "Replicas have not yet been evicted",
			status:  true,
		},
	}
)

// InitializePodReadinessCondition set the default value for a pod's readiness condition after pod creation.
func InitializePodReadinessCondition(pod *corev1.Pod, conditionType corev1.PodConditionType) (conditionNeedsInitializing bool) {
	if foundInitialPodReadinessCondition, found := initialSolrPodReadinessConditions[conditionType]; found {
		return InitializeCustomPodReadinessCondition(
			pod,
			conditionType,
			foundInitialPodReadinessCondition.reason,
			foundInitialPodReadinessCondition.message,
			foundInitialPodReadinessCondition.status)
	} else {
		// If there is no default given for this readinessCondition, do nothing
		return false
	}
}

// InitializeCustomPodReadinessCondition set the default value for a pod's readiness condition after pod creation, given all the default values to set
func InitializeCustomPodReadinessCondition(pod *corev1.Pod, conditionType corev1.PodConditionType, reason PodConditionChangeReason, message string, status bool) (conditionNeedsInitializing bool) {
	conditionNeedsInitializing = true
	conditionIndex := -1
	for i, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			conditionNeedsInitializing = condition.Reason == ""
			conditionIndex = i
			break
		}
	}

	if conditionNeedsInitializing {
		patchTime := metav1.Now()
		conditionStatus := corev1.ConditionFalse
		if status {
			conditionStatus = corev1.ConditionTrue
		}
		initializedCondition := corev1.PodCondition{
			Type:               conditionType,
			Status:             conditionStatus,
			Reason:             string(reason),
			Message:            message,
			LastProbeTime:      patchTime,
			LastTransitionTime: patchTime,
		}

		if conditionIndex < 0 {
			// The pod status does not contain the readiness condition, so add it
			pod.Status.Conditions = append(pod.Status.Conditions, initializedCondition)
		} else {
			pod.Status.Conditions[conditionIndex] = initializedCondition
		}
	}

	return
}

// EnsurePodReadinessCondition ensure the podCondition is set to the given values
func EnsurePodReadinessCondition(pod *corev1.Pod, conditionType corev1.PodConditionType, ensureCondition podReadinessConditionChange) (conditionNeedsChange bool) {
	conditionNeedsChange = false
	conditionIndex := -1
	for i, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			if ensureCondition.matchPreviousReason != nil {
				conditionNeedsChange = condition.Reason == string(*ensureCondition.matchPreviousReason)
			} else {
				conditionNeedsChange = condition.Reason != string(ensureCondition.reason)
			}
			if (condition.Status == corev1.ConditionTrue) != ensureCondition.status {
				conditionNeedsChange = true
			}
			conditionIndex = i
			break
		}
	}

	if conditionNeedsChange {
		patchTime := metav1.Now()
		conditionStatus := corev1.ConditionFalse
		if ensureCondition.status {
			conditionStatus = corev1.ConditionTrue
		}
		initializedCondition := corev1.PodCondition{
			Type:               conditionType,
			Status:             conditionStatus,
			Reason:             string(ensureCondition.reason),
			Message:            ensureCondition.message,
			LastProbeTime:      patchTime,
			LastTransitionTime: patchTime,
		}

		pod.Status.Conditions[conditionIndex] = initializedCondition
	}

	return
}

// PodConditionEquals check if a podCondition equals what is expected
func PodConditionEquals(pod *corev1.Pod, conditionType corev1.PodConditionType, reason PodConditionChangeReason) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return string(reason) == condition.Reason
		}
	}

	return false
}
