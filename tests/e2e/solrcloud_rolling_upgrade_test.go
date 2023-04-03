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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

var _ = FDescribe("E2E - SolrCloud - Rolling Upgrades", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"

		solrCollection2 = "e2e-2"
	)

	BeforeEach(func() {
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: testNamespace(),
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &three,
				SolrImage: &solrv1beta1.ContainerImage{
					Repository: strings.Split(solrImage, ":")[0],
					Tag:        strings.Split(solrImage+":", ":")[1],
					PullPolicy: corev1.PullIfNotPresent,
				},
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas:  &one,
						Ephemeral: &solrv1beta1.ZKEphemeral{},
					},
				},
				SolrJavaMem: "-Xms512m -Xmx512m",
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("600Mi"),
								corev1.ResourceCPU:    resource.MustParse("1"),
							},
						},
					},
				},
			},
		}
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Status.ReadyReplicas).To(Equal(*found.Spec.Replicas), "The SolrCloud should have all nodes come up healthy")
		})

		By("creating a first Solr Collection")
		createAndQueryCollection(solrCloud, solrCollection1, 1, 2)

		By("creating a second Solr Collection")
		createAndQueryCollection(solrCloud, solrCollection2, 2, 1)
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Managed Update - Ephemeral Data - Slow", func() {
		BeforeEach(func() {
			one := intstr.FromInt(1)
			hundredPerc := intstr.FromString("100%")
			solrCloud.Spec.UpdateStrategy = solrv1beta1.SolrUpdateStrategy{
				Method: solrv1beta1.ManagedUpdate,
				ManagedUpdateOptions: solrv1beta1.ManagedUpdateOptions{
					MaxPodsUnavailable:          &one,
					MaxShardReplicasUnavailable: &hundredPerc,
				},
			}
		})

		FIt("Fully Restarts", func(ctx context.Context) {
			patchedSolrCloud := solrCloud.DeepCopy()
			patchedSolrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solrv1beta1.PodOptions{
				Annotations: map[string]string{
					"test": "restart-1",
				},
			}
			By("triggering a rolling restart via pod annotations")
			Expect(k8sClient.Patch(ctx, patchedSolrCloud, client.MergeFrom(solrCloud))).To(Succeed(), "Could not add annotation to SolrCloud pod to initiate rolling restart")

			By("waiting for the rolling restart to begin")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
				g.Expect(cloud.Status.UpToDateNodes).To(BeZero(), "Cloud did not get to a state with zero up-to-date replicas when rolling restart began.")
				for _, nodeStatus := range cloud.Status.SolrNodes {
					g.Expect(nodeStatus.SpecUpToDate).To(BeFalse(), "Node not starting as out-of-date when rolling restart begins: %s", nodeStatus.Name)
				}
			})

			By("waiting for the rolling restart to complete")
			// Expect the SolrCloud to be up-to-date, or in a valid restarting state
			lastCheckNodeStatuses := make(map[string]solrv1beta1.SolrNodeStatus, *solrCloud.Spec.Replicas)
			lastCheckReplicas := *solrCloud.Spec.Replicas
			foundSolrCloud := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, cloud *solrv1beta1.SolrCloud) {
				// If there are more than 1 pods not ready, then fail because we have set MaxPodsUnavailable to 1
				if cloud.Status.ReadyReplicas < *solrCloud.Spec.Replicas-int32(1) {
					StopTrying("More than 1 pod (replica) is not ready, which is not allowed by the managed upgrade options").
						Attach("Replicas", *solrCloud.Spec.Replicas).
						Attach("ReadyReplicas", cloud.Status.ReadyReplicas).
						Attach("SolrCloud Status", cloud.Status).
						Now()
				}
				// As long as the current restart is in a healthy place, keep checking if the restart is finished
				g.Expect(cloud.Status.UpToDateNodes).To(Equal(*cloud.Spec.Replicas), "The SolrCloud did not finish the rolling restart, not all nodes are up-to-date")
				g.Expect(cloud.Status.ReadyReplicas).To(Equal(cloud.Status.UpToDateNodes), "The SolrCloud did not finish the rolling restart, all nodes are up-to-date, but not all are ready")

				// Make sure that if a pod is deleted/recreated, it was first taken offline and "scheduledForDeletion" was set to true
				// TODO: Try to find a better way to make sure that the deletion readinessCondition works
				if cloud.Status.Replicas < lastCheckReplicas {
					// We only want to check the statuses of nodes that the pods have been deleted, or they have been re-created since our last check
					for _, nodeStatus := range cloud.Status.SolrNodes {
						if !nodeStatus.SpecUpToDate || lastCheckNodeStatuses[nodeStatus.Name].SpecUpToDate {
							delete(lastCheckNodeStatuses, nodeStatus.Name)
						}
					}
					for _, nodeStatus := range cloud.Status.SolrNodes {
						oldNodeStatus := lastCheckNodeStatuses[nodeStatus.Name]
						g.Expect(oldNodeStatus.ScheduledForDeletion).To(BeTrue(), "Before SolrNode %s is taken down, scheduledForDeletion should be true", nodeStatus.Name)
						g.Expect(oldNodeStatus.Ready).To(BeFalse(), "Before SolrNode %s is taken down, it should not be ready", nodeStatus.Name)
					}
				}

				// Update the nodeStatuses for the next iteration's readinessCondition check
				lastCheckReplicas = cloud.Status.Replicas
				for _, nodeStatus := range cloud.Status.SolrNodes {
					lastCheckNodeStatuses[nodeStatus.Name] = nodeStatus

					if nodeStatus.Ready || nodeStatus.SpecUpToDate {
						g.Expect(nodeStatus.ScheduledForDeletion).To(BeFalse(), "SolrNode %s cannot be scheduledForDeletion while being 'ready' or 'upToDate'", nodeStatus.Name)
					} else {
						g.Expect(nodeStatus.ScheduledForDeletion).To(BeTrue(), "SolrNode %s must be scheduledForDeletion while not being 'ready' or 'upToDate', so it was taken down for the update", nodeStatus.Name)
					}
				}
			})

			// Make sure that the status object is correct for the nodes
			for _, nodeStatus := range foundSolrCloud.Status.SolrNodes {
				Expect(nodeStatus.SpecUpToDate).To(BeTrue(), "Node not finishing as up-to-date when rolling restart ends: %s", nodeStatus.Name)
				Expect(nodeStatus.Ready).To(BeTrue(), "Node not finishing as ready when rolling restart ends: %s", nodeStatus.Name)
			}

			By("checking that the collections can be queried after the restart")
			queryCollection(solrCloud, solrCollection1, 0)
			queryCollection(solrCloud, solrCollection2, 0)
		})
	})
})
