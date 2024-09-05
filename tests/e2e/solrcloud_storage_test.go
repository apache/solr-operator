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
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = FDescribe("E2E - SolrCloud - Storage", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"

		solrCollection2 = "e2e-2"
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(2)
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

		By("creating a second Solr Collection")
		createAndQueryCollection(ctx, solrCloud, solrCollection2, 2, 1)
	})

	FContext("Persistent Data - Expansion", func() {
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				PersistentStorage: &solrv1beta1.SolrPersistentDataStorageOptions{
					PersistentVolumeClaimTemplate: solrv1beta1.PersistentVolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceStorage: resource.MustParse("1G"),
								},
							},
						},
					},
				},
			}
		})

		FIt("Fully Expands", func(ctx context.Context) {
			newStorageSize := resource.MustParse("1500M")
			patchedSolrCloud := solrCloud.DeepCopy()
			patchedSolrCloud.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage] = newStorageSize
			By("triggering a rolling restart via pod annotations")
			Expect(k8sClient.Patch(ctx, patchedSolrCloud, client.MergeFrom(solrCloud))).To(Succeed(), "Could not add annotation to SolrCloud pod to initiate rolling restart")

			// Wait for new pods to come up, and when they do we should be doing a balanceReplicas clusterOp
			expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*5, time.Millisecond*50, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a PvcExpansion lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.PvcExpansionLock), "StatefulSet does not have a PvcExpansion lock after starting managed update.")
			})

			By("waiting for the expansion's rolling restart to begin")
			solrCloud = expectSolrCloudWithChecksAndTimeout(ctx, solrCloud, time.Second*30, time.Millisecond*100, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Status.UpToDateNodes).To(BeZero(), "Cloud did not get to a state with zero up-to-date replicas when rolling restart began.")
				for _, nodeStatus := range found.Status.SolrNodes {
					g.Expect(nodeStatus.SpecUpToDate).To(BeFalse(), "Node not starting as out-of-date when rolling restart begins: %s", nodeStatus.Name)
				}
			})

			By("checking that all PVCs have been expanded when the restart begins")
			internalLabels := map[string]string{
				util.SolrPVCTechnologyLabel: util.SolrCloudPVCTechnology,
				util.SolrPVCStorageLabel:    util.SolrCloudPVCDataStorage,
				util.SolrPVCInstanceLabel:   solrCloud.Name,
			}
			pvcListOps := &client.ListOptions{
				Namespace:     solrCloud.Namespace,
				LabelSelector: labels.SelectorFromSet(internalLabels),
			}

			foundPVCs := &corev1.PersistentVolumeClaimList{}
			Expect(k8sClient.List(ctx, foundPVCs, pvcListOps)).To(Succeed(), "Could not fetch PVC list")
			Expect(foundPVCs.Items).To(HaveLen(int(*solrCloud.Spec.Replicas)), "Did not find the same number of PVCs as Solr Pods")
			for _, pvc := range foundPVCs.Items {
				Expect(pvc.Spec.Resources).To(HaveKeyWithValue(corev1.ResourceStorage, newStorageSize), "The PVC %q does not have the new storage size in its resource requests", pvc.Name)
				Expect(pvc.Status.Capacity).To(HaveKeyWithValue(corev1.ResourceStorage, newStorageSize), "The PVC %q does not have the new storage size in its status.capacity", pvc.Name)
			}

			statefulSet := expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), 1, time.Millisecond, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).ToNot(BeNil(), "StatefulSet does not have a RollingUpdate lock.")
				g.Expect(clusterOp.Operation).To(Equal(controllers.UpdateLock), "StatefulSet does not have a RollingUpdate lock after starting managed update to increase the storage size.")
				g.Expect(clusterOp.Metadata).To(Equal(controllers.RollingUpdateMetadata{RequiresReplicaMigration: false}), "StatefulSet should not require replica migration, since PVCs are being used.")
			})

			By("waiting for the rolling restart to complete")
			expectSolrCloudWithChecksAndTimeout(ctx, solrCloud, time.Second*90, time.Millisecond*5, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
				g.Expect(cloud.Status.UpToDateNodes).To(BeEquivalentTo(*statefulSet.Spec.Replicas), "The Rolling Update never completed, not all replicas up to date")
				g.Expect(cloud.Status.ReadyReplicas).To(BeEquivalentTo(*statefulSet.Spec.Replicas), "The Rolling Update never completed, not all replicas ready")
			})

			By("waiting for the rolling restart to complete")
			expectStatefulSetWithConsistentChecksAndDuration(ctx, solrCloud, solrCloud.StatefulSetName(), time.Second*2, func(g Gomega, found *appsv1.StatefulSet) {
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should not have any cluster lock after finishing its rolling update.")
			})

			By("checking that the collections can be queried after the restart")
			queryCollection(ctx, solrCloud, solrCollection1, 0)
			queryCollection(ctx, solrCloud, solrCollection2, 0)
		})
	})
})
