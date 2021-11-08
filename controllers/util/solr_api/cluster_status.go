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

package solr_api

import "k8s.io/apimachinery/pkg/util/intstr"

type SolrOverseerStatusResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	Leader string `json:"leader,omitempty"`

	// +optional
	QueueSize int `json:"overseer_queue_size,omitempty"`

	// +optional
	WorkQueueSize int `json:"overseer_work_queue_size,omitempty"`

	// +optional
	CollectionQueueSize int `json:"overseer_collection_queue_size,omitempty"`
}

type SolrClusterStatusResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	ClusterStatus SolrClusterStatus `json:"cluster,omitempty"`
}

type SolrClusterStatus struct {
	// +optional
	Collections map[string]SolrCollectionStatus `json:"collections,omitempty"`

	// +optional
	Aliases map[string]string `json:"aliases,omitempty"`

	// +optional
	Roles map[string][]string `json:"roles,omitempty"`

	// +optional
	LiveNodes []string `json:"live_nodes,omitempty"`
}

type SolrCollectionStatus struct {
	// +optional
	Shards map[string]SolrShardStatus `json:"shards,omitempty"`

	// +optional
	ConfigName string `json:"configName,omitempty"`

	// +optional
	ZnodeVersion intstr.IntOrString `json:"znodeVersion,omitempty"`

	// +optional
	AutoAddReplicas string `json:"autoAddReplicas,omitempty"`

	// +optional
	NrtReplicas intstr.IntOrString `json:"nrtReplicas,omitempty"`

	// +optional
	TLogReplicas intstr.IntOrString `json:"tlogReplicas,omitempty"`

	// +optional
	PullReplicas intstr.IntOrString `json:"pullReplicas,omitempty"`

	// +optional
	MaxShardsPerNode intstr.IntOrString `json:"maxShardsPerNode,omitempty"`

	// +optional
	ReplicationFactor intstr.IntOrString `json:"replicationFactor,omitempty"`

	// +optional
	Router SolrCollectionRouter `json:"router,omitempty"`
}

type SolrCollectionRouter struct {
	Name CollectionRouterName `json:"name"`
}

type SolrShardStatus struct {
	// +optional
	Replicas map[string]SolrReplicaStatus `json:"replicas,omitempty"`

	// +optional
	Range string `json:"range,omitempty"`

	// +optional
	State SolrShardState `json:"state,omitempty"`
}

type SolrShardState string

const (
	ShardActive SolrShardState = "active"
	ShardDown   SolrShardState = "down"
)

type SolrReplicaStatus struct {
	State SolrReplicaState `json:"state"`

	Core string `json:"core"`

	NodeName string `json:"node_name"`

	BaseUrl string `json:"base_url"`

	Leader bool `json:"leader,string"`

	// +optional
	Type SolrReplicaType `json:"type,omitempty"`
}

type SolrReplicaState string

const (
	ReplicaActive         SolrReplicaState = "active"
	ReplicaDown           SolrReplicaState = "down"
	ReplicaRecovering     SolrReplicaState = "recovering"
	ReplicaRecoveryFailed SolrReplicaState = "recovery_failed"
)

type SolrReplicaType string

const (
	NRT  SolrReplicaType = "NRT"
	TLOG SolrReplicaType = "TLOG"
	PULL SolrReplicaType = "PULL"
)

// CollectionRouterName is a string enumeration type that enumerates the ways that documents can be routed for a collection.
type CollectionRouterName string

const (
	ImplicitRouter    CollectionRouterName = "implicit"
	CompositeIdRouter CollectionRouterName = "compositeId"
)
