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
	"testing"

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
	instance := &solr.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace},
		Spec: solr.SolrPrometheusExporterSpec{
			CustomKubeOptions: solr.CustomExporterKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables:       extraVars,
					PodSecurityContext: &podSecurityContext,
					Volumes:            extraVolumes,
					Affinity:           affinity,
					Resources:          resources,
				},
			},
			ExporterEntrypoint: "/test/entry-point",
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

	expectNoConfigMap(g, metricsCMKey)

	deployment := expectDeployment(t, g, requests, expectedMetricsRequest, metricsDKey, "")

	// Pod Options Checks
	assert.Equal(t, extraVars, deployment.Spec.Template.Spec.Containers[0].Env, "Extra Env Vars are not the same as the ones provided in podOptions")
	assert.Equal(t, podSecurityContext, *deployment.Spec.Template.Spec.SecurityContext, "PodSecurityContext is not the same as the one provided in podOptions")
	assert.Equal(t, affinity, deployment.Spec.Template.Spec.Affinity, "Affinity is not the same as the one provided in podOptions")
	assert.Equal(t, resources.Limits, deployment.Spec.Template.Spec.Containers[0].Resources.Limits, "Resources.Limits is not the same as the one provided in podOptions")
	assert.Equal(t, resources.Requests, deployment.Spec.Template.Spec.Containers[0].Resources.Requests, "Resources.Requests is not the same as the one provided in podOptions")
	extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
	assert.Equal(t, len(extraVolumes), len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts), "Container has wrong number of volumeMounts")
	assert.Equal(t, extraVolumes[0].DefaultContainerMount, deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0], "Additional Volume from podOptions not mounted into container properly.")
	assert.Equal(t, len(extraVolumes), len(deployment.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, extraVolumes[0].Name, deployment.Spec.Template.Spec.Volumes[0].Name, "Additional Volume from podOptions not loaded into pod properly.")
	assert.Equal(t, extraVolumes[0].Source, deployment.Spec.Template.Spec.Volumes[0].VolumeSource, "Additional Volume from podOptions not loaded into pod properly.")

	service := expectService(t, g, requests, expectedMetricsRequest, metricsSKey, deployment.Spec.Template.Labels)
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
	assert.EqualValues(t, "solr-metrics", service.Spec.Ports[0].Name, "Wrong port name on common Service")
}

func TestMetricsReconcileWithExporterConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace},
		Spec: solr.SolrPrometheusExporterSpec{
			Config: testExporterConfig,
			CustomKubeOptions: solr.CustomExporterKubeOptions{
				PodOptions: &solr.PodOptions{
					Annotations:  testPodAnnotations,
					Labels:       testPodLabels,
					Volumes:      extraVolumes,
					Tolerations:  testTolerationsPromExporter,
					NodeSelector: testNodeSelectors,
				},
				DeploymentOptions: &solr.DeploymentOptions{
					Annotations: testDeploymentAnnotations,
					Labels:      testDeploymentLabels,
				},
				ServiceOptions: &solr.ServiceOptions{
					Annotations: testMetricsServiceAnnotations,
					Labels:      testMetricsServiceLabels,
				},
				ConfigMapOptions: &solr.ConfigMapOptions{
					Annotations: testConfigMapAnnotations,
					Labels:      testConfigMapLabels,
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

	cleanupTest(g, instance.Namespace)

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
	testMapsEqual(t, "configMap labels", util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), testConfigMapLabels), configMap.Labels)
	testMapsEqual(t, "configMap annotations", testConfigMapAnnotations, configMap.Annotations)

	deployment := expectDeployment(t, g, requests, expectedMetricsRequest, metricsDKey, configMap.Name)
	expectedDeploymentLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"technology": solr.SolrPrometheusExporterTechnologyLabel})
	testMapsEqual(t, "deployment labels", util.MergeLabelsOrAnnotations(expectedDeploymentLabels, testDeploymentLabels), deployment.Labels)
	testMapsEqual(t, "deployment annotations", testDeploymentAnnotations, deployment.Annotations)
	testMapsEqual(t, "pod labels", util.MergeLabelsOrAnnotations(expectedDeploymentLabels, testPodLabels), deployment.Spec.Template.ObjectMeta.Labels)
	testMapsEqual(t, "pod annotations", testPodAnnotations, deployment.Spec.Template.ObjectMeta.Annotations)

	// Test tolerations and node selectors
	testMapsEqual(t, "pod node selectors", testNodeSelectors, deployment.Spec.Template.Spec.NodeSelector)
	testPodTolerations(t, testTolerationsPromExporter, deployment.Spec.Template.Spec.Tolerations)

	// Other Pod Options
	extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
	assert.Equal(t, len(extraVolumes)+1, len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts), "Container has wrong number of volumeMounts")
	assert.Equal(t, extraVolumes[0].DefaultContainerMount, deployment.Spec.Template.Spec.Containers[0].VolumeMounts[1], "Additional Volume from podOptions not mounted into container properly.")
	assert.Equal(t, len(extraVolumes)+1, len(deployment.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, extraVolumes[0].Name, deployment.Spec.Template.Spec.Volumes[1].Name, "Additional Volume from podOptions not loaded into pod properly.")
	assert.Equal(t, extraVolumes[0].Source, deployment.Spec.Template.Spec.Volumes[1].VolumeSource, "Additional Volume from podOptions not loaded into pod properly.")

	expectedServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "metrics"})
	expectedServiceAnnotations := map[string]string{"prometheus.io/path": "/metrics", "prometheus.io/port": "80", "prometheus.io/scheme": "http", "prometheus.io/scrape": "true"}
	service := expectService(t, g, requests, expectedMetricsRequest, metricsSKey, expectedDeploymentLabels)
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
	testMapsEqual(t, "service labels", util.MergeLabelsOrAnnotations(expectedServiceLabels, testMetricsServiceLabels), service.Labels)
	testMapsEqual(t, "service annotations", util.MergeLabelsOrAnnotations(expectedServiceAnnotations, testMetricsServiceAnnotations), service.Annotations)
	assert.EqualValues(t, "solr-metrics", service.Spec.Ports[0].Name, "Wrong port name on common Service")
}
