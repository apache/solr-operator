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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("SolrCloud controller - Storage", func() {
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
			Spec: solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				SolrJavaMem: "-Xmx4G",
				SolrOpts:    "extra-opts",
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables:       extraVars,
						PodSecurityContext: &testPodSecurityContext,
						Volumes:            extraVolumes,
						Affinity:           testAffinity,
						Resources:          testResources,
					},
				},
			},
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

	FContext("Persistent Data - Custom name & labels", func() {
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				PersistentStorage: &solrv1beta1.SolrPersistentDataStorageOptions{
					VolumeReclaimPolicy: solrv1beta1.VolumeReclaimPolicyRetain,
					PersistentVolumeClaimTemplate: solrv1beta1.PersistentVolumeClaimTemplate{
						ObjectMeta: solrv1beta1.TemplateMeta{
							Name:   "other-data-1",
							Labels: map[string]string{"base": "here"},
						},
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithConsistentChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(0), "The solrcloud should have no finalizers when the reclaim policy is Retain")
			})

			By("testing the Solr StatefulSet PVC Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(2), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Name).To(Equal(solrCloud.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name), "Data volume claim doesn't exist")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCTechnologyLabel]).To(Equal("solr-cloud"), "PVC Technology label doesn't match")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCStorageLabel]).To(Equal("data"), "PVC Storage label doesn't match")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCInstanceLabel]).To(Equal(solrCloud.Name), "PVC Instance label doesn't match")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels["base"]).To(Equal("here"), "Additional PVC label doesn't match")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(found.Spec.VolumeClaimTemplates[0].Name), "Custom PVC name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(found.Spec.VolumeClaimTemplates[0].Name), "Custom PVC name not used in volume mount")
			})
		})
	})

	FContext("Persistent Data - PVC Delete on Reclaim", func() {
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				PersistentStorage: &solrv1beta1.SolrPersistentDataStorageOptions{
					VolumeReclaimPolicy: solrv1beta1.VolumeReclaimPolicyDelete,
					PersistentVolumeClaimTemplate: solrv1beta1.PersistentVolumeClaimTemplate{
						ObjectMeta: solrv1beta1.TemplateMeta{
							Name:   "other-data-2",
							Labels: map[string]string{"base": "here"},
						},
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(1), "The solrcloud should have no finalizers when the reclaim policy is Retain")
				g.Expect(found.GetFinalizers()[0]).To(Equal(util.SolrStorageFinalizer), "Incorrect finalizer set for deleting persistent storage.")
			})

			By("testing the Solr StatefulSet PVC Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(2), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Name).To(Equal(solrCloud.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name), "Data volume claim doesn't exist")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCTechnologyLabel]).To(Equal("solr-cloud"), "PVC Technology label doesn't match")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCStorageLabel]).To(Equal("data"), "PVC Storage label doesn't match")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels[util.SolrPVCInstanceLabel]).To(Equal(solrCloud.Name), "PVC Instance label doesn't match")
				g.Expect(found.Spec.VolumeClaimTemplates[0].Labels["base"]).To(Equal("here"), "Additional PVC label doesn't match")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(found.Spec.VolumeClaimTemplates[0].Name), "Custom PVC name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(found.Spec.VolumeClaimTemplates[0].Name), "Custom PVC name not used in volume mount")
			})

			By("ensuring that the PVCs are deleted and the finalizer is removed when the SolrCloud is deleted")
			Expect(k8sClient.Delete(ctx, solrCloud)).To(Succeed(), "Could not delete solrCloud")

			Eventually(func() error {
				return k8sClient.Get(ctx, resourceKey(solrCloud, solrCloud.Name), &solrv1beta1.SolrCloud{})
			}).Should(MatchError("solrclouds.solr.apache.org \""+solrCloud.Name+"\" not found"), "Cloud has not been deleted, thus the finalizers have not been removed from the object.")
		})
	})

	FContext("Ephemeral Data - Default", func() {
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				EphemeralStorage: &solrv1beta1.SolrEphemeralDataStorageOptions{},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithConsistentChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(0), "The solrcloud should have no finalizers when ephemeral storage is used")
			})

			By("testing the Solr StatefulSet Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(3), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates).To(HaveLen(0), "No data volume claims should exist when using ephemeral storage")
				dataVolume := found.Spec.Template.Spec.Volumes[1]
				g.Expect(dataVolume.EmptyDir).To(Not(BeNil()), "The data volume should be an empty-dir.")
				g.Expect(dataVolume.HostPath).To(BeNil(), "The data volume should not be a hostPath volume.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
			})
		})
	})

	FContext("Ephemeral Data - Nil EmptyDir", func() {
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				EphemeralStorage: &solrv1beta1.SolrEphemeralDataStorageOptions{
					EmptyDir: nil,
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithConsistentChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(0), "The solrcloud should have no finalizers when ephemeral storage is used")
			})

			By("testing the Solr StatefulSet Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(3), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates).To(HaveLen(0), "No data volume claims should exist when using ephemeral storage")
				dataVolume := found.Spec.Template.Spec.Volumes[1]
				g.Expect(dataVolume.EmptyDir).To(Not(BeNil()), "The data volume should be an empty-dir.")
				g.Expect(dataVolume.HostPath).To(BeNil(), "The data volume should not be a hostPath volume.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
			})
		})
	})

	FContext("Ephemeral Data - EmptyDir with Specs", func() {
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				EphemeralStorage: &solrv1beta1.SolrEphemeralDataStorageOptions{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium:    corev1.StorageMediumMemory,
						SizeLimit: resource.NewQuantity(1028*1028*1028, resource.BinarySI),
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(0), "The solrcloud should have no finalizers when ephemeral storage is used")
			})

			By("testing the Solr StatefulSet Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(3), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates).To(HaveLen(0), "No data volume claims should exist when using ephemeral storage")
				dataVolume := found.Spec.Template.Spec.Volumes[1]
				g.Expect(dataVolume.EmptyDir).To(Not(BeNil()), "The data volume should be an empty-dir.")
				g.Expect(dataVolume.HostPath).To(BeNil(), "The data volume should not be a hostPath volume.")
				g.Expect(dataVolume.EmptyDir).To(Equal(solrCloud.Spec.StorageOptions.EphemeralStorage.EmptyDir), "The empty dir settings do not match with what was provided.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
			})
		})
	})

	FContext("Ephemeral Data - HostPath with Specs", func() {
		hostPathType := corev1.HostPathDirectoryOrCreate
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				EphemeralStorage: &solrv1beta1.SolrEphemeralDataStorageOptions{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp",
						Type: &hostPathType,
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithConsistentChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(0), "The solrcloud should have no finalizers when ephemeral storage is used")
			})

			By("testing the Solr StatefulSet Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(3), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates).To(HaveLen(0), "No data volume claims should exist when using ephemeral storage")
				dataVolume := found.Spec.Template.Spec.Volumes[1]
				g.Expect(dataVolume.EmptyDir).To(BeNil(), "The data volume should not be an empty-dir.")
				g.Expect(dataVolume.HostPath).To(Not(BeNil()), "The data volume should be a hostPath volume.")
				g.Expect(dataVolume.HostPath).To(Equal(solrCloud.Spec.StorageOptions.EphemeralStorage.HostPath), "The hostPath  settings do not match with what was provided.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
			})
		})
	})

	FContext("Ephemeral Data - HostPath and EmptyDir (HostPath should win)", func() {
		hostPathType := corev1.HostPathDirectoryOrCreate
		BeforeEach(func() {
			solrCloud.Spec.StorageOptions = solrv1beta1.SolrDataStorageOptions{
				EphemeralStorage: &solrv1beta1.SolrEphemeralDataStorageOptions{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp",
						Type: &hostPathType,
					},
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium:    corev1.StorageMediumMemory,
						SizeLimit: resource.NewQuantity(1028*1028*1028, resource.BinarySI),
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the SolrCloud finalizers")
			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				g.Expect(found.GetFinalizers()).To(HaveLen(0), "The solrcloud should have no finalizers when ephemeral storage is used")
			})

			By("testing the Solr StatefulSet Spec")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(3), "Pod has wrong number of volumes")
				g.Expect(found.Spec.VolumeClaimTemplates).To(HaveLen(0), "No data volume claims should exist when using ephemeral storage")
				dataVolume := found.Spec.Template.Spec.Volumes[1]
				g.Expect(dataVolume.EmptyDir).To(BeNil(), "The data volume should not be an empty-dir.")
				g.Expect(dataVolume.HostPath).To(Not(BeNil()), "The data volume should be a hostPath volume.")
				g.Expect(dataVolume.HostPath).To(Equal(solrCloud.Spec.StorageOptions.EphemeralStorage.HostPath), "The hostPath  settings do not match with what was provided.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
				g.Expect(found.Spec.Template.Spec.InitContainers[0].VolumeMounts[1].Name).To(Equal(dataVolume.Name), "Ephemeral Data volume name not used in volume mount")
			})
		})
	})
})
