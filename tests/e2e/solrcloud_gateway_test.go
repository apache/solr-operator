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
)

var _ = FDescribe("E2E - SolrCloud - Gateway API", func() {
	var (
		solrCloud        *solrv1beta1.SolrCloud
		gatewayNamespace = "default"
		gatewayName      = "test-gateway"
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(1)
		solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
			External: &solrv1beta1.ExternalAddressability{
				Method:             solrv1beta1.Gateway,
				UseExternalAddress: true,
				DomainName:         testDomain,
				Gateway: &solrv1beta1.SolrGatewayOptions{
					ParentRefs: []solrv1beta1.GatewayParentReference{
						{
							Name:      gatewayName,
							Namespace: &gatewayNamespace,
						},
					},
				},
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

	FContext("Can Remove HTTPRoutes and Services when changing addressability", func() {

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

			By("making sure Common HTTPRoute was created correctly")
			expectHTTPRoute(ctx, solrCloud, solrCloud.CommonHTTPRouteName())

			By("making sure Node HTTPRoutes were created correctly")
			for _, nodeName := range nodeNames {
				expectHTTPRoute(ctx, solrCloud, solrCloud.NodeHTTPRouteName(nodeName))
			}

			By("Turning off node external addressability and making sure the node services are deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.HideNodes = true
				found.Spec.SolrAddressability.External.UseExternalAddress = false
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to not advertise the nodes externally.")
			})

			// Since node external addressability is off, but common external addressability is on, the common HTTPRoute should exist, but the node HTTPRoutes should not
			expectHTTPRoute(ctx, solrCloud, solrCloud.CommonHTTPRouteName())

			expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)

			for _, nodeName := range nodeNames {
				eventuallyExpectNoHTTPRoute(ctx, solrCloud, solrCloud.NodeHTTPRouteName(nodeName))
			}
			pod := expectPodNow(ctx, solrCloud, solrCloud.GetSolrPodName(0))
			Expect(pod.Annotations).To(HaveKeyWithValue(util.ServiceTypeAnnotation, util.HeadlessServiceType))

			By("Turning off common external addressability and making sure the HTTPRoutes are deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External = nil
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to remove external addressability")
			})
			eventuallyExpectNoHTTPRoute(ctx, solrCloud, solrCloud.CommonHTTPRouteName())

			By("Turning back on common external addressability and making sure the headless service is deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
					External: &solrv1beta1.ExternalAddressability{
						Method:             solrv1beta1.Gateway,
						UseExternalAddress: true,
						DomainName:         testDomain,
						Gateway: &solrv1beta1.SolrGatewayOptions{
							ParentRefs: []solrv1beta1.GatewayParentReference{
								{
									Name:      gatewayName,
									Namespace: &gatewayNamespace,
								},
							},
						},
					},
				}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to add external addressability")
			})
			expectHTTPRoute(ctx, solrCloud, solrCloud.CommonHTTPRouteName())

			By("testing the Solr Common Service")
			expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")
		})
	})

	FContext("BackendTLSPolicy Management", func() {
		var (
			caCertConfigMapName = "solr-ca-cert"
		)

		FIt("Creates and manages BackendTLSPolicy resources", func(ctx context.Context) {
			By("verifying BackendTLSPolicy resources do not exist initially")
			expectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
			nodeNames := solrCloud.GetAllSolrPodNames()
			for _, nodeName := range nodeNames {
				expectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}

			By("enabling BackendTLSPolicy with CA certificate reference")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy = &solrv1beta1.SolrBackendTLSPolicy{
					CACertificateRefs: []solrv1beta1.GatewayCertificateReference{
						{
							Name: caCertConfigMapName,
						},
					},
				}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to add BackendTLSPolicy")
			})

			By("verifying Common BackendTLSPolicy was created correctly")
			commonPolicy := expectBackendTLSPolicyWithChecks(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName(), func(g Gomega, policy *gatewayv1.BackendTLSPolicy) {
				g.Expect(policy.Spec.TargetRefs).To(HaveLen(1), "BackendTLSPolicy should have one target ref")
				g.Expect(string(policy.Spec.TargetRefs[0].Name)).To(Equal(solrCloud.CommonServiceName()), "BackendTLSPolicy should target common service")
				g.Expect(string(policy.Spec.TargetRefs[0].Kind)).To(Equal("Service"), "BackendTLSPolicy should target Service kind")
				g.Expect(string(policy.Spec.Validation.Hostname)).To(Equal(solrCloud.CommonServiceName()), "BackendTLSPolicy hostname should match service name")
				g.Expect(policy.Spec.Validation.CACertificateRefs).To(HaveLen(1), "BackendTLSPolicy should have one CA cert ref")
				g.Expect(string(policy.Spec.Validation.CACertificateRefs[0].Name)).To(Equal(caCertConfigMapName), "CA cert ref name should match")
				g.Expect(string(policy.Spec.Validation.CACertificateRefs[0].Kind)).To(Equal("ConfigMap"), "CA cert ref kind should default to ConfigMap")
				g.Expect(string(policy.Spec.Validation.CACertificateRefs[0].Group)).To(Equal(""), "CA cert ref group should be empty (core API)")
			})
			Expect(commonPolicy).ToNot(BeNil())

			By("verifying Node BackendTLSPolicy resources were created correctly")
			for _, nodeName := range nodeNames {
				nodePolicy := expectBackendTLSPolicyWithChecks(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName), func(g Gomega, policy *gatewayv1.BackendTLSPolicy) {
					g.Expect(policy.Spec.TargetRefs).To(HaveLen(1), "BackendTLSPolicy should have one target ref")
					g.Expect(string(policy.Spec.TargetRefs[0].Name)).To(Equal(nodeName), "BackendTLSPolicy should target node service")
					g.Expect(string(policy.Spec.Validation.Hostname)).To(Equal(nodeName), "BackendTLSPolicy hostname should match node service name")
					g.Expect(policy.Spec.Validation.CACertificateRefs).To(HaveLen(1), "BackendTLSPolicy should have one CA cert ref")
				})
				Expect(nodePolicy).ToNot(BeNil())
			}

			By("updating BackendTLSPolicy to use wellKnownCACertificates")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				wellKnown := "System"
				found.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy = &solrv1beta1.SolrBackendTLSPolicy{
					WellKnownCACertificates: &wellKnown,
				}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud BackendTLSPolicy to use wellKnownCACertificates")
			})

			By("verifying BackendTLSPolicy was updated to use wellKnownCACertificates")
			expectBackendTLSPolicyWithChecks(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName(), func(g Gomega, policy *gatewayv1.BackendTLSPolicy) {
				g.Expect(policy.Spec.Validation.CACertificateRefs).To(BeNil(), "CACertificateRefs should be nil when using wellKnownCACertificates")
				g.Expect(policy.Spec.Validation.WellKnownCACertificates).ToNot(BeNil(), "WellKnownCACertificates should be set")
				g.Expect(string(*policy.Spec.Validation.WellKnownCACertificates)).To(Equal("System"), "WellKnownCACertificates should be 'System'")
			})

			By("disabling node external addressability and verifying node BackendTLSPolicy resources are deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.HideNodes = true
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to hide nodes")
			})

			By("verifying node BackendTLSPolicy resources were deleted")
			for _, nodeName := range nodeNames {
				eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}

			By("verifying common BackendTLSPolicy still exists")
			expectBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())

			By("removing BackendTLSPolicy configuration")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy = nil
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to remove BackendTLSPolicy")
			})

			By("verifying all BackendTLSPolicy resources were deleted")
			eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
		})

		FIt("Cleans up BackendTLSPolicy when changing from Gateway method", func(ctx context.Context) {
			By("enabling BackendTLSPolicy")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy = &solrv1beta1.SolrBackendTLSPolicy{
					CACertificateRefs: []solrv1beta1.GatewayCertificateReference{
						{
							Name: caCertConfigMapName,
						},
					},
				}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to add BackendTLSPolicy")
			})

			By("verifying BackendTLSPolicy resources exist")
			expectBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
			nodeNames := solrCloud.GetAllSolrPodNames()
			for _, nodeName := range nodeNames {
				expectBackendTLSPolicy(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}

			By("changing external addressability method from Gateway to Ingress")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.Method = solrv1beta1.Ingress
				found.Spec.SolrAddressability.External.Gateway = nil
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to change method to Ingress")
			})

			By("verifying all BackendTLSPolicy resources were cleaned up")
			eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
			for _, nodeName := range nodeNames {
				eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}

			By("verifying HTTPRoutes were also cleaned up")
			eventuallyExpectNoHTTPRoute(ctx, solrCloud, solrCloud.CommonHTTPRouteName())
			for _, nodeName := range nodeNames {
				eventuallyExpectNoHTTPRoute(ctx, solrCloud, solrCloud.NodeHTTPRouteName(nodeName))
			}
		})
	})
})
