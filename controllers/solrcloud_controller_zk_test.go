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
	"fmt"
	"github.com/apache/solr-operator/controllers/util"
	zkv1beta1 "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	"github.com/pravega/zookeeper-operator/pkg/controller/zookeepercluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"

	"github.com/stretchr/testify/assert"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &SolrCloudReconciler{}

func expectZookeeperCluster(g *gomega.GomegaWithT, requests chan reconcile.Request, expectedRequest reconcile.Request, zookeeperClusterKey types.NamespacedName) *zkv1beta1.ZookeeperCluster {
	zk := &zkv1beta1.ZookeeperCluster{}
	g.Eventually(func() error { return testClient.Get(context.TODO(), zookeeperClusterKey, zk) }, timeout).
		Should(gomega.Succeed())

	// Delete the ZK and expect Reconcile to be called for ZK deletion
	g.Expect(testClient.Delete(context.TODO(), zk)).NotTo(gomega.HaveOccurred())
	g.Eventually(func() error { return testClient.Get(context.TODO(), zookeeperClusterKey, zk) }, timeout).
		Should(gomega.Succeed())

	// Manually delete ZK since GC isn't enabled in the test control plane
	g.Eventually(func() error { return testClient.Delete(context.TODO(), zk) }, timeout).
		Should(gomega.MatchError("zookeeperclusters.zookeeper.pravega.io \"" + zookeeperClusterKey.Name + "\" not found"))

	return zk
}

func TestCloudWithProvidedEphemeralZookeeperReconcile(t *testing.T) {
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ProvidedZookeeper: &solr.ZookeeperSpec{
					Replicas: &four,
					Ephemeral: &zkv1beta1.Ephemeral{
						EmptyDirVolumeSource: corev1.EmptyDirVolumeSource{
							Medium: "Memory",
						},
					},
					ZookeeperPod: solr.ZookeeperPodPolicy{
						Affinity:           affinity,
						NodeSelector:       testNodeSelectors,
						Tolerations:        testTolerations,
						Env:                extraVars,
						Resources:          resources,
						ServiceAccountName: testServiceAccountName,
					},
					ChRoot: "a-ch/root",
				},
			},
			UpdateStrategy: solr.SolrUpdateStrategy{
				Method: solr.ManualUpdate,
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	// Include ZK Operator since we are creating a ZK Cluster
	g.Expect(zookeepercluster.AddZookeeperReconciler(mgr)).NotTo(gomega.HaveOccurred())

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the zkCluster will have been created
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Check that the ZkConnectionInformation is correct
	expectedZkConnStr := "foo-clo-solrcloud-zookeeper-0.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-clo-solrcloud-zookeeper-1.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-clo-solrcloud-zookeeper-2.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-clo-solrcloud-zookeeper-3.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181"
	assert.Equal(t, expectedZkConnStr, instance.Status.ZookeeperConnectionInfo.InternalConnectionString, "Wrong zkConnectionString in status")
	assert.Equal(t, "/a-ch/root", instance.Status.ZookeeperConnectionInfo.ChRoot, "Wrong zk chRoot in status")
	assert.Nil(t, instance.Status.ZookeeperConnectionInfo.ExternalConnectionString, "Since a provided zk is used, the externalConnectionString in the status should be Nil")

	// Check that the statefulSet has been created, using the given chRoot
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")
	expectedZKHost := expectedZkConnStr + "/a-ch/root"
	expectedEnvVars := map[string]string{
		"ZK_HOST":   expectedZKHost,
		"SOLR_HOST": "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + cloudHsKey.Namespace,
		"ZK_SERVER": expectedZkConnStr,
		"ZK_CHROOT": "/a-ch/root",
		"SOLR_PORT": "8983",
		"GC_TUNE":   "",
	}
	expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: expectedZKHost}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	testMapsEqual(t, "statefulSet annotations", expectedStatefulSetAnnotations, statefulSet.Annotations)
	assert.EqualValues(t, []string{"sh", "-c", "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart.Exec.Command, "Incorrect post-start command")
	assert.Empty(t, statefulSet.Spec.Template.Spec.ServiceAccountName, "No custom serviceAccountName specified, so the field should be empty.")

	// Check the update strategy
	assert.EqualValues(t, appsv1.OnDeleteStatefulSetStrategyType, statefulSet.Spec.UpdateStrategy.Type, "Incorrect statefulset update strategy")
	assert.EqualValues(t, appsv1.ParallelPodManagement, statefulSet.Spec.PodManagementPolicy, "Incorrect statefulset pod management policy")

	// Check that the Zookeeper Cluster has been created
	zkCluster := expectZookeeperCluster(g, requests, expectedCloudRequest, cloudZkKey)

	// Check the settings of the Zookeeper Cluster
	assert.EqualValues(t, four, zkCluster.Spec.Replicas, "Incorrect zkCluster replicas")
	assert.EqualValues(t, "ephemeral", zkCluster.Spec.StorageType, "Incorrect zkCluster storage type")
	assert.NotNil(t, zkCluster.Spec.Ephemeral, "ZkCluster.spec.ephemeral should not be nil")
	assert.EqualValues(t, "Memory", zkCluster.Spec.Ephemeral.EmptyDirVolumeSource.Medium, "Incorrect EmptyDir medium for ZK Cluster ephemeral storage")

	// Check ZK Pod Options
	assert.EqualValues(t, affinity, zkCluster.Spec.Pod.Affinity, "Incorrect zkCluster affinity")
	assert.EqualValues(t, testTolerations, zkCluster.Spec.Pod.Tolerations, "Incorrect zkCluster tolerations")
	assert.EqualValues(t, testNodeSelectors, zkCluster.Spec.Pod.NodeSelector, "Incorrect zkCluster nodeSelectors")
	assert.EqualValues(t, resources, zkCluster.Spec.Pod.Resources, "Incorrect zkCluster resources")
	assert.EqualValues(t, extraVars, zkCluster.Spec.Pod.Env, "Incorrect zkCluster env vars")
	assert.EqualValues(t, testServiceAccountName, zkCluster.Spec.Pod.ServiceAccountName, "Incorrect zkCluster serviceAccountName")
}

func TestCloudWithProvidedPersistentZookeeperReconcile(t *testing.T) {
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ProvidedZookeeper: &solr.ZookeeperSpec{
					Replicas: &four,
					Image: &solr.ContainerImage{
						Repository: "test-repo",
						Tag:        "test-tag",
						PullPolicy: corev1.PullNever,
						// TODO: Test when ZK operator v0.2.10 is supported
						ImagePullSecret: testImagePullSecretName,
					},
					Persistence: &zkv1beta1.Persistence{
						VolumeReclaimPolicy: zkv1beta1.VolumeReclaimPolicyRetain,
					},
				},
			},
			UpdateStrategy: solr.SolrUpdateStrategy{
				Method: solr.ManualUpdate,
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	// Include ZK Operator since we are creating a ZK Cluster
	g.Expect(zookeepercluster.AddZookeeperReconciler(mgr)).NotTo(gomega.HaveOccurred())

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the zkCluster will have been created
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	g.Eventually(func() error { return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance) }, timeout).Should(gomega.Succeed())

	// Check that the ZkConnectionInformation is correct
	expectedZkConnStr := "foo-clo-solrcloud-zookeeper-0.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-clo-solrcloud-zookeeper-1.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-clo-solrcloud-zookeeper-2.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181,foo-clo-solrcloud-zookeeper-3.foo-clo-solrcloud-zookeeper-headless.default.svc.cluster.local:2181"
	assert.Equal(t, expectedZkConnStr, instance.Status.ZookeeperConnectionInfo.InternalConnectionString, "Wrong zkConnectionString in status")
	assert.Equal(t, "/", instance.Status.ZookeeperConnectionInfo.ChRoot, "Wrong zk chRoot in status")
	assert.Nil(t, instance.Status.ZookeeperConnectionInfo.ExternalConnectionString, "Since a provided zk is used, the externalConnectionString in the status should be Nil")

	// Check that the statefulSet has been created, using the given chRoot
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")
	expectedZKHost := expectedZkConnStr + "/"
	expectedEnvVars := map[string]string{
		"ZK_HOST":   expectedZKHost,
		"SOLR_HOST": "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + cloudHsKey.Namespace,
		"ZK_SERVER": expectedZkConnStr,
		"ZK_CHROOT": "/",
		"SOLR_PORT": "8983",
		"GC_TUNE":   "",
	}
	expectedStatefulSetAnnotations := map[string]string{util.SolrZKConnectionStringAnnotation: expectedZKHost}
	testPodEnvVariables(t, expectedEnvVars, statefulSet.Spec.Template.Spec.Containers[0].Env)
	testMapsEqual(t, "statefulSet annotations", expectedStatefulSetAnnotations, statefulSet.Annotations)
	assert.Nil(t, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PostStart, "chroot is empty, so there should be no postStart command")

	// Check the update strategy
	assert.EqualValues(t, appsv1.OnDeleteStatefulSetStrategyType, statefulSet.Spec.UpdateStrategy.Type, "Incorrect statefulset update strategy")
	assert.EqualValues(t, appsv1.ParallelPodManagement, statefulSet.Spec.PodManagementPolicy, "Incorrect statefulset pod management policy")

	// Check that the Zookeeper Cluster has been created
	zkCluster := expectZookeeperCluster(g, requests, expectedCloudRequest, cloudZkKey)

	// Check the settings of the Zookeeper Cluster
	assert.EqualValues(t, four, zkCluster.Spec.Replicas, "Incorrect zkCluster replicas")
	assert.EqualValues(t, "persistence", zkCluster.Spec.StorageType, "Incorrect zkCluster storage type")
	assert.NotNil(t, zkCluster.Spec.Persistence, "ZkCluster.spec.persistence should not be nil")
	assert.EqualValues(t, zkv1beta1.VolumeReclaimPolicyRetain, zkCluster.Spec.Persistence.VolumeReclaimPolicy, "Incorrect VolumeReclaimPolicy for ZK Cluster persistent storage")

	// Check ZK Pod Options
	assert.EqualValues(t, "test-repo", zkCluster.Spec.Image.Repository, "Incorrect zkCluster image repo")
	assert.EqualValues(t, "test-tag", zkCluster.Spec.Image.Tag, "Incorrect zkCluster image tag")
	assert.EqualValues(t, corev1.PullNever, zkCluster.Spec.Image.PullPolicy, "Incorrect zkCluster image pull policy")
}

func TestZKACLsCloudReconcile(t *testing.T) {
	UseZkCRD(false)
	g := gomega.NewGomegaWithT(t)

	replicas := int32(3)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			Replicas: &replicas,
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
					AllACL: &solr.ZookeeperACL{
						SecretRef:   "secret-name",
						UsernameKey: "user",
						PasswordKey: "pass",
					},
				},
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables: extraVars,
				},
			},
			SolrOpts: "-Dextra -Dopts",
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the services will have IP addresses for the hostAliases to use
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        "host:7271/",
		"SOLR_HOST":      "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + instance.Namespace,
		"SOLR_PORT":      "8983",
		"SOLR_NODE_PORT": "8983",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_CREDS_AND_ACLS) -Dextra -Dopts",
	}
	foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
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
			Name:      "SOLR_ZK_CREDS_AND_ACLS",
			Value:     "-DzkACLProvider=org.apache.solr.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solr.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider -DzkDigestUsername=$(ZK_ALL_ACL_USERNAME) -DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD)",
			ValueFrom: nil,
		},
	}
	assert.Equal(t, zkAclEnvVars, foundEnv[len(foundEnv)-6:len(foundEnv)-3], "ZK ACL Env Vars are not correct")
	assert.Equal(t, extraVars, foundEnv[len(foundEnv)-3:len(foundEnv)-1], "Extra Env Vars are not the same as the ones provided in podOptions")
	// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
	// TODO: Make this not invalidate foundEnv
	testPodEnvVariables(t, expectedEnvVars, append(foundEnv[:len(foundEnv)-6], foundEnv[len(foundEnv)-1]))
	assert.Equal(t, []string{"solr", "stop", "-p", "8983"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	assert.EqualValues(t, 80, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	assert.EqualValues(t, 8983, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)
}

func TestBothZKACLsCloudReconcile(t *testing.T) {
	replicas := int32(3)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			Replicas: &replicas,
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
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables: extraVars,
				},
			},
			SolrOpts: "-Dextra -Dopts",
		},
	}
	testZkACLsReconcile(t, false, instance, "host:7271/")
}

func TestZKACLsForProvidedReconcile(t *testing.T) {
	replicas := int32(3)
	zkReplicas := int32(1)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			Replicas: &replicas,
			ZookeeperRef: &solr.ZookeeperRef{
				ProvidedZookeeper: &solr.ZookeeperSpec{
					Replicas: &zkReplicas,
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
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables: extraVars,
				},
			},
			SolrOpts: "-Dextra -Dopts",
		},
	}

	expectedZkHost := fmt.Sprintf("%s-zookeeper-0.%s-zookeeper-headless.%s.svc.cluster.local:2181/",
		cloudSsKey.Name, cloudSsKey.Name, cloudSsKey.Namespace)
	testZkACLsReconcile(t, true, instance, expectedZkHost)
}

func testZkACLsReconcile(t *testing.T, useZkCRD bool, instance *solr.SolrCloud, expectedZkHost string) {
	UseZkCRD(useZkCRD)
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(testCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	testClient = mgr.GetClient()

	solrCloudReconciler := &SolrCloudReconciler{
		Client: testClient,
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}
	newRec, requests := SetupTestReconcile(solrCloudReconciler)
	g.Expect(solrCloudReconciler.SetupWithManagerAndReconciler(mgr, newRec)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	cleanupTest(g, instance.Namespace)

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = testClient.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer testClient.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	// Add an additional check for reconcile, so that the services will have IP addresses for the hostAliases to use
	// Otherwise the reconciler will have 'blockReconciliationOfStatefulSet' set to true, and the stateful set will not be created
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 1, len(statefulSet.Spec.Template.Spec.Containers), "Solr StatefulSet requires a container.")

	// Env Variable Tests
	expectedEnvVars := map[string]string{
		"ZK_HOST":        expectedZkHost,
		"SOLR_HOST":      "$(POD_HOSTNAME)." + cloudHsKey.Name + "." + instance.Namespace,
		"SOLR_PORT":      "8983",
		"SOLR_NODE_PORT": "8983",
		"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT) $(SOLR_ZK_CREDS_AND_ACLS) -Dextra -Dopts",
	}
	foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
	testACLEnvVars(t, foundEnv[len(foundEnv)-8:len(foundEnv)-3])
	assert.Equal(t, extraVars, foundEnv[len(foundEnv)-3:len(foundEnv)-1], "Extra Env Vars are not the same as the ones provided in podOptions")
	// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
	// TODO: Make this not invalidate foundEnv
	testPodEnvVariables(t, expectedEnvVars, append(foundEnv[:len(foundEnv)-8], foundEnv[len(foundEnv)-1]))
	assert.Equal(t, []string{"solr", "stop", "-p", "8983"}, statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command, "Incorrect pre-stop command")

	// Check the client Service
	service := expectService(t, g, requests, expectedCloudRequest, cloudCsKey, statefulSet.Spec.Template.Labels)
	assert.EqualValues(t, 80, service.Spec.Ports[0].Port, "Wrong port on common Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on common Service")

	// Check the headless Service
	service = expectService(t, g, requests, expectedCloudRequest, cloudHsKey, statefulSet.Spec.Template.Labels)
	assert.EqualValues(t, 8983, service.Spec.Ports[0].Port, "Wrong port on headless Service")
	assert.EqualValues(t, "solr-client", service.Spec.Ports[0].TargetPort.StrVal, "Wrong podPort name on headless Service")

	// Make sure individual Node services don't exist
	nodeNames := instance.GetAllSolrNodeNames()
	for _, nodeName := range nodeNames {
		nodeSKey := types.NamespacedName{Name: nodeName, Namespace: "default"}
		expectNoService(g, nodeSKey, "Node service shouldn't exist, but it does.")
	}

	// Check the ingress
	expectNoIngress(g, cloudIKey)

	// now update the env vars on the zk spec
	if instance.Spec.ZookeeperRef.ProvidedZookeeper != nil {
		err = testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		updateEnvVars := []corev1.EnvVar{
			{
				Name:  "VAR_1",
				Value: "VAL_1",
			},
		}
		instance.Spec.ZookeeperRef.ProvidedZookeeper.ZookeeperPod = solr.ZookeeperPodPolicy{Env: updateEnvVars}
		err = testClient.Update(context.TODO(), instance)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
		lookup := &solr.SolrCloud{}
		err = testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, lookup)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		assert.EqualValues(t, updateEnvVars, lookup.Spec.ZookeeperRef.ProvidedZookeeper.ZookeeperPod.Env, "Updated ZK Pod env not reconciled!")
	}
}
