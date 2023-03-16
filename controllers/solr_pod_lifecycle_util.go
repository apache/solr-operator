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

func DeletePodForUpdate(ctx context.Context, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud, pod *corev1.Pod, podHasReplicas bool, logger logr.Logger) (requeueAfterDuration time.Duration, err error) {
	// Before doing anything to the pod, make sure that users cannot send requests to the pod anymore.
	if updatedPod, e := EnsurePodStoppedReadinessCondition(ctx, r, pod, PodUpdate, logger); e != nil {
		err = e
		return
	} else {
		pod = updatedPod
	}

	// If the pod needs to be drained of replicas (i.e. upgrading a pod with ephemeral storage), do that before deleting the pod
	deletePod := false
	if podHasReplicas {
		// Only evict pods that contain replicas in the clusterState
		if evictError, canDeletePod := util.EvictReplicasForPodIfNecessary(ctx, instance, pod, logger); evictError != nil {
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

// PodConditionChangeReason describes the reason why a Pod is being stopped.
type PodConditionChangeReason string

const (
	PodStarted           PodConditionChangeReason = "PodStarted"
	PodUpdate            PodConditionChangeReason = "PodUpdate"
	StatefulSetScaleDown PodConditionChangeReason = "StatefulSetScaleDown"
)

func EnsurePodStoppedReadinessCondition(ctx context.Context, r *SolrCloudReconciler, pod *corev1.Pod, reason PodConditionChangeReason, logger logr.Logger) (updatedPod *corev1.Pod, err error) {
	updatedPod = pod

	readinessConditionNeedsChange := true
	readinessConditionIndex := -1
	for i, condition := range pod.Status.Conditions {
		if condition.Type == util.SolrIsNotStoppedReadinessCondition {
			readinessConditionNeedsChange = condition.Status == corev1.ConditionTrue
			readinessConditionIndex = i
			break
		}
	}

	// The pod status does not contain the readiness condition.
	// This is likely during an upgrade from a previous solr-operator version.
	if readinessConditionIndex < 0 {
		return
	}

	if readinessConditionNeedsChange {
		patchedPod := pod.DeepCopy()

		patchTime := metav1.Now()
		patchedPod.Status.Conditions[readinessConditionIndex].Status = corev1.ConditionFalse
		patchedPod.Status.Conditions[readinessConditionIndex].LastTransitionTime = patchTime
		patchedPod.Status.Conditions[readinessConditionIndex].LastProbeTime = patchTime
		patchedPod.Status.Conditions[readinessConditionIndex].Reason = string(reason)
		patchedPod.Status.Conditions[readinessConditionIndex].Message = "Pod is being deleted, traffic to the pod must be stopped"

		if err = r.Status().Patch(ctx, patchedPod, client.MergeFrom(pod)); err != nil {
			logger.Error(err, "Could not patch readiness condition for pod to stop traffic", "pod", pod.Name)
		} else {
			updatedPod = patchedPod
		}

		// TODO: Create event for the CRD.
	}

	return
}

type initialPodReadinessCondition struct {
	reason  PodConditionChangeReason
	message string
	status  bool
}

var (
	initialSolrPodReadinessConditions = map[corev1.PodConditionType]initialPodReadinessCondition{
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

		// The pod status does not contain the readiness condition, so add it
		if conditionIndex < 0 {
			pod.Status.Conditions = append(pod.Status.Conditions, initializedCondition)
		} else {
			pod.Status.Conditions[conditionIndex] = initializedCondition
		}
	}

	return
}
