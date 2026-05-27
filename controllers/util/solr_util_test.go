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
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strings"
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
	// No specified libs
	xmlString := GenerateAdditionalLibXMLPart([]string{}, []string{}, false)
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">${solr.sharedLib:}</str>", "Wrong sharedLib xml for no specified libs")

	// Just 1 repeated solr module
	xmlString = GenerateAdditionalLibXMLPart([]string{"gcs-repository", "gcs-repository"}, []string{}, false)
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">${solr.sharedLib:},/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for just 1 repeated solr module")

	// Just 2 different solr modules
	xmlString = GenerateAdditionalLibXMLPart([]string{"gcs-repository", "analytics"}, []string{}, false)
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">${solr.sharedLib:},/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for just 2 different solr modules")

	// Just 2 repeated libs
	xmlString = GenerateAdditionalLibXMLPart([]string{}, []string{"/ext/lib", "/ext/lib"}, false)
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib</str>", "Wrong sharedLib xml for just 1 repeated additional lib")

	// Just 2 different libs
	xmlString = GenerateAdditionalLibXMLPart([]string{}, []string{"/ext/lib2", "/ext/lib1"}, false)
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2</str>", "Wrong sharedLib xml for just 2 different additional libs")

	// Combination of everything
	xmlString = GenerateAdditionalLibXMLPart([]string{"gcs-repository", "analytics", "analytics"}, []string{"/ext/lib2", "/ext/lib2", "/ext/lib1"}, false)
	assert.EqualValuesf(t, xmlString, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2,/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for mix of additional libs and solr modules")
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
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2,/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with a backupRepo, additionalLibs and solrModules")

	// Just SolrModules and AdditionalLibs
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			AdditionalLibs: []string{"/ext/lib2", "/ext/lib1"},
			SolrModules:    []string{"ltr", "analytics"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2,/opt/solr/contrib/analytics/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with additionalLibs and solrModules")

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
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/opt/solr/contrib/analytics/lib,/opt/solr/contrib/gcs-repository/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with a backupRepo and solrModules")

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
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2,/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with a backupRepo and additionalLibs")

	// Just SolrModules
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrModules: []string{"ltr", "analytics"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/opt/solr/contrib/analytics/lib,/opt/solr/contrib/ltr/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with just solrModules")

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
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/opt/solr/contrib/gcs-repository/lib,/opt/solr/dist</str>", "Wrong sharedLib xml for a cloud with just a backupRepo")

	// Just AdditionalLibs
	solrCloud = &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			AdditionalLibs: []string{"/ext/lib2", "/ext/lib1"},
		},
	}
	assert.Containsf(t, GenerateSolrXMLStringForCloud(solrCloud), "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2</str>", "Wrong sharedLib xml for a cloud with a just additionalLibs")
}

func TestIsSolr10OrLater(t *testing.T) {
	assert.False(t, (&solr.SolrCloud{}).IsSolr10OrLater(), "zero value → pre-10")
	assert.False(t, (&solr.SolrCloud{Spec: solr.SolrCloudSpec{SolrMajorVersion: 8}}).IsSolr10OrLater(), "8 → pre-10")
	assert.False(t, (&solr.SolrCloud{Spec: solr.SolrCloudSpec{SolrMajorVersion: 9}}).IsSolr10OrLater(), "9 → pre-10")
	assert.True(t, (&solr.SolrCloud{Spec: solr.SolrCloudSpec{SolrMajorVersion: 10}}).IsSolr10OrLater(), "10 → Solr 10+")

	// Custom image tag doesn't affect the outcome
	cloud := &solr.SolrCloud{Spec: solr.SolrCloudSpec{SolrMajorVersion: 8, SolrImage: &solr.ContainerImage{Tag: "261.162.1"}}}
	assert.False(t, cloud.IsSolr10OrLater(), "solrMajorVersion=8 with custom tag '261.162.1'")
}

func TestGenerateAdditionalLibXMLPartSolr10(t *testing.T) {
	// Solr 10: modules should NOT generate contrib paths (contrib dir doesn't exist)
	xmlString := GenerateAdditionalLibXMLPart([]string{"gcs-repository", "analytics"}, []string{}, true)
	assert.EqualValuesf(t, "<str name=\"sharedLib\">${solr.sharedLib:}</str>", xmlString, "Solr 10 should not include contrib paths for modules")
	assert.NotContains(t, xmlString, "/opt/solr/contrib/", "Solr 10 should not reference contrib directory")
	assert.NotContains(t, xmlString, "/opt/solr/dist", "Solr 10 should not reference dist directory for modules")

	// Solr 10: custom additional libs should still be included
	xmlString = GenerateAdditionalLibXMLPart([]string{}, []string{"/ext/lib1", "/ext/lib2"}, true)
	assert.EqualValuesf(t, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2</str>", xmlString, "Solr 10 should still include custom additional libs")

	// Solr 10: combination of modules (ignored in sharedLib) and custom libs
	xmlString = GenerateAdditionalLibXMLPart([]string{"ltr"}, []string{"/ext/lib1"}, true)
	assert.EqualValuesf(t, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1</str>", xmlString, "Solr 10 should include custom libs but not module contrib paths")
}

func TestGenerateSolrXMLStringForCloudSolr10(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrMajorVersion: 10,
			SolrImage: &solr.ContainerImage{
				Repository: "library/solr",
				Tag:        "10.0.0",
			},
			SolrModules: []string{"ltr", "analytics"},
		},
	}
	xmlString := GenerateSolrXMLStringForCloud(solrCloud)

	// Should NOT contain Solr 9-specific settings
	assert.NotContains(t, xmlString, "genericCoreNodeNames", "Solr 10 XML should not contain genericCoreNodeNames")
	assert.NotContains(t, xmlString, "hostContext", "Solr 10 XML should not contain hostContext")
	assert.NotContains(t, xmlString, "allowPaths", "Solr 10 XML should not contain allowPaths")
	assert.NotContains(t, xmlString, "metricsEnabled", "Solr 10 XML should not contain metricsEnabled")

	// Should use the new system property name for host
	assert.Contains(t, xmlString, "${solr.host.advertise:}", "Solr 10 XML should use solr.host.advertise sysprop")
	assert.NotContains(t, xmlString, "${host:}", "Solr 10 XML should not use deprecated host sysprop")

	// Should still contain valid Solr 10 settings
	assert.Contains(t, xmlString, "hostPort", "Solr 10 XML should still contain hostPort")
	assert.Contains(t, xmlString, "zkClientTimeout", "Solr 10 XML should still contain zkClientTimeout")
	assert.Contains(t, xmlString, "zkCredentialsProvider", "Solr 10 XML should still contain zkCredentialsProvider")

	// Should NOT contain contrib paths (modules loaded via SOLR_MODULES env var)
	assert.NotContains(t, xmlString, "/opt/solr/contrib/", "Solr 10 XML should not reference contrib directory")
}

func TestGenerateSolrXMLStringForCloudSolr9(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrMajorVersion: 9,
			SolrImage: &solr.ContainerImage{
				Repository: "library/solr",
				Tag:        "9.10.0",
			},
			SolrModules: []string{"ltr"},
		},
	}
	xmlString := GenerateSolrXMLStringForCloud(solrCloud)

	// Should contain Solr 9-specific settings
	assert.Contains(t, xmlString, "genericCoreNodeNames", "Solr 9 XML should contain genericCoreNodeNames")
	assert.Contains(t, xmlString, "hostContext", "Solr 9 XML should contain hostContext")
	assert.Contains(t, xmlString, "<str name=\"host\">", "Solr 9 XML should contain host setting")
	assert.Contains(t, xmlString, "allowPaths", "Solr 9 XML should contain allowPaths")

	// Should contain contrib paths for modules
	assert.Contains(t, xmlString, "/opt/solr/contrib/ltr/lib", "Solr 9 XML should reference contrib directory for modules")
}

func newProbeForTest() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/solr/admin/info/system",
				Port: intstr.FromInt32(8983),
			},
		},
	}
}

func TestUseSecureProbeForSolr9(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrMajorVersion: 9,
			SolrImage:        &solr.ContainerImage{Repository: "library/solr", Tag: "9.10.0"},
		},
	}
	probe := newProbeForTest()
	useSecureProbe(solrCloud, probe, "/etc/secrets/foo")

	assert.Nil(t, probe.HTTPGet, "HTTPGet should be replaced by Exec")
	assert.NotNil(t, probe.Exec, "Exec command should be set")
	cmd := probe.Exec.Command[2]
	assert.Contains(t, cmd, "solr api -get", "Solr 9 should use 'solr api -get' syntax")
	assert.Contains(t, cmd, "${SOLR_HOST}", "Solr 9 probe should reference SOLR_HOST env var")
	assert.NotContains(t, cmd, "--solr-url", "Solr 9 should not use Solr 10 --solr-url flag")
}

func TestUseSecureProbeForSolr10(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrMajorVersion: 10,
			SolrImage:        &solr.ContainerImage{Repository: "library/solr", Tag: "10.0.0"},
		},
	}
	probe := newProbeForTest()
	useSecureProbe(solrCloud, probe, "/etc/secrets/foo")

	assert.Nil(t, probe.HTTPGet, "HTTPGet should be replaced by Exec")
	assert.NotNil(t, probe.Exec, "Exec command should be set")
	cmd := probe.Exec.Command[2]
	assert.Contains(t, cmd, "solr api --solr-url", "Solr 10 should use 'solr api --solr-url' syntax")
	assert.NotContains(t, cmd, "-get ", "Solr 10 should not use Solr 9 -get flag")
}

func TestSetUrlSchemeClusterPropCmd(t *testing.T) {
	// Solr 9 uses zkcli.sh
	solr9Cmd := setUrlSchemeClusterPropCmd(false)
	assert.Contains(t, solr9Cmd, "zkcli.sh", "Solr 9 should call zkcli.sh")
	assert.Contains(t, solr9Cmd, "clusterprop", "Solr 9 should use the clusterprop subcommand")

	// Solr 10 uses `solr cluster --property`
	solr10Cmd := setUrlSchemeClusterPropCmd(true)
	assert.NotContains(t, solr10Cmd, "zkcli.sh", "Solr 10 must not call removed zkcli.sh")
	assert.Contains(t, solr10Cmd, "solr cluster --property urlScheme --value https", "Solr 10 should set the cluster property via the `solr cluster` CLI")
}

func newSolrCloudForTest(tag string, modules []string) *solr.SolrCloud {
	// Test tags are always well-formed (e.g. "10.0.0", "9.10.0")
	solrVersion := 9
	if len(tag) > 0 && tag[0] == '1' && len(tag) > 1 && tag[1] == '0' {
		solrVersion = 10
	}
	cloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrMajorVersion: solrVersion,
			SolrImage:        &solr.ContainerImage{Repository: "library/solr", Tag: tag},
			SolrModules:      modules,
		},
	}
	cloud.WithDefaults(logr.Discard())
	return cloud
}

func solrNodeContainerEnv(t *testing.T, cloud *solr.SolrCloud) []corev1.EnvVar {
	t.Helper()
	status := &solr.SolrCloudStatus{
		ZookeeperConnectionInfo: solr.ZookeeperConnectionInfo{
			InternalConnectionString: "zk:2181",
			ChRoot:                   "/solr",
		},
	}
	ss := GenerateStatefulSet(cloud, status, map[string]string{}, map[string]string{}, nil, nil)
	for _, c := range ss.Spec.Template.Spec.Containers {
		if c.Name == SolrNodeContainer {
			return c.Env
		}
	}
	t.Fatalf("solr node container not found in generated StatefulSet")
	return nil
}

func envValue(env []corev1.EnvVar, name string) (string, bool) {
	for _, e := range env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func TestGenerateStatefulSetSolr10EnvVars(t *testing.T) {
	cloud := newSolrCloudForTest("10.0.0", []string{"ltr", "analytics"})
	env := solrNodeContainerEnv(t, cloud)

	advertise, ok := envValue(env, "SOLR_HOST_ADVERTISE")
	assert.True(t, ok, "Solr 10 pod should have SOLR_HOST_ADVERTISE set")
	assert.NotEmpty(t, advertise, "SOLR_HOST_ADVERTISE should not be empty")

	modules, ok := envValue(env, "SOLR_MODULES")
	assert.True(t, ok, "Solr 10 pod should have SOLR_MODULES set when modules are configured")
	assert.Contains(t, modules, "ltr")
	assert.Contains(t, modules, "analytics")

	solrOpts, _ := envValue(env, "SOLR_OPTS")
	assert.NotContains(t, solrOpts, "-DhostPort=", "Solr 10 should not set the hostPort sysprop")
}

func TestGenerateStatefulSetSolr10NoModules(t *testing.T) {
	cloud := newSolrCloudForTest("10.0.0", nil)
	env := solrNodeContainerEnv(t, cloud)

	_, ok := envValue(env, "SOLR_MODULES")
	assert.False(t, ok, "SOLR_MODULES should not be set when no modules are configured")
}

func TestGenerateStatefulSetSolr9EnvVars(t *testing.T) {
	cloud := newSolrCloudForTest("9.10.0", []string{"ltr"})
	env := solrNodeContainerEnv(t, cloud)

	_, ok := envValue(env, "SOLR_HOST_ADVERTISE")
	assert.False(t, ok, "Solr 9 pod should not have SOLR_HOST_ADVERTISE set")

	_, ok = envValue(env, "SOLR_MODULES")
	assert.False(t, ok, "Solr 9 pod should not have SOLR_MODULES set; modules are loaded via sharedLib")

	solrOpts, ok := envValue(env, "SOLR_OPTS")
	assert.True(t, ok, "Solr 9 pod should have SOLR_OPTS set")
	assert.True(t, strings.Contains(solrOpts, "-DhostPort="), "Solr 9 should set the hostPort sysprop")
}
