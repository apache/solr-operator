/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"testing"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/stretchr/testify/assert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

const testZkTarget = "zk-0:2181,zk-1:2181/my-chroot"

func cloudWithPlacement(class string, config string) *solr.SolrCloud {
	cloud := &solr.SolrCloud{}
	if class == "" {
		return cloud
	}
	cloud.Spec.ReplicaPlacementFactory = &solr.SolrReplicaPlacementFactory{Class: class}
	if config != "" {
		cloud.Spec.ReplicaPlacementFactory.Config = &apiextensionsv1.JSON{Raw: []byte(config)}
	}
	return cloud
}

func TestDesiredReplicaPlacementPluginNotConfigured(t *testing.T) {
	plugin, hash, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("", ""), testZkTarget)
	assert.NoError(t, err)
	assert.Nil(t, plugin, "No plugin should be desired when replicaPlacementFactory is unset")
	assert.Empty(t, hash, "The hash must be empty when no plugin is desired, so that no call to Solr is made")
}

func TestDesiredReplicaPlacementPluginUsesReservedName(t *testing.T) {
	plugin, hash, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", ""), testZkTarget)
	assert.NoError(t, err)
	assert.Equal(t, solr_api.PlacementPluginName, plugin.Name, "Solr requires the reserved placement plugin name")
	assert.Equal(t, "org.example.MyFactory", plugin.Class)
	assert.Nil(t, plugin.Config, "No config should be sent when none is configured")
	assert.NotEmpty(t, hash)
}

func TestDesiredReplicaPlacementPluginPassesConfigThrough(t *testing.T) {
	plugin, _, err := DesiredReplicaPlacementPlugin(
		cloudWithPlacement("org.example.MyFactory", `{"minimalFreeDiskGB":10,"spreadAcrossDomains":true,"withCollection":{"a":"b"}}`), testZkTarget)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"minimalFreeDiskGB":10,"spreadAcrossDomains":true,"withCollection":{"a":"b"}}`, string(*plugin.Config),
		"The config must reach Solr unchanged, including nested objects and non-string types")
}

func TestDesiredReplicaPlacementPluginHashIgnoresFormatting(t *testing.T) {
	_, hash1, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", `{"a":1,"b":2}`), testZkTarget)
	assert.NoError(t, err)
	_, hash2, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", "{\n  \"b\": 2,\n  \"a\": 1\n}"), testZkTarget)
	assert.NoError(t, err)
	assert.Equal(t, hash1, hash2, "Reformatting or reordering the config must not re-apply the plugin")

	_, hash3, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", `{"a":1,"b":3}`), testZkTarget)
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash3, "A changed config value must re-apply the plugin")

	_, hash4, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.OtherFactory", `{"a":1,"b":2}`), testZkTarget)
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash4, "A changed class must re-apply the plugin")
}

func TestDesiredReplicaPlacementPluginRejectsInvalidConfig(t *testing.T) {
	_, _, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", `{"a":`), testZkTarget)
	assert.Error(t, err, "Invalid JSON in the config must be reported rather than sent to Solr")
}

func TestDesiredReplicaPlacementPluginPreservesLargeIntegers(t *testing.T) {
	// 2^53+1 cannot be held exactly by a float64, so decoding through one would corrupt it
	plugin, _, err := DesiredReplicaPlacementPlugin(
		cloudWithPlacement("org.example.MyFactory", `{"big":9007199254740993}`), testZkTarget)
	assert.NoError(t, err)
	assert.Equal(t, `{"big":9007199254740993}`, string(*plugin.Config),
		"A large integer must reach Solr unchanged")

	_, hash1, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", `{"big":9007199254740993}`), testZkTarget)
	assert.NoError(t, err)
	_, hash2, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", `{"big":9007199254740992}`), testZkTarget)
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash2, "Two large integers that differ must not collide in the hash")
}

func TestDesiredReplicaPlacementPluginRejectsTrailingJson(t *testing.T) {
	_, _, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("org.example.MyFactory", `{"a":1} {"b":2}`), testZkTarget)
	assert.Error(t, err, "Data after the JSON value must be rejected rather than silently dropped")
}

func TestDesiredReplicaPlacementPluginHashCoversZkTarget(t *testing.T) {
	cloud := cloudWithPlacement("org.example.MyFactory", `{"a":1}`)

	_, hash1, err := DesiredReplicaPlacementPlugin(cloud, testZkTarget)
	assert.NoError(t, err)
	_, hash2, err := DesiredReplicaPlacementPlugin(cloud, "other-zk:2181/my-chroot")
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash2, "A different ZooKeeper ensemble must re-apply the plugin")

	_, hash3, err := DesiredReplicaPlacementPlugin(cloud, "zk-0:2181,zk-1:2181/other-chroot")
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash3, "A different chroot must re-apply the plugin")
}

func TestDesiredReplicaPlacementPluginUnsetIgnoresZkTarget(t *testing.T) {
	_, hash1, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("", ""), testZkTarget)
	assert.NoError(t, err)
	_, hash2, err := DesiredReplicaPlacementPlugin(cloudWithPlacement("", ""), "other-zk:2181/other")
	assert.NoError(t, err)
	assert.Empty(t, hash1)
	assert.Equal(t, hash1, hash2, "With no plugin configured the hash stays empty, so Solr is never called")
}
