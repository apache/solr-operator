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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("SolrPrometheusExporter controller - Cross-namespace SolrCloud reference", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 5
		duration = time.Second * 1
		interval = time.Millisecond * 250

		// envtest has no namespace controller, so a created namespace can never be fully deleted.
		// Therefore this namespace is created once and re-used by all specs in this suite.
		solrCloudNamespace = "cross-namespace-solr"

		imageBeforeChange = "before-change"
		imageAfterChange  = "after-change"
	)
	SetDefaultConsistentlyDuration(duration)
	SetDefaultConsistentlyPollingInterval(interval)
	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	var (
		solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter
		solrRef                *solrv1beta1.SolrCloud
	)

	BeforeEach(func(ctx context.Context) {
		By("creating the namespace that the referenced SolrCloud lives in")
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: solrCloudNamespace}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, namespace))).To(Succeed(), "Creating the namespace for the referenced SolrCloud")

		By("creating the SolrCloud in the other namespace")
		solrRef = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-solr",
				Namespace: solrCloudNamespace,
			},
			Spec: solrv1beta1.SolrCloudSpec{
				SolrImage: &solrv1beta1.ContainerImage{
					Tag: imageBeforeChange,
				},
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host-from-solr:2181",
						ChRoot:                   "/this/path",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, solrRef)).To(Succeed(), "Creating test SolrCloud for Prometheus Exporter to connect to")

		By("creating the SolrPrometheusExporter that references it")
		solrPrometheusExporter = &solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						Name:      solrRef.Name,
						Namespace: solrRef.Namespace,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrPrometheusExporter)
		cleanupTest(ctx, solrRef)
	})

	FIt("reconciles the exporter when the referenced SolrCloud changes", func(ctx context.Context) {
		By("using the Solr image of the referenced SolrCloud")
		expectExporterSolrImageTag(ctx, solrPrometheusExporter, imageBeforeChange)

		By("changing the Solr image of the SolrCloud, leaving the exporter untouched")
		foundSolrCloud := &solrv1beta1.SolrCloud{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(solrRef), foundSolrCloud)).To(Succeed(), "Fetching the SolrCloud to update")
		foundSolrCloud.Spec.SolrImage.Tag = imageAfterChange
		Expect(k8sClient.Update(ctx, foundSolrCloud)).To(Succeed(), "Updating the Solr image of the SolrCloud")

		By("picking up the new Solr image, which requires watching SolrClouds in other namespaces")
		expectExporterSolrImageTag(ctx, solrPrometheusExporter, imageAfterChange)
	})
})

// expectExporterSolrImageTag waits for the Deployment of the given SolrPrometheusExporter to use the
// given Solr image tag. Unlike expectDeployment(), it leaves the Deployment alone, since deleting it
// would trigger a reconcile of the exporter through the Deployment watch, which is exactly what must
// not be relied on here.
func expectExporterSolrImageTag(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, imageTag string, additionalOffset ...int) {
	deploymentName := solrPrometheusExporter.MetricsDeploymentName()
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		deployment := &appsv1.Deployment{}
		g.Expect(k8sClient.Get(ctx, resourceKey(solrPrometheusExporter, deploymentName), deployment)).To(Succeed(), "Expected Deployment does not exist")
		g.Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(solrv1beta1.DefaultSolrRepo+":"+imageTag), "Incorrect image, should be taken from the referenced SolrCloud")
	}).Should(Succeed())
}
