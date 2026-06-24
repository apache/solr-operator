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
	"fmt"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// expectEvent waits until at least one Event of the given type and reason has been recorded against
// the given object. Events are matched on the involved object's UID so that events left over from
// other specs (cleanupTest does not delete Events) cannot cause a false positive.
func expectEvent(ctx context.Context, involvedObject client.Object, eventType, reason string, additionalOffset ...int) {
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		eventList := &corev1.EventList{}
		g.Expect(k8sClient.List(ctx, eventList, client.InNamespace(involvedObject.GetNamespace()))).To(Succeed())
		found := false
		for i := range eventList.Items {
			e := &eventList.Items[i]
			if e.InvolvedObject.UID == involvedObject.GetUID() && e.Type == eventType && e.Reason == reason {
				found = true
				break
			}
		}
		g.Expect(found).To(BeTrue(), fmt.Sprintf("expected a %q event with reason %q on %s/%s", eventType, reason, involvedObject.GetNamespace(), involvedObject.GetName()))
	}).Should(Succeed())
}

var _ = FDescribe("SolrCloud controller - Events", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "events",
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

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FContext("with a scheduled restart configured", func() {
		BeforeEach(func() {
			solrCloud.Spec.UpdateStrategy = solrv1beta1.SolrUpdateStrategy{
				RestartSchedule: "@every 10h",
			}
		})

		FIt("records a RestartScheduled event on the SolrCloud", func(ctx context.Context) {
			By("creating the SolrCloud")
			Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

			By("waiting for the SolrCloud to be fully defaulted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
			})

			By("checking that a RestartScheduled event was recorded")
			expectEvent(ctx, solrCloud, corev1.EventTypeNormal, "RestartScheduled")
		})
	})

	FContext("with a provided ConfigMap that is missing the required keys", func() {
		var providedConfigMap *corev1.ConfigMap

		BeforeEach(func() {
			providedConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-provided-config",
					Namespace: solrCloud.Namespace,
				},
				// Neither "solr.xml" nor "log4j2.xml" is present, which is invalid.
				Data: map[string]string{"unrelated-key": "unrelated-value"},
			}
			solrCloud.Spec.CustomSolrKubeOptions = solrv1beta1.CustomSolrKubeOptions{
				ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
					ProvidedConfigMap: providedConfigMap.Name,
				},
			}
		})

		FIt("records an InvalidConfigMap event on the SolrCloud", func(ctx context.Context) {
			By("creating the invalid ConfigMap")
			Expect(k8sClient.Create(ctx, providedConfigMap)).To(Succeed())

			By("creating the SolrCloud")
			Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

			By("waiting for the SolrCloud to be fully defaulted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
			})

			By("checking that an InvalidConfigMap event was recorded")
			expectEvent(ctx, solrCloud, corev1.EventTypeWarning, "InvalidConfigMap")
		})
	})

	FContext("with a provided ConfigMap whose solr.xml is missing the port placeholder", func() {
		var providedConfigMap *corev1.ConfigMap

		BeforeEach(func() {
			providedConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-solrxml-config",
					Namespace: solrCloud.Namespace,
				},
				// solr.xml is present but does not contain the required port placeholder.
				Data: map[string]string{"solr.xml": "<solr></solr>"},
			}
			solrCloud.Spec.CustomSolrKubeOptions = solrv1beta1.CustomSolrKubeOptions{
				ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
					ProvidedConfigMap: providedConfigMap.Name,
				},
			}
		})

		FIt("records an InvalidConfigMap event on the SolrCloud", func(ctx context.Context) {
			By("creating the invalid ConfigMap")
			Expect(k8sClient.Create(ctx, providedConfigMap)).To(Succeed())

			By("creating the SolrCloud")
			Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

			By("waiting for the SolrCloud to be fully defaulted")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
			})

			By("checking that an InvalidConfigMap event was recorded")
			expectEvent(ctx, solrCloud, corev1.EventTypeWarning, "InvalidConfigMap")
		})
	})
})

var _ = FDescribe("SolrPrometheusExporter controller - Events", func() {
	var (
		solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter
	)

	BeforeEach(func() {
		solrPrometheusExporter = &solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "events",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: "host:2181",
						},
					},
				},
				RestartSchedule: "@every 10h",
			},
		}
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrPrometheusExporter)
	})

	FIt("records a RestartScheduled event on the SolrPrometheusExporter", func(ctx context.Context) {
		By("creating the SolrPrometheusExporter")
		Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())

		By("waiting for the SolrPrometheusExporter to be fully defaulted")
		expectSolrPrometheusExporterWithChecks(ctx, solrPrometheusExporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.WithDefaults()).To(BeFalse(), "The SolrPrometheusExporter spec should not need to be defaulted eventually")
		})

		By("checking that a RestartScheduled event was recorded")
		expectEvent(ctx, solrPrometheusExporter, corev1.EventTypeNormal, "RestartScheduled")
	})

	FContext("with a provided ConfigMap missing the required key", func() {
		var providedConfigMap *corev1.ConfigMap

		BeforeEach(func() {
			providedConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-exporter-config",
					Namespace: solrPrometheusExporter.Namespace,
				},
				// The required exporter config key is absent, which is invalid.
				Data: map[string]string{"unrelated-key": "unrelated-value"},
			}
			solrPrometheusExporter.Spec.CustomKubeOptions = solrv1beta1.CustomExporterKubeOptions{
				ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
					ProvidedConfigMap: providedConfigMap.Name,
				},
			}
		})

		FIt("records an InvalidConfigMap event on the SolrPrometheusExporter", func(ctx context.Context) {
			By("creating the invalid ConfigMap")
			Expect(k8sClient.Create(ctx, providedConfigMap)).To(Succeed())

			By("creating the SolrPrometheusExporter")
			Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())

			By("checking that an InvalidConfigMap event was recorded")
			expectEvent(ctx, solrPrometheusExporter, corev1.EventTypeWarning, "InvalidConfigMap")
		})
	})
})
