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
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	zk_crd "github.com/apache/solr-operator/controllers/zk_api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
)

var _ = FDescribe("SolrCloud controller - Zookeeper", func() {
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

		cleanupTest(ctx, solrCloud)
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

	FContext("ZK Connection String - Admin ACL", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
						AllACL: &solrv1beta1.ZookeeperACL{
							SecretRef:   "secret-name",
							UsernameKey: "user",
							PasswordKey: "pass",
						},
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
					},
				},
				SolrOpts: "-Dextra -Dopts",
			}
		})
		FIt("has the correct resources", func() {
			By("testing the ZK information in the SolrCloud status")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal("host:7271"), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(BeNil(), "An internal ZK Connection String was given, so the external connection string in the status should be nil")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(Not(BeNil()), "All ACL in SolrCloud Status should not be nil when it is provided in the Spec")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(Equal(solrCloud.Spec.ZookeeperRef.ConnectionInfo.AllACL), "Incorrect All ACL in SolrCloud Status")
				g.Expect(found.ZookeeperConnectionInfo.ReadOnlyACL).To(BeNil(), "ReadOnly ACL in SolrCloud Status should be nil when not provided in the Spec")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Check extra containers
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires the solr container only.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_CREDS_AND_ACLS) -Dextra -Dopts",
			}
			insertExpectedAclEnvVars(expectedEnvVars, false)
			for _, envVar := range extraVars {
				expectedEnvVars[envVar.Name] = envVar.Value
			}
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			testPodEnvVariables(expectedEnvVars, foundEnv)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), "Incorrect pre-stop command")
		})
	})

	FContext("ZK Connection String - Admin & Read ACL", func() {
		connectionString := "host:7271"
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						ExternalConnectionString: &connectionString,
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
						ChRoot: "a-ch/root",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
					},
				},
				SolrOpts: "-Dextra -Dopts",
			}
		})
		FIt("has the correct resources", func() {
			By("testing the ZK information in the SolrCloud status")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(connectionString), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/a-ch/root"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(Not(BeNil()), "An external connection string was given in the spec, so it should not be nil in the status")
				g.Expect(*found.ZookeeperConnectionInfo.ExternalConnectionString).To(Equal(connectionString), "Wrong external zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(Not(BeNil()), "All ACL in SolrCloud Status should not be nil when it is provided in the Spec")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(Equal(solrCloud.Spec.ZookeeperRef.ConnectionInfo.AllACL), "Incorrect All ACL in SolrCloud Status")
				g.Expect(found.ZookeeperConnectionInfo.ReadOnlyACL).To(Not(BeNil()), "ReadOnly ACL in SolrCloud Status should not be nil when it is provided in the Spec")
				g.Expect(found.ZookeeperConnectionInfo.ReadOnlyACL).To(Equal(solrCloud.Spec.ZookeeperRef.ConnectionInfo.ReadOnlyACL), "Incorrect ReadOnly ACL in SolrCloud Status")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Check extra containers
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires the solr container only.")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/a-ch/root",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_CREDS_AND_ACLS) -Dextra -Dopts",
			}
			insertExpectedAclEnvVars(expectedEnvVars, true)
			for _, envVar := range extraVars {
				expectedEnvVars[envVar.Name] = envVar.Value
			}
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			testPodEnvVariables(expectedEnvVars, foundEnv)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), "Incorrect pre-stop command")

			By("testing that no ZookeeperCluster is created when using a connection String")
			expectNoZookeeperCluster(ctx, solrCloud, solrCloud.ProvidedZookeeperName())
		})
	})

	FContext("Provided ZK - Ephemeral", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas: &four,
						Image:    &solrv1beta1.ContainerImage{ImagePullSecret: testImagePullSecretName},
						Ephemeral: &solrv1beta1.ZKEphemeral{
							EmptyDirVolumeSource: corev1.EmptyDirVolumeSource{
								Medium: "Memory",
							},
						},
						ZookeeperPod: solrv1beta1.ZookeeperPodPolicy{
							Affinity:                      testAffinity,
							NodeSelector:                  testNodeSelectors,
							Tolerations:                   testTolerations,
							Env:                           extraVars,
							Resources:                     testResources,
							ServiceAccountName:            testServiceAccountName,
							Labels:                        testSSLabels,
							Annotations:                   testSSAnnotations,
							SecurityContext:               &testPodSecurityContext,
							TerminationGracePeriodSeconds: testTerminationGracePeriodSeconds,
							ImagePullSecrets:              testAdditionalImagePullSecrets,
						},
						Config: zkConf,
						ChRoot: "a-ch/root",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					StatefulSetOptions: &solrv1beta1.StatefulSetOptions{
						Annotations: testSSAnnotations,
					},
				},
				UpdateStrategy: solrv1beta1.SolrUpdateStrategy{
					Method: solrv1beta1.ManualUpdate,
				},
			}
		})
		FIt("has the correct resources", func() {
			expectedZkConnStr := "foo-solrcloud-zookeeper-0.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-solrcloud-zookeeper-1.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-solrcloud-zookeeper-2.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-solrcloud-zookeeper-3.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181"

			By("testing the ZK information in the SolrCloud status")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(expectedZkConnStr), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/a-ch/root"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(BeNil(), "Since a provided zk is used, the externalConnectionString in the status should be Nil")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(BeNil(), "All ACL for provided Zookeeper in SolrCloud Status should be nil when not provided in Spec")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(BeNil(), "ReadOnly ACL for provided Zookeeper in SolrCloud Status should be nil when not provided in Spec")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(len(statefulSet.Spec.Template.Spec.Containers)).To(Equal(1), "Solr StatefulSet requires a container.")
			expectedZKHost := expectedZkConnStr + "/a-ch/root"
			expectedEnvVars := map[string]string{
				"ZK_HOST":   expectedZKHost,
				"SOLR_HOST": "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"ZK_SERVER": expectedZkConnStr,
				"ZK_CHROOT": "/a-ch/root",
				"SOLR_PORT": "8983",
				"GC_TUNE":   "",
			}
			expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: expectedZKHost}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Annotations).To(Equal(util.MergeLabelsOrAnnotations(testSSAnnotations, expectedStatefulSetAnnotations)), "Wrong statefulSet annotations")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart.Exec.Command).To(Equal([]string{"sh", "-c", "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}"}), "Incorrect post-start command")
			Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(BeEmpty(), "No custom serviceAccountName specified, so the field should be empty.")

			// Check the update strategy
			Expect(statefulSet.Spec.UpdateStrategy.Type).To(Equal(appsv1.OnDeleteStatefulSetStrategyType), "Incorrect statefulset update strategy")
			Expect(statefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.ParallelPodManagement), "Incorrect statefulset pod management policy")

			zkCluster := expectZookeeperCluster(ctx, solrCloud, solrCloud.ProvidedZookeeperName())

			By("testing the created ZookeeperCluster")
			Expect(zkCluster.Spec.Replicas).To(Equal(four), "Incorrect zkCluster replicas")
			Expect(zkCluster.Spec.StorageType).To(Equal("ephemeral"), "Incorrect zkCluster storage type")
			Expect(zkCluster.Spec.Ephemeral).To(Not(BeNil()), "ZkCluster.spec.ephemeral should not be nil")
			Expect(zkCluster.Spec.Ephemeral.EmptyDirVolumeSource.Medium).To(BeEquivalentTo("Memory"), "Incorrect EmptyDir medium for ZK Cluster ephemeral storage")
			Expect(zkCluster.Spec.Persistence).To(BeNil(), "ZkCluster.spec.persistence should be nil when using ephermeral storage")

			// Check ZK Pod Options
			Expect(zkCluster.Spec.Pod.Affinity).To(Equal(testAffinity), "Incorrect zkCluster affinity")
			Expect(zkCluster.Spec.Pod.Tolerations).To(Equal(testTolerations), "Incorrect zkCluster tolerations")
			Expect(zkCluster.Spec.Pod.NodeSelector).To(Equal(testNodeSelectors), "Incorrect zkCluster nodeSelectors")
			Expect(zkCluster.Spec.Pod.Resources).To(Equal(testResources), "Incorrect zkCluster resources")
			Expect(zkCluster.Spec.Pod.Env).To(Equal(extraVars), "Incorrect zkCluster env vars")
			Expect(zkCluster.Spec.Pod.ServiceAccountName).To(Equal(testServiceAccountName), "Incorrect zkCluster serviceAccountName")
			Expect(zkCluster.Spec.Pod.Labels).To(Equal(util.MergeLabelsOrAnnotations(testSSLabels, map[string]string{"app": "foo-solrcloud-zookeeper", "release": "foo-solrcloud-zookeeper"})), "Incorrect zkCluster pod labels")
			Expect(zkCluster.Spec.Pod.Annotations).To(Equal(testSSAnnotations), "Incorrect zkCluster pod annotations")
			Expect(zkCluster.Spec.Pod.SecurityContext).To(Equal(&testPodSecurityContext), "Incorrect zkCluster pod securityContext")
			Expect(zkCluster.Spec.Pod.TerminationGracePeriodSeconds).To(Equal(testTerminationGracePeriodSeconds), "Incorrect zkCluster pod terminationGracePeriodSeconds")
			Expect(zkCluster.Spec.Pod.ImagePullSecrets).To(Equal(append(append(make([]corev1.LocalObjectReference, 0), testAdditionalImagePullSecrets...), corev1.LocalObjectReference{Name: testImagePullSecretName})), "Incorrect zkCluster imagePullSecrets")

			// Check ZK Config Options
			Expect(zkCluster.Spec.Conf.InitLimit).To(Equal(zkConf.InitLimit), "Incorrect zkCluster Config InitLimit")
			Expect(zkCluster.Spec.Conf.SyncLimit).To(Equal(zkConf.SyncLimit), "Incorrect zkCluster Config SyncLimit")
			Expect(zkCluster.Spec.Conf.PreAllocSize).To(Equal(zkConf.PreAllocSize), "Incorrect zkCluster Config PreAllocSize")
			Expect(zkCluster.Spec.Conf.CommitLogCount).To(Equal(zkConf.CommitLogCount), "Incorrect zkCluster Config CommitLogCount")
			Expect(zkCluster.Spec.Conf.MaxCnxns).To(Equal(zkConf.MaxCnxns), "Incorrect zkCluster Config MaxCnxns")
			Expect(zkCluster.Spec.Conf.MinSessionTimeout).To(Equal(zkConf.MinSessionTimeout), "Incorrect zkCluster Config MinSessionTimeout")
			Expect(zkCluster.Spec.Conf.QuorumListenOnAllIPs).To(Equal(zkConf.QuorumListenOnAllIPs), "Incorrect zkCluster Config QuorumListenOnAllIPs")
		})
	})

	FContext("Provided ZK - Persistent", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas: &four,
						Image: &solrv1beta1.ContainerImage{
							Repository:      "test-repo",
							Tag:             "test-tag",
							PullPolicy:      corev1.PullNever,
							ImagePullSecret: testImagePullSecretName,
						},
						Persistence: &solrv1beta1.ZKPersistence{
							VolumeReclaimPolicy: solrv1beta1.VolumeReclaimPolicyRetain,
							Annotations:         testDeploymentAnnotations,
						},
					},
				},
				UpdateStrategy: solrv1beta1.SolrUpdateStrategy{
					Method: solrv1beta1.ManualUpdate,
				},
			}
		})
		FIt("has the correct resources", func() {
			expectedZkConnStr := "foo-solrcloud-zookeeper-0.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-solrcloud-zookeeper-1.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-solrcloud-zookeeper-2.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-solrcloud-zookeeper-3.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181"

			By("testing the ZK information in the SolrCloud status")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(expectedZkConnStr), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(BeNil(), "Since a provided zk is used, the externalConnectionString in the status should be Nil")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(BeNil(), "All ACL for provided Zookeeper in SolrCloud Status should be nil when not provided in Spec")
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(BeNil(), "ReadOnly ACL for provided Zookeeper in SolrCloud Status should be nil when not provided in Spec")
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			Expect(len(statefulSet.Spec.Template.Spec.Containers)).To(Equal(1), "Solr StatefulSet requires a container.")
			expectedZKHost := expectedZkConnStr + "/"
			expectedEnvVars := map[string]string{
				"ZK_HOST":   expectedZKHost,
				"SOLR_HOST": "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"ZK_SERVER": expectedZkConnStr,
				"SOLR_PORT": "8983",
				"GC_TUNE":   "",
			}
			expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: expectedZKHost}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
			Expect(statefulSet.Annotations).To(Equal(expectedStatefulSetAnnotations), "Wrong statefulSet annotations")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart).To(BeNil(), "No post-start command should be provided when no Chroot is used")
			Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(BeEmpty(), "No custom serviceAccountName specified, so the field should be empty.")

			// Check the update strategy
			Expect(statefulSet.Spec.UpdateStrategy.Type).To(Equal(appsv1.OnDeleteStatefulSetStrategyType), "Incorrect statefulset update strategy")
			Expect(statefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.ParallelPodManagement), "Incorrect statefulset pod management policy")

			By("testing the created ZookeeperCluster")
			zkCluster := expectZookeeperCluster(ctx, solrCloud, solrCloud.ProvidedZookeeperName())

			By("testing the created ZookeeperCluster")
			Expect(zkCluster.Spec.Replicas).To(Equal(four), "Incorrect zkCluster replicas")
			Expect(zkCluster.Spec.StorageType).To(Equal("persistence"), "Incorrect zkCluster storage type")
			Expect(zkCluster.Spec.Persistence).To(Not(BeNil()), "ZkCluster.spec.persistence should not be nil")
			Expect(zkCluster.Spec.Persistence.VolumeReclaimPolicy).To(BeEquivalentTo(solrv1beta1.VolumeReclaimPolicyRetain), "Incorrect VolumeReclaimPolicy for ZK Cluster persistent storage")
			Expect(zkCluster.Spec.Persistence.Annotations).To(Equal(testDeploymentAnnotations), "Incorrect Annotations for ZK Cluster persistent storage")
			Expect(zkCluster.Spec.Ephemeral).To(BeNil(), "ZkCluster.spec.ephemeral should be nil when using persistence")

			// Check ZK Pod Options
			Expect(zkCluster.Spec.Image.Repository).To(Equal("test-repo"), "Incorrect zkCluster image repo")
			Expect(zkCluster.Spec.Image.Tag).To(Equal("test-tag"), "Incorrect zkCluster image tag")
			Expect(zkCluster.Spec.Image.PullPolicy).To(Equal(corev1.PullNever), "Incorrect zkCluster image pull policy")
			Expect(zkCluster.Spec.Pod.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{{Name: testImagePullSecretName}}), "Incorrect zkCluster image pull policy")
		})
	})

	FContext("ZK Connection String - Admin ACL", func() {
		zkReplicas := int32(1)
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas: &zkReplicas,
						AllACL: &solrv1beta1.ZookeeperACL{
							SecretRef:   "secret-name",
							UsernameKey: "user",
							PasswordKey: "pass",
						},
						ZookeeperPod: solrv1beta1.ZookeeperPodPolicy{
							Env: extraVars,
						},
						ChRoot: "/a-ch/root",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
					},
				},
				SolrOpts: "-Dextra -Dopts",
			}
		})
		FIt("has the correct resources", func() {
			expectedZkConnStr := "foo-solrcloud-zookeeper-0.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181"

			By("testing the ZK information in the SolrCloud status")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(expectedZkConnStr), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/a-ch/root"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(BeNil(), "Since a provided zk is used, the externalConnectionString in the status should be Nil")
				/* Zookeeper Operator does not Support ACLs yet
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(Not(BeNil()), "All ACL in SolrCloud Status should not be nil when it is provided in the Spec")
				g.Expect(*found.ZookeeperConnectionInfo.AllACL).To(Equal(solrCloud.Spec.ZookeeperRef.ProvidedZookeeper.AllACL), "Incorrect All ACL in SolrCloud Status")
				g.Expect(found.ZookeeperConnectionInfo.ReadOnlyACL).To(BeNil(), "ReadOnly ACL in SolrCloud Status should be nil when not provided in the Spec")
				*/
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Check extra containers
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires the solr container only.")

			// Env Variable Tests
			expectedZKHost := expectedZkConnStr + "/a-ch/root"
			expectedEnvVars := map[string]string{
				"ZK_HOST":        expectedZKHost,
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"ZK_CHROOT":      "/a-ch/root",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_CREDS_AND_ACLS) -Dextra -Dopts",
			}
			insertExpectedAclEnvVars(expectedEnvVars, false)
			for _, envVar := range extraVars {
				expectedEnvVars[envVar.Name] = envVar.Value
			}
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			testPodEnvVariables(expectedEnvVars, foundEnv)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), "Incorrect pre-stop command")

			By("testing the created ZookeeperCluster")
			expectZookeeperCluster(ctx, solrCloud, solrCloud.ProvidedZookeeperName())
		})
	})

	FContext("ZK Connection String - Admin & Read ACL", func() {
		zkReplicas := int32(1)
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas: &zkReplicas,
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
						ZookeeperPod: solrv1beta1.ZookeeperPodPolicy{
							Env: extraVars,
						},
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
					},
				},
				SolrOpts: "-Dextra -Dopts",
			}
		})
		FIt("has the correct resources", func() {
			expectedZkConnStr := "foo-solrcloud-zookeeper-0.foo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181"

			By("testing the ZK information in the SolrCloud status")
			expectSolrCloudStatusWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloudStatus) {
				g.Expect(found.ZookeeperConnectionInfo.InternalConnectionString).To(Equal(expectedZkConnStr), "Wrong internal zkConnectionString in status")
				g.Expect(found.ZookeeperConnectionInfo.ChRoot).To(Equal("/"), "Wrong zk chRoot in status")
				g.Expect(found.ZookeeperConnectionInfo.ExternalConnectionString).To(BeNil(), "Since a provided zk is used, the externalConnectionString in the status should be Nil")

				/* Zookeeper Operator does not Support ACLs yet
				g.Expect(found.ZookeeperConnectionInfo.AllACL).To(Not(BeNil()), "All ACL in SolrCloud Status should not be nil when it is provided in the Spec")
				g.Expect(*found.ZookeeperConnectionInfo.AllACL).To(Equal(solrCloud.Spec.ZookeeperRef.ProvidedZookeeper.AllACL), "Incorrect All ACL in SolrCloud Status")
				g.Expect(found.ZookeeperConnectionInfo.ReadOnlyACL).To(Not(BeNil()), "ReadOnly ACL in SolrCloud Status should not be nil when it is provided in the Spec")
				g.Expect(*found.ZookeeperConnectionInfo.ReadOnlyACL).To(Equal(solrCloud.Spec.ZookeeperRef.ProvidedZookeeper.ReadOnlyACL), "Incorrect ReadOnly ACL in SolrCloud Status")
				*/
			})

			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Check extra containers
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires the solr container only.")

			// Env Variable Tests
			expectedZKHost := expectedZkConnStr + "/"
			expectedEnvVars := map[string]string{
				"ZK_HOST":        expectedZKHost,
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_CREDS_AND_ACLS) -Dextra -Dopts",
			}
			insertExpectedAclEnvVars(expectedEnvVars, true)
			for _, envVar := range extraVars {
				expectedEnvVars[envVar.Name] = envVar.Value
			}
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			testPodEnvVariables(expectedEnvVars, foundEnv)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), "Incorrect pre-stop command")

			By("testing the created ZookeeperCluster")
			expectZookeeperCluster(ctx, solrCloud, solrCloud.ProvidedZookeeperName())
		})
	})

	FContext("Solr Cloud with Solr ZK Connection Options", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
						ChRoot:                   "/test",
					},
				},
				SolrZkOpts: testSolrZKOpts,
				SolrOpts:   testSolrOpts,
				SolrSecurity: &solrv1beta1.SolrSecurityOptions{
					AuthenticationType: solrv1beta1.Basic,
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr StatefulSet")
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1), "Solr StatefulSet requires a container.")
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/test",
				"SOLR_HOST":      "$(POD_HOSTNAME).foo-solrcloud-headless.default",
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"SOLR_ZK_OPTS":   testSolrZKOpts,
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_OPTS) " + testSolrOpts,
				"SOLR_STOP_WAIT": strconv.FormatInt(60-5, 10),
			}
			testPodEnvVariables(expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)

			expectedInitContainerEnvVars := map[string]string{
				"SOLR_ZK_OPTS":    testSolrZKOpts,
				"SOLR_OPTS":       "$(SOLR_ZK_OPTS) " + testSolrOpts,
				"ZKCLI_JVM_FLAGS": "-Dsolr.zk.opts=this",
			}
			testPodEnvVariables(expectedInitContainerEnvVars, statefulSet.Spec.Template.Spec.InitContainers[1].Env)
		})
	})
})

func expectZookeeperCluster(ctx context.Context, parentResource client.Object, zkName string, additionalOffset ...int) *zk_crd.ZookeeperCluster {
	return expectZookeeperClusterWithChecks(ctx, parentResource, zkName, nil, resolveOffset(additionalOffset))
}

func expectZookeeperClusterWithChecks(ctx context.Context, parentResource client.Object, zkName string, additionalChecks func(Gomega, *zk_crd.ZookeeperCluster), additionalOffset ...int) *zk_crd.ZookeeperCluster {
	found := &zk_crd.ZookeeperCluster{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, zkName), found)).To(Succeed(), "Expected ZookeeperCluster does not exist")
		if additionalChecks != nil {
			additionalChecks(g, found)
		}
	}).Should(Succeed())

	By("recreating the ZookeeperCluster after it is deleted")
	ExpectWithOffset(resolveOffset(additionalOffset), k8sClient.Delete(ctx, found)).To(Succeed())
	Eventually(
		func(g Gomega) types.UID {
			newResource := &zk_crd.ZookeeperCluster{}
			g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, zkName), newResource)).To(Succeed(), "ZookeeperCluster not recreated after deletion")
			return newResource.UID
		}).Should(And(Not(BeEmpty()), Not(Equal(found.UID))), "New ZookeeperCluster, with new UID, not created.")
	return found
}

func expectNoZookeeperCluster(ctx context.Context, parentResource client.Object, zkName string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, zkName), &zk_crd.ZookeeperCluster{})
	}).Should(MatchError("zookeeperclusters.zookeeper.pravega.io \""+zkName+"\" not found"), "ZookeeperCluster exists when it should not")
}
