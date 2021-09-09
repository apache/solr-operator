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
	"strconv"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SolrCloud controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 5
		duration = time.Second * 3
		interval = time.Millisecond * 250
	)
	SetDefaultConsistentlyDuration(duration)
	SetDefaultConsistentlyPollingInterval(interval)
	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud
		updatedSolrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		ctx = context.Background()

		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("defaulting the missing SolrCloud values")
		updatedSolrCloud = &solrv1beta1.SolrCloud{}
		// We'll need to retry getting this newly created SolrCloud, given that creation may not immediately happen.
		Eventually(func() (bool, error) {
			err := k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), updatedSolrCloud)
			if err != nil {
				return true, err
			}
			return updatedSolrCloud.WithDefaults(), nil
		}).Should(BeFalse())

		By("setting a status for the SolrCloud")
		// We'll need to retry getting this newly created SolrCloud, given that creation may not immediately happen.
		Eventually(func() (string, error) {
			err := k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), updatedSolrCloud)
			if err != nil {
				return "", err
			}
			return updatedSolrCloud.Status.Version, nil
		}).Should(Not(BeEmpty()), "Wait until the SolrCloud has a Status")
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	Context("Regular Solr Cloud", func() {
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
						PodSecurityContext: &podSecurityContext,
						Volumes:            extraVolumes,
						Affinity:           affinity,
						Resources:          resources,
						SidecarContainers:  extraContainers1,
						InitContainers:     extraContainers2,
					},
				},
			}
		})
		It("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Check extra containers
			Expect(len(statefulSet.Spec.Template.Spec.Containers)).To(Equal(3), "Solr StatefulSet requires the solr container plus the desired sidecars.")
			Expect(statefulSet.Spec.Template.Spec.Containers[1:]).To(Equal(extraContainers1), "Solr StatefulSet has incorrect extra containers")

			Expect(len(statefulSet.Spec.Template.Spec.InitContainers)).To(Equal(3), "Solr StatefulSet requires a default initContainer plus the additional specified.")
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
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
			// TODO: Make this not invalidate foundEnv
			Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")

			testPodEnvVariables(expectedEnvVars, append(foundEnv[:len(foundEnv)-3], foundEnv[len(foundEnv)-1]))

			// Other Pod Options Checks
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart).To(BeNil(), "Post-start command should be nil since there is no chRoot to ensure exists.")
			Expect(statefulSet.Spec.Template.Spec.SecurityContext).To(Not(BeNil()), "PodSecurityContext is not the same as the one provided in podOptions")
			Expect(*statefulSet.Spec.Template.Spec.SecurityContext).To(Equal(podSecurityContext), "PodSecurityContext is not the same as the one provided in podOptions")
			Expect(statefulSet.Spec.Template.Spec.Affinity).To(Equal(affinity), "Affinity is not the same as the one provided in podOptions")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits).To(Equal(resources.Limits), "Resources.Limits is not the same as the one provided in podOptions")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests).To(Equal(resources.Requests), "Resources.Requests is not the same as the one provided in podOptions")
			extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
			Expect(len(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts)).To(Equal(len(extraVolumes)+1), "Container has wrong number of volumeMounts")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[1]).To(Equal(*extraVolumes[0].DefaultContainerMount), "Additional Volume from podOptions not mounted into container properly.")
			Expect(len(statefulSet.Spec.Template.Spec.Volumes)).To(Equal(len(extraVolumes)+2), "Pod has wrong number of volumes")
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

	Context("Solr Cloud with Custom Kube Options", func() {
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
					},
				},
				UpdateStrategy: solrv1beta1.SolrUpdateStrategy{
					Method:          solrv1beta1.StatefulSetUpdate,
					RestartSchedule: "@every 30m",
				},
				SolrGCTune: "gc Options",
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						Annotations:                   testPodAnnotations,
						Labels:                        testPodLabels,
						Tolerations:                   testTolerations,
						NodeSelector:                  testNodeSelectors,
						LivenessProbe:                 testProbeLivenessNonDefaults,
						ReadinessProbe:                testProbeReadinessNonDefaults,
						StartupProbe:                  testProbeStartup,
						PriorityClassName:             testPriorityClass,
						ImagePullSecrets:              testAdditionalImagePullSecrets,
						TerminationGracePeriodSeconds: &testTerminationGracePeriodSeconds,
						ServiceAccountName:            testServiceAccountName,
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
		It("has the correct resources", func() {
			By("testing the Solr ConfigMap")
			configMap := expectConfigMap(ctx, solrCloud, solrCloud.ConfigMapName(), map[string]string{"solr.xml": util.DefaultSolrXML})
			testMapsEqual("configMap labels", util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), testConfigMapLabels), configMap.Labels)
			testMapsEqual("configMap annotations", testConfigMapAnnotations, configMap.Annotations)

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			Expect(statefulSet.Spec.Replicas).To(Equal(solrCloud.Spec.Replicas), "Solr StatefulSet has incorrect number of replicas.")

			Expect(statefulSet.Spec.Template.Annotations).To(HaveKey(util.SolrXmlMd5Annotation), "solr.xml MD5 annotation should be in the pod template!")
			Expect(statefulSet.Spec.Template.Annotations[util.SolrXmlMd5Annotation]).To(Equal(fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))), "Wrong solr.xml MD5 annotation in the pod template!")

			Expect(len(statefulSet.Spec.Template.Spec.Containers)).To(Equal(1), "Solr StatefulSet requires a container.")
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME).foo-solrcloud-headless.default",
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"GC_TUNE":        "gc Options",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
				"SOLR_STOP_WAIT": strconv.FormatInt(testTerminationGracePeriodSeconds-5, 10),
			}
			expectedStatefulSetLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"technology": util.SolrCloudPVCTechnology})
			expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: "host:7271/"}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			testMapsEqual("statefulSet labels", util.MergeLabelsOrAnnotations(expectedStatefulSetLabels, testSSLabels), statefulSet.Labels)
			testMapsEqual("statefulSet annotations", util.MergeLabelsOrAnnotations(expectedStatefulSetAnnotations, testSSAnnotations), statefulSet.Annotations)
			testMapsEqual("pod labels", util.MergeLabelsOrAnnotations(expectedStatefulSetLabels, testPodLabels), statefulSet.Spec.Template.ObjectMeta.Labels)
			Expect(statefulSet.Spec.Template.ObjectMeta.Annotations).To(HaveKey(util.SolrScheduledRestartAnnotation), "Pod Template does not have scheduled restart annotation when it should")
			// Remove the annotation when we know that it exists, we don't know the exact value so we can't check it below.
			delete(statefulSet.Spec.Template.Annotations, util.SolrScheduledRestartAnnotation)
			testMapsEqual("pod annotations", util.MergeLabelsOrAnnotations(map[string]string{"solr.apache.org/solrXmlMd5": fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data["solr.xml"])))}, testPodAnnotations), statefulSet.Spec.Template.Annotations)
			testMapsEqual("pod node selectors", testNodeSelectors, statefulSet.Spec.Template.Spec.NodeSelector)
			testPodProbe(testProbeLivenessNonDefaults, statefulSet.Spec.Template.Spec.Containers[0].LivenessProbe, "liveness")
			testPodProbe(testProbeReadinessNonDefaults, statefulSet.Spec.Template.Spec.Containers[0].ReadinessProbe, "readiness")
			testPodProbe(testProbeStartup, statefulSet.Spec.Template.Spec.Containers[0].StartupProbe, "startup")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")
			testPodTolerations(testTolerations, statefulSet.Spec.Template.Spec.Tolerations)
			Expect(statefulSet.Spec.Template.Spec.PriorityClassName).To(Equal(testPriorityClass), "Incorrect Priority class name for Pod Spec")
			Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(append(testAdditionalImagePullSecrets, corev1.LocalObjectReference{Name: testImagePullSecretName})), "Incorrect imagePullSecrets")
			Expect(statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(&testTerminationGracePeriodSeconds), "Incorrect terminationGracePeriodSeconds")
			Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal(testServiceAccountName), "Incorrect serviceAccountName")

			// Check the update strategy
			Expect(statefulSet.Spec.UpdateStrategy.Type).To(Equal(appsv1.RollingUpdateStatefulSetStrategyType), "Incorrect statefulset update strategy")
			Expect(statefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.OrderedReadyPodManagement), "Incorrect statefulset pod management policy")

			By("testing the Solr Common Service")
			commonService := expectService(ctx, solrCloud, solrCloud.CommonServiceName(), statefulSet.Spec.Selector.MatchLabels, false)
			expectedCommonServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "common"})
			testMapsEqual("common service labels", util.MergeLabelsOrAnnotations(expectedCommonServiceLabels, testCommonServiceLabels), commonService.Labels)
			testMapsEqual("common service annotations", testCommonServiceAnnotations, commonService.Annotations)

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			expectedHeadlessServiceLabels := util.MergeLabelsOrAnnotations(solrCloud.SharedLabelsWith(solrCloud.Labels), map[string]string{"service-type": "headless"})
			testMapsEqual("headless service labels", util.MergeLabelsOrAnnotations(expectedHeadlessServiceLabels, testHeadlessServiceLabels), headlessService.Labels)
			testMapsEqual("headless service annotations", testHeadlessServiceAnnotations, headlessService.Annotations)
		})
	})

	Context("Solr Cloud with External Zookeeper & Chroot", func() {
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
		It("has the correct resources", func() {
			By("testing the Solr Status values")
			status := expectStatus(ctx, solrCloud)
			Expect(status.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(connString), "Wrong internal zkConnectionString in status")
			Expect(status.ZookeeperConnectionInfo.ExternalConnectionString).To(Not(BeNil()), "External zkConnectionString in status should not be nil")
			Expect(*status.ZookeeperConnectionInfo.ExternalConnectionString).To(Equal(connString), "Wrong external zkConnectionString in status")
			Expect(status.ZookeeperConnectionInfo.ChRoot).To(Equal("/a-ch/root"), "Wrong zk chRoot in status")
			Expect(status.ZookeeperConnectionInfo.ZkConnectionString()).To(Equal(connString + "/a-ch/root"), "Wrong zkConnectionString()")

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(len(statefulSet.Spec.Template.Spec.Containers)).To(Equal(1), "Solr StatefulSet requires a container.")
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
			testMapsEqual("statefulSet annotations", expectedStatefulSetAnnotations, statefulSet.Annotations)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart.Exec.Command).To(ConsistOf("sh", "-c", "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}"), "Incorrect post-start command")
			Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(BeEmpty(), "No custom serviceAccountName specified, so the field should be empty.")
		})
	})

	Context("Setting default values for the SolrCloud resource", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Persistence: &solrv1beta1.ZKPersistence{},
					},
				},
			}
		})
		It("has the correct default values", func() {
			// Solr defaults
			Expect(updatedSolrCloud.Spec.Replicas).To(Not(BeNil()), "Bad Default - Spec.Replicas")
			Expect(*updatedSolrCloud.Spec.Replicas).To(Equal(solrv1beta1.DefaultSolrReplicas), "Bad Default - Spec.Replicas")
			Expect(updatedSolrCloud.Spec.SolrImage).To(Not(BeNil()), "Bad Default - instance.Spec.SolrImage")
			Expect(updatedSolrCloud.Spec.SolrImage.Repository).To(Equal(solrv1beta1.DefaultSolrRepo), "Bad Default - instance.Spec.SolrImage.Repository")
			Expect(updatedSolrCloud.Spec.SolrImage.Tag).To(Equal(solrv1beta1.DefaultSolrVersion), "Bad Default - instance.Spec.SolrImage.Tag")
			Expect(updatedSolrCloud.Spec.BusyBoxImage).To(Not(BeNil()), "Bad Default - instance.Spec.BusyBoxImage")
			Expect(updatedSolrCloud.Spec.BusyBoxImage.Repository).To(Equal(solrv1beta1.DefaultBusyBoxImageRepo), "Bad Default - instance.Spec.BusyBoxImage.Repository")
			Expect(updatedSolrCloud.Spec.BusyBoxImage.Tag).To(Equal(solrv1beta1.DefaultBusyBoxImageVersion), "Bad Default - instance.Spec.BusyBoxImage.Tag")
			Expect(updatedSolrCloud.Spec.SolrAddressability.CommonServicePort).To(Equal(80), "Bad Default - instance.Spec.SolrAddressability.CommonServicePort")
			Expect(updatedSolrCloud.Spec.SolrAddressability.PodPort).To(Equal(8983), "Bad Default - instance.Spec.SolrAddressability.PodPort")
			Expect(updatedSolrCloud.Spec.SolrAddressability.External).To(BeNil(), "Bad Default - instance.Spec.SolrAddressability.External")

			// Check the default Zookeeper
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Replicas).To(Not(BeNil()), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Replicas")
			Expect(*updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Replicas).To(Equal(solrv1beta1.DefaultZkReplicas), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Replicas")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Image).To(Not(BeNil()), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Image.Repository).To(Equal(solrv1beta1.DefaultZkRepo), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Repository")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Image.Tag).To(Equal(solrv1beta1.DefaultZkVersion), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Image.Tag")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Persistence).To(Not(BeNil()), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.VolumeReclaimPolicy).To(Equal(solrv1beta1.DefaultZkVolumeReclaimPolicy), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.VolumeReclaimPolicy).To(Equal(solrv1beta1.DefaultZkVolumeReclaimPolicy), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.VolumeReclaimPolicy")
			Expect(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.PersistentVolumeClaimSpec).To(Not(BeNil()), "Bad Default - instance.Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec")
			Expect(len(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.PersistentVolumeClaimSpec.Resources.Requests)).To(Equal(1), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.Resources length")
			Expect(len(updatedSolrCloud.Spec.ZookeeperRef.ProvidedZookeeper.Persistence.PersistentVolumeClaimSpec.AccessModes)).To(Equal(1), "Bad Default - Spec.ZookeeperRef.ProvidedZookeeper.Zookeeper.Persistence.PersistentVolumeClaimSpec.AccesModes length")

		})
	})

	Context("Setting default values for the SolrCloud resource", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
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
		It("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(len(statefulSet.Spec.Template.Spec.Containers)).To(Equal(1), "Solr StatefulSet requires the solr container.")
			
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
			testMapsEqual("common service annotations", testCommonServiceAnnotations, commonService.Annotations)
			Expect(commonService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(commonService.Spec.Ports[0].Port).To(Equal(int32(5000)), "Wrong port on common Service")
			Expect(commonService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on common Service")

			By("testing the Solr Headless Service")
			headlessService := expectService(ctx, solrCloud, solrCloud.HeadlessServiceName(), statefulSet.Spec.Selector.MatchLabels, true)
			testMapsEqual("headless service annotations", testHeadlessServiceAnnotations, headlessService.Annotations)
			Expect(headlessService.Spec.Ports[0].Name).To(Equal("solr-client"), "Wrong port name on common Service")
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(2000)), "Wrong port on headless Service")
			Expect(headlessService.Spec.Ports[0].TargetPort.StrVal).To(Equal("solr-client"), "Wrong podPort name on headless Service")

			By("making sure no individual Solr Node Services exist")
			nodeNames := solrCloud.GetAllSolrNodeNames()
			for _, nodeName := range nodeNames {
				expectNoService(ctx, solrCloud, nodeName, "Node service shouldn't exist, but it does.")
			}

			By("making sure no Ingress was created")
			expectNoIngress(ctx, solrCloud, solrCloud.CommonIngressName())

			By("making sure the node addresses in the Status are correct")
			status := expectStatus(ctx, solrCloud)
			Expect(status.InternalCommonAddress).To(Equal("http://"+solrCloud.CommonServiceName()+"."+solrCloud.Namespace+".svc."+testKubeDomain+":5000"), "Wrong internal common address in status")
			Expect(status.ExternalCommonAddress).To(BeNil(), "External common address in status should be nil")
		})
	})
})
