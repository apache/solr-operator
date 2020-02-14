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
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

func expectStatefulSet(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, statefulSetKey types.NamespacedName) *appsv1.StatefulSet {
	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), statefulSetKey, stateful) }, timeout).
		Should(gomega.Succeed())

	// Verify the statefulSet Specs
	testMapContainsOther(t, "StatefulSet pod template selector", stateful.Spec.Template.Labels, stateful.Spec.Selector.MatchLabels)
	assert.GreaterOrEqual(t, len(stateful.Spec.Selector.MatchLabels), 1, "StatefulSet pod template selector must have at least 1 label")

	// Delete the StatefulSet and expect Reconcile to be called for StatefulSet deletion
	g.Expect(testClient.Delete(context.TODO(), stateful)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return testClient.Get(context.TODO(), statefulSetKey, stateful) }, timeout).
		Should(gomega.Succeed())

	// Manually delete StatefulSet since GC isn't enabled in the test control plane
	g.Eventually(func() error { return testClient.Delete(context.TODO(), stateful) }, timeout).
		Should(gomega.MatchError("statefulsets.apps \"" + statefulSetKey.Name + "\" not found"))

	return stateful
}

func expectNoStatefulSet(g *gomega.GomegaWithT, statefulSetKey types.NamespacedName) {
	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), statefulSetKey, stateful) }, timeout).
		Should(gomega.MatchError("StatefulSet.apps \"" + statefulSetKey.Name + "\" not found"))
}

func expectService(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, serviceKey types.NamespacedName, selectorLables map[string]string) *corev1.Service {
	service := &corev1.Service{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.Succeed())

	assert.Equal(t, selectorLables, service.Spec.Selector, "Service is not pointing to the correct Pods.")

	// Delete the Service and expect Reconcile to be called for Service deletion
	g.Expect(testClient.Delete(context.TODO(), service)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return testClient.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Service since GC isn't enabled in the test control plane
	g.Eventually(func() error { return testClient.Delete(context.TODO(), service) }, timeout).
		Should(gomega.MatchError("services \"" + serviceKey.Name + "\" not found"))

	return service
}

func expectIngress(g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, ingressKey types.NamespacedName) *extv1.Ingress {
	ingress := &extv1.Ingress{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), ingressKey, ingress) }, timeout).
		Should(gomega.Succeed())

	// Delete the Ingress and expect Reconcile to be called for Ingress deletion
	g.Expect(testClient.Delete(context.TODO(), ingress)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return testClient.Get(context.TODO(), ingressKey, ingress) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Ingress since GC isn't enabled in the test control plane
	g.Eventually(func() error { return testClient.Delete(context.TODO(), ingress) }, timeout).
		Should(gomega.MatchError("ingresses.extensions \"" + ingressKey.Name + "\" not found"))

	return ingress
}

func expectNoIngress(g *gomega.GomegaWithT, requests chan reconcile.Request, ingressKey types.NamespacedName) {
	ingress := &extv1.Ingress{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), ingressKey, ingress) }, timeout).
		Should(gomega.MatchError("Ingress.extensions \"" + ingressKey.Name + "\" not found"))
}

func expectConfigMap(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, configMapKey types.NamespacedName, configMapData map[string]string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.Succeed())

	// Verify the ConfigMap Specs
	for k, d := range configMapData {
		assert.Equal(t, configMap.Data[k], d, "ConfigMap does not have the correct data.")
	}

	// Delete the ConfigMap and expect Reconcile to be called for Deployment deletion
	g.Expect(testClient.Delete(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return testClient.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.Succeed())

	// Manually delete ConfigMap since GC isn't enabled in the test control plane
	g.Eventually(func() error { return testClient.Delete(context.TODO(), configMap) }, timeout).
		Should(gomega.MatchError("configmaps \"" + configMapKey.Name + "\" not found"))

	return configMap
}

func expectNoConfigMap(g *gomega.GomegaWithT, requests chan reconcile.Request, configMapKey types.NamespacedName) {
	configMap := &corev1.ConfigMap{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.MatchError("ConfigMap \"" + configMapKey.Name + "\" not found"))
}

func expectDeployment(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, deploymentKey types.NamespacedName, configMapName string) *appsv1.Deployment {
	deploy := &appsv1.Deployment{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), deploymentKey, deploy) }, timeout).
		Should(gomega.Succeed())

	// Verify the deployment Specs
	assert.Equal(t, deploy.Spec.Template.Labels, deploy.Spec.Selector.MatchLabels, "Deployment has different Pod template labels and selector labels.")

	if configMapName != "" {
		if assert.Equal(t, 1, len(deploy.Spec.Template.Spec.Volumes), "Deployment should have 1 volume, the configMap. More or less were found.") {
			assert.Equal(t, configMapName, deploy.Spec.Template.Spec.Volumes[0].ConfigMap.Name, "Deployment should have 1 volume, the configMap. More or less were found.")
		}
	} else {
		assert.Equal(t, 0, len(deploy.Spec.Template.Spec.Volumes), "Deployment should have no volumes, since there is no configMap. Volumes were found.")
	}

	// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	g.Expect(testClient.Delete(context.TODO(), deploy)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return testClient.Get(context.TODO(), deploymentKey, deploy) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Deployment since GC isn't enabled in the test control plane
	g.Eventually(func() error { return testClient.Delete(context.TODO(), deploy) }, timeout).
		Should(gomega.MatchError("deployments.apps \"" + deploymentKey.Name + "\" not found"))

	return deploy
}

func testPodEnvVariables(t *testing.T, expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar) {
	matchCount := 0
	for _, envVar := range foundEnvVars {
		if expectedVal, match := expectedEnvVars[envVar.Name]; match {
			matchCount += 1
			assert.Equal(t, expectedVal, envVar.Value, "Wrong value for env variable '%s' in podSpec", envVar.Name)
		}
	}
	assert.Equal(t, len(expectedEnvVars), matchCount, "Not all expected env variables found in podSpec")
}

func testMapsEqual(t *testing.T, mapName string, expected map[string]string, found map[string]string) {
	for k, v := range expected {
		foundV, foundExists := found[k]
		assert.True(t, foundExists, "Expected key '%s' does not exist in found %s", k, mapName)
		if foundExists {
			assert.Equal(t, v, foundV, "Wrong value for %s key '%s'", mapName, k)
		}
	}
	assert.Equal(t, len(expected), len(found), "Expected and found %s do not have the same number of entries", mapName)
}

func testMapContainsOther(t *testing.T, mapName string, base map[string]string, other map[string]string) {
	for k, v := range other {
		foundV, foundExists := base[k]
		assert.True(t, foundExists, "Expected key '%s' does not exist in found %s", k, mapName)
		if foundExists {
			assert.Equal(t, v, foundV, "Wrong value for %s key '%s'", mapName, k)
		}
	}
}

func cleanupTest(g *gomega.GomegaWithT, namespace string) {
	deleteOpts := []client.DeleteAllOfOption{
		client.InNamespace(namespace),
	}

	cleanupObjects := []runtime.Object{
		// Solr Operator CRDs, modify this list whenever CRDs are added/deleted
		&solr.SolrCloud{}, &solr.SolrBackup{}, &solr.SolrCollection{}, &solr.SolrCollectionAlias{}, &solr.SolrPrometheusExporter{},

		// All dependent Kubernetes types, in order of dependence (deployment then replicaSet then pod)
		&corev1.ConfigMap{}, &batchv1.Job{}, &extv1.Ingress{},
		&corev1.PersistentVolumeClaim{}, &corev1.PersistentVolume{},
		&appsv1.StatefulSet{}, &appsv1.Deployment{}, &appsv1.ReplicaSet{}, &corev1.Pod{},
	}
	cleanupTestObjects(g, namespace, deleteOpts, cleanupObjects)

	// Delete all Services separately (https://github.com/kubernetes/kubernetes/pull/85802#issuecomment-561239845)
	opts := []client.ListOption{
		client.InNamespace(namespace),
	}
	services := &corev1.ServiceList{}
	g.Eventually(func() error { return testClient.List(context.TODO(), services, opts...) }, timeout).Should(gomega.Succeed())

	for _, item := range services.Items {
		g.Eventually(func() error { return testClient.Delete(context.TODO(), &item) }, timeout).Should(gomega.Succeed())
	}
}

func cleanupTestObjects(g *gomega.GomegaWithT, namespace string, deleteOpts []client.DeleteAllOfOption, objects []runtime.Object) {
	// Delete all SolrClouds
	for _, obj := range objects {
		g.Eventually(func() error { return testClient.DeleteAllOf(context.TODO(), obj, deleteOpts...) }, timeout).Should(gomega.Succeed())
	}
}
