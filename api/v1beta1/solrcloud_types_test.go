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

package v1beta1

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPVCSpecBackCompat(t *testing.T) {
	storageClassName := "test Storage class"
	instance := &SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: SolrCloudSpec{
			StorageOptions: SolrDataStorageOptions{
				PersistentStorage: &SolrPersistentDataStorageOptions{
					VolumeReclaimPolicy: VolumeReclaimPolicyRetain,
					PersistentVolumeClaimTemplate: PersistentVolumeClaimTemplate{
						ObjectMeta: TemplateMeta{
							Name:   "other-data",
							Labels: map[string]string{"base": "here"},
						},
					},
				},
			},
			DataPvcSpec: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: &storageClassName,
			},
			BackupRestoreVolume: &corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		},
	}

	// Make sure that a change was detected when transferring the old PvcSpec and backupRestoreVolume
	assert.True(t, instance.WithDefaults(""), "WithDefaults() method did not return true when making backCompat change for pvcSpec")

	// Check migration of dataPvcSpec
	assert.Nil(t, instance.Spec.DataPvcSpec, "After WithDefaults() the dataPvcSpec should be nil, having been transferred to the storageOptions")
	assert.NotNil(t, instance.Spec.StorageOptions.PersistentStorage, "After WithDefaults() the storageOptions.PersistentStorage should be filled with the old dataPvcSpec")
	assert.EqualValues(t, VolumeReclaimPolicyRetain, instance.Spec.StorageOptions.PersistentStorage.VolumeReclaimPolicy, "Default volume reclaim policy should be retain when copying from dataPvcSpec")
	assert.EqualValues(t, "other-data", instance.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name, "Do not overwrite pvcTemplate metadata when copying the dataPvcSpec")
	assert.EqualValues(t, storageClassName, *instance.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.Spec.StorageClassName, "Storage class name not transferred from the old dataPvcSpec")

	// Check migration of backupRestoreVolume
	assert.Nil(t, instance.Spec.BackupRestoreVolume, "After WithDefaults() the old backupRestoreVolume should be nil, having been transferred to the storageOptions")
	assert.NotNil(t, instance.Spec.StorageOptions.BackupRestoreOptions, "After WithDefaults() the storageOptions.BackupRestoreOptions.Volume should be filled with the value from the old location")
	assert.NotNil(t, instance.Spec.StorageOptions.BackupRestoreOptions.Volume.EmptyDir, "After WithDefaults() the storageOptions.BackupRestoreOptions.Volume should be filled with the value from the old location")
	assert.Equal(t, corev1.StorageMediumMemory, instance.Spec.StorageOptions.BackupRestoreOptions.Volume.EmptyDir.Medium, "After WithDefaults() the storageOptions.BackupRestoreOptions.Volume should be filled with the value from the old location")
	assert.Equal(t, "", instance.Spec.StorageOptions.BackupRestoreOptions.Directory, "After WithDefaults() the storageOptions.BackupRestoreOptions.Directory should still be empty")
}

func TestPVCSpecBackCompatFromNoStorageOptions(t *testing.T) {
	storageClassName := "test Storage class"
	instance := &SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: SolrCloudSpec{
			DataPvcSpec: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: &storageClassName,
			},
		},
	}

	// Make sure that a change was detected when transferring the old PvcSpec and backupRestoreVolume
	assert.True(t, instance.WithDefaults(""), "WithDefaults() method did not return true when making backCompat change for pvcSpec")

	// Check migration of dataPvcSpec
	assert.Nil(t, instance.Spec.DataPvcSpec, "After WithDefaults() the dataPvcSpec should be nil, having been transferred to the storageOptions")
	assert.NotNil(t, instance.Spec.StorageOptions.PersistentStorage, "After WithDefaults() the storageOptions.PersistentStorage should be filled with the old dataPvcSpec")
	assert.EqualValues(t, VolumeReclaimPolicyRetain, instance.Spec.StorageOptions.PersistentStorage.VolumeReclaimPolicy, "Default volume reclaim policy should be retain when copying from dataPvcSpec")
	assert.EqualValues(t, "", instance.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name, "Do not overwrite pvcTemplate metadata when copying the dataPvcSpec")
	assert.EqualValues(t, storageClassName, *instance.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.Spec.StorageClassName, "Storage class name not transferred from the old dataPvcSpec")
}

func TestSolrPodPolicyBackCompat(t *testing.T) {
	resources := corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: *resource.NewMilliQuantity(5300, resource.DecimalSI),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceEphemeralStorage: resource.MustParse("5Gi"),
		},
	}
	affinity := &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					TopologyKey: "testKey",
				},
			},
			PreferredDuringSchedulingIgnoredDuringExecution: nil,
		},
	}

	instance := &SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: SolrCloudSpec{
			SolrPod: SolrPodPolicy{
				Affinity:  affinity,
				Resources: resources,
			},
		},
	}

	// Make sure that a change was detected when transferring the old SolrPod
	assert.True(t, instance.WithDefaults(""), "WithDefaults() method did not return true when making backCompat change for SolrPodPolicy")

	// Check Affinity transition
	assert.Nil(t, instance.Spec.SolrPod.Affinity, "After WithDefaults() the solrPod.affinity should be nil, having been transferred to the customSolrKubeOptions.podOptions.affinity")
	assert.NotNil(t, instance.Spec.CustomSolrKubeOptions.PodOptions.Affinity, "After WithDefaults() the customSolrKubeOptions.podOptions.affinity should not be nil, having been transferred from solrPod.affinity")
	assert.EqualValues(t, affinity, instance.Spec.CustomSolrKubeOptions.PodOptions.Affinity, "Transferred pod affinity does not match what was specified in old location")

	// Check Resources transition
	assert.Equal(t, 0, len(instance.Spec.SolrPod.Resources.Requests), "After WithDefaults() the solrPod.resources.requests should not have any entries, having been transferred to the customSolrKubeOptions.podOptions.resources.requests")
	assert.Equal(t, 0, len(instance.Spec.SolrPod.Resources.Limits), "After WithDefaults() the solrPod.resources.limits should not have any entries, having been transferred to the customSolrKubeOptions.podOptions.resources.limits")
	assert.EqualValues(t, resources, instance.Spec.CustomSolrKubeOptions.PodOptions.Resources, "Transferred pod resources do not match what was specified in old location")
}
