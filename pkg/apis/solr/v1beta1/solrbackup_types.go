/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SolrBackupSpec defines the desired state of SolrBackup
type SolrBackupSpec struct {
	// A reference to the SolrCloud to create a backup for
	SolrCloud string `json:"solrCloud"`

	// The list of collections to backup. If empty, all collections in the cloud will be backed up.
	// +optional
	Collections []string `json:"collections,omitempty"`

	// The path to backup the data to within the PersistentVolume
	// Defaults to '/'
	// +optional
	BackupPath string `json:"backupPath,omitempty"`

	// PersistentVolumeClaimSpec is the spec to describe PVC to store the backup in.
	// This must have `accessModes: - ReadWriteMany`, as the PV will be shared across solr nodes.
	PersistentVolumeClaimSpec corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec"`
}

// SolrBackupStatus defines the observed state of SolrBackup
type SolrBackupStatus struct {
	// Version of the Solr being backed up
	SolrVersion string `json:"solrVersion"`

	// Whether the backup has started
	Started bool `json:"started"`

	// Whether the backup has finished
	Finished bool `json:"finished"`

	// Version of the Solr being backed up
	// +optional
	TimeFinished *metav1.Time `json:"finishedTimestamp,omitempty"`

	// Whether the backup was successful
	// +optional
	Successful *bool `json:"successful,omitempty"`

	// The name of the PVC
	PVC string `json:"pvc,omitempty"`

	// Whether the PVC has been mapped to a Persistent Volume
	PVCPhase corev1.PersistentVolumeClaimPhase `json:"pvcPhase,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SolrBackup is the Schema for the solrbackups API
// +k8s:openapi-gen=true
// +kubebuilder:categories=all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cloud",type="string",JSONPath=".spec.solrCloud",description="Solr Cloud"
// +kubebuilder:printcolumn:name="Started",type="string",JSONPath=".status.started",description="Whether the backup has started"
// +kubebuilder:printcolumn:name="Finished",type="integer",JSONPath=".status.finished",description="Whether the backup has finished"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type SolrBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrBackupSpec   `json:"spec,omitempty"`
	Status SolrBackupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SolrBackupList contains a list of SolrBackup
type SolrBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrBackup{}, &SolrBackupList{})
}
