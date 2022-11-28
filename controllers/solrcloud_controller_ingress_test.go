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

package controllers

import (
	"context"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

var _ = FDescribe("SolrCloud controller - Ingress", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud
	)

	replicas := 3
	BeforeEach(func() {
		ctx = context.Background()

		int32Replicas := int32(replicas)
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &int32Replicas,
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					CommonServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testCommonServiceAnnotations,
						Labels:      testCommonServiceLabels,
					},
					IngressOptions: &solrv1beta1.IngressOptions{
						Annotations:      testIngressAnnotations,
						Labels:           testIngressLabels,
						IngressClassName: &testIngressClass,
					},
					NodeServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testNodeServiceAnnotations,
						Labels:      testNodeServiceLabels,
					},
					HeadlessServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testHeadlessServiceAnnotations,
						Labels:      testHeadlessServiceLabels,
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("defaulting the missing SolrCloud values")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Full Ingress", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: true,
					DomainName:         testDomain,
					NodePortOverride:   100,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(HaveLen(replicas), "Since external address is used for advertising, host aliases should be used for every node.")
			for i, hostAlias := range statefulSet.Spec.Template.Spec.HostAliases {
				Expect(hostAlias.Hostnames[0]).To(Equal(solrCloud.Namespace+"-"+statefulSet.Name+"-"+strconv.Itoa(i)+"."+testDomain), "Since external address is used for advertising, host aliases should be used for every node.")
			}

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      solrCloud.Namespace + "-$(POD_HOSTNAME)." + testDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "100",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(replicas), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
				expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "external"})
				Expect(service.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels)), "Incorrect node '"+nodeName+"' service labels")
				Expect(service.Annotations).To(Equal(testNodeServiceAnnotations), "Incorrect node '"+nodeName+"' service annotations")
				Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(100)), "Wrong port on node Service")
				Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
			}

			By("making sure Ingress was created correctly")
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
			Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
			Expect(ingress.Spec.IngressClassName).To(Not(BeNil()), "Ingress class name should not be nil")
			Expect(*ingress.Spec.IngressClassName).To(Equal(testIngressClass), "Incorrect ingress class name")
			testIngressRules(solrCloud, ingress, true, replicas, 4000, 100, testDomain)

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud"+"."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Hide Nodes from external connections - Using default ingress class", func() {
		ingressClass := &netv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example",
				Annotations: map[string]string{
					"ingressclass.kubernetes.io/is-default-class": "true",
				},
			},
			Spec: netv1.IngressClassSpec{
				Controller: "acme.io/foo",
			},
		}
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: true,
					HideNodes:          true,
					DomainName:         testDomain,
					NodePortOverride:   100,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			}
			solrCloud.Spec.CustomSolrKubeOptions.IngressOptions.IngressClassName = nil

			By("Create a default ingress class, so that the ingress is defaulted with this ingress class name")
			Expect(k8sClient.Create(ctx, ingressClass)).To(Succeed(), "Create a default ingress class for the ingress")
		})
		AfterEach(func() {
			By("Deleting the ingress class, so other tests do not use the default")
			Expect(k8sClient.Delete(ctx, ingressClass)).To(Succeed(), "Delete the default ingress class")
		})
		FIt("has the correct resources", func() {
			By("ensuring the SolrCloud resource is updated with correct specs")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Spec.SolrAddressability.External).To(Not(BeNil()), "Solr External addressability settings should not be nullified while setting defaults")
				g.Expect(found.Spec.SolrAddressability.External.UseExternalAddress).To(BeFalse(), "useExternalAddress should be set to 'false' when hideNodes is 'true'")
				g.Expect(found.Spec.SolrAddressability.External.NodePortOverride).To(Equal(0), "nodePortOverride should be set to '0' when hideNodes is 'true'")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(BeNil(), "There is no need for host aliases because traffic is going directly to pods.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "3000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")
			Expect(commonService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on common Service")
			Expect(commonService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on common Service should not be nil")
			Expect(*commonService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			Expect(headlessService.Annotations).To(Equal(testHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(3000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")
			Expect(headlessService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on headless Service")
			Expect(headlessService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on headless Service should not be nil")
			Expect(*headlessService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure Ingress was created correctly with a defaulted ingress class name")
			expectIngressWithConsistentChecks(ctx, solrCloud, solrCloud.CommonIngressName(), func(g Gomega, ingress *netv1.Ingress) {
				g.Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
				g.Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
				g.Expect(ingress.Spec.IngressClassName).To(Not(BeNil()), "Ingress class name should not be nil when none is provided in custom ingress options, but a default ingressClass exists")
				g.Expect(*ingress.Spec.IngressClassName).To(Equal(ingressClass.Name), "The wrong ingressClass was defaulted")
				testIngressRulesWithGomega(g, solrCloud, ingress, true, 0, 4000, 100, testDomain)
			})

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud"+"."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Hide Common endpoint from external connections", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: true,
					HideCommon:         true,
					DomainName:         testDomain,
					NodePortOverride:   100,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			}
			solrCloud.Spec.CustomSolrKubeOptions.IngressOptions.IngressClassName = nil
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(HaveLen(replicas), "Since external address is used for advertising, host aliases should be used for every node.")
			for i, hostAlias := range statefulSet.Spec.Template.Spec.HostAliases {
				Expect(hostAlias.Hostnames[0]).To(Equal(solrCloud.Namespace+"-"+statefulSet.Name+"-"+strconv.Itoa(i)+"."+testDomain), "Since external address is used for advertising, host aliases should be used for every node.")
			}

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      solrCloud.Namespace + "-$(POD_HOSTNAME)." + testDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "100",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")
			Expect(commonService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on common Service")
			Expect(commonService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on common Service should not be nil")
			Expect(*commonService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on common Service")

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(replicas), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
				expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "external"})
				Expect(service.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels)), "Incorrect node '"+nodeName+"' service labels")
				Expect(service.Annotations).To(Equal(testNodeServiceAnnotations), "Incorrect node '"+nodeName+"' service annotations")
				Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(100)), "Wrong port on node Service")
				Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
				Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on node Service")
				Expect(service.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on node Service should not be nil")
				Expect(*service.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on node Service")
			}

			By("making sure Ingress was created correctly")
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
			Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
			Expect(ingress.Spec.IngressClassName).To(BeNil(), "Ingress class name should not be nil when none is provided in custom ingress options")
			testIngressRules(solrCloud, ingress, false, replicas, 4000, 100, testDomain)

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(BeNil(), "External common address in status should be nil when hideCommon=true")
			})
		})
	})

	FContext("Use internal address for addressability in Solr", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: false,
					DomainName:         testDomain,
					NodePortOverride:   100,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(BeNil(), "There is no need for host aliases because traffic is going directly to pods.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.Namespace,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "100",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(replicas), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
				expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "external"})
				Expect(service.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels)), "Incorrect node '"+nodeName+"' service labels")
				Expect(service.Annotations).To(Equal(testNodeServiceAnnotations), "Incorrect node '"+nodeName+"' service annotations")
				Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(100)), "Wrong port on node Service")
				Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
			}

			By("making sure Ingress was created correctly")
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
			Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
			Expect(ingress.Spec.IngressClassName).To(Not(BeNil()), "Ingress class name should not be nil")
			Expect(*ingress.Spec.IngressClassName).To(Equal(testIngressClass), "Incorrect ingress class name")
			testIngressRules(solrCloud, ingress, true, replicas, 4000, 100, testDomain)

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud"+"."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Use extra domains", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:                solrv1beta1.Ingress,
					UseExternalAddress:    true,
					DomainName:            testDomain,
					AdditionalDomainNames: testAdditionalDomains,
					NodePortOverride:      100,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(HaveLen(replicas), "Since external address is used for advertising, host aliases should be used for every node.")
			for i, hostAlias := range statefulSet.Spec.Template.Spec.HostAliases {
				Expect(hostAlias.Hostnames[0]).To(Equal(solrCloud.Namespace+"-"+statefulSet.Name+"-"+strconv.Itoa(i)+"."+testDomain), "Since external address is used for advertising, host aliases should be used for every node.")
			}

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      solrCloud.Namespace + "-$(POD_HOSTNAME)." + testDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "100",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(replicas), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
				expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "external"})
				Expect(service.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels)), "Incorrect node '"+nodeName+"' service labels")
				Expect(service.Annotations).To(Equal(testNodeServiceAnnotations), "Incorrect node '"+nodeName+"' service annotations")
				Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(100)), "Wrong port on node Service")
				Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
			}

			By("making sure Ingress was created correctly")
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
			Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
			Expect(ingress.Spec.IngressClassName).To(Not(BeNil()), "Ingress class name should not be nil")
			Expect(*ingress.Spec.IngressClassName).To(Equal(testIngressClass), "Incorrect ingress class name")
			testIngressRules(solrCloud, ingress, true, replicas, 4000, 100, append([]string{testDomain}, testAdditionalDomains...)...)

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud"+"."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Use explicit kube domain & use internal address for Solr", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: false,
					DomainName:         testDomain,
					NodePortOverride:   100,
				},
				PodPort:    3000,
				KubeDomain: testKubeDomain,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(BeNil(), "There is no need for host aliases because traffic is going directly to pods.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.Namespace + ".svc." + testKubeDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "100",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(80)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(replicas), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
				expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "external"})
				Expect(service.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels)), "Incorrect node '"+nodeName+"' service labels")
				Expect(service.Annotations).To(Equal(testNodeServiceAnnotations), "Incorrect node '"+nodeName+"' service annotations")
				Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(100)), "Wrong port on node Service")
				Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
			}

			By("making sure Ingress was created correctly")
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
			Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
			Expect(ingress.Spec.IngressClassName).To(Not(BeNil()), "Ingress class name should not be nil")
			Expect(*ingress.Spec.IngressClassName).To(Equal(testIngressClass), "Incorrect ingress class name")
			testIngressRules(solrCloud, ingress, true, replicas, 80, 100, testDomain)

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+".svc."+testKubeDomain), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud"+"."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Use explicit kube domain & use external address for Solr", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.Ingress,
					UseExternalAddress: true,
					DomainName:         testDomain,
					NodePortOverride:   100,
				},
				PodPort:    3000,
				KubeDomain: testKubeDomain,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(HaveLen(replicas), "Since external address is used for advertising, host aliases should be used for every node.")
			for i, hostAlias := range statefulSet.Spec.Template.Spec.HostAliases {
				Expect(hostAlias.Hostnames[0]).To(Equal(solrCloud.Namespace+"-"+statefulSet.Name+"-"+strconv.Itoa(i)+"."+testDomain), "Since external address is used for advertising, host aliases should be used for every node.")
			}

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      solrCloud.Namespace + "-$(POD_HOSTNAME)." + testDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "100",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(80)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("ensuring the Solr Headless Service does not exist")
			expectNoService(ctx, solrCloud, solrCloud.HeadlessServiceName(), "Headless service shouldn't exist, but it does.")

			By("making sure the individual Solr Node Services exist and route correctly")
			nodeNames := solrCloud.GetAllSolrPodNames()
			Expect(nodeNames).To(HaveLen(replicas), "SolrCloud has incorrect number of nodeNames.")
			for _, nodeName := range nodeNames {
				service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
				expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "external"})
				Expect(service.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels)), "Incorrect node '"+nodeName+"' service labels")
				Expect(service.Annotations).To(Equal(testNodeServiceAnnotations), "Incorrect node '"+nodeName+"' service annotations")
				Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(100)), "Wrong port on node Service")
				Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
			}

			By("making sure Ingress was created correctly")
			ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
			Expect(ingress.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testIngressLabels)), "Incorrect ingress labels")
			Expect(ingress.Annotations).To(Equal(ingressLabelsWithDefaults(testIngressAnnotations)), "Incorrect ingress annotations")
			Expect(ingress.Spec.IngressClassName).To(Not(BeNil()), "Ingress class name should not be nil")
			Expect(*ingress.Spec.IngressClassName).To(Equal(testIngressClass), "Incorrect ingress class name")
			testIngressRules(solrCloud, ingress, true, replicas, 80, 100, testDomain)

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+".svc."+testKubeDomain), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud"+"."+testDomain), "Wrong external common address in status")
			})
		})
	})
})

func testIngressRules(solrCloud *solrv1beta1.SolrCloud, ingress *netv1.Ingress, withCommon bool, withNodes int, commonPort int, nodePort int, domainNames ...string) {
	testIngressRulesWithGomega(Default, solrCloud, ingress, withCommon, withNodes, commonPort, nodePort, domainNames...)
}

func testIngressRulesWithGomega(g Gomega, solrCloud *solrv1beta1.SolrCloud, ingress *netv1.Ingress, withCommon bool, withNodes int, commonPort int, nodePort int, domainNames ...string) {
	expected := 0
	if withCommon {
		expected += 1
	}
	if withNodes > 0 {
		expected += withNodes
	}
	perDomain := expected
	numDomains := len(domainNames)
	expected *= numDomains
	g.Expect(ingress.Spec.Rules).To(HaveLen(expected), "Wrong number of ingress rules.")
	for i := 0; i < perDomain; i++ {
		// Common Rules
		ruleName := "common"
		hostAppend := ""
		port := commonPort
		serviceSuffix := "common"

		// Node Rules
		if i > 0 || !withCommon {
			nodeNum := strconv.Itoa(i - 1)
			if !withCommon {
				nodeNum = strconv.Itoa(i)
			}
			ruleName = "node-" + nodeNum
			hostAppend = "-" + nodeNum
			serviceSuffix = nodeNum
			port = nodePort
		}
		for j, domainName := range domainNames {
			ruleIndex := j + i*numDomains
			rule := ingress.Spec.Rules[ruleIndex]
			expectedHost := solrCloud.Namespace + "-" + solrCloud.Name + "-solrcloud" + hostAppend + "." + domainName
			g.Expect(rule.Host).To(Equal(expectedHost), "Wrong host for ingress rule: "+ruleName)
			g.Expect(rule.HTTP.Paths).To(HaveLen(1), "Wrong number of path rules in ingress host: "+ruleName)
			path := rule.HTTP.Paths[0]
			g.Expect(path.Path).To(Equal(""), "There should be no path value for ingress rule: "+ruleName)
			g.Expect(path.Backend.Service).To(Not(BeNil()), "Backend Service should not be nil")
			g.Expect(path.Backend.Service.Name).To(Equal(solrCloud.Name+"-solrcloud-"+serviceSuffix), "Wrong service name for ingress rule: "+ruleName)
			g.Expect(path.Backend.Service.Port.Number).To(Equal(int32(port)), "Wrong port name for ingress rule: "+ruleName)
			g.Expect(path.Backend.Service.Port.Name).To(BeEmpty(), "Port name should not be used in ingress rules")
		}
	}
}

func ingressLabelsWithDefaults(labels map[string]string) map[string]string {
	return util.MergeLabelsOrAnnotations(labels, map[string]string{"nginx.ingress.kubernetes.io/backend-protocol": "HTTP"})
}
