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
	"k8s.io/api/core/v1"
)

func (backupOptions *SolrBackupRestoreOptions) IsManaged() bool {
	return true
}

func (backupOptions *SolrBackupRestoreOptions) GetVolumeSource() v1.VolumeSource {
	return *backupOptions.Volume
}

func (backupOptions *SolrBackupRestoreOptions) GetDirectory() string {
	return backupOptions.Directory
}

func (backupOptions *SolrBackupRestoreOptions) GetSolrMountPath() string {
	return BaseBackupRestorePath
}

func (backupOptions *SolrBackupRestoreOptions) GetInitPodMountPath() string {
	return BackupRestoreInitContainerPath
}

func (backupOptions *SolrBackupRestoreOptions) GetBackupPath(backupName string) string {
	return backupOptions.GetSolrMountPath() + "/backups/" + backupName
}

func (backupOptions *SolrBackupRestoreOptions) GetVolumeName() string {
	return BackupRestoreVolume
}

func (gcsRepository *GcsStorage) IsManaged() bool { return false }

func (gcsRepository *GcsStorage) GetSolrMountPath() string {
	return fmt.Sprintf("%s-%s-%s", BaseBackupRestorePath, "gcscredential", gcsRepository.Name)
}

func (gcsRepository *GcsStorage) GetInitPodMountPath() string {
	return ""
}

func (gcsRepository *GcsStorage) GetBackupPath(backupName string) string {
	if gcsRepository.BaseLocation != "" {
		return gcsRepository.BaseLocation
	}
	return "/"
}

func (gcsRepository *GcsStorage) GetVolumeName() string {
	return fmt.Sprintf("%s-%s", gcsRepository.Name, BackupRestoreCredentialVolume)
}

func (managedRepository *ManagedStorage) IsManaged() bool { return true }

func (managedRepository *ManagedStorage) GetVolumeSource() v1.VolumeSource {
	return *managedRepository.Volume
}

func (managedRepository *ManagedStorage) GetDirectory() string {
	return managedRepository.Directory
}

func (managedRepository *ManagedStorage) GetSolrMountPath() string {
	return fmt.Sprintf("%s-%s-%s", BaseBackupRestorePath, "managed", managedRepository.Name)
}

func (managedRepository *ManagedStorage) GetInitPodMountPath() string {
	return fmt.Sprintf("/backup-restore-managed-%s", managedRepository.Name)
}

func (managedRepository *ManagedStorage) GetBackupPath(backupName string) string {
	return managedRepository.GetSolrMountPath() + "/backups/" + backupName
}

func (managedRepository *ManagedStorage) GetVolumeName() string {
	return fmt.Sprintf("%s-%s", managedRepository.Name, BackupRestoreVolume)
}

func (gcs *GcsStorage) IsCredentialVolumePresent(volumes []v1.Volume) bool {
	expectedVolumeName := gcs.GetVolumeName()
	return VolumeExistsWithName(expectedVolumeName, volumes)
}

func (managedRepository *ManagedStorage) IsBackupVolumePresent(volumes []v1.Volume) bool {
	expectedVolumeName := managedRepository.GetVolumeName()
	return VolumeExistsWithName(expectedVolumeName, volumes)
}

func VolumeExistsWithName(needle string, haystack []v1.Volume) bool {
	for _, volume := range haystack {
		if volume.Name == needle {
			return true
		}
	}
	return false
}
