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
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

func expectStatefulSet(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, statefulSetKey types.NamespacedName) *appsv1.StatefulSet {
	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), statefulSetKey, stateful) }, timeout).
		Should(gomega.Succeed())

	// Verify the statefulSet Specs
	assert.Equal(t, stateful.Spec.Template.Labels, stateful.Spec.Selector.MatchLabels, "StatefulSet has different Pod template labels and selector labels.")

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
