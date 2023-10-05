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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/*
There is no need to spin up a lot of SolrClouds for a Prometheus Exporter test,
so all tests will run in the same parallel process and execute serially.
*/

var _ = FDescribe("E2E - Prometheus Exporter - TLS ", Ordered, func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter

		solrCollection = "e2e"
	)

	/*
		Create a single SolrCloud that all PrometheusExporter tests in this "Describe" will use.
	*/
	BeforeAll(func(ctx context.Context) {
		installSolrIssuer(ctx, testNamespace())
		solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, true)

		solrCloud.Spec.SolrTLS.CheckPeerName = true

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

	BeforeEach(func() {
		solrPrometheusExporter = &solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: testNamespace(),
			},
			Spec: solrv1beta1.SolrPrometheusExporterSpec{
				ScrapeInterval: five,
			},
		}
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating a SolrPrometheusExporter")
		Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			deleteAndWait(ctx, solrPrometheusExporter)
		})

		By("waiting for the SolrPrometheusExporter to come up healthy")
		solrPrometheusExporter = expectSolrPrometheusExporterWithChecks(ctx, solrPrometheusExporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.Status.Ready).To(BeTrue(), "The SolrPrometheusExporter should come up healthy")
		})

		By("checking that some base metrics are correct")
		checkMetrics(ctx, solrPrometheusExporter, solrCloud, solrCollection)
	})

	FContext("Secret - Solr Reference", func() {
		BeforeEach(func() {
			solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
				Cloud: &solrv1beta1.SolrCloudReference{
					Name: solrCloud.Name,
				},
				SolrTLS: solrCloud.Spec.SolrClientTLS,
			}
		})

		// The base metrics tests are run in the "JustBeforeEach" - no additional tests necessary
		FIt("Has the correct metrics", func() {})
	})

	FContext("Secret - Solr ZK Connection String", func() {
		BeforeEach(func() {
			solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
				Cloud: &solrv1beta1.SolrCloudReference{
					ZookeeperConnectionInfo: &solrCloud.Status.ZookeeperConnectionInfo,
				},
				SolrTLS: solrCloud.Spec.SolrClientTLS,
			}
		})

		FIt("Has the correct metrics", func() {})
	})

	FContext("Secret - Solr Host", func() {
		BeforeEach(func() {
			solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
				Standalone: &solrv1beta1.StandaloneSolrReference{
					Address: solrCloud.Status.InternalCommonAddress + "/solr",
				},
				SolrTLS: solrCloud.Spec.SolrClientTLS,
			}
		})

		FIt("Has the correct metrics", func() {})
	})

	/*
		These can be uncommented when Cert Manager CSI Driver supports truststore generation

		FContext("MountedDir - Solr Reference", func() {
			BeforeEach(func() {
				solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						Name: solrCloud.Name,
					},
				}
				addCSITLSToPrometheusExporter(solrPrometheusExporter)
			})

			// The base metrics tests are run in the "JustBeforeEach" - no additional tests necessary
			FIt("Has the correct metrics", func() {})
		})

		FContext("MountedDir - Solr ZK Connection String", func() {
			BeforeEach(func() {
				solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrCloud.Status.ZookeeperConnectionInfo,
					},
				}
				addCSITLSToPrometheusExporter(solrPrometheusExporter)
			})

			FIt("Has the correct metrics", func() {})
		})

		FContext("MountedDir - Solr Host", func() {
			BeforeEach(func() {
				solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
					Standalone: &solrv1beta1.StandaloneSolrReference{
						Address: solrCloud.Status.InternalCommonAddress + "/solr",
					},
				}
				addCSITLSToPrometheusExporter(solrPrometheusExporter)
			})

			FIt("Has the correct metrics", func() {})
		})
	*/
})
