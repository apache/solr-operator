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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SolrBackupSpec defines the desired state of SolrBackup
type SolrBackupSpec struct {
	// A reference to the SolrCloud to create a backup for
	//
	// +kubebuilder:validation:Pattern:=[a-z0-9]([-a-z0-9]*[a-z0-9])?
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=63
	SolrCloud string `json:"solrCloud"`

	// The name of the repository to use for the backup.  Defaults to "legacy_local_repository" if not specified (the
	// auto-configured repository for legacy singleton volumes).
	//
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=100
	// +optional
	RepositoryName string `json:"repositoryName,omitempty"`

	// The list of collections to backup.
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
}

func (spec *SolrBackupSpec) withDefaults() (changed bool) {
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
	// If not provided, this defaults to 5.
	//
	// +kubebuilder:default:=5
	// +kubebuilder:validation:Minimum:=1
	// +optional
	MaxSaved int `json:"maxSaved,omitempty"`

	// Disable the recurring backups. Note this will not affect any currently-running backup.
	//
	// +kubebuilder:default:=false
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

func (recurrence *BackupRecurrence) IsEnabled() bool {
	return recurrence != nil && !recurrence.Disabled
}

// SolrBackupStatus defines the observed state of SolrBackup
type SolrBackupStatus struct {
	// The current Backup Status, which all fields are added to this struct
	IndividualSolrBackupStatus `json:",inline"`

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
	// +optional
	SolrVersion string `json:"solrVersion,omitempty"`

	// The time that this backup was initiated
	// +optional
	StartTime metav1.Time `json:"startTimestamp,omitempty"`

	// The status of each collection's backup progress
	// +optional
	CollectionBackupStatuses []CollectionBackupStatus `json:"collectionBackupStatuses,omitempty"`

	// The time that this backup was finished
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
//+kubebuilder:printcolumn:name="Started",type="date",JSONPath=".status.startTimestamp",description="Most recent time the backup started"
//+kubebuilder:printcolumn:name="Finished",type="boolean",JSONPath=".status.finished",description="Whether the most recent backup has finished"
//+kubebuilder:printcolumn:name="Successful",type="boolean",JSONPath=".status.successful",description="Whether the most recent backup was successful"
//+kubebuilder:printcolumn:name="NextBackup",type="string",JSONPath=".status.nextScheduledTime",description="Next scheduled time for a recurrent backup",format="date-time"
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
	return sb.Spec.withDefaults()
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
