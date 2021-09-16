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

func TestNoRepositoryXmlGeneratedWhenNoRepositoriesExist(t *testing.T) {
	backupOptions := solr.SolrBackupRestoreOptions{}

	xmlString := GenerateBackupRepositoriesForSolrXml(&backupOptions)

	assert.Equal(t, "", xmlString)
}

func TestGeneratedSolrXmlContainsEntryForEachRepository(t *testing.T) {
	managedRepository1 := solr.ManagedStorage{Name: "managedrepository1", Volume: &corev1.VolumeSource{}}
	managedRepository2 := solr.ManagedStorage{Name: "managedrepository2", Volume: &corev1.VolumeSource{}}
	gcsRepository1 := solr.GcsStorage{Name: "gcsrepository1", Bucket: "some-bucket-name1", GcsCredentialSecret: "some-secret-name1"}
	gcsRepository2 := solr.GcsStorage{Name: "gcsrepository2", Bucket: "some-bucket-name2", GcsCredentialSecret: "some-secret-name2"}
	backupOptions := solr.SolrBackupRestoreOptions{
		Volume:              &corev1.VolumeSource{}, // Define a 'legacy'-style backup repository
		Directory:           "/somedirectory",
		ManagedRepositories: &[]solr.ManagedStorage{managedRepository1, managedRepository2},
		GcsRepositories:     &[]solr.GcsStorage{gcsRepository1, gcsRepository2},
	}

	xmlString := GenerateBackupRepositoriesForSolrXml(&backupOptions)

	// These assertions don't fully guarantee valid XML, but they at least make sure each repo is defined and uses the correct class.
	// If we wanted to bring in an xpath library for assertions we could be a lot more comprehensive here.
	assert.Contains(t, xmlString, "<repository name=\"legacy_local_repository\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>")
	assert.Contains(t, xmlString, "<repository name=\"managedrepository1\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>")
	assert.Contains(t, xmlString, "<repository name=\"managedrepository2\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>")
	assert.Contains(t, xmlString, "<repository name=\"gcsrepository1\" class=\"org.apache.solr.gcs.GCSBackupRepository\">")
	assert.Contains(t, xmlString, "<repository name=\"gcsrepository2\" class=\"org.apache.solr.gcs.GCSBackupRepository\">")

	// Since GCS repositories are defined, make sure the contrib is on the classpath
	assert.Contains(t, xmlString, "<str name=\"sharedLib\">/opt/solr/dist,/opt/solr/contrib/gcs-repository/lib</str>")
}
