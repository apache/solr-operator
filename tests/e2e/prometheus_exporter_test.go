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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

var _ = FDescribe("E2E - Prometheus Exporter", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud

		solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter

		solrCollection = "e2e"

		solrImage = getEnvWithDefault(solrImageEnv, defaultSolrImage)
	)

	BeforeEach(func() {
		ctx = context.Background()

		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: testNamespace(),
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &three,
				SolrImage: &solrv1beta1.ContainerImage{
					Repository: strings.Split(solrImage, ":")[0],
					Tag:        strings.Split(solrImage+":", ":")[1],
					PullPolicy: corev1.PullIfNotPresent,
				},
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas: &one,
						Persistence: &solrv1beta1.ZKPersistence{
							PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("5Gi"),
									},
								},
							},
						},
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("300m"),
							},
						},
					},
				},
			},
		}

		solrPrometheusExporter = &solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: testNamespace(),
			},
			Spec: solrv1beta1.SolrPrometheusExporterSpec{},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("waiting for the SolrCloud to come up healthy")
		foundSolrCloud := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Status.ReadyReplicas).To(Equal(*found.Spec.Replicas), "The SolrCloud should have all nodes come up healthy")
		})

		By("creating a Solr Collection to backup")
		createAndQueryCollection(foundSolrCloud, solrCollection)

		By("creating a SolrPrometheusExporter")
		Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())

		By("waiting for the SolrPrometheusExporter to come up healthy")
		foundPrometheusExporter := expectSolrPrometheusExporterWithChecks(ctx, solrPrometheusExporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.Status.Ready).To(BeTrue(), "The SolrPrometheusExporter should come up healthy")
		})

		By("checking that some base metrics are correct")
		checkMetrics(ctx, foundPrometheusExporter, foundSolrCloud, solrCollection)
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Default - Solr Reference", func() {
		BeforeEach(func() {
			solrPrometheusExporter.Spec.SolrReference = solrv1beta1.SolrReference{
				Cloud: &solrv1beta1.SolrCloudReference{
					Name: solrCloud.Name,
				},
			}
		})

		FIt("Has the correct metrics", func() {})
	})
})
