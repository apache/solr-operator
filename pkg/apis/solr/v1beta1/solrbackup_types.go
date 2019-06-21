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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SolrBackupSpec defines the desired state of SolrBackup
type SolrBackupSpec struct {
	// A reference to the SolrCloud to create a backup for
	SolrCloudRef types.NamespacedName `json:"solrCloudRef"`

	// A reference to a persistent volume to store the generated backup into
	PVReference types.NamespacedName `json:"persistentVolumeRef"`

	// The list of collections to backup. If empty, all collections in the cloud will be backed up.
	// +optional
	Collections []string `json:"collections,omitempty"`
}

// SolrBackupStatus defines the observed state of SolrBackup
type SolrBackupStatus struct {
	// Is the backup complete?
	Done bool `json:"done"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SolrBackup is the Schema for the solrbackups API
// +k8s:openapi-gen=true
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
