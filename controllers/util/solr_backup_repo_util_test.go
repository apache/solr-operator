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
	"testing"
)

func TestGCSRepoXML(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "gcsrepository1",
		GCS: &solr.GcsRepository{
			Bucket: "some-bucket-name1",
			GcsCredentialSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
				Key:                  "some-secret-key",
			},
		},
	}
	assert.EqualValuesf(t, `
<repository name="gcsrepository1" class="org.apache.solr.gcs.GCSBackupRepository">
    <str name="gcsBucket">some-bucket-name1</str>
    <str name="gcsCredentialPath">/var/solr/data/backup-restore/gcsrepository1/gcscredential/service-account-key.json</str>
</repository>`, RepoXML(repo), "Wrong SolrXML entry for the GCS Repo")

	// Test with a base Location
	repo.GCS.BaseLocation = "/this/directory"
	assert.EqualValuesf(t, `
<repository name="gcsrepository1" class="org.apache.solr.gcs.GCSBackupRepository">
    <str name="gcsBucket">some-bucket-name1</str>
    <str name="gcsCredentialPath">/var/solr/data/backup-restore/gcsrepository1/gcscredential/service-account-key.json</str>
</repository>`, RepoXML(repo), "Wrong SolrXML entry for the GCS Repo with a base location set")
}

func TestGCSRepoAdditionalLibs(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "gcsrepository1",
		GCS: &solr.GcsRepository{
			Bucket: "some-bucket-name1",
			GcsCredentialSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
				Key:                  "some-secret-key",
			},
		},
	}
	assert.Empty(t, AdditionalRepoLibs(repo), "GCS Repos require no additional libraries for Solr")
}

func TestGCSRepoSolrModules(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "gcsrepository1",
		GCS: &solr.GcsRepository{
			Bucket: "some-bucket-name1",
			GcsCredentialSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
				Key:                  "some-secret-key",
			},
		},
	}
	assert.EqualValues(t, []string{"gcs-repository"}, RepoSolrModules(repo), "GCS Repos require the gcs-repository solr module")
}

func TestS3RepoXML(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "repo1",
		S3: &solr.S3Repository{
			Bucket: "some-bucket-name1",
			Region: "ap-northeast-2",
		},
	}
	assert.EqualValuesf(t, `
<repository name="repo1" class="org.apache.solr.s3.S3BackupRepository">
    <str name="s3.bucket.name">some-bucket-name1</str>
    <str name="s3.region">ap-northeast-2</str>
    
</repository>`, RepoXML(repo), "Wrong SolrXML entry for the S3 Repo")

	// Test with an endpoint
	repo.S3.Endpoint = "http://other.s3.location:3242"
	assert.EqualValuesf(t, `
<repository name="repo1" class="org.apache.solr.s3.S3BackupRepository">
    <str name="s3.bucket.name">some-bucket-name1</str>
    <str name="s3.region">ap-northeast-2</str>
    <str name="s3.endpoint">http://other.s3.location:3242</str>
</repository>`, RepoXML(repo), "Wrong SolrXML entry for the S3 Repo with an endpoint set")

	// Test with a proxy url and endpoint
	repo.S3.Endpoint = "http://other.s3.location:3242"
	repo.S3.ProxyUrl = "https://proxy.url:3242"
	assert.EqualValuesf(t, `
<repository name="repo1" class="org.apache.solr.s3.S3BackupRepository">
    <str name="s3.bucket.name">some-bucket-name1</str>
    <str name="s3.region">ap-northeast-2</str>
    <str name="s3.endpoint">http://other.s3.location:3242</str>
    <str name="s3.proxy.url">https://proxy.url:3242</str>
</repository>`, RepoXML(repo), "Wrong SolrXML entry for the S3 Repo with an endpoint and proxy url set")
}

func TestS3RepoAdditionalLibs(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "repo1",
		S3: &solr.S3Repository{
			Bucket: "some-bucket-name1",
			Region: "us-west-2",
		},
	}
	assert.Empty(t, AdditionalRepoLibs(repo), "S3 Repos require no additional libraries for Solr")
}

func TestS3RepoSolrModules(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "repo1",
		S3: &solr.S3Repository{
			Bucket: "some-bucket-name1",
			Region: "us-west-2",
		},
	}
	assert.EqualValues(t, []string{"s3-repository"}, RepoSolrModules(repo), "S3 Repos require the s3-repository solr module")
}

func TestVolumeRepoXML(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "volumerepository2",
		Volume: &solr.VolumeRepository{
			Source: corev1.VolumeSource{},
		},
	}
	assert.EqualValuesf(t, "<repository name=\"volumerepository2\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>", RepoXML(repo), "Wrong SolrXML entry for the Volume Repo")
}

func TestVolumeRepoAdditionalLibs(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "volumerepository2",
		Volume: &solr.VolumeRepository{
			Source: corev1.VolumeSource{},
		},
	}
	assert.Empty(t, AdditionalRepoLibs(repo), "Volume Repos require no additional libraries for Solr")
}

func TestVolumeRepoSolrModules(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "volumerepository2",
		Volume: &solr.VolumeRepository{
			Source: corev1.VolumeSource{},
		},
	}
	assert.Empty(t, RepoSolrModules(repo), "Volume Repos require no solr modules")
}
