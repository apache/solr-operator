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
)

// SolrCollectionAliasSpec defines the desired state of SolrCollectionAlias
type SolrCollectionAliasSpec struct {
	// A reference to the SolrCloud to create alias on
	SolrCloud string `json:"solrCloud"`

	// Collections is a list of collections to apply alias to
	Collections []string `json:"collections"`

	// AliasType is a either standard or routed, right now we support standard
	AliasType string `json:"aliasType"`
}

// SolrCollectionAliasStatus defines the observed state of SolrCollectionAlias
type SolrCollectionAliasStatus struct {
	// Time the alias was created
	// +optional
	CreatedTime *metav1.Time `json:"createdTime,omitempty"`

	// Created or not status
	// +optional
	Created bool `json:"created,omitempty"`

	// Associated collections to the alias
	// +optional
	Collections []string `json:"collections,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced

// SolrCollectionAlias is the Schema for the solrcollectionaliases API
type SolrCollectionAlias struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrCollectionAliasSpec   `json:"spec,omitempty"`
	Status SolrCollectionAliasStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SolrCollectionAliasList contains a list of SolrCollectionAlias
type SolrCollectionAliasList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrCollectionAlias `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrCollectionAlias{}, &SolrCollectionAliasList{})
}
