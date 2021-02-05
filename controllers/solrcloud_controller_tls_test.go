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
	solr "github.com/apache/lucene-solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sync"
	"testing"
)

var _ reconcile.Reconciler = &SolrCloudReconciler{}

var (
	expectedCloudWithTLSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-tls", Namespace: "default"}}
	expectedIngressWithTLS      = types.NamespacedName{Name: "foo-tls-solrcloud-common", Namespace: "default"}
	expectedStatefulSetName     = types.NamespacedName{Name: "foo-tls-solrcloud", Namespace: "default"}
)

// For TLS, all we really need is a secret holding the keystore password and a secret holding the pkcs12 keystore,
// which can come from anywhere really, so this method tests handling of user-supplied secrets
func TestUserSuppliedTLSSecretWithPkcs12Keystore(t *testing.T) {
	tlsSecretName := "tls-cert-secret-from-user"
	keystorePassKey := "some-password-key-thingy"
	instance := buildTestSolrCloud()
	instance.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
	verifyUserSuppliedTLSConfig(t, instance.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName, false)
	verifyReconcileUserSuppliedTLS(t, instance, false)
}

// Test upgrade from non-TLS cluster to TLS enabled cluster
func TestEnableTLSOnExistingCluster(t *testing.T) {

	instance := buildTestSolrCloud()
	changed := instance.WithDefaults()
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")

	g := gomega.NewGomegaWithT(t)
	helper := NewTLSTestHelper(g)
	defer func() {
		helper.StopTest()
	}()

	ctx := context.TODO()
	helper.ReconcileSolrCloud(ctx, instance, 2)
	defer testClient.Delete(ctx, instance)

	// now, update the config to enable TLS
	tlsSecretName := "tls-cert-secret-from-user"
	keystorePassKey := "some-password-key-thingy"

	err := testClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	instance.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, true)
	foundTLSSecret := &corev1.Secret{}
	lookupErr := testClient.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, foundTLSSecret)
	// TLS secret should not exist
	assert.True(t, errors.IsNotFound(lookupErr))

	// apply the update to trigger the upgrade to https
	err = testClient.Update(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Create a mock secret in the background so the isCert ready function returns
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, "keystore.p12", instance.Namespace, instance.Spec.SolrTLS.KeyStorePasswordSecret.Key)
		wg.Done()
	}()
	helper.WaitForReconcile(4)
	wg.Wait()

	expectStatefulSetTLSConfig(t, g, instance, false)
	expectIngressTLSConfig(t, g, tlsSecretName)
	expectUrlSchemeJob(t, g, instance)
}

func TestUserSuppliedTLSSecretWithPkcs12Conversion(t *testing.T) {
	tlsSecretName := "tls-cert-secret-from-user-no-pkcs12"
	keystorePassKey := "some-password-key-thingy"
	instance := buildTestSolrCloud()
	instance.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
	verifyUserSuppliedTLSConfig(t, instance.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName, true)
	verifyReconcileUserSuppliedTLS(t, instance, true)
}

func verifyReconcileUserSuppliedTLS(t *testing.T, instance *solr.SolrCloud, needsPkcs12InitContainer bool) {
	g := gomega.NewGomegaWithT(t)
	ctx := context.TODO()

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

	// create the secret required for reconcile, it has both keys ...
	tlsKey := "keystore.p12"
	if needsPkcs12InitContainer {
		tlsKey = "tls.key" // to trigger the initContainer creation, don't want keystore.p12 in the secret
	}

	_, err = createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, tlsKey, instance.Namespace, instance.Spec.SolrTLS.KeyStorePasswordSecret.Key)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// now try to reconcile
	err = testClient.Create(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(ctx, instance)

	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))

	expectStatefulSetTLSConfig(t, g, instance, needsPkcs12InitContainer)
}

// ensures the TLS settings are applied correctly to the STS
func expectStatefulSetTLSConfig(t *testing.T, g *gomega.GomegaWithT, sc *solr.SolrCloud, needsPkcs12InitContainer bool) *appsv1.StatefulSet {
	ctx := context.TODO()
	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(ctx, expectedStatefulSetName, stateful) }, timeout).Should(gomega.Succeed())
	expectTLSConfigOnPodTemplate(t, sc.Spec.SolrTLS, &stateful.Spec.Template, needsPkcs12InitContainer)
	return stateful
}

func expectIngressTLSConfig(t *testing.T, g *gomega.GomegaWithT, expectedTLSSecretName string) {
	ingress := &extv1.Ingress{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedIngressWithTLS, ingress) }, timeout).Should(gomega.Succeed())
	assert.True(t, ingress.Spec.TLS != nil && len(ingress.Spec.TLS) == 1)
	assert.Equal(t, expectedTLSSecretName, ingress.Spec.TLS[0].SecretName)
	assert.Equal(t, "HTTPS", ingress.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/backend-protocol"])
}

func expectUrlSchemeJob(t *testing.T, g *gomega.GomegaWithT, sc *solr.SolrCloud) {
	expectedJobName := types.NamespacedName{Name: sc.Name + "-set-https-scheme", Namespace: sc.Namespace}
	job := &batchv1.Job{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedJobName, job) }, timeout).Should(gomega.Succeed())
	assert.True(t, job.Spec.Template.Spec.Containers != nil && len(job.Spec.Template.Spec.Containers) == 1)
	assert.Equal(t, "run-zkcli", job.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "library/solr:7.7.0", job.Spec.Template.Spec.Containers[0].Image)
	assert.True(t, len(job.Spec.Template.Spec.Containers[0].Command) == 3)
	assert.True(t, len(job.Spec.Template.Spec.Containers[0].Env) == 3)
}

func buildTestSolrCloud() *solr.SolrCloud {
	replicas := int32(1)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudWithTLSRequest.Name, Namespace: expectedCloudWithTLSRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			Replicas: &replicas,
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrAddressability: solr.SolrAddressabilityOptions{
				External: &solr.ExternalAddressability{
					Method:             solr.Ingress,
					UseExternalAddress: true,
					DomainName:         testDomain,
					HideNodes:          true,
				},
			},
		},
	}
	return instance
}

// Consolidate common reconcile TLS test setup and behavior
type TLSTestHelper struct {
	g          *gomega.GomegaWithT
	mgr        manager.Manager
	requests   chan reconcile.Request
	stopMgr    chan struct{}
	mgrStopped *sync.WaitGroup
}

func NewTLSTestHelper(g *gomega.GomegaWithT) *TLSTestHelper {
	UseZkCRD(false)

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

	return &TLSTestHelper{
		g:          g,
		mgr:        mgr,
		requests:   requests,
		stopMgr:    stopMgr,
		mgrStopped: mgrStopped,
	}
}

func (helper *TLSTestHelper) StopTest() {
	close(helper.stopMgr)
	helper.mgrStopped.Wait()
}

func (helper *TLSTestHelper) WaitForReconcile(expectedRequests int) {
	g := helper.g
	requests := helper.requests
	for r := 0; r < expectedRequests; r++ {
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))
	}
	// clear so we can wait for update reconcile to occur
	emptyRequests(requests)
}

func (helper *TLSTestHelper) ReconcileSolrCloud(ctx context.Context, instance *solr.SolrCloud, expectedRequests int) {
	g := helper.g
	cleanupTest(g, instance.Namespace)

	// trigger the reconcile process and then wait for reconcile to finish
	// expectedRequests gives the expected number of requests created during the reconcile process
	err := testClient.Create(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Create a mock secret in the background so the isCert ready function returns
	var wg sync.WaitGroup
	if instance.Spec.SolrTLS != nil {
		wg.Add(1)
		go func() {
			createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, instance.Spec.SolrTLS.PKCS12Secret.Key, instance.Namespace, instance.Spec.SolrTLS.KeyStorePasswordSecret.Key)
			wg.Done()
		}()
	}
	helper.WaitForReconcile(expectedRequests)
	wg.Wait()

	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(ctx, expectedStatefulSetName, stateful) }, timeout).Should(gomega.Succeed())
}
