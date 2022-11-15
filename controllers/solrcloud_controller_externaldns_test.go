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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("SolrCloud controller - External DNS", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		ctx = context.Background()

		replicas := int32(2)
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &replicas,
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

	FContext("Full ExternalDNS", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.ExternalDNS,
					UseExternalAddress: true,
					DomainName:         testDomain,
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
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.Namespace + "." + testDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "3000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(commonService.Annotations).To(Equal(expectedCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")
			Expect(commonService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on common Service")
			Expect(commonService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on common Service should not be nil")
			Expect(*commonService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(headlessService.Annotations).To(Equal(expectedHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(3000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")
			Expect(headlessService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on headless Service")
			Expect(headlessService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on headless Service should not be nil")
			Expect(*headlessService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+"."+testDomain+":4000"), "Wrong external common address in status")
			})
		})
	})

	FContext("Hiding Nodes from ExternalDNS", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.ExternalDNS,
					UseExternalAddress: true,
					HideNodes:          true,
					DomainName:         testDomain,
				},
				PodPort:           2000,
				CommonServicePort: 5000,
			}
		})
		FIt("has the correct resources", func() {
			By("ensuring the SolrCloud resource is updated with correct specs")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Spec.SolrAddressability.External).To(Not(BeNil()), "Solr External addressability settings should not be nullified while setting defaults")
				g.Expect(found.Spec.SolrAddressability.External.UseExternalAddress).To(BeFalse(), "useExternalAddress should be set to 'false' when hideNodes is 'true'")
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
				"SOLR_PORT":      "2000",
				"SOLR_NODE_PORT": "2000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "2000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(commonService.Annotations).To(Equal(expectedCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(5000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			Expect(headlessService.Annotations).To(Equal(testHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(2000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":5000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+"."+testDomain+":5000"), "Wrong external common address in status")
			})
		})
	})

	FContext("Hiding Common from ExternalDNS", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.ExternalDNS,
					UseExternalAddress: true,
					HideCommon:         true,
					DomainName:         testDomain,
				},
				PodPort:           3000,
				CommonServicePort: 2000,
			}
		})
		FIt("has the correct resources", func() {
			By("ensuring the SolrCloud resource is updated with correct specs")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Spec.SolrAddressability.External).To(Not(BeNil()), "Solr External addressability settings should not be nullified while setting defaults")
				g.Expect(found.Spec.SolrAddressability.External.UseExternalAddress).To(BeTrue(), "useExternalAddress should not be set to 'false' when hideNodes is 'false'")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(BeNil(), "There is no need for host aliases because traffic is going directly to pods.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.Namespace + "." + testDomain,
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
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(2000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(headlessService.Annotations).To(Equal(expectedHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(3000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":2000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(BeNil(), "External common address in status should be nil")
			})
		})
	})

	FContext("Use internal address for addressability in Solr", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.ExternalDNS,
					UseExternalAddress: false,
					DomainName:         testDomain,
					NodePortOverride:   454,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			}
		})
		FIt("has the correct resources", func() {
			By("ensuring the SolrCloud resource is updated with correct specs")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Spec.SolrAddressability.External).To(Not(BeNil()), "Solr External addressability settings should not be nullified while setting defaults")
				g.Expect(found.Spec.SolrAddressability.External.NodePortOverride).To(Equal(0), "nodePortOverride should not be set to '0' when used with ExternalDNS")
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
			expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(commonService.Annotations).To(Equal(expectedCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(headlessService.Annotations).To(Equal(expectedHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(3000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+"."+testDomain+":4000"), "Wrong external common address in status")
			})
		})
	})

	FContext("Use extra domains with ExternalDNS", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:                solrv1beta1.ExternalDNS,
					UseExternalAddress:    true,
					DomainName:            testDomain,
					AdditionalDomainNames: testAdditionalDomains,
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
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.Namespace + "." + testDomain,
				"SOLR_PORT":      "3000",
				"SOLR_NODE_PORT": "3000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "3000"}), "Incorrect pre-stop command")

			hostnameAnnotation := solrCloud.Namespace + "." + testDomain
			for _, domain := range testAdditionalDomains {
				hostnameAnnotation += "," + solrCloud.Namespace + "." + domain
			}

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": hostnameAnnotation,
			})
			Expect(commonService.Annotations).To(Equal(expectedCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(4000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": hostnameAnnotation,
			})
			Expect(headlessService.Annotations).To(Equal(expectedHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(3000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+":4000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+"."+testDomain+":4000"), "Wrong external common address in status")
			})
		})
	})

	FContext("Use explicit kube domain & use internal address for Solr", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.ExternalDNS,
					UseExternalAddress: true,
					HideNodes:          true,
					DomainName:         testDomain,
				},
				PodPort:           2000,
				CommonServicePort: 5000,
				KubeDomain:        testKubeDomain,
			}
		})
		FIt("has the correct resources", func() {
			By("ensuring the SolrCloud resource is updated with correct specs")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Spec.SolrAddressability.External).To(Not(BeNil()), "Solr External addressability settings should not be nullified while setting defaults")
				g.Expect(found.Spec.SolrAddressability.External.UseExternalAddress).To(BeFalse(), "useExternalAddress should be set to 'false' when hideNodes is 'true'")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(BeNil(), "There is no need for host aliases because traffic is going directly to pods.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace + ".svc." + testKubeDomain,
				"SOLR_PORT":      "2000",
				"SOLR_NODE_PORT": "2000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "2000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(commonService.Annotations).To(Equal(expectedCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(5000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			Expect(headlessService.Annotations).To(Equal(testHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(2000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+".svc."+testKubeDomain+":5000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+"."+testDomain+":5000"), "Wrong external common address in status")
			})
		})
	})

	FContext("Use explicit kube domain & use external address for Solr", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrAddressability = solrv1beta1.SolrAddressabilityOptions{
				External: &solrv1beta1.ExternalAddressability{
					Method:             solrv1beta1.ExternalDNS,
					UseExternalAddress: true,
					DomainName:         testDomain,
				},
				PodPort:           2000,
				CommonServicePort: 5000,
				KubeDomain:        testKubeDomain,
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
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.Namespace + "." + testDomain,
				"SOLR_PORT":      "2000",
				"SOLR_NODE_PORT": "2000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "2000"}), "Incorrect pre-stop command")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(commonService.Annotations).To(Equal(expectedCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(5000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": solrCloud.Namespace + "." + testDomain,
			})
			Expect(headlessService.Annotations).To(Equal(expectedHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(2000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+".svc."+testKubeDomain+":5000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in status should not be nil")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+"."+testDomain+":5000"), "Wrong external common address in status")
			})
		})
	})
})
