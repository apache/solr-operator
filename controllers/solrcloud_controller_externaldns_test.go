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
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/apache/solr-operator/controllers/util"
	"github.com/stretchr/testify/assert"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &SolrCloudReconciler{}

func TestEDSCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:             solr.ExternalDNS,
					UseExternalAddress: true,
					DomainName:         testDomain,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Host Alias Tests
	assert.Nil(t, statefulSet.Spec.Template.Spec.HostAliases, "There is no need for host aliases because traffic is going directly to pods.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + instance.Namespace + "." + testDomain,
		"SOLR_PORT":      "3000",
		"SOLR_NODE_PORT": "3000",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, []string{"solr", "stop", "-p", "3000"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "common service annotations", expectedCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 4000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "headless service annotations", expectedHeadlessServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 3000, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// Check that the Addresses in the status are correct
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())
	assert.Equal(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+":4000", instance.Status.InternalCommonAddress, "Wrong internal common address in status")
	assert.NotNil(t, instance.Status.ExternalCommonAddress, "External common address in Status should not be nil.")
	assert.EqualValues(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+"."+testDomain+":4000", *instance.Status.ExternalCommonAddress, "Wrong external common address in status")
}

func TestEDSNoNodesCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:             solr.ExternalDNS,
					UseExternalAddress: true,
					HideNodes:          true,
					DomainName:         testDomain,
				},
				PodPort:           2000,
				CommonServicePort: 5000,
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Host Alias Tests
	assert.Nil(t, statefulSet.Spec.Template.Spec.HostAliases, "There is no need for host aliases because traffic is going directly to pods.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + instance.Namespace,
		"SOLR_PORT":      "2000",
		"SOLR_NODE_PORT": "2000",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, []string{"solr", "stop", "-p", "2000"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "common service annotations", expectedCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 5000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	testMapsEqual(t, "headless service annotations", testHeadlessServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 2000, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// Check that the Addresses in the status are correct
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())
	assert.Equal(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+":5000", instance.Status.InternalCommonAddress, "Wrong internal common address in status")
	assert.NotNil(t, instance.Status.ExternalCommonAddress, "External common address in Status should not be nil.")
	assert.EqualValues(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+"."+testDomain+":5000", *instance.Status.ExternalCommonAddress, "Wrong external common address in status")
}

func TestEDSNoCommonCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:             solr.ExternalDNS,
					UseExternalAddress: true,
					HideCommon:         true,
					DomainName:         testDomain,
				},
				PodPort:           3000,
				CommonServicePort: 2000,
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Host Alias Tests
	assert.Nil(t, statefulSet.Spec.Template.Spec.HostAliases, "There is no need for host aliases because traffic is going directly to pods.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + instance.Namespace + "." + testDomain,
		"SOLR_PORT":      "3000",
		"SOLR_NODE_PORT": "3000",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, []string{"solr", "stop", "-p", "3000"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	testMapsEqual(t, "common service annotations", testCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 2000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "headless service annotations", expectedHeadlessServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 3000, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// Check that the Addresses in the status are correct
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())
	assert.Equal(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+":2000", instance.Status.InternalCommonAddress, "Wrong internal common address in status")
	assert.Nil(t, instance.Status.ExternalCommonAddress, "External common address in status should be nil")
}

func TestEDSUseInternalAddressCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:             solr.ExternalDNS,
					UseExternalAddress: false,
					DomainName:         testDomain,
					NodePortOverride:   454,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Host Alias Tests
	assert.Nil(t, statefulSet.Spec.Template.Spec.HostAliases, "There is no need for host aliases because traffic is going directly to pods.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + instance.Namespace,
		"SOLR_PORT":      "3000",
		"SOLR_NODE_PORT": "3000",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, []string{"solr", "stop", "-p", "3000"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")
	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "common service annotations", expectedCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 4000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "headless service annotations", expectedHeadlessServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 3000, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// Check that the Addresses in the status are correct
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())
	assert.Equal(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+":4000", instance.Status.InternalCommonAddress, "Wrong internal common address in status")
	assert.NotNil(t, instance.Status.ExternalCommonAddress, "External common address in Status should not be nil.")
	assert.EqualValues(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+"."+testDomain+":4000", *instance.Status.ExternalCommonAddress, "Wrong external common address in status")
}

func TestEDSExtraDomainsCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:                solr.ExternalDNS,
					UseExternalAddress:    true,
					DomainName:            testDomain,
					AdditionalDomainNames: testAdditionalDomains,
				},
				PodPort:           3000,
				CommonServicePort: 4000,
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Host Alias Tests
	assert.Nil(t, statefulSet.Spec.Template.Spec.HostAliases, "There is no need for host aliases because traffic is going directly to pods.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + instance.Namespace + "." + testDomain,
		"SOLR_PORT":      "3000",
		"SOLR_NODE_PORT": "3000",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, []string{"solr", "stop", "-p", "3000"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	hostnameAnnotation := instance.Namespace + "." + testDomain
	for _, domain := range testAdditionalDomains {
		hostnameAnnotation += "," + instance.Namespace + "." + domain
	}

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": hostnameAnnotation,
	})
	testMapsEqual(t, "common service annotations", expectedCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 4000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	expectedHeadlessServiceAnnotations := util.MergeLabelsOrAnnotations(testHeadlessServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": hostnameAnnotation,
	})
	testMapsEqual(t, "headless service annotations", expectedHeadlessServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 3000, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// Check that the Addresses in the status are correct
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())
	assert.Equal(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+":4000", instance.Status.InternalCommonAddress, "Wrong internal common address in status")
	assert.NotNil(t, instance.Status.ExternalCommonAddress, "External common address in Status should not be nil.")
	assert.EqualValues(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+"."+testDomain+":4000", *instance.Status.ExternalCommonAddress, "Wrong external common address in status")
}

func TestEDSKubeDomainCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:             solr.ExternalDNS,
					UseExternalAddress: true,
					HideNodes:          true,
					DomainName:         testDomain,
				},
				PodPort:           2000,
				CommonServicePort: 5000,
				KubeDomain:        testKubeDomain,
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Host Alias Tests
	assert.Nil(t, statefulSet.Spec.Template.Spec.HostAliases, "There is no need for host aliases because traffic is going directly to pods.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + instance.Namespace + ".svc." + testKubeDomain,
		"SOLR_PORT":      "2000",
		"SOLR_NODE_PORT": "2000",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, []string{"solr", "stop", "-p", "2000"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	expectedCommonServiceAnnotations := util.MergeLabelsOrAnnotations(testCommonServiceAnnotations, map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": instance.Namespace + "." + testDomain,
	})
	testMapsEqual(t, "common service annotations", expectedCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 5000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	testMapsEqual(t, "headless service annotations", testHeadlessServiceAnnotations, service.Annotations)
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].Name, "Wrong port name on common Service")
	assert.EqualValues(t, 2000, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// Check that the Addresses in the status are correct
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())
	assert.Equal(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+".svc."+testKubeDomain+":5000", instance.Status.InternalCommonAddress, "Wrong internal common address in status")
	assert.NotNil(t, instance.Status.ExternalCommonAddress, "External common address in Status should not be nil.")
	assert.EqualValues(t, "http://"+cloudCsKey.Name+"."+instance.Namespace+"."+testDomain+":5000", *instance.Status.ExternalCommonAddress, "Wrong external common address in status")
}
