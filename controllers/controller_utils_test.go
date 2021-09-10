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
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

const timeout = 5

func emptyRequests(requests chan reconcile.Request) {
	for len(requests) > 0 {
		<-requests
	}
}

func resourceKey(solrCloud *solrv1beta1.SolrCloud, name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: solrCloud.Namespace}
}

func expectStatus(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) *solrv1beta1.SolrCloudStatus {
	return expectStatusWithChecks(ctx, solrCloud, nil)
}

func expectStatusWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, additionalChecks func(Gomega, *solrv1beta1.SolrCloudStatus)) *solrv1beta1.SolrCloudStatus {
	foundSolrCloud := &solrv1beta1.SolrCloud{}
	Eventually(func(g Gomega) error {
		err := k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), foundSolrCloud)
		if err == nil && additionalChecks != nil {
			additionalChecks(g, &foundSolrCloud.Status)
		}
		return err
	}).Should(Succeed())

	return &foundSolrCloud.Status
}

func expectStatefulSet(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, statefulSetName string) *appsv1.StatefulSet {
	return expectStatefulSetWithChecks(ctx, solrCloud, statefulSetName, nil)
}

func expectStatefulSetWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, statefulSetName string, additionalChecks func(Gomega, *appsv1.StatefulSet)) *appsv1.StatefulSet {
	statefulSet := &appsv1.StatefulSet{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, statefulSetName), statefulSet)).To(Succeed())

		testMapContainsOtherWithGomega(g, "StatefulSet pod template selector", statefulSet.Spec.Template.Labels, statefulSet.Spec.Selector.MatchLabels)
		g.Expect(len(statefulSet.Spec.Selector.MatchLabels)).To(BeNumerically(">=", 1), "StatefulSet pod template selector must have at least 1 label")

		if additionalChecks != nil {
			additionalChecks(g, statefulSet)
		}
	}).Should(Succeed())


	By("recreating the StatefulSet after it is deleted")
	Expect(k8sClient.Delete(ctx, statefulSet)).To(Succeed())
	Eventually(
		func() (types.UID, error) {
			newResource := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, resourceKey(solrCloud, statefulSetName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(statefulSet.UID))), "New StatefulSet, with new UID, not created.")

	return statefulSet
}

func expectNoStatefulSet(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, statefulSetName string) {
	Consistently(func() error {
		return k8sClient.Get(ctx, resourceKey(solrCloud, statefulSetName), &appsv1.StatefulSet{})
	}).Should(MatchError("statefulsets.apps \"" + statefulSetName + "\" not found"))
}

func expectService(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, serviceName string, selectorLables map[string]string, isHeadless bool) *corev1.Service {
	return expectServiceWithChecks(ctx, solrCloud, serviceName, selectorLables, isHeadless, nil)
}

func expectServiceWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, serviceName string, selectorLables map[string]string, isHeadless bool, additionalChecks func(Gomega, *corev1.Service)) *corev1.Service {
	service := &corev1.Service{}
	Eventually(func(g Gomega) {
		Expect(k8sClient.Get(ctx, resourceKey(solrCloud, serviceName), service)).To(Succeed())

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
	Expect(k8sClient.Delete(ctx, service)).To(Succeed())
	Eventually(
		func() (types.UID, error) {
			newResource := &corev1.Service{}
			err := k8sClient.Get(ctx, resourceKey(solrCloud, serviceName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(service.UID))), "New Service, with new UID, not created.")

	return service
}

func expectNoService(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, serviceName string, message string) {
	Consistently(func() error {
		return k8sClient.Get(ctx, resourceKey(solrCloud, serviceName), &corev1.Service{})
	}).Should(MatchError("services \""+serviceName+"\" not found"), message)
}

func expectIngress(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, ingressName string) *netv1.Ingress {
	return expectIngressWithChecks(ctx, solrCloud, ingressName, nil)
}

func expectIngressWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, ingressName string, additionalChecks func(Gomega, *netv1.Ingress)) *netv1.Ingress {
	ingress := &netv1.Ingress{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, ingressName), ingress)).To(Succeed())

		if additionalChecks != nil {
			additionalChecks(g, ingress)
		}
	}).Should(Succeed())

	By("recreating the Ingress after it is deleted")
	Expect(k8sClient.Delete(ctx, ingress)).To(Succeed())
	Eventually(
		func() (types.UID, error) {
			newResource := &netv1.Ingress{}
			err := k8sClient.Get(ctx, resourceKey(solrCloud, ingressName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(ingress.UID))), "New Ingress, with new UID, not created.")

	return ingress
}

func expectNoIngress(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, ingressName string) {
	Consistently(func() error {
		return k8sClient.Get(ctx, resourceKey(solrCloud, ingressName), &netv1.Ingress{})
	}).Should(MatchError("ingresses.networking.k8s.io \"" + ingressName + "\" not found"))
}

func expectConfigMap(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, configMapName string, configMapData map[string]string) *corev1.ConfigMap {
	return expectConfigMapWithChecks(ctx, solrCloud, configMapName, configMapData, nil)
}

func expectConfigMapWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, configMapName string, configMapData map[string]string, additionalChecks func(Gomega, *corev1.ConfigMap)) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, configMapName), configMap)).To(Succeed())

		// Verify the ConfigMap Data
		g.Expect(configMap.Data).To(Equal(configMapData), "ConfigMap does not have the correct data.")

		if additionalChecks != nil {
			additionalChecks(g, configMap)
		}
	}).Should(Succeed())

	By("recreating the ConfigMap after it is deleted")
	Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
	Eventually(
		func() (types.UID, error) {
			newResource := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, resourceKey(solrCloud, configMapName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(configMap.UID))), "New ConfigMap, with new UID, not created.")

	return configMap
}

func expectNoConfigMap(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, configMapName string) {
	Consistently(func() error {
		return k8sClient.Get(ctx, resourceKey(solrCloud, configMapName), &corev1.ConfigMap{})
	}).Should(MatchError("configmaps \"" + configMapName + "\" not found"))
}

func expectDeployment(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, deploymentName string) *appsv1.Deployment {
	return expectDeploymentWithChecks(ctx, solrCloud, deploymentName, nil)
}

func expectDeploymentWithChecks(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, deploymentName string, additionalChecks func(Gomega, *appsv1.Deployment)) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(solrCloud, deploymentName), deployment)).To(Succeed())

		// Verify the Deployment Specs
		testMapContainsOtherWithGomega(g, "Deployment pod template selector", deployment.Spec.Template.Labels, deployment.Spec.Selector.MatchLabels)
		g.Expect(len(deployment.Spec.Selector.MatchLabels)).To(BeNumerically(">=", 1), "Deployment pod template selector must have at least 1 label")

		if additionalChecks != nil {
			additionalChecks(g, deployment)
		}
	}).Should(Succeed())

	By("recreating the Deployment after it is deleted")
	Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
	Eventually(
		func() (types.UID, error) {
			newResource := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, resourceKey(solrCloud, deploymentName), newResource)
			if err != nil {
				return "", err
			}
			return newResource.UID, nil
		}).Should(And(Not(BeEmpty()), Not(Equal(deployment.UID))), "New Deployment, with new UID, not created.")

	return deployment
}

func createBasicAuthSecret(name string, key string, ns string) *corev1.Secret {
	secretData := map[string][]byte{corev1.BasicAuthUsernameKey: []byte(key), corev1.BasicAuthPasswordKey: []byte("secret password")}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: secretData, Type: corev1.SecretTypeBasicAuth}
}

func expectInitdbVolumeMount(podTemplate *corev1.PodTemplateSpec) {
	Expect(podTemplate.Spec.Volumes).To(Not(BeNil()), "No volumes given for Pod")
	var initdbVol *corev1.Volume = nil
	for _, vol := range podTemplate.Spec.Volumes {
		if vol.Name == "initdb" {
			initdbVol = &vol
			break
		}
	}
	Expect(initdbVol).To(Not(BeNil()), "initdb volume not found in pod template; volumes: %v", podTemplate.Spec.Volumes)
	Expect(initdbVol.VolumeSource.EmptyDir).To(Not(BeNil()), "initdb volume should be an emptyDir")

	Expect(podTemplate.Spec.Containers).To(Not(BeNil()), "No containers specified for Pod")
	Expect(len(podTemplate.Spec.Containers)).To(BeNumerically(">", 0), "Pod must have at least 1 container")
	mainContainer := podTemplate.Spec.Containers[0]
	var initdbMount *corev1.VolumeMount = nil
	for _, m := range mainContainer.VolumeMounts {
		if m.Name == "initdb" {
			initdbMount = &m
			break
		}
	}
	Expect(initdbMount).To(Not(BeNil()), "No volume mount found for initdb")
	Expect(initdbMount.MountPath).To(Equal(util.InitdbPath), "Incorrect Mount Path")
}

func expectInitContainer(podTemplate *corev1.PodTemplateSpec, expName string, expVolMountName string, expVolMountPath string) *corev1.Container {
	var expInitContainer *corev1.Container = nil
	for _, cnt := range podTemplate.Spec.InitContainers {
		if cnt.Name == expName {
			expInitContainer = &cnt
			break
		}
	}
	Expect(expInitContainer).To(Not(BeNil()), "Didn't find the %s InitContainer", expName)
	Expect(len(expInitContainer.Command)).To(Equal(3), "Wrong command length for %s init container", expName)

	var volMount *corev1.VolumeMount = nil
	for _, m := range expInitContainer.VolumeMounts {
		if m.Name == expVolMountName {
			volMount = &m
			break
		}
	}
	Expect(volMount).To(Not(BeNil()), "No %s volumeMount for the %s InitContainer", expVolMountName, expName)
	Expect(volMount.MountPath).To(Equal(expVolMountPath), "Wrong mount path for the %s InitContainer", expName)

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

func testPodEnvVariables(expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar) {
	testGenericPodEnvVariables(expectedEnvVars, foundEnvVars, "SOLR_OPTS")
}

func testPodEnvVariablesWithGomega(g Gomega, expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar) {
	testGenericPodEnvVariablesWithGomega(g, expectedEnvVars, foundEnvVars, "SOLR_OPTS")
}

func testMetricsPodEnvVariables(expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar) {
	testGenericPodEnvVariables(expectedEnvVars, foundEnvVars, "JAVA_OPTS")
}

func testMetricsPodEnvVariablesWithGomega(g Gomega, expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar) {
	testGenericPodEnvVariablesWithGomega(g, expectedEnvVars, foundEnvVars, "JAVA_OPTS")
}

func testGenericPodEnvVariables(expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar, lastVarName string) {
	testGenericPodEnvVariablesWithGomega(Default, expectedEnvVars, foundEnvVars, lastVarName)
}

func testGenericPodEnvVariablesWithGomega(g Gomega, expectedEnvVars map[string]string, foundEnvVars []corev1.EnvVar, lastVarName string) {
	matchCount := 0
	for _, envVar := range foundEnvVars {
		if expectedVal, match := expectedEnvVars[envVar.Name]; match {
			matchCount += 1
			g.Expect(envVar.Value).To(Equal(expectedVal), "Wrong value for env variable '%s' in podSpec", envVar.Name)
		}
	}
	g.Expect(matchCount).To(Equal(len(expectedEnvVars)), "Not all expected env variables found in podSpec")
	g.Expect(foundEnvVars[len(foundEnvVars)-1].Name).To(Equal(lastVarName), "%s must be the last envVar set, as it uses other envVars.", lastVarName)
}

func testPodTolerations(expectedTolerations []corev1.Toleration, foundTolerations []corev1.Toleration) {
	Expect(foundTolerations).To(Equal(expectedTolerations), "Expected tolerations and found tolerations don't match")
}

func testPodProbe(expectedProbe *corev1.Probe, foundProbe *corev1.Probe, probeType string) {
	Expect(foundProbe).To(Equal(expectedProbe), "Incorrect default container %s probe", probeType)
}

func testMapsEqual(mapName string, expected map[string]string, found map[string]string) {
	Expect(found).To(Equal(expected), "Expected and found %s are not the same", mapName)
}

func testMapContainsOther(mapName string, base map[string]string, other map[string]string) {
	testMapContainsOtherWithGomega(Default, mapName, base, other)
}

func testMapContainsOtherWithGomega(g Gomega, mapName string, base map[string]string, other map[string]string) {
	for k, v := range other {
		foundV, foundExists := base[k]
		g.Expect(foundExists).To(BeTrue(), "Expected key '%s' does not exist in found %s", k, mapName)
		if foundExists {
			g.Expect(foundV).To(Equal(v), "Wrong value for %s key '%s'", mapName, k)
		}
	}
}

func testACLEnvVars(t *testing.T, actualEnvVars []corev1.EnvVar) {
	/*
		This test verifies ACL related env vars are set correctly and in the correct order, but expects a very specific config to be used in your test SolrCloud config:

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
		{
			Name: "ZK_READ_ACL_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "read-secret-name"},
					Key:                  "read-only-user",
					Optional:             &f,
				},
			},
		},
		{
			Name: "ZK_READ_ACL_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "read-secret-name"},
					Key:                  "read-only-pass",
					Optional:             &f,
				},
			},
		},
		{
			Name:      "SOLR_ZK_CREDS_AND_ACLS",
			Value:     "-DzkACLProvider=org.apache.solrv1beta1.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solrv1beta1.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider -DzkDigestUsername=$(ZK_ALL_ACL_USERNAME) -DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD) -DzkDigestReadonlyUsername=$(ZK_READ_ACL_USERNAME) -DzkDigestReadonlyPassword=$(ZK_READ_ACL_PASSWORD)",
			ValueFrom: nil,
		},
	}
	Expect(actualEnvVars).To(Equal(zkAclEnvVars), "ZK ACL Env Vars are not correct")
}

func cleanupTest(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) {
	By("deleting the SolrCloud")
	Expect(k8sClient.Delete(ctx, solrCloud)).To(Succeed())

	cleanupObjects := []client.Object{
		// Solr Operator CRDs, modify this list whenever CRDs are added/deleted
		&solrv1beta1.SolrCloud{}, &solrv1beta1.SolrBackup{}, &solrv1beta1.SolrPrometheusExporter{},

		// All dependent Kubernetes types, in order of dependence (deployment then replicaSet then pod)
		&corev1.ConfigMap{}, &batchv1.Job{}, &netv1.Ingress{},
		&corev1.PersistentVolumeClaim{}, &corev1.PersistentVolume{},
		&appsv1.StatefulSet{}, &appsv1.Deployment{}, &appsv1.ReplicaSet{}, &corev1.Pod{}, &corev1.PersistentVolumeClaim{},
		&corev1.Secret{},
	}
	By("deleting all managed resources")
	for _, obj := range cleanupObjects {
		Expect(k8sClient.DeleteAllOf(ctx, obj, client.InNamespace(solrCloud.Namespace))).To(Succeed())
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
		Handler: corev1.Handler{
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
		Handler: corev1.Handler{
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
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"ls",
				},
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
	one                = int64(1)
	two                = int64(2)
	four               = int32(4)
	five               = int32(5)
	podSecurityContext = corev1.PodSecurityContext{
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
	/*zkConf                 = zkv1beta1.ZookeeperConfig{
		InitLimit:            1,
		SyncLimit:            5,
		PreAllocSize:         2,
		CommitLogCount:       10,
		MaxCnxns:             4,
		MinSessionTimeout:    6,
		QuorumListenOnAllIPs: true,
	}*/
)
