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
	"strings"
	"time"
)

var _ = FDescribe("E2E - SolrCloud - Scale Down", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"
		solrCollection2 = "e2e-2"
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(3)
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
		createAndQueryCollection(ctx, solrCloud, solrCollection1, 1, 1, 1)

		By("creating a second Solr Collection")
		createAndQueryCollection(ctx, solrCloud, solrCollection2, 1, 1, 2)
	})

	FContext("with replica migration", func() {
		FIt("Scales Down", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(1)
			By("triggering a scale down via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale down")

			By("waiting for the scaleDown of first pod to begin")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should still have 3 pods, because the scale down should first move Solr replicas")
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("2"), "StatefulSet scaling lock operation has the wrong metadata.")
			})
			queryCollection(ctx, solrCloud, solrCollection2, 0)

			By("waiting for the scaleDown of the first pod to finish")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should now have 2 pods, after the replicas have been moved off the first pod.")
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("2"), "StatefulSet scaling lock operation has the wrong metadata.")
			})
			queryCollection(ctx, solrCloud, solrCollection2, 0)

			// Wait till the pod has actually been deleted
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Status.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should now have 2 pods, after the replicas have been moved off the first pod.")
			})

			By("waiting for the scaleDown of second pod to begin")
			statefulSet := expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("1"), "StatefulSet scaling lock operation has the wrong metadata.")
			})
			// When the next scale down happens, the 3rd solr pod (ordinal 2) should be gone, and the statefulSet replicas should be 2 across the board.
			// The first scale down should not be complete until this is done.
			Expect(statefulSet.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should still have 2 pods configured, because the scale down should first move Solr replicas")
			Expect(statefulSet.Status.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should only have 2 pods running, because previous pod scale down should have completely finished")
			// This pod check must happen after the above clusterLock and replicas check.
			// The StatefulSet controller might take a good amount of time to actually delete the pod,
			// and the replica migration/cluster op might already be done by the time the first pod is deleted.
			expectNoPodNow(ctx, solrCloud, solrCloud.GetSolrPodName(2))
			queryCollection(ctx, solrCloud, solrCollection1, 0)

			By("waiting for the scaleDown to finish")
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)), "StatefulSet should now have 1 pods, after the replicas have been moved.")
			})
			// Once the scale down actually occurs, the clusterOp is not complete. We need to wait till the last pod is deleted
			clusterOp, err := controllers.GetCurrentClusterOp(statefulSet)
			Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
			Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
			Expect(clusterOp.Metadata).To(Equal("1"), "StatefulSet scaling lock operation has the wrong metadata.")

			// Wait for the last pod to be deleted
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Status.Replicas).To(HaveValue(BeEquivalentTo(1)), "StatefulSet should now have 1 pods, after the replicas have been moved.")
			})
			// Once the scale down actually occurs, the statefulSet annotations should be removed very soon
			expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*2, time.Millisecond*500, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err = controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a ScaleDown lock after scaling is complete.")
			})

			expectNoPod(ctx, solrCloud, solrCloud.GetSolrPodName(1))
			queryCollection(ctx, solrCloud, solrCollection1, 0)
			queryCollection(ctx, solrCloud, solrCollection2, 0)
		})
	})

	FContext("with replica migration using an ingress for node addresses", func() {

		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: true,
					DomainName:         "test.solr.org",
				},
			}
		})
		FIt("Scales Down", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(1)
			By("triggering a scale down via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale down")

			By("waiting for the scaleDown of first pod to begin")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should still have 3 pods, because the scale down should first move Solr replicas")
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("2"), "StatefulSet scaling lock operation has the wrong metadata.")
			})
			queryCollection(ctx, solrCloud, solrCollection2, 0)

			// Make sure that ingress still has 3 nodes at this point
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Spec.Rules).To(HaveLen(4), "Wrong number of ingress rules. 3 nodes + 1 common endpoint. The third node rule should not be deleted until the node itself is deleted")

			By("waiting for the scaleDown of the first pod to finish")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should now have 2 pods, after the replicas have been moved off the first pod.")
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("2"), "StatefulSet scaling lock operation has the wrong metadata.")
			})
			queryCollection(ctx, solrCloud, solrCollection2, 0)

			// Wait till the pod has actually been deleted
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Status.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should now have 2 pods, after the replicas have been moved off the first pod.")
			})

			By("waiting for the scaleDown of second pod to begin")
			statefulSet := expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("1"), "StatefulSet scaling lock operation has the wrong metadata.")
			})

			// Make sure that ingress still has 2 nodes at this point
			ingress = expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Spec.Rules).To(HaveLen(3), "Wrong number of ingress rules. 2 nodes + 1 common endpoint. The second node rule should not be deleted until the node itself is deleted")

			// When the next scale down happens, the 3rd solr pod (ordinal 2) should be gone, and the statefulSet replicas should be 2 across the board.
			// The first scale down should not be complete until this is done.
			Expect(statefulSet.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should still have 2 pods configured, because the scale down should first move Solr replicas")
			Expect(statefulSet.Status.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should only have 2 pods running, because previous pod scale down should have completely finished")
			// This pod check must happen after the above clusterLock and replicas check.
			// The StatefulSet controller might take a good amount of time to actually delete the pod,
			// and the replica migration/cluster op might already be done by the time the first pod is deleted.
			expectNoPodNow(ctx, solrCloud, solrCloud.GetSolrPodName(2))
			queryCollection(ctx, solrCloud, solrCollection1, 0)

			By("waiting for the scaleDown to finish")
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)), "StatefulSet should now have 1 pods, after the replicas have been moved.")
			})
			// Once the scale down actually occurs, the clusterOp is not complete. We need to wait till the last pod is deleted
			clusterOp, err := controllers.GetCurrentClusterOp(statefulSet)
			Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
			Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
			Expect(clusterOp.Metadata).To(Equal("1"), "StatefulSet scaling lock operation has the wrong metadata.")

			// Wait for the last pod to be deleted
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Status.Replicas).To(HaveValue(BeEquivalentTo(1)), "StatefulSet should now have 1 pods, after the replicas have been moved.")
			})
			// Once the scale down actually occurs, the statefulSet annotations should be removed very soon
			expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*2, time.Millisecond*500, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err = controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a ScaleDown lock after scaling is complete.")
			})

			// Make sure that ingress has 1 node at this point
			ingress = expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Spec.Rules).To(HaveLen(2), "Wrong number of ingress rules. 1 nodes + 1 common endpoint.")

			expectNoPod(ctx, solrCloud, solrCloud.GetSolrPodName(1))
			queryCollection(ctx, solrCloud, solrCollection1, 0)
			queryCollection(ctx, solrCloud, solrCollection2, 0)
		})
	})

	FContext("without replica migration", func() {

		BeforeEach(func() {
			solrCloud.Spec.Scaling.VacatePodsOnScaleDown = pointer.Bool(false)
		})

		FIt("Scales Down", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(1))
			By("triggering a scale down via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale down")

			By("make sure scaleDown happens without a clusterLock and eventually the replicas are removed")
			statefulSet := expectStatefulSetWithConsistentChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a scaling lock, since scaleDown is unmanaged.")
			})
			Expect(statefulSet.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)), "StatefulSet should immediately have 1 pod, since the scaleDown is unmanaged.")

			expectNoPod(ctx, solrCloud, solrCloud.GetSolrPodName(1))
			queryCollectionWithNoReplicaAvailable(ctx, solrCloud, solrCollection1)
		})
	})
})

var _ = FDescribe("E2E - SolrCloud - Scale Up", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"
		solrCollection2 = "e2e-2"
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
		createAndQueryCollection(ctx, solrCloud, solrCollection1, 1, 1)

		By("creating a second Solr Collection")
		createAndQueryCollection(ctx, solrCloud, solrCollection2, 2, 1)
	})

	FContext("with replica migration", func() {

		FIt("Scales Up", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(3))
			By("triggering a scale up via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale up")

			By("waiting for the scaleUp to begin")
			statefulSet := expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*5, time.Millisecond*5, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleUp lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleUpLock), "StatefulSet does not have a scaleUp lock.")
				g.Expect(clusterOp.Metadata).To(Equal("3"), "StatefulSet scaling lock operation has the wrong metadata.")
			})

			// The first step is to increase the number of pods
			// Check very often, as the new pods will be created quickly, which will cause the cluster op to change.
			if int(*statefulSet.Spec.Replicas) != 3 {
				statefulSet = expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*5, time.Millisecond*5, func(g Gomega, found *appsv1.StatefulSet) {
					g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should be configured for 3 pods when scaling up")
				})
			}
			clusterOp, err := controllers.GetCurrentClusterOp(statefulSet)
			Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
			Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleUp lock.")
			Expect(clusterOp.Operation).To(Equal(controllers.ScaleUpLock), "StatefulSet does not have a scaleUp lock.")
			Expect(clusterOp.Metadata).To(Equal("3"), "StatefulSet scaling lock operation has the wrong metadata.")

			// Wait for new pods to come up, and when they do we should be doing a balanceReplicas clusterOp
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Status.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should eventually have 3 pods created for it")
			})
			clusterOp, err = controllers.GetCurrentClusterOp(statefulSet)
			Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
			Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a balanceReplicas lock after new pods are created.")
			Expect(clusterOp.Operation).To(Equal(controllers.BalanceReplicasLock), "StatefulSet does not have a balanceReplicas lock after new pods are created.")
			Expect(clusterOp.Metadata).To(Equal("ScaleUp"), "StatefulSet balanceReplicas lock operation has the wrong metadata.")

			By("waiting for the scaleUp to finish")
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a balanceReplicas lock after balancing is complete.")
			})

			queryCollection(ctx, solrCloud, solrCollection1, 0)
			queryCollection(ctx, solrCloud, solrCollection2, 0)
		})
	})

	FContext("with replica migration using an ingress for node addresses", func() {

		BeforeEach(func() {
			solrCloud.Spec.Replicas = pointer.Int32(2)
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: true,
					DomainName:         "test.solr.org",
				},
			}
		})

		FIt("Scales Up", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(3))
			By("triggering a scale up via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale up")

			By("waiting for the rolling restart to begin with hostAliases")
			statefulSet := expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*5, time.Millisecond*5, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a update lock to add hostAliases.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.UpdateLock), "StatefulSet does not have a update lock to add hostAliases.")
				g.Expect(found.Spec.Template.Spec.HostAliases).To(HaveLen(3), "Host aliases in the pod template do not have length 3, which is the number of pods to scale up to")
			})
			expectSolrCloudWithChecksAndTimeout(ctx, solrCloud, time.Second*5, time.Millisecond*5, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
				g.Expect(cloud.Status.UpToDateNodes).To(BeEquivalentTo(0), "The Rolling Update never started, upToDateNodes should eventually be 0 when starting a restart")
			})

			By("Wait for the rolling update to be done")
			expectSolrCloudWithChecksAndTimeout(ctx, solrCloud, time.Second*90, time.Millisecond*5, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
				g.Expect(cloud.Status.UpToDateNodes).To(BeEquivalentTo(*statefulSet.Spec.Replicas), "The Rolling Update never completed, not all replicas up to date")
				g.Expect(cloud.Status.ReadyReplicas).To(BeEquivalentTo(*statefulSet.Spec.Replicas), "The Rolling Update never completed, not all replicas ready")
			})

			By("waiting for the scaleUp to begin")
			statefulSet = expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*5, time.Millisecond*5, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleUp lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleUpLock), "StatefulSet does not have a scaleUp lock.")
				g.Expect(clusterOp.Metadata).To(Equal("3"), "StatefulSet scaling lock operation has the wrong metadata.")
			})

			// The first step is to increase the number of pods
			// Check very often, as the new pods will be created quickly, which will cause the cluster op to change.
			if int(*statefulSet.Spec.Replicas) != 3 {
				statefulSet = expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*5, time.Millisecond*5, func(g Gomega, found *appsv1.StatefulSet) {
					g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should be configured for 3 pods when scaling up")
				})
			}
			clusterOp, err := controllers.GetCurrentClusterOp(statefulSet)
			Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
			Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleUp lock.")
			Expect(clusterOp.Operation).To(Equal(controllers.ScaleUpLock), "StatefulSet does not have a scaleUp lock.")
			Expect(clusterOp.Metadata).To(Equal("3"), "StatefulSet scaling lock operation has the wrong metadata.")

			// Wait for new pods to come up, and when they do we should be doing a balanceReplicas clusterOp
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Status.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should eventually have 3 pods created for it")
			})
			statefulSet = expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Millisecond*50, time.Millisecond*10, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err = controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a balanceReplicas lock after new pods are created.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.BalanceReplicasLock), "StatefulSet does not have a balanceReplicas lock after new pods are created.")
				g.Expect(clusterOp.Metadata).To(Equal("ScaleUp"), "StatefulSet balanceReplicas lock operation has the wrong metadata.")
			})

			By("waiting for the scaleUp to finish")
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a balanceReplicas lock after balancing is complete.")
			})

			queryCollection(ctx, solrCloud, solrCollection1, 0)
			queryCollection(ctx, solrCloud, solrCollection2, 0)
		})
	})

	FContext("without replica migration", func() {

		BeforeEach(func() {
			solrCloud.Spec.Scaling.PopulatePodsOnScaleUp = pointer.Bool(false)
		})

		FIt("Scales Up", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(3))
			By("triggering a scale down via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale down")

			By("make sure scaleDown happens without a clusterLock and eventually the replicas are removed")
			statefulSet := expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(3)), "StatefulSet should immediately have 3 pods.")
			})
			clusterOp, err := controllers.GetCurrentClusterOp(statefulSet)
			Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
			Expect(clusterOp).To(BeNil(), "StatefulSet should not have a scaling lock, since scaleUp is unmanaged.")

			By("Waiting for the new solrCloud pods to become ready")
			solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

			queryCollection(ctx, solrCloud, solrCollection1, 0)
			queryCollection(ctx, solrCloud, solrCollection2, 0)
		})
	})
})

var _ = FDescribe("E2E - SolrCloud - Scale Down Abandon", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloudWithPlacementPolicy(2, "minimizecores")

		if strings.Contains(solrImage, ":8") || strings.Contains(solrImage, "8.") {
			Skip("Cannot run the Scale Down Abandon test with Solr 8, as a working placementPolicy for the test cannot be defaulted")
		}
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
		createAndQueryCollection(ctx, solrCloud, solrCollection1, 1, 2)
	})

	FContext("with replica migration", func() {

		FIt("Abandons the ScaleDown", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(1))
			By("triggering a scale down via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale up")

			By("waiting for the scaleDown to begin")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should still have 2 pods, because the scale down should first move Solr replicas")
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("1"), "StatefulSet scaling lock operation has the wrong metadata.")
			})

			By("Undo the scale down because the replicas cannot fit")
			originalSolrCloud = solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(2))
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to cancel scale down")

			By("Make sure the scaleDown attempts for a minute until it times out")
			// The scaleDown will timeout after a minute, so we have to wait a bit over a minute
			expectStatefulSetWithConsistentChecksAndDuration(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*50, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.ScaleDownLock), "StatefulSet does not have a scaleDown lock.")
				g.Expect(clusterOp.Metadata).To(Equal("1"), "StatefulSet scaleDown lock operation has the wrong metadata.")
			})

			By("Make sure that the operation is changed to a balanceReplicas to redistribute replicas")
			// The scaleDown will timeout after a minute, so we have to wait a bit over a minute
			expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*30, time.Millisecond*10, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a balanceReplicas lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.BalanceReplicasLock), "StatefulSet does not have a balanceReplicas lock.")
				g.Expect(clusterOp.Metadata).To(Equal("UndoFailedScaleDown"), "StatefulSet balanceReplicas lock operation has the wrong metadata.")
			})

			By("waiting for the balanceReplicas to finish")
			statefulSet := expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have a balanceReplicas lock after balancing is complete.")
			})

			Expect(statefulSet.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "After everything, the statefulset should be configured to have 2 pods")
			Expect(statefulSet.Status.Replicas).To(HaveValue(BeEquivalentTo(2)), "After everything, the statefulset should have 2 pods running")

			queryCollection(ctx, solrCloud, solrCollection1, 0)
		})
	})
})
