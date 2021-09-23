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

package v1beta1

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
)

const (
	BaseBackupRestoreSecretsPath = "/var/solr/data/backup-restore-secrets"

	GCSCredentialSecretKey = "service-account-key.json"

	DistLibs = "/opt/solr/dist"
	ContribLibs = "/opt/solr/contrib/%s/lib"
)

func (repo *SolrBackupRepository) VolumeName() string {
	return fmt.Sprintf("backup-repository-%s", repo.Name)
}

func (repo *SolrBackupRepository) IsManaged() bool {
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

func (repo *SolrBackupRepository) GcsSecretMountPath() string {
	return fmt.Sprintf("%s/%s", BaseBackupRestoreSecretsPath, repo.Name, "gcscredential")
}

func (repo *SolrBackupRepository) ManagedVolumeMountPath() string {
	return fmt.Sprintf("%s/%s", BaseBackupRestorePath, repo.Name)
}

func (repo *SolrBackupRepository) GetVolumeSourceAndMount(solrCloudName string) (source *corev1.VolumeSource, mount *corev1.VolumeMount) {
	f := false
	if repo.Managed != nil {
		source = &repo.Managed.Volume
		mount = &corev1.VolumeMount{
			MountPath: repo.ManagedVolumeMountPath(),
			SubPath:   BackupRestoreSubPathForCloud(repo.Managed.Directory, solrCloudName),
			ReadOnly:  false,
		}
	} else if repo.GCS != nil {
		source = &corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: repo.GCS.GcsCredentialSecret.Name,
				Items:      []corev1.KeyToPath{{Key: repo.GCS.GcsCredentialSecret.Key, Path: GCSCredentialSecretKey}},
				Optional:   &f,
			},
		}
		mount = &corev1.VolumeMount{
			MountPath: repo.GcsSecretMountPath(),
			ReadOnly:  true,
		}
	}
	return
}

func (repo *SolrBackupRepository) GetAdditionalLibs() (libs []string) {
	if repo.GCS != nil {
		libs = []string{DistLibs, fmt.Sprintf(ContribLibs, "gcs-repository")}
	}
	return
}

func (repo *SolrBackupRepository) GetRepoXML() (xml string) {
	if repo.Managed != nil {
		xml = fmt.Sprintf(`<repository name="%s" class="org.apache.solr.core.backup.repository.LocalFileSystemRepository"/>`, repo.Name)
	} else if repo.GCS != nil {
		xml = fmt.Sprintf(`
<repository name="%s" class="org.apache.solr.gcs.GCSBackupRepository">
    <str name="gcsBucket">%s</str>
    <str name="gcsCredentialPath">%s/%s</str>
</repository>`, repo.Name, repo.GCS.Bucket, repo.GcsSecretMountPath(), GCSCredentialSecretKey)
	}
	return
}

func (repo *SolrBackupRepository) GetEnvVars() (envVars []corev1.EnvVar) {
	return envVars
}

func (repo *SolrBackupRepository) IsBackupVolumePresent(pod *corev1.Pod) bool {
	expectedVolumeName := repo.VolumeName()
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == expectedVolumeName {
			return true
		}
	}
	return false
}

func (repo *SolrBackupRepository) BackupLocationPath(backupName string) string {
	if repo.Managed != nil {
		return fmt.Sprintf("%s/backups/%s", repo.ManagedVolumeMountPath(), backupName)
	} else if repo.GCS != nil {
		if repo.GCS.BaseLocation != "" {
			return repo.GCS.BaseLocation
		}
		return "/"
	}
	return ""
}
