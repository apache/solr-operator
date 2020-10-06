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
	b64 "encoding/base64"
	"fmt"
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
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
)

func TestAutoCreateSelfSignedTLS(t *testing.T) {

	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(false)

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
			SolrTLS: &solr.SolrTLSOptions{
				AutoCreate: &solr.CreateCertificate{
					SubjectDistinguishedName: "CN=testCN, O=testO, OU=testOU",
				},
			},
		},
	}

	changed := instance.WithDefaults("")
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")
	// check the config gets setup correctly before reconcile
	verifyAutoCreateSelfSignedTLSConfig(t, instance)

	// config is good, reconcile the TLS state
	verifyReconcileSelfSignedTLS(t, instance)
}

func verifyReconcileSelfSignedTLS(t *testing.T, instance *solr.SolrCloud) {
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

	// this triggers the reconcile process, which should create a self-signed TLS cert
	err = testClient.Create(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(ctx, instance)

	// Create a mock secret in the background so the isCert ready function returns
	go func() {
		pkcs12Keystore := map[string][]byte{}
		// this is not a real pkck12 keystore but works for testing
		pkcs12Keystore["keystore.p12"] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore")))

		mockTLSSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace},
			Data:       pkcs12Keystore,
			Type:       corev1.SecretTypeOpaque,
		}
		_ = testClient.Create(ctx, &mockTLSSecret)
	}()

	// higher timeout needed b/c of cert reconciliation taking longer
	g.Eventually(requests, 9*time.Second).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))
	// clear so we can wait for update reconcile to occur
	emptyRequests(requests)

	// Cert was created?
	findByName := types.NamespacedName{Name: expectedCloudWithTLSRequest.Name + "-solr-tls", Namespace: expectedCloudWithTLSRequest.Namespace}
	foundCert := &certv1.Certificate{}
	g.Eventually(func() error { return testClient.Get(ctx, findByName, foundCert) }, timeout).Should(gomega.Succeed())
	err = testClient.Get(ctx, findByName, foundCert)
	verifySelfSignedCert(t, foundCert, instance.Name)

	expectStatefulSetTLSConfig(t, g, instance)

	// trigger an update to the TLS secret (since cert-manager is not actually issuing the cert for us during testing)

	// update the subject field and verify update occurs
	instance.Spec.SolrTLS.AutoCreate.SubjectDistinguishedName = "CN=testCN, O=testO, OU=testOU, S=testS"

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		// wait a max of 2 reconcile TLS cycles ~ 10 secs
		for i := 0; i < 10; i++ {
			tlsSecret := &corev1.Secret{}
			err := testClient.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, tlsSecret)
			if err != nil && errors.IsNotFound(err) {
				pkcs12Keystore := map[string][]byte{}
				// this is not a real pkcs12 keystore but works for testing
				pkcs12Keystore["keystore.p12"] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore updated")))
				mockTLSSecret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace},
					Data:       pkcs12Keystore,
					Type:       corev1.SecretTypeOpaque,
				}
				_ = testClient.Create(ctx, &mockTLSSecret)
				time.Sleep(6 * time.Second) // wait a full TLS reconcile cycle ...
				wg.Done()
				return
			} else {
				time.Sleep(1 * time.Second)
			}
		}
	}()

	err = testClient.Update(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))
	g.Eventually(requests, 6*time.Second).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))
	wg.Wait() // waits to make sure the TLS secret was created in the background

	// was the cert updated after the subject changed?
	findByName = types.NamespacedName{Name: expectedCloudWithTLSRequest.Name + "-solr-tls", Namespace: expectedCloudWithTLSRequest.Namespace}
	foundCert = &certv1.Certificate{}
	g.Eventually(func() error { return testClient.Get(ctx, findByName, foundCert) }, timeout).Should(gomega.Succeed())
	err = testClient.Get(ctx, findByName, foundCert)
	verifySelfSignedCert(t, foundCert, instance.Name)
	// was the change to the subject picked up?
	assert.Equal(t, "testS", foundCert.Spec.Subject.Provinces[0])

	// ensure the STS was updated after the cert changed
	mainContainer := expectStatefulSetTLSConfig(t, g, instance)

	// make sure the env var that tracks the TLS secret version was updated ...
	secretVersion := FilterVarsByName(mainContainer.Env, func(n string) bool {
		return n == "SOLR_TLS_SECRET_VERS"
	})
	assert.NotNil(t, secretVersion)
	foundTLSSecret := &corev1.Secret{}
	err = testClient.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, foundTLSSecret)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Equal(t, foundTLSSecret.ResourceVersion, secretVersion[0].Value)
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
	expectTLSEnvVars(t, util.TLSEnvVars(tls), fmt.Sprintf("%s-pkcs12-keystore", sc.Name), "password-key")
}

func verifySelfSignedCert(t *testing.T, cert *certv1.Certificate, solrCloudName string) {
	assert.Equal(t, solrCloudName+"-solr-tls", cert.Name)
	assert.Equal(t, "testCN", cert.Spec.CommonName)
	assert.NotNil(t, cert.Spec.Subject)
	assert.Equal(t, "testO", cert.Spec.Subject.Organizations[0])
	assert.Equal(t, "testOU", cert.Spec.Subject.OrganizationalUnits[0])
	assert.True(t, len(cert.Spec.DNSNames) == 3)
	assert.Equal(t, "*.test.domain.com", cert.Spec.DNSNames[0])
	assert.Equal(t, fmt.Sprintf("default-%s-solrcloud.test.domain.com", solrCloudName), cert.Spec.DNSNames[1])
	assert.Equal(t, "test.domain.com", cert.Spec.DNSNames[2])
	assert.Equal(t, fmt.Sprintf("%s-selfsigned-solr-tls", solrCloudName), cert.Spec.SecretName)
	assert.NotNil(t, cert.Spec.Keystores)
	assert.NotNil(t, cert.Spec.Keystores.PKCS12)
	assert.Equal(t, fmt.Sprintf("%s-pkcs12-keystore", solrCloudName), cert.Spec.Keystores.PKCS12.PasswordSecretRef.Name)
	assert.Equal(t, "password-key", cert.Spec.Keystores.PKCS12.PasswordSecretRef.Key)
	assert.Equal(t, fmt.Sprintf("%s-selfsigned-issuer", solrCloudName), cert.Spec.IssuerRef.Name)
	assert.Equal(t, "Issuer", cert.Spec.IssuerRef.Kind)
}

// For TLS, all we really need is a secret holding the keystore password and a secret holding the pkcs12 keystore,
// which can come from anywhere really, so this method tests handling of user-supplied secrets
func TestUserSuppliedTLS(t *testing.T) {

	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(false)

	replicas := int32(1)
	tlsSecretName := "tls-cert-secret-from-user"

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudWithTLSRequest.Name, Namespace: expectedCloudWithTLSRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			Replicas: &replicas,
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrTLS: &solr.SolrTLSOptions{
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
			},
		},
	}

	changed := instance.WithDefaults("")
	assert.True(t, changed, "WithDefaults should have changed the test SolrCloud instance")
	verifyUserSuppliedTLSConfig(t, instance, tlsSecretName, "some-password-key-thingy", tlsSecretName)
	verifyReconcileUserSuppliedTLS(t, instance)
}

func verifyReconcileUserSuppliedTLS(t *testing.T, instance *solr.SolrCloud) {
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
	pkcs12Keystore := map[string][]byte{}
	pkcs12Keystore["keystore.p12"] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore")))
	pkcs12Keystore[instance.Spec.SolrTLS.KeyStorePasswordSecret.Key] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore password")))

	mockTLSSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace},
		Data:       pkcs12Keystore,
		Type:       corev1.SecretTypeOpaque,
	}

	err = testClient.Create(ctx, &mockTLSSecret)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// now try to reconcile
	err = testClient.Create(ctx, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(ctx, instance)

	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudWithTLSRequest)))

	expectStatefulSetTLSConfig(t, g, instance)
}

func verifyUserSuppliedTLSConfig(t *testing.T, sc *solr.SolrCloud, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string, expectedTlsSecretName string) {
	assert.NotNil(t, sc.Spec.SolrTLS)
	tls := sc.Spec.SolrTLS
	assert.Nil(t, tls.AutoCreate)

	assert.Equal(t, expectedKeystorePasswordSecretName, tls.KeyStorePasswordSecret.Name)
	assert.Equal(t, expectedKeystorePasswordSecretKey, tls.KeyStorePasswordSecret.Key)
	assert.Equal(t, expectedTlsSecretName, tls.PKCS12Secret.Name)
	assert.Equal(t, "keystore.p12", tls.PKCS12Secret.Key)

	expectTLSEnvVars(t, util.TLSEnvVars(tls), expectedKeystorePasswordSecretName, expectedKeystorePasswordSecretKey)
}

// ensures the TLS settings are applied correctly to the STS
func expectStatefulSetTLSConfig(t *testing.T, g *gomega.GomegaWithT, sc *solr.SolrCloud) *corev1.Container {
	ctx := context.TODO()
	// expect the StatefulSet to have a Volume / VolumeMount for the TLS secret
	stateful := &appsv1.StatefulSet{}
	statefulSetKey := types.NamespacedName{Name: "foo-tls-solrcloud", Namespace: "default"}
	g.Eventually(func() error { return testClient.Get(ctx, statefulSetKey, stateful) }, timeout).Should(gomega.Succeed())

	assert.NotNil(t, stateful.Spec.Template.Spec.Volumes)
	var keystoreVol *corev1.Volume = nil
	for _, vol := range stateful.Spec.Template.Spec.Volumes {
		if vol.Name == "keystore" {
			keystoreVol = &vol
			break
		}
	}
	assert.NotNil(t, keystoreVol, "Didn't find TLS keystore volume in sts config!")
	assert.NotNil(t, keystoreVol.VolumeSource.Secret)
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
	assert.NotNil(t, mainContainer, "Didn't find the main solrcloud-node container in the sts!")
	expectTLSEnvVars(t, mainContainer.Env, sc.Spec.SolrTLS.KeyStorePasswordSecret.Name, sc.Spec.SolrTLS.KeyStorePasswordSecret.Key)

	return mainContainer
}

// ensure the TLS related env vars are set for the Solr pod
func expectTLSEnvVars(t *testing.T, envVars []corev1.EnvVar, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string) {
	assert.NotNil(t, envVars)
	envVars = FilterVarsByName(envVars, func(n string) bool {
		return strings.HasPrefix(n, "SOLR_SSL_")
	})
	assert.True(t, len(envVars) == 9)

	for _, envVar := range envVars {
		if envVar.Name == "SOLR_SSL_ENABLED" {
			assert.Equal(t, "true", envVar.Value)
		}

		if envVar.Name == "SOLR_SSL_TRUST_STORE" {
			assert.Equal(t, "/var/solr/tls/keystore.p12", envVar.Value)
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
func FilterVarsByName(envVars []corev1.EnvVar, f func(string) bool) []corev1.EnvVar {
	filtered := make([]corev1.EnvVar, 0)
	for _, v := range envVars {
		if f(v.Name) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}
