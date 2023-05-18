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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// PodConditionEquals check if a podCondition equals what is expected
func PodConditionHasStatus(pod *corev1.Pod, conditionType corev1.PodConditionType, status corev1.ConditionStatus) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return status == condition.Status
		}
	}

	return false
}
