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
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	DefaultAWSCliImageRepo    = "infrastructureascode/aws-cli"
	DefaultAWSCliImageVersion = "1.16.204"
	DefaultS3Retries = 5
)

// SolrBackupSpec defines the desired state of SolrBackup
type SolrBackupSpec struct {
	// A reference to the SolrCloud to create a backup for
	SolrCloud string `json:"solrCloud"`

	// The list of collections to backup. If empty, all collections in the cloud will be backed up.
	// +optional
	Collections []string `json:"collections,omitempty"`

	// Persistence is the specification on how to persist the backup data.
	Persistence PersistenceSource `json:"persistence"`
}

func (spec *SolrBackupSpec) withDefaults() (changed bool) {
	changed = spec.Persistence.withDefaults() || changed

	return changed
}

// PersistenceSource defines the location and method of persisting the backup data.
// Exactly one member must be specified.
type PersistenceSource struct {
	// Persist to an s3 compatible endpoint
	// +optional
	S3 *S3PersistenceSource `json:"S3,omitempty"`

	// Persist to a volume
	// +optional
	Volume *VolumePersistenceSource `json:"volume,omitempty"`
}

func (spec *PersistenceSource) withDefaults() (changed bool) {
	if spec.Volume != nil {
		changed = spec.Volume.withDefaults() || changed
	}

	if spec.S3 != nil {
		changed = spec.S3.withDefaults() || changed
	}

	return changed
}

// UploadSpec defines the location and method of uploading the backup data
type S3PersistenceSource struct {
	// The s3 compatible endpoint
	Endpoint string `json:"endpoint"`

	// Image containing the AWS Cli
	// +optional
	AWSCliImage ContainerImage `json:"AWSCliImage,omitempty"`

	// The number of retries to communicate with S3
	// +optional
	Retries *int32 `json:"retries,omitempty"`
}

func (spec *S3PersistenceSource) withDefaults() (changed bool) {
	changed = spec.AWSCliImage.withDefaults(DefaultAWSCliImageRepo, DefaultAWSCliImageVersion, DefaultPullPolicy) || changed

	return changed
}

// UploadSpec defines the location and method of uploading the backup data
type VolumePersistenceSource struct {
	// The volume for persistence
	VolumeSource corev1.VolumeSource `json:"source"`

	// The location of the persistence directory within the volume
	// +optional
	Path string `json:"path,omitempty"`

	// BusyBox image for manipulating and moving data
	// +optional
	BusyBoxImage ContainerImage `json:"busyBoxImage,omitempty"`
}

func (spec *VolumePersistenceSource) withDefaults() (changed bool) {
	changed = spec.BusyBoxImage.withDefaults(DefaultBusyBoxImageRepo, DefaultBusyBoxImageVersion, DefaultPullPolicy) || changed

	if spec.Path != "" && strings.HasPrefix(spec.Path, "/") {
		spec.Path = strings.TrimPrefix(spec.Path, "/")
		changed = true
	}

	return changed
}

// SolrBackupStatus defines the observed state of SolrBackup
type SolrBackupStatus struct {
	// Version of the Solr being backed up
	SolrVersion string `json:"solrVersion"`

	// The status of each collection's backup progress
	// +optional
	CollectionBackupStatuses []CollectionBackupStatus `json:"collectionBackupStatuses,omitempty"`

	// Whether the backups are in progress of being persisted
	PersistenceStatus BackupPersistenceStatus `json:"persistenceStatus"`

	// Version of the Solr being backed up
	// +optional
	FinishTime *metav1.Time `json:"finishTimestamp,omitempty"`

	// Whether the backup was successful
	// +optional
	Successful *bool `json:"successful,omitempty"`

	// Whether the backup has finished
	Finished bool `json:"finished,omitempty"`
}

// CollectionBackupStatus defines the progress of a Solr Collection's backup
type CollectionBackupStatus struct {
	// Solr Collection name
	Collection string `json:"collection"`

	// Whether the collection is being backed up
	// +optional
	InProgress bool `json:"inProgress,omitempty"`

	// Time that the collection backup started at
	// +optional
	StartTime *metav1.Time `json:"startTimestamp,omitempty"`

	// The status of the asynchronous backup call to solr
	// +optional
	AsyncBackupStatus string `json:"asyncBackupStatus,omitempty"`

	// Whether the backup has finished
	Finished bool `json:"finished,omitempty"`

	// Time that the collection backup finished at
	// +optional
	FinishTime *metav1.Time `json:"finishTimestamp,omitempty"`

	// Whether the backup was successful
	// +optional
	Successful *bool `json:"successful,omitempty"`
}

// BackupPersistenceStatus defines the status of persisting Solr backup data
type BackupPersistenceStatus struct {
	// Whether the collection is being backed up
	// +optional
	InProgress bool `json:"inProgress,omitempty"`

	// Time that the collection backup started at
	// +optional
	StartTime *metav1.Time `json:"startTimestamp,omitempty"`

	// Whether the persistence has finished
	Finished bool `json:"finished,omitempty"`

	// Time that the collection backup finished at
	// +optional
	FinishTime *metav1.Time `json:"finishTimestamp,omitempty"`

	// Whether the backup was successful
	// +optional
	Successful *bool `json:"successful,omitempty"`
}

func (sb *SolrBackup) SharedLabels() map[string]string {
	return sb.SharedLabelsWith(map[string]string{})
}

func (sb *SolrBackup) SharedLabelsWith(labels map[string]string) map[string]string {
	newLabels := map[string]string{}

	if labels != nil {
		for k, v := range labels {
			newLabels[k] = v
		}
	}

	newLabels["solr-backup"] = sb.Name
	return newLabels
}

// HeadlessServiceName returns the name of the headless service for the cloud
func (sb *SolrBackup) PersistenceJobName() string {
	return fmt.Sprintf("%s-solr-backup-persistence", sb.GetName())
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

// WithDefaults set default values when not defined in the spec.
func (sb *SolrBackup) WithDefaults() bool {
	return sb.Spec.withDefaults()
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
