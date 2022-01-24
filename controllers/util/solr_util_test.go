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
	xmlString, modules, libs := GenerateBackupRepositoriesForSolrXml(make([]solr.SolrBackupRepository, 0))
	assert.Equal(t, "", xmlString, "There should be no backup XML when no backupRepos are specified")
	assert.Empty(t, modules, "There should be no modules for the backupRepos when no backupRepos are specified")
	assert.Empty(t, libs, "There should be no libs for the backupRepos when no backupRepos are specified")
}

func TestGeneratedSolrXmlContainsEntryForEachRepository(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name: "volumerepository1",
			Volume: &solr.VolumeRepository{
				Source: corev1.VolumeSource{},
			},
		},
		{
			Name: "gcsrepository1",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name1",
				GcsCredentialSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name1"},
					Key:                  "some-secret-key",
				},
			},
		},
		{
			Name: "s3repository1",
			S3: &solr.S3Repository{
				Bucket: "some-bucket-name1",
				Region: "us-west-2",
			},
		},
		{
			Name: "volumerepository2",
			Volume: &solr.VolumeRepository{
				Source: corev1.VolumeSource{},
			},
		},
		{
			Name: "gcsrepository2",
			GCS: &solr.GcsRepository{
				Bucket: "some-bucket-name2",
				GcsCredentialSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "some-secret-name2"},
					Key:                  "some-secret-key-2",
				},
				BaseLocation: "location-2",
			},
		},
		{
			Name: "s3repository2",
			S3: &solr.S3Repository{
				Bucket: "some-bucket-name2",
				Region: "ap-northeast-2",
			},
		},
	}
	xmlString, modules, libs := GenerateBackupRepositoriesForSolrXml(repos)

	// These assertions don't fully guarantee valid XML, but they at least make sure each repo is defined and uses the correct class.
	// If we wanted to bring in an xpath library for assertions we could be a lot more comprehensive here.
	assert.Containsf(t, xmlString, "<repository name=\"volumerepository1\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>", "Did not find '%s' in the list of backup repositories", "volumerepository1")
	assert.Containsf(t, xmlString, "<repository name=\"volumerepository2\" class=\"org.apache.solr.core.backup.repository.LocalFileSystemRepository\"/>", "Did not find '%s' in the list of backup repositories", "volumerepository2")
	assert.Containsf(t, xmlString, "<repository name=\"gcsrepository1\" class=\"org.apache.solr.gcs.GCSBackupRepository\">", "Did not find '%s' in the list of backup repositories", "gcsrepository1")
	assert.Containsf(t, xmlString, "<repository name=\"gcsrepository2\" class=\"org.apache.solr.gcs.GCSBackupRepository\">", "Did not find '%s' in the list of backup repositories", "gcsrepository2")
	assert.Containsf(t, xmlString, "<repository name=\"s3repository1\" class=\"org.apache.solr.s3.S3BackupRepository\">", "Did not find '%s' in the list of backup repositories", "s3repository1")
	assert.Containsf(t, xmlString, "<repository name=\"s3repository2\" class=\"org.apache.solr.s3.S3BackupRepository\">", "Did not find '%s' in the list of backup repositories", "s3repository2")

	assert.Contains(t, modules, "gcs-repository", "The modules for the backupRepos should contain gcs-repository")
	assert.Contains(t, modules, "s3-repository", "The modules for the backupRepos should contain s3-repository")
	assert.Empty(t, libs, "There should be no libs for the backupRepos")
}

func TestGeneratedGcsRepositoryXmlSkipsCredentialIfUnset(t *testing.T) {
	repos := []solr.SolrBackupRepository{
		{
			Name: "gcsrepository1",
			GCS: &solr.GcsRepository{
				Bucket:              "some-bucket-name1",
				GcsCredentialSecret: nil,
			},
		},
	}
	xmlString, _, _ := GenerateBackupRepositoriesForSolrXml(repos)

	// These assertions don't fully guarantee valid XML, but they at least make sure each repo is defined and uses the correct class.
	// If we wanted to bring in an xpath library for assertions we could be a lot more comprehensive here.
	assert.Containsf(t, xmlString, "<repository name=\"gcsrepository1\" class=\"org.apache.solr.gcs.GCSBackupRepository\">", "Did not find '%s' in the list of backup repositories", "gcsrepository1")
	assert.NotContainsf(t, xmlString, "<str name=\"gcsCredentialPath\">", "Found unexpected gcsCredentialPath param in GCS repository configuration")
}

func TestGenerateAdditionalLibXMLPart(t *testing.T) {
	// Just 1 repeated solr module
	xmlString := GenerateAdditionalLibXMLPart([]string{"gcs-repository", "gcs-repository"}, []string{})
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for just 1 repeated solr module")

	// Just 2 different solr modules
	xmlString = GenerateAdditionalLibXMLPart([]string{"gcs-repository", "analytics"}, []string{})
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for just 2 different solr modules")

	// Just 2 repeated libs
	xmlString = GenerateAdditionalLibXMLPart([]string{}, []string{"/ext/lib", "/ext/lib"})
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">/ext/lib</str>", "Wrong sharedLib xml for just 1 repeated additional lib")

	// Just 2 different libs
	xmlString = GenerateAdditionalLibXMLPart([]string{}, []string{"/ext/lib2", "/ext/lib1"})
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">/ext/lib1,/ext/lib2</str>", "Wrong sharedLib xml for just 2 different additional libs")

	// Combination of everything
	xmlString = GenerateAdditionalLibXMLPart([]string{"gcs-repository", "analytics", "analytics"}, []string{"/ext/lib2", "/ext/lib2", "/ext/lib1"})
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">/ext/lib1,/ext/lib2,/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for mix of additional libs and solr modules")
}

func TestGenerateSolrXMLStringForCloud(t *testing.T) {
	// All 3 options that factor into the sharedLib
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			BackupRepositories: []solr.SolrBackupRepository{
				{
					Name: "test",
					GCS:  &solr.GcsRepository{},
				},
			},
			AdditionalLibs: []string{"/ext/lib2", "/ext/lib1"},
			SolrModules:    []string{"ltr", "analytics"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/ext/lib1,/ext/lib2,/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with a backupRepo, additionalLibs and solrModules")

	// Just SolrModules and AdditionalLibs
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			AdditionalLibs: []string{"/ext/lib2", "/ext/lib1"},
			SolrModules:    []string{"ltr", "analytics"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/ext/lib1,/ext/lib2,/opt/solr/contrib/analytics/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with additionalLibs and solrModules")

	// Just SolrModules and Backups
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			BackupRepositories: []solr.SolrBackupRepository{
				{
					Name: "test",
					GCS:  &solr.GcsRepository{},
				},
			},
			SolrModules: []string{"ltr", "analytics"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with a backupRepo and solrModules")

	// Just AdditionalLibs and Backups
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			BackupRepositories: []solr.SolrBackupRepository{
				{
					Name: "test",
					GCS:  &solr.GcsRepository{},
				},
			},
			AdditionalLibs: []string{"/ext/lib2", "/ext/lib1"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/ext/lib1,/ext/lib2,/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with a backupRepo and additionalLibs")

	// Just SolrModules
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrModules: []string{"ltr", "analytics"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/opt/solr/contrib/analytics/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with just solrModules")

	// Just Backups
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			BackupRepositories: []solr.SolrBackupRepository{
				{
					Name: "test",
					GCS:  &solr.GcsRepository{},
				},
			},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with just a backupRepo")

	// Just AdditionalLibs
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			AdditionalLibs: []string{"/ext/lib2", "/ext/lib1"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">/ext/lib1,/ext/lib2</str>", "Wrong sharedLib xml for a cloud with a just additionalLibs")
}
