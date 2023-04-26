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
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = FDescribe("E2E - SolrCloud - Scaling", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(2)
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Status.ReadyReplicas).To(Equal(*found.Spec.Replicas), "The SolrCloud should have all nodes come up healthy")
		})

		By("creating a first Solr Collection")
		createAndQueryCollection(solrCloud, solrCollection1, 1, 1, 1)
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Scale Down with replica migration", func() {
		FIt("Scales Down", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = &one
			By("triggering a scale down via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale down")

			By("waiting for the scaleDown to begin")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, statefulSet *appsv1.StatefulSet) {
				g.Expect(statefulSet.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should still have 3 pods, because the scale down should first move Solr replicas")
				g.Expect(statefulSet.Annotations).To(HaveKeyWithValue(util.ClusterOpsLockAnnotation, util.ScaleLock), "StatefulSet does not have a scaling lock.")
				g.Expect(statefulSet.Annotations).To(HaveKeyWithValue(util.ClusterOpsMetadataAnnotation, "1"), "StatefulSet scaling lock operation has the wrong metadata.")
			})
			queryCollection(solrCloud, solrCollection1, 0)
			By("waiting for the scaleDown to finish")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, statefulSet *appsv1.StatefulSet) {
				g.Expect(statefulSet.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)), "StatefulSet should now have 2 pods, after the replicas have been moved.")
				g.Expect(statefulSet.Annotations).To(Not(HaveKey(util.ClusterOpsLockAnnotation)), "StatefulSet should not have a scaling lock after scaling is complete.")
				g.Expect(statefulSet.Annotations).To(Not(HaveKey(util.ClusterOpsMetadataAnnotation)), "StatefulSet should not have scaling lock metadata after scaling is complete.")
			})
			expectNoPod(ctx, solrCloud, solrCloud.GetSolrPodName(1))
			queryCollection(solrCloud, solrCollection1, 0)
		})
	})
})
