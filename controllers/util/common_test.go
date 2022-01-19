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
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"testing"
)

func TestCopyResources(t *testing.T) {
	log := ctrl.Log

	newResources := &corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: *resource.NewQuantity(400, resource.DecimalSI),
		},
	}
	baseResources := &corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{},
	}
	assert.True(t, CopyResources(newResources, baseResources, "", log))
	assert.NotNil(t, baseResources.Limits, "Limits should still exist but be empty after the copy. 'nil' does not overwrite an empty list/map")
	assert.Len(t, baseResources.Limits, 0, "Incorrect length for resource limits")
	assert.NotNil(t, baseResources.Requests, "Requests should exist after the copy")
	assert.Len(t, baseResources.Requests, 1, "Incorrect length for resource requests")

	newResources.Requests = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: *resource.NewQuantity(400, resource.BinarySI),
	}
	assert.False(t, CopyResources(newResources, baseResources, "", log), "No change should be detected if only the format of a quantity changes")

	baseResources.Limits = nil
	newResources.Limits = map[corev1.ResourceName]resource.Quantity{}
	assert.False(t, CopyResources(newResources, baseResources, "", log), "No change should be detected if a map goes from nil to empty")

	newResources.Requests = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: *resource.NewQuantity(600, resource.BinarySI),
	}
	assert.True(t, CopyResources(newResources, baseResources, "", log), "A change should be detected if a resource quantity changes")
	assert.Len(t, baseResources.Requests, 1, "Incorrect length for resource requests")
	assert.Contains(t, baseResources.Requests, corev1.ResourceCPU, "CPU should now be in the resource requests")
	assert.Equal(t, resource.NewQuantity(600, resource.BinarySI), baseResources.Requests.Cpu(), "Incorrect value for request cpu")

	newResources.Requests = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: *resource.NewQuantity(400, resource.BinarySI),
	}
	assert.True(t, CopyResources(newResources, baseResources, "", log), "A change should be detected when completely changing fields, though staying at the same length")
	assert.Len(t, baseResources.Requests, 1, "Incorrect length for resource requests")
	assert.NotContains(t, baseResources.Requests, corev1.ResourceCPU, "CPU should no longer be in the resource requests")
	assert.Contains(t, baseResources.Requests, corev1.ResourceMemory, "Memory should now be in the resource requests")
	assert.Equal(t, resource.NewQuantity(400, resource.BinarySI), baseResources.Requests.Memory(), "Incorrect value for request memory")

	newResources.Requests = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: *resource.NewQuantity(600, resource.BinarySI),
		corev1.ResourceCPU:    *resource.NewQuantity(500, resource.BinarySI),
	}
	assert.True(t, CopyResources(newResources, baseResources, "", log), "A change should be detected when adding resources")
	assert.Len(t, baseResources.Requests, 2, "Incorrect length for resource requests")
	assert.Contains(t, baseResources.Requests, corev1.ResourceCPU, "CPU should now be in the resource requests")
	assert.Equal(t, resource.NewQuantity(500, resource.BinarySI), baseResources.Requests.Cpu(), "Incorrect value for request cpu")
	assert.Contains(t, baseResources.Requests, corev1.ResourceMemory, "Memory should still be in the resource requests")
	assert.Equal(t, resource.NewQuantity(600, resource.BinarySI), baseResources.Requests.Memory(), "Incorrect value for request memory")

	newResources.Requests = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: *resource.NewQuantity(500, resource.BinarySI),
	}
	assert.True(t, CopyResources(newResources, baseResources, "", log), "A change should be detected when removing resources")
	assert.Len(t, baseResources.Requests, 1, "Incorrect length for resource requests")
	assert.Contains(t, baseResources.Requests, corev1.ResourceCPU, "CPU should still be in the resource requests")
	assert.Equal(t, resource.NewQuantity(500, resource.BinarySI), baseResources.Requests.Cpu(), "Incorrect value for request cpu")
	assert.NotContains(t, baseResources.Requests, corev1.ResourceMemory, "Memory should no longer be in the resource requests")

	newResources = &corev1.ResourceRequirements{}
	assert.True(t, CopyResources(newResources, baseResources, "", log), "A change should be detected when removing everything")
	assert.Len(t, baseResources.Requests, 0, "Incorrect length for resource requests")
	assert.Nil(t, baseResources.Requests, "Resource requests should be nil")
}
