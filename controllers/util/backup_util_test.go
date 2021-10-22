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

func TestSolrBackupApiParamsForManagedRepositoryBackup(t *testing.T) {
	managedRepository := &solr.SolrBackupRepository{
		Name: "somemanagedrepository",
		Managed: &solr.ManagedRepository{
			Volume:    corev1.VolumeSource{}, // Actual volume info doesn't matter here
			Directory: "/somedirectory",
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somemanagedrepository",
			Collections:    []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(managedRepository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/var/solr/data/backup-restore/somemanagedrepository/backups", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somemanagedrepository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestSolrBackupApiParamsForManagedRepositoryBackupWithLocation(t *testing.T) {
	managedRepository := &solr.SolrBackupRepository{
		Name: "somemanagedrepository",
		Managed: &solr.ManagedRepository{
			Volume:    corev1.VolumeSource{}, // Actual volume info doesn't matter here
			Directory: "/somedirectory",
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somemanagedrepository",
			Collections:    []string{"col1", "col2"},
			Location:       "test/location",
		},
	}

	queryParams := GenerateQueryParamsForBackup(managedRepository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/var/solr/data/backup-restore/somemanagedrepository/test/location", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somemanagedrepository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestSolrBackupApiParamsForGcsRepositoryBackup(t *testing.T) {
	gcsRepository := &solr.SolrBackupRepository{
		Name: "somegcsrepository",
		GCS: &solr.GcsRepository{
			Bucket: "some-gcs-bucket",
			GcsCredentialSecret: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name"},
				Key:                  "some-secret-key",
			},
			BaseLocation: "/some/gcs/path",
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somegcsrepository",
			Collections:    []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(gcsRepository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/some/gcs/path", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somegcsrepository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestSolrBackupApiParamsForGcsRepositoryBackupWithLocation(t *testing.T) {
	gcsRepository := &solr.SolrBackupRepository{
		Name: "somegcsrepository",
		GCS: &solr.GcsRepository{
			Bucket: "some-gcs-bucket",
			GcsCredentialSecret: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name"},
				Key:                  "some-secret-key",
			},
			BaseLocation: "/some/gcs/path",
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somegcsrepository",
			Collections:    []string{"col1", "col2"},
			Location:       "/another/gcs/path/test",
		},
	}

	queryParams := GenerateQueryParamsForBackup(gcsRepository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/another/gcs/path/test", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somegcsrepository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestSolrBackupApiParamsForGcsRepositoryBackupWithNoLocations(t *testing.T) {
	gcsRepository := &solr.SolrBackupRepository{
		Name: "somegcsrepository",
		GCS: &solr.GcsRepository{
			Bucket: "some-gcs-bucket",
			GcsCredentialSecret: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name"},
				Key:                  "some-secret-key",
			},
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somegcsrepository",
			Collections:    []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(gcsRepository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somegcsrepository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestSolrBackupApiParamsForS3RepositoryBackup(t *testing.T) {
	s3Repository := &solr.SolrBackupRepository{
		Name: "somes3repository",
		S3: &solr.S3Repository{
			Bucket: "some-s3-bucket",
			Region: "us-west-2",
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somes3repository",
			Collections:    []string{"col1", "col2"},
		},
	}

	queryParams := GenerateQueryParamsForBackup(s3Repository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somes3repository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestSolrBackupApiParamsForS3RepositoryBackupWithLocation(t *testing.T) {
	s3Repository := &solr.SolrBackupRepository{
		Name: "somes3repository",
		S3: &solr.S3Repository{
			Bucket: "some-gcs-bucket",
			Region: "us-west-2",
		},
	}
	backupConfig := solr.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "somebackupname",
		},
		Spec: solr.SolrBackupSpec{
			SolrCloud:      "solrcloudcluster",
			RepositoryName: "somes3repository",
			Collections:    []string{"col1", "col2"},
			Location:       "/another/path",
		},
	}

	queryParams := GenerateQueryParamsForBackup(s3Repository, &backupConfig, "col2")

	assert.Equalf(t, "BACKUP", queryParams.Get("action"), "Wrong %s for Collections API Call", "action")
	assert.Equalf(t, "col2", queryParams.Get("collection"), "Wrong %s for Collections API Call", "collection name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("name"), "Wrong %s for Collections API Call", "backup name")
	assert.Equalf(t, "somebackupname-col2", queryParams.Get("async"), "Wrong %s for Collections API Call", "async id")
	assert.Equalf(t, "/another/path", queryParams.Get("location"), "Wrong %s for Collections API Call", "backup location")
	assert.Equalf(t, "somes3repository", queryParams.Get("repository"), "Wrong %s for Collections API Call", "repository")
}

func TestReportsFailureWhenBackupRepositoryCannotBeFoundByName(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name:    "managedrepository1",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository1",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name1",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
					Key:                  "some-secret-key",
				},
			},
		},
	}
	found := GetBackupRepositoryByName(repos, "managedrepository2")

	assert.Nil(t, found, "Expected GetBackupRepositoryByName to report that no match was found")
}

func TestCanLookupManagedRepositoryByName(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name:    "managedrepository1",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository1",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name1",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
					Key:                  "some-secret-key",
				},
			},
		},
		{
			Name:    "managedrepository2",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository2",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name2",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name2"},
					Key:                  "some-secret-key-2",
				},
				BaseLocation: "location-2",
			},
		},
	}
	found := GetBackupRepositoryByName(repos, "managedrepository2")

	assert.NotNil(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, repos[2], *found, "Wrong repo found")
}

func TestCanLookupGcsRepositoryByName(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name:    "managedrepository1",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository1",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name1",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
					Key:                  "some-secret-key",
				},
			},
		},
		{
			Name:    "managedrepository2",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository2",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name2",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name2"},
					Key:                  "some-secret-key-2",
				},
				BaseLocation: "location-2",
			},
		},
	}
	found := GetBackupRepositoryByName(repos, "gcsrepository2")

	assert.NotNil(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, repos[3], *found, "Wrong repo found")
}

func TestCanLookupLegacyRepositoryByName(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name:    "managedrepository1",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository1",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name1",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
					Key:                  "some-secret-key",
				},
			},
		},
		{
			Name:    "managedrepository2",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name: "gcsrepository2",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name2",
				GcsCredentialSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name2"},
					Key:                  "some-secret-key-2",
				},
				BaseLocation: "location-2",
			},
		},
		{
			Name:    "legacy_local_repository",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
	}
	found := GetBackupRepositoryByName(repos, "legacy_local_repository")

	assert.NotNil(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, repos[4], *found, "Wrong repo found")
}

// If 'solrcloud' only has 1 repository configured and the backup doesn't specify a name, we still want to choose the
// only option available.
func TestRepositoryLookupSucceedsIfNoNameProvidedButOnlyOneRepositoryDefined(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name:    "managedrepository1",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
	}

	found := GetBackupRepositoryByName(repos, "")

	assert.NotNil(t, found, "Expected GetBackupRepositoryByName to report a found match")
	assert.Equal(t, repos[0], *found, "Wrong repo found")
}

func TestRepositoryLookupFailsIfNoNameProvidedAndMultipleRepositoriesDefined(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name:    "managedrepository1",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
		{
			Name:    "managedrepository2",
			Managed: &solr.ManagedRepository{Volume: corev1.VolumeSource{}},
		},
	}
	found := GetBackupRepositoryByName(repos, "")

	assert.Nil(t, found, "Expected GetBackupRepositoryByName to report no match")
}
