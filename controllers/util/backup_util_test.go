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
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestSolrBackupApiParamsForLegacyBackup(t *testing.T) {
	legacyRepository := solr.SolrBackupRestoreOptions{
		Volume: &corev1.VolumeSource{}, // Actual volume info doesn't matter here
		Directory: "/somedirectory",
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud: "solrcloudcluster",
			RepositoryName: "legacy_local_repository",
			Collections: []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(&legacyRepository, &backupConfig, "col2")

	assert.Equal(t, "BACKUP", queryParams.Get("action"))
	assert.Equal(t, "col2", queryParams.Get("collection"))
	assert.Equal(t, "col2", queryParams.Get("name"))
	assert.Equal(t, "somebackupname-col2", queryParams.Get("async"))
	assert.Equal(t, "/var/solr/data/backup-restore/backups/somebackupname", queryParams.Get("location"))
	assert.Equal(t, "legacy_local_repository", queryParams.Get("repository"))
}

func TestSolrBackupApiParamsForManagedRepositoryBackup(t *testing.T) {
	managedRepository := solr.ManagedStorage{
		Name: "somemanagedrepository",
		Volume: &corev1.VolumeSource{}, // Actual volume info doesn't matter here
		Directory: "/somedirectory",
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud: "solrcloudcluster",
			RepositoryName: "somemanagedrepository",
			Collections: []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(&managedRepository, &backupConfig, "col2")

	assert.Equal(t, "BACKUP", queryParams.Get("action"))
	assert.Equal(t, "col2", queryParams.Get("collection"))
	assert.Equal(t, "col2", queryParams.Get("name"))
	assert.Equal(t, "somebackupname-col2", queryParams.Get("async"))
	assert.Equal(t, "/var/solr/data/backup-restore-managed-somemanagedrepository/backups/somebackupname", queryParams.Get("location"))
	assert.Equal(t, "somemanagedrepository", queryParams.Get("repository"))
}

func TestSolrBackupApiParamsForGcsRepositoryBackup(t *testing.T) {
	gcsRepository := solr.GcsStorage{
		Name: "somegcsrepository",
		Bucket: "some-gcs-bucket",
		GcsCredentialSecret: "some-secret-name",
		BaseLocation: "/some/gcs/path",
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud: "solrcloudcluster",
			RepositoryName: "somegcsrepository",
			Collections: []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(&gcsRepository, &backupConfig, "col2")

	assert.Equal(t, "BACKUP", queryParams.Get("action"))
	assert.Equal(t, "col2", queryParams.Get("collection"))
	assert.Equal(t, "col2", queryParams.Get("name"))
	assert.Equal(t, "somebackupname-col2", queryParams.Get("async"))
	assert.Equal(t, "/some/gcs/path", queryParams.Get("location"))
	assert.Equal(t, "somegcsrepository", queryParams.Get("repository"))
}

func TestReportsFailureWhenBackupRepositoryCannotBeFoundByName(t *testing.T) {
	backupOptions := solr.SolrBackupRestoreOptions{
		Volume: &corev1.VolumeSource{}, // Define a 'legacy'-style backup repository
		Directory: "/somedirectory",
		ManagedRepositories: &[]solr.ManagedStorage{},
		GcsRepositories: &[]solr.GcsStorage{},
	}

	_, found := GetBackupRepositoryByName(&backupOptions, "managedrepository2")

	assert.False(t, found, "Expected GetBackupRepositoryByName to report that no match was found")
}

func TestCanLookupManagedRepositoryByName(t *testing.T) {
	managedRepository1 := solr.ManagedStorage{Name: "managedrepository1", Volume: &corev1.VolumeSource{},}
	managedRepository2 := solr.ManagedStorage{Name: "managedrepository2", Volume: &corev1.VolumeSource{},}
	gcsRepository1 := solr.GcsStorage{Name: "gcsrepository1", Bucket: "some-bucket-name1", GcsCredentialSecret: "some-secret-name1",}
	gcsRepository2 := solr.GcsStorage{Name: "gcsrepository2", Bucket: "some-bucket-name2", GcsCredentialSecret: "some-secret-name2",}
	backupOptions := solr.SolrBackupRestoreOptions{
		Volume: &corev1.VolumeSource{}, // Define a 'legacy'-style backup repository
		Directory: "/somedirectory",
		ManagedRepositories: &[]solr.ManagedStorage{managedRepository1, managedRepository2},
		GcsRepositories: &[]solr.GcsStorage{gcsRepository1, gcsRepository2},
	}

	foundRepo, found := GetBackupRepositoryByName(&backupOptions, "managedrepository2")

	assert.True(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, &managedRepository2, foundRepo)
}

func TestCanLookupGcsRepositoryByName(t *testing.T) {
	managedRepository1 := solr.ManagedStorage{Name: "managedrepository1", Volume: &corev1.VolumeSource{},}
	managedRepository2 := solr.ManagedStorage{Name: "managedrepository2", Volume: &corev1.VolumeSource{},}
	gcsRepository1 := solr.GcsStorage{Name: "gcsrepository1", Bucket: "some-bucket-name1", GcsCredentialSecret: "some-secret-name1",}
	gcsRepository2 := solr.GcsStorage{Name: "gcsrepository2", Bucket: "some-bucket-name2", GcsCredentialSecret: "some-secret-name2",}
	backupOptions := solr.SolrBackupRestoreOptions{
		Volume: &corev1.VolumeSource{}, // Define a 'legacy'-style backup repository
		Directory: "/somedirectory",
		ManagedRepositories: &[]solr.ManagedStorage{managedRepository1, managedRepository2},
		GcsRepositories: &[]solr.GcsStorage{gcsRepository1, gcsRepository2},
	}

	foundRepo, found := GetBackupRepositoryByName(&backupOptions, "gcsrepository2")

	assert.True(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, &gcsRepository2, foundRepo)
}

func TestCanLookupLegacyRepositoryByName(t *testing.T) {
	managedRepository1 := solr.ManagedStorage{Name: "managedrepository1", Volume: &corev1.VolumeSource{},}
	managedRepository2 := solr.ManagedStorage{Name: "managedrepository2", Volume: &corev1.VolumeSource{},}
	gcsRepository1 := solr.GcsStorage{Name: "gcsrepository1", Bucket: "some-bucket-name1", GcsCredentialSecret: "some-secret-name1",}
	gcsRepository2 := solr.GcsStorage{Name: "gcsrepository2", Bucket: "some-bucket-name2", GcsCredentialSecret: "some-secret-name2",}
	backupOptions := solr.SolrBackupRestoreOptions{
		Volume: &corev1.VolumeSource{}, // Define a 'legacy'-style backup repository
		Directory: "/somedirectory",
		ManagedRepositories: &[]solr.ManagedStorage{managedRepository1, managedRepository2},
		GcsRepositories: &[]solr.GcsStorage{gcsRepository1, gcsRepository2},
	}

	foundRepo, found := GetBackupRepositoryByName(&backupOptions, "legacy_local_repository")

	assert.True(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, &backupOptions, foundRepo)
}

// If 'solrcloud' only has 1 repository configured and the backup doesn't specify a name, we still want to choose the
// only option available.
func TestRepositoryLookupSucceedsIfNoNameProvidedButOnlyOneRepositoryDefined(t *testing.T) {
	managedRepository1 := solr.ManagedStorage{Name: "managedrepository1", Volume: &corev1.VolumeSource{},}
	backupOptions := solr.SolrBackupRestoreOptions{
		ManagedRepositories: &[]solr.ManagedStorage{managedRepository1,},
		GcsRepositories: &[]solr.GcsStorage{},
	}

	foundRepo, found := GetBackupRepositoryByName(&backupOptions, "")

	assert.True(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, &managedRepository1, foundRepo)
}

func TestRepositoryLookupFailsIfNoNameProvidedAndMultipleRepositoriesDefined(t *testing.T) {
	managedRepository1 := solr.ManagedStorage{Name: "managedrepository1", Volume: &corev1.VolumeSource{},}
	managedRepository2 := solr.ManagedStorage{Name: "managedrepository2", Volume: &corev1.VolumeSource{},}
	backupOptions := solr.SolrBackupRestoreOptions{
		ManagedRepositories: &[]solr.ManagedStorage{managedRepository1, managedRepository2},
		GcsRepositories: &[]solr.GcsStorage{},
	}

	_, found := GetBackupRepositoryByName(&backupOptions, "")

	assert.False(t, found, "Expected GetBackupRepositoryByName to report no match")
}