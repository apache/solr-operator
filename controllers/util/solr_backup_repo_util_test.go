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
			GcsCredentialSecret: corev1.SecretKeySelector{
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

func TestManagedRepoXML(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "managedrepository2",
		Managed: &solr.ManagedRepository{
			Volume: corev1.VolumeSource{},
		},
	}
	assert.EqualValuesf(t, "<repository name=\"managedrepository2\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>", RepoXML(repo), "Wrong SolrXML entry for the Managed Repo")
}

func TestGCSRepoAdditionalLibs(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "gcsrepository1",
		GCS: &solr.GcsRepository{
			Bucket: "some-bucket-name1",
			GcsCredentialSecret: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
				Key:                  "some-secret-key",
			},
		},
	}
	assert.EqualValues(t, []string{"/opt/solr/dist", "/opt/solr/contrib/gcs-repository/lib"}, AdditionalRepoLibs(repo), "GCS Repos require no additional libraries for Solr")
}

func TestManagedRepoAdditionalLibs(t *testing.T) {
	repo := &solr.SolrBackupRepository{
		Name: "managedrepository2",
		Managed: &solr.ManagedRepository{
			Volume: corev1.VolumeSource{},
		},
	}
	assert.Empty(t, AdditionalRepoLibs(repo), "Managed Repos require no additional libraries for Solr")
}
