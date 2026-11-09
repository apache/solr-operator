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
func DesiredReplicaPlacementPlugin(solrCloud *solr.SolrCloud) (plugin *solr_api.SolrClusterPlugin, hash string, err error) {
	if solrCloud.Spec.ReplicaPlacementFactory == nil {
		return nil, "", nil
	}

	plugin = &solr_api.SolrClusterPlugin{
		Name:  solr_api.PlacementPluginName,
		Class: solrCloud.Spec.ReplicaPlacementFactory.Class,
	}
	if config := solrCloud.Spec.ReplicaPlacementFactory.Config; config != nil && len(config.Raw) > 0 {
		// Round-trip the config so that the hash does not change with insignificant formatting
		var normalized interface{}
		if err = json.Unmarshal(config.Raw, &normalized); err != nil {
			return nil, "", fmt.Errorf("replicaPlacementFactory.config is not valid JSON: %w", err)
		}
		var raw []byte
		if raw, err = json.Marshal(normalized); err != nil {
			return nil, "", err
		}
		rawMessage := json.RawMessage(raw)
		plugin.Config = &rawMessage
	}

	var encoded []byte
	if encoded, err = json.Marshal(plugin); err != nil {
		return nil, "", err
	}
	return plugin, fmt.Sprintf("%x", md5.Sum(encoded)), nil
}

// ReconcileReplicaPlacementPlugin brings the SolrCloud's replica placement plugin in line with its
// spec, by calling Solr's cluster plugin API. It is a no-op unless the desired state differs from
// what the operator last applied, recorded in appliedHash, so a SolrCloud that configures no
// placement plugin never results in a call to Solr.
//
// The caller must ensure the cloud is reachable and runs at least
// MinSolrVersionForClusterPlugins. The returned hash is what should be stored for the next
// comparison.
func ReconcileReplicaPlacementPlugin(ctx context.Context, solrCloud *solr.SolrCloud, appliedHash string, logger logr.Logger) (newHash string, err error) {
	desiredPlugin, desiredHash, err := DesiredReplicaPlacementPlugin(solrCloud)
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
