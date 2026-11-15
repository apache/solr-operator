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

package solr_api

import (
	"context"
	"encoding/json"

	solr "github.com/apache/solr-operator/api/v1beta1"
)

// PlacementPluginName is the reserved cluster plugin name that Solr requires for the
// cluster-wide replica placement plugin.
const PlacementPluginName = ".placement-plugin"

const clusterPluginApiPath = "/api/cluster/plugin"

type SolrClusterPlugin struct {
	Name   string           `json:"name"`
	Class  string           `json:"class"`
	Config *json.RawMessage `json:"config,omitempty"`
}

type SolrClusterPluginsResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// Plugins are keyed by plugin name, under the "plugin" field that Solr wraps them in
	Plugins map[string]SolrClusterPlugin `json:"plugin,omitempty"`

	Error *SolrErrorResponse `json:"error,omitempty"`
}

type SolrClusterPluginCommand struct {
	Add    *SolrClusterPlugin `json:"add,omitempty"`
	Update *SolrClusterPlugin `json:"update,omitempty"`
	Remove *string            `json:"remove,omitempty"`
}

// GetClusterPlugins returns the cluster plugins currently registered with the SolrCloud.
func GetClusterPlugins(ctx context.Context, cloud *solr.SolrCloud) (plugins map[string]SolrClusterPlugin, err error) {
	response := &SolrClusterPluginsResponse{}
	if err = CallCollectionsApiV2(ctx, cloud, "GET", clusterPluginApiPath, nil, nil, response); err != nil {
		return nil, err
	}
	if _, apiErr := CheckForCollectionsApiError("GET cluster plugins", response.ResponseHeader, response.Error); apiErr != nil {
		return nil, apiErr
	}
	return response.Plugins, nil
}

// SetClusterPlugin registers the given plugin with the SolrCloud, updating it if a plugin of the
// same name is already registered.
func SetClusterPlugin(ctx context.Context, cloud *solr.SolrCloud, plugin SolrClusterPlugin, alreadyExists bool) (err error) {
	command := SolrClusterPluginCommand{}
	if alreadyExists {
		command.Update = &plugin
	} else {
		command.Add = &plugin
	}
	return postClusterPluginCommand(ctx, cloud, command, "set cluster plugin "+plugin.Name)
}

// RemoveClusterPlugin unregisters the named plugin from the SolrCloud.
func RemoveClusterPlugin(ctx context.Context, cloud *solr.SolrCloud, name string) (err error) {
	return postClusterPluginCommand(ctx, cloud, SolrClusterPluginCommand{Remove: &name}, "remove cluster plugin "+name)
}

func postClusterPluginCommand(ctx context.Context, cloud *solr.SolrCloud, command SolrClusterPluginCommand, operation string) (err error) {
	response := &SolrAsyncResponse{}
	if err = CallCollectionsApiV2(ctx, cloud, "POST", clusterPluginApiPath, nil, command, response); err != nil {
		return err
	}
	_, apiErr := CheckForCollectionsApiError(operation, response.ResponseHeader, response.Error)
	return apiErr
}
