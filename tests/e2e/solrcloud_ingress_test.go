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
	corev1 "k8s.io/api/core/v1"
)

var _ = FDescribe("E2E - SolrCloud - Ingress", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(1)
		solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
			External: &solrv1beta1.ExternalAddressability{
				Method:             solrv1beta1.Ingress,
				UseExternalAddress: true,
				DomainName:         testDomain,
			},
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
		createAndQueryCollection(ctx, solrCloud, "basic", 1, 1)
	})

	FContext("Can Remove Ingress and Services when changing addressability", func() {

		FIt("Can adapt to changing needs", func(ctx context.Context) {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			// Pod Annotations test
			Expect(statefulSet.Spec.Template.Annotations).To(HaveKeyWithValue(util.ServiceTypeAnnotation, util.PerNodeServiceType), "Since external address is used for advertising, the perNode service should be specified in the pod annotations.")

			By("testing the Solr Common Service")
			expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(1), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
			}

			By("making sure Ingress was created correctly")
			expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("Turning off node external addressability and making sure the node services are deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.UseExternalAddress = false
				found.Spec.SolrAddressability.External.HideNodes = true
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to not advertise the nodes externally.")
			})

			// Since node external addressability is off, but common external addressability is on, the ingress should exist, but the node services should not
			expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)

			eventuallyExpectNoServices(ctx, solrCloud, "Node services shouldn't exist after the update, but they do.", nodeNames)
			pod := expectPodNow(ctx, solrCloud, solrCloud.GetSolrPodName(0))
			Expect(pod.Annotations).To(HaveKeyWithValue(util.ServiceTypeAnnotation, util.HeadlessServiceType))

			By("Turning off common external addressability and making sure the ingress is deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External = nil
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to remove external addressability")
			})
			eventuallyExpectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("Turning back on common external addressability and making sure the headless service is deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
					External: &solrv1beta1.ExternalAddressability{
						Method:             solrv1beta1.Ingress,
						UseExternalAddress: true,
						DomainName:         testDomain,
					},
				}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to add external addressability")
			})
			expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("testing the Solr Common Service")
			expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)

			By("making sure the individual Solr Node Services are created")

			Expect(nodeNames).To(HaveLen(1), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
			}

			By("ensuring the Solr Headless Service is deleted after the pod is updated")
			expectPodWithChecks(ctx, solrCloud, solrCloud.GetSolrPodName(0), func(g Gomega, pod *corev1.Pod) {
				g.Expect(pod.Annotations).To(HaveKeyWithValue(util.ServiceTypeAnnotation, util.PerNodeServiceType))
			})
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

		})
	})
})
