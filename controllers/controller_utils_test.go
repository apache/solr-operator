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
	"reflect"
	"testing"

	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func emptyRequests(requests chan reconcile.Request) {
	for len(requests) > 0 {
		<-requests
	}
}

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

func expectNoService(g *gomega.GomegaWithT, serviceKey types.NamespacedName, message string) {
	service := &corev1.Service{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.MatchError("Service \""+serviceKey.Name+"\" not found"), message)
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

func expectNoIngress(g *gomega.GomegaWithT, ingressKey types.NamespacedName) {
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

func expectNoConfigMap(g *gomega.GomegaWithT, configMapKey types.NamespacedName) {
	configMap := &corev1.ConfigMap{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), configMapKey, configMap) }, timeout).
		Should(gomega.MatchError("ConfigMap \"" + configMapKey.Name + "\" not found"))
}

func expectDeployment(t *testing.T, g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, deploymentKey types.NamespacedName, configMapName string) *appsv1.Deployment {
	deploy := &appsv1.Deployment{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), deploymentKey, deploy) }, timeout).
		Should(gomega.Succeed())

	// Verify the deployment Specs
	testMapContainsOther(t, "Deployment pod template selector", deploy.Spec.Template.Labels, deploy.Spec.Selector.MatchLabels)
	assert.GreaterOrEqual(t, len(deploy.Spec.Selector.MatchLabels), 1, "Deployment pod template selector must have at least 1 label")

	if configMapName != "" {
		if assert.LessOrEqual(t, 1, len(deploy.Spec.Template.Spec.Volumes), "Deployment should have at least 1 volume, the configMap. Less were found.") {
			assert.Equal(t, configMapName, deploy.Spec.Template.Spec.Volumes[0].ConfigMap.Name, "Deployment configMap volume has the wrong name.")
		}
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

func testPodTolerations(t *testing.T, expectedTolerations []corev1.Toleration, foundTolerations []corev1.Toleration) {
	assert.True(t, reflect.DeepEqual(expectedTolerations, foundTolerations), "Expected tolerations and found tolerations don't match")
}

func testMapsEqual(t *testing.T, mapName string, expected map[string]string, found map[string]string) {
	assert.Equal(t, expected, found, "Expected and found %s are not the same", mapName)
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

var (
	testKubeDomain        = "kube.domain.com"
	testDomain            = "test.domain.com"
	testAdditionalDomains = []string{
		"test1.domain.com",
		"test2.domain.com",
	}
	testPodAnnotations = map[string]string{
		"testP1": "valueP1",
		"testP2": "valueP2",
	}
	testPodLabels = map[string]string{
		"testP3": "valueP3",
		"testP4": "valueP4",
	}
	testSSAnnotations = map[string]string{
		"testSS1": "valueSS1",
		"testSS2": "valueSS2",
	}
	testSSLabels = map[string]string{
		"testSS3": "valueSS3",
		"testSS4": "valueSS4",
	}
	testIngressLabels = map[string]string{
		"testI1": "valueI1",
		"testI2": "valueI2",
	}
	testIngressAnnotations = map[string]string{
		"testI3": "valueI3",
		"testI4": "valueI4",
	}
	testCommonServiceLabels = map[string]string{
		"testCS1": "valueCS1",
		"testCS2": "valueCS2",
	}
	testCommonServiceAnnotations = map[string]string{
		"testCS3": "valueCS3",
		"testCS4": "valueCS4",
	}
	testHeadlessServiceLabels = map[string]string{
		"testHS1": "valueHS1",
		"testHS2": "valueHS2",
	}
	testHeadlessServiceAnnotations = map[string]string{
		"testHS3": "valueHS3",
		"testHS4": "valueHS4",
	}
	testNodeServiceLabels = map[string]string{
		"testNS1": "valueNS1",
		"testNS2": "valueNS2",
	}
	testNodeServiceAnnotations = map[string]string{
		"testNS3": "valueNS3",
		"testNS4": "valueNS4",
	}
	testConfigMapLabels = map[string]string{
		"testCM1": "valueCM1",
		"testCM2": "valueCM2",
	}
	testConfigMapAnnotations = map[string]string{
		"testCM3": "valueCM3",
		"testCM4": "valueCM4",
	}
	testDeploymentAnnotations = map[string]string{
		"testD1": "valueD1",
		"testD2": "valueD2",
	}
	testDeploymentLabels = map[string]string{
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
	testNodeSelectors = map[string]string{
		"beta.kubernetes.io/arch": "amd64",
		"beta.kubernetes.io/os":   "linux",
		"solrclouds":              "true",
	}
	testTolerations = []corev1.Toleration{
		{
			Effect:   "NoSchedule",
			Key:      "node-restriction.kubernetes.io/dedicated",
			Value:    "solrclouds",
			Operator: "Exists",
		},
	}
	testTolerationsPromExporter = []corev1.Toleration{
		{
			Effect:   "NoSchedule",
			Operator: "Exists",
		},
	}
	extraVars = []corev1.EnvVar{
		{
			Name:  "VAR_1",
			Value: "VAL_1",
		},
		{
			Name: "VAR_2",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
	}
	one                = int64(1)
	two                = int64(2)
	podSecurityContext = corev1.PodSecurityContext{
		RunAsUser:  &one,
		RunAsGroup: &two,
	}
	extraVolumes = []solr.AdditionalVolume{
		{
			Name: "vol1",
			Source: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
			DefaultContainerMount: corev1.VolumeMount{
				Name:      "ignore",
				ReadOnly:  false,
				MountPath: "/test/mount/path",
				SubPath:   "sub/",
			},
		},
	}
	affinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					TopologyKey: "testKey",
				},
			},
			PreferredDuringSchedulingIgnoredDuringExecution: nil,
		},
	}
	resources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: *resource.NewMilliQuantity(5300, resource.DecimalSI),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceEphemeralStorage: resource.MustParse("5Gi"),
		},
	}
)
