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

package solrprometheusexporter

import (
	"testing"
	"time"

	solrv1beta1 "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo-solr-metrics", Namespace: "default"}
var serviceKey = types.NamespacedName{Name: "foo-solr-metrics", Namespace: "default"}
var configMapKey = types.NamespacedName{Name: "foo-solr-metrics", Namespace: "default"}
var additionalLables = map[string]string{
	"additional": "label",
	"another":    "test",
}

const testExporterConfig = "THis is a test config."

const timeout = time.Second * 5

func TestReconcileWithoutExporterConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solrv1beta1.SolrPrometheusExporter{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"}}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the SolrPrometheusExporter object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	expectNoConfigMap(g, requests, configMapKey)

	foundDeployment := expectDeployment(t, g, requests, depKey, false)

	expectService(t, g, requests, serviceKey, foundDeployment)
}

func TestReconcileWithExporterConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solrv1beta1.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solrv1beta1.SolrPrometheusExporterSpec{
			Config: testExporterConfig,
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the SolrPrometheusExporter object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	expectConfigMap(t, g, requests, configMapKey)

	foundDeployment := expectDeployment(t, g, requests, depKey, true)

	expectService(t, g, requests, serviceKey, foundDeployment)
}

func expectConfigMap(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, configMapKey types.NamespacedName) {
	configMap := &corev1.ConfigMap{}
	g.Eventually(func() error { return c.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.Succeed())

	// Verify the ConfigMap Specs
	assert.Equal(t, configMap.Data["solr-prometheus-exporter.xml"], testExporterConfig, "Metrics ConfigMap does not have the correct data.")

	// Delete the ConfigMap and expect Reconcile to be called for Deployment deletion
	g.Expect(c.Delete(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.Succeed())

	// Manually delete ConfigMap since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), configMap) }, timeout).
		Should(gomega.MatchError("configmaps \"" + configMapKey.Name + "\" not found"))
}

func expectNoConfigMap(g *gomega.GomegaWithT, requests chan reconcile.Request, configMapKey types.NamespacedName) {
	configMap := &corev1.ConfigMap{}
	g.Eventually(func() error { return c.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.MatchError("ConfigMap \"" + configMapKey.Name + "\" not found"))
}

func expectDeployment(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, deploymentKey types.NamespacedName, usesConfig bool) *appsv1.Deployment {
	deploy := &appsv1.Deployment{}
	g.Eventually(func() error { return c.Get(context.TODO(), deploymentKey, deploy) }, timeout).
		Should(gomega.Succeed())

	// Verify the deployment Specs
	assert.Equal(t, deploy.Spec.Template.Labels, deploy.Spec.Selector.MatchLabels, "Metrics Deployment has different Pod template labels and selector labels.")

	if usesConfig {
		if assert.Equal(t, 1, len(deploy.Spec.Template.Spec.Volumes), "Metrics Deployment should have 1 volume, the configMap. More or less were found.") {
			assert.Equal(t, configMapKey.Name, deploy.Spec.Template.Spec.Volumes[0].ConfigMap.Name, "Metrics Deployment should have 1 volume, the configMap. More or less were found.")
		}
	} else {
		assert.Equal(t, 0, len(deploy.Spec.Template.Spec.Volumes), "Metrics Deployment should have no volumes, since there is no configMap. Volumes were found.")
	}

	// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	g.Expect(c.Delete(context.TODO(), deploy)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), deploymentKey, deploy) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Deployment since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), deploy) }, timeout).
		Should(gomega.MatchError("deployments.apps \"" + deploymentKey.Name + "\" not found"))

	return deploy
}

func expectService(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, serviceKey types.NamespacedName, foundDeployment *appsv1.Deployment) {
	service := &corev1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.Succeed())

	// Verify the Service specs
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
	assert.Equal(t, foundDeployment.Spec.Template.Labels, service.Spec.Selector, "Metrics Service is not pointing to the correct Pods.")

	// Delete the Service and expect Reconcile to be called for Service deletion
	g.Expect(c.Delete(context.TODO(), service)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Service since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), service) }, timeout).
		Should(gomega.MatchError("services \"" + serviceKey.Name + "\" not found"))
}
