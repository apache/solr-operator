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
	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"

	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	extraVars := []corev1.EnvVar{
		{
			Name:  "VAR_1",
			Value: "VAL_1",
		},
		{
			Name: "VAR_2",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "test.path",
				},
			},
		},
	}
	one := int64(1)
	two := int64(2)
	podSecurityContext := corev1.PodSecurityContext{
		RunAsUser:  &one,
		RunAsGroup: &two,
	}
	extraVolumes := []solr.AdditionalVolume{
		{
			Name: "vol1",
			Source: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
			DefaultContainerMount: corev1.VolumeMount{
				Name:      "ignore",
				ReadOnly:  false,
				MountPath: "/test/mount/path",
				SubPath:   "/sub",
			},
		},
	}
	affinity := &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					TopologyKey: "testKey",
				},
			},
			PreferredDuringSchedulingIgnoredDuringExecution: nil,
		},
	}
	resources := corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: *resource.NewMilliQuantity(5300, resource.DecimalSI),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceStorage: *resource.NewQuantity(1028*1028*1028, resource.BinarySI),
		},
	}

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
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + instance.HeadlessServiceName(),
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
	assert.Equal(t, extraVolumes[0].DefaultContainerMount, statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[1], "Additional Volume from podOptions not mounted into container properly.")
	assert.Equal(t, extraVolumes[0].Name, statefulSet.Spec.Template.Spec.Volumes[2].Name, "Additional Volume from podOptions not loaded into pod properly.")
	assert.Equal(t, extraVolumes[0].Source, statefulSet.Spec.Template.Spec.Volumes[2].VolumeSource, "Additional Volume from podOptions not loaded into pod properly.")

	// Check the client Service
	expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)

	// Check the headless Service
	expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)

	// Check the ingress
	expectNoIngress(g, requests, cloudIKey)
}

var (
	testPodAnnotations = map[string]string{
		"testP1": "valueP1",
		"testP2": "valueP2",
	}
	testPodLabels = map[string]string{
		"testP3": "valueP3",
		"testP4": "valueP4",
	}
	testSSAnnotations = map[string]string{
		"testSS1": "valueSS1",
		"testSS2": "valueSS2",
	}
	testSSLabels = map[string]string{
		"testSS3": "valueSS3",
		"testSS4": "valueSS4",
	}
	testIngressLabels = map[string]string{
		"testI1": "valueI1",
		"testI2": "valueI2",
	}
	testIngressAnnotations = map[string]string{
		"testI3": "valueI3",
		"testI4": "valueI4",
	}
	testCommonServiceLabels = map[string]string{
		"testCS1": "valueCS1",
		"testCS2": "valueCS2",
	}
	testCommonServiceAnnotations = map[string]string{
		"testCS3": "valueCS3",
		"testCS4": "valueCS4",
	}
	testHeadlessServiceLabels = map[string]string{
		"testHS1": "valueHS1",
		"testHS2": "valueHS2",
	}
	testHeadlessServiceAnnotations = map[string]string{
		"testHS3": "valueHS3",
		"testHS4": "valueHS4",
	}
	testNodeServiceLabels = map[string]string{
		"testNS1": "valueNS1",
		"testNS2": "valueNS2",
	}
	testNodeServiceAnnotations = map[string]string{
		"testNS3": "valueNS3",
		"testNS4": "valueNS4",
	}
	testConfigMapLabels = map[string]string{
		"testCM1": "valueCM1",
		"testCM2": "valueCM2",
	}
	testConfigMapAnnotations = map[string]string{
		"testCM3": "valueCM3",
		"testCM4": "valueCM4",
	}
)

func TestCloudReconcileWithIngress(t *testing.T) {
	ingressBaseDomain := "ing.base.domain"
	SetIngressBaseUrl(ingressBaseDomain)
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedCloudRequest.Name,
			Namespace: expectedCloudRequest.Namespace,
			Labels:    map[string]string{"base": "here"},
		},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrGCTune: "gc Options",
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					Annotations: testPodAnnotations,
					Labels:      testPodLabels,
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
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the services will have IP addresses for the hostAliases to use
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")
	expectedEnvVars := map[string]string{
		"ZK_HOST":   "host:7271/",
		"SOLR_HOST": instance.NodeIngressUrl("$(POD_HOSTNAME)", ingressBaseDomain),
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

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Selector.MatchLabels)
	expectedCommonServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "common"})
	testMapsEqual(t, "common service labels", util.MergeLabelsOrAnnotations(expectedCommonServiceLabels, testCommonServiceLabels), service.Labels)
	testMapsEqual(t, "common service annotations", testCommonServiceAnnotations, service.Annotations)

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Selector.MatchLabels)
	expectedHeadlessServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "headless"})
	testMapsEqual(t, "headless service labels", util.MergeLabelsOrAnnotations(expectedHeadlessServiceLabels, testHeadlessServiceLabels), service.Labels)
	testMapsEqual(t, "headless service annotations", testHeadlessServiceAnnotations, service.Annotations)

	// Check the ingress
	ingress := expectIngress(g, requests, expectedCloudRequest, cloudIKey)
	testMapsEqual(t, "ingress labels", util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), testIngressLabels), ingress.Labels)
	testMapsEqual(t, "ingress annotations", testIngressAnnotations, ingress.Annotations)

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
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the zkCluster will have been created
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Check that the ZkConnectionInformation is correct
	assert.Equal(t, instance.ProvidedZookeeperName()+"-client:2181", instance.Status.ZookeeperConnectionInfo.InternalConnectionString, "Wrong zkConnectionString in status")
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
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
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
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
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
