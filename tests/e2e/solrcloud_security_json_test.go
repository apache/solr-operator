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
	"github.com/apache/solr-operator/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = FDescribe("E2E - SolrCloud - Security JSON", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloudWithSecurityJSON(2)
	})

	JustBeforeEach(func(ctx context.Context) {
		By("generating the security.json secret and basic auth secret")
		generateStarterSolrBasicAuthSecret(ctx, solrCloud)
		generateBasicSolrSecuritySecret(ctx, solrCloud)

		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a first Solr Collection")
		createAndQueryCollection(ctx, solrCloud, "basic", 2, 1)

		// Because overwrite=false, this won't apply new security json
		// Pods will start back up, demonstrating that this was not applied
		By("updating security.json secret")
		generateBreakingSolrSecuritySecret(ctx, solrCloud)

		patchedSolrCloud := solrCloud.DeepCopy()
		patchedSolrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solrv1beta1.PodOptions{
			Annotations: map[string]string{
				"test": "restart-1",
			},
		}
		By("triggering a restart via pod annotations")
		Expect(k8sClient.Patch(ctx, patchedSolrCloud, client.MergeFrom(solrCloud))).To(Succeed(), "Could not add annotation to SolrCloud pod to initiate restart")

		// Trimmed down check where we just want the rolling restart to begin then end, as we're not testing restarts here
		By("waiting for the first restart to begin")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
			g.Expect(cloud.Status.UpToDateNodes).To(BeZero(), "Cloud did not get to a state with zero up-to-date replicas when rolling restart began.")
			for _, nodeStatus := range cloud.Status.SolrNodes {
				g.Expect(nodeStatus.SpecUpToDate).To(BeFalse(), "Node not starting as out-of-date when rolling restart begins: %s", nodeStatus.Name)
			}
		})

		// After all pods are ready, make sure that the SolrCloud status is correct
		By("waiting for the first restart to complete")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
			g.Expect(cloud.Status.UpToDateNodes).To(Equal(int32(2)), "The SolrCloud did not finish the rolling restart as not all nodes are up to date. Only reached %d", solrCloud.Status.UpToDateNodes)
			g.Expect(cloud.Status.ReadyReplicas).To(Equal(int32(2)), "The SolrCloud did not finish the rolling restart as not all nodes are ready. Only reached %d", solrCloud.Status.ReadyReplicas)
		})

		By("waiting for the balanceReplicas to finish")
		expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*70, time.Second, func(g Gomega, found *appsv1.StatefulSet) {
			clusterOp, err := controllers.GetCurrentClusterOp(found)
			g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
			g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a balanceReplicas lock after balancing is complete.")
		})

		By("Waiting for the SolrCloud to come up healthy with old security secret")
		expectSolrCloudToBeReady(ctx, solrCloud)

		By("querying existing Solr Collection")
		queryCollection(ctx, solrCloud, "basic", 0, 0)

		// Now with overwrite set to true, we'll update the applied security to allow "new-oper" user
		// Operator will continue to use "test-oper" user until we update the basic auth secret
		By("updating security.json secret")
		generateUpdatedSolrSecuritySecret(ctx, solrCloud)

		secondPatchedSolrCloud := solrCloud.DeepCopy()
		secondPatchedSolrCloud.Spec.SolrSecurity.BootstrapSecurityJson.Overwrite = true
		secondPatchedSolrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solrv1beta1.PodOptions{
			Annotations: map[string]string{
				"test": "restart-2",
			},
		}

		By("triggering a restart to overwrite the security.json")
		Expect(k8sClient.Patch(ctx, secondPatchedSolrCloud, client.MergeFrom(solrCloud))).To(Succeed(), "Could not update security spec for SolrCloud pod to initiate restart")

		// Trimmed down check where we just want the rolling restart to begin then end, as we're not testing restarts here
		By("waiting for the second restart to begin")
		newFoundCloud := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
			g.Expect(cloud.Status.UpToDateNodes).To(BeZero(), "Cloud did not get to a state with zero up-to-date replicas when rolling restart began.")
		})

		// After all pods are ready, make sure that the SolrCloud status is correct
		By("waiting for the second restart to complete")
		expectSolrCloudWithChecks(ctx, newFoundCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
			g.Expect(cloud.Status.UpToDateNodes).To(Equal(int32(2)), "The SolrCloud did not finish the rolling restart as not all nodes are up to date. Only reached %d", solrCloud.Status.UpToDateNodes)
			g.Expect(cloud.Status.ReadyReplicas).To(Equal(int32(2)), "The SolrCloud did not finish the rolling restart as not all nodes are ready. Only reached %d", solrCloud.Status.ReadyReplicas)
		})

		By("waiting for the balanceReplicas to finish")
		expectStatefulSetWithChecksAndTimeout(ctx, newFoundCloud, newFoundCloud.StatefulSetName(), time.Second*70, time.Second, func(g Gomega, found *appsv1.StatefulSet) {
			clusterOp, err := controllers.GetCurrentClusterOp(found)
			g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
			g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a balanceReplicas lock after balancing is complete.")
		})

		// This takes effect immediately (e.g. operater starts using it)
		// We will perform one more rolling update to demonstrate that the operater can still auth to Solr
		By("updating basic auth secret with new username")
		generateSolrBasicAuthSecret(ctx, solrCloud, "new-oper")

		finalPatchedSolrCloud := solrCloud.DeepCopy()
		finalPatchedSolrCloud.Spec.SolrSecurity.BootstrapSecurityJson.Overwrite = true
		finalPatchedSolrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solrv1beta1.PodOptions{
			Annotations: map[string]string{
				"test": "restart-3",
			},
		}

		By("triggering final restart to show operater authorizing with new uesrname")
		Expect(k8sClient.Patch(ctx, finalPatchedSolrCloud, client.MergeFrom(solrCloud))).To(Succeed(), "Could not update security spec for SolrCloud pod to initiate restart")

		// Trimmed down check where we just want the rolling restart to begin then end, as we're not testing restarts here
		By("waiting for the final restart to begin")
		finalFoundCloud := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
			g.Expect(cloud.Status.UpToDateNodes).To(BeZero(), "Cloud did not get to a state with zero up-to-date replicas when rolling restart began.")
		})

		// After all pods are ready, make sure that the SolrCloud status is correct
		By("waiting for the final restart to complete")
		expectSolrCloudWithChecks(ctx, finalFoundCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
			g.Expect(cloud.Status.UpToDateNodes).To(Equal(int32(2)), "The SolrCloud did not finish the rolling restart as not all nodes are up to date. Only reached %d", solrCloud.Status.UpToDateNodes)
			g.Expect(cloud.Status.ReadyReplicas).To(Equal(int32(2)), "The SolrCloud did not finish the rolling restart as not all nodes are ready. Only reached %d", solrCloud.Status.ReadyReplicas)
		})
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
