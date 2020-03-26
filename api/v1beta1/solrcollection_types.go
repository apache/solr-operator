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

// SolrCollectionSpec defines the desired state of SolrCollection
type SolrCollectionSpec struct {
	// A reference to the SolrCloud to create a collection for
	SolrCloud string `json:"solrCloud"`

	// The name of the collection to perform the action on
	Collection string `json:"collection"`

	// Define a configset to use for the collection. Use '_default' if you don't have a custom configset
	CollectionConfigName string `json:"collectionConfigName"`

	// The router name that will be used. The router defines how documents will be distributed
	// +optional
	RouterName CollectionRouterName `json:"routerName,omitempty"`

	// If this parameter is specified, the router will look at the value of the field in an input document
	// to compute the hash and identify a shard instead of looking at the uniqueKey field.
	// If the field specified is null in the document, the document will be rejected.
	// +optional
	RouterField string `json:"routerField,omitempty"`

	// The num of shards to create, used if RouteName is compositeId
	// +optional
	NumShards int64 `json:"numShards,omitempty"`

	// The replication factor to be used
	// +optional
	ReplicationFactor int64 `json:"replicationFactor,omitempty"`

	// Max shards per node
	// +optional
	MaxShardsPerNode int64 `json:"maxShardsPerNode,omitempty"`

	// A comma separated list of shard names, e.g., shard-x,shard-y,shard-z. This is a required parameter when the router.name is implicit
	// +optional
	Shards string `json:"shards,omitempty"`

	// When set to true, enables automatic addition of replicas when the number of active replicas falls below the value set for replicationFactor
	// +optional
	AutoAddReplicas bool `json:"autoAddReplicas,omitempty"`
}

// CollectionRouterName is a string enumeration type that enumerates the ways that documents can be routed for a collection.
// +kubebuilder:validation:Enum=implicit;compositeId
type CollectionRouterName string

const (
	// The Implicit router
	ImplicitRouter CollectionRouterName = "implicit"

	// The CompositeId router
	CompositeIdRouter CollectionRouterName = "compositeId"
)

// SolrCollectionStatus defines the observed state of SolrCollection
type SolrCollectionStatus struct {
	// Whether the collection has been created or not
	// +optional
	Created bool `json:"created,omitempty"`

	// Time the collection was created
	// +optional
	CreatedTime *metav1.Time `json:"createdTime,omitempty"`

	// Set the status of the collection creation process
	// +optional
	InProgressCreation bool `json:"inProgressCreation,omitempty"`
}

// +kubebuilder:object:root=true

// SolrCollection is the Schema for the solrcollections API
type SolrCollection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrCollectionSpec   `json:"spec,omitempty"`
	Status SolrCollectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SolrCollectionList contains a list of SolrCollection
type SolrCollectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrCollection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrCollection{}, &SolrCollectionList{})
}
