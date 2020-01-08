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
	"github.com/stretchr/testify/assert"
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
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + instance.HeadlessServiceName(),
		"SOLR_JAVA_MEM":  "-Xmx4G",
		"SOLR_PORT":      "8983",
		"SOLR_LOG_LEVEL": "DEBUG",
		"SOLR_OPTS":      "extra-opts",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)

	// Check the client Service
	expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)

	// Check the headless Service
	expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)

	// Check the ingress
	expectNoIngress(g, requests, cloudIKey)
}

func TestCloudReconcileWithIngress(t *testing.T) {
	ingressBaseDomain := "ing.base.domain"
	SetIngressBaseUrl(ingressBaseDomain)
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
					ChRoot:                   "chroot/other",
				},
			},
			SolrGCTune: "gc Options",
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

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet require a container.")
	expectedEnvVars := map[string]string{
		"ZK_HOST":   "host:7271/chroot/other",
		"SOLR_HOST": instance.NodeIngressUrl("$(POD_HOSTNAME)", ingressBaseDomain),
		"SOLR_PORT": "8983",
		"GC_TUNE":   "gc Options",
	}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)

	// Check the client Service
	expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)

	// Check the headless Service
	expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)

	// Check the ingress
	expectIngress(g, requests, expectedCloudRequest, cloudIKey)
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
