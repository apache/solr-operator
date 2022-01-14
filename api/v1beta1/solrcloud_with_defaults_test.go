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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestDeprecatedBackupRepo(t *testing.T) {
	volume := corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{Path: "temp"},
	}

	directory := "/another/dir"

	solrCloud := &SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec:       SolrCloudSpec{},
	}

	// Set defaults for SolrCloud
	solrCloud.WithDefaults()

	//No deprecated repository to move, no existing repos
	assert.Emptyf(t, solrCloud.Spec.BackupRepositories, "No backup repositories should exist when defaulting a SolrCloud without any")
	assert.Nilf(t, solrCloud.Spec.StorageOptions.BackupRestoreOptions, "No deprecated BackupRestoreOptions should exist when defaulting a SolrCloud that didn't start with one")

	var solrCloudTest *SolrCloud
	backupRepos := []SolrBackupRepository{
		{
			Name: "volumerepository1",
			Volume: &VolumeRepository{
				Source: corev1.VolumeSource{},
			},
		},
		{
			Name: "gcsrepository1",
			GCS: &GcsRepository{
				Bucket: "some-bucket-name1",
				GcsCredentialSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
					Key:                  "some-secret-key",
				},
			},
		},
	}

	//No deprecated repository to move, 2 existing repos
	solrCloudTest = solrCloud.DeepCopy()
	solrCloudTest.Spec.BackupRepositories = backupRepos
	solrCloudTest = solrCloudTest.DeepCopy()
	assert.False(t, solrCloudTest.WithDefaults(), "WithDefaults() returned true when nothing should have been changed (no legacy repo)")
	assert.ElementsMatch(t, backupRepos, solrCloudTest.Spec.BackupRepositories, "Backup repos modified when they should be exactly the same")

	//Has deprecated repository to move, no existing repos
	solrCloudTest = solrCloud.DeepCopy()
	solrCloudTest.Spec.StorageOptions.BackupRestoreOptions = &SolrBackupRestoreOptions{
		Volume:    volume,
		Directory: directory,
	}
	assert.True(t, solrCloudTest.WithDefaults(), "WithDefaults() returned false when the provided legacy repo should have been moved to the new location")
	assert.Nil(t, solrCloudTest.Spec.StorageOptions.BackupRestoreOptions, "Legacy BackupRestore location should be nil after defaulting")
	assert.Len(t, solrCloudTest.Spec.BackupRepositories, 1, "Backup repos should have 1 entry, the legacy backup repo")
	assertLegacyBackupRepo(t, solrCloudTest.Spec.BackupRepositories[0], volume, directory)

	//Has deprecated repository to move, 2 existing repos
	solrCloudTest = solrCloud.DeepCopy()
	solrCloudTest.Spec.StorageOptions.BackupRestoreOptions = &SolrBackupRestoreOptions{
		Volume:    volume,
		Directory: directory,
	}
	solrCloudTest.Spec.BackupRepositories = backupRepos
	solrCloudTest = solrCloudTest.DeepCopy()
	assert.True(t, solrCloudTest.WithDefaults(), "WithDefaults() returned false when the provided legacy repo should have been moved to the new location")
	assert.Nil(t, solrCloudTest.Spec.StorageOptions.BackupRestoreOptions, "Legacy BackupRestore location should be nil after defaulting")
	assert.Len(t, solrCloudTest.Spec.BackupRepositories, 3, "Backup repos should have 3 entries, the legacy backup repo and the 2 provided by the user")
	assert.EqualValues(t, backupRepos, solrCloudTest.Spec.BackupRepositories[0:2], "The pre-existing repos should not have been modified during the defaulting")
	assertLegacyBackupRepo(t, solrCloudTest.Spec.BackupRepositories[2], volume, directory)
}

func assertLegacyBackupRepo(t *testing.T, repository SolrBackupRepository, volume corev1.VolumeSource, dir string) {
	assert.Equal(t, LegacyBackupRepositoryName, repository.Name, "Wrong name for the legacy backup repo")
	assert.Nil(t, repository.GCS, "Legacy backup repo should not have GCS specs")
	assert.NotNil(t, repository.Volume, "Legacy backup repo must have Volume specs")
	assert.EqualValuesf(t, volume, repository.Volume.Source, "Volume Source incorrectly copied over for legacy backup repo")
	assert.Equal(t, dir, repository.Volume.Directory, "Directory incorrectly copied over for legacy backup repo")
}
