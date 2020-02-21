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

package util

import (
	"net/url"
	"strconv"
	"strings"
)

// CreateCollection to request collection creation on SolrCloud
func CreateCollection(cloud string, collection string, numShards int64, replicationFactor int64, autoAddReplicas bool, maxShardsPerNode int64, routerName string, routerField string, shards string, collectionConfigName string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	replicationFactorParameter := strconv.FormatInt(replicationFactor, 10)
	numShardsParameter := strconv.FormatInt(numShards, 10)
	maxShardsPerNodeParameter := strconv.FormatInt(maxShardsPerNode, 10)
	queryParams.Add("action", "CREATE")
	queryParams.Add("name", collection)
	queryParams.Add("replicationFactor", replicationFactorParameter)
	queryParams.Add("maxShardsPerNode", maxShardsPerNodeParameter)
	queryParams.Add("autoAddReplicas", strconv.FormatBool(autoAddReplicas))
	queryParams.Add("collection.configName", collectionConfigName)
	queryParams.Add("router.field", routerField)

	if routerName == "implicit" {
		queryParams.Add("router.name", routerName)
		queryParams.Add("shards", shards)
	} else if routerName == "compositeId" {
		queryParams.Add("router.name", routerName)
		queryParams.Add("numShards", numShardsParameter)
	} else {
		log.Info("router.name must be either compositeId or implicit. Provided: ", routerName)
	}

	resp := &SolrAsyncResponse{}

	log.Info("Calling to create collection", "namespace", namespace, "cloud", cloud, "collection", collection)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			success = true
		}
	} else {
		log.Error(err, "Error creating collection", "namespace", namespace, "cloud", cloud, "collection", collection)
	}

	return success, err
}

// CreateCollectionAlias to request the creation of an alias to one or more collections
func CreateCollectionAlias(cloud string, alias string, aliasType string, collections []string, namespace string) (err error) {
	queryParams := url.Values{}
	collectionsArray := strings.Join(collections, ",")
	queryParams.Add("action", "CREATEALIAS")
	queryParams.Add("name", alias)
	queryParams.Add("collections", collectionsArray)

	resp := &SolrAsyncResponse{}

	log.Info("Calling to create alias", "namespace", namespace, "cloud", cloud, "alias", alias, "to collections", collectionsArray)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			log.Info("Created alias", "alias", alias, "to collection", collectionsArray)
		}
	} else {
		log.Error(err, "Error creating alias", "namespace", namespace, "cloud", cloud, "alias", alias, "to collections", collectionsArray)
	}

	return err

}

// DeleteCollection to request collection deletion on SolrCloud
func DeleteCollection(cloud string, collection string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "DELETE")
	queryParams.Add("name", collection)

	resp := &SolrAsyncResponse{}

	log.Info("Calling to delete collection", "namespace", namespace, "cloud", cloud, "collection", collection)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			success = true
		}
	} else {
		log.Error(err, "Error deleting collection", "namespace", namespace, "cloud", cloud, "collection")
	}

	return success, err
}

// DeleteCollectionAlias removes an alias
func DeleteCollectionAlias(cloud string, alias string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "DELETEALIAS")
	queryParams.Add("name", alias)

	resp := &SolrAsyncResponse{}

	log.Info("Calling to delete collection alias", "namespace", namespace, "cloud", cloud, "alias", alias)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			success = true
		}
	} else {
		log.Error(err, "Error deleting collection alias", "namespace", namespace, "cloud", cloud, "alias", alias)
	}

	return success, err
}

// ModifyCollection to request collection modification on SolrCloud.
func ModifyCollection(cloud string, collection string, replicationFactor int64, autoAddReplicas bool, maxShardsPerNode int64, collectionConfigName string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	replicationFactorParameter := strconv.FormatInt(replicationFactor, 10)
	maxShardsPerNodeParameter := strconv.FormatInt(maxShardsPerNode, 10)
	queryParams.Add("action", "MODIFYCOLLECTION")
	queryParams.Add("collection", collection)
	queryParams.Add("replicationFactor", replicationFactorParameter)
	queryParams.Add("maxShardsPerNode", maxShardsPerNodeParameter)
	queryParams.Add("autoAddReplicas", strconv.FormatBool(autoAddReplicas))
	queryParams.Add("collection.configName", collectionConfigName)

	resp := &SolrAsyncResponse{}

	log.Info("Calling to modify collection", "namespace", namespace, "cloud", cloud, "collection", collection)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			success = true
		}
	} else {
		log.Error(err, "Error modifying collection", "namespace", namespace, "cloud", cloud, "collection")
	}

	return success, err
}

// CheckIfCollectionModificationRequired to check if the collection's modifiable parameters have changed in spec and need to be updated
func CheckIfCollectionModificationRequired(cloud string, collection string, replicationFactor int64, autoAddReplicas bool, maxShardsPerNode int64, collectionConfigName string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	replicationFactorParameter := strconv.FormatInt(replicationFactor, 10)
	maxShardsPerNodeParameter := strconv.FormatInt(maxShardsPerNode, 10)
	autoAddReplicasParameter := strconv.FormatBool(autoAddReplicas)
	success = false
	queryParams.Add("action", "CLUSTERSTATUS")
	queryParams.Add("collection", collection)

	resp := &SolrClusterStatusResponse{}

	err = CallCollectionsApi(cloud, namespace, queryParams, &resp)

	if collectionResp, ok := resp.Cluster.Collections[collection].(map[string]interface{}); ok {
		// Check modifiable collection parameters
		if collectionResp["autoAddReplicas"] != autoAddReplicasParameter {
			log.Info("Collection modification required, autoAddReplicas changed", "autoAddReplicas", autoAddReplicasParameter)
			success = true
		}

		if collectionResp["maxShardsPerNode"] != maxShardsPerNodeParameter {
			log.Info("Collection modification required, maxShardsPerNode changed", "maxShardsPerNode", maxShardsPerNodeParameter)
			success = true
		}

		if collectionResp["replicationFactor"] != replicationFactorParameter {
			log.Info("Collection modification required, replicationFactor changed", "replicationFactor", replicationFactorParameter)
			success = true
		}
		if collectionResp["configName"] != collectionConfigName {
			log.Info("Collection modification required, configName changed", "configName", collectionConfigName)
			success = true
		}
	} else {
		log.Error(err, "Error calling collection API status", "namespace", namespace, "cloud", cloud, "collection", collection)
	}

	return success, err
}

// CheckIfCollectionExists to request if collection exists in list of collection
func CheckIfCollectionExists(cloud string, collection string, namespace string) (success bool) {
	queryParams := url.Values{}
	queryParams.Add("action", "LIST")

	resp := &SolrCollectionsListResponse{}

	log.Info("Calling to list collections", "namespace", namespace, "cloud", cloud, "collection", collection)
	err := CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if containsCollection(resp.Collections, collection) {
			success = true
		}
	} else {
		log.Error(err, "Error listing collections", "namespace", namespace, "cloud", cloud, "collection")
	}

	return success
}

// CurrentCollectionAliasDetails will return a success if details found for an alias and comma separated string of associated collections
func CurrentCollectionAliasDetails(cloud string, alias string, namespace string) (success bool, collections string) {
	queryParams := url.Values{}
	queryParams.Add("action", "LISTALIASES")

	resp := &SolrCollectionAliasDetailsResponse{}

	err := CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		success, collections := containsAlias(resp.Aliases, alias)
		if success {
			return success, collections
		}
	}

	return success, ""
}

type SolrCollectionAliasDetailsResponse struct {
	SolrResponseHeader SolrCollectionResponseHeader `json:"responseHeader"`

	// +optional
	Aliases map[string]string `json:"aliases"`
}

type SolrCollectionResponseHeader struct {
	Status int `json:"status"`

	QTime int `json:"QTime"`
}

type SolrCollectionAsyncStatus struct {
	AsyncState string `json:"state"`

	Message string `json:"msg"`
}

type SolrCollectionsListResponse struct {
	ResponseHeader SolrCollectionResponseHeader `json:"responseHeader"`

	// +optional
	RequestId string `json:"requestId"`

	// +optional
	Status SolrCollectionAsyncStatus `json:"status"`

	Collections []string `json:"collections"`
}

type SolrClusterStatusResponse struct {
	ResponseHeader SolrCollectionResponseHeader `json:"responseHeader"`

	Cluster SolrClusterStatusCluster `json:"cluster"`
}

type SolrClusterStatusCluster struct {
	Collections map[string]interface{} `json:"collections"`
}

// ContainsString helper function to test string contains
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString helper function to remove string
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// containsCollection helper function to check if collection in list
func containsCollection(collections []string, collection string) bool {
	for _, a := range collections {
		if a == collection {
			return true
		}
	}
	return false
}

// containsAlias helper function to check if alias defined and return success and associated collections
func containsAlias(aliases map[string]string, alias string) (success bool, collections string) {
	for k, v := range aliases {
		if k == alias {
			return true, v
		}
	}
	return false, ""
}
