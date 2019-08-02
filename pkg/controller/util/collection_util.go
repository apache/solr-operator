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
)

// CreateCollection to request collection creation on SolrCloud
func CreateCollection(cloud string, collection string, numShards int64, replicationFactor int64, autoAddReplicas bool, routerName string, shards string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	replicationFactorParameter := strconv.FormatInt(replicationFactor, 10)
	numShardsParameter := strconv.FormatInt(numShards, 10)
	queryParams.Add("action", "CREATE")
	queryParams.Add("name", collection)
	queryParams.Add("replicationFactor", replicationFactorParameter)
	queryParams.Add("autoAddReplicas", strconv.FormatBool(autoAddReplicas))

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

// ModifyCollection to request collection modification on SolrCloud.
func ModifyCollection(cloud string, collection string, replicationFactor int64, autoAddReplicas bool, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	replicationFactorParameter := strconv.FormatInt(replicationFactor, 10)
	queryParams.Add("action", "MODIFYCOLLECTION")
	queryParams.Add("collection", collection)
	queryParams.Add("replicationFactor", replicationFactorParameter)
	queryParams.Add("autoAddReplicas", strconv.FormatBool(autoAddReplicas))

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
