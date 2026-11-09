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
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
)

// MinSolrVersionForClusterPlugins is the first Solr major version with the /api/cluster/plugin API.
const MinSolrVersionForClusterPlugins = 9

// DesiredReplicaPlacementPlugin returns the cluster plugin that the SolrCloud's spec asks for, and a
// hash identifying it. Both are empty when the SolrCloud configures no placement plugin.
//
// zkTarget identifies the ZooKeeper ensemble and chroot that the plugin would be applied to. It is
// part of the hash because cluster plugins live in ZooKeeper, so pointing the SolrCloud at a
// different target means the plugin has to be applied again.
func DesiredReplicaPlacementPlugin(solrCloud *solr.SolrCloud, zkTarget string) (plugin *solr_api.SolrClusterPlugin, hash string, err error) {
	if solrCloud.Spec.ReplicaPlacementFactory == nil {
		return nil, "", nil
	}

	plugin = &solr_api.SolrClusterPlugin{
		Name:  solr_api.PlacementPluginName,
		Class: solrCloud.Spec.ReplicaPlacementFactory.Class,
	}
	if config := solrCloud.Spec.ReplicaPlacementFactory.Config; config != nil && len(config.Raw) > 0 {
		var normalized []byte
		if normalized, err = normalizeConfigJson(config.Raw); err != nil {
			return nil, "", err
		}
		rawMessage := json.RawMessage(normalized)
		plugin.Config = &rawMessage
	}

	identity := struct {
		Plugin   *solr_api.SolrClusterPlugin `json:"plugin"`
		ZkTarget string                      `json:"zkTarget"`
	}{plugin, zkTarget}

	var encoded []byte
	if encoded, err = json.Marshal(identity); err != nil {
		return nil, "", err
	}
	return plugin, fmt.Sprintf("%x", md5.Sum(encoded)), nil
}

// normalizeConfigJson re-encodes the config so that the hash does not change with insignificant
// formatting or key ordering. Numbers keep their original literal, so values that a float64 cannot
// hold exactly reach Solr unchanged.
func normalizeConfigJson(raw []byte) (normalized []byte, err error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value interface{}
	if err = decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("replicaPlacementFactory.config is not valid JSON: %w", err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("replicaPlacementFactory.config is not valid JSON: unexpected data after the JSON value")
	}
	return json.Marshal(value)
}

// ReconcileReplicaPlacementPlugin brings the SolrCloud's replica placement plugin in line with its
// spec, by calling Solr's cluster plugin API. It is a no-op unless the desired state differs from
// what the operator last applied, recorded in appliedHash, so a SolrCloud that configures no
// placement plugin never results in a call to Solr.
//
// The caller must ensure the cloud is reachable and runs at least
// MinSolrVersionForClusterPlugins. The returned hash is what should be stored for the next
// comparison.
func ReconcileReplicaPlacementPlugin(ctx context.Context, solrCloud *solr.SolrCloud, zkTarget string, appliedHash string, logger logr.Logger) (newHash string, err error) {
	desiredPlugin, desiredHash, err := DesiredReplicaPlacementPlugin(solrCloud, zkTarget)
	if err != nil {
		return appliedHash, err
	}

	if desiredHash == appliedHash {
		return appliedHash, nil
	}

	plugins, err := solr_api.GetClusterPlugins(ctx, solrCloud)
	if err != nil {
		return appliedHash, err
	}
	_, exists := plugins[solr_api.PlacementPluginName]

	if desiredPlugin == nil {
		if !exists {
			return "", nil
		}
		if err = solr_api.RemoveClusterPlugin(ctx, solrCloud, solr_api.PlacementPluginName); err != nil {
			return appliedHash, err
		}
		logger.Info("Removed the replica placement plugin from the Solr cluster")
		return "", nil
	}

	if err = solr_api.SetClusterPlugin(ctx, solrCloud, *desiredPlugin, exists); err != nil {
		return appliedHash, err
	}
	logger.Info("Applied the replica placement plugin to the Solr cluster", "class", desiredPlugin.Class, "updated", exists)
	return desiredHash, nil
}
