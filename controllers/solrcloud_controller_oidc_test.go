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
	b64 "encoding/base64"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("SolrCloud controller - OIDC", func() {
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
						InternalConnectionString: "host:7271",
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
			g.Expect(found.WithDefaults()).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Boostrap Security JSON for OIDC", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Oidc,
				Oidc: &solrv1beta1.OidcOptions{
					ClientCredentialsSecret: "test-oauth2-secret",
					WellKnownUrl:            "http://example.com",
				},
				BootstrapSecurityJson: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "user-provided-security-json"},
					Key:                  "security.json",
				},
				ProbesRequireAuth: false,
			}
		})
		FIt("has the correct resources", func() {
			expectStatefulSetOidcConfig(ctx, solrCloud)
		})
	})
})

func expectStatefulSetOidcConfig(ctx context.Context, sc *solrv1beta1.SolrCloud) *appsv1.StatefulSet {
	Expect(sc.Spec.SolrSecurity).To(Not(BeNil()), "solrSecurity is not configured for this SolrCloud instance!")

	createMockSecurityJsonConfigMap(ctx, sc.Spec.SolrSecurity.BootstrapSecurityJson.Name, sc.Namespace)
	createMockOidcSecret(ctx, sc.Spec.SolrSecurity.Oidc.ClientCredentialsSecret, sc.Namespace)

	stateful := expectStatefulSetWithChecks(ctx, sc, sc.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
		expectOidcConfigOnPodTemplateWithGomega(g, sc, &found.Spec.Template)
	})

	return stateful
}

func createMockOidcSecret(ctx context.Context, secretName string, ns string) corev1.Secret {
	secretData := map[string][]byte{}
	secretData[util.ClientIdKey] = []byte(b64.StdEncoding.EncodeToString([]byte("TEST_CLIENT")))
	secretData[util.ClientSecretKey] = []byte(b64.StdEncoding.EncodeToString([]byte("TEST_CLIENT_SECRET")))
	mockOidcSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Data:       secretData,
		Type:       corev1.SecretTypeOpaque,
	}
	Expect(k8sClient.Create(ctx, &mockOidcSecret)).To(Succeed(), "Could not create mock Oidc Secret")
	return mockOidcSecret
}

func expectOidcConfigOnPodTemplateWithGomega(g Gomega, solrCloud *solrv1beta1.SolrCloud, podTemplate *corev1.PodTemplateSpec) *corev1.Container {
	// check the env vars needed for the probes to work with auth
	g.Expect(podTemplate.Spec.Containers).To(Not(BeEmpty()), "Solr Pod requires containers")
	mainContainer := podTemplate.Spec.Containers[0]
	g.Expect(mainContainer).To(Not(BeNil()), "Didn't find the main solrcloud-node container in the sts!")
	g.Expect(mainContainer.Env).To(Not(BeEmpty()), "Didn't find the main solrcloud-node container in the sts!")

	// find the setup-zk initContainer
	g.Expect(podTemplate.Spec.InitContainers).To(Not(BeEmpty()), "The Solr Pod template requires an init container to bootstrap the security.json")
	var expInitContainer *corev1.Container = nil
	for _, cnt := range podTemplate.Spec.InitContainers {
		if cnt.Name == "setup-zk" {
			expInitContainer = &cnt
			break
		}
	}
	expectPutSecurityJsonInZkCmd(g, expInitContainer)

	return &mainContainer // return as a convenience in case tests want to do more checking on the main container
}
