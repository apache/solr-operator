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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	DefaultAWSCliImageRepo    = "infrastructureascode/aws-cli"
	DefaultAWSCliImageVersion = "1.16.204"
	DefaultS3Retries          = 5
)

// SolrBackupSpec defines the desired state of SolrBackup
type SolrBackupSpec struct {
	// A reference to the SolrCloud to create a backup for
	SolrCloud string `json:"solrCloud"`

	// The name of the repository to use for the backup.  Defaults to "legacy_local_repository" if not specified (the
	// auto-configured repository for legacy singleton volumes).
	// +optional
	RepositoryName string `json:"repositoryName,omitempty"`

	// The list of collections to backup. If empty, all collections in the cloud will be backed up.
	// +optional
	Collections []string `json:"collections,omitempty"`

	// The location to store the backup in the specified backup repository.
	// +optional
	Location string `json:"location,omitempty"`

	// Set this backup to be taken recurrently, with options for scheduling and storage.
	//
	// NOTE: This is only supported for Solr Clouds version 8.9+, as it uses the incremental backup API.
	//
	// +optional
	Recurrence *BackupRecurrence `json:"recurrence,omitempty"`

	// Persistence is the specification on how to persist the backup data.
	// +optional
	Persistence *PersistenceSource `json:"persistence,omitempty"`
}

func (spec *SolrBackupSpec) withDefaults(backupName string) (changed bool) {
	if spec.Persistence != nil {
		changed = spec.Persistence.withDefaults(backupName) || changed
	}

	return changed
}

// BackupRecurrence defines the recurrence of the incremental backup
type BackupRecurrence struct {
	// Perform a backup on the given schedule, in CRON format.
	//
	// Multiple CRON syntaxes are supported
	//   - Standard CRON (e.g. "CRON_TZ=Asia/Seoul 0 6 * * ?")
	//   - Predefined Schedules (e.g. "@yearly", "@weekly", "@daily", etc.)
	//   - Intervals (e.g. "@every 10h30m")
	//
	// For more information please check this reference:
	// https://pkg.go.dev/github.com/robfig/cron/v3?utm_source=godoc#hdr-CRON_Expression_Format
	Schedule string `json:"schedule"`

	// Define the number of backup points to save for this backup at any given time.
	// The oldest backups will be deleted if too many exist when a backup is taken.
	// If not provided, this defaults to 10.
	//
	// +kubebuilder:default:=10
	// +kubebuilder:validation:Minimum:=1
	// +optional
	MaxSaved int `json:"maxSaved,omitempty"`
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

func (spec *PersistenceSource) withDefaults(backupName string) (changed bool) {
	if spec.Volume != nil {
		changed = spec.Volume.withDefaults(backupName) || changed
	}

	if spec.S3 != nil {
		changed = spec.S3.withDefaults(backupName) || changed
	}

	return changed
}

// S3PersistenceSource defines the specs for connecting to s3 for persistence
type S3PersistenceSource struct {
	// The S3 compatible endpoint URL
	// +optional
	EndpointUrl string `json:"endpointUrl,omitempty"`

	// The Default region to use with AWS.
	// Can also be provided through a configFile in the secrets.
	// Overridden by any endpointUrl value provided.
	// +optional
	Region string `json:"region,omitempty"`

	// The S3 bucket to store/find the backup data
	Bucket string `json:"bucket"`

	// The key for the referenced tarred & zipped backup file
	// Defaults to the name of the backup/restore + '.tgz'
	// +optional
	Key string `json:"key"`

	// The number of retries to communicate with S3
	// +optional
	Retries *int32 `json:"retries,omitempty"`

	// The secrets to use when configuring and authenticating s3 calls
	Secrets S3Secrets `json:"secrets"`

	// Image containing the AWS Cli
	// +optional
	AWSCliImage ContainerImage `json:"AWSCliImage,omitempty"`
}

func (spec *S3PersistenceSource) withDefaults(backupName string) (changed bool) {
	changed = spec.AWSCliImage.withDefaults(DefaultAWSCliImageRepo, DefaultAWSCliImageVersion, DefaultPullPolicy) || changed

	if spec.Key == "" {
		spec.Key = backupName + ".tgz"
		changed = true
	} else if strings.HasPrefix(spec.Key, "/") {
		spec.Key = strings.TrimPrefix(spec.Key, "/")
		changed = true
	}
	if spec.Retries == nil {
		retries := int32(DefaultS3Retries)
		spec.Retries = &retries
		changed = true
	}

	return changed
}

// S3Secrets describes the secrets provided for accessing s3.
type S3Secrets struct {
	// The name of the secrets object to use
	Name string `json:"fromSecret"`

	// The key (within the provided secret) of an AWS Config file to use
	// +optional
	ConfigFile string `json:"configFile,omitempty"`

	// The key (within the provided secret) of an AWS Credentials file to use
	// +optional
	CredentialsFile string `json:"credentialsFile,omitempty"`

	// The key (within the provided secret) of the Access Key ID to use
	// +optional
	AccessKeyId string `json:"accessKeyId,omitempty"`

	// The key (within the provided secret) of the Secret Access Key to use
	// +optional
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// UploadSpec defines the location and method of uploading the backup data
type VolumePersistenceSource struct {
	// The volume for persistence
	VolumeSource corev1.VolumeSource `json:"source"`

	// The location of the persistence directory within the volume
	// +optional
	Path string `json:"path,omitempty"`

	// The filename of the tarred & zipped backup file
	// Defaults to the name of the backup/restore + '.tgz'
	// +optional
	Filename string `json:"filename"`

	// BusyBox image for manipulating and moving data
	// +optional
	BusyBoxImage ContainerImage `json:"busyBoxImage,omitempty"`
}

func (spec *VolumePersistenceSource) withDefaults(backupName string) (changed bool) {
	changed = spec.BusyBoxImage.withDefaults(DefaultBusyBoxImageRepo, DefaultBusyBoxImageVersion, DefaultPullPolicy) || changed

	if spec.Path != "" && strings.HasPrefix(spec.Path, "/") {
		spec.Path = strings.TrimPrefix(spec.Path, "/")
		changed = true
	}

	if spec.Filename == "" {
		spec.Filename = backupName + ".tgz"
		changed = true
	}

	return changed
}

// SolrBackupStatus defines the observed state of SolrBackup
type SolrBackupStatus struct {
	// The current Backup Status, which all fields are added to this struct
	Current IndividualSolrBackupStatus `json:",inline"`

	// The scheduled time for the next backup to occur
	// +optional
	NextScheduledTime *metav1.Time `json:"nextScheduledTime,omitempty"`

	// The status history of recurring backups
	// +optional
	History []IndividualSolrBackupStatus `json:"history,omitempty"`
}

// IndividualSolrBackupStatus defines the observed state of a single issued SolrBackup
type IndividualSolrBackupStatus struct {
	// Version of the Solr being backed up
	SolrVersion string `json:"solrVersion"`

	// The time that this backup was initiated
	// +optional
	StartTime metav1.Time `json:"startTimestamp,omitempty"`

	// The status of each collection's backup progress
	// +optional
	CollectionBackupStatuses []CollectionBackupStatus `json:"collectionBackupStatuses,omitempty"`

	// Whether the backups are in progress of being persisted
	// +optional
	PersistenceStatus *BackupPersistenceStatus `json:"persistenceStatus,omitempty"`

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

	// BackupName of this collection's backup in Solr
	// +optional
	BackupName string `json:"backupName,omitempty"`

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

// PersistenceJobName returns the name of the persistence job for the backup
func (sb *SolrBackup) PersistenceJobName() string {
	return fmt.Sprintf("%s-solr-backup-persistence", sb.GetName())
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:storageversion
//+kubebuilder:categories=all
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cloud",type="string",JSONPath=".spec.solrCloud",description="Solr Cloud"
//+kubebuilder:printcolumn:name="Finished",type="boolean",JSONPath=".status.finished",description="Whether the backup has finished"
//+kubebuilder:printcolumn:name="Successful",type="boolean",JSONPath=".status.successful",description="Whether the backup was successful"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SolrBackup is the Schema for the solrbackups API
type SolrBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrBackupSpec   `json:"spec,omitempty"`
	Status SolrBackupStatus `json:"status,omitempty"`
}

// WithDefaults set default values when not defined in the spec.
func (sb *SolrBackup) WithDefaults() bool {
	return sb.Spec.withDefaults(sb.Name)
}

//+kubebuilder:object:root=true

// SolrBackupList contains a list of SolrBackup
type SolrBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrBackup{}, &SolrBackupList{})
}
