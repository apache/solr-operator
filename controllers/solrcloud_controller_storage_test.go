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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"

	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/stretchr/testify/assert"

	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &SolrCloudReconciler{}

func TestPersistentStorageVolumesRetain(t *testing.T) {
	SetIngressBaseUrl("")
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrJavaMem:  "-Xmx4G",
			SolrOpts:     "extra-opts",
			SolrLogLevel: "DEBUG",
			StorageOptions: solr.SolrDataStorageOptions{
				PersistentStorage: &solr.SolrPersistentDataStorageOptions{
					VolumeReclaimPolicy: solr.VolumeReclaimPolicyRetain,
					PersistentVolumeClaimTemplate: solr.PersistentVolumeClaimTemplate{
						ObjectMeta: solr.TemplateMeta{
							Name:   "other-data",
							Labels: map[string]string{"base": "here"},
						},
					},
				},
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables:       extraVars,
					PodSecurityContext: &podSecurityContext,
					Volumes:            extraVolumes,
					Affinity:           affinity,
					Resources:          resources,
				},
			},
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

	// Fetch new value of instance to check finalizers
	foundInstance := &solr.SolrCloud{}
	g.Eventually(func() error {
		return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, foundInstance)
	}, timeout).Should(gomega.Succeed())
	assert.Equal(t, 0, len(foundInstance.GetFinalizers()), "The solrcloud should have no finalizers when the reclaim policy is Retain")

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 2, len(statefulSet.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, instance.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name, statefulSet.Spec.VolumeClaimTemplates[0].Name, "Data volume claim doesn't exist")
	assert.Equal(t, "solr-cloud", statefulSet.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCTechnologyLabel], "PVC Technology label doesn't match")
	assert.Equal(t, "data", statefulSet.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCStorageLabel], "PVC Storage label doesn't match")
	assert.Equal(t, "foo-clo", statefulSet.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCInstanceLabel], "PVC Instance label doesn't match")
	assert.Equal(t, "here", statefulSet.Spec.VolumeClaimTemplates[0].Labels["base"], "Additional PVC label doesn't match")
}

func TestPersistentStorageVolumesDelete(t *testing.T) {
	SetIngressBaseUrl("")
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrJavaMem:  "-Xmx4G",
			SolrOpts:     "extra-opts",
			SolrLogLevel: "DEBUG",
			StorageOptions: solr.SolrDataStorageOptions{
				PersistentStorage: &solr.SolrPersistentDataStorageOptions{
					VolumeReclaimPolicy: solr.VolumeReclaimPolicyDelete,
					PersistentVolumeClaimTemplate: solr.PersistentVolumeClaimTemplate{
						ObjectMeta: solr.TemplateMeta{
							Name:        "other-data",
							Annotations: map[string]string{"base": "here"},
						},
					},
				},
				EphemeralStorage: &solr.SolrEphemeralDataStorageOptions{},
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables:       extraVars,
					PodSecurityContext: &podSecurityContext,
					Volumes:            extraVolumes,
					Affinity:           affinity,
					Resources:          resources,
				},
			},
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
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)))

	// Fetch new value of instance to check finalizers
	foundInstance := &solr.SolrCloud{}
	g.Eventually(func() error {
		return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, foundInstance)
	}, timeout).Should(gomega.Succeed())
	assert.Equal(t, 1, len(foundInstance.GetFinalizers()), "The solrcloud should have 1 storage finalizer when persistent storage reclaim policy is set to Delete")
	assert.Equal(t, util.SolrStorageFinalizer, foundInstance.GetFinalizers()[0], "Incorrect finalizer set for deleting persistent storage.")

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 2, len(statefulSet.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, instance.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name, statefulSet.Spec.VolumeClaimTemplates[0].Name, "Data volume claim doesn't exist")
	assert.Equal(t, "solr-cloud", statefulSet.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCTechnologyLabel], "PVC Technology label doesn't match")
	assert.Equal(t, "data", statefulSet.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCStorageLabel], "PVC Storage label doesn't match")
	assert.Equal(t, "foo-clo", statefulSet.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCInstanceLabel], "PVC Instance label doesn't match")
	assert.Equal(t, "here", statefulSet.Spec.VolumeClaimTemplates[0].Annotations["base"], "Additional PVC label doesn't match")

	// Explicitly delete, make sure that finalizers are removed from the object so that kubernetes can delete it.
	testClient.Delete(context.TODO(), instance)
	for i := 0; i < 5; i++ {
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCloudRequest)), "Cloud has not been deleted, thus the finalizers have not been removed from the object.")
		err = testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, instance)
		if err != nil {
			break
		}
	}
	g.Expect(err).To(gomega.HaveOccurred(), "Cloud has not been deleted, thus the finalizers have not been removed from the object.")
}

func TestDefaultEphemeralStorage(t *testing.T) {
	SetIngressBaseUrl("")
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrJavaMem:  "-Xmx4G",
			SolrOpts:     "extra-opts",
			SolrLogLevel: "DEBUG",
			StorageOptions: solr.SolrDataStorageOptions{
				EphemeralStorage: &solr.SolrEphemeralDataStorageOptions{},
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables:       extraVars,
					PodSecurityContext: &podSecurityContext,
					Volumes:            extraVolumes,
					Affinity:           affinity,
					Resources:          resources,
				},
			},
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

	// Fetch new value of instance to check finalizers
	foundInstance := &solr.SolrCloud{}
	g.Eventually(func() error {
		return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, foundInstance)
	}, timeout).Should(gomega.Succeed())
	assert.Equal(t, 0, len(foundInstance.GetFinalizers()), "The solrcloud should have no finalizers when ephemeral storage is used")

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 3, len(statefulSet.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, 0, len(statefulSet.Spec.VolumeClaimTemplates), "No data volume claims should exist when using ephemeral storage")
	dataVolume := statefulSet.Spec.Template.Spec.Volumes[1]
	assert.NotNil(t, dataVolume.EmptyDir, "The data volume should be an empty-dir.")
}

func TestEphemeralStorageWithSpecs(t *testing.T) {
	SetIngressBaseUrl("")
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)

	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: expectedCloudRequest.Name, Namespace: expectedCloudRequest.Namespace},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
			SolrJavaMem:  "-Xmx4G",
			SolrOpts:     "extra-opts",
			SolrLogLevel: "DEBUG",
			StorageOptions: solr.SolrDataStorageOptions{
				EphemeralStorage: &solr.SolrEphemeralDataStorageOptions{
					EmptyDir: corev1.EmptyDirVolumeSource{
						Medium:    corev1.StorageMediumMemory,
						SizeLimit: resource.NewQuantity(1028*1028*1028, resource.BinarySI),
					},
				},
			},
			CustomSolrKubeOptions: solr.CustomSolrKubeOptions{
				PodOptions: &solr.PodOptions{
					EnvVariables:       extraVars,
					PodSecurityContext: &podSecurityContext,
					Volumes:            extraVolumes,
					Affinity:           affinity,
					Resources:          resources,
				},
			},
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

	// Fetch new value of instance to check finalizers
	foundInstance := &solr.SolrCloud{}
	g.Eventually(func() error {
		return testClient.Get(context.TODO(), expectedCloudRequest.NamespacedName, foundInstance)
	}, timeout).Should(gomega.Succeed())
	assert.Equal(t, 0, len(foundInstance.GetFinalizers()), "The solrcloud should have no finalizers when ephemeral storage is used")

	// Check the statefulSet
	statefulSet := expectStatefulSet(t, g, requests, expectedCloudRequest, cloudSsKey)

	assert.Equal(t, 3, len(statefulSet.Spec.Template.Spec.Volumes), "Pod has wrong number of volumes")
	assert.Equal(t, 0, len(statefulSet.Spec.VolumeClaimTemplates), "No data volume claims should exist when using ephemeral storage")
	dataVolume := statefulSet.Spec.Template.Spec.Volumes[1]
	assert.NotNil(t, dataVolume.EmptyDir, "The data volume should be an empty-dir.")
	assert.EqualValues(t, instance.Spec.StorageOptions.EphemeralStorage.EmptyDir, *dataVolume.EmptyDir, "The empty dir settings do not match with what was provided.")
}
