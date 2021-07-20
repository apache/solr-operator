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

package util

import (
	solr "github.com/apache/solr-operator/api/v1beta1"
	zkv1beta1 "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestDefaultStorageOptions(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec:       solr.SolrCloudSpec{},
	}
	zkSpec := &solr.ZookeeperSpec{
		Persistence: nil,
		Ephemeral:   nil,
	}
	zkSpec.WithDefaults()

	var zkCluster *zkv1beta1.ZookeeperCluster

	// Solr uses nothing (defaults to ephemeral)
	zkCluster = GenerateZookeeperCluster(solrCloud, zkSpec)
	assert.Equal(t, "ephemeral", zkCluster.Spec.StorageType, "By default when no storage is specified for Solr or ZK, the storage should be ephemeral. Wrong storageType")
	assert.Nil(t, zkCluster.Spec.Persistence, "By default when no storage is specified for Solr or ZK, the storage should be ephemeral. Therefore 'persistence' should be nil")

	// Solr uses Persistent
	solrCloud.Spec.StorageOptions.PersistentStorage = &solr.SolrPersistentDataStorageOptions{}
	solrCloud.Spec.StorageOptions.EphemeralStorage = nil
	zkCluster = GenerateZookeeperCluster(solrCloud, zkSpec)
	assert.Equal(t, "persistence", zkCluster.Spec.StorageType, "By default when Solr is using persistent storage, zk should as well. Wrong storageType")
	assert.Nil(t, zkCluster.Spec.Ephemeral, "By default when Solr is using persistent storage, zk should as well. Therefore 'ephemeral' should be nil")

	// Solr uses Ephemeral
	solrCloud.Spec.StorageOptions.PersistentStorage = nil
	solrCloud.Spec.StorageOptions.EphemeralStorage = &solr.SolrEphemeralDataStorageOptions{}
	zkCluster = GenerateZookeeperCluster(solrCloud, zkSpec)
	assert.Equal(t, "ephemeral", zkCluster.Spec.StorageType, "By default when Solr is using ephemeral storage, zk should as well. Wrong storageType")
	assert.Nil(t, zkCluster.Spec.Persistence, "By default when Solr is using ephemeral storage, zk should as well. Therefore 'persistence' should be nil")
}
