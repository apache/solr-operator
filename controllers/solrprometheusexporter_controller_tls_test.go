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
	"crypto/md5"
	"fmt"
	"github.com/apache/solr-operator/controllers/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strings"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("SolrPrometheusExporter controller - TLS", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 5
		duration = time.Second * 1
		interval = time.Millisecond * 250
	)
	SetDefaultConsistentlyDuration(duration)
	SetDefaultConsistentlyPollingInterval(interval)
	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	var (
		ctx context.Context

		solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter
	)

	BeforeEach(func() {
		ctx = context.Background()

		solrPrometheusExporter = &solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrPrometheusExporterSpec{},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())

		By("defaulting the missing SolrCloud values")
		expectSolrPrometheusExporterWithChecks(ctx, solrPrometheusExporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.WithDefaults()).To(BeFalse(), "The SolrPrometheusExporter spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func() {
		cleanupTest(ctx, solrPrometheusExporter)
	})

	FContext("TLS Secret", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS: createTLSOptions(tlsSecretName, "keystore-passwords-are-important", false),
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTLS(ctx, solrPrometheusExporter, tlsSecretName, false)
		})
	})

	FContext("TLS Secret - PKCS12 Conversion", func() {
		tlsSecretName := "tls-cert-secret-from-user-no-pkcs12"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS: createTLSOptions(tlsSecretName, "keystore-passwords-are-important", false),
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTLS(ctx, solrPrometheusExporter, tlsSecretName, true)
		})
	})

	FContext("TLS Secret - Restart on Secret Update", func() {
		tlsSecretName := "tls-cert-secret-update"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS: createTLSOptions(tlsSecretName, "keystore-passwords-are-important", true),
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTLS(ctx, solrPrometheusExporter, tlsSecretName, false)
		})
	})

	FContext("TLS Secret - With BasicAuth", func() {
		tlsSecretName := "tls-cert-secret-update"
		basicAuthSecretName := tlsSecretName + "-basic-auth"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS:         createTLSOptions(tlsSecretName, "keystore-passwords-are-important", true),
					BasicAuthSecret: basicAuthSecretName,
				},
			}

			By("Creating secret to use with BasicAuth")
			Expect(k8sClient.Create(ctx, createBasicAuthSecret(basicAuthSecretName, solrv1beta1.DefaultBasicAuthUsername, solrPrometheusExporter.Namespace))).To(Succeed(), "Could not create basic auth secret")
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTLS(ctx, solrPrometheusExporter, tlsSecretName, false)
		})
	})

	FContext("TLS Secret - With BasicAuth - Restart on Secret Update", func() {
		tlsSecretName := "tls-cert-secret-update"
		basicAuthSecretName := tlsSecretName + "-basic-auth"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS:         createTLSOptions(tlsSecretName, "keystore-passwords-are-important", true),
					BasicAuthSecret: basicAuthSecretName,
				},
			}

			By("Creating secret to use with BasicAuth")
			Expect(k8sClient.Create(ctx, createBasicAuthSecret(basicAuthSecretName, solrv1beta1.DefaultBasicAuthUsername, solrPrometheusExporter.Namespace))).To(Succeed(), "Could not create basic auth secret")
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTLS(ctx, solrPrometheusExporter, tlsSecretName, false)

			By("expect Deployment to respond when changing the basicAuthKey")
			testChangingBasicAuthSecret(ctx, solrPrometheusExporter)
		})
	})

	FContext("TLS Secret - TrustStore Only", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS: &solrv1beta1.SolrTLSOptions{
						TrustStorePasswordSecret: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: tlsSecretName},
							Key:                  "keystore-passwords-are-important",
						},
						TrustStoreSecret: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: tlsSecretName},
							Key:                  util.DefaultPkcs12TruststoreFile,
						},
						RestartOnTLSSecretUpdate: false,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTruststoreOnly(ctx, solrPrometheusExporter, tlsSecretName)
		})
	})

	FContext("TLS Secret - TrustStore Only - Restart on Secret Update", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS: &solrv1beta1.SolrTLSOptions{
						TrustStorePasswordSecret: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: tlsSecretName},
							Key:                  "keystore-passwords-are-important",
						},
						TrustStoreSecret: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: tlsSecretName},
							Key:                  util.DefaultPkcs12TruststoreFile,
						},
						RestartOnTLSSecretUpdate: true,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			testReconcileWithTruststoreOnly(ctx, solrPrometheusExporter, tlsSecretName)
		})
	})

	FContext("Mounted TLS Certs", func() {
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
							ChRoot:                   "/this/path",
						},
					},
					SolrTLS: &solrv1beta1.SolrTLSOptions{
						MountedTLSDir: &solrv1beta1.MountedTLSDirectory{
							Path:                 "/mounted-tls-dir",
							KeystoreFile:         util.DefaultPkcs12KeystoreFile,
							TruststoreFile:       util.DefaultPkcs12TruststoreFile,
							KeystorePasswordFile: util.DefaultKeystorePasswordFile,
						},
						CheckPeerName:        true,
						ClientAuth:           "Need",
						VerifyClientHostname: false,
					},
				},
				BusyBoxImage: &solrv1beta1.ContainerImage{
					Repository: "myBBImage",
					Tag:        "test",
					PullPolicy: "",
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			deployment := expectDeployment(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName())

			podTemplate := deployment.Spec.Template

			// verify the mountedTLSDir config on the deployment
			Expect(podTemplate.Spec.Containers).To(Not(BeEmpty()), "The SolrPrometheusExporter requires containers")
			mainContainer := podTemplate.Spec.Containers[0]
			Expect(mainContainer.Env).To(Not(BeEmpty()), "No Env vars for main exporter container in the deployment!")
			// make sure JAVA_OPTS is set correctly with the TLS related sys props
			envVars := filterVarsByName(mainContainer.Env, func(n string) bool {
				return strings.HasPrefix(n, "JAVA_OPTS")
			})
			Expect(envVars).To(HaveLen(1), "Wrong number of Env vars with JAVA_OPTS")
			Expect(envVars[0].Value).To(And(
				ContainSubstring("-Djavax.net.ssl.trustStore=$(SOLR_SSL_CLIENT_TRUST_STORE)"),
				ContainSubstring("-Djavax.net.ssl.trustStoreType=PKCS12"),
				ContainSubstring("-Dsolr.ssl.checkPeerName=$(SOLR_SSL_CHECK_PEER_NAME)"),
				Not(ContainSubstring("-Djavax.net.ssl.keyStorePassword")),
				Not(ContainSubstring("-Djavax.net.ssl.trustStorePassword")),
			), "JAVA_OPTS does not contain correct SSL properties")

			// verify initContainer to create a wrapper script around the solr-exporter script
			initContainerName := "create-tls-wrapper-script"
			expInitContainer := expectInitContainer(&podTemplate, initContainerName, "tls-wrapper-script", "/usr/local/solr-exporter-tls")
			Expect(expInitContainer.Command).To(HaveLen(3), "Wrong command length for %s init container", initContainerName)

			Expect(expInitContainer.Command[2]).To(And(
				ContainSubstring("-Djavax.net.ssl.keyStorePassword"),
				ContainSubstring("-Djavax.net.ssl.trustStorePassword"),
				ContainSubstring("/opt/solr/contrib/prometheus-exporter/bin/solr-exporter"),
			), "Init container command does not have necessary arguments")

			Expect(expInitContainer.Image).To(Equal("myBBImage:test"), "Wrong image for BusyBox initContainer")

			By("testing the SolrPrometheusExporter Service")
			service := expectService(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsServiceName(), deployment.Spec.Selector.MatchLabels, false)
			Expect(service.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"), "Metrics Service Prometheus scraping is not enabled.")
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(util.SolrMetricsPort)), "Wrong target port on metrics Service")
			Expect(service.Spec.Ports[0].Name).To(Equal(util.SolrMetricsPortName), "Wrong port name on metrics Service")
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)), "Wrong port number on metrics Service")
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on metrics Service")
			Expect(service.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on metrics Service should not be nil")
			Expect(*service.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on metrics Service")
		})
	})
})

func testReconcileWithTLS(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, tlsSecretName string, needsPkcs12InitContainer bool) {
	keystorePassKey := "keystore-passwords-are-important"

	// create the TLS and keystore secrets needed for reconciling TLS options
	tlsKey := "keystore.p12"
	if needsPkcs12InitContainer {
		tlsKey = "tls.key" // to trigger the initContainer creation, don't want keystore.p12 in the secret
	}
	mockSecret := createMockTLSSecret(ctx, solrPrometheusExporter, tlsSecretName, tlsKey, keystorePassKey, "")

	deployment := expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
		expectTLSConfigOnPodTemplateWithGomega(g, solrPrometheusExporter.Spec.SolrReference.SolrTLS, &found.Spec.Template, needsPkcs12InitContainer, true, nil)
	})
	mainContainer := &deployment.Spec.Template.Spec.Containers[0]

	// make sure JAVA_OPTS is set correctly with the TLS related sys props
	envVars := filterVarsByName(mainContainer.Env, func(n string) bool {
		return strings.HasPrefix(n, "JAVA_OPTS")
	})
	Expect(envVars).To(HaveLen(1), "Wrong number of Env vars with JAVA_OPTS")
	Expect(envVars[0].Value).To(ContainSubstring("-Dsolr.ssl.checkPeerName=$(SOLR_SSL_CHECK_PEER_NAME)"), "JAVA_OPTS does not contain SSL property")

	if !solrPrometheusExporter.Spec.SolrReference.SolrTLS.RestartOnTLSSecretUpdate && solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret == "" {
		// shouldn't be any annotations on the podTemplateSpec if we're not tracking updates to the TLS secret
		Expect(deployment.Spec.Template.ObjectMeta.Annotations).To(BeEmpty(), "there shouldn't be any annotations on the podTemplateSpec if we're not tracking updates to the TLS secret")
		return
	}

	// Check that the hashshum annotations are correct on the pod
	expectedAnnotations := map[string]string{
		util.SolrClientTlsCertMd5Annotation: fmt.Sprintf("%x", md5.Sum(mockSecret.Data[util.TLSCertKey])),
	}

	if solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret != "" {
		lookupAuthSecret := expectSecret(ctx, solrPrometheusExporter, solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret)
		expectBasicAuthEnvVars(mainContainer.Env, lookupAuthSecret)

		// if auth enabled, then annotations also include an md5 for the password so exporters get restarted when it changes
		creds := fmt.Sprintf("%s:%s", lookupAuthSecret.Data[corev1.BasicAuthUsernameKey], lookupAuthSecret.Data[corev1.BasicAuthPasswordKey])
		expectedAnnotations[util.BasicAuthMd5Annotation] = fmt.Sprintf("%x", md5.Sum([]byte(creds)))
	}

	Expect(deployment.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "wrong pod annotations")

	foundTLSSecret := expectSecret(ctx, solrPrometheusExporter, solrPrometheusExporter.Spec.SolrReference.SolrTLS.PKCS12Secret.Name)

	// change the tls.crt which should trigger a rolling restart
	updatedTlsCertData := "certificate renewed"
	foundTLSSecret.Data[util.TLSCertKey] = []byte(updatedTlsCertData)
	Expect(k8sClient.Update(ctx, foundTLSSecret)).To(Succeed(), "Cannot renew certificate for TLS Cert Secret")

	expectedAnnotations[util.SolrClientTlsCertMd5Annotation] = fmt.Sprintf("%x", md5.Sum(foundTLSSecret.Data[util.TLSCertKey]))
	expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
		g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "wrong pod annotations after updating TLS Secret")
	})
}

func testChangingBasicAuthSecret(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter) {
	initialDeployment := expectDeployment(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName())

	// verify the pods would get restarted if the basic auth secret changes
	lookupBasicAuthSecret := expectSecret(ctx, solrPrometheusExporter, solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret)
	// change the tls.crt which should trigger a rolling restart
	updatedPassword := "updated-password"
	lookupBasicAuthSecret.Data[corev1.BasicAuthUsernameKey] = []byte(updatedPassword)
	Expect(k8sClient.Update(ctx, lookupBasicAuthSecret)).To(Succeed(), "Cannot update basicAuth secret")

	creds := string(lookupBasicAuthSecret.Data[corev1.BasicAuthUsernameKey]) + ":" + string(lookupBasicAuthSecret.Data[corev1.BasicAuthPasswordKey])
	expectedAnnotations := initialDeployment.Spec.Template.ObjectMeta.Annotations
	// Update the expected BasicAuthMd5 for new password
	expectedAnnotations[util.BasicAuthMd5Annotation] = fmt.Sprintf("%x", md5.Sum([]byte(creds)))
	// Check the annotation on the pod template to make sure a rolling restart will take place
	expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
		g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "wrong pod annotations after updating TLS Secret")
	})
}

func testReconcileWithTruststoreOnly(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, tlsSecretName string) {
	keystorePassKey := "keystore-passwords-are-important"
	// create the TLS and keystore secrets needed for reconciling TLS options
	tlsKey := util.DefaultPkcs12TruststoreFile
	mockSecret := createMockTLSSecret(ctx, solrPrometheusExporter, tlsSecretName, tlsKey, keystorePassKey, "")

	deployment := expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
		expectTLSConfigOnPodTemplateWithGomega(g, solrPrometheusExporter.Spec.SolrReference.SolrTLS, &found.Spec.Template, false, true, nil)
	})
	mainContainer := &deployment.Spec.Template.Spec.Containers[0]

	// make sure JAVA_OPTS is set correctly with the TLS related sys props
	envVars := filterVarsByName(mainContainer.Env, func(n string) bool {
		return strings.HasPrefix(n, "JAVA_OPTS")
	})
	Expect(envVars).To(HaveLen(1), "Wrong number of Env vars with JAVA_OPTS")
	Expect(envVars[0].Value).To(ContainSubstring("-Dsolr.ssl.checkPeerName=$(SOLR_SSL_CHECK_PEER_NAME)"), "JAVA_OPTS does not contain SSL property")

	if !solrPrometheusExporter.Spec.SolrReference.SolrTLS.RestartOnTLSSecretUpdate && solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret == "" {
		// shouldn't be any annotations on the podTemplateSpec if we're not tracking updates to the TLS secret
		Expect(deployment.Spec.Template.ObjectMeta.Annotations).To(BeEmpty(), "there shouldn't be any annotations on the podTemplateSpec if we're not tracking updates to the TLS secret")
		return
	}

	// Check that the hashshum annotations are correct on the pod
	expectedAnnotations := map[string]string{
		util.SolrClientTlsCertMd5Annotation: fmt.Sprintf("%x", md5.Sum(mockSecret.Data[tlsKey])),
	}

	if solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret != "" {
		lookupAuthSecret := expectSecret(ctx, solrPrometheusExporter, solrPrometheusExporter.Spec.SolrReference.BasicAuthSecret)
		expectBasicAuthEnvVars(mainContainer.Env, lookupAuthSecret)

		// if auth enabled, then annotations also include an md5 for the password so exporters get restarted when it changes
		creds := fmt.Sprintf("%s:%s", lookupAuthSecret.Data[corev1.BasicAuthUsernameKey], lookupAuthSecret.Data[corev1.BasicAuthPasswordKey])
		expectedAnnotations[util.BasicAuthMd5Annotation] = fmt.Sprintf("%x", md5.Sum([]byte(creds)))
	}

	Expect(deployment.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "wrong pod annotations")

	foundTLSSecret := expectSecret(ctx, solrPrometheusExporter, solrPrometheusExporter.Spec.SolrReference.SolrTLS.TrustStoreSecret.Name)

	// change the truststore.p12 which should trigger a rolling restart
	updatedTlsCertData := "certificate renewed"
	foundTLSSecret.Data[tlsKey] = []byte(updatedTlsCertData)
	Expect(k8sClient.Update(ctx, foundTLSSecret)).To(Succeed(), "Cannot update secret for TLS Truststore")

	expectedAnnotations[util.SolrClientTlsCertMd5Annotation] = fmt.Sprintf("%x", md5.Sum(foundTLSSecret.Data[tlsKey]))
	expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
		g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "wrong pod annotations after updating TLS Secret")
	})
}

func expectBasicAuthEnvVars(envVars []corev1.EnvVar, basicAuthSecret *corev1.Secret) {
	Expect(envVars).To(Not(BeEmpty()), "Env vars should not be empty when using basic auth")
	envVars = filterVarsByName(envVars, func(n string) bool {
		return n == "BASIC_AUTH_PASS" || n == "BASIC_AUTH_USER" || n == "JAVA_OPTS"
	})
	Expect(envVars).To(HaveLen(3), "Env vars should have values for each: BASIC_AUTH_PASS, BASIC_AUTH_USER, JAVA_OPTS")

	for _, envVar := range envVars {
		if envVar.Name == "JAVA_OPTS" {
			Expect(envVar.Value).To(ContainSubstring("-Dbasicauth=$(BASIC_AUTH_USER):$(BASIC_AUTH_PASS)"), "Expected basic auth creds in JAVA_OPTS")
		}

		if envVar.Name == "BASIC_AUTH_PASS" {
			Expect(envVar.Value).To(BeEmpty(), "Env var %s comes from a secret, so should not have an explicit value", envVar.Name)
			Expect(envVar.ValueFrom).To(Not(BeNil()), "Env var %s comes from a secret, so valueFrom should be used", envVar.Name)
			Expect(envVar.ValueFrom.SecretKeyRef).To(Not(BeNil()), "Env var %s comes from a secret, so valueFrom.secretKeyRef should be used", envVar.Name)
			Expect(envVar.ValueFrom.SecretKeyRef.Name).To(Equal(basicAuthSecret.Name), "Wrong secret name for basicAuth envVar %s", envVar.Name)
			Expect(envVar.ValueFrom.SecretKeyRef.Key).To(Equal(corev1.BasicAuthPasswordKey), "Wrong secret key for basicAuth envVar %s", envVar.Name)
		}

		if envVar.Name == "BASIC_AUTH_USER" {
			Expect(envVar.Value).To(BeEmpty(), "Env var %s comes from a secret, so should not have an explicit value", envVar.Name)
			Expect(envVar.ValueFrom).To(Not(BeNil()), "Env var %s comes from a secret, so valueFrom should be used", envVar.Name)
			Expect(envVar.ValueFrom.SecretKeyRef).To(Not(BeNil()), "Env var %s comes from a secret, so valueFrom.secretKeyRef should be used", envVar.Name)
			Expect(envVar.ValueFrom.SecretKeyRef.Name).To(Equal(basicAuthSecret.Name), "Wrong secret name for basicAuth envVar %s", envVar.Name)
			Expect(envVar.ValueFrom.SecretKeyRef.Key).To(Equal(corev1.BasicAuthUsernameKey), "Wrong secret key for basicAuth envVar %s", envVar.Name)
		}
	}
}
