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

func TestSolrMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		image   *solr.ContainerImage
		want    int
		wantErr bool
	}{
		{name: "Solr 8 release", image: &solr.ContainerImage{Tag: "8.11.4"}, want: 8},
		{name: "Solr 9 release", image: &solr.ContainerImage{Tag: "9.10.0"}, want: 9},
		{name: "Solr 10 release", image: &solr.ContainerImage{Tag: "10.0.0"}, want: 10},
		{name: "snapshot", image: &solr.ContainerImage{Tag: "10.1.0-SNAPSHOT"}, want: 10},
		{name: "custom suffix", image: &solr.ContainerImage{Tag: "10.2.0-my-company-custom-overlay-9.3"}, want: 10},
		{name: "more than three version parts", image: &solr.ContainerImage{Tag: "10.0.0.1.2"}, want: 10},
		{name: "pinned digest", image: &solr.ContainerImage{Tag: "10.0.0@sha256:0123456789abcdef"}, want: 10},
		{name: "slim variant", image: &solr.ContainerImage{Tag: "9.10.0-slim"}, want: 9},
		{name: "major only", image: &solr.ContainerImage{Tag: "10"}, want: 10},
		{name: "major.minor", image: &solr.ContainerImage{Tag: "9.10"}, want: 9},
		{name: "default tag of operator v0.6 to v0.9", image: &solr.ContainerImage{Tag: "8.11"}, want: 8},
		{name: "default tag of operator v0.5", image: &solr.ContainerImage{Tag: "8.9"}, want: 8},
		{name: "major with variant", image: &solr.ContainerImage{Tag: "9-slim"}, want: 9},
		{name: "trailing dot", image: &solr.ContainerImage{Tag: "10."}, wantErr: true},
		{name: "dot without number", image: &solr.ContainerImage{Tag: "10..0"}, wantErr: true},
		{name: "v prefix", image: &solr.ContainerImage{Tag: "v10.0.0"}, wantErr: true},
		{name: "latest", image: &solr.ContainerImage{Tag: "latest"}, wantErr: true},
		{name: "slim", image: &solr.ContainerImage{Tag: "slim"}, wantErr: true},
		{name: "non-version tag", image: &solr.ContainerImage{Tag: "my-build-42"}, wantErr: true},
		{name: "unsupported old major", image: &solr.ContainerImage{Tag: "7.7.3"}, wantErr: true},
		{name: "unsupported new major", image: &solr.ContainerImage{Tag: "11.0.0"}, wantErr: true},
		{name: "calendar-versioned tag", image: &solr.ContainerImage{Tag: "2026.10.1"}, wantErr: true},
		{name: "no tag", image: &solr.ContainerImage{}, wantErr: true},
		{name: "no image", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloud := &solr.SolrCloud{Spec: solr.SolrCloudSpec{SolrImage: tt.image}}
			got, err := cloud.SolrMajorVersion()
			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, cloud.IsSolr10OrLater(), "an unsupported version is treated as pre-10")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want >= 10, cloud.IsSolr10OrLater())
		})
	}
}

func TestSolrMajorVersionFromDefaultImage(t *testing.T) {
	cloud := &solr.SolrCloud{}
	cloud.WithDefaults(logr.Discard())
	got, err := cloud.SolrMajorVersion()
	assert.NoError(t, err, "the default Solr image tag must be supported")
	assert.Equal(t, 9, got)
}

func TestGenerateAdditionalLibXMLPartSolr10(t *testing.T) {
	// Solr 10: modules are loaded from the modules directory, the contrib and dist directories don't exist anymore
	xmlString := GenerateAdditionalLibXMLPart([]string{"gcs-repository", "analytics"}, []string{}, true)
	assert.EqualValuesf(t, "<str name=\"sharedLib\">${solr.sharedLib:},/opt/solr/modules/analytics/lib,/opt/solr/modules/gcs-repository/lib</str>", xmlString, "Wrong sharedLib xml for Solr 10 modules")
	assert.NotContains(t, xmlString, "/opt/solr/contrib/", "Solr 10 should not reference the contrib directory")
	assert.NotContains(t, xmlString, "/opt/solr/dist", "Solr 10 should not reference the dist directory")

	// Solr 10: custom additional libs should still be included
	xmlString = GenerateAdditionalLibXMLPart([]string{}, []string{"/ext/lib1", "/ext/lib2"}, true)
	assert.EqualValuesf(t, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/ext/lib2</str>", xmlString, "Solr 10 should still include custom additional libs")

	// Solr 10: combination of modules and custom libs, with a duplicate module
	xmlString = GenerateAdditionalLibXMLPart([]string{"ltr", "ltr"}, []string{"/ext/lib1"}, true)
	assert.EqualValuesf(t, "<str name=\"sharedLib\">${solr.sharedLib:},/ext/lib1,/opt/solr/modules/ltr/lib</str>", xmlString, "Wrong sharedLib xml for Solr 10 modules and custom libs")
}

func TestGenerateSolrXMLStringForCloudSolr10(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
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
	assert.Contains(t, xmlString, "${solr.security.allow.paths:}", "Solr 10 XML should read allowPaths from its Solr 10 sysprop")
	assert.NotContains(t, xmlString, "${solr.allowPaths:}", "Solr 10 XML should not use the Solr 9 allowPaths sysprop")
	assert.NotContains(t, xmlString, "metricsEnabled", "Solr 10 XML should not contain metricsEnabled")

	// Should use the new system property name for host
	assert.Contains(t, xmlString, "${solr.host.advertise:}", "Solr 10 XML should use solr.host.advertise sysprop")
	assert.NotContains(t, xmlString, "${host:}", "Solr 10 XML should not use deprecated host sysprop")

	// Should still contain valid Solr 10 settings
	assert.Contains(t, xmlString, "hostPort", "Solr 10 XML should still contain hostPort")
	assert.Contains(t, xmlString, "zkClientTimeout", "Solr 10 XML should still contain zkClientTimeout")
	assert.Contains(t, xmlString, "zkCredentialsProvider", "Solr 10 XML should still contain zkCredentialsProvider")

	// Should load modules from the modules directory, not the removed contrib directory
	assert.Contains(t, xmlString, "/opt/solr/modules/ltr/lib", "Solr 10 XML should load modules from the modules directory")
	assert.NotContains(t, xmlString, "/opt/solr/contrib/", "Solr 10 XML should not reference contrib directory")
}

// solr10StockSolrXml is the body of server/solr/solr.xml from the Solr 10.0.0 release (license header and comments removed).
const solr10StockSolrXml = `<solr>
  <int name="maxBooleanClauses">${solr.max.booleanClauses:1024}</int>
  <str name="sharedLib">${solr.sharedLib:}</str>
  <str name="modules">${solr.modules:}</str>
  <str name="allowPaths">${solr.security.allow.paths:}</str>
  <str name="allowUrls">${solr.security.allow.urls:}</str>
  <str name="hideStackTrace">${solr.hideStackTrace:false}</str>
  <int name="indexSearcherExecutorThreads">${solr.searchThreads:0}</int>
  <solrcloud>
    <str name="host">${solr.host.advertise:}</str>
    <int name="hostPort">${solr.port.advertise:0}</int>
    <int name="zkClientTimeout">${zkClientTimeout:30000}</int>
    <int name="distribUpdateSoTimeout">${distribUpdateSoTimeout:600000}</int>
    <int name="distribUpdateConnTimeout">${distribUpdateConnTimeout:60000}</int>
    <str name="zkCredentialsProvider">${zkCredentialsProvider:org.apache.solr.common.cloud.DefaultZkCredentialsProvider}</str>
    <str name="zkACLProvider">${zkACLProvider:org.apache.solr.common.cloud.DefaultZkACLProvider}</str>
    <str name="zkCredentialsInjector">${zkCredentialsInjector:org.apache.solr.common.cloud.DefaultZkCredentialsInjector}</str>
    <int name="minStateByteLenForCompression">${minStateByteLenForCompression:-1}</int>
    <str name="stateCompressor">${stateCompressor:org.apache.solr.common.util.ZLibCompressor}</str>
  </solrcloud>
  <shardHandlerFactory name="shardHandlerFactory" class="${defaultShardHandlerFactory:HttpShardHandlerFactory}">
    <int name="socketTimeout">${socketTimeout:600000}</int>
    <int name="connTimeout">${connTimeout:60000}</int>
  </shardHandlerFactory>
</solr>`

// Without extra libraries or backup repositories, the generated Solr 10 solr.xml should be identical to Solr's stock one.
func TestSolr10SolrXmlMatchesStock(t *testing.T) {
	normalize := func(xml string) []string {
		lines := make([]string, 0)
		for _, line := range strings.Split(xml, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "<?xml") {
				lines = append(lines, line)
			}
		}
		return lines
	}
	generated := GenerateSolrXMLString("", []string{}, []string{}, true)
	assert.Equal(t, normalize(solr10StockSolrXml), normalize(generated))
}

func TestGenerateSolrXMLStringForCloudSolr9(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
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
			SolrImage: &solr.ContainerImage{Repository: "library/solr", Tag: "9.10.0"},
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
			SolrImage: &solr.ContainerImage{Repository: "library/solr", Tag: "10.0.0"},
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

func TestUseSecureProbeWithBasicAuth(t *testing.T) {
	newCloud := func(tag string) *solr.SolrCloud {
		return &solr.SolrCloud{
			Spec: solr.SolrCloudSpec{
				SolrImage: &solr.ContainerImage{Repository: "library/solr", Tag: tag},
				SolrSecurity: &solr.SolrSecurityOptions{
					AuthenticationType: solr.Basic,
					ProbesRequireAuth:  true,
				},
			},
		}
	}
	creds := `"$(cat /etc/secrets/foo/username):$(cat /etc/secrets/foo/password)"`

	probe := newProbeForTest()
	useSecureProbe(newCloud("9.10.0"), probe, "/etc/secrets/foo")
	cmd := probe.Exec.Command[2]
	assert.Contains(t, cmd, "-Dbasicauth=$(cat /etc/secrets/foo/username):$(cat /etc/secrets/foo/password)", "Solr 9 should pass credentials via JAVA_TOOL_OPTIONS")
	assert.NotContains(t, cmd, "--credentials", "Solr 9 should not use the Solr 10 --credentials flag")

	probe = newProbeForTest()
	useSecureProbe(newCloud("10.0.0"), probe, "/etc/secrets/foo")
	cmd = probe.Exec.Command[2]
	assert.Equal(t, `solr api --solr-url "http://${SOLR_HOST}:8983/solr/admin/info/system" --credentials `+creds, cmd, "Solr 10 should pass credentials via --credentials")
	assert.NotContains(t, cmd, "JAVA_TOOL_OPTIONS", "Solr 10 no longer reads the basicauth sysprop")
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
	cloud := &solr.SolrCloud{
		Spec: solr.SolrCloudSpec{
			SolrImage:   &solr.ContainerImage{Repository: "library/solr", Tag: tag},
			SolrModules: modules,
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

	_, ok = envValue(env, "SOLR_MODULES")
	assert.False(t, ok, "SOLR_MODULES should not be set, modules are loaded via sharedLib")

	solrOpts, _ := envValue(env, "SOLR_OPTS")
	assert.Contains(t, solrOpts, "-DhostPort=", "Solr 10 should still set the hostPort sysprop for custom solr.xml files")
}

func TestGenerateStatefulSetListenPort(t *testing.T) {
	for _, tag := range []string{"9.10.0", "10.0.0"} {
		cloud := newSolrCloudForTest(tag, nil)
		cloud.Spec.SolrAddressability.PodPort = 8080
		env := solrNodeContainerEnv(t, cloud)

		port, _ := envValue(env, "SOLR_PORT")
		assert.Equal(t, "8080", port, "SOLR_PORT should be the pod port for Solr %s", tag)

		listenPort, ok := envValue(env, "SOLR_PORT_LISTEN")
		if tag == "10.0.0" {
			assert.Equal(t, "8080", listenPort, "Solr 10 reads the listen port from SOLR_PORT_LISTEN")
		} else {
			assert.False(t, ok, "SOLR_PORT_LISTEN should only be set for Solr 10+, to not restart older pods")
		}
	}
}

func TestGenerateStatefulSetSolr9EnvVars(t *testing.T) {
	cloud := newSolrCloudForTest("9.10.0", []string{"ltr"})
	env := solrNodeContainerEnv(t, cloud)

	_, ok := envValue(env, "SOLR_HOST_ADVERTISE")
	assert.False(t, ok, "Solr 9 pod should not have SOLR_HOST_ADVERTISE set")

	_, ok = envValue(env, "SOLR_MODULES")
	assert.False(t, ok, "SOLR_MODULES should not be set, modules are loaded via sharedLib")

	solrOpts, ok := envValue(env, "SOLR_OPTS")
	assert.True(t, ok, "Solr 9 pod should have SOLR_OPTS set")
	assert.True(t, strings.Contains(solrOpts, "-DhostPort="), "Solr 9 should set the hostPort sysprop")
}

func TestImageVersion(t *testing.T) {
	assert.Equal(t, "10.0.0", solr.ImageVersion("library/solr:10.0.0"))
	assert.Equal(t, "10.0.0", solr.ImageVersion("library/solr:10.0.0@sha256:0123456789abcdef"), "a digest should be ignored")
	assert.Equal(t, "10.0.0", solr.ImageVersion("my-registry:5000/solr:10.0.0"))
	assert.Equal(t, "", solr.ImageVersion("library/solr"))
}
