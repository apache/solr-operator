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
	"context"
	"crypto/md5"
	"fmt"
	"github.com/apache/solr-operator/controllers/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("SolrPrometheusExporter controller - General", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 5
		duration = time.Second * 1
		interval = time.Millisecond * 250
	)
	SetDefaultConsistentlyDuration(duration)
	SetDefaultConsistentlyPollingInterval(interval)
	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	var (
		ctx context.Context

		solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter
		solrRef                *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		ctx = context.Background()

		solrPrometheusExporter = &solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrPrometheusExporterSpec{},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrPrometheusExporter)).To(Succeed())

		By("defaulting the missing SolrCloud values")
		expectSolrPrometheusExporterWithChecks(ctx, solrPrometheusExporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.WithDefaults()).To(BeFalse(), "The SolrPrometheusExporter spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func() {
		cleanupTest(ctx, solrPrometheusExporter)
	})

	FContext("Use explicit ZK Connection Info - Default Config", func() {
		testNumThreads := int32(4)
		testScrapeInterval := int32(10)
		testZkCnxString := "host:2181"
		testZKChroot := "/this/path"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: testZkCnxString,
							ChRoot:                   testZKChroot,
						},
					},
				},
				NumThreads:     testNumThreads,
				ScrapeInterval: testScrapeInterval,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			deployment := expectDeployment(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName())

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1), "Wrong number of containers for the Deployment")
			Expect(deployment.Spec.Template.Spec.InitContainers).To(BeEmpty(), "Wrong number of initContainers for the Deployment")

			expectedArgs := []string{
				"-p", strconv.Itoa(util.SolrMetricsPort),
				"-n", strconv.Itoa(int(testNumThreads)),
				"-s", strconv.Itoa(int(testScrapeInterval)),
				"-z", testZkCnxString + testZKChroot,
				"-f", "/opt/solr/contrib/prometheus-exporter/conf/solr-exporter-config.xml",
			}
			Expect(deployment.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedArgs), "Incorrect arguments for the SolrPrometheusExporter container")
			Expect(deployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{util.DefaultPrometheusExporterEntrypoint}), "Incorrect command for the SolrPrometheusExporter container")
		})
	})

	FContext("All possible customization options - Custom Config", func() {
		testExporterConfig := "This is a test config."
		testNumThreads := int32(4)
		testScrapeInterval := int32(10)
		testZkCnxString := "host:2181"
		testZKChroot := "/this/path"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: testZkCnxString,
							ChRoot:                   testZKChroot,
						},
					},
				},
				Image: &solrv1beta1.ContainerImage{
					ImagePullSecret: testImagePullSecretName,
				},
				NumThreads:     testNumThreads,
				ScrapeInterval: testScrapeInterval,
				CustomKubeOptions: solrv1beta1.CustomExporterKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						Affinity:                      testAffinity,
						Resources:                     testResources,
						Volumes:                       extraVolumes,
						PodSecurityContext:            &testPodSecurityContext,
						EnvVariables:                  extraVars,
						Annotations:                   testPodAnnotations,
						Labels:                        testPodLabels,
						Tolerations:                   testTolerationsPromExporter,
						NodeSelector:                  testNodeSelectors,
						LivenessProbe:                 testProbeLivenessNonDefaults,
						ReadinessProbe:                testProbeReadinessNonDefaults,
						StartupProbe:                  testProbeStartup,
						PriorityClassName:             testPriorityClass,
						SidecarContainers:             extraContainers1,
						InitContainers:                extraContainers2,
						ImagePullSecrets:              testAdditionalImagePullSecrets,
						TerminationGracePeriodSeconds: &testTerminationGracePeriodSeconds,
						ServiceAccountName:            testServiceAccountName,
						Lifecycle:                     testLifecycle,
						TopologySpreadConstraints:     testTopologySpreadConstraints,
						DefaultInitContainerResources: testResources2,
					},
					DeploymentOptions: &solrv1beta1.DeploymentOptions{
						Annotations: testDeploymentAnnotations,
						Labels:      testDeploymentLabels,
					},
					ServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testMetricsServiceAnnotations,
						Labels:      testMetricsServiceLabels,
					},
					ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
						Annotations: testConfigMapAnnotations,
						Labels:      testConfigMapLabels,
					},
				},
				Config: testExporterConfig,
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter ConfigMap")
			configMap := expectConfigMap(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsConfigMapName(), map[string]string{util.PrometheusExporterConfigMapKey: testExporterConfig})
			Expect(configMap.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrPrometheusExporter.SharedLabelsWith(solrPrometheusExporter.Labels), testConfigMapLabels)), "Incorrect configMap labels")
			Expect(configMap.Annotations).To(Equal(testConfigMapAnnotations), "Incorrect configMap annotations")

			By("testing the SolrPrometheusExporter Deployment")
			deployment := expectDeployment(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName())
			expectedDeploymentLabels := util.MergeLabelsOrAnnotations(solrPrometheusExporter.SharedLabelsWith(solrPrometheusExporter.Labels), map[string]string{"technology": solrv1beta1.SolrPrometheusExporterTechnologyLabel})
			Expect(deployment.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedDeploymentLabels, testDeploymentLabels)), "Incorrect deployment labels")
			Expect(deployment.Annotations).To(Equal(testDeploymentAnnotations), "Incorrect deployment annotations")
			Expect(deployment.Spec.Template.ObjectMeta.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedDeploymentLabels, testPodLabels)), "Incorrect pod labels")
			Expect(deployment.Spec.Template.ObjectMeta.Annotations).To(Equal(util.MergeLabelsOrAnnotations(testPodAnnotations, map[string]string{
				util.PrometheusExporterConfigXmlMd5Annotation: fmt.Sprintf("%x", md5.Sum([]byte(testExporterConfig))),
			})), "Incorrect pod annotations")

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(len(extraContainers1)+1), "Wrong number of containers for the Deployment")
			Expect(deployment.Spec.Template.Spec.Containers[1:]).To(Equal(extraContainers1), "Incorrect sidecar containers")

			Expect(deployment.Spec.Template.Spec.InitContainers).To(HaveLen(len(extraContainers2)), "Wrong number of initContainers for the Deployment")
			Expect(deployment.Spec.Template.Spec.InitContainers).To(Equal(extraContainers2), "Incorrect init containers")

			expectedArgs := []string{
				"-p", strconv.Itoa(util.SolrMetricsPort),
				"-n", strconv.Itoa(int(testNumThreads)),
				"-s", strconv.Itoa(int(testScrapeInterval)),
				"-z", testZkCnxString + testZKChroot,
				"-f", "/opt/solr-exporter/solr-prometheus-exporter.xml",
			}
			Expect(deployment.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedArgs), "Incorrect arguments for the SolrPrometheusExporter container")
			Expect(deployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{util.DefaultPrometheusExporterEntrypoint}), "Incorrect command for the SolrPrometheusExporter container")

			By("testing the SolrPrometheusExporter Deployment Custom Options")
			Expect(deployment.Spec.Template.Spec.PriorityClassName).To(Equal(testPriorityClass), "Incorrect Priority class name for Pod Spec")

			// Test tolerations and node selectors
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(testNodeSelectors), "Incorrect pod node selectors")

			// Other Pod Options
			Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
			Expect(*deployment.Spec.Template.Spec.SecurityContext).To(Equal(testPodSecurityContext), "PodSecurityContext is not the same as the one provided in podOptions")
			Expect(deployment.Spec.Template.Spec.Affinity).To(Equal(testAffinity), "Affinity is not the same as the one provided in podOptions")
			Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits).To(Equal(testResources.Limits), "Resources.Limits is not the same as the one provided in podOptions")
			Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Requests).To(Equal(testResources.Requests), "Resources.Requests is not the same as the one provided in podOptions")
			Expect(deployment.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(append(testAdditionalImagePullSecrets, corev1.LocalObjectReference{Name: testImagePullSecretName})), "Incorrect imagePullSecrets")
			Expect(deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Not(BeNil()), "Incorrect terminationGracePeriodSeconds")
			Expect(*deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(testTerminationGracePeriodSeconds), "Incorrect terminationGracePeriodSeconds")
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(testNodeSelectors), "Incorrect nodeSelector")
			Expect(deployment.Spec.Template.Spec.PriorityClassName).To(Equal(testPriorityClass), "Incorrect priorityClassName")
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal(testServiceAccountName), "Incorrect serviceAccountName")
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(testTolerationsPromExporter), "Incorrect tolerations")
			Expect(deployment.Spec.Template.Spec.SecurityContext).To(Not(BeNil()), "Incorrect tolerations")
			Expect(deployment.Spec.Template.Spec.SecurityContext).To(Not(BeNil()), "Incorrect Pod securityContext")
			Expect(*deployment.Spec.Template.Spec.SecurityContext).To(Equal(testPodSecurityContext), "Incorrect Pod securityContext")
			Expect(deployment.Spec.Template.Spec.Containers[0].Lifecycle).To(Equal(testLifecycle), "Incorrect Container lifecycle")

			Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(len(testTopologySpreadConstraints)), "Wrong number of topologySpreadConstraints")
			Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints[0]).To(Equal(testTopologySpreadConstraints[0]), "Wrong first topologySpreadConstraint")
			expectedSecondTopologyConstraint := testTopologySpreadConstraints[1].DeepCopy()
			expectedSecondTopologyConstraint.LabelSelector = deployment.Spec.Selector
			Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints[1]).To(Equal(*expectedSecondTopologyConstraint), "Wrong second topologySpreadConstraint")

			// Volumes
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(len(extraVolumes)+1), "Container has wrong number of volumeMounts")
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(len(extraVolumes)+1), "Pod has wrong number of volumes")

			// PrometheusExporter config Volume
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("solr-prometheus-exporter-xml"), "PrometheusExporter Config VolumeMount uses wrong name")
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/opt/solr-exporter"), "PrometheusExporter Config VolumeMount uses wrong mountPath")
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].ReadOnly).To(BeTrue(), "PrometheusExporter Config VolumeMount must be readOnly")
			Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal("solr-prometheus-exporter-xml"), "PrometheusExporter Config VolumeMount not using the correct volume name.")
			Expect(deployment.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap).To(Not(BeNil()), "PrometheusExporter Config Volume not using a configMap volume source.")
			Expect(deployment.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.Name).To(Equal(configMap.Name), "PrometheusExporter Config Volume not using the correct configMap.")
			Expect(deployment.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.Items).To(Equal([]corev1.KeyToPath{{Key: util.PrometheusExporterConfigMapKey, Path: util.PrometheusExporterConfigMapKey}}), "PrometheusExporter Config Volume ConfigMap has wrong items.")

			// Extra Volumes
			for i, volume := range extraVolumes {
				volume.DefaultContainerMount.Name = volume.Name
				Expect(deployment.Spec.Template.Spec.Containers[i].VolumeMounts[i+1]).To(Equal(*volume.DefaultContainerMount), "Additional Volume [%d] from podOptions not mounted into container properly.", i)
				Expect(deployment.Spec.Template.Spec.Volumes[i+1].Name).To(Equal(volume.Name), "Additional Volume [%d] from podOptions not loaded into pod properly.", i)
				Expect(deployment.Spec.Template.Spec.Volumes[i+1].VolumeSource).To(Equal(volume.Source), "Additional Volume [%d] from podOptions not loaded into pod properly.", i)
			}

			// Probes
			Expect(deployment.Spec.Template.Spec.Containers[0].LivenessProbe, testProbeLivenessNonDefaults, "Incorrect Liveness Probe")
			Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe, testProbeReadinessNonDefaults, "Incorrect Readiness Probe")
			Expect(deployment.Spec.Template.Spec.Containers[0].StartupProbe, testProbeStartup, "Incorrect Startup Probe")

			By("testing the SolrPrometheusExporter Service")
			service := expectService(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsServiceName(), deployment.Spec.Selector.MatchLabels, false)
			Expect(service.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"), "Metrics Service Prometheus scraping is not enabled.")
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(util.SolrMetricsPort)), "Wrong target port on metrics Service")
			Expect(service.Spec.Ports[0].Name).To(Equal(util.SolrMetricsPortName), "Wrong port name on metrics Service")
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)), "Wrong port number on metrics Service")
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on metrics Service")
			Expect(service.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on metrics Service should not be nil")
			Expect(*service.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on metrics Service")
		})
	})

	FContext("With a Solr Reference & ZK ACLs", func() {
		solrName := "test-solr"
		testZkCnxString := "host-from-solr:2181"
		testZKChroot := "/this/path"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						Name: solrName,
					},
				},
				CustomKubeOptions: solrv1beta1.CustomExporterKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
					},
				},
			}

			solrRef = &solrv1beta1.SolrCloud{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-solr",
					Namespace: "default",
				},
				Spec: solrv1beta1.SolrCloudSpec{
					SolrImage: &solrv1beta1.ContainerImage{
						Tag:             "should-be-the-same",
						PullPolicy:      corev1.PullAlways,
						ImagePullSecret: testImagePullSecretName2,
					},
					ZookeeperRef: &solrv1beta1.ZookeeperRef{
						ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: testZkCnxString,
							ChRoot:                   testZKChroot,
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
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, solrRef)).To(Succeed(), "Creating test SolrCloud for Prometheus Exporter to connect to")
		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment")
			expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
				expectedArgs := []string{
					"-p", strconv.Itoa(util.SolrMetricsPort),
					"-n", strconv.Itoa(1),
					"-z", testZkCnxString + testZKChroot,
					"-f", "/opt/solr/contrib/prometheus-exporter/conf/solr-exporter-config.xml",
				}
				g.Expect(found.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedArgs), "Incorrect arguments for the SolrPrometheusExporter container")
				g.Expect(found.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{util.DefaultPrometheusExporterEntrypoint}), "Incorrect command for the SolrPrometheusExporter container")
				g.Expect(found.Spec.Template.Spec.Containers[0].Image).To(Equal(solrv1beta1.DefaultSolrRepo+":should-be-the-same"), "Incorrect image, should be pulled from the SolrCloud")
				g.Expect(found.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways), "Incorrect imagePullPolicy, should be pulled from the SolrCloud")
				g.Expect(found.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: testImagePullSecretName2}), "Incorrect imagePullSecrets, should be pulled from the SolrCloud")

				// Env Variable Tests
				expectedEnvVars := map[string]string{
					"JAVA_OPTS": "$(SOLR_ZK_CREDS_AND_ACLS)",
				}
				foundEnv := found.Spec.Template.Spec.Containers[0].Env
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
				for _, zkAclEnvVar := range zkAclEnvVars {
					expectedEnvVars[zkAclEnvVar.Name] = zkAclEnvVar.Value
				}
				for _, extraVar := range extraVars {
					expectedEnvVars[extraVar.Name] = extraVar.Value
				}
				g.Expect(foundEnv[0:5]).To(Equal(zkAclEnvVars), "ZK ACL Env Vars are not correct")
				g.Expect(foundEnv[5:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
				testPodEnvVariablesWithGomega(g, expectedEnvVars, foundEnv)
			})
			By("Changing the ZKConnection String of the SolrCloud")

			newZkCnxString := "new-host:2181,another:2181"
			Eventually(func(g Gomega) {
				foundSolrRef := &solrv1beta1.SolrCloud{}
				g.Expect(k8sClient.Get(ctx, resourceKey(solrRef, solrRef.Name), foundSolrRef)).To(Succeed(), "Failed getting test SolrCloud reffered to by the SolrPrometheusExporter")
				foundSolrRef.Spec.ZookeeperRef.ConnectionInfo.InternalConnectionString = newZkCnxString
				g.Expect(k8sClient.Update(ctx, foundSolrRef)).To(Succeed(), "Failed updating the ZK info for the test SolrCloud")
			}).Should(Succeed(), "Could not update the ZKConnectionString for the SolrCloud")

			By("making sure the PrometheusExporter Deployment updates to use the new ZK Connection String")
			expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
				expectedArgs := []string{
					"-p", strconv.Itoa(util.SolrMetricsPort),
					"-n", strconv.Itoa(1),
					"-z", newZkCnxString + testZKChroot,
					"-f", "/opt/solr/contrib/prometheus-exporter/conf/solr-exporter-config.xml",
				}
				g.Expect(found.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedArgs), "Incorrect arguments for the SolrPrometheusExporter container, Zookeeper info not updated after SolrCloud changed")
			})
		})
	})

	FContext("Updating a user-provided ConfigMap | Use explicitly defined image", func() {
		testZkCnxString := "host-from-solr:2181"
		testZKChroot := "/this/path"
		withUserProvidedConfigMapName := "custom-exporter-config"
		BeforeEach(func() {
			solrPrometheusExporter.Spec = solrv1beta1.SolrPrometheusExporterSpec{
				SolrReference: solrv1beta1.SolrReference{
					Cloud: &solrv1beta1.SolrCloudReference{
						ZookeeperConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: testZkCnxString,
							ChRoot:                   testZKChroot,
						},
					},
				},
				CustomKubeOptions: solrv1beta1.CustomExporterKubeOptions{
					ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
						ProvidedConfigMap: withUserProvidedConfigMapName,
					},
				},
				Image: &solrv1beta1.ContainerImage{
					Repository: "test/repo",
				},
			}

			solrRef = &solrv1beta1.SolrCloud{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-solr",
					Namespace: "default",
				},
				Spec: solrv1beta1.SolrCloudSpec{
					SolrImage: &solrv1beta1.ContainerImage{
						Tag:             "should-be-the-same",
						PullPolicy:      corev1.PullAlways,
						ImagePullSecret: testImagePullSecretName2,
					},
					ZookeeperRef: &solrv1beta1.ZookeeperRef{
						ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
							InternalConnectionString: testZkCnxString,
							ChRoot:                   testZKChroot,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, solrRef)).To(Succeed(), "Creating test SolrCloud for Prometheus Exporter to connect to")

		})
		FIt("has the correct resources", func() {
			By("testing the SolrPrometheusExporter Deployment is not created without an existing configMap")
			expectNoDeployment(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName())

			By("creating the user-provided configMap")
			// create the user-provided ConfigMap but w/o the expected key
			userProvidedConfigXml := "<config/>"
			userProvidedConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      withUserProvidedConfigMapName,
					Namespace: solrPrometheusExporter.Namespace,
				},
				Data: map[string]string{
					"foo": userProvidedConfigXml,
				},
			}
			Expect(k8sClient.Create(ctx, userProvidedConfigMap)).To(Succeed(), "Couldn't create the user-provided configMap")

			By("testing the SolrPrometheusExporter Deployment is not created without a correctly configured configMap")
			expectNoDeployment(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName())

			updateUserProvidedConfigMap(ctx, solrPrometheusExporter, withUserProvidedConfigMapName, map[string]string{util.PrometheusExporterConfigMapKey: userProvidedConfigXml})

			By("testing the SolrPrometheusExporter Deployment")
			expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
				expectedAnnotations := map[string]string{
					util.PrometheusExporterConfigXmlMd5Annotation: fmt.Sprintf("%x", md5.Sum([]byte(userProvidedConfigXml))),
				}
				g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "Incorrect pod annotations after updating configMap")
			})

			updatedConfigXml := "<config>updated by user</config>"
			updateUserProvidedConfigMap(ctx, solrPrometheusExporter, withUserProvidedConfigMapName, map[string]string{util.PrometheusExporterConfigMapKey: updatedConfigXml})

			By("testing the SolrPrometheusExporter Deployment")
			expectDeploymentWithChecks(ctx, solrPrometheusExporter, solrPrometheusExporter.MetricsDeploymentName(), func(g Gomega, found *appsv1.Deployment) {
				expectedAnnotations := map[string]string{
					util.PrometheusExporterConfigXmlMd5Annotation: fmt.Sprintf("%x", md5.Sum([]byte(updatedConfigXml))),
				}
				g.Expect(found.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations), "Incorrect pod annotations after updating configMap")

				g.Expect(found.Spec.Template.Spec.Containers[0].Image).To(Equal("test/repo:"+solrv1beta1.DefaultSolrVersion), "The prometheus exporter container has the wrong image, should be using the explictly defined information, not copying from the SolrCloud")
				g.Expect(found.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent), "The prometheus exporter container has the wrong imagePullPolicy, should not be copying from the SolrCloud")

				g.Expect(found.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty(), "The prometheus exporter should not have any imagePullSecrets, since it's not copying image information from the SolrCloud")
			})
		})
	})
})

func updateUserProvidedConfigMap(ctx context.Context, parentResource client.Object, configMapName string, dataMap map[string]string) {
	foundConfigMap := &corev1.ConfigMap{}
	Eventually(func() error { return k8sClient.Get(ctx, resourceKey(parentResource, configMapName), foundConfigMap) }).Should(Succeed())
	foundConfigMap.Data = dataMap
	Expect(k8sClient.Update(ctx, foundConfigMap)).NotTo(HaveOccurred(), "Issue updating the user provided configMap")
}
