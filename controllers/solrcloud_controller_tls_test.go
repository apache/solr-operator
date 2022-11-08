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
	b64 "encoding/base64"
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

var _ = FDescribe("SolrCloud controller - TLS", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		ctx = context.Background()

		replicas := int32(1)
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &replicas,
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						ChRoot:                   "tls-test",
						InternalConnectionString: "host:7271",
					},
				},
				SolrAddressability: solrv1beta1.SolrAddressabilityOptions{
					External: &solrv1beta1.ExternalAddressability{
						Method:             solrv1beta1.Ingress,
						UseExternalAddress: true,
						DomainName:         testDomain,
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

	FContext("Secret TLS - With Boostrap BasicAuth", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		keystorePassKey := "some-password-key-thingy"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic} // with basic-auth too
			solrCloud.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
		})
		FIt("has the correct resources", func() {
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.Spec.SolrAddressability.External.NodePortOverride).To(Equal(443), "Node port override wrong after Spec defaulting")
			})
			verifyUserSuppliedTLSConfig(solrCloud.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName)
			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloud, false, false)
			By("checking that the BasicAuth works as expected when using TLS")
			foundStatefulSet := expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)

			By("Checking that the Service has the correct settings")
			expectTLSService(ctx, solrCloud, foundStatefulSet.Spec.Selector.MatchLabels, true)

			By("Checking that the Ingress will passthrough with Server TLS")
			expectPassthroughIngressTLSConfig(ctx, solrCloud, tlsSecretName)

			By("Checking that the SolrCloud status has the correct scheme for URLs")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("https://"+solrCloud.Name+"-solrcloud-common."+solrCloud.Namespace), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in Status should not be nil.")
				g.Expect(*found.ExternalCommonAddress).To(Equal("https://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Secret TLS - With Boostrap BasicAuth - Custom Probes", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		keystorePassKey := "some-password-key-thingy"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
				ProbesRequireAuth:  true,
			}
			solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solrv1beta1.PodOptions{
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Scheme: corev1.URISchemeHTTPS,
							Path:   "/solr/admin/info/health",
							Port:   intstr.FromInt(8983),
						},
					},
				},
			}
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
			solrCloud.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
		})
		FIt("has the correct resources", func() {
			Expect(util.GetCustomProbePaths(solrCloud)).To(ConsistOf("/solr/admin/info/health"), "Utility Probe paths command gives wrong result")

			verifyUserSuppliedTLSConfig(solrCloud.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName)
			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloud, false, false)
			By("checking that the BasicAuth works as expected when using TLS")
			foundStatefulSet := expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)

			By("Checking that the Service has the correct settings")
			expectTLSService(ctx, solrCloud, foundStatefulSet.Spec.Selector.MatchLabels, true)
		})
	})

	FContext("Mounted TLS", func() {
		mountedDir := mountedTLSDir("/mounted-tls-dir")
		BeforeEach(func() {
			solrCloud.Spec.SolrTLS = &solrv1beta1.SolrTLSOptions{
				MountedTLSDir:        mountedDir,
				CheckPeerName:        true,
				ClientAuth:           "Need",
				VerifyClientHostname: true,
			}
		})
		FIt("has the correct resources", func() {
			By("checking that the Mounted TLS Config is correct in the generated StatefulSet")
			expectStatefulSetMountedTLSDirConfig(ctx, solrCloud)
		})
	})

	FContext("Mounted TLS - Non-default file names", func() {
		mountedDir := &solrv1beta1.MountedTLSDirectory{
			Path:                   "/mounted-non-default",
			KeystoreFile:           "ks.p12",
			TruststoreFile:         "ts.p12",
			KeystorePasswordFile:   "ks-password",
			TruststorePasswordFile: "ts-password",
		}
		BeforeEach(func() {
			solrCloud.Spec.SolrTLS = &solrv1beta1.SolrTLSOptions{
				MountedTLSDir:        mountedDir,
				CheckPeerName:        true,
				ClientAuth:           "Need",
				VerifyClientHostname: true,
			}
		})
		FIt("has the correct resources", func() {
			By("checking that the Mounted TLS Config is correct in the generated StatefulSet")
			expectStatefulSetMountedTLSDirConfig(ctx, solrCloud)

			By("Checking that the SolrCloud status has the correct scheme for URLs")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("https://"+solrCloud.Name+"-solrcloud-common."+solrCloud.Namespace), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in Status should not be nil.")
				g.Expect(*found.ExternalCommonAddress).To(Equal("https://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Mounted TLS - With Bootstrap BasicAuth", func() {
		mountedDir := mountedTLSDir("/mounted-tls-dir")
		BeforeEach(func() {
			solrCloud.Spec.SolrTLS = &solrv1beta1.SolrTLSOptions{
				MountedTLSDir:        mountedDir,
				CheckPeerName:        true,
				ClientAuth:           "Need",
				VerifyClientHostname: true,
			}
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
		})
		FIt("has the correct resources", func() {
			By("checking that the Mounted TLS Config is correct in the generated StatefulSet")
			expectStatefulSetMountedTLSDirConfig(ctx, solrCloud)
			By("checking that the BasicAuth works as expected when using TLS")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)
		})
	})

	FContext("Mounted TLS - Server and Client Certs", func() {
		mountedServerTLSDir := mountedTLSDir("/mounted-tls-dir")
		mountedClientTLSDir := mountedTLSDir("/mounted-client-tls-dir")
		BeforeEach(func() {
			solrCloud.Spec.SolrTLS = &solrv1beta1.SolrTLSOptions{
				MountedTLSDir:        mountedServerTLSDir,
				CheckPeerName:        true,
				ClientAuth:           "Need",
				VerifyClientHostname: true,
			}
			solrCloud.Spec.SolrClientTLS = &solrv1beta1.SolrTLSOptions{
				MountedTLSDir: mountedClientTLSDir,
				CheckPeerName: true,
			}
			solrCloud.Spec.SolrAddressability.External.HideNodes = true
		})
		FIt("has the correct resources", func() {
			By("checking that the Mounted TLS Config is correct in the generated StatefulSet")
			foundStatefulSet := expectStatefulSetMountedTLSDirConfig(ctx, solrCloud)

			By("Checking that the Service has the correct settings")
			expectTLSService(ctx, solrCloud, foundStatefulSet.Spec.Selector.MatchLabels, true)
		})
	})

	FContext("Secret TLS - Separate Truststore", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		keystorePassKey := "some-password-key-thingy"
		trustStoreSecretName := "custom-truststore-secret"
		trustStoreFile := "truststore.p12"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
			solrCloud.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
			solrCloud.Spec.SolrTLS.TrustStoreSecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: trustStoreSecretName},
				Key:                  trustStoreFile,
			}
			solrCloud.Spec.SolrTLS.TrustStorePasswordSecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: trustStoreSecretName},
				Key:                  "truststore-pass",
			}
			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Need // require client auth too (mTLS between the pods)
		})
		FIt("has the correct resources", func() {
			verifyUserSuppliedTLSConfig(solrCloud.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName)
			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloud, false, false)
			By("checking that the BasicAuth works as expected when using TLS")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)

			By("Checking that the Ingress will passthrough with Server TLS")
			expectPassthroughIngressTLSConfig(ctx, solrCloud, tlsSecretName)
		})
	})

	FContext("Secret TLS - Pkcs12 Conversion", func() {
		tlsSecretName := "tls-cert-secret-from-user-no-pkcs12"
		keystorePassKey := "some-password-key-thingy"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
			solrCloud.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
		})
		FIt("has the correct resources", func() {
			verifyUserSuppliedTLSConfig(solrCloud.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName)
			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloud, true, false)
			By("checking that the BasicAuth works as expected when using TLS")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)
		})
	})

	FContext("Secret TLS - Restart on Secret Update", func() {
		tlsSecretName := "tls-cert-secret-update"
		keystorePassKey := "some-password-key-thingy"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
			solrCloud.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, true)
			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Need
		})
		FIt("has the correct resources", func() {
			verifyUserSuppliedTLSConfig(solrCloud.Spec.SolrTLS, tlsSecretName, keystorePassKey, tlsSecretName)
			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloud, false, true)
			By("checking that the BasicAuth works as expected when using TLS")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)
		})
	})

	FContext("Secret TLS - Server & Client Certs - Restart on Secret Update", func() {
		serverCertSecret := "tls-server-cert"
		clientCertSecret := "tls-client-cert"
		keystorePassKey := "some-password-key-thingy"
		BeforeEach(func() {
			solrCloud.Spec.SolrTLS = createTLSOptions(serverCertSecret, keystorePassKey, true)
			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Need

			// Additional client cert
			solrCloud.Spec.SolrClientTLS = &solrv1beta1.SolrTLSOptions{
				KeyStorePasswordSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: clientCertSecret},
					Key:                  keystorePassKey,
				},
				PKCS12Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: clientCertSecret},
					Key:                  util.DefaultPkcs12KeystoreFile,
				},
				TrustStoreSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: clientCertSecret},
					Key:                  util.DefaultPkcs12TruststoreFile,
				},
				RestartOnTLSSecretUpdate: true,
			}
		})
		FIt("has the correct resources", func() {
			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloud, false, true)

			By("Checking that the Ingress will passthrough with Server TLS")
			expectPassthroughIngressTLSConfig(ctx, solrCloud, serverCertSecret)
		})
	})

	FContext("Secret TLS - Enable on Existing Cluster", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		keystorePassKey := "some-password-key-thingy"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
		})
		FIt("has the correct resources", func() {
			By("checking that the BasicAuth works as expected when using TLS")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)

			By("Checking that the SolrCloud status has the correct scheme for URLs before TLS is enabled")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.Name+"-solrcloud-common."+solrCloud.Namespace), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in Status should not be nil.")
				g.Expect(*found.ExternalCommonAddress).To(Equal("http://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud."+testDomain), "Wrong external common address in status")
			})

			By("Adding TLS to the existing cluster")
			solrCloudWithTlS := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.SolrTLS = createTLSOptions(tlsSecretName, keystorePassKey, false)
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Could not update SolrCloud to enable TLS")
			})

			By("checking that the User supplied TLS Config is correct in the generated StatefulSet")
			verifyReconcileUserSuppliedTLS(ctx, solrCloudWithTlS, false, false)

			By("checking that the BasicAuth works as expected when using TLS")
			expectStatefulSetBasicAuthConfig(ctx, solrCloudWithTlS, true)

			By("Checking that the Ingress will passthrough with Server TLS")
			expectPassthroughIngressTLSConfig(ctx, solrCloudWithTlS, tlsSecretName)

			By("Checking that the SolrCloud status has the correct scheme for URLs after TLS is enabled")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("https://"+solrCloud.Name+"-solrcloud-common."+solrCloud.Namespace+":80"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in Status should not be nil.")
				g.Expect(*found.ExternalCommonAddress).To(Equal("https://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud."+testDomain), "Wrong external common address in status")
			})
		})
	})

	FContext("Common Ingress TLS Termination", func() {
		tlsSecretName := "tls-cert-secret-from-user"
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
			solrCloud.Spec.SolrAddressability.External.IngressTLSTermination = &solrv1beta1.SolrIngressTLSTermination{
				TLSSecret: tlsSecretName,
			}
		})
		FIt("has the correct resources - Explicit Secret", func() {
			By("Checking that the Ingress will terminate TLS")
			expectTerminateIngressTLSConfig(ctx, solrCloud, tlsSecretName, false)

			By("Checking that the SolrCloud status has the correct scheme for URLs")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.Name+"-solrcloud-common."+solrCloud.Namespace), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in Status should not be nil.")
				g.Expect(*found.ExternalCommonAddress).To(Equal("https://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud."+testDomain), "Wrong external common address in status")
			})

			foundStatefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			By("Checking that the Service has the correct settings")
			expectTLSService(ctx, solrCloud, foundStatefulSet.Spec.Selector.MatchLabels, false)
		})
	})

	FContext("Common Ingress TLS Termination - Default Secret", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{AuthenticationType: solrv1beta1.Basic}
			solrCloud.Spec.SolrAddressability.External.IngressTLSTermination = &solrv1beta1.SolrIngressTLSTermination{
				UseDefaultTLSSecret: true,
			}
		})
		FIt("has the correct resources", func() {
			By("Checking that the Ingress will terminate TLS")
			expectTerminateIngressTLSConfig(ctx, solrCloud, "", false)

			By("Checking that the SolrCloud status has the correct scheme for URLs")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.Name+"-solrcloud-common."+solrCloud.Namespace), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(Not(BeNil()), "External common address in Status should not be nil.")
				g.Expect(*found.ExternalCommonAddress).To(Equal("https://"+solrCloud.Namespace+"-"+solrCloud.Name+"-solrcloud."+testDomain), "Wrong external common address in status")
			})

			foundStatefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			By("Checking that the Service has the correct settings")
			expectTLSService(ctx, solrCloud, foundStatefulSet.Spec.Selector.MatchLabels, false)
		})
	})
})

// Ensures all the TLS env vars, volume mounts and initContainers are setup for the PodTemplateSpec
func expectTLSConfigOnPodTemplate(tls *solrv1beta1.SolrTLSOptions, podTemplate *corev1.PodTemplateSpec, needsPkcs12InitContainer bool, clientOnly bool, clientTLS *solrv1beta1.SolrTLSOptions) *corev1.Container {
	return expectTLSConfigOnPodTemplateWithGomega(Default, tls, podTemplate, needsPkcs12InitContainer, clientOnly, clientTLS)
}

// Ensures all the TLS env vars, volume mounts and initContainers are setup for the PodTemplateSpec
func expectTLSConfigOnPodTemplateWithGomega(g Gomega, tls *solrv1beta1.SolrTLSOptions, podTemplate *corev1.PodTemplateSpec, needsPkcs12InitContainer bool, clientOnly bool, clientTLS *solrv1beta1.SolrTLSOptions) *corev1.Container {
	g.Expect(podTemplate.Spec.Volumes).To(Not(BeNil()), "Solr Pod Volumes should not be nil when using TLS")

	if tls.PKCS12Secret != nil {
		var keystoreVol *corev1.Volume = nil
		for _, vol := range podTemplate.Spec.Volumes {
			if vol.Name == "keystore" {
				keystoreVol = &vol
				break
			}
		}
		g.Expect(keystoreVol).To(Not(BeNil()), fmt.Sprintf("keystore volume not found in pod template; volumes: %v", podTemplate.Spec.Volumes))
		g.Expect(keystoreVol.VolumeSource.Secret).To(Not(BeNil()), "Didn't find TLS keystore volume in sts config! keystoreVol: "+keystoreVol.String())
		g.Expect(keystoreVol.VolumeSource.Secret.SecretName).To(Equal(tls.PKCS12Secret.Name), "Incorrect PKCS12Secret Secret")
		//g.Expect(keystoreVol.VolumeSource.Secret.Items).To(Not(BeEmpty()), "PKCS12Secret Secret Volume does not specify a key to mount")
		//g.Expect(keystoreVol.VolumeSource.Secret.Items[0].Key).To(Equal(tls.PKCS12Secret.Key), "Incorrect PKCS12Secret key in PKCS12 volume items")
	}

	// check the SOLR_SSL_ related env vars on the sts
	g.Expect(podTemplate.Spec.Containers).To(Not(BeEmpty()), "The Solr Pod should have at least 1 container")
	mainContainer := podTemplate.Spec.Containers[0]
	g.Expect(mainContainer).To(Not(BeNil()), "Didn't find the main solrcloud-node container in the sts!")
	g.Expect(mainContainer.Env).To(Not(BeEmpty()), "Didn't find the main solrcloud-node container in the sts!")

	// is there a separate truststore?
	expectedTrustStorePath := ""
	if tls.TrustStoreSecret != nil {
		expectedTrustStorePath = util.DefaultTrustStorePath + "/" + tls.TrustStoreSecret.Key
	}

	if tls.PKCS12Secret != nil {
		expectTLSEnvVarsWithGomega(g, mainContainer.Env, tls.KeyStorePasswordSecret.Name, tls.KeyStorePasswordSecret.Key, needsPkcs12InitContainer, expectedTrustStorePath, clientOnly, clientTLS)
	} else if tls.TrustStoreSecret != nil {
		envVars := mainContainer.Env
		g.Expect(envVars).To(Not(BeEmpty()), "Env vars for Pod should not be empty")
		envVars = filterVarsByName(envVars, func(n string) bool {
			return strings.HasPrefix(n, "SOLR_SSL_")
		})
		g.Expect(len(envVars)).To(Equal(3), "Wrong number of SSL env vars for Pod")
		for _, envVar := range envVars {
			if envVar.Name == "SOLR_SSL_CLIENT_TRUST_STORE" {
				g.Expect(envVar.Value).To(Equal(expectedTrustStorePath), "Wrong envVar value for %s", envVar.Name)
			}
			if envVar.Name == "SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD" {
				g.Expect(envVar.Value).To(BeEmpty(), "EnvVar %s should not use an explicit Value, since it is populated from a secret", envVar.Name)
				g.Expect(envVar.ValueFrom).To(Not(BeNil()), "EnvVar %s must have a ValueFrom, since it is populated from a secret", envVar.Name)
				g.Expect(envVar.ValueFrom.SecretKeyRef).To(Not(BeNil()), "EnvVar %s must have a ValueFrom.SecretKeyRef, since it is populated from a secret", envVar.Name)
				g.Expect(envVar.ValueFrom.SecretKeyRef.Name).To(Equal(tls.TrustStorePasswordSecret.Name), "EnvVar %s is using the wrong secret to populate the value", envVar.Name)
				g.Expect(envVar.ValueFrom.SecretKeyRef.Key).To(Equal(tls.TrustStorePasswordSecret.Key), "EnvVar %s is using the wrong secret key to populate the value", envVar.Name)
			}
		}
	}

	// different trust store?
	if tls.TrustStoreSecret != nil {
		var truststoreVol *corev1.Volume = nil
		for _, vol := range podTemplate.Spec.Volumes {
			if vol.Name == "truststore" {
				truststoreVol = &vol
				break
			}
		}
		g.Expect(truststoreVol).To(Not(BeNil()), fmt.Sprintf("truststore volume not found in pod template; volumes: %v", podTemplate.Spec.Volumes))
		g.Expect(truststoreVol.VolumeSource.Secret).To(Not(BeNil()), "Didn't find TLS truststore volume in sts config!")
		g.Expect(truststoreVol.VolumeSource.Secret.SecretName).To(Equal(tls.TrustStoreSecret.Name), "Truststore Volume is referencing the wrong Secret")
		//g.Expect(truststoreVol.VolumeSource.Secret.Items).To(Not(BeEmpty()), "truststore Secret Volume does not specify a key to mount")
		//g.Expect(truststoreVol.VolumeSource.Secret.Items[0].Key).To(Equal(tls.PKCS12Secret.Key), "Incorrect truststore Secret key in truststore volume items")
	}

	// initContainers
	if needsPkcs12InitContainer {
		var pkcs12Vol *corev1.Volume = nil
		for _, vol := range podTemplate.Spec.Volumes {
			if vol.Name == "pkcs12" {
				pkcs12Vol = &vol
				break
			}
		}

		g.Expect(pkcs12Vol).To(Not(BeNil()), "Didn't find TLS keystore volume in sts config!")
		g.Expect(pkcs12Vol.EmptyDir).To(Not(BeNil()), "pkcs12 vol should by an emptyDir")

		g.Expect(podTemplate.Spec.InitContainers).To(Not(BeEmpty()), "An init container should exist to do necessary SSL logic")
		var expInitContainer *corev1.Container = nil
		for _, cnt := range podTemplate.Spec.InitContainers {
			if cnt.Name == "gen-pkcs12-keystore" {
				expInitContainer = &cnt
				break
			}
		}
		expCmd := "openssl pkcs12 -export -in /var/solr/tls/tls.crt -in /var/solr/tls/ca.crt -inkey /var/solr/tls/tls.key -out /var/solr/tls/pkcs12/keystore.p12 -passout pass:${SOLR_SSL_KEY_STORE_PASSWORD}"
		g.Expect(expInitContainer).To(Not(BeNil()), "Didn't find the gen-pkcs12-keystore InitContainer in the sts!")
		g.Expect(expInitContainer.Command[2]).To(Equal(expCmd), "Wrong TLS initContainer command")
	}

	if tls.ClientAuth == solrv1beta1.Need {
		// verify the probes use a command with SSL opts
		tlsProps := ""
		if clientTLS != nil {
			tlsProps = "-Djavax.net.ssl.trustStore=$SOLR_SSL_CLIENT_TRUST_STORE -Djavax.net.ssl.keyStore=$SOLR_SSL_CLIENT_KEY_STORE" +
				" -Djavax.net.ssl.keyStorePassword=$SOLR_SSL_CLIENT_KEY_STORE_PASSWORD -Djavax.net.ssl.trustStorePassword=$SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD"
		} else {
			tlsProps = "-Djavax.net.ssl.trustStore=$SOLR_SSL_TRUST_STORE -Djavax.net.ssl.keyStore=$SOLR_SSL_KEY_STORE" +
				" -Djavax.net.ssl.keyStorePassword=$SOLR_SSL_KEY_STORE_PASSWORD -Djavax.net.ssl.trustStorePassword=$SOLR_SSL_TRUST_STORE_PASSWORD"
		}
		g.Expect(mainContainer.LivenessProbe).To(Not(BeNil()), "main container should have a liveness probe defined")
		g.Expect(mainContainer.LivenessProbe.Exec).To(Not(BeNil()), "liveness probe should have an exec when auth is enabled")
		g.Expect(mainContainer.LivenessProbe.Exec.Command).To(HaveLen(3), "liveness probe command has wrong number of args")
		g.Expect(mainContainer.LivenessProbe.Exec.Command[2]).To(ContainSubstring(tlsProps), "liveness probe should invoke java with SSL opts")
		g.Expect(mainContainer.ReadinessProbe).To(Not(BeNil()), "main container should have a readiness probe defined")
		g.Expect(mainContainer.ReadinessProbe.Exec).To(Not(BeNil()), "readiness probe should have an exec when auth is enabled")
		g.Expect(mainContainer.ReadinessProbe.Exec.Command).To(HaveLen(3), "readiness probe command has wrong number of args")
		g.Expect(mainContainer.ReadinessProbe.Exec.Command[2]).To(ContainSubstring(tlsProps), "readiness probe should invoke java with SSL opts")
	}

	return &mainContainer // return as a convenience in case tests want to do more checking on the main container
}

func expectMountedTLSDirEnvVars(envVars []corev1.EnvVar, solrCloud *solrv1beta1.SolrCloud) {
	Expect(envVars).To(Not(BeNil()), "Solr Pod must have env vars when using Mounted TLS Dir")
	envVars = filterVarsByName(envVars, func(n string) bool {
		return strings.HasPrefix(n, "SOLR_SSL_")
	})

	if solrCloud.Spec.SolrClientTLS != nil {
		Expect(len(envVars)).To(Equal(8), "expected SOLR_SSL and SOLR_SSL_CLIENT related env vars not found")
	} else {
		Expect(len(envVars)).To(Equal(6), "expected SOLR_SSL related env vars not found")
	}

	expectedKeystorePath := solrCloud.Spec.SolrTLS.MountedTLSDir.Path + "/" + solrCloud.Spec.SolrTLS.MountedTLSDir.KeystoreFile
	expectedTruststorePath := solrCloud.Spec.SolrTLS.MountedTLSDir.Path + "/" + solrCloud.Spec.SolrTLS.MountedTLSDir.TruststoreFile

	for _, envVar := range envVars {
		if envVar.Name == "SOLR_SSL_ENABLED" {
			Expect(envVar.Value).To(Equal("true"), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_KEY_STORE" {
			Expect(envVar.Value).To(Equal(expectedKeystorePath), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_TRUST_STORE" {
			Expect(envVar.Value).To(Equal(expectedTruststorePath), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_WANT_CLIENT_AUTH" {
			Expect(envVar.Value).To(Equal("false"), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_NEED_CLIENT_AUTH" {
			Expect(envVar.Value).To(Equal("true"), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_CHECK_PEER_NAME" {
			Expect(envVar.Value).To(Equal("true"), "Wrong envVar value for %s", envVar.Name)
		}
	}

	if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.MountedTLSDir != nil {
		for _, envVar := range envVars {
			if envVar.Name == "SOLR_SSL_CLIENT_KEY_STORE" {
				expectedKeystorePath = solrCloud.Spec.SolrClientTLS.MountedTLSDir.Path + "/" + solrCloud.Spec.SolrClientTLS.MountedTLSDir.KeystoreFile
				Expect(envVar.Value).To(Equal(expectedKeystorePath), "Wrong envVar value for %s", envVar.Name)
			}

			if envVar.Name == "SOLR_SSL_CLIENT_TRUST_STORE" {
				expectedTruststorePath = solrCloud.Spec.SolrClientTLS.MountedTLSDir.Path + "/" + solrCloud.Spec.SolrClientTLS.MountedTLSDir.TruststoreFile
				Expect(envVar.Value).To(Equal(expectedTruststorePath), "Wrong envVar value for %s", envVar.Name)
			}

		}
	}
}

// ensure the TLS related env vars are set for the Solr pod
func expectTLSEnvVars(envVars []corev1.EnvVar, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string, needsPkcs12InitContainer bool, expectedTruststorePath string, clientOnly bool, clientTLS *solrv1beta1.SolrTLSOptions) {
	expectTLSEnvVarsWithGomega(Default, envVars, expectedKeystorePasswordSecretName, expectedKeystorePasswordSecretKey, needsPkcs12InitContainer, expectedTruststorePath, clientOnly, clientTLS)
}

// ensure the TLS related env vars are set for the Solr pod
func expectTLSEnvVarsWithGomega(g Gomega, envVars []corev1.EnvVar, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string, needsPkcs12InitContainer bool, expectedTruststorePath string, clientOnly bool, clientTLS *solrv1beta1.SolrTLSOptions) {
	g.Expect(envVars).To(Not(BeNil()), "Env Vars should not be nil")
	envVars = filterVarsByName(envVars, func(n string) bool {
		return strings.HasPrefix(n, "SOLR_SSL_")
	})

	if clientOnly {
		g.Expect(len(envVars)).To(Equal(5), "Wrong number of SSL env vars when only using Client Certs")
	} else {
		if clientTLS != nil {
			g.Expect(len(envVars)).To(Equal(13), "Wrong number of SSL env vars when using Client & Server Certs")
		} else {
			g.Expect(len(envVars)).To(Equal(9), "Wrong number of SSL env vars when only using Server Certs")
		}
	}

	expectedKeystorePath := util.DefaultKeyStorePath + "/keystore.p12"
	if needsPkcs12InitContainer {
		expectedKeystorePath = util.DefaultWritableKeyStorePath + "/keystore.p12"
	}

	if expectedTruststorePath == "" {
		expectedTruststorePath = expectedKeystorePath
	}

	for _, envVar := range envVars {
		if envVar.Name == "SOLR_SSL_ENABLED" {
			g.Expect(envVar.Value).To(Equal("true"), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_KEY_STORE" {
			g.Expect(envVar.Value).To(Equal(expectedKeystorePath), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_TRUST_STORE" {
			g.Expect(envVar.Value).To(Equal(expectedTruststorePath), "Wrong envVar value for %s", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_KEY_STORE_PASSWORD" {
			g.Expect(envVar.Value).To(BeEmpty(), "EnvVar %s should not use an explicit Value, since it is populated from a secret", envVar.Name)
			g.Expect(envVar.ValueFrom).To(Not(BeNil()), "EnvVar %s must have a ValueFrom, since it is populated from a secret", envVar.Name)
			g.Expect(envVar.ValueFrom.SecretKeyRef).To(Not(BeNil()), "EnvVar %s must have a ValueFrom.SecretKeyRef, since it is populated from a secret", envVar.Name)
			g.Expect(envVar.ValueFrom.SecretKeyRef.Name).To(Equal(expectedKeystorePasswordSecretName), "EnvVar %s is using the wrong secret to populate the value", envVar.Name)
			g.Expect(envVar.ValueFrom.SecretKeyRef.Key).To(Equal(expectedKeystorePasswordSecretKey), "EnvVar %s is using the wrong secret key to populate the value", envVar.Name)
		}

		if envVar.Name == "SOLR_SSL_TRUST_STORE_STORE_PASSWORD" {
			g.Expect(envVar.Value).To(BeEmpty(), "EnvVar %s should not use an explicit Value, since it is populated from a secret", envVar.Name)
			g.Expect(envVar.ValueFrom).To(Not(BeNil()), "EnvVar %s must have a ValueFrom, since it is populated from a secret", envVar.Name)
			g.Expect(envVar.ValueFrom.SecretKeyRef).To(Not(BeNil()), "EnvVar %s must have a ValueFrom.SecretKeyRef, since it is populated from a secret", envVar.Name)
		}
	}

	if clientTLS != nil {
		for _, envVar := range envVars {

			if envVar.Name == "SOLR_SSL_CLIENT_KEY_STORE" {
				g.Expect(envVar.Value).To(Equal("/var/solr/client-tls/keystore.p12"), "Wrong envVar value for %s", envVar.Name)
			}

			if envVar.Name == "SOLR_SSL_CLIENT_TRUST_STORE" {
				g.Expect(envVar.Value).To(Equal("/var/solr/client-tls/truststore.p12"), "Wrong envVar value for %s", envVar.Name)
			}

			if envVar.Name == "SOLR_SSL_CLIENT_KEY_STORE_PASSWORD" || envVar.Name == "SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD" {
				g.Expect(envVar.Value).To(BeEmpty(), "EnvVar %s should not use an explicit Value, since it is populated from a secret", envVar.Name)
				g.Expect(envVar.ValueFrom).To(Not(BeNil()), "EnvVar %s must have a ValueFrom, since it is populated from a secret", envVar.Name)
				g.Expect(envVar.ValueFrom.SecretKeyRef).To(Not(BeNil()), "EnvVar %s must have a ValueFrom.SecretKeyRef, since it is populated from a secret", envVar.Name)
			}
		}
	}
}

func verifyUserSuppliedTLSConfig(tls *solrv1beta1.SolrTLSOptions, expectedKeystorePasswordSecretName string, expectedKeystorePasswordSecretKey string, expectedTlsSecretName string) {
	Expect(tls).To(Not(BeNil()), "TLS Options should not be nil")
	Expect(tls.KeyStorePasswordSecret.Name).To(Equal(expectedKeystorePasswordSecretName), "Incorrect Keystore Password Secret")
	Expect(tls.KeyStorePasswordSecret.Key).To(Equal(expectedKeystorePasswordSecretKey), "Incorrect Keystore Password Secret key")
	Expect(tls.PKCS12Secret.Name).To(Equal(expectedTlsSecretName), "Incorrect PKCS12Secret Secret")
	Expect(tls.PKCS12Secret.Key).To(Equal("keystore.p12"), "Incorrect PKCS12Secret Secret key")
}

func createTLSOptions(tlsSecretName string, keystorePassKey string, restartOnTLSSecretUpdate bool) *solrv1beta1.SolrTLSOptions {
	return &solrv1beta1.SolrTLSOptions{
		KeyStorePasswordSecret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: tlsSecretName},
			Key:                  keystorePassKey,
		},
		PKCS12Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: tlsSecretName},
			Key:                  util.DefaultPkcs12KeystoreFile,
		},
		RestartOnTLSSecretUpdate: restartOnTLSSecretUpdate,
	}
}

func createMockTLSSecret(ctx context.Context, parentObject client.Object, secretName string, secretKey string, keystorePasswordKey string, truststoreKey string) corev1.Secret {
	secretData := map[string][]byte{}
	secretData[secretKey] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore")))
	secretData[util.TLSCertKey] = []byte(b64.StdEncoding.EncodeToString([]byte("mock tls.crt")))

	if keystorePasswordKey != "" {
		secretData[keystorePasswordKey] = []byte(b64.StdEncoding.EncodeToString([]byte("mock keystore password")))
	}

	if truststoreKey != "" {
		secretData[truststoreKey] = []byte(b64.StdEncoding.EncodeToString([]byte("mock truststore")))
	}

	mockTLSSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: parentObject.GetNamespace()},
		Data:       secretData,
		Type:       corev1.SecretTypeOpaque,
	}
	Expect(k8sClient.Create(ctx, &mockTLSSecret)).To(Succeed(), "Could not create mock TLS Secret")
	return mockTLSSecret
}

func expectStatefulSetMountedTLSDirConfig(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) *appsv1.StatefulSet {
	statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
	podTemplate := &statefulSet.Spec.Template
	expectMountedTLSDirConfigOnPodTemplate(podTemplate, solrCloud)

	// Check HTTPS cluster prop setup container
	Expect(podTemplate.Spec.InitContainers).To(Not(BeEmpty()), "Init containers cannot be empty when using Mounted TLS")
	expectZkSetupInitContainerForTLS(solrCloud, statefulSet)

	// should have a mount for the initdb on main container
	expectInitdbVolumeMount(podTemplate)

	// verify initContainer to create initdb solrCloudript to export the keystore & truststore passwords before launching the main container
	name := "export-tls-password"
	expInitContainer := expectInitContainer(podTemplate, name, "initdb", util.InitdbPath)
	Expect(len(expInitContainer.Command)).To(Equal(3), "Wrong command length for %s init container command", name)
	Expect(expInitContainer.Command[2]).To(ContainSubstring("SOLR_SSL_KEY_STORE_PASSWORD"), "Wrong shell command for init container: %s", name)
	Expect(expInitContainer.Command[2]).To(ContainSubstring("SOLR_SSL_TRUST_STORE_PASSWORD"), "Wrong shell command for init container: %s", name)

	if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.MountedTLSDir != nil {
		Expect(expInitContainer.Command[2]).To(ContainSubstring("SOLR_SSL_CLIENT_KEY_STORE_PASSWORD"), "Wrong shell command for init container: %s", name)
		Expect(expInitContainer.Command[2]).To(ContainSubstring("SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD"), "Wrong shell command for init container: %s", name)
	} else {
		Expect(expInitContainer.Command[2]).To(Not(ContainSubstring("SOLR_SSL_CLIENT_KEY_STORE_PASSWORD")), "Wrong shell command for init container: %s", name)
		Expect(expInitContainer.Command[2]).To(Not(ContainSubstring("SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD")), "Wrong shell command for init container: %s", name)
	}
	return statefulSet
}

func expectMountedTLSDirConfigOnPodTemplate(podTemplate *corev1.PodTemplateSpec, solrCloud *solrv1beta1.SolrCloud) {
	Expect(podTemplate.Spec.Containers).To(Not(BeEmpty()), "Solr Pod must have containers")
	mainContainer := podTemplate.Spec.Containers[0]
	Expect(mainContainer).To(Not(BeNil()), "Didn't find the main solrcloud-node container in the sts!")
	Expect(mainContainer.Env).To(Not(BeEmpty()), "No Env vars for main solrcloud-node container in the sts!")
	expectMountedTLSDirEnvVars(mainContainer.Env, solrCloud)

	// verify the probes use a command with SSL opts
	tlsJavaToolOpts, tlsJavaSysProps := "", ""
	// if there's a client cert, then the probe should use that, else uses the server cert
	if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.MountedTLSDir != nil {
		expectedKeystorePasswordFile := solrCloud.Spec.SolrClientTLS.MountedTLSDir.Path + "/" + solrCloud.Spec.SolrClientTLS.MountedTLSDir.KeystorePasswordFile
		expectedTruststorePasswordFile := solrCloud.Spec.SolrClientTLS.MountedTLSDir.Path + "/"
		if solrCloud.Spec.SolrClientTLS.MountedTLSDir.TruststorePasswordFile != "" {
			expectedTruststorePasswordFile += solrCloud.Spec.SolrClientTLS.MountedTLSDir.TruststorePasswordFile
		} else {
			expectedTruststorePasswordFile += solrCloud.Spec.SolrClientTLS.MountedTLSDir.KeystorePasswordFile
		}

		tlsJavaToolOpts = "-Djavax.net.ssl.keyStorePassword=$(cat " + expectedKeystorePasswordFile + ") " +
			"-Djavax.net.ssl.trustStorePassword=$(cat " + expectedTruststorePasswordFile + ")"
		tlsJavaSysProps = "-Djavax.net.ssl.trustStore=$SOLR_SSL_CLIENT_TRUST_STORE -Djavax.net.ssl.keyStore=$SOLR_SSL_CLIENT_KEY_STORE"
	} else {
		expectedKeystorePasswordFile := solrCloud.Spec.SolrTLS.MountedTLSDir.Path + "/" + solrCloud.Spec.SolrTLS.MountedTLSDir.KeystorePasswordFile
		expectedTruststorePasswordFile := solrCloud.Spec.SolrTLS.MountedTLSDir.Path + "/"
		if solrCloud.Spec.SolrTLS.MountedTLSDir.TruststorePasswordFile != "" {
			expectedTruststorePasswordFile += solrCloud.Spec.SolrTLS.MountedTLSDir.TruststorePasswordFile
		} else {
			expectedTruststorePasswordFile += solrCloud.Spec.SolrTLS.MountedTLSDir.KeystorePasswordFile
		}

		tlsJavaToolOpts = "-Djavax.net.ssl.keyStorePassword=$(cat " + expectedKeystorePasswordFile + ") " +
			"-Djavax.net.ssl.trustStorePassword=$(cat " + expectedTruststorePasswordFile + ")"
		tlsJavaSysProps = "-Djavax.net.ssl.trustStore=$SOLR_SSL_TRUST_STORE -Djavax.net.ssl.keyStore=$SOLR_SSL_KEY_STORE"
	}

	Expect(mainContainer.LivenessProbe).To(Not(BeNil()), "main container should have a liveness probe defined")
	Expect(mainContainer.LivenessProbe.Exec).To(Not(BeNil()), "liveness probe should have an exec when mTLS is enabled")
	Expect(mainContainer.LivenessProbe.Exec.Command).To(Not(BeNil()), "liveness probe should have an exec when mTLS is enabled")
	Expect(mainContainer.LivenessProbe.Exec.Command).To(Not(BeEmpty()), "liveness probe command cannot be empty")
	Expect(mainContainer.LivenessProbe.Exec.Command[2]).To(ContainSubstring(tlsJavaToolOpts), "liveness probe should invoke java with SSL opts")
	Expect(mainContainer.LivenessProbe.Exec.Command[2]).To(ContainSubstring(tlsJavaSysProps), "liveness probe should invoke java with SSL opts")
	Expect(mainContainer.ReadinessProbe).To(Not(BeNil()), "main container should have a readiness probe defined")
	Expect(mainContainer.ReadinessProbe.Exec).To(Not(BeNil()), "readiness probe should have an exec when mTLS is enabled")
	Expect(mainContainer.ReadinessProbe.Exec.Command).To(Not(BeEmpty()), "readiness probe command cannot be empty")
	Expect(mainContainer.ReadinessProbe.Exec.Command[2]).To(ContainSubstring(tlsJavaToolOpts), "readiness probe should invoke java with SSL opts")
	Expect(mainContainer.ReadinessProbe.Exec.Command[2]).To(ContainSubstring(tlsJavaSysProps), "readiness probe should invoke java with SSL opts")
}

// ensures the TLS settings are applied correctly to the STS
func expectStatefulSetTLSConfig(solrCloud *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, needsPkcs12InitContainer bool) {
	expectStatefulSetTLSConfigWithGomega(Default, solrCloud, statefulSet, needsPkcs12InitContainer)
}

func expectStatefulSetTLSConfigWithGomega(g Gomega, solrCloud *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, needsPkcs12InitContainer bool) {
	podTemplate := &statefulSet.Spec.Template
	expectTLSConfigOnPodTemplateWithGomega(g, solrCloud.Spec.SolrTLS, podTemplate, needsPkcs12InitContainer, false, solrCloud.Spec.SolrClientTLS)

	// Check HTTPS cluster prop setup container
	g.Expect(podTemplate.Spec.InitContainers).To(Not(BeEmpty()), "Solr Pods with TLS require init containers")
	expectZkSetupInitContainerForTLSWithGomega(g, solrCloud, statefulSet)
}

func expectZkSetupInitContainerForTLS(solrCloud *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet) {
	expectZkSetupInitContainerForTLSWithGomega(Default, solrCloud, statefulSet)
}

func expectZkSetupInitContainerForTLSWithGomega(g Gomega, solrCloud *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet) {
	var zkSetupInitContainer *corev1.Container = nil
	podTemplate := &statefulSet.Spec.Template
	for _, cnt := range podTemplate.Spec.InitContainers {
		if cnt.Name == "setup-zk" {
			zkSetupInitContainer = &cnt
			break
		}
	}
	expChrootCmd := "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER};"
	expCmd := "/opt/solr/server/scripts/cloud-scripts/zkcli.sh -zkhost ${ZK_HOST} -cmd clusterprop -name urlScheme -val https"
	if solrCloud.Spec.SolrTLS != nil {
		g.Expect(zkSetupInitContainer).To(Not(BeNil()), "Didn't find the zk-setup InitContainer in the sts!")
		if zkSetupInitContainer != nil {
			g.Expect(zkSetupInitContainer.Image).To(Equal(statefulSet.Spec.Template.Spec.Containers[0].Image), "The zk-setup init container should use the same image as the Solr container")
			g.Expect(zkSetupInitContainer.Command).To(HaveLen(3), "Wrong command length for zk-setup init container")
			g.Expect(zkSetupInitContainer.Command[2]).To(ContainSubstring(expCmd), "ZK Setup command does not set urlScheme")
			g.Expect(zkSetupInitContainer.Command[2]).To(ContainSubstring(expChrootCmd), "ZK Setup command does init the chroot")
			expNumVars := 3
			if solrCloud.Spec.SolrSecurity != nil && solrCloud.Spec.SolrSecurity.BasicAuthSecret == "" {
				expNumVars = 4 // one more for SECURITY_JSON
			}
			g.Expect(zkSetupInitContainer.Env).To(HaveLen(expNumVars), "Wrong number of envVars for zk-setup init container")
		}
	} else {
		g.Expect(zkSetupInitContainer).To(BeNil(), "Shouldn't find the zk-setup InitContainer in the sts, when not using https!")
	}
}

func expectTLSService(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, selectorLables map[string]string, podsHaveTLSEnabled bool) {
	appProtocol := "http"
	podPort := 8983
	servicePort := 80
	if podsHaveTLSEnabled {
		appProtocol = "https"
		servicePort = 443
	}
	By("testing the Solr Common Service")
	commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), selectorLables, false)
	Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
	Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(servicePort)), "Wrong port on common Service")
	Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")
	Expect(commonService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on common Service")
	Expect(commonService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on common Service should not be nil")
	Expect(*commonService.Spec.Ports[0].AppProtocol).To(Equal(appProtocol), "Wrong appProtocol on common Service")

	if solrCloud.Spec.SolrAddressability.External.UsesIndividualNodeServices() {
		nodeNames := solrCloud.GetAllSolrPodNames()
		By("testing the Solr Node Service(s)")
		for _, nodeName := range nodeNames {
			service := expectService(ctx, solrCloud, nodeName, util.MergeLabelsOrAnnotations(selectorLables, map[string]string{"statefulset.kubernetes.io/pod-name": nodeName}), false)
			Expect(service.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(servicePort)), "Wrong port on node Service")
			Expect(service.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on node Service")
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on node Service")
			Expect(service.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on node Service should not be nil")
			Expect(*service.Spec.Ports[0].AppProtocol).To(Equal(appProtocol), "Wrong appProtocol on node Service")
		}
	} else {
		By("testing the Solr Headless Service")
		headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), selectorLables, true)
		Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
		Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(podPort)), "Wrong port on headless Service")
		Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")
		Expect(headlessService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on headless Service")
		Expect(headlessService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on headless Service should not be nil")
		Expect(*headlessService.Spec.Ports[0].AppProtocol).To(Equal(appProtocol), "Wrong appProtocol on headless Service")
	}
}

func expectPassthroughIngressTLSConfig(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, expectedTLSSecretName string) {
	ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
	Expect(ingress.Spec.TLS).To(Not(BeEmpty()), "Ingress does not have TLS secrets provided")
	Expect(ingress.Spec.TLS[0].SecretName).To(Equal(expectedTLSSecretName), "Wrong secretName for ingress TLS")
	if solrCloud.Spec.SolrTLS != nil {
		Expect(ingress.ObjectMeta.Annotations).To(HaveKeyWithValue("nginx.ingress.kubernetes.io/backend-protocol", "HTTPS"), "Ingress Backend Protocol annotation incorrect")
	} else {
		Expect(ingress.ObjectMeta.Annotations).To(HaveKeyWithValue("nginx.ingress.kubernetes.io/backend-protocol", "HTTP"), "Ingress Backend Protocol annotation incorrect")
	}
	if len(ingress.Spec.TLS) > 0 {
		Expect(ingress.ObjectMeta.Annotations).To(HaveKeyWithValue("nginx.ingress.kubernetes.io/ssl-redirect", "true"), "Ingress SSL Redirect annotation incorrect")
	} else {
		Expect(ingress.ObjectMeta.Annotations).To(Not(HaveKey("nginx.ingress.kubernetes.io/ssl-redirect")), "Ingress should not have SSL Redirect annotation")
	}
}

func expectTerminateIngressTLSConfig(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, expectedTLSSecretName string, isBackendTls bool) {
	ingress := expectIngress(ctx, solrCloud, solrCloud.CommonIngressName())
	Expect(ingress.Spec.TLS).To(HaveLen(1), "Wrong number of TLS Secrets for ingress")
	Expect(ingress.Spec.TLS[0].SecretName).To(Equal(expectedTLSSecretName), "Wrong secretName for ingress TLS")
	Expect(ingress.Spec.TLS[0].Hosts).To(ConsistOf("default-foo-solrcloud."+testDomain, "default-foo-solrcloud-0."+testDomain), "Wrong hosts for Ingress TLS termination")

	if isBackendTls {
		Expect(ingress.ObjectMeta.Annotations).To(HaveKeyWithValue("nginx.ingress.kubernetes.io/backend-protocol", "HTTPS"), "Ingress Backend Protocol annotation incorrect")
	} else {
		Expect(ingress.ObjectMeta.Annotations).To(HaveKeyWithValue("nginx.ingress.kubernetes.io/backend-protocol", "HTTP"), "Ingress Backend Protocol annotation incorrect")
	}
	Expect(ingress.ObjectMeta.Annotations).To(HaveKeyWithValue("nginx.ingress.kubernetes.io/ssl-redirect", "true"), "Ingress SSL Redirect annotation incorrect")
}

func expectRestartOnTLSSecretUpdate(ctx context.Context, secretName string, solrCloud *solrv1beta1.SolrCloud, needsPkcs12InitContainer bool, certAnnotation string) {
	foundTLSSecret := expectSecret(ctx, solrCloud, secretName)
	// let's trigger an update to the TLS secret to simulate the cert getting renewed and the pods getting restarted
	expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
		expectedCertMd5 := fmt.Sprintf("%x", md5.Sum(foundTLSSecret.Data[util.TLSCertKey]))
		g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(HaveKeyWithValue(certAnnotation, expectedCertMd5), "TLS cert MD5 annotation on STS does not match the secret")
	})

	// change the tls.crt which should trigger a rolling restart
	updatedTlsCertData := "certificate renewed"
	foundTLSSecret.Data[util.TLSCertKey] = []byte(updatedTlsCertData)
	Expect(k8sClient.Update(ctx, foundTLSSecret)).To(Succeed(), "Cannot update the TLS Secret")

	// Check the annotation on the pod template to make sure a rolling restart will take place
	By("Checking that the cert MD5 has changed on the pod template annotations")
	expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
		expectStatefulSetTLSConfigWithGomega(g, solrCloud, found, needsPkcs12InitContainer)

		expectedCertMd5 := fmt.Sprintf("%x", md5.Sum(foundTLSSecret.Data[util.TLSCertKey]))
		g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(HaveKeyWithValue(certAnnotation, expectedCertMd5), "TLS cert MD5 annotation on STS does not match the UPDATED secret")
	})
}

func verifyReconcileUserSuppliedTLS(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, needsPkcs12InitContainer bool, restartOnTLSSecretUpdate bool) {
	// Custom truststore?
	if solrCloud.Spec.SolrTLS.TrustStoreSecret != nil {
		// create the mock truststore secret
		secretData := map[string][]byte{}
		secretData[solrCloud.Spec.SolrTLS.TrustStoreSecret.Key] = []byte(b64.StdEncoding.EncodeToString([]byte("mock truststore")))
		secretData[solrCloud.Spec.SolrTLS.TrustStorePasswordSecret.Key] = []byte(b64.StdEncoding.EncodeToString([]byte("mock truststore password")))
		trustStoreSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: solrCloud.Spec.SolrTLS.TrustStoreSecret.Name, Namespace: solrCloud.Namespace},
			Data:       secretData,
			Type:       corev1.SecretTypeOpaque,
		}
		Expect(k8sClient.Create(ctx, &trustStoreSecret)).To(Succeed(), "Cannot create mock truststore secret")
	}

	// create the secret required for reconcile, it has both keys ...
	tlsKey := "keystore.p12"
	if needsPkcs12InitContainer {
		tlsKey = "tls.key" // to trigger the initContainer creation, don't want keystore.p12 in the secret
	}

	// Create a mock secret in the background so the isCert ready function returns
	if solrCloud.Spec.SolrTLS != nil && solrCloud.Spec.SolrTLS.PKCS12Secret != nil {
		createMockTLSSecret(ctx, solrCloud, solrCloud.Spec.SolrTLS.PKCS12Secret.Name, tlsKey,
			solrCloud.Spec.SolrTLS.KeyStorePasswordSecret.Key, "")
	}

	// need a secret for the client cert too?
	if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.PKCS12Secret != nil {
		truststoreKey := ""
		if solrCloud.Spec.SolrClientTLS.TrustStoreSecret == solrCloud.Spec.SolrClientTLS.PKCS12Secret {
			truststoreKey = solrCloud.Spec.SolrClientTLS.TrustStoreSecret.Key
		}
		createMockTLSSecret(ctx, solrCloud, solrCloud.Spec.SolrClientTLS.PKCS12Secret.Name, util.DefaultPkcs12KeystoreFile,
			solrCloud.Spec.SolrClientTLS.KeyStorePasswordSecret.Key, truststoreKey)
	}

	statefulSet := expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
		expectStatefulSetTLSConfigWithGomega(g, solrCloud, found, needsPkcs12InitContainer)
	})

	if !restartOnTLSSecretUpdate {
		Expect(statefulSet.Spec.Template.ObjectMeta.Annotations).To(Not(HaveKey(util.SolrTlsCertMd5Annotation)), "Shouldn't have a cert MD5 as we're not tracking updates to the secret")
		return
	}

	// let's trigger an update to the TLS secret to simulate the cert getting renewed and the pods getting restarted
	expectRestartOnTLSSecretUpdate(ctx, solrCloud.Spec.SolrTLS.PKCS12Secret.Name, solrCloud, needsPkcs12InitContainer, util.SolrTlsCertMd5Annotation)

	// Update the client cert and see the change get picked up
	if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.PKCS12Secret != nil && solrCloud.Spec.SolrClientTLS.RestartOnTLSSecretUpdate {
		expectRestartOnTLSSecretUpdate(ctx, solrCloud.Spec.SolrClientTLS.PKCS12Secret.Name, solrCloud, false, util.SolrClientTlsCertMd5Annotation)
	}
}

func mountedTLSDir(path string) *solrv1beta1.MountedTLSDirectory {
	return &solrv1beta1.MountedTLSDirectory{
		Path:                 path,
		KeystoreFile:         util.DefaultPkcs12KeystoreFile,
		TruststoreFile:       util.DefaultPkcs12TruststoreFile,
		KeystorePasswordFile: util.DefaultKeystorePasswordFile,
	}
}
