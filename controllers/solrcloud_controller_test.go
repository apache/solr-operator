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
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
	"strings"
)

var _ = FDescribe("SolrCloud controller - General", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		ctx = context.Background()

		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("defaulting the missing SolrCloud values")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Regular Solr Cloud", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				SolrJavaMem:  "-Xmx4G",
				SolrOpts:     "extra-opts",
				SolrLogLevel: "DEBUG",
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables:       extraVars,
						PodSecurityContext: &testPodSecurityContext,
						Volumes:            extraVolumes,
						Affinity:           testAffinity,
						Resources:          testResources,
						SidecarContainers:  extraContainers1,
						InitContainers:     extraContainers2,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Check extra containers
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(3), "Solr StatefulSet requires the solr container plus the desired sidecars.")
			Expect(statefulSet.Spec.Template.Spec.Containers[1:]).To(Equal(extraContainers1), "Solr StatefulSet has incorrect extra containers")

			Expect(statefulSet.Spec.Template.Spec.InitContainers).To(HaveLen(3), "Solr StatefulSet requires a default initContainer plus the additional specified.")
			Expect(statefulSet.Spec.Template.Spec.InitContainers[1:]).To(Equal(extraContainers2), "Solr StatefulSet has incorrect extra initContainers")

			// Check the update strategy
			Expect(statefulSet.Spec.UpdateStrategy.Type).To(Equal(appsv1.OnDeleteStatefulSetStrategyType), "Incorrect statefulset update strategy")
			Expect(statefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.ParallelPodManagement), "Incorrect statefulset pod management policy")

			// Host Alias Tests
			Expect(statefulSet.Spec.Template.Spec.HostAliases).To(BeNil(), "There is no need for host aliases because traffic is going directly to pods.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_JAVA_MEM":  "-Xmx4G",
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"SOLR_LOG_LEVEL": "DEBUG",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) extra-opts",
			}
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
			// TODO: Make this not invalidate foundEnv
			Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")

			testPodEnvVariables(expectedEnvVars, append(foundEnv[:len(foundEnv)-3], foundEnv[len(foundEnv)-1]))

			// Other Pod Options Checks
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart).To(BeNil(), "Post-start command should be nil since there is no chRoot to ensure exists.")
			Expect(statefulSet.Spec.Template.Spec.SecurityContext).To(Not(BeNil()), "PodSecurityContext is not the same as the one provided in podOptions")
			Expect(*statefulSet.Spec.Template.Spec.SecurityContext).To(Equal(testPodSecurityContext), "PodSecurityContext is not the same as the one provided in podOptions")
			Expect(statefulSet.Spec.Template.Spec.Affinity).To(Equal(testAffinity), "Affinity is not the same as the one provided in podOptions")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits).To(Equal(testResources.Limits), "Resources.Limits is not the same as the one provided in podOptions")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests).To(Equal(testResources.Requests), "Resources.Requests is not the same as the one provided in podOptions")
			extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
			Expect(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(len(extraVolumes)+1), "Container has wrong number of volumeMounts")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[1]).To(Equal(*extraVolumes[0].DefaultContainerMount), "Additional Volume from podOptions not mounted into container properly.")
			Expect(statefulSet.Spec.Template.Spec.Volumes).To(HaveLen(len(extraVolumes)+2), "Pod has wrong number of volumes")
			Expect(statefulSet.Spec.Template.Spec.Volumes[2].Name).To(Equal(extraVolumes[0].Name), "Additional Volume from podOptions not loaded into pod properly.")
			Expect(statefulSet.Spec.Template.Spec.Volumes[2].VolumeSource).To(Equal(extraVolumes[0].Source), "Additional Volume from podOptions not loaded into pod properly.")

			By("testing the Solr Common Service")
			expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)

			By("testing the Solr Headless Service")
			expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())
		})
	})

	FContext("Solr Cloud with Custom Kube Options", func() {
		BeforeEach(func() {
			replicas := int32(4)
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				SolrImage: &solrv1beta1.ContainerImage{
					ImagePullSecret: testImagePullSecretName,
				},
				Replicas: &replicas,
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
						ChRoot:                   "/test",
					},
				},
				UpdateStrategy: solrv1beta1.SolrUpdateStrategy{
					Method:          solrv1beta1.StatefulSetUpdate,
					RestartSchedule: "@every 30m",
				},
				SolrGCTune: "gc Options",
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						Annotations:    testPodAnnotations,
						Labels:         testPodLabels,
						Resources:      testResources,
						Tolerations:    testTolerations,
						NodeSelector:   testNodeSelectors,
						LivenessProbe:  testProbeLivenessNonDefaults,
						ReadinessProbe: testProbeReadinessNonDefaults,
						StartupProbe:   testProbeStartup,
						Lifecycle:      testLifecycle, PriorityClassName: testPriorityClass,
						ImagePullSecrets:              testAdditionalImagePullSecrets,
						TerminationGracePeriodSeconds: &testTerminationGracePeriodSeconds,
						ServiceAccountName:            testServiceAccountName,
						TopologySpreadConstraints:     testTopologySpreadConstraints,
						DefaultInitContainerResources: testResources2,
						InitContainers:                extraContainers1,
					},
					StatefulSetOptions: &solrv1beta1.StatefulSetOptions{
						Annotations:         testSSAnnotations,
						Labels:              testSSLabels,
						PodManagementPolicy: appsv1.OrderedReadyPodManagement,
					},
					CommonServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testCommonServiceAnnotations,
						Labels:      testCommonServiceLabels,
					},
					HeadlessServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testHeadlessServiceAnnotations,
						Labels:      testHeadlessServiceLabels,
					},
					ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
						Annotations: testConfigMapAnnotations,
						Labels:      testConfigMapLabels,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr ConfigMap")
			configMap := expectConfigMap(ctx, solrCloud, solrCloud.ConfigMapName(), map[string]string{"solr.xml": util.GenerateSolrXMLString("", []string{}, []string{})})
			Expect(configMap.Labels).To(Equal(util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testConfigMapLabels)), "Incorrect configMap labels")
			Expect(configMap.Annotations).To(Equal(testConfigMapAnnotations), "Incorrect configMap annotations")

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			Expect(statefulSet.Spec.Replicas).To(Equal(solrCloud.Spec.Replicas), "Solr StatefulSet has incorrect number of replicas.")

			Expect(statefulSet.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))), "Wrong solr.xml MD5 annotation in the pod template!")

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/test",
				"SOLR_HOST":      "$(POD_HOSTNAME).foo-solrcloud-headless.default",
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"GC_TUNE":        "gc Options",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
				"SOLR_STOP_WAIT": strconv.FormatInt(testTerminationGracePeriodSeconds-5, 10),
			}
			expectedStatefulSetLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"technology": util.SolrCloudPVCTechnology})
			expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: "host:7271/test"}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedStatefulSetLabels, testSSLabels)), "Incorrect statefulSet labels")
			Expect(statefulSet.Annotations).To(Equal(util.MergeLabelsOrAnnotations(expectedStatefulSetAnnotations, testSSAnnotations)), "Incorrect statefulSet annotations")
			Expect(statefulSet.Spec.Template.ObjectMeta.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedStatefulSetLabels, testPodLabels)), "Incorrect pod labels")
			Expect(statefulSet.Spec.Template.ObjectMeta.Annotations).To(HaveKey(util.SolrScheduledRestartAnnotation), "Pod Template does not have scheduled restart annotation when it should")
			// Remove the annotation when we know that it exists, we don't know the exact value so we can't check it below.
			delete(statefulSet.Spec.Template.Annotations, util.SolrScheduledRestartAnnotation)
			Expect(statefulSet.Spec.Template.Annotations).To(Equal(util.MergeLabelsOrAnnotations(map[string]string{"solr.apache.org/solrXmlMd5": fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data["solr.xml"])))}, testPodAnnotations)), "Incorrect pod annotations")
			Expect(statefulSet.Spec.Template.Spec.NodeSelector).To(Equal(testNodeSelectors), "Incorrect pod node selectors")

			Expect(statefulSet.Spec.Template.Spec.Containers[0].LivenessProbe, testProbeLivenessNonDefaults, "Incorrect Liveness Probe")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].ReadinessProbe, testProbeReadinessNonDefaults, "Incorrect Readiness Probe")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].StartupProbe, testProbeStartup, "Incorrect Startup Probe")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle).To(Equal(testLifecycle), "Incorrect container lifecycle")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources).To(Equal(testResources), "Incorrect container resources")
			Expect(statefulSet.Spec.Template.Spec.InitContainers[0].Resources).To(Equal(testResources2), "Incorrect initContainer[0] resources")
			Expect(statefulSet.Spec.Template.Spec.InitContainers[1].Resources).To(Equal(testResources2), "Incorrect initContainer[1] resources")
			Expect(statefulSet.Spec.Template.Spec.InitContainers[2].Resources).ToNot(Equal(testResources2), "Incorrect initContainer[2] resources, should not use the default override")
			Expect(statefulSet.Spec.Template.Spec.Tolerations).To(Equal(testTolerations), "Incorrect Tolerations for Pod")
			Expect(statefulSet.Spec.Template.Spec.PriorityClassName).To(Equal(testPriorityClass), "Incorrect Priority class name for Pod Spec")
			Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(append(testAdditionalImagePullSecrets, corev1.LocalObjectReference{Name: testImagePullSecretName})), "Incorrect imagePullSecrets")
			Expect(statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(&testTerminationGracePeriodSeconds), "Incorrect terminationGracePeriodSeconds")
			Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal(testServiceAccountName), "Incorrect serviceAccountName")
			Expect(statefulSet.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(len(testTopologySpreadConstraints)), "Wrong number of topologySpreadConstraints")
			Expect(statefulSet.Spec.Template.Spec.TopologySpreadConstraints[0]).To(Equal(testTopologySpreadConstraints[0]), "Wrong first topologySpreadConstraint")
			expectedSecondTopologyConstraint := testTopologySpreadConstraints[1].DeepCopy()
			expectedSecondTopologyConstraint.LabelSelector = statefulSet.Spec.Selector
			Expect(statefulSet.Spec.Template.Spec.TopologySpreadConstraints[1]).To(Equal(*expectedSecondTopologyConstraint), "Wrong second topologySpreadConstraint")

			// Check the update strategy
			Expect(statefulSet.Spec.UpdateStrategy.Type).To(Equal(appsv1.RollingUpdateStatefulSetStrategyType), "Incorrect statefulset update strategy")
			Expect(statefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.OrderedReadyPodManagement), "Incorrect statefulset pod management policy")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "common"})
			Expect(commonService.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedCommonServiceLabels, testCommonServiceLabels)), "Incorrect common service labels")
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on common Service")
			Expect(commonService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on common Service should not be nil")
			Expect(*commonService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "headless"})
			Expect(headlessService.Labels).To(Equal(util.MergeLabelsOrAnnotations(expectedHeadlessServiceLabels, testHeadlessServiceLabels)), "Incorrect headless service labels")
			Expect(headlessService.Annotations).To(Equal(testHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "Wrong protocol on headless Service")
			Expect(headlessService.Spec.Ports[0].AppProtocol).ToNot(BeNil(), "AppProtocol on headless Service should not be nil")
			Expect(*headlessService.Spec.Ports[0].AppProtocol).To(Equal("http"), "Wrong appProtocol on headless Service")
		})
	})

	FContext("Solr Cloud with External Zookeeper & Chroot", func() {
		connString := "host:7271,host2:7271"
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						ChRoot:                   "a-ch/root",
						ExternalConnectionString: &connString,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr Status values")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(connString), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(Not(BeNil()), "External zkConnectionString in status should not be nil")
				g.Expect(*found.ZookeeperConnectionInfo.ExternalConnectionString).To(Equal(connString), "Wrong external zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/a-ch/root"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ZkConnectionString()).To(Equal(connString+"/a-ch/root"), "Wrong zkConnectionString()")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")
			expectedZKHost := "host:7271,host2:7271/a-ch/root"
			expectedEnvVars := map[string]string{
				"ZK_HOST":   expectedZKHost,
				"ZK_SERVER": "host:7271,host2:7271",
				"ZK_CHROOT": "/a-ch/root",
				"SOLR_HOST": "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT": "8983",
				"GC_TUNE":   "",
			}
			expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: expectedZKHost}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Annotations).To(Equal(expectedStatefulSetAnnotations), "Incorrect statefulSet annotations")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart.Exec.Command).To(ConsistOf("sh", "-c", "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}"), "Incorrect post-start command")
			Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(BeEmpty(), "No custom serviceAccountName specified, so the field should be empty.")
		})
	})

	FContext("Setting default values for the SolrCloud resource", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Persistence: &solrv1beta1.ZKPersistence{},
					},
				},
			}
		})
		FIt("has the correct default values", func() {
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {

				// Solr defaults
				g.Expect(found.Spec.Replicas).To(Not(BeNil()), "Bad Default - Spec.Replicas")
				g.Expect(*found.Spec.Replicas).To(Equal(solrv1beta1.DefaultSolrReplicas), "Bad Default - Spec.Replicas")
				g.Expect(found.Spec.SolrImage).To(Not(BeNil()), "Bad Default - instance.Spec.SolrImage")
				g.Expect(found.Spec.SolrImage.Repository).To(Equal(solrv1beta1.DefaultSolrRepo), "Bad Default - instance.Spec.SolrImage.Repository")
				g.Expect(found.Spec.SolrImage.Tag).To(Equal(solrv1beta1.DefaultSolrVersion), "Bad Default - instance.Spec.SolrImage.Tag")
				g.Expect(found.Spec.BusyBoxImage).To(Not(BeNil()), "Bad Default - instance.Spec.BusyBoxImage")
				g.Expect(found.Spec.BusyBoxImage.Repository).To(Equal(solrv1beta1.DefaultBusyBoxImageRepo), "Bad Default - instance.Spec.BusyBoxImage.Repository")
				g.Expect(found.Spec.BusyBoxImage.Tag).To(Equal(solrv1beta1.DefaultBusyBoxImageVersion), "Bad Default - instance.Spec.BusyBoxImage.Tag")
				g.Expect(found.Spec.SolrAddressability.CommonServicePort).To(Equal(80), "Bad Default - instance.Spec.SolrAddressability.CommonServicePort")
				g.Expect(found.Spec.SolrAddressability.PodPort).To(Equal(8983), "Bad Default - instance.Spec.SolrAddressability.PodPort")
				g.Expect(found.Spec.SolrAddressability.External).To(BeNil(), "Bad Default - instance.Spec.SolrAddressability.External")

				// Check the default Zookeeper
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Replicas).To(Not(BeNil()), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Replicas")
				g.Expect(*found.Spec.ZookeeperRef.ProvidedZookeeper.Replicas).To(Equal(solrv1beta1.DefaultZkReplicas), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Replicas")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Image).To(Not(BeNil()), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Image.Repository).To(Equal(solrv1beta1.DefaultZkRepo), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Repository")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Image.Tag).To(Equal(solrv1beta1.DefaultZkVersion), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Tag")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Persistence).To(Not(BeNil()), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.VolumeReclaimPolicy).To(Equal(solrv1beta1.DefaultZkVolumeReclaimPolicy), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.VolumeReclaimPolicy).To(Equal(solrv1beta1.DefaultZkVolumeReclaimPolicy), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.PersistentVolumeClaimSpec).To(Not(BeNil()), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.PersistentVolumeClaimSpec.Resources.Requests).To(HaveLen(1), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.Resources length")
				g.Expect(found.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.PersistentVolumeClaimSpec.AccessModes).To(HaveLen(1), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.AccesModes length")
			})

		})
	})

	FContext("Solr Cloud with an explicit kube domain", func() {
		BeforeEach(func() {
			replicas := int32(2)
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				Replicas: &replicas,
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				SolrAddressability: solrv1beta1.SolrAddressabilityOptions{
					PodPort:           2000,
					CommonServicePort: 5000,
					KubeDomain:        testKubeDomain,
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					CommonServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testCommonServiceAnnotations,
						Labels:      testCommonServiceLabels,
					},
					HeadlessServiceOptions: &solrv1beta1.ServiceOptions{
						Annotations: testHeadlessServiceAnnotations,
						Labels:      testHeadlessServiceLabels,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires the solr container.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace + ".svc." + testKubeDomain,
				"SOLR_PORT":      "2000",
				"SOLR_NODE_PORT": "2000",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			Expect(commonService.Annotations).To(Equal(testCommonServiceAnnotations), "Incorrect common service annotations")
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(5000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			Expect(headlessService.Annotations).To(Equal(testHeadlessServiceAnnotations), "Incorrect headless service annotations")
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(2000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			expectNoServices(ctx, solrCloud, "Node service shouldn't exist, but it does.", solrCloud.GetAllSolrPodNames())

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+".svc."+testKubeDomain+":5000"), "Wrong internal common address in status")
				g.Expect(found.ExternalCommonAddress).To(BeNil(), "External common address in status should be nil")
			})
		})
	})

	FContext("Solr Cloud with changing generated SolrXML", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				SolrModules: []string{"analytics", "ltr"},
				BackupRepositories: []solrv1beta1.SolrBackupRepository{
					{
						Name: "test1",
						GCS:  &solrv1beta1.GcsRepository{},
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr ConfigMap")
			configMap := expectConfigMap(ctx, solrCloud, solrCloud.ConfigMapName(), map[string]string{"solr.xml": util.GenerateSolrXMLStringForCloud(solrCloud)})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			Expect(statefulSet.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))), "Wrong solr.xml MD5 annotation in the pod template!")

			By("making sure the solr.xml is updated and a rolling restart happens when libs change")
			foundSolrCloud := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.AdditionalLibs = []string{"/ext/lib2", "/ext/lib1"}
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Change the additionalLibs for the SolrCloud")
			})

			newConfigMap := expectConfigMap(ctx, solrCloud, solrCloud.ConfigMapName(), map[string]string{"solr.xml": util.GenerateSolrXMLStringForCloud(foundSolrCloud)})

			updateSolrXmlMd5 := fmt.Sprintf("%x", md5.Sum([]byte(newConfigMap.Data[util.SolrXmlFile])))
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, updateSolrXmlMd5), "Custom solr.xml MD5 annotation should be updated on the pod template.")
			})
		})
	})

	FContext("Solr Cloud with a custom Solr XML ConfigMap", func() {
		testCustomSolrXmlConfigMap := "my-custom-solr-xml"
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
						ProvidedConfigMap: testCustomSolrXmlConfigMap,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("ensuring no statefulSet exists when the configMap doesn't exist")
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			By("ensuring no statefulSet exists when the configMap is invalid")
			invalidConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCustomSolrXmlConfigMap,
					Namespace: solrCloud.Namespace,
				},
				Data: map[string]string{
					"foo": "bar",
				},
			}
			Expect(k8sClient.Create(ctx, invalidConfigMap)).To(Succeed(), "Create the invalid configMap")
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			invalidConfigMap.Data[util.SolrXmlFile] = "Invalid XML"
			Expect(k8sClient.Update(ctx, invalidConfigMap)).To(Succeed(), "Update the invalid configMap")
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			By("checking that a configured statefulSet exists when the configMap is valid")
			validConfigMap := util.GenerateConfigMap(solrCloud)
			validConfigMap.Name = testCustomSolrXmlConfigMap
			Expect(k8sClient.Update(ctx, validConfigMap)).To(Succeed(), "Make the test configMap valid")

			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(statefulSet.Spec.Template.Annotations).To(HaveKey(util.SolrXmlMd5Annotation), "Custom solr.xml MD5 annotation should be set on the pod template!")
			Expect(statefulSet.Spec.Template.Annotations).To(Not(HaveKey(util.LogXmlFile)), "Custom log4j2.xml MD5 annotation should not be set on the pod template.")

			Expect(statefulSet.Spec.Template.Annotations).To(Not(BeNil()), "The Solr pod volumes should not be empty, because they must contain a solr-xml entry")
			var solrXmlVol *corev1.Volume = nil
			for _, vol := range statefulSet.Spec.Template.Spec.Volumes {
				if vol.Name == "solr-xml" {
					solrXmlVol = &vol
					break
				}
			}
			Expect(solrXmlVol).To(Not(BeNil()), "Didn't find custom solr.xml volume in sts config!")
			Expect(solrXmlVol.VolumeSource.ConfigMap).To(Not(BeNil()), "solr-xml Volume should have a ConfigMap source")
			Expect(solrXmlVol.VolumeSource.ConfigMap.Name).To(Equal(testCustomSolrXmlConfigMap), "solr-xml has the wrong configMap as its source")

			expectNoConfigMap(ctx, solrCloud, fmt.Sprintf("%s-solrcloud-configmap", solrCloud.GetName()))

			By("making sure a rolling restart happens when the solr.xml is changed")
			validConfigMap.Data[util.SolrXmlFile] = strings.Replace(validConfigMap.Data[util.SolrXmlFile], "${zkClientTimeout:30000}", "${zkClientTimeout:15000}", 1)
			Expect(k8sClient.Update(ctx, validConfigMap)).To(Succeed(), "Change the valid test configMap")

			updateSolrXmlMd5 := fmt.Sprintf("%x", md5.Sum([]byte(validConfigMap.Data[util.SolrXmlFile])))
			statefulSet = expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, updateSolrXmlMd5), "Custom solr.xml MD5 annotation should be updated on the pod template.")
			})
		})
	})

	FContext("Solr Cloud with a custom Solr Log4J ConfigMap", func() {
		testCustomConfigMap := "my-custom-config-xml"
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
						ProvidedConfigMap: testCustomConfigMap,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCustomConfigMap,
					Namespace: solrCloud.Namespace,
				},
				Data: map[string]string{
					util.LogXmlFile: "<Configuration/>",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed(), "Create the valid configMap")

			expectedMountPath := fmt.Sprintf("/var/solr/%s", testCustomConfigMap)
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKey(util.LogXmlMd5Annotation), "Custom log XML MD5 annotation should be set on the pod template!")

				g.Expect(found.Spec.Template.Spec.Volumes).To(Not(BeNil()), "Volumes are required for the Solr Pod")
				var customConfigVol *corev1.Volume = nil
				for _, vol := range found.Spec.Template.Spec.Volumes {
					if vol.Name == "log4j2-xml" {
						customConfigVol = &vol
						break
					}
				}
				g.Expect(customConfigVol).To(Not(BeNil()), "Didn't find custom solr-xml volume in pod template!")
				g.Expect(customConfigVol.VolumeSource.ConfigMap, "solr-xml Volume should have a ConfigMap source")
				g.Expect(customConfigVol.VolumeSource.ConfigMap.Name).To(Equal(testCustomConfigMap), "solr-xml has the wrong configMap as its source")

				var logXmlVolMount *corev1.VolumeMount = nil
				for _, mount := range found.Spec.Template.Spec.Containers[0].VolumeMounts {
					if mount.Name == "log4j2-xml" {
						logXmlVolMount = &mount
						break
					}
				}
				g.Expect(logXmlVolMount).To(Not(BeNil()), "Didn't find the log4j2-xml Volume mount")
				g.Expect(logXmlVolMount.MountPath).To(Equal(expectedMountPath), "log4j2-xml Volume mount has the wrong path")

				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(util.GenerateSolrXMLString("", []string{}, []string{}))))), "Custom solr.xml MD5 annotation should be set on the pod template.")

				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.LogXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.LogXmlFile])))), "Custom log4j2.xml MD5 annotation should be set on the pod template.")
				expectedEnvVars := map[string]string{"LOG4J_PROPS": fmt.Sprintf("%s/%s", expectedMountPath, util.LogXmlFile)}
				testPodEnvVariablesWithGomega(g, expectedEnvVars, found.Spec.Template.Spec.Containers[0].Env)
			})

			expectConfigMap(ctx, solrCloud, fmt.Sprintf("%s-solrcloud-configmap", solrCloud.GetName()), map[string]string{util.SolrXmlFile: util.GenerateSolrXMLString("", []string{}, []string{})})

			By("updating the user-provided log XML to trigger a pod rolling restart")
			configMap.Data[util.LogXmlFile] = "<Configuration>Updated!</Configuration>"
			Expect(k8sClient.Update(ctx, configMap)).To(Succeed(), "Change the test log4j configMap")

			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.LogXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.LogXmlFile])))), "Custom log4j2.xml MD5 annotation should be set on the pod template.")

				expectedEnvVars := map[string]string{"LOG4J_PROPS": fmt.Sprintf("%s/%s", expectedMountPath, util.LogXmlFile)}
				testPodEnvVariablesWithGomega(g, expectedEnvVars, found.Spec.Template.Spec.Containers[0].Env)
			})

			By("updating the user-provided log XML again to trigger another pod rolling restart")
			configMap.Data[util.LogXmlFile] = "<Configuration monitorInterval=\\\"30\\\">Updated!</Configuration>"
			Expect(k8sClient.Update(ctx, configMap)).To(Succeed(), "Change the test log4j configMap")

			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(Not(HaveKey(util.LogXmlMd5Annotation)), "Custom log XML MD5 annotation should not be set on the pod template, when monitorInterval is used")

				expectedEnvVars := map[string]string{"LOG4J_PROPS": fmt.Sprintf("%s/%s", expectedMountPath, util.LogXmlFile)}
				testPodEnvVariablesWithGomega(g, expectedEnvVars, found.Spec.Template.Spec.Containers[0].Env)
			})
		})
	})

	FContext("Solr Cloud with a custom Solr Log4J and solr.xml ConfigMap", func() {
		testCustomConfigMap := "my-custom-config-xml"
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					ConfigMapOptions: &solrv1beta1.ConfigMapOptions{
						ProvidedConfigMap: testCustomConfigMap,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCustomConfigMap,
					Namespace: solrCloud.Namespace,
				},
				Data: map[string]string{
					util.LogXmlFile:  "<Configuration/>",
					util.SolrXmlFile: "<solr> ${hostPort:} </solr>", // the controller checks for ${hostPort: in the solr.xml
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed(), "Create the valid configMap")

			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKey(util.LogXmlMd5Annotation), "Custom log XML MD5 annotation should be set on the pod template!")

				g.Expect(found.Spec.Template.Spec.Volumes).To(Not(BeNil()), "Volumes are required for the Solr Pod")
				var customConfigVol *corev1.Volume = nil
				for _, vol := range found.Spec.Template.Spec.Volumes {
					if vol.Name == "solr-xml" {
						customConfigVol = &vol
						break
					}
				}
				g.Expect(customConfigVol).To(Not(BeNil()), "Didn't find custom solr-xml volume in pod template!")
				g.Expect(customConfigVol.VolumeSource.ConfigMap, "solr-xml Volume should have a ConfigMap source")
				g.Expect(customConfigVol.VolumeSource.ConfigMap.Name).To(Equal(testCustomConfigMap), "solr-xml has the wrong configMap as its source")

				var logXmlVolMount *corev1.VolumeMount = nil
				for _, mount := range found.Spec.Template.Spec.Containers[0].VolumeMounts {
					if mount.Name == "solr-xml" {
						logXmlVolMount = &mount
						break
					}
				}
				g.Expect(logXmlVolMount).To(Not(BeNil()), "Didn't find the log4j2-xml Volume mount")
				expectedMountPath := fmt.Sprintf("/var/solr/%s", testCustomConfigMap)
				g.Expect(logXmlVolMount.MountPath).To(Equal(expectedMountPath), "log4j2-xml Volume mount has the wrong path")

				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))), "Custom solr.xml MD5 annotation should be set on the pod template.")

				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.LogXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.LogXmlFile])))), "Custom log4j2.xml MD5 annotation should be set on the pod template.")
				expectedEnvVars := map[string]string{"LOG4J_PROPS": fmt.Sprintf("%s/%s", expectedMountPath, util.LogXmlFile)}
				testPodEnvVariablesWithGomega(g, expectedEnvVars, found.Spec.Template.Spec.Containers[0].Env)
			})

			expectNoConfigMap(ctx, solrCloud, fmt.Sprintf("%s-solrcloud-configmap", solrCloud.GetName()))
		})
	})
})
