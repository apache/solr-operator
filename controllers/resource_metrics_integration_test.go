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
	"strings"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// The collector is registered with the operator's cache in the test suite, like main.go does, so this exercises
// the real manager cache and the counters incremented by the controllers, not a fake client.
var _ = FDescribe("Operator metrics", func() {
	var solrCloud *solrv1beta1.SolrCloud

	BeforeEach(func() {
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics-foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: pointer.Int32(2),
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
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FIt("reports the SolrCloud from the operator's cache, and its reconciles", func(ctx context.Context) {
		expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

		Eventually(func(g Gomega) {
			series := gatherOperatorMetrics(g)
			g.Expect(series).To(HaveKeyWithValue(`solr_operator_managed_resources{kind="SolrCloud",namespace="default"}`, BeNumerically(">=", 1)))
			g.Expect(series).To(HaveKeyWithValue(`solr_operator_solrcloud_desired_replicas{name="metrics-foo",namespace="default"}`, 2.0))
			g.Expect(series).To(HaveKeyWithValue(`solr_operator_solrcloud_ready_replicas{name="metrics-foo",namespace="default"}`, 0.0), "no pods ever run in envtest")
			g.Expect(series).To(HaveKeyWithValue(`solr_operator_solrcloud_ready{name="metrics-foo",namespace="default"}`, 0.0))

			reconciles := 0.0
			for name, value := range series {
				if strings.HasPrefix(name, `solr_operator_reconciles_total{kind="SolrCloud",name="metrics-foo",namespace="default"`) {
					reconciles += value
				}
			}
			g.Expect(reconciles).To(BeNumerically(">=", 1), "the reconciles of the SolrCloud are counted")
		}).Should(Succeed())

		By("deleting the SolrCloud")
		Expect(k8sClient.Delete(ctx, solrCloud)).To(Succeed())
		Eventually(func(g Gomega) {
			series := gatherOperatorMetrics(g)
			g.Expect(series).NotTo(HaveKey(`solr_operator_solrcloud_desired_replicas{name="metrics-foo",namespace="default"}`), "the gauges go away with the SolrCloud")
			for name := range series {
				g.Expect(name).NotTo(HavePrefix(`solr_operator_reconciles_total{kind="SolrCloud",name="metrics-foo"`), "the counters go away with the SolrCloud")
			}
		}).Should(Succeed())
	})
})

// gatherOperatorMetrics scrapes the registry behind the operator's metrics endpoint, and returns the Solr Operator
// series as `name{label="value",...}` -> value.
func gatherOperatorMetrics(g Gomega) map[string]float64 {
	families, err := ctrlmetrics.Registry.Gather()
	g.Expect(err).ToNot(HaveOccurred())
	return metricSeries(families)
}
