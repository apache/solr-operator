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

package e2e

import (
	"context"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/pointer"
)

const (
	solrIssuerName = "solr-issuer"

	clientAuthPasswordSecret = "client-auth-password"
	clientAuthSecret         = "client-auth"
)

func generateBaseSolrCloudWithCSI(replicas int, csiClientTLS bool, secretClientTLS bool) (solrCloud *solrv1beta1.SolrCloud) {
	solrCloud = generateBaseSolrCloud(1)
	solrCloud.Spec.CustomSolrKubeOptions.PodOptions.Volumes = []solrv1beta1.AdditionalVolume{
		{
			Name: "server-tls",
			Source: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:   "csi.cert-manager.io",
					ReadOnly: pointer.Bool(true),
					VolumeAttributes: map[string]string{
						"csi.cert-manager.io/issuer-name": solrIssuerName,
						"csi.cert-manager.io/common-name": "${POD_NAME}." + solrCloud.Name + "-solrcloud-headless.${POD_NAMESPACE}",
						"csi.cert-manager.io/dns-names": "${POD_NAME}." + solrCloud.Name + "-solrcloud-headless.${POD_NAMESPACE}.svc.cluster.local," +
							solrCloud.Name + "-solrcloud-common.${POD_NAMESPACE}," +
							solrCloud.Name + "-solrcloud-common.${POD_NAMESPACE}.svc.cluster.local," +
							"${POD_NAME}," +
							"${POD_NAME}.${POD_NAMESPACE}," +
							"${POD_NAME}.${POD_NAMESPACE}.svc.cluster.local",
						"csi.cert-manager.io/key-usages":      "server auth,digital signature",
						"csi.cert-manager.io/pkcs12-enable":   "true",
						"csi.cert-manager.io/pkcs12-password": "pass",
						"csi.cert-manager.io/fs-group":        "8983",
					},
				},
			},
			DefaultContainerMount: &corev1.VolumeMount{
				ReadOnly:  true,
				MountPath: "/opt/server-tls",
			},
		},
	}

	solrCloud.Spec.SolrTLS = &solrv1beta1.SolrTLSOptions{
		MountedTLSDir: &solrv1beta1.MountedTLSDirectory{
			Path:             "/opt/server-tls",
			KeystoreFile:     "keystore.p12",
			KeystorePassword: "pass",
		},
	}

	if csiClientTLS {
		solrCloud.Spec.CustomSolrKubeOptions.PodOptions.Volumes = append(
			solrCloud.Spec.CustomSolrKubeOptions.PodOptions.Volumes,
			solrv1beta1.AdditionalVolume{
				Name: "client-tls",
				Source: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "csi.cert-manager.io",
						ReadOnly: pointer.Bool(true),
						VolumeAttributes: map[string]string{
							"csi.cert-manager.io/issuer-name": solrIssuerName,
							"csi.cert-manager.io/common-name": "${POD_NAME}." + solrCloud.Name + "-solrcloud-headless.${POD_NAMESPACE}",
							"csi.cert-manager.io/dns-names": "${POD_NAME}." + solrCloud.Name + "-solrcloud-headless.${POD_NAMESPACE}.svc.cluster.local," +
								"${POD_NAME}," +
								"${POD_NAME}.${POD_NAMESPACE}.svc.cluster.local",
							"csi.cert-manager.io/key-usages":      "client auth,digital signature",
							"csi.cert-manager.io/pkcs12-enable":   "true",
							"csi.cert-manager.io/pkcs12-password": "pass",
							"csi.cert-manager.io/fs-group":        "8983",
						},
					},
				},
				DefaultContainerMount: &corev1.VolumeMount{
					ReadOnly:  true,
					MountPath: "/opt/client-tls",
				},
			})

		solrCloud.Spec.SolrClientTLS = &solrv1beta1.SolrTLSOptions{
			MountedTLSDir: &solrv1beta1.MountedTLSDirectory{
				Path:             "/opt/client-tls",
				KeystoreFile:     "keystore.p12",
				KeystorePassword: "pass",
			},
		}
	} else if secretClientTLS {
		solrCloud.Spec.SolrClientTLS = &solrv1beta1.SolrTLSOptions{
			PKCS12Secret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: clientAuthSecret,
				},
				Key: "keystore.p12",
			},
			TrustStoreSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: clientAuthSecret,
				},
				Key: "truststore.p12",
			},
			TrustStorePasswordSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: clientAuthPasswordSecret,
				},
				Key: "password",
			},
		}
	}
	return
}

var _ = FDescribe("E2E - SolrCloud - TLS", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection = "e2e"
	)

	/*
		Create a single SolrCloud that has TLS Enabled
	*/
	BeforeEach(func(ctx context.Context) {
		installSolrIssuer(ctx, testNamespace())
	})

	/*
		Start the SolrCloud and ensure that it is running
	*/
	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a Solr Collection to query metrics for")
		createAndQueryCollection(ctx, solrCloud, solrCollection, 1, 2)
	})

	FContext("ClientAuth - Want", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithCSI(1, false, false)

			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Want

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func() {})
	})

	//FContext("ClientAuth - Need", func() {
	//
	//	BeforeEach(func(ctx context.Context) {
	//		solrCloud = generateBaseSolrCloudWithCSI(1, false, true)
	//
	//		solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Need
	//
	//		//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
	//	})
	//
	//	FIt("Can run", func() {})
	//})
})

func installBootstrapIssuer(ctx context.Context) {
	bootstrapIssuer := &certmanagerv1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bootstrap-issuer",
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}
	Expect(k8sClient.Create(ctx, bootstrapIssuer)).To(Succeed(), "Failed to install SelfSigned ClusterIssuer for bootstrapping CA")
	DeferCleanup(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, bootstrapIssuer)).To(Succeed(), "Failed to delete SelfSigned bootstrapping ClusterIssuer")
	})
}

func installSolrIssuer(ctx context.Context, namespace string) {
	secretName := "solr-ca-key-pair"
	clusterCA := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "solr-ca",
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			IsCA:       true,
			CommonName: "solr-ca",
			SecretName: secretName,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				RotationPolicy: certmanagerv1.RotationPolicyNever,
				Algorithm:      "RSA",
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name:  "bootstrap-issuer",
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
		},
	}
	Expect(k8sClient.Create(ctx, clusterCA)).To(Succeed(), "Failed to install Solr CA for tests")
	DeferCleanup(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, clusterCA)).To(Succeed(), "Failed to delete Solr CA in namespace "+namespace)
	})

	namespaceIssuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrIssuerName,
			Namespace: namespace,
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				CA: &certmanagerv1.CAIssuer{
					SecretName: secretName,
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespaceIssuer)).To(Succeed(), "Failed to install CA Issuer for issuing test certs in namespace "+namespace)
	DeferCleanup(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, namespaceIssuer)).To(Succeed(), "Failed to delete CA Issuer in namespace "+namespace)
	})

	expectSecret(ctx, clusterCA, secretName)

	truststorePasswordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientAuthPasswordSecret,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"password": rand.String(10),
		},
	}
	Expect(k8sClient.Create(ctx, truststorePasswordSecret)).To(Succeed(), "Could not create secret with password for clientAuth trustStore namespace "+namespace)
	DeferCleanup(func(ctx context.Context) {
		//Expect(k8sClient.Delete(ctx, truststorePasswordSecret)).To(Succeed(), "Failed to delete secret for clientAuth truststore password in namespace "+namespace)
	})

	clientAuthCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "client-auth",
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: "client-auth",
			SecretName: clientAuthSecret,
			Keystores: &certmanagerv1.CertificateKeystores{
				PKCS12: &certmanagerv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: certmanagermetav1.SecretKeySelector{
						LocalObjectReference: certmanagermetav1.LocalObjectReference{
							Name: clientAuthPasswordSecret,
						},
						Key: "password",
					},
				},
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name:  namespaceIssuer.Name,
				Kind:  "Issuer",
				Group: "cert-manager.io",
			},
			IsCA:   false,
			Usages: []certmanagerv1.KeyUsage{certmanagerv1.UsageClientAuth, certmanagerv1.UsageDigitalSignature},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				RotationPolicy: certmanagerv1.RotationPolicyNever,
				Algorithm:      "RSA",
			},
		},
	}
	Expect(k8sClient.Create(ctx, clientAuthCert)).To(Succeed(), "Failed to install ClientAuth cert for tests")
	DeferCleanup(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, clientAuthCert)).To(Succeed(), "Failed to delete ClientAuth cert in namespace "+namespace)
	})

	expectSecret(ctx, clientAuthCert, clientAuthSecret)
}
