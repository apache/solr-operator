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
	corev1 "k8s.io/api/core/v1"
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
					Annotations:       testPodAnnotations,
					Labels:            testPodLabels,
					Volumes:           extraVolumes,
					Tolerations:       testTolerationsPromExporter,
					NodeSelector:      testNodeSelectors,
					PriorityClassName: testPriorityClass,
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
	assert.EqualValues(t, testPriorityClass, deployment.Spec.Template.Spec.PriorityClassName, "Incorrect Priority class name for Pod Spec")

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

func TestMetricsReconcileWithGivenZkAcls(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace},
		Spec: solr.SolrPrometheusExporterSpec{
			Config: testExporterConfig,
			SolrReference: solr.SolrReference{
				Cloud: &solr.SolrCloudReference{
					ZookeeperConnectionInfo: &solr.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
						AllACL: &solr.ZookeeperACL{
							SecretRef:   "secret-name",
							UsernameKey: "user",
							PasswordKey: "pass",
						},
						ReadOnlyACL: &solr.ZookeeperACL{
							SecretRef:   "read-secret-name",
							UsernameKey: "read-only-user",
							PasswordKey: "read-only-pass",
						},
					},
				},
			},
			CustomKubeOptions: solr.CustomExporterKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables: extraVars,
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

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"JAVA_OPTS": "$(SOLR_ZK_CREDS_AND_ACLS)",
	}
	foundEnv := deployment.Spec.Template.Spec.Containers[0].Env
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
			Value:     "-DzkACLProvider=org.apache.solr.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solr.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider -DzkDigestUsername=$(ZK_ALL_ACL_USERNAME) -DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD) -DzkDigestReadonlyUsername=$(ZK_READ_ACL_USERNAME) -DzkDigestReadonlyPassword=$(ZK_READ_ACL_PASSWORD)",
			ValueFrom: nil,
		},
	}
	assert.Equal(t, zkAclEnvVars, foundEnv[0:5], "ZK ACL Env Vars are not correct")
	assert.Equal(t, extraVars, foundEnv[5:len(foundEnv)-1], "Extra Env Vars are not the same as the ones provided in podOptions")
	// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
	// TODO: Make this not invalidate foundEnv
	testMetricsPodEnvVariables(t, expectedEnvVars, foundEnv[len(foundEnv)-1:])

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
}

func TestMetricsReconcileWithSolrZkAcls(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrPrometheusExporter{
		ObjectMeta: metav1.ObjectMeta{Name: expectedMetricsRequest.Name, Namespace: expectedMetricsRequest.Namespace},
		Spec: solr.SolrPrometheusExporterSpec{
			Config: testExporterConfig,
			SolrReference: solr.SolrReference{
				Cloud: &solr.SolrCloudReference{
					Name: expectedCloudRequest.Name,
				},
			},
			CustomKubeOptions: solr.CustomExporterKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables: extraVars,
				},
			},
		},
	}

	solrRef := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
					AllACL: &solr.ZookeeperACL{
						SecretRef:   "secret-name",
						UsernameKey: "user",
						PasswordKey: "pass",
					},
					ReadOnlyACL: &solr.ZookeeperACL{
						SecretRef:   "read-secret-name",
						UsernameKey: "read-only-user",
						PasswordKey: "read-only-pass",
					},
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

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	cloudRec, cloudRequests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, cloudRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object that the PrometheusExporter will reference
	err = testClient.Create(context.TODO(), solrRef)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	defer testClient.Delete(context.TODO(), solrRef)
	g.Eventually(cloudRequests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the services will have IP addresses for the hostAliases to use
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(cloudRequests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

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
	testMapsEqual(t, "configMap labels", instance.SharedLabelsWith(instance.Labels), configMap.Labels)

	deployment := expectDeployment(t, g, requests, expectedMetricsRequest, metricsDKey, configMap.Name)
	expectedDeploymentLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"technology": solr.SolrPrometheusExporterTechnologyLabel})
	testMapsEqual(t, "deployment labels", expectedDeploymentLabels, deployment.Labels)
	testMapsEqual(t, "pod labels", expectedDeploymentLabels, deployment.Spec.Template.ObjectMeta.Labels)

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"JAVA_OPTS": "$(SOLR_ZK_CREDS_AND_ACLS)",
	}
	foundEnv := deployment.Spec.Template.Spec.Containers[0].Env
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
			Value:     "-DzkACLProvider=org.apache.solr.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solr.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider -DzkDigestUsername=$(ZK_ALL_ACL_USERNAME) -DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD) -DzkDigestReadonlyUsername=$(ZK_READ_ACL_USERNAME) -DzkDigestReadonlyPassword=$(ZK_READ_ACL_PASSWORD)",
			ValueFrom: nil,
		},
	}
	assert.Equal(t, 8, len(foundEnv), "Incorrect number of Env Vars found")
	assert.Equal(t, zkAclEnvVars, foundEnv[0:5], "ZK ACL Env Vars are not correct")
	assert.Equal(t, extraVars, foundEnv[5:len(foundEnv)-1], "Extra Env Vars are not the same as the ones provided in podOptions")
	testMetricsPodEnvVariables(t, expectedEnvVars, foundEnv[len(foundEnv)-1:])

	expectedServiceLabels := util.MergeLabelsOrAnnotations(instance.SharedLabelsWith(instance.Labels), map[string]string{"service-type": "metrics"})
	expectedServiceAnnotations := map[string]string{"prometheus.io/path": "/metrics", "prometheus.io/port": "80", "prometheus.io/scheme": "http", "prometheus.io/scrape": "true"}
	service := expectService(t, g, requests, expectedMetricsRequest, metricsSKey, expectedDeploymentLabels)
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"], "Metrics Service Prometheus scraping is not enabled.")
	testMapsEqual(t, "service labels", expectedServiceLabels, service.Labels)
	testMapsEqual(t, "service annotations", expectedServiceAnnotations, service.Annotations)
}
