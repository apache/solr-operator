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
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sort"
	"strings"
)

const (
	BaseBackupRestorePath = "/var/solr/data/backup-restore"

	GCSCredentialSecretKey = "service-account-key.json"
	S3CredentialFileName   = "credentials"
)

func RepoVolumeName(repo *solrv1beta1.SolrBackupRepository) string {
	return fmt.Sprintf("backup-repository-%s", repo.Name)
}

func IsRepoManaged(repo *solrv1beta1.SolrBackupRepository) bool {
	return repo.Managed != nil
}

func BackupRestoreSubPathForCloud(directoryOverride string, cloud string) string {
	if directoryOverride == "" {
		directoryOverride = cloud
	}
	return "cloud/" + directoryOverride
}

func BackupSubPathForCloud(directoryOverride string, cloud string, backupName string) string {
	return BackupRestoreSubPathForCloud(directoryOverride, cloud) + "/backups/" + backupName
}

func GcsRepoSecretMountPath(repo *solrv1beta1.SolrBackupRepository) string {
	return fmt.Sprintf("%s/%s/%s", BaseBackupRestorePath, repo.Name, "gcscredential")
}

func S3RepoSecretMountPath(repo *solrv1beta1.SolrBackupRepository) string {
	return fmt.Sprintf("%s/%s/%s", BaseBackupRestorePath, repo.Name, "s3credential")
}

func ManagedRepoVolumeMountPath(repo *solrv1beta1.SolrBackupRepository) string {
	return fmt.Sprintf("%s/%s", BaseBackupRestorePath, repo.Name)
}

func RepoVolumeSourceAndMount(repo *solrv1beta1.SolrBackupRepository, solrCloudName string) (source *corev1.VolumeSource, mount *corev1.VolumeMount) {
	f := false
	if repo.Managed != nil {
		source = &repo.Managed.Volume
		mount = &corev1.VolumeMount{
			MountPath: ManagedRepoVolumeMountPath(repo),
			SubPath:   BackupRestoreSubPathForCloud(repo.Managed.Directory, solrCloudName),
			ReadOnly:  false,
		}
	} else if repo.GCS != nil {
		source = &corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  repo.GCS.GcsCredentialSecret.Name,
				Items:       []corev1.KeyToPath{{Key: repo.GCS.GcsCredentialSecret.Key, Path: GCSCredentialSecretKey}},
				DefaultMode: &SecretReadOnlyPermissions,
				Optional:    &f,
			},
		}
		mount = &corev1.VolumeMount{
			MountPath: GcsRepoSecretMountPath(repo),
			ReadOnly:  true,
		}
	} else if repo.S3 != nil && repo.S3.Credentials != nil && repo.S3.Credentials.CredentialsFileSecret != nil {
		source = &corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  repo.S3.Credentials.CredentialsFileSecret.Name,
				Items:       []corev1.KeyToPath{{Key: repo.S3.Credentials.CredentialsFileSecret.Key, Path: S3CredentialFileName}},
				DefaultMode: &SecretReadOnlyPermissions,
				Optional:    &f,
			},
		}
		mount = &corev1.VolumeMount{
			MountPath: S3RepoSecretMountPath(repo),
			ReadOnly:  true,
		}
	}
	return
}

func RepoSolrModules(repo *solrv1beta1.SolrBackupRepository) (libs []string) {
	if repo.GCS != nil {
		libs = []string{"gcs-repository"}
	} else if repo.S3 != nil {
		libs = []string{"s3-repository"}
	}
	return
}

func AdditionalRepoLibs(repo *solrv1beta1.SolrBackupRepository) (libs []string) {
	return
}

func RepoXML(repo *solrv1beta1.SolrBackupRepository) (xml string) {
	if repo.Managed != nil {
		xml = fmt.Sprintf(`<repository name="%s" class="org.apache.solr.core.backup.repository.LocalFileSystemRepository"/>`, repo.Name)
	} else if repo.GCS != nil {
		xml = fmt.Sprintf(`
<repository name="%s" class="org.apache.solr.gcs.GCSBackupRepository">
    <str name="gcsBucket">%s</str>
    <str name="gcsCredentialPath">%s/%s</str>
</repository>`, repo.Name, repo.GCS.Bucket, GcsRepoSecretMountPath(repo), GCSCredentialSecretKey)
	} else if repo.S3 != nil {
		s3Extras := make([]string, 0)
		if repo.S3.Endpoint != "" {
			s3Extras = append(s3Extras, fmt.Sprintf("<str name=\"s3.endpoint\">%s</str>", repo.S3.Endpoint))
		}
		if repo.S3.ProxyUrl != "" {
			s3Extras = append(s3Extras, fmt.Sprintf("<str name=\"s3.proxy.url\">%s</str>", repo.S3.ProxyUrl))
		}
		xml = fmt.Sprintf(`
<repository name="%s" class="org.apache.solr.s3.S3BackupRepository">
    <str name="s3.bucket.name">%s</str>
    <str name="s3.region">%s</str>
    %s
</repository>`, repo.Name, repo.S3.Bucket, repo.S3.Region, strings.Join(s3Extras, `
    `))
	}
	return
}

func RepoEnvVars(repo *solrv1beta1.SolrBackupRepository) (envVars []corev1.EnvVar) {
	if repo.S3 != nil && repo.S3.Credentials != nil {
		// Env Var names sourced from: https://docs.aws.amazon.com/sdk-for-java/v2/developer-guide/credentials.html
		if repo.S3.Credentials.AccessKeyIdSecret != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:      "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: repo.S3.Credentials.AccessKeyIdSecret},
			})
		}
		if repo.S3.Credentials.SecretAccessKeySecret != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:      "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: repo.S3.Credentials.SecretAccessKeySecret},
			})
		}
		if repo.S3.Credentials.SessionTokenSecret != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:      "AWS_SESSION_TOKEN",
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: repo.S3.Credentials.SessionTokenSecret},
			})
		}
		// Env Var name sourced from: https://docs.aws.amazon.com/sdkref/latest/guide/file-location.html
		if repo.S3.Credentials.CredentialsFileSecret != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "AWS_SHARED_CREDENTIALS_FILE",
				Value: fmt.Sprintf("%s/%s", S3RepoSecretMountPath(repo), S3CredentialFileName),
			})
		}
	}
	return envVars
}

func GenerateBackupRepositoriesForSolrXml(backupRepos []solrv1beta1.SolrBackupRepository) (repoXML string, solrModules []string, additionalLibs []string) {
	if len(backupRepos) == 0 {
		return
	}
	repoXMLs := make([]string, len(backupRepos))

	for i, repo := range backupRepos {
		solrModules = append(solrModules, RepoSolrModules(&repo)...)
		additionalLibs = append(additionalLibs, AdditionalRepoLibs(&repo)...)
		repoXMLs[i] = RepoXML(&repo)
	}
	sort.Strings(repoXMLs)

	repoXML = fmt.Sprintf(
		`<backup>
		%s
		</backup>`, strings.Join(repoXMLs, `
`))
	return
}

func IsBackupVolumePresent(repo *solrv1beta1.SolrBackupRepository, pod *corev1.Pod) bool {
	expectedVolumeName := RepoVolumeName(repo)
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == expectedVolumeName {
			return true
		}
	}
	return false
}

func BackupLocationPath(repo *solrv1beta1.SolrBackupRepository, backupName string) string {
	if repo.Managed != nil {
		return fmt.Sprintf("%s/backups/%s", ManagedRepoVolumeMountPath(repo), backupName)
	} else if repo.GCS != nil {
		if repo.GCS.BaseLocation != "" {
			return repo.GCS.BaseLocation
		}
		return "/"
	}
	return ""
}
