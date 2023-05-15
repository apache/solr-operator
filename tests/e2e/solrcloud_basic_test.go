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

package e2e

import (
	"context"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
)

var _ = FDescribe("E2E - SolrCloud - Basic", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(1)
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a first Solr Collection")
		createAndQueryCollection(ctx, solrCloud, "basic", 1, 1)
	})

	FContext("Provided Zookeeper", func() {
		BeforeEach(func() {
			solrCloud.Spec.ZookeeperRef = &solrv1beta1.ZookeeperRef{
				ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
					Replicas:  pointer.Int32(1),
					Ephemeral: &solrv1beta1.ZKEphemeral{},
				},
			}
		})

		// All testing will be done in the "JustBeforeEach" logic, no additional tests required here
		FIt("Starts correctly", func(ctx context.Context) {})
	})
})
