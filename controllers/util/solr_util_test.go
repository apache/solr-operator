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
	assert.Equal(t, "", GenerateBackupRepositoriesForSolrXml(make([]solr.SolrBackupRepository, 0)), "There should be no backup XML when no backupRepos are specified")
}

func TestGeneratedSolrXmlContainsEntryForEachRepository(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name: "managedrepository1",
			Managed: &solr.ManagedRepository{
				Volume: corev1.VolumeSource{},
			},
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
			Name: "managedrepository2",
			Managed: &solr.ManagedRepository{
				Volume: corev1.VolumeSource{},
			},
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
	xmlString := GenerateBackupRepositoriesForSolrXml(repos)

	// These assertions don't fully guarantee valid XML, but they at least make sure each repo is defined and uses the correct class.
	// If we wanted to bring in an xpath library for assertions we could be a lot more comprehensive here.
	assert.Containsf(t, xmlString, "<repository name=\"managedrepository1\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>", "Did not find '%s' in the list of backup repositories", "managedrepository1")
	assert.Containsf(t, xmlString, "<repository name=\"managedrepository2\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>", "Did not find '%s' in the list of backup repositories", "managedrepository2")
	assert.Containsf(t, xmlString, "<repository name=\"gcsrepository1\" class=\"org.apache.solr.gcs.GCSBackupRepository\">", "Did not find '%s' in the list of backup repositories", "gcsrepository1")
	assert.Containsf(t, xmlString, "<repository name=\"gcsrepository2\" class=\"org.apache.solr.gcs.GCSBackupRepository\">", "Did not find '%s' in the list of backup repositories", "gcsrepository2")

	// Since GCS repositories are defined, make sure the contrib is on the classpath
	assert.Contains(t, xmlString, "<str name=\"sharedLib\">/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>")
}
