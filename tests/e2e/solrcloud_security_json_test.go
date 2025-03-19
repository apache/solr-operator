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
	"github.com/apache/solr-operator/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = FDescribe("E2E - SolrCloud - Security JSON", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		solrCloud = generateBaseSolrCloudWithSecurityJSON(1)
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a first Solr Collection")
		createAndQueryCollection(ctx, solrCloud, "basic", 2, 1)
	})

	FContext("Provided Security JSON", func() {
		BeforeEach(func(ctx context.Context) {
			solrCloud.Spec.ZookeeperRef = &solrv1beta1.ZookeeperRef{
				ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
					Replicas:  pointer.Int32(1),
					Ephemeral: &solrv1beta1.ZKEphemeral{},
				},
			}

			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: "Basic",
				BasicAuthSecret:    solrCloud.Name + "-basic-auth-secret",
				BootstrapSecurityJson: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: solrCloud.Name + "-security-secret",
					},
					Key: "security.json",
				},
			}

			By("generating the security.json secret and basic auth secret")
			generateSolrSecuritySecret(ctx, solrCloud)
			generateSolrBasicAuthSecret(ctx, solrCloud)
		})

		// All testing will be done in the "JustBeforeEach" logic, no additional tests required here
		FIt("Starts correctly", func(ctx context.Context) {})
	})

	FContext("Bootstrapped Security", func() {

		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: "Basic",
			}
		})

		FIt("Scales up with replica migration", func(ctx context.Context) {
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.Replicas = pointer.Int32(int32(2))
			By("triggering a scale up via solrCloud replicas")
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not patch SolrCloud replicas to initiate scale down")

			By("make sure scaleDown happens without a clusterLock and eventually the replicas are removed")
			// Once the scale down actually occurs, the statefulSet annotations should be removed very soon
			expectStatefulSetWithChecksAndTimeout(ctx, solrCloud, solrCloud.StatefulSetName(), time.Minute*2, time.Millisecond*500, func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)), "StatefulSet should eventually have 2 pods.")
				clusterOp, err := controllers.GetCurrentClusterOp(found)
				g.Expect(err).ToNot(HaveOccurred(), "Error occurred while finding clusterLock for SolrCloud")
				g.Expect(clusterOp).To(BeNil(), "StatefulSet should have a ScaleDown lock after scaling is complete.")
			})

			queryCollection(ctx, solrCloud, "basic", 0)

			// TODO: When balancing is in all Operator supported Solr versions, add a test to make sure balancing occurred
		})
	})
})
