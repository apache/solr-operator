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
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// pvcWithCondition builds a PVC carrying a single resize condition.
func pvcWithCondition(condType corev1.PersistentVolumeClaimConditionType, status corev1.ConditionStatus) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{{Type: condType, Status: status}},
		},
	}
}

// pvcWithAllocatedStatus builds a PVC carrying a storage allocatedResourceStatus.
func pvcWithAllocatedStatus(status corev1.ClaimResourceStatus) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		Status: corev1.PersistentVolumeClaimStatus{
			AllocatedResourceStatuses: map[corev1.ResourceName]corev1.ClaimResourceStatus{
				corev1.ResourceStorage: status,
			},
		},
	}
}

// TestPvcControllerExpansionComplete verifies that the controller-side expansion is reported as
// complete for the "offline" provisioner signals (FileSystemResizePending condition or a pending/
// in-progress node resize status), so that the rolling restart is not gated on status.capacity.
func TestPvcControllerExpansionComplete(t *testing.T) {
	cases := []struct {
		name string
		pvc  *corev1.PersistentVolumeClaim
		want bool
	}{
		{"empty pvc", &corev1.PersistentVolumeClaim{}, false},
		{"filesystem resize pending (offline ready-to-restart)", pvcWithCondition(corev1.PersistentVolumeClaimFileSystemResizePending, corev1.ConditionTrue), true},
		{"filesystem resize pending but condition false", pvcWithCondition(corev1.PersistentVolumeClaimFileSystemResizePending, corev1.ConditionFalse), false},
		{"unrelated resizing condition", pvcWithCondition(corev1.PersistentVolumeClaimResizing, corev1.ConditionTrue), false},
		{"node resize pending status", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimNodeResizePending), true},
		{"node resize in progress status", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimNodeResizeInProgress), true},
		{"controller resize in progress status", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimControllerResizeInProgress), false},
		{"controller resize infeasible status", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimControllerResizeInfeasible), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pvcControllerExpansionComplete(tc.pvc); got != tc.want {
				t.Errorf("pvcControllerExpansionComplete() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvcResizeInfeasible verifies that a backend-declared infeasible expansion is detected from the
// allocatedResourceStatuses (best-effort; populated on Kubernetes >= 1.34).
func TestPvcResizeInfeasible(t *testing.T) {
	cases := []struct {
		name string
		pvc  *corev1.PersistentVolumeClaim
		want bool
	}{
		{"empty pvc", &corev1.PersistentVolumeClaim{}, false},
		{"controller resize infeasible", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimControllerResizeInfeasible), true},
		{"node resize infeasible", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimNodeResizeInfeasible), true},
		{"node resize pending is not infeasible", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimNodeResizePending), false},
		{"controller resize in progress is not infeasible", pvcWithAllocatedStatus(corev1.PersistentVolumeClaimControllerResizeInProgress), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pvcResizeInfeasible(tc.pvc); got != tc.want {
				t.Errorf("pvcResizeInfeasible() = %v, want %v", got, tc.want)
			}
		})
	}
}
