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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"regexp"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	zk_api "github.com/apache/solr-operator/controllers/zk_api"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Add one to an optional offset
func resolveOffset(additionalOffset []int) (offset int) {
	if len(additionalOffset) == 0 {
		offset = 0
	} else {
		offset = additionalOffset[0]
	}
	return offset + 1
}

func resourceKey(parentResource client.Object, name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: parentResource.GetNamespace()}
}

func expectSolrCloud(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalOffset ...int) *solrv1beta1.SolrCloud {
	return expectSolrCloudWithChecks(ctx, solrCloud, nil, resolveOffset(additionalOffset))
}

func expectSolrCloudWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalChecks func(Gomega, *solrv1beta1.SolrCloud), additionalOffset ...int) *solrv1beta1.SolrCloud {
	foundSolrCloud := &solrv1beta1.SolrCloud{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), foundSolrCloud)).To(Succeed(), "Expected SolrCloud does not exist")
		if additionalChecks != nil {
			additionalChecks(g, foundSolrCloud)
		}
	}).Should(Succeed())

	return foundSolrCloud
}

func expectSolrCloudWithConsistentChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalChecks func(Gomega, *solrv1beta1.SolrCloud), additionalOffset ...int) *solrv1beta1.SolrCloud {
	foundSolrCloud := &solrv1beta1.SolrCloud{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), foundSolrCloud)).To(Succeed(), "Expected SolrCloud does not exist")
		if additionalChecks != nil {
			additionalChecks(g, foundSolrCloud)
		}
	}).Should(Succeed())

	return foundSolrCloud
}

func expectSolrCloudStatus(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalOffset ...int) *solrv1beta1.SolrCloudStatus {
	return expectSolrCloudStatusWithChecks(ctx, solrCloud, nil, resolveOffset(additionalOffset))
}

func expectSolrCloudStatusWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalChecks func(Gomega, *solrv1beta1.SolrCloudStatus), additionalOffset ...int) *solrv1beta1.SolrCloudStatus {
	foundSolrCloud := &solrv1beta1.SolrCloud{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), foundSolrCloud)).To(Succeed(), "Expected SolrCloud does not exist")
		if additionalChecks != nil {
			additionalChecks(g, &foundSolrCloud.Status)
		}
	}).Should(Succeed())

	return &foundSolrCloud.Status
}

func expectSolrCloudStatusConsistentWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalChecks func(Gomega, *solrv1beta1.SolrCloudStatus), additionalOffset ...int) *solrv1beta1.SolrCloudStatus {
	foundSolrCloud := &solrv1beta1.SolrCloud{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), foundSolrCloud)).To(Succeed(), "Expected SolrCloud does not exist")
		if additionalChecks != nil {
			additionalChecks(g, &foundSolrCloud.Status)
		}
	}).Should(Succeed())

	return &foundSolrCloud.Status
}

func expectSolrPrometheusExporter(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, additionalOffset ...int) *solrv1beta1.SolrPrometheusExporter {
	return expectSolrPrometheusExporterWithChecks(ctx, solrPrometheusExporter, nil, resolveOffset(additionalOffset))
}

func expectSolrPrometheusExporterWithChecks(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, additionalChecks func(Gomega, *solrv1beta1.SolrPrometheusExporter), additionalOffset ...int) *solrv1beta1.SolrPrometheusExporter {
	foundSolrPrometheusExporter := &solrv1beta1.SolrPrometheusExporter{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrPrometheusExporter, solrPrometheusExporter.Name), foundSolrPrometheusExporter)).To(Succeed(), "Expected SolrPrometheusExporter does not exist")
		if additionalChecks != nil {
			additionalChecks(g, foundSolrPrometheusExporter)
		}
	}).Should(Succeed())

	return foundSolrPrometheusExporter
}

func expectSolrPrometheusExporterWithConsistentChecks(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrCloud, additionalChecks func(Gomega, *solrv1beta1.SolrCloud), additionalOffset ...int) *solrv1beta1.SolrCloud {
	foundSolrPrometheusExporter := &solrv1beta1.SolrCloud{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrPrometheusExporter, solrPrometheusExporter.Name), foundSolrPrometheusExporter)).To(Succeed(), "Expected SolrPrometheusExporter does not exist")
		if additionalChecks != nil {
			additionalChecks(g, foundSolrPrometheusExporter)
		}
	}).Should(Succeed())

	return foundSolrPrometheusExporter
}

func expectSecret(ctx context.Context, parentResource client.Object, secretName string, additionalOffset ...int) *corev1.Secret {
	return expectSecretWithChecks(ctx, parentResource, secretName, nil, resolveOffset(additionalOffset))
}

func expectSecretWithChecks(ctx context.Context, parentResource client.Object, secretName string, additionalChecks func(Gomega, *corev1.Secret), additionalOffset ...int) *corev1.Secret {
	found := &corev1.Secret{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, secretName), found)).To(Succeed(), "Expected Secret does not exist")
		if additionalChecks != nil {
			additionalChecks(g, found)
		}
	}).Should(Succeed())

	return found
}

func expectStatefulSet(ctx context.Context, parentResource client.Object, statefulSetName string, additionalOffset ...int) *appsv1.StatefulSet {
	return expectStatefulSetWithChecks(ctx, parentResource, statefulSetName, nil, resolveOffset(additionalOffset))
}

func expectStatefulSetWithChecks(ctx context.Context, parentResource client.Object, statefulSetName string, additionalChecks func(Gomega, *appsv1.StatefulSet), additionalOffset ...int) *appsv1.StatefulSet {
	statefulSet := &appsv1.StatefulSet{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, statefulSetName), statefulSet)).To(Succeed(), "Expected StatefulSet does not exist")

		testMapContainsOtherWithGomega(g, "StatefulSet pod template selector", statefulSet.Spec.Template.Labels, statefulSet.Spec.Selector.MatchLabels)
		g.Expect(len(statefulSet.Spec.Selector.MatchLabels)).To(BeNumerically(">=", 1), "StatefulSet pod template selector must have at least 1 label")

		if additionalChecks != nil {
			additionalChecks(g, statefulSet)
		}
	}).Should(Succeed())

	By("recreating the StatefulSet after it is deleted")
	ExpectWithOffset(resolveOffset(additionalOffset), k8sClient.Delete(ctx, statefulSet)).To(Succeed())
	EventuallyWithOffset(
		resolveOffset(additionalOffset),
		func() (types.UID, error) {
			newResource := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, resourceKey(parentResource, statefulSetName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(statefulSet.UID))), "New StatefulSet, with new UID, not created.")

	return statefulSet
}

func expectStatefulSetWithConsistentChecks(ctx context.Context, parentResource client.Object, statefulSetName string, additionalChecks func(Gomega, *appsv1.StatefulSet), additionalOffset ...int) *appsv1.StatefulSet {
	statefulSet := &appsv1.StatefulSet{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, statefulSetName), statefulSet)).To(Succeed(), "Expected StatefulSet does not exist")

		testMapContainsOtherWithGomega(g, "StatefulSet pod template selector", statefulSet.Spec.Template.Labels, statefulSet.Spec.Selector.MatchLabels)
		g.Expect(len(statefulSet.Spec.Selector.MatchLabels)).To(BeNumerically(">=", 1), "StatefulSet pod template selector must have at least 1 label")

		if additionalChecks != nil {
			additionalChecks(g, statefulSet)
		}
	}).Should(Succeed())

	return statefulSet
}

func expectNoStatefulSet(ctx context.Context, parentResource client.Object, statefulSetName string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, statefulSetName), &appsv1.StatefulSet{})
	}).Should(MatchError("statefulsets.apps \""+statefulSetName+"\" not found"), "StatefulSet exists when it should not")
}

func expectService(ctx context.Context, parentResource client.Object, serviceName string, selectorLables map[string]string, isHeadless bool, additionalOffset ...int) *corev1.Service {
	return expectServiceWithChecks(ctx, parentResource, serviceName, selectorLables, isHeadless, nil, resolveOffset(additionalOffset))
}

func expectServiceWithChecks(ctx context.Context, parentResource client.Object, serviceName string, selectorLables map[string]string, isHeadless bool, additionalChecks func(Gomega, *corev1.Service), additionalOffset ...int) *corev1.Service {
	service := &corev1.Service{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		Expect(k8sClient.Get(ctx, resourceKey(parentResource, serviceName), service)).To(Succeed(), "Expected Service does not exist")

		g.Expect(service.Spec.Selector).To(Equal(selectorLables), "Service is not pointing to the correct Pods.")

		if isHeadless {
			g.Expect(service.Spec.ClusterIP).To(Equal("None"), "The clusterIP field of a headless service should be None")
		} else {
			g.Expect(service.Spec.ClusterIP).To(Not(Equal("None")), "The clusterIP field of a non-headless service should not be None")
		}

		if additionalChecks != nil {
			additionalChecks(g, service)
		}
	}).Should(Succeed())

	By("recreating the Service after it is deleted")
	ExpectWithOffset(resolveOffset(additionalOffset), k8sClient.Delete(ctx, service)).To(Succeed())
	EventuallyWithOffset(
		resolveOffset(additionalOffset),
		func() (types.UID, error) {
			newResource := &corev1.Service{}
			err := k8sClient.Get(ctx, resourceKey(parentResource, serviceName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(service.UID))), "New Service, with new UID, not created.")

	return service
}

func expectServiceWithConsistentChecks(ctx context.Context, parentResource client.Object, serviceName string, selectorLables map[string]string, isHeadless bool, additionalChecks func(Gomega, *corev1.Service), additionalOffset ...int) *corev1.Service {
	service := &corev1.Service{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		Expect(k8sClient.Get(ctx, resourceKey(parentResource, serviceName), service)).To(Succeed(), "Expected Service does not exist")

		g.Expect(service.Spec.Selector).To(Equal(selectorLables), "Service is not pointing to the correct Pods.")

		if isHeadless {
			g.Expect(service.Spec.ClusterIP).To(Equal("None"), "The clusterIP field of a headless service should be None")
		} else {
			g.Expect(service.Spec.ClusterIP).To(Not(Equal("None")), "The clusterIP field of a non-headless service should not be None")
		}

		if additionalChecks != nil {
			additionalChecks(g, service)
		}
	}).Should(Succeed())

	return service
}

func expectNoService(ctx context.Context, parentResource client.Object, serviceName string, message string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, serviceName), &corev1.Service{})
	}).Should(MatchError("services \""+serviceName+"\" not found"), message, "Service exists when it should not")
}

func expectNoServices(ctx context.Context, parentResource client.Object, message string, serviceNames []string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		for _, serviceName := range serviceNames {
			g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, serviceName), &corev1.Service{})).To(MatchError("services \""+serviceName+"\" not found"), message)
		}
	}).Should(Succeed())
}

func expectIngress(ctx context.Context, parentResource client.Object, ingressName string, additionalOffset ...int) *netv1.Ingress {
	return expectIngressWithChecks(ctx, parentResource, ingressName, nil, resolveOffset(additionalOffset))
}

func expectIngressWithChecks(ctx context.Context, parentResource client.Object, ingressName string, additionalChecks func(Gomega, *netv1.Ingress), additionalOffset ...int) *netv1.Ingress {
	ingress := &netv1.Ingress{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, ingressName), ingress)).To(Succeed(), "Expected Ingress does not exist")

		if additionalChecks != nil {
			additionalChecks(g, ingress)
		}
	}).Should(Succeed())

	By("recreating the Ingress after it is deleted")
	ExpectWithOffset(resolveOffset(additionalOffset), k8sClient.Delete(ctx, ingress)).To(Succeed())
	EventuallyWithOffset(
		resolveOffset(additionalOffset),
		func() (types.UID, error) {
			newResource := &netv1.Ingress{}
			err := k8sClient.Get(ctx, resourceKey(parentResource, ingressName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(ingress.UID))), "New Ingress, with new UID, not created.")

	return ingress
}

func expectIngressWithConsistentChecks(ctx context.Context, parentResource client.Object, ingressName string, additionalChecks func(Gomega, *netv1.Ingress), additionalOffset ...int) *netv1.Ingress {
	ingress := &netv1.Ingress{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, ingressName), ingress)).To(Succeed(), "Expected Ingress does not exist")

		if additionalChecks != nil {
			additionalChecks(g, ingress)
		}
	}).Should(Succeed())

	return ingress
}

func expectNoIngress(ctx context.Context, parentResource client.Object, ingressName string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, ingressName), &netv1.Ingress{})
	}).Should(MatchError("ingresses.networking.k8s.io \""+ingressName+"\" not found"), "Ingress exists when it should not")
}

func expectConfigMap(ctx context.Context, parentResource client.Object, configMapName string, configMapData map[string]string, additionalOffset ...int) *corev1.ConfigMap {
	return expectConfigMapWithChecks(ctx, parentResource, configMapName, configMapData, nil, resolveOffset(additionalOffset))
}

func expectConfigMapWithChecks(ctx context.Context, parentResource client.Object, configMapName string, configMapData map[string]string, additionalChecks func(Gomega, *corev1.ConfigMap), additionalOffset ...int) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, configMapName), configMap)).To(Succeed(), "Expected ConfigMap does not exist")

		// Verify the ConfigMap Data
		g.Expect(configMap.Data).To(Equal(configMapData), "ConfigMap does not have the correct data.")

		if additionalChecks != nil {
			additionalChecks(g, configMap)
		}
	}).Should(Succeed())

	By("recreating the ConfigMap after it is deleted")
	ExpectWithOffset(resolveOffset(additionalOffset), k8sClient.Delete(ctx, configMap)).To(Succeed())
	EventuallyWithOffset(
		resolveOffset(additionalOffset),
		func() (types.UID, error) {
			newResource := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, resourceKey(parentResource, configMapName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(configMap.UID))), "New ConfigMap, with new UID, not created.")

	return configMap
}

func expectConfigMapWithConsistentChecks(ctx context.Context, parentResource client.Object, configMapName string, configMapData map[string]string, additionalChecks func(Gomega, *corev1.ConfigMap), additionalOffset ...int) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, configMapName), configMap)).To(Succeed(), "Expected ConfigMap does not exist")

		// Verify the ConfigMap Data
		g.Expect(configMap.Data).To(Equal(configMapData), "ConfigMap does not have the correct data.")

		if additionalChecks != nil {
			additionalChecks(g, configMap)
		}
	}).Should(Succeed())

	return configMap
}

func expectNoConfigMap(ctx context.Context, parentResource client.Object, configMapName string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, configMapName), &corev1.ConfigMap{})
	}).Should(MatchError("configmaps \""+configMapName+"\" not found"), "ConfigMap exists when it should not")
}

func expectDeployment(ctx context.Context, parentResource client.Object, deploymentName string, additionalOffset ...int) *appsv1.Deployment {
	return expectDeploymentWithChecks(ctx, parentResource, deploymentName, nil, resolveOffset(additionalOffset))
}

func expectDeploymentWithChecks(ctx context.Context, parentResource client.Object, deploymentName string, additionalChecks func(Gomega, *appsv1.Deployment), additionalOffset ...int) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, deploymentName), deployment)).To(Succeed(), "Expected Deployment does not exist")

		// Verify the Deployment Specs
		testMapContainsOtherWithGomega(g, "Deployment pod template selector", deployment.Spec.Template.Labels, deployment.Spec.Selector.MatchLabels)
		g.Expect(len(deployment.Spec.Selector.MatchLabels)).To(BeNumerically(">=", 1), "Deployment pod template selector must have at least 1 label")

		if additionalChecks != nil {
			additionalChecks(g, deployment)
		}
	}).Should(Succeed())

	By("recreating the Deployment after it is deleted")
	ExpectWithOffset(resolveOffset(additionalOffset), k8sClient.Delete(ctx, deployment)).To(Succeed())
	EventuallyWithOffset(
		resolveOffset(additionalOffset),
		func() (types.UID, error) {
			newResource := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, resourceKey(parentResource, deploymentName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(deployment.UID))), "New Deployment, with new UID, not created.")

	return deployment
}

func expectDeploymentWithConsistentChecks(ctx context.Context, parentResource client.Object, deploymentName string, additionalChecks func(Gomega, *appsv1.Deployment), additionalOffset ...int) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, deploymentName), deployment)).To(Succeed(), "Expected Deployment does not exist")

		// Verify the Deployment Specs
		testMapContainsOtherWithGomega(g, "Deployment pod template selector", deployment.Spec.Template.Labels, deployment.Spec.Selector.MatchLabels)
		g.Expect(len(deployment.Spec.Selector.MatchLabels)).To(BeNumerically(">=", 1), "Deployment pod template selector must have at least 1 label")

		if additionalChecks != nil {
			additionalChecks(g, deployment)
		}
	}).Should(Succeed())

	return deployment
}

func expectNoDeployment(ctx context.Context, parentResource client.Object, deploymentName string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, deploymentName), &appsv1.Deployment{})
	}).Should(MatchError("deployments.apps \""+deploymentName+"\" not found"), "Deployment exists when it should not")
}

func createBasicAuthSecret(name string, key string, ns string) *corev1.Secret {
	secretData := map[string][]byte{corev1.BasicAuthUsernameKey: []byte(key), corev1.BasicAuthPasswordKey: []byte("secret password")}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: secretData, Type: corev1.SecretTypeBasicAuth}
}

func expectInitdbVolumeMount(podTemplate *corev1.PodTemplateSpec, additionalOffset ...int) {
	offset := resolveOffset(additionalOffset)
	ExpectWithOffset(offset, podTemplate.Spec.Volumes).To(Not(BeNil()), "No volumes given for Pod")
	var initdbVol *corev1.Volume = nil
	for _, vol := range podTemplate.Spec.Volumes {
		if vol.Name == "initdb" {
			initdbVol = &vol
			break
		}
	}
	ExpectWithOffset(offset, initdbVol).To(Not(BeNil()), "initdb volume not found in pod template; volumes: %v", podTemplate.Spec.Volumes)
	ExpectWithOffset(offset, initdbVol.VolumeSource.EmptyDir).To(Not(BeNil()), "initdb volume should be an emptyDir")

	ExpectWithOffset(offset, podTemplate.Spec.Containers).To(Not(BeEmpty()), "No containers specified for Pod")
	mainContainer := podTemplate.Spec.Containers[0]
	var initdbMount *corev1.VolumeMount = nil
	for _, m := range mainContainer.VolumeMounts {
		if m.Name == "initdb" {
			initdbMount = &m
			break
		}
	}
	ExpectWithOffset(offset, initdbMount).To(Not(BeNil()), "No volume mount found for initdb")
	ExpectWithOffset(offset, initdbMount.MountPath).To(Equal(util.InitdbPath), "Incorrect Mount Path")
}

func expectInitContainer(podTemplate *corev1.PodTemplateSpec, expName string, expVolMountName string, expVolMountPath string, additionalOffset ...int) *corev1.Container {
	offset := resolveOffset(additionalOffset)
	var expInitContainer *corev1.Container = nil
	for _, cnt := range podTemplate.Spec.InitContainers {
		if cnt.Name == expName {
			expInitContainer = &cnt
			break
		}
	}
	ExpectWithOffset(offset, expInitContainer).To(Not(BeNil()), "Didn't find the %s InitContainer", expName)
	ExpectWithOffset(offset, expInitContainer.Command).To(HaveLen(3), "Wrong command length for %s init container", expName)

	var volMount *corev1.VolumeMount = nil
	for _, m := range expInitContainer.VolumeMounts {
		if m.Name == expVolMountName {
			volMount = &m
			break
		}
	}
	ExpectWithOffset(offset, volMount).To(Not(BeNil()), "No %s volumeMount for the %s InitContainer", expVolMountName, expName)
	ExpectWithOffset(offset, volMount.MountPath).To(Equal(expVolMountPath), "Wrong mount path for the %s InitContainer", expName)

	return expInitContainer
}

// filter env vars by name using a supplied match function
func filterVarsByName(envVars []corev1.EnvVar, f func(string) bool) []corev1.EnvVar {
	filtered := make([]corev1.EnvVar, 0)
	for _, v := range envVars {
		if f(v.Name) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func testPodEnvVariables(expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar, additionalOffset ...int) {
	testPodEnvVariablesWithGomega(Default, expectedEnvVars, foundEnvVars, resolveOffset(additionalOffset))
}

func testPodEnvVariablesWithGomega(g Gomega, expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar, additionalOffset ...int) {
	envVarRegex := regexp.MustCompile(`\$\([a-zA-Z0-9_]+\)`)
	offset := resolveOffset(additionalOffset)
	matchCount := 0
	var processedEnvVarNames = make([]string, len(foundEnvVars))
	for i, envVar := range foundEnvVars {
		if expectedVal, match := expectedEnvVars[envVar.Name]; match {
			matchCount += 1
			g.ExpectWithOffset(offset, envVar.Value).To(Equal(expectedVal), "Wrong value for env variable '%s' in podSpec", envVar.Name)
		}

		// Check that the current envVar only references other env-vars that have already been defined
		envVarsReferencedByCurrent := envVarRegex.FindAllString(envVar.Value, -1)
		for _, referencedVar := range envVarsReferencedByCurrent {
			referencedVarTrimmed := referencedVar[2 : len(referencedVar)-1] // "$(ENV_VAR_NAME)" -> "ENV_VAR_NAME"
			g.Expect(processedEnvVarNames).To(ContainElement(referencedVarTrimmed),
				"Env-var %s with value [%s] must be defined after the env-var it depends on: %s",
				envVar.Name, envVar.Value, referencedVarTrimmed)
		}

		processedEnvVarNames[i] = envVar.Name
	}
	g.ExpectWithOffset(offset, matchCount).To(Equal(len(expectedEnvVars)), "Not all expected env variables found in podSpec")
}

func testMapContainsOther(mapName string, base map[string]string, other map[string]string, additionalOffset ...int) {
	testMapContainsOtherWithGomega(Default, mapName, base, other, resolveOffset(additionalOffset))
}

func testMapContainsOtherWithGomega(g Gomega, mapName string, base map[string]string, other map[string]string, additionalOffset ...int) {
	offset := resolveOffset(additionalOffset)
	for k, v := range other {
		g.ExpectWithOffset(offset, base).To(HaveKeyWithValue(k, v), "Expected key '%s' is not correct in found %s", k, mapName)
	}
}

func insertExpectedAclEnvVars(dest map[string]string, hasReadOnly bool) {
	expectedEnvVars := getExpectedAclEnvVars(hasReadOnly)
	for _, expectedEnvVar := range expectedEnvVars {
		dest[expectedEnvVar.Name] = expectedEnvVar.Value
	}
}

func getExpectedAclEnvVars(hasReadOnly bool) []corev1.EnvVar {
	/*
		Populates ACL related env vars are set correctly and in the correct order, assuming a very specific test SolrCloud config:
		set hasReadOnly = false if ReadOnlyACL is not provided
					AllACL: &solrv1beta1.ZookeeperACL{
						SecretRef:   "secret-name",
						UsernameKey: "user",
						PasswordKey: "pass",
					},
					ReadOnlyACL: &solrv1beta1.ZookeeperACL{
						SecretRef:   "read-secret-name",
						UsernameKey: "read-only-user",
						PasswordKey: "read-only-pass",
					},

	*/
	f := false
	zkAclEnvVars := []corev1.EnvVar{
		{
			Name: "ZK_ALL_ACL_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-name"},
					Key:                  "user",
					Optional:             &f,
				},
			},
		},
		{
			Name: "ZK_ALL_ACL_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-name"},
					Key:                  "pass",
					Optional:             &f,
				},
			},
		},
	}
	if hasReadOnly {
		zkAclEnvVars = append(zkAclEnvVars,
			corev1.EnvVar{
				Name: "ZK_READ_ACL_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "read-secret-name"},
						Key:                  "read-only-user",
						Optional:             &f,
					},
				},
			},
			corev1.EnvVar{
				Name: "ZK_READ_ACL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "read-secret-name"},
						Key:                  "read-only-pass",
						Optional:             &f,
					},
				},
			},
			corev1.EnvVar{
				Name:      "SOLR_ZK_CREDS_AND_ACLS",
				Value:     "-DzkACLProvider=org.apache.solr.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solr.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider -DzkDigestUsername=$(ZK_ALL_ACL_USERNAME) -DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD) -DzkDigestReadonlyUsername=$(ZK_READ_ACL_USERNAME) -DzkDigestReadonlyPassword=$(ZK_READ_ACL_PASSWORD)",
				ValueFrom: nil,
			})
	} else {
		zkAclEnvVars = append(zkAclEnvVars,
			corev1.EnvVar{
				Name:      "SOLR_ZK_CREDS_AND_ACLS",
				Value:     "-DzkACLProvider=org.apache.solr.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solr.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider -DzkDigestUsername=$(ZK_ALL_ACL_USERNAME) -DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD)",
				ValueFrom: nil,
			})
	}
	return zkAclEnvVars
}

func testACLEnvVarsWithGomega(g Gomega, actualEnvVars []corev1.EnvVar, hasReadOnly bool, additionalOffset ...int) {
	zkAclEnvVars := getExpectedAclEnvVars(hasReadOnly)
	g.ExpectWithOffset(resolveOffset(additionalOffset), actualEnvVars).To(Equal(zkAclEnvVars), "ZK ACL Env Vars are not correct")
}

func cleanupTest(ctx context.Context, parentResource client.Object) {
	cleanupObjects := []client.Object{
		// Solr Operator CRDs, modify this list whenever CRDs are added/deleted
		&solrv1beta1.SolrCloud{}, &solrv1beta1.SolrBackup{}, &solrv1beta1.SolrPrometheusExporter{},
		&zk_api.ZookeeperCluster{},

		// All dependent Kubernetes types, in order of dependence (deployment then replicaSet then pod)
		&corev1.ConfigMap{}, &netv1.Ingress{},
		&corev1.PersistentVolumeClaim{}, &corev1.PersistentVolume{},
		&appsv1.StatefulSet{}, &appsv1.Deployment{}, &appsv1.ReplicaSet{}, &corev1.Pod{}, &corev1.PersistentVolumeClaim{},
		&corev1.Secret{},
	}
	By("deleting all managed resources")
	for _, obj := range cleanupObjects {
		Expect(k8sClient.DeleteAllOf(ctx, obj, client.InNamespace(parentResource.GetNamespace()))).To(Succeed())
	}

	By("deleting all services individually")
	// Clean up Services individually, since they do not support delete collection
	serviceList := &corev1.ServiceList{}
	Expect(k8sClient.List(ctx, serviceList, client.InNamespace(parentResource.GetNamespace()))).To(Succeed(), "List all of the services to delete in the namespace")
	for _, item := range serviceList.Items {
		Expect(k8sClient.Delete(ctx, &item)).To(Succeed())
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
	testProbeLivenessNonDefaults = &corev1.Probe{
		InitialDelaySeconds: 20,
		TimeoutSeconds:      1,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		PeriodSeconds:       10,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Scheme: corev1.URISchemeHTTP,
				Path:   "/solr/admin/info/system",
				Port:   intstr.FromInt(8983),
			},
		},
	}
	testProbeReadinessNonDefaults = &corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		PeriodSeconds:       5,
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(8983),
			},
		},
	}
	testProbeStartup = &corev1.Probe{
		InitialDelaySeconds: 1,
		TimeoutSeconds:      1,
		SuccessThreshold:    1,
		FailureThreshold:    5,
		PeriodSeconds:       5,
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"ls",
				},
			},
		},
	}
	testLifecycle = &corev1.Lifecycle{
		PostStart: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "echo Hello from the postStart handler"},
			},
		},
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "echo Hello from the preStop handler"},
			},
		},
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
	testPriorityClass              = "p4"
	testImagePullSecretName        = "MAIN_SECRET"
	testImagePullSecretName2       = "ANOTHER_SECRET"
	testAdditionalImagePullSecrets = []corev1.LocalObjectReference{
		{Name: "ADDITIONAL_SECRET_1"},
		{Name: "ADDITIONAL_SECRET_2"},
	}
	testTerminationGracePeriodSeconds = int64(50)
	extraVars                         = []corev1.EnvVar{
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
	one                    = int64(1)
	two                    = int64(2)
	four                   = int32(4)
	five                   = int32(5)
	testPodSecurityContext = corev1.PodSecurityContext{
		RunAsUser:  &one,
		RunAsGroup: &two,
	}
	extraVolumes = []solrv1beta1.AdditionalVolume{
		{
			Name: "vol1",
			Source: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
			DefaultContainerMount: &corev1.VolumeMount{
				Name:      "ignore",
				ReadOnly:  false,
				MountPath: "/test/mount/path",
				SubPath:   "sub/",
			},
		},
	}
	testAffinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					TopologyKey: "testKey",
				},
			},
			PreferredDuringSchedulingIgnoredDuringExecution: nil,
		},
	}
	testResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: resource.MustParse("5300m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceEphemeralStorage: resource.MustParse("5Gi"),
		},
	}
	testResources2 = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: resource.MustParse("400m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceEphemeralStorage: resource.MustParse("3Gi"),
		},
	}
	extraContainers1 = []corev1.Container{
		{
			Name:                     "container1",
			Image:                    "image1",
			TerminationMessagePolicy: "File",
			TerminationMessagePath:   "/dev/termination-log",
			ImagePullPolicy:          "Always",
		},
		{
			Name:                     "container2",
			Image:                    "image2",
			TerminationMessagePolicy: "File",
			TerminationMessagePath:   "/dev/termination-log",
			ImagePullPolicy:          "Always",
		},
	}
	extraContainers2 = []corev1.Container{
		{
			Name:                     "container3",
			Image:                    "image3",
			TerminationMessagePolicy: "File",
			TerminationMessagePath:   "/dev/termination-log",
			ImagePullPolicy:          "Always",
		},
		{
			Name:                     "container4",
			Image:                    "image4",
			TerminationMessagePolicy: "File",
			TerminationMessagePath:   "/dev/termination-log",
			ImagePullPolicy:          "Always",
		},
	}
	testServiceAccountName = "test-service-account"
	zkConf                 = solrv1beta1.ZookeeperConfig{
		InitLimit:            1,
		SyncLimit:            5,
		PreAllocSize:         2,
		CommitLogCount:       10,
		MaxCnxns:             4,
		MinSessionTimeout:    6,
		QuorumListenOnAllIPs: true,
	}
	testTopologySpreadConstraints = []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           3,
			TopologyKey:       "zone",
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "label"},
			},
		},
		{
			MaxSkew:           3,
			TopologyKey:       "region",
			WhenUnsatisfiable: corev1.ScheduleAnyway,
		},
	}
	testIngressClass = "test-ingress-class"
	testSolrZKOpts   = "-Dsolr.zk.opts=this"
	testSolrOpts     = "-Dsolr.opts=this"
)
