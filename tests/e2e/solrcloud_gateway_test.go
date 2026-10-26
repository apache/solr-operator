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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = FDescribe("E2E - SolrCloud - Gateway API", func() {
	var (
		solrCloud   *solrv1beta1.SolrCloud
		gatewayName = "test-gateway"
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(1)
		solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
			External: &solrv1beta1.ExternalAddressability{
				Method:             solrv1beta1.Gateway,
				UseExternalAddress: true,
				DomainName:         testDomain,
				Gateway: &solrv1beta1.SolrGatewayOptions{
					// The Gateway is created in the SolrCloud's namespace, so parentRefs default to that namespace.
					ParentRefs: []solrv1beta1.GatewayParentReference{
						{
							Name: gatewayName,
						},
					},
				},
			},
		}
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the Gateway the operator's HTTPRoutes will attach to")
		createGatewayForCloud(ctx, solrCloud, gatewayName)

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

			By("making sure Envoy Gateway accepted and programmed the HTTPRoutes")
			expectHTTPRouteToBeAccepted(ctx, solrCloud, solrCloud.CommonHTTPRouteName())
			for _, nodeName := range nodeNames {
				expectHTTPRouteToBeAccepted(ctx, solrCloud, solrCloud.NodeHTTPRouteName(nodeName))
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
			// The pod rolls to pick up the headless service-type annotation; wait for that rather than reading it
			// immediately, since the operator removes the node HTTPRoutes before the pod has finished rolling.
			expectPodWithChecks(ctx, solrCloud, solrCloud.GetSolrPodName(0), func(g Gomega, pod *corev1.Pod) {
				g.Expect(pod.Annotations).To(HaveKeyWithValue(util.ServiceTypeAnnotation, util.HeadlessServiceType))
			})

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
									Name: gatewayName,
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

	FContext("Automatic Backend TLS", func() {
		BeforeEach(func(ctx context.Context) {
			// Run Solr with TLS so the operator automatically manages BackendTLSPolicy resources for the Gateway.
			// The cert-manager-issued secret carries a `ca.crt` entry, which the operator derives the backend CA
			// from, and the cert covers the internal service addresses used as the validation hostnames.
			installSolrIssuer(ctx, testNamespace())
			configureSolrCloudWithSecretTLS(ctx, solrCloud, false, true)
		})

		FIt("Automatically creates and manages BackendTLSPolicy resources from spec.solrTLS", func(ctx context.Context) {
			nodeNames := solrCloud.GetAllSolrPodNames()

			By("verifying the operator derived a backend CA ConfigMap from the Solr TLS secret")
			Eventually(func(g Gomega) {
				configMap := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.BackendTLSCACertConfigMapName()), configMap)).To(Succeed(), "Derived backend CA ConfigMap does not exist")
				g.Expect(configMap.Data).To(HaveKey("ca.crt"), "Derived backend CA ConfigMap should contain a ca.crt entry")
				g.Expect(configMap.Data["ca.crt"]).ToNot(BeEmpty(), "Derived backend CA ConfigMap ca.crt should not be empty")
			}).Should(Succeed())

			By("verifying the common BackendTLSPolicy was auto-created with derived defaults")
			expectBackendTLSPolicyWithChecks(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName(), func(g Gomega, policy *gatewayv1.BackendTLSPolicy) {
				g.Expect(policy.Spec.TargetRefs).To(HaveLen(1), "BackendTLSPolicy should have one target ref")
				g.Expect(string(policy.Spec.TargetRefs[0].Name)).To(Equal(solrCloud.CommonServiceName()), "BackendTLSPolicy should target common service")
				g.Expect(string(policy.Spec.TargetRefs[0].Kind)).To(Equal("Service"), "BackendTLSPolicy should target Service kind")
				// Hostname defaults to the internal service address (which the Solr cert covers), not the external domain.
				g.Expect(string(policy.Spec.Validation.Hostname)).To(Equal(solrCloud.InternalCommonUrl(false)), "BackendTLSPolicy hostname should be the internal common service address")
				g.Expect(policy.Spec.Validation.CACertificateRefs).To(HaveLen(1), "BackendTLSPolicy should reference the derived CA ConfigMap")
				g.Expect(string(policy.Spec.Validation.CACertificateRefs[0].Name)).To(Equal(solrCloud.BackendTLSCACertConfigMapName()), "CA cert ref should point at the derived ConfigMap")
				g.Expect(string(policy.Spec.Validation.CACertificateRefs[0].Kind)).To(Equal("ConfigMap"), "CA cert ref kind should be ConfigMap")
				g.Expect(policy.Spec.Validation.WellKnownCACertificates).To(BeNil(), "WellKnownCACertificates should be unset when a CA ref is derived")
			})

			By("verifying node BackendTLSPolicy resources were auto-created")
			for _, nodeName := range nodeNames {
				expectBackendTLSPolicyWithChecks(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName), func(g Gomega, policy *gatewayv1.BackendTLSPolicy) {
					g.Expect(policy.Spec.TargetRefs).To(HaveLen(1), "BackendTLSPolicy should have one target ref")
					g.Expect(string(policy.Spec.TargetRefs[0].Name)).To(Equal(nodeName), "BackendTLSPolicy should target the node service")
					g.Expect(string(policy.Spec.Validation.Hostname)).To(Equal(solrCloud.InternalNodeUrl(nodeName, false)), "BackendTLSPolicy hostname should be the internal node service address")
					g.Expect(policy.Spec.Validation.CACertificateRefs).To(HaveLen(1), "Node BackendTLSPolicy should reference the derived CA ConfigMap")
					g.Expect(string(policy.Spec.Validation.CACertificateRefs[0].Name)).To(Equal(solrCloud.BackendTLSCACertConfigMapName()), "CA cert ref should point at the derived ConfigMap")
				})
			}

			By("verifying Envoy Gateway accepted the HTTPRoutes and BackendTLSPolicies (backend re-encryption is programmed)")
			expectHTTPRouteToBeAccepted(ctx, solrCloud, solrCloud.CommonHTTPRouteName())
			expectBackendTLSPolicyToBeAccepted(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
			for _, nodeName := range nodeNames {
				expectHTTPRouteToBeAccepted(ctx, solrCloud, solrCloud.NodeHTTPRouteName(nodeName))
				expectBackendTLSPolicyToBeAccepted(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}

			By("issuing a real request to a Solr node through the Gateway (exercises the auto-generated BackendTLSPolicy)")
			// Route to a node, whose backend cert covers the internal node URL used as the BackendTLSPolicy hostname.
			// This forces Envoy to open a TLS connection to the TLS-enabled Solr backend, validated against the
			// operator-derived CA, and proxy the response back — end-to-end proof the backend TLS actually works.
			clientPod := deployGatewayTestClientPod(ctx, solrCloud.Namespace)
			proxyAddress := envoyProxyAddressForGateway(ctx, gatewayName, solrCloud.Namespace, 80)
			nodeHost := solrCloud.ExternalNodeUrl(nodeNames[0], solrCloud.Spec.SolrAddressability.External.DomainName, false)
			Eventually(func(g Gomega) {
				response, err := callSolrThroughGateway(ctx, solrCloud.Namespace, clientPod, proxyAddress, nodeHost, "/solr/admin/info/system?wt=json")
				g.Expect(err).ToNot(HaveOccurred(), "Request to Solr through the Gateway failed")
				g.Expect(response).To(ContainSubstring("HTTP_STATUS:200"), "Expected a 200 from Solr through the Gateway, got: %s", response)
				g.Expect(response).To(ContainSubstring("lucene"), "Expected a Solr system-info response through the Gateway, got: %s", response)
			}).Should(Succeed(), "Never got a successful response from Solr through the Gateway")

			By("overriding the derived CA with wellKnownCACertificates")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				wellKnown := "System"
				found.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy = &solrv1beta1.SolrBackendTLSPolicy{
					WellKnownCACertificates: &wellKnown,
				}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't override the backendTLSPolicy CA")
			})

			By("verifying the override took effect and the derived CA ConfigMap was cleaned up")
			expectBackendTLSPolicyWithChecks(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName(), func(g Gomega, policy *gatewayv1.BackendTLSPolicy) {
				g.Expect(policy.Spec.Validation.CACertificateRefs).To(BeNil(), "CACertificateRefs should be nil when using wellKnownCACertificates")
				g.Expect(policy.Spec.Validation.WellKnownCACertificates).ToNot(BeNil(), "WellKnownCACertificates should be set")
				g.Expect(string(*policy.Spec.Validation.WellKnownCACertificates)).To(Equal("System"), "WellKnownCACertificates should be 'System'")
			})
			eventuallyExpectNoConfigMap(ctx, solrCloud, solrCloud.BackendTLSCACertConfigMapName())

			By("hiding nodes and verifying only the node BackendTLSPolicy resources are deleted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrAddressability.External.HideNodes = true
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Couldn't update the solrCloud to hide nodes")
			})
			for _, nodeName := range nodeNames {
				eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}
			expectBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
		})

		FIt("Cleans up BackendTLSPolicy when changing away from the Gateway method", func(ctx context.Context) {
			By("verifying the auto-created BackendTLSPolicy resources exist")
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

			By("verifying all BackendTLSPolicy resources and the derived CA ConfigMap were cleaned up")
			eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.CommonBackendTLSPolicyName())
			for _, nodeName := range nodeNames {
				eventuallyExpectNoBackendTLSPolicy(ctx, solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName))
			}
			eventuallyExpectNoConfigMap(ctx, solrCloud, solrCloud.BackendTLSCACertConfigMapName())

			By("verifying HTTPRoutes were also cleaned up")
			eventuallyExpectNoHTTPRoute(ctx, solrCloud, solrCloud.CommonHTTPRouteName())
			for _, nodeName := range nodeNames {
				eventuallyExpectNoHTTPRoute(ctx, solrCloud, solrCloud.NodeHTTPRouteName(nodeName))
			}
		})
	})
})
