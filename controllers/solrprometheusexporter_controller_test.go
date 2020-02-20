/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

var _ reconcile.Reconciler = &SolrPrometheusExporterReconciler{}

var (
	expectedMetricsRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-met", Namespace: "default"}}
	metricsDKey            = types.NamespacedName{Name: "foo-met-solr-metrics", Namespace: "default"}
	metricsSKey            = types.NamespacedName{Name: "foo-met-solr-metrics", Namespace: "default"}
	metricsCMKey           = types.NamespacedName{Name: "foo-met-solr-metrics", Namespace: "default"}

	testExporterConfig = "THis is a test config."
)

func TestMetricsReconcileWithoutExporterConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrPrometheusExporter{ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace}}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrPrometheusExporterReconciler := &SolrPrometheusExporterReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrPrometheusExporter"),
	}
	newRec, requests := SetupTestReconcile(solrPrometheusExporterReconciler)
	g.Expect(solrPrometheusExporterReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the SolrPrometheusExporter object and expect the Reconcile and Deployment to be created
	err = testClient.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedMetricsRequest)))

	expectNoConfigMap(g, requests, metricsCMKey)

	deployment := expectDeployment(t, g, requests, expectedMetricsRequest, metricsDKey, "")

	service := expectService(t, g, requests, expectedMetricsRequest, metricsSKey, deployment.Spec.Template.Labels)
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
}

var (
	testMetricsPodAnnotations = map[string]string{
		"testP1": "valueP1",
		"testP2": "valueP2",
	}
	testMetricsPodLabels = map[string]string{
		"testP3": "valueP3",
		"testP4": "valueP4",
	}
	testMetricsDeploymentAnnotations = map[string]string{
		"testD1": "valueD1",
		"testD2": "valueD2",
	}
	testMetricsDeploymentLabels = map[string]string{
		"testD3": "valueD3",
		"testD4": "valueD4",
	}
	testMetricsServiceLabels = map[string]string{
		"testS1": "valueS1",
		"testS2": "valueS2",
	}
	testMetricsServiceAnnotations = map[string]string{
		"testS3": "valueS3",
		"testS4": "valueS4",
	}
	testMetricsConfigMapLabels = map[string]string{
		"testCM1": "valueCM1",
		"testCM2": "valueCM2",
	}
	testMetricsConfigMapAnnotations = map[string]string{
		"testCM3": "valueCM3",
		"testCM4": "valueCM4",
	}
)

func TestMetricsReconcileWithExporterConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace},
		Spec: solr.SolrPrometheusExporterSpec{
			Config: testExporterConfig,
			CustomKubeOptions: solr.CustomExporterKubeOptions{
				PodOptions: &solr.PodOptions{
					Annotations: testMetricsPodAnnotations,
					Labels:      testMetricsPodLabels,
				},
				DeploymentOptions: &solr.DeploymentOptions{
					Annotations: testMetricsDeploymentAnnotations,
					Labels:      testMetricsDeploymentLabels,
				},
				ServiceOptions: &solr.ServiceOptions{
					Annotations: testMetricsServiceAnnotations,
					Labels:      testMetricsServiceLabels,
				},
				ConfigMapOptions: &solr.ConfigMapOptions{
					Annotations: testMetricsConfigMapAnnotations,
					Labels:      testMetricsConfigMapLabels,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrPrometheusExporterReconciler := &SolrPrometheusExporterReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrPrometheusExporter"),
	}
	newRec, requests := SetupTestReconcile(solrPrometheusExporterReconciler)
	g.Expect(solrPrometheusExporterReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the SolrPrometheusExporter object and expect the Reconcile and Deployment to be created
	err = testClient.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedMetricsRequest)))

	configMap := expectConfigMap(t, g, requests, expectedMetricsRequest, metricsCMKey, map[string]string{"solr-prometheus-exporter.xml": testExporterConfig})
	testMapsEqual(t, "configMap labels", util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), testMetricsConfigMapLabels), configMap.Labels)
	testMapsEqual(t, "configMap annotations", testMetricsConfigMapAnnotations, configMap.Annotations)

	deployment := expectDeployment(t, g, requests, expectedMetricsRequest, metricsDKey, configMap.Name)
	expectedDeploymentLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"technology": solr.SolrPrometheusExporterTechnologyLabel})
	testMapsEqual(t, "deployment labels", util.MergeLabelsOrAnnotations(expectedDeploymentLabels, testMetricsDeploymentLabels), deployment.Labels)
	testMapsEqual(t, "deployment annotations", testMetricsDeploymentAnnotations, deployment.Annotations)
	testMapsEqual(t, "pod labels", util.MergeLabelsOrAnnotations(expectedDeploymentLabels, testMetricsPodLabels), deployment.Spec.Template.ObjectMeta.Labels)
	testMapsEqual(t, "pod annotations", testMetricsPodAnnotations, deployment.Spec.Template.ObjectMeta.Annotations)

	expectedServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "metrics"})
	expectedServiceAnnotations := map[string]string{"prometheus.io/path": "/metrics", "prometheus.io/port": "80", "prometheus.io/scheme": "http", "prometheus.io/scrape": "true"}
	service := expectService(t, g, requests, expectedMetricsRequest, metricsSKey, expectedDeploymentLabels)
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
	testMapsEqual(t, "service labels", util.MergeLabelsOrAnnotations(expectedServiceLabels, testMetricsServiceLabels), service.Labels)
	testMapsEqual(t, "service annotations", util.MergeLabelsOrAnnotations(expectedServiceAnnotations, testMetricsServiceAnnotations), service.Annotations)
}
