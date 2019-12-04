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

func TestMetricsReconcileWithExporterConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace},
		Spec: solr.SolrPrometheusExporterSpec{
			Config: testExporterConfig,
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

	deployment := expectDeployment(t, g, requests, expectedMetricsRequest, metricsDKey, configMap.Name)

	service := expectService(t, g, requests, expectedMetricsRequest, metricsSKey, deployment.Spec.Template.Labels)
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
}
