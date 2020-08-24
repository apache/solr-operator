/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"testing"

	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/stretchr/testify/assert"

	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &SolrCloudReconciler{}

var (
	expectedCloudRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-clo", Namespace: "default"}}
	cloudSsKey           = types.NamespacedName{Name: "foo-clo-solrcloud", Namespace: "default"}
	cloudCsKey           = types.NamespacedName{Name: "foo-clo-solrcloud-common", Namespace: "default"}
	cloudHsKey           = types.NamespacedName{Name: "foo-clo-solrcloud-headless", Namespace: "default"}
	cloudIKey            = types.NamespacedName{Name: "foo-clo-solrcloud-common", Namespace: "default"}
	cloudCMKey           = types.NamespacedName{Name: "foo-clo-solrcloud-configmap", Namespace: "default"}
)

func TestCloudReconcile(t *testing.T) {
	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrJavaMem:  "-Xmx4G",
			SolrOpts:     "extra-opts",
			SolrLogLevel: "DEBUG",
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables:       extraVars,
					PodSecurityContext: &podSecurityContext,
					Volumes:            extraVolumes,
					Affinity:           affinity,
					Resources:          resources,
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
		"SOLR_HOST":      "$(POD_HOSTNAME)." + instance.HeadlessServiceName() + "." + instance.Namespace,
		"SOLR_JAVA_MEM":  "-Xmx4G",
		"SOLR_PORT":      "8983",
		"SOLR_LOG_LEVEL": "DEBUG",
		"SOLR_OPTS":      "extra-opts",
	}
	foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
	testPodEnvVariables(t, expectedEnvVars, foundEnv[:len(foundEnv)-2])
	assert.Equal(t, extraVars, foundEnv[len(foundEnv)-2:], "Extra Env Vars are not the same as the ones provided in podOptions")

	// Other Pod Options Checks
	assert.Equal(t, podSecurityContext, *statefulSet.Spec.Template.Spec.SecurityContext, "PodSecurityContext is not the same as the one provided in podOptions")
	assert.Equal(t, affinity, statefulSet.Spec.Template.Spec.Affinity, "Affinity is not the same as the one provided in podOptions")
	assert.Equal(t, resources.Limits, statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits, "Resources.Limits is not the same as the one provided in podOptions")
	assert.Equal(t, resources.Requests, statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests, "Resources.Requests is not the same as the one provided in podOptions")
	extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
	assert.Equal(t, len(extraVolumes)+1, len(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts), "Container has wrong number of volumeMounts")
	assert.Equal(t, extraVolumes[0].DefaultContainerMount, statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[1], "Additional Volume from podOptions not mounted into container properly.")
	assert.Equal(t, len(extraVolumes)+2, len(statefulSet.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, extraVolumes[0].Name, statefulSet.Spec.Template.Spec.Volumes[2].Name, "Additional Volume from podOptions not loaded into pod properly.")
	assert.Equal(t, extraVolumes[0].Source, statefulSet.Spec.Template.Spec.Volumes[2].VolumeSource, "Additional Volume from podOptions not loaded into pod properly.")

	// Check the client Service
	expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)

	// Check the headless Service
	expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)

	// Check the ingress
	expectNoIngress(g, cloudIKey)
}

func TestCustomKubeOptionsCloudReconcile(t *testing.T) {
	ingressBaseDomain := "ing.base.domain"
	SetIngressBaseUrl(ingressBaseDomain)
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	replicas := int32(4)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedCloudRequest.Name,
			Namespace: expectedCloudRequest.Namespace,
			Labels:    map[string]string{"base": "here"},
		},
		Spec: solr.SolrCloudSpec{
			Replicas: &replicas,
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrGCTune: "gc Options",
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					Annotations:    testPodAnnotations,
					Labels:         testPodLabels,
					Tolerations:    testTolerations,
					NodeSelector:   testNodeSelectors,
					LivenessProbe:  testProbeLivenessNonDefaults,
					ReadinessProbe: testProbeReadinessNonDefaults,
					StartupProbe:   testProbeStartup,
				},
				StatefulSetOptions: &solr.StatefulSetOptions{
					Annotations: testSSAnnotations,
					Labels:      testSSLabels,
				},
				CommonServiceOptions: &solr.ServiceOptions{
					Annotations: testCommonServiceAnnotations,
					Labels:      testCommonServiceLabels,
				},
				HeadlessServiceOptions: &solr.ServiceOptions{
					Annotations: testHeadlessServiceAnnotations,
					Labels:      testHeadlessServiceLabels,
				},
				NodeServiceOptions: &solr.ServiceOptions{
					Annotations: testNodeServiceAnnotations,
					Labels:      testNodeServiceLabels,
				},
				IngressOptions: &solr.IngressOptions{
					Annotations: testIngressAnnotations,
					Labels:      testIngressLabels,
				},
				ConfigMapOptions: &solr.ConfigMapOptions{
					Annotations: testConfigMapAnnotations,
					Labels:      testConfigMapLabels,
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
	// Add an additional check for reconcile, so that the services will have IP addresses for the hostAliases to use
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)
	assert.EqualValues(t, replicas, *statefulSet.Spec.Replicas, "Solr StatefulSet has incorrect number of replicas.")

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")
	expectedEnvVars := map[string]string{
		"ZK_HOST":   "host:7271/",
		"SOLR_HOST": "default-$(POD_HOSTNAME).ing.base.domain",
		"SOLR_PORT": "8983",
		"GC_TUNE":   "gc Options",
	}
	expectedStatefulSetLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"technology": "solr-cloud"})
	expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: "host:7271/"}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	testMapsEqual(t, "statefulSet labels", util.MergeLabelsOrAnnotations(expectedStatefulSetLabels, testSSLabels), statefulSet.Labels)
	testMapsEqual(t, "statefulSet annotations", util.MergeLabelsOrAnnotations(expectedStatefulSetAnnotations, testSSAnnotations), statefulSet.Annotations)
	testMapsEqual(t, "pod labels", util.MergeLabelsOrAnnotations(expectedStatefulSetLabels, testPodLabels), statefulSet.Spec.Template.ObjectMeta.Labels)
	testMapsEqual(t, "pod annotations", testPodAnnotations, statefulSet.Spec.Template.Annotations)
	testMapsEqual(t, "pod node selectors", testNodeSelectors, statefulSet.Spec.Template.Spec.NodeSelector)
	testPodProbe(t, testProbeLivenessNonDefaults, statefulSet.Spec.Template.Spec.Containers[0].LivenessProbe)
	testPodProbe(t, testProbeReadinessNonDefaults, statefulSet.Spec.Template.Spec.Containers[0].ReadinessProbe)
	assert.ElementsMatch(t, []string{"solr", "stop", "-p", "8983"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")
	testPodTolerations(t, testTolerations, statefulSet.Spec.Template.Spec.Tolerations)

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Selector.MatchLabels)
	expectedCommonServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "common"})
	testMapsEqual(t, "common service labels", util.MergeLabelsOrAnnotations(expectedCommonServiceLabels, testCommonServiceLabels), service.Labels)
	testMapsEqual(t, "common service annotations", testCommonServiceAnnotations, service.Annotations)

	// Check that the headless Service does not exist
	expectNoService(g, cloudHsKey, "Headless service shouldn't exist, but it does.")

	// Check the ingress
	ingress := expectIngress(g, requests, expectedCloudRequest, cloudIKey)
	testMapsEqual(t, "ingress labels", util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), testIngressLabels), ingress.Labels)
	testMapsEqual(t, "ingress annotations", testIngressAnnotations, ingress.Annotations)

	nodeNames := instance.GetAllSolrNodeNames()
	assert.EqualValues(t, replicas, len(nodeNames), "SolrCloud has incorrect number of nodeNames.")
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		service := expectService(t, g, requests, expectedCloudRequest, nodeSKey, util.MergeLabelsOrAnnotations(statefulSet.Spec.Selector.MatchLabels, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}))
		expectedNodeServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "external"})
		testMapsEqual(t, "node '"+nodeName+"' service labels", util.MergeLabelsOrAnnotations(expectedNodeServiceLabels, testNodeServiceLabels), service.Labels)
		testMapsEqual(t, "node '"+nodeName+"' service annotations", testNodeServiceAnnotations, service.Annotations)
	}

	// Check the configMap
	configMap := expectConfigMap(t, g, requests, expectedCloudRequest, cloudCMKey, map[string]string{})
	testMapsEqual(t, "configMap labels", util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), testConfigMapLabels), configMap.Labels)
	testMapsEqual(t, "configMap annotations", testConfigMapAnnotations, configMap.Annotations)
}

func TestCloudWithProvidedZookeeperReconcile(t *testing.T) {
	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ProvidedZookeeper: &solr.ProvidedZookeeper{
					ChRoot:    "a-ch/root",
					Zookeeper: &solr.ZookeeperSpec{},
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	// Blocked until https://github.com/pravega/zookeeper-operator/pull/99 is merged
	//g.Expect(zookeepercluster.AddZookeeperReconciler(mgr)).NotTo(gomega.HaveOccurred())

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
	// Add an additional check for reconcile, so that the zkCluster will have been created
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Check that the ZkConnectionInformation is correct
	expectedZkConnStr := "foo-clo-solrcloud-zookeeper-0.foo-clo-solrcloud-zookeeper-headless.default:2181,foo-clo-solrcloud-zookeeper-1.foo-clo-solrcloud-zookeeper-headless.default:2181,foo-clo-solrcloud-zookeeper-2.foo-clo-solrcloud-zookeeper-headless.default:2181"
	assert.Equal(t, expectedZkConnStr, instance.Status.ZookeeperConnectionInfo.InternalConnectionString, "Wrong zkConnectionString in status")
	assert.Equal(t, "/a-ch/root", instance.Status.ZookeeperConnectionInfo.ChRoot, "Wrong zk chRoot in status")
	assert.Nil(t, instance.Status.ZookeeperConnectionInfo.ExternalConnectionString, "Since a provided zk is used, the externalConnectionString in the status should be Nil")

	// Check that the statefulSet has not been created, because the ZkChRoot is not able to be created or verified
	expectNoStatefulSet(g, cloudSsKey)
}

func TestCloudWithExternalZookeeperChroot(t *testing.T) {
	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					ChRoot:                   "a-ch/root",
					InternalConnectionString: "host:7271,host2:7271",
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
	// Add an additional check for reconcile, so that the zkCluster will have been created
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Check that the ZkConnectionInformation is correct
	assert.Equal(t, "host:7271,host2:7271", instance.Status.ZookeeperConnectionInfo.InternalConnectionString, "Wrong internal zkConnectionString in status")
	assert.Equal(t, "host:7271,host2:7271", instance.Status.ZookeeperConnectionInfo.InternalConnectionString, "Wrong external zkConnectionString in status")
	assert.Equal(t, "/a-ch/root", instance.Status.ZookeeperConnectionInfo.ChRoot, "Wrong zk chRoot in status")
	assert.Equal(t, "host:7271,host2:7271/a-ch/root", instance.Status.ZookeeperConnectionInfo.ZkConnectionString(), "Wrong zkConnectionString())")

	// Check that the statefulSet has not been created, because the ZkChRoot is not able to be created or verified
	expectNoStatefulSet(g, cloudSsKey)
}

func TestDefaults(t *testing.T) {
	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ProvidedZookeeper: &solr.ProvidedZookeeper{
					Zookeeper: &solr.ZookeeperSpec{
						Replicas:                  nil,
						Image:                     nil,
						PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
						ZookeeperPod:              solr.ZookeeperPodPolicy{},
					},
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

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Solr defaults
	assert.Equal(t, solr.DefaultSolrReplicas, *instance.Spec.Replicas, "Bad Default - Spec.Replicas")
	assert.NotNil(t, instance.Spec.SolrImage, "Bad Default - instance.Spec.SolrImage")
	assert.Equal(t, solr.DefaultSolrRepo, instance.Spec.SolrImage.Repository, "Bad Default - instance.Spec.SolrImage.Repository")
	assert.Equal(t, solr.DefaultSolrVersion, instance.Spec.SolrImage.Tag, "Bad Default - instance.Spec.SolrImage.Tag")
	assert.NotNil(t, instance.Spec.BusyBoxImage, "Bad Default - instance.Spec.BusyBoxImage")
	assert.Equal(t, solr.DefaultBusyBoxImageRepo, instance.Spec.BusyBoxImage.Repository, "Bad Default - instance.Spec.BusyBoxImage.Repository")
	assert.Equal(t, solr.DefaultBusyBoxImageVersion, instance.Spec.BusyBoxImage.Tag, "Bad Default - instance.Spec.BusyBoxImage.Tag")
	assert.Equal(t, 80, instance.Spec.SolrAddressability.CommonServicePort, "Bad Default - instance.Spec.SolrAddressability.CommonServicePort")
	assert.Equal(t, 8983, instance.Spec.SolrAddressability.PodPort, "Bad Default - instance.Spec.SolrAddressability.PodPort")
	assert.Nil(t, instance.Spec.SolrAddressability.External, "Bad Default - instance.Spec.SolrAddressability.External")

	// Check the default Zookeeper
	assert.Equal(t, solr.DefaultZkReplicas, *instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Replicas, "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Replicas")
	assert.NotNil(t, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image")
	assert.Equal(t, solr.DefaultZkRepo, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Repository, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Repository")
	assert.Equal(t, solr.DefaultZkVersion, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Tag, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Tag")
	assert.NotNil(t, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence")
	assert.Equal(t, solr.DefaultZkVolumeReclaimPolicy, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy")
	assert.Equal(t, solr.DefaultZkVolumeReclaimPolicy, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy")
	assert.NotNil(t, instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec, "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec")
	assert.Equal(t, 1, len(instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.Resources.Requests), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.Resources length")
	assert.Equal(t, 1, len(instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.AccessModes), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.AccesModes length")
}

func TestIngressDefaults(t *testing.T) {
	SetIngressBaseUrl("test.ingress.url")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ProvidedZookeeper: &solr.ProvidedZookeeper{
					Zookeeper: &solr.ZookeeperSpec{
						Replicas:                  nil,
						Image:                     nil,
						PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
						ZookeeperPod:              solr.ZookeeperPodPolicy{},
					},
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
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	emptyRequests(requests)

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Solr defaults
	assert.Equal(t, 80, instance.Spec.SolrAddressability.CommonServicePort, "Bad Default - instance.Spec.SolrAddressability.CommonServicePort")
	assert.Equal(t, 8983, instance.Spec.SolrAddressability.PodPort, "Bad Default - instance.Spec.SolrAddressability.PodPort")
	assert.NotNil(t, instance.Spec.SolrAddressability.External, "Bad Default - instance.Spec.SolrAddressability.External")
	assert.Equal(t, false, instance.Spec.SolrAddressability.External.HideNodes, "Bad Default - instance.Spec.SolrAddressability.External.HideNodes")
	assert.Equal(t, false, instance.Spec.SolrAddressability.External.HideCommon, "Bad Default - instance.Spec.SolrAddressability.External.HideCommon")
	assert.Equal(t, "test.ingress.url", instance.Spec.SolrAddressability.External.DomainName, "Bad Default - instance.Spec.SolrAddressability.External.DomainName")
	assert.Equal(t, solr.Ingress, instance.Spec.SolrAddressability.External.Method, "Bad Default - instance.Spec.SolrAddressability.External.Method")
	assert.Equal(t, 80, instance.Spec.SolrAddressability.External.NodePortOverride, "Bad Default - instance.Spec.SolrAddressability.External.NodePortOverride")

	// Test NodeServicePort automatic change when using ExternalDNS
	instance.Spec.SolrAddressability.External = &solr.ExternalAddressability{}
	instance.Spec.SolrAddressability.External.Method = solr.ExternalDNS
	instance.Spec.SolrAddressability.External.DomainName = "test.ingress.url"
	instance.Spec.SolrAddressability.PodPort = 8000

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Update(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	emptyRequests(requests)

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Solr defaults
	assert.Equal(t, 80, instance.Spec.SolrAddressability.CommonServicePort, "Bad Default - instance.Spec.SolrAddressability.CommonServicePort")
	assert.Equal(t, 8000, instance.Spec.SolrAddressability.PodPort, "Bad Default - instance.Spec.SolrAddressability.PodPort")
	assert.NotNil(t, instance.Spec.SolrAddressability.External, "Bad Default - instance.Spec.SolrAddressability.External")
	assert.Equal(t, false, instance.Spec.SolrAddressability.External.HideNodes, "Bad Default - instance.Spec.SolrAddressability.External.HideNodes")
	assert.Equal(t, false, instance.Spec.SolrAddressability.External.HideCommon, "Bad Default - instance.Spec.SolrAddressability.External.HideCommon")
	assert.Equal(t, "test.ingress.url", instance.Spec.SolrAddressability.External.DomainName, "Bad Default - instance.Spec.SolrAddressability.External.DomainName")
	assert.Equal(t, solr.ExternalDNS, instance.Spec.SolrAddressability.External.Method, "Bad Default - instance.Spec.SolrAddressability.External.Method")
	assert.Equal(t, 0, instance.Spec.SolrAddressability.External.NodePortOverride, "Bad Default - instance.Spec.SolrAddressability.External.NodePortOverride")

	// Test NodePortOverride automatic set with an ingress that has nodes exposed.
	instance.Spec.SolrAddressability.External.Method = solr.Ingress
	instance.Spec.SolrAddressability.PodPort = 7000

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Update(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	emptyRequests(requests)

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Solr defaults
	assert.Equal(t, 80, instance.Spec.SolrAddressability.CommonServicePort, "Bad Default - instance.Spec.SolrAddressability.CommonServicePort")
	assert.Equal(t, 7000, instance.Spec.SolrAddressability.PodPort, "Bad Default - instance.Spec.SolrAddressability.PodPort")
	assert.NotNil(t, instance.Spec.SolrAddressability.External, "Bad Default - instance.Spec.SolrAddressability.External")
	assert.Equal(t, false, instance.Spec.SolrAddressability.External.HideNodes, "Bad Default - instance.Spec.SolrAddressability.External.HideNodes")
	assert.Equal(t, false, instance.Spec.SolrAddressability.External.HideCommon, "Bad Default - instance.Spec.SolrAddressability.External.HideCommon")
	assert.Equal(t, "test.ingress.url", instance.Spec.SolrAddressability.External.DomainName, "Bad Default - instance.Spec.SolrAddressability.External.DomainName")
	assert.Equal(t, solr.Ingress, instance.Spec.SolrAddressability.External.Method, "Bad Default - instance.Spec.SolrAddressability.External.Method")
	assert.Equal(t, 80, instance.Spec.SolrAddressability.External.NodePortOverride, "Bad Default - instance.Spec.SolrAddressability.External.NodePortOverride")
}

func TestExternalKubeDomainCloudReconcile(t *testing.T) {
	SetIngressBaseUrl("")
	UseEtcdCRD(false)
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

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":   "host:7271/",
		"SOLR_HOST": "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + instance.Namespace + ".svc." + testKubeDomain,
		"SOLR_PORT": "2000",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	assert.ElementsMatch(t, []string{"-DhostPort=2000"}, statefulSet.Spec.Template.Spec.Containers[0].Args, "Wrong Solr container arguments")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	testMapsEqual(t, "common service annotations", testCommonServiceAnnotations, service.Annotations)
	assert.EqualValues(t, 5000, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	testMapsEqual(t, "headless service annotations", testHeadlessServiceAnnotations, service.Annotations)
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
}
