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
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"strconv"
	"testing"
	"time"
)

func TestPickPodsToUpgrade(t *testing.T) {
	log := ctrl.Log

	overseerLeader := "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr"

	maxshardReplicasUnavailable := intstr.FromInt(1)

	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			Replicas: Replicas(6),
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
			UpdateStrategy: solr.SolrUpdateStrategy{
				Method: solr.ManagedUpdate,
				ManagedUpdateOptions: solr.ManagedUpdateOptions{
					MaxShardReplicasUnavailable: &maxshardReplicasUnavailable,
				},
			},
		},
	}

	allPods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-0"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-1"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-2"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-3"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-4"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-5"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-6"}, Spec: corev1.PodSpec{}},
	}

	halfPods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-0"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-1"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-3"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-5"}, Spec: corev1.PodSpec{}},
	}

	lastPod := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-0"}, Spec: corev1.PodSpec{}},
	}

	/*
		Test upgrades with down replicas (which do not affect what nodes can be upgraded)
	*/

	// Normal inputs
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgradeDetailed, podsHaveReplicas := pickPodsToUpdate(solrCloud, allPods, testDownClusterStatus, overseerLeader, 6, log)
	assert.Equal(t, podsHaveReplicas, map[string]bool{"foo-solrcloud-2": true, "foo-solrcloud-6": false})
	podsToUpgrade := getPodNames(podsToUpgradeDetailed)
	assert.ElementsMatch(t, []string{"foo-solrcloud-2", "foo-solrcloud-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Do to the down/non-live replicas, only the node without replicas and one more can be upgraded.")

	// Test the maxBatchNodeUpgradeSpec
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgradeDetailed, podsHaveReplicas = pickPodsToUpdate(solrCloud, allPods, testDownClusterStatus, overseerLeader, 1, log)
	assert.Equal(t, podsHaveReplicas, map[string]bool{"foo-solrcloud-6": false})
	podsToUpgrade = getPodNames(podsToUpgradeDetailed)
	assert.ElementsMatch(t, []string{"foo-solrcloud-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Only 1 node should be upgraded when maxBatchNodeUpgradeSpec=1")

	// Test the maxShardReplicasDownSpec
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgradeDetailed, podsHaveReplicas = pickPodsToUpdate(solrCloud, allPods, testDownClusterStatus, overseerLeader, 6, log)
	assert.Equal(t, podsHaveReplicas, map[string]bool{"foo-solrcloud-2": true, "foo-solrcloud-3": true, "foo-solrcloud-4": true, "foo-solrcloud-6": false})
	podsToUpgrade = getPodNames(podsToUpgradeDetailed)
	assert.ElementsMatch(t, []string{"foo-solrcloud-2", "foo-solrcloud-3", "foo-solrcloud-4", "foo-solrcloud-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade.")

	/*
		Test upgrades with replicas in recovery (which are treated as "active" when calculating how many nodes can be taken down) and a non-live node.
	*/

	// Normal inputs
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, allPods, testRecoveringClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-4", "foo-solrcloud-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Do to the recovering/down/non-live replicas, only the non-live node and node without replicas can be upgraded.")

	// Test the maxBatchNodeUpgradeSpec
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, allPods, testRecoveringClusterStatus, overseerLeader, 1, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-4"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Only 1 node should be upgraded when maxBatchNodeUpgradeSpec=1, and it should be the non-live node.")

	// Test the maxShardReplicasDownSpec
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, allPods, testRecoveringClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-2", "foo-solrcloud-3", "foo-solrcloud-4", "foo-solrcloud-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. More nodes should be upgraded when maxShardReplicasDown=2")

	// The overseer should be upgraded when given enough leeway
	maxshardReplicasUnavailable = intstr.FromString("50%")
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testDownClusterStatus, overseerLeader, 2, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The last pod, the overseer, should be chosen because it has been given enough leeway.")

	/*
		Test upgrades with a healthy cluster state.
	*/

	// Normal inputs
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, halfPods, testHealthyClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-1"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Do to replica placement, only the node with the least leaders can be upgraded and replicas.")

	// Test the maxShardReplicasDownSpec
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, halfPods, testHealthyClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-1", "foo-solrcloud-5"}, podsToUpgrade, "Incorrect set of next pods to upgrade. More nodes should be upgraded when maxShardReplicasDown=2")

	// The overseer should be upgraded when given enough leeway
	maxshardReplicasUnavailable = intstr.FromString("50%")
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testDownClusterStatus, overseerLeader, 2, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The last pod, the overseer, should be chosen because it has been given enough leeway.")

	/*
		Test the overseer node being taken down.
	*/

	// The overseer should be not be upgraded if the clusterstate is not healthy enough
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testRecoveringClusterStatus, overseerLeader, 3, log))
	assert.ElementsMatch(t, []string{}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be not be upgraded if the clusterstate is not healthy enough.")

	// The overseer should be not be upgraded if the clusterstate is not healthy enough
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testRecoveringClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be not be upgraded if there are other non-live nodes.")

	// The overseer should be upgraded when given enough leeway
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testDownClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be upgraded when given enough leeway.")

	// The overseer should be upgraded when everything is healthy and it is the last node
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testHealthyClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be upgraded when everything is healthy and it is the last node")

	/*
		Test the overseer node being taken down when there might be nodes managed outside of this SolrCloud resource
	*/

	// The overseer should not be upgraded when everything is healthy and it is the last node but one pod is not in the live nodes
	maxshardReplicasUnavailable = intstr.FromInt(1)
	solrCloud.Spec.Replicas = Replicas(7)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testHealthyClusterStatus, overseerLeader, 6, log))
	assert.Empty(t, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be not be upgraded when one of the managed pods is not live")

	// The overseer should be upgraded when everything is healthy and it is the last node even though this SolrCloud resource doesn't manage all Nodes
	maxshardReplicasUnavailable = intstr.FromInt(1)
	solrCloud.Spec.Replicas = Replicas(4)
	podsToUpgrade = getPodNames(pickPodsToUpdate(solrCloud, lastPod, testHealthyClusterStatus, overseerLeader, 6, log))
	assert.ElementsMatch(t, []string{"foo-solrcloud-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be upgraded when everything is healthy and it is the last node, even though this SolrCloud resource doesn't manage all Nodes")
}

func TestPodUpgradeOrdering(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
		},
	}

	pods := make([]corev1.Pod, 13)
	for i := range pods {
		pods[i] = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-" + strconv.Itoa(i)}, Spec: corev1.PodSpec{}}
	}

	nodeMap := map[string]*SolrNodeContents{
		// This node should be last as it is the overseer
		SolrNodeName(solrCloud, pods[0].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[0].Name),
			leaders:         4,
			replicas:        10,
			notDownReplicas: 10,
			overseerLeader:  true,
			live:            true,
		},
		SolrNodeName(solrCloud, pods[1].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[1].Name),
			leaders:         8,
			replicas:        20,
			notDownReplicas: 20,
			overseerLeader:  false,
			live:            false,
		},
		SolrNodeName(solrCloud, pods[2].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[2].Name),
			leaders:         0,
			replicas:        16,
			notDownReplicas: 16,
			overseerLeader:  false,
			live:            true,
		},
		SolrNodeName(solrCloud, pods[3].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[3].Name),
			leaders:         3,
			replicas:        10,
			notDownReplicas: 10,
			overseerLeader:  false,
			live:            true,
		},
		// This node should come second to last as it is not the overseer, but it has the most leaders.
		SolrNodeName(solrCloud, pods[4].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[4].Name),
			leaders:         10,
			replicas:        10,
			notDownReplicas: 10,
			overseerLeader:  false,
			live:            true,
		},
		// This node should come after pod 3 since they are identically ordered, but the name foo-solrcloud-3 comes before foo-solrcloud-5.
		SolrNodeName(solrCloud, pods[5].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[5].Name),
			leaders:         3,
			replicas:        12,
			notDownReplicas: 12,
			overseerLeader:  false,
			live:            true,
		},
		SolrNodeName(solrCloud, pods[6].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[6].Name),
			leaders:         3,
			replicas:        12,
			notDownReplicas: 12,
			overseerLeader:  false,
			live:            false,
		},
		SolrNodeName(solrCloud, pods[7].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[7].Name),
			leaders:         3,
			replicas:        12,
			notDownReplicas: 12,
			overseerLeader:  false,
			live:            true,
		},
		SolrNodeName(solrCloud, pods[8].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[8].Name),
			leaders:         3,
			replicas:        12,
			notDownReplicas: 12,
			overseerLeader:  false,
			live:            true,
		},
		SolrNodeName(solrCloud, pods[10].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[10].Name),
			leaders:         0,
			replicas:        0,
			notDownReplicas: 0,
			overseerLeader:  false,
			live:            false,
		},
		SolrNodeName(solrCloud, pods[11].Name): {
			nodeName:        SolrNodeName(solrCloud, pods[11].Name),
			leaders:         0,
			replicas:        0,
			notDownReplicas: 0,
			overseerLeader:  false,
			live:            true,
		},
	}

	expectedOrdering := []string{"foo-solrcloud-12", "foo-solrcloud-9", "foo-solrcloud-10", "foo-solrcloud-6", "foo-solrcloud-1", "foo-solrcloud-11", "foo-solrcloud-2", "foo-solrcloud-3", "foo-solrcloud-8", "foo-solrcloud-7", "foo-solrcloud-5", "foo-solrcloud-4", "foo-solrcloud-0"}

	sortNodePodsBySafety(pods, nodeMap, solrCloud)
	foundOrdering := make([]string, len(pods))
	for i, pod := range pods {
		foundOrdering[i] = pod.Name
	}
	assert.EqualValues(t, expectedOrdering, foundOrdering, "Ordering of pods not correct.")
}

func TestFindSolrNodeContents(t *testing.T) {
	overseerLeader := "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr"

	// Test allManagedPodsLive when true
	_, _, _, allManagedPodsLive := findSolrNodeContents(testRecoveringClusterStatus, overseerLeader, map[string]bool{
		"foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr": true,
		"foo-solrcloud-6.foo-solrcloud-headless.default:2000_solr": true,
	})
	assert.True(t, allManagedPodsLive, "allManagedPodsLive should be true, because both managed pods are live in cluster status")

	// Test allManagedPodsLive when false
	_, _, _, allManagedPodsLive = findSolrNodeContents(testRecoveringClusterStatus, overseerLeader, map[string]bool{
		"foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr": true,
		"foo-solrcloud-6.foo-solrcloud-headless.default:2000_solr": true,
		"foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr": true,
	})
	assert.False(t, allManagedPodsLive, "allManagedPodsLive should be false, because there is a managed pod that is not live")

	nodeContents, totalShardReplicas, shardReplicasNotActive, _ := findSolrNodeContents(testRecoveringClusterStatus, overseerLeader, map[string]bool{})

	expectedNodeContents := map[string]*SolrNodeContents{
		"foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr": {
			nodeName:        "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
			leaders:         0,
			replicas:        2,
			notDownReplicas: 1,
			totalReplicasPerShard: map[string]int{
				"col1|shard1": 1,
				"col2|shard2": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard1": 1,
			},
			downReplicasPerShard: map[string]int{
				"col2|shard2": 1,
			},
			overseerLeader: true,
			live:           true,
		},
		"foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr": {
			nodeName:        "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
			leaders:         1,
			replicas:        2,
			notDownReplicas: 2,
			totalReplicasPerShard: map[string]int{
				"col1|shard2": 1,
				"col2|shard1": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard2": 1,
				"col2|shard1": 1,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 true,
		},
		"foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr": {
			nodeName:        "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
			leaders:         0,
			replicas:        2,
			notDownReplicas: 2,
			totalReplicasPerShard: map[string]int{
				"col1|shard1": 1,
				"col1|shard2": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard2": 1,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 true,
		},
		"foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr": {
			nodeName:        "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
			leaders:         2,
			replicas:        3,
			notDownReplicas: 1,
			totalReplicasPerShard: map[string]int{
				"col1|shard1": 1,
				"col2|shard1": 1,
				"col2|shard2": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard1": 1,
			},
			downReplicasPerShard: map[string]int{
				"col2|shard1": 1,
				"col2|shard2": 1,
			},
			overseerLeader: false,
			live:           true,
		},
		"foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr": {
			nodeName:        "foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr",
			leaders:         0,
			replicas:        1,
			notDownReplicas: 1,
			totalReplicasPerShard: map[string]int{
				"col2|shard1": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col2|shard1": 1,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 false,
		},
		"foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr": {
			nodeName:        "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
			leaders:         1,
			replicas:        3,
			notDownReplicas: 3,
			totalReplicasPerShard: map[string]int{
				"col1|shard2": 1,
				"col2|shard2": 2,
			},
			activeReplicasPerShard: map[string]int{
				"col2|shard2": 2,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 true,
		},
		"foo-solrcloud-6.foo-solrcloud-headless.default:2000_solr": {
			nodeName:               "foo-solrcloud-6.foo-solrcloud-headless.default:2000_solr",
			leaders:                0,
			replicas:               0,
			notDownReplicas:        0,
			totalReplicasPerShard:  map[string]int{},
			activeReplicasPerShard: map[string]int{},
			downReplicasPerShard:   map[string]int{},
			overseerLeader:         false,
			live:                   true,
		},
	}
	assert.Equal(t, len(nodeContents), len(nodeContents), "Number of Solr nodes with content information is incorrect.")
	for node, foundNodeContents := range nodeContents {
		expectedContents, found := expectedNodeContents[node]
		assert.Truef(t, found, "No nodeContents found for node %s", node)
		assert.EqualValuesf(t, expectedContents, foundNodeContents, "NodeContents information from clusterstate is incorrect for node %s", node)
	}

	expectedTotalShardReplicas := map[string]int{
		"col1|shard1": 3,
		"col1|shard2": 3,
		"col2|shard1": 3,
		"col2|shard2": 4,
	}
	assert.EqualValues(t, expectedTotalShardReplicas, totalShardReplicas, "Shards replica count is incorrect.")

	expectedShardReplicasNotActive := map[string]int{
		"col1|shard1": 1,
		"col1|shard2": 1,
		"col2|shard1": 2,
		"col2|shard2": 2,
	}
	assert.EqualValues(t, expectedShardReplicasNotActive, shardReplicasNotActive, "Shards with replicas not active information is incorrect.")
}

func TestCalculateMaxPodsToUpgrade(t *testing.T) {
	maxPodsUnavailable := intstr.FromInt(2)

	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			Replicas: Replicas(10),
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
			UpdateStrategy: solr.SolrUpdateStrategy{
				Method: solr.ManagedUpdate,
				ManagedUpdateOptions: solr.ManagedUpdateOptions{
					MaxPodsUnavailable: &maxPodsUnavailable,
				},
			},
		},
	}

	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate := calculateMaxPodsToUpdate(solrCloud, 4, 0, 4)
	assert.Equal(t, 2, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, 2, foundUnavailableUpdatedPodCount, "Incorrect value of unavailableUpdatedPodCount")
	assert.Equal(t, 0, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")

	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 4, 0, 3)
	assert.Equal(t, 2, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, 3, foundUnavailableUpdatedPodCount, "Incorrect value of unavailableUpdatedPodCount")
	assert.Equal(t, -1, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")

	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 3, 1, 3)
	assert.Equal(t, 2, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, 3, foundUnavailableUpdatedPodCount, "Incorrect value of unavailableUpdatedPodCount")
	assert.Equal(t, -2, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")

	maxPodsUnavailable = intstr.FromString("45%")
	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 3, 0, 5)
	assert.Equal(t, 4, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromString(\"45%\")")
	assert.Equal(t, 2, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")

	maxPodsUnavailable = intstr.FromString("45%")
	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 1, 2, 5)
	assert.Equal(t, 4, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromString(\"45%\")")
	assert.Equal(t, 0, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")

	maxPodsUnavailable = intstr.FromString("70%")
	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 3, 0, 2)
	assert.Equal(t, 7, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromString(\"70%\")")
	assert.Equal(t, 2, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")

	solrCloud.Spec.UpdateStrategy.ManagedUpdateOptions.MaxPodsUnavailable = nil
	foundMaxPodsUnavailable, foundUnavailableUpdatedPodCount, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 3, 0, 2)
	assert.Equal(t, 2, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromString(\"25%\")")
	assert.Equal(t, -3, foundMaxPodsToUpdate, "Incorrect value of maxPodsToUpdate")
}

func TestSolrNodeName(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort:           2000,
				CommonServicePort: 80,
			},
		},
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "foo-solrcloud-0"},
		Spec:       corev1.PodSpec{},
	}

	assert.Equal(t, "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr", SolrNodeName(solrCloud, pod.Name), "Incorrect generation of Solr nodeName")

	solrCloud.Spec.SolrAddressability.PodPort = 3000
	assert.Equal(t, "foo-solrcloud-0.foo-solrcloud-headless.default:3000_solr", SolrNodeName(solrCloud, pod.Name), "Incorrect generation of Solr nodeName")
}

var (
	testRecoveringClusterStatus = solr_api.SolrClusterStatus{
		LiveNodes: []string{
			"foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-6.foo-solrcloud-headless.default:2000_solr",
		},
		Collections: map[string]solr_api.SolrCollectionStatus{
			"col1": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-0.foo-solrcloud-headless.default:2000/solr/rep-1-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-1-1-2": {
								State:    solr_api.ReplicaRecovering,
								Core:     "core1",
								NodeName: "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-2.foo-solrcloud-headless.default:2000/solr/rep-1-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-1-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-1-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-2.foo-solrcloud-headless.default:2000/solr/rep-1-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-2": {
								State:    solr_api.ReplicaRecovering,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-1-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-1.foo-solrcloud-headless.default:2000/solr/rep-1-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
			"col2": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-4.foo-solrcloud-headless.default:2000/solr/rep-2-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-2-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-1.foo-solrcloud-headless.default:2000/solr/rep-2-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-2-1-3": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-2-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-2-1": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-2-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-2": {
								State:    solr_api.ReplicaRecoveryFailed,
								Core:     "core1",
								NodeName: "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-0.foo-solrcloud-headless.default:2000/solr/rep-2-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
							"rep-2-2-4": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-4",
								Leader:   false,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
		},
	}

	testDownClusterStatus = solr_api.SolrClusterStatus{
		LiveNodes: []string{
			"foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
		},
		Collections: map[string]solr_api.SolrCollectionStatus{
			"col1": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-0.foo-solrcloud-headless.default:2000/solr/rep-1-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-1-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-2.foo-solrcloud-headless.default:2000/solr/rep-1-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-1-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-1-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-2.foo-solrcloud-headless.default:2000/solr/rep-1-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-1-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-1.foo-solrcloud-headless.default:2000/solr/rep-1-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
			"col2": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-4.foo-solrcloud-headless.default:2000/solr/rep-2-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-2-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-1.foo-solrcloud-headless.default:2000/solr/rep-2-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-2-1-3": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-2-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-2-1": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-2-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-0.foo-solrcloud-headless.default:2000/solr/rep-2-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
							"rep-2-2-4": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-4",
								Leader:   false,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
		},
	}

	testHealthyClusterStatus = solr_api.SolrClusterStatus{
		LiveNodes: []string{
			"foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr",
			"foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
		},
		Collections: map[string]solr_api.SolrCollectionStatus{
			"col1": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-0.foo-solrcloud-headless.default:2000/solr/rep-1-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-1-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-2.foo-solrcloud-headless.default:2000/solr/rep-1-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-1-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-1-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-2.foo-solrcloud-headless.default:2000/solr/rep-1-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-1-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-1.foo-solrcloud-headless.default:2000/solr/rep-1-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
			"col2": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-4.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-4.foo-solrcloud-headless.default:2000/solr/rep-2-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-2-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-1.foo-solrcloud-headless.default:2000/solr/rep-2-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-2-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-2-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-3.foo-solrcloud-headless.default:2000/solr/rep-2-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-0.foo-solrcloud-headless.default:2000/solr/rep-2-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
							"rep-2-2-4": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "foo-solrcloud-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "foo-solrcloud-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-4",
								Leader:   false,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
		},
	}
)

func getPodNames(pods []corev1.Pod, ignored ...map[string]bool) []string {
	names := make([]string, len(pods))
	for i, pod := range pods {
		names[i] = pod.Name
	}
	return names
}

func TestScheduleNextRestart(t *testing.T) {
	var schedule, nextRestart string
	var lastRestart time.Time
	var reconcileWaitDuration *time.Duration
	var err error
	now := time.Date(2020, 8, 10, 20, 10, 22, 0, time.Local)
	utcNow := now.UTC()

	// Test that no restart should happen
	nextRestart, reconcileWaitDuration, err = scheduleNextRestartWithTime(schedule, map[string]string{}, now)
	assert.Empty(t, nextRestart, "There should be no restart when the schedule is blank")
	assert.Nil(t, reconcileWaitDuration, "There should be no reconcile wait when the schedule is blank")
	assert.Empty(t, err, "There should be no error when the schedule is blank")

	// Test a bad schedule string
	schedule = "adasfdsdas"
	nextRestart, reconcileWaitDuration, err = scheduleNextRestartWithTime(schedule, map[string]string{}, now)
	assert.Empty(t, nextRestart, "There should be no restart when the schedule is blank")
	assert.Nil(t, reconcileWaitDuration, "There should be no reconcile wait when the schedule is blank")
	assert.Error(t, err, "There should be a parsing error for a bad schedule")

	// Test first restart
	schedule = "@every 10m"
	nextRestart, reconcileWaitDuration, err = scheduleNextRestartWithTime(schedule, map[string]string{}, now)
	assert.EqualValuesf(t, utcNow.Add(time.Minute*10).Format(time.RFC3339), nextRestart, "The restart is incorrect for schedule: %s", schedule)
	if assert.NotNil(t, reconcileWaitDuration, "There should be a reconcile wait when there is a non-empty schedule") {
		assert.EqualValues(t, time.Minute*10, *reconcileWaitDuration, "The reconcile wait is incorrect for the first scheduled restart")
	}
	assert.Emptyf(t, err, "There should be no error when the schedule is: %s", schedule)

	// Test new restart scheduled
	schedule = "@every 10m"
	lastRestart = utcNow.Add(time.Minute * -1)
	nextRestart, reconcileWaitDuration, err = scheduleNextRestartWithTime(schedule, map[string]string{SolrScheduledRestartAnnotation: lastRestart.Format(time.RFC3339)}, now)
	assert.EqualValuesf(t, utcNow.Add(time.Minute*9).Format(time.RFC3339), nextRestart, "The new next restart time is incorrect for previous nextRestart at \"%s\" and schedule: %s", lastRestart.Format(time.RFC3339), schedule)
	if assert.NotNil(t, reconcileWaitDuration, "There should be a reconcile wait when there is a non-empty schedule") {
		assert.EqualValuesf(t, time.Minute*9, *reconcileWaitDuration, "The reconcile wait is incorrect for a next restart at \"%s\" and a current time of \"%s\"", nextRestart, now)
	}
	assert.Emptyf(t, err, "There should be no error when the schedule is: %s", schedule)

	// Test new restart scheduled
	schedule = "@every 10m"
	lastRestart = utcNow.Add(time.Minute * 6)
	nextRestart, reconcileWaitDuration, err = scheduleNextRestartWithTime(schedule, map[string]string{SolrScheduledRestartAnnotation: lastRestart.Format(time.RFC3339)}, now)
	assert.Emptyf(t, nextRestart, "There should be no new restart time when the nextRestart is in the future: \"%s\"", lastRestart)
	if assert.NotNil(t, reconcileWaitDuration, "There should be a reconcile wait when there is a non-empty schedule") {
		assert.EqualValuesf(t, time.Minute*6, *reconcileWaitDuration, "The reconcile wait is incorrect for a next restart at \"%s\" and a current time of \"%s\"", nextRestart, now)
	}
	assert.Emptyf(t, err, "There should be no error when the schedule is: %s", schedule)
}
