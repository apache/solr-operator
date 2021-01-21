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
	"crypto/md5"
	b64 "encoding/base64"
	"fmt"
	solr "github.com/apache/lucene-solr-operator/api/v1beta1"
	"github.com/apache/lucene-solr-operator/controllers/util"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
	"sync"
	"testing"
	"time"
)

var _ reconcile.Reconciler = &SolrCloudReconciler{}

var (
	expectedCloudWithTLSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-tls", Namespace: "default"}}
	expectedIngressWithTLS      = types.NamespacedName{Name: "foo-tls-solrcloud-common", Namespace: "default"}
	expectedStatefulSetName     = types.NamespacedName{Name: "foo-tls-solrcloud", Namespace: "default"}
)

func TestAutoCreateSelfSignedTLS(t *testing.T) {

	instance := buildTestSolrCloud()

	// Add the TLS config to create a self-signed cert
	instance.Spec.SolrTLS = &solr.SolrTLSOptions{
		AutoCreate: &solr.CreateCertificate{
			SubjectDistinguishedName: "CN=testCN, O=testO, OU=testOU",
		},
		RestartOnTLSSecretUpdate: true, // opt-in: restart the Solr pods when the TLS secret changes
	}

	changed := instance.WithDefaults()
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")

	// check the config gets setup correctly before reconcile
	verifyAutoCreateSelfSignedTLSConfig(t, instance)

	// config is good, reconcile the TLS state
	verifyReconcileSelfSignedTLS(t, instance)
}

// Test upgrade from non-TLS cluster to TLS enabled cluster
func TestEnableTLSOnExisting(t *testing.T) {

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
	err := testClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	instance.Spec.SolrTLS = &solr.SolrTLSOptions{
		AutoCreate: &solr.CreateCertificate{
			SubjectDistinguishedName: "CN=testCN, O=testO, OU=testOU",
		},
		RestartOnTLSSecretUpdate: true, // opt-in: restart the Solr pods when the TLS secret changes
	}
	changed = instance.WithDefaults()
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")

	// check the config gets setup correctly before reconcile
	verifyAutoCreateSelfSignedTLSConfig(t, instance)

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
		_ = createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, "keystore.p12", instance.Namespace)
		wg.Done()
	}()
	helper.WaitForReconcile(4)
	wg.Wait()

	// Cert was created?
	findByName := types.NamespacedName{Name: expectedCloudWithTLSRequest.Name + "-solr-tls", Namespace: expectedCloudWithTLSRequest.Namespace}
	foundCert := &certv1.Certificate{}
	g.Eventually(func() error { return testClient.Get(ctx, findByName, foundCert) }, timeout).Should(gomega.Succeed())
	err = testClient.Get(ctx, findByName, foundCert)

	verifySelfSignedCert(t, foundCert, instance.Name)
	expectStatefulSetTLSConfig(t, g, instance, false)
	expectIngressTLSConfig(t, g)
	expectUrlSchemeJob(t, g, instance)
}

// For TLS, all we really need is a secret holding the keystore password and a secret holding the pkcs12 keystore,
// which can come from anywhere really, so this method tests handling of user-supplied secrets
func TestUserSuppliedTLS(t *testing.T) {

	tlsSecretName := "tls-cert-secret-from-user"
	instance := buildTestSolrCloud()
	// Add the TLS config
	instance.Spec.SolrTLS = &solr.SolrTLSOptions{
		KeyStorePasswordSecret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: tlsSecretName,
			},
			Key: "some-password-key-thingy",
		},
		PKCS12Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: tlsSecretName,
			},
			Key: "keystore.p12",
		},
	}

	changed := instance.WithDefaults()
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")
	verifyUserSuppliedTLSConfig(t, instance, tlsSecretName, "some-password-key-thingy", tlsSecretName, false)
	verifyReconcileUserSuppliedTLS(t, instance, false)
}

func TestUserSuppliedTLSWithPkcs12Conversion(t *testing.T) {

	tlsSecretName := "tls-cert-secret-from-user-no-pkcs12"
	instance := buildTestSolrCloud()
	instance.Spec.SolrTLS = &solr.SolrTLSOptions{
		KeyStorePasswordSecret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: tlsSecretName,
			},
			Key: "some-password-key-thingy",
		},
		PKCS12Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: tlsSecretName,
			},
			Key: "keystore.p12",
		},
	}
	changed := instance.WithDefaults()
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")
	verifyUserSuppliedTLSConfig(t, instance, tlsSecretName, "some-password-key-thingy", tlsSecretName, true)
	verifyReconcileUserSuppliedTLS(t, instance, true)
}

func verifyReconcileSelfSignedTLS(t *testing.T, instance *solr.SolrCloud) {
	g := gomega.NewGomegaWithT(t)
	helper := NewTLSTestHelper(g)
	defer func() {
		helper.StopTest()
	}()

	ctx := context.TODO()
	helper.ReconcileSolrCloud(ctx, instance, 4)
	defer testClient.Delete(ctx, instance)

	// Cert was created?
	findByName := types.NamespacedName{Name: expectedCloudWithTLSRequest.Name + "-solr-tls", Namespace: expectedCloudWithTLSRequest.Namespace}
	foundCert := &certv1.Certificate{}
	g.Eventually(func() error { return testClient.Get(ctx, findByName, foundCert) }, timeout).Should(gomega.Succeed())
	err := testClient.Get(ctx, findByName, foundCert)
	verifySelfSignedCert(t, foundCert, instance.Name)
	expectStatefulSetTLSConfig(t, g, instance, false)
	expectIngressTLSConfig(t, g)
	expectUrlSchemeJob(t, g, instance)

	// trigger an update to the TLS secret (since cert-manager is not actually issuing the cert for us during testing)

	// update the subject field and verify update occurs
	err = testClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	instance.Spec.SolrTLS.AutoCreate.SubjectDistinguishedName = "CN=testCN, O=testO, OU=testOU, S=testS"

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// wait a max of 2 reconcile TLS cycles ~ 10 secs
		for i := 0; i < 10; i++ {
			tlsSecret := &corev1.Secret{}
			err := testClient.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, tlsSecret)
			if err != nil && errors.IsNotFound(err) {
				_ = createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, "keystore.p12", instance.Namespace)
				time.Sleep(2 * time.Second) // wait a full TLS reconcile cycle ...
				wg.Done()
				return
			} else {
				time.Sleep(1 * time.Second)
			}
		}
		// timed out waiting to see the secret get deleted after cert config changed, the test will fail
		wg.Done()
	}()

	err = testClient.Update(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	helper.WaitForReconcile(2)
	wg.Wait() // waits to make sure the TLS secret was created in the background

	// was the cert updated after the subject changed?
	findByName = types.NamespacedName{Name: expectedCloudWithTLSRequest.Name + "-solr-tls", Namespace: expectedCloudWithTLSRequest.Namespace}
	foundCert = &certv1.Certificate{}
	g.Eventually(func() error { return testClient.Get(ctx, findByName, foundCert) }, timeout).Should(gomega.Succeed())
	err = testClient.Get(ctx, findByName, foundCert)
	verifySelfSignedCert(t, foundCert, instance.Name)
	// was the change to the subject picked up?
	assert.NotNil(t, foundCert.Spec.Subject.Provinces, "Update cert subject not applied!")
	assert.Equal(t, "testS", foundCert.Spec.Subject.Provinces[0])

	// ensure the STS was updated after the cert changed
	statefulSet := expectStatefulSetTLSConfig(t, g, instance, false)
	tlsCertMd5FromSts := statefulSet.Spec.Template.Annotations[util.SolrTlsCertMd5Annotation]

	foundTLSSecret := &corev1.Secret{}
	err = testClient.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, foundTLSSecret)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	foundTlsCertMd5 := fmt.Sprintf("%x", md5.Sum([]byte(foundTLSSecret.Data["tls.crt"])))
	assert.Equal(t, foundTlsCertMd5, tlsCertMd5FromSts)
}

func verifyAutoCreateSelfSignedTLSConfig(t *testing.T, sc *solr.SolrCloud) {
	assert.NotNil(t, sc.Spec.SolrTLS)
	tls := sc.Spec.SolrTLS
	assert.NotNil(t, tls.AutoCreate)
	assert.Equal(t, sc.Name+"-solr-tls", tls.AutoCreate.Name)
	assert.Equal(t, fmt.Sprintf("%s-pkcs12-keystore", sc.Name), tls.KeyStorePasswordSecret.Name)
	assert.Equal(t, "password-key", tls.KeyStorePasswordSecret.Key)
	assert.Equal(t, fmt.Sprintf("%s-selfsigned-solr-tls", sc.Name), tls.PKCS12Secret.Name)
	assert.Equal(t, "keystore.p12", tls.PKCS12Secret.Key)

	cert := util.GenerateCertificate(sc)
	verifySelfSignedCert(t, &cert, sc.Name)
	expectTLSEnvVars(t, util.TLSEnvVars(tls, false), fmt.Sprintf("%s-pkcs12-keystore", sc.Name), "password-key", false)
}

func verifySelfSignedCert(t *testing.T, cert *certv1.Certificate, solrCloudName string) {
	assert.Equal(t, solrCloudName+"-solr-tls", cert.Name)
	assert.Equal(t, "testCN", cert.Spec.CommonName)
	assert.NotNil(t, cert.Spec.Subject)
	assert.Equal(t, "testO", cert.Spec.Subject.Organizations[0])
	assert.Equal(t, "testOU", cert.Spec.Subject.OrganizationalUnits[0])
	assert.True(t, len(cert.Spec.DNSNames) == 2)
	assert.Equal(t, "*.test.domain.com", cert.Spec.DNSNames[0])
	assert.Equal(t, "test.domain.com", cert.Spec.DNSNames[1])
	assert.Equal(t, fmt.Sprintf("%s-selfsigned-solr-tls", solrCloudName), cert.Spec.SecretName)
	assert.NotNil(t, cert.Spec.Keystores)
	assert.NotNil(t, cert.Spec.Keystores.PKCS12)
	assert.Equal(t, fmt.Sprintf("%s-pkcs12-keystore", solrCloudName), cert.Spec.Keystores.PKCS12.PasswordSecretRef.Name)
	assert.Equal(t, "password-key", cert.Spec.Keystores.PKCS12.PasswordSecretRef.Key)
	assert.Equal(t, fmt.Sprintf("%s-selfsigned-issuer", solrCloudName), cert.Spec.IssuerRef.Name)
	assert.Equal(t, "Issuer", cert.Spec.IssuerRef.Kind)
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

	err = createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, tlsKey, instance.Namespace)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// now try to reconcile
	err = testClient.Create(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(ctx, instance)

	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))

	expectStatefulSetTLSConfig(t, g, instance, needsPkcs12InitContainer)
}

func verifyUserSuppliedTLSConfig(t *testing.T, sc *solr.SolrCloud, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string, expectedTlsSecretName string, needsPkcs12InitContainer bool) {
	assert.NotNil(t, sc.Spec.SolrTLS)
	tls := sc.Spec.SolrTLS
	assert.Nil(t, tls.AutoCreate)

	assert.Equal(t, expectedKeystorePasswordSecretName, tls.KeyStorePasswordSecret.Name)
	assert.Equal(t, expectedKeystorePasswordSecretKey, tls.KeyStorePasswordSecret.Key)
	assert.Equal(t, expectedTlsSecretName, tls.PKCS12Secret.Name)
	assert.Equal(t, "keystore.p12", tls.PKCS12Secret.Key)

	expectTLSEnvVars(t, util.TLSEnvVars(tls, needsPkcs12InitContainer), expectedKeystorePasswordSecretName, expectedKeystorePasswordSecretKey, needsPkcs12InitContainer)
}

// ensures the TLS settings are applied correctly to the STS
func expectStatefulSetTLSConfig(t *testing.T, g *gomega.GomegaWithT, sc *solr.SolrCloud, needsPkcs12InitContainer bool) *appsv1.StatefulSet {
	ctx := context.TODO()
	// expect the StatefulSet to have a Volume / VolumeMount for the TLS secret
	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(ctx, expectedStatefulSetName, stateful) }, timeout).Should(gomega.Succeed())

	assert.NotNil(t, stateful.Spec.Template.Spec.Volumes)
	var keystoreVol *corev1.Volume = nil
	for _, vol := range stateful.Spec.Template.Spec.Volumes {
		if vol.Name == "keystore" {
			keystoreVol = &vol
			break
		}
	}
	assert.NotNil(t, keystoreVol)
	assert.NotNil(t, keystoreVol.VolumeSource.Secret, "Didn't find TLS keystore volume in sts config!")
	assert.Equal(t, sc.Spec.SolrTLS.PKCS12Secret.Name, keystoreVol.VolumeSource.Secret.SecretName)

	// check the SOLR_SSL_ related env vars on the sts
	assert.NotNil(t, stateful.Spec.Template.Spec.Containers)
	var mainContainer *corev1.Container = nil
	for _, cnt := range stateful.Spec.Template.Spec.Containers {
		if cnt.Name == "solrcloud-node" {
			mainContainer = &cnt
			break
		}
	}
	assert.NotNil(t, mainContainer.Env, "Didn't find the main solrcloud-node container in the sts!")
	expectTLSEnvVars(t, mainContainer.Env, sc.Spec.SolrTLS.KeyStorePasswordSecret.Name, sc.Spec.SolrTLS.KeyStorePasswordSecret.Key, needsPkcs12InitContainer)

	// initContainers
	if needsPkcs12InitContainer {
		var pkcs12Vol *corev1.Volume = nil
		for _, vol := range stateful.Spec.Template.Spec.Volumes {
			if vol.Name == "pkcs12" {
				pkcs12Vol = &vol
				break
			}
		}

		assert.NotNil(t, pkcs12Vol, "Didn't find TLS keystore volume in sts config!")
		assert.NotNil(t, pkcs12Vol.EmptyDir, "pkcs12 vol should by an emptyDir")

		assert.NotNil(t, stateful.Spec.Template.Spec.InitContainers)
		var expInitContainer *corev1.Container = nil
		for _, cnt := range stateful.Spec.Template.Spec.InitContainers {
			if cnt.Name == "gen-pkcs12-keystore" {
				expInitContainer = &cnt
				break
			}
		}
		expCmd := "openssl pkcs12 -export -in /var/solr/tls/tls.crt -in /var/solr/tls/ca.crt -inkey /var/solr/tls/tls.key -out /var/solr/tls/pkcs12/keystore.p12 -passout pass:${SOLR_SSL_KEY_STORE_PASSWORD}"
		assert.NotNil(t, expInitContainer, "Didn't find the gen-pkcs12-keystore InitContainer in the sts!")
		assert.Equal(t, expCmd, expInitContainer.Command[2])
	}

	return stateful
}

// ensure the TLS related env vars are set for the Solr pod
func expectTLSEnvVars(t *testing.T, envVars []corev1.EnvVar, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string, needsPkcs12InitContainer bool) {
	assert.NotNil(t, envVars)
	envVars = filterVarsByName(envVars, func(n string) bool {
		return strings.HasPrefix(n, "SOLR_SSL_")
	})
	assert.True(t, len(envVars) == 9)

	expectedKeystorePath := solr.DefaultKeyStorePath + "/keystore.p12"
	if needsPkcs12InitContainer {
		expectedKeystorePath = solr.DefaultWritableKeyStorePath + "/keystore.p12"
	}
	for _, envVar := range envVars {
		if envVar.Name == "SOLR_SSL_ENABLED" {
			assert.Equal(t, "true", envVar.Value)
		}

		if envVar.Name == "SOLR_SSL_TRUST_STORE" {
			assert.Equal(t, expectedKeystorePath, envVar.Value)
		}

		if envVar.Name == "SOLR_SSL_KEY_STORE_PASSWORD" {
			assert.NotNil(t, envVar.ValueFrom)
			assert.NotNil(t, envVar.ValueFrom.SecretKeyRef)
			assert.Equal(t, expectedKeystorePasswordSecretName, envVar.ValueFrom.SecretKeyRef.Name)
			assert.Equal(t, expectedKeystorePasswordSecretKey, envVar.ValueFrom.SecretKeyRef.Key)
		}
	}
}

// filter env vars by name using a supplied match function
func filterVarsByName(envVars []corev1.EnvVar, f func(string) bool) []corev1.EnvVar {
	filtered := make([]corev1.EnvVar, 0)
	for _, v := range envVars {
		if f(v.Name) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func expectIngressTLSConfig(t *testing.T, g *gomega.GomegaWithT) {
	ingress := &extv1.Ingress{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedIngressWithTLS, ingress) }, timeout).Should(gomega.Succeed())
	assert.True(t, ingress.Spec.TLS != nil && len(ingress.Spec.TLS) == 1)
	assert.Equal(t, "foo-tls-selfsigned-solr-tls", ingress.Spec.TLS[0].SecretName)
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

func createMockTLSSecret(ctx context.Context, apiClient client.Client, secretName string, secretKey string, ns string) error {
	secretData := map[string][]byte{}
	secretData[secretKey] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore")))
	secretData["tls.crt"] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore")))

	mockTLSSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Data:       secretData,
		Type:       corev1.SecretTypeOpaque,
	}
	return apiClient.Create(ctx, &mockTLSSecret)
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
			_ = createMockTLSSecret(ctx, testClient, instance.Spec.SolrTLS.PKCS12Secret.Name, instance.Spec.SolrTLS.PKCS12Secret.Key, instance.Namespace)
			wg.Done()
		}()
	}
	helper.WaitForReconcile(expectedRequests)
	wg.Wait()

	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(ctx, expectedStatefulSetName, stateful) }, timeout).Should(gomega.Succeed())
}
