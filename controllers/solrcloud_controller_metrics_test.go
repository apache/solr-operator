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

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("SolrCloud controller - Metrics", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
			},
		}
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("defaulting the missing SolrCloud values")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FContext("No metrics options given", func() {
		FIt("does not annotate the Solr pods", func(ctx context.Context) {
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			Expect(statefulSet.Spec.Template.Annotations).To(Not(HaveKey("prometheus.io/scrape")), "The Solr pods should not be annotated for scraping unless asked for")
		})
	})

	FContext("Prometheus scrape annotations", func() {
		BeforeEach(func() {
			solrCloud.Spec.Prometheus = solrv1beta1.SolrPrometheusOptions{
				ScrapeAnnotations: &solrv1beta1.SolrPrometheusScrapeAnnotationsOptions{Enable: ptr.To(true)},
			}
		})
		FIt("annotates the Solr pods with the metrics endpoint", func(ctx context.Context) {
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			podAnnotations := statefulSet.Spec.Template.Annotations
			Expect(podAnnotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"), "Scraping of the Solr pods is not enabled")
			Expect(podAnnotations).To(HaveKeyWithValue("prometheus.io/scheme", "http"), "Wrong scrape scheme on the Solr pods")
			Expect(podAnnotations).To(HaveKeyWithValue("prometheus.io/path", "/solr/admin/metrics"), "Wrong scrape path on the Solr pods")
			Expect(podAnnotations).To(HaveKeyWithValue("prometheus.io/port", "8983"), "Wrong scrape port on the Solr pods")
			Expect(podAnnotations).To(HaveKeyWithValue("prometheus.io/param-wt", "prometheus"), "The metrics API must be asked for the Prometheus format")
		})
	})

	FContext("Prometheus scrape annotations - disabled", func() {
		BeforeEach(func() {
			solrCloud.Spec.Prometheus = solrv1beta1.SolrPrometheusOptions{
				ScrapeAnnotations: &solrv1beta1.SolrPrometheusScrapeAnnotationsOptions{Enable: ptr.To(false)},
			}
		})
		FIt("does not annotate the Solr pods", func(ctx context.Context) {
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			Expect(statefulSet.Spec.Template.Annotations).To(Not(HaveKey("prometheus.io/scrape")), "The Solr pods should not be annotated for scraping when it is disabled")
		})
	})

	FContext("Prometheus scrape annotations - overridden by the user", func() {
		BeforeEach(func() {
			solrCloud.Spec.Prometheus = solrv1beta1.SolrPrometheusOptions{
				ScrapeAnnotations: &solrv1beta1.SolrPrometheusScrapeAnnotationsOptions{Enable: ptr.To(true)},
			}
			solrCloud.Spec.CustomSolrKubeOptions = solrv1beta1.CustomSolrKubeOptions{
				PodOptions: &solrv1beta1.PodOptions{
					// Only a subset of the metrics, through the group parameter of Solr's metrics API
					Annotations: map[string]string{"prometheus.io/param-group": "core"},
				},
			}
		})
		FIt("keeps the annotations given in podOptions", func(ctx context.Context) {
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/param-group", "core"), "A user-provided annotation must be kept")
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/param-wt", "prometheus"), "The generated annotations must still be set")
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"), "The other generated annotations must still be set")
			})
		})
	})
})
