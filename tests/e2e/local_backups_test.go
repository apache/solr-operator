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

package e2e

import (
	"context"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"strings"
	"time"
)

var _ = FDescribe("E2E - Backups", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud

		solrBackup *solrv1beta1.SolrBackup

		solrCollection = "e2e"

		solrImage = getEnvWithDefault(solrImageEnv, defaultSolrImage)
	)

	BeforeEach(func() {
		ctx = context.Background()

		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: testNamespace(),
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &three,
				SolrImage: &solrv1beta1.ContainerImage{
					Repository: strings.Split(solrImage, ":")[0],
					Tag:        strings.Split(solrImage+":", ":")[1],
					PullPolicy: corev1.PullIfNotPresent,
				},
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ProvidedZookeeper: &solrv1beta1.ZookeeperSpec{
						Replicas: &one,
						Persistence: &solrv1beta1.ZKPersistence{
							PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("5Gi"),
									},
								},
							},
						},
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("300m"),
							},
						},
					},
				},
			},
		}

		solrBackup = &solrv1beta1.SolrBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: testNamespace(),
			},
			Spec: solrv1beta1.SolrBackupSpec{
				SolrCloud: "foo",
				Collections: []string{
					solrCollection,
				},
				Location: "test-dir/dir-" + rand.String(5),
			},
		}
	})

	JustBeforeEach(func() {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		By("Waiting for the SolrCloud to come up healthy")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Status.ReadyReplicas).To(Equal(*found.Spec.Replicas), "The SolrCloud should have all nodes come up healthy")
		})

		By("creating a Solr Collection to backup")
		createAndQueryCollection(solrCloud, solrCollection)

		By("creating a SolrBackup")
		Expect(k8sClient.Create(ctx, solrBackup)).To(Succeed())
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	FContext("Local Directory - Recurring", func() {
		BeforeEach(func() {
			backupName := "local"
			solrCloud.Spec.BackupRepositories = []solrv1beta1.SolrBackupRepository{
				{
					Name: backupName,
					Volume: &solrv1beta1.VolumeRepository{
						Source: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: backupDirHostPath,
							},
						},
					},
				},
			}
			solrBackup.Spec.RepositoryName = backupName
			solrBackup.Spec.Recurrence = &solrv1beta1.BackupRecurrence{
				Schedule: "@every 10s",
				MaxSaved: 3,
			}
		})

		FIt("Takes a backup correctly", func() {
			By("waiting until more backups have been taken than can be saved")
			time.Sleep(time.Second * 45)
			foundSolrBackup := expectSolrBackup(ctx, solrBackup)
			Expect(foundSolrBackup.Status.History).To(HaveLen(solrBackup.Spec.Recurrence.MaxSaved), "The SolrBackup does not have the correct number of saved backups in its status")
			Expect(foundSolrBackup.Status.History[len(foundSolrBackup.Status.History)-1].Successful).To(PointTo(BeTrue()), "The latest backup was not successful")

			By("disabling further backup recurrence")
			//patchedSolrBackup := foundSolrBackup.DeepCopy()
			//patchedSolrBackup.Spec.Recurrence.Disabled = true
			foundSolrBackup = expectSolrBackupWithChecks(ctx, solrBackup, func(g Gomega, backup *solrv1beta1.SolrBackup) {
				backup.Spec.Recurrence.Disabled = true
				g.Expect(k8sClient.Update(ctx, backup)).To(Succeed(), "Could not update SolrBackup to disable recurrence")
			})
			time.Sleep(time.Second * 15)
			nextFoundSolrBackup := expectSolrBackup(ctx, solrBackup)
			// Use start time because we might have disabled the recurrence mid-backup, and the finish time might not have been set
			Expect(nextFoundSolrBackup.Status.StartTime).To(Equal(foundSolrBackup.Status.StartTime), "The last backup start time should be unchanged after recurrence is disabled")
		})
	})

	FContext("Local Directory - Single", func() {
		BeforeEach(func() {
			backupName := "local"
			solrCloud.Spec.BackupRepositories = []solrv1beta1.SolrBackupRepository{
				{
					Name: backupName,
					Volume: &solrv1beta1.VolumeRepository{
						Source: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: backupDirHostPath,
							},
						},
					},
				},
			}
			solrBackup.Spec.RepositoryName = backupName
			solrBackup.Spec.Recurrence = nil
		})

		FIt("Takes a backup correctly", func() {
			By("waiting until more backups have been taken than can be saved")
			foundSolrBackup := expectSolrBackupWithChecks(ctx, solrBackup, func(g Gomega, backup *solrv1beta1.SolrBackup) {
				g.Expect(backup.Status.Successful).To(PointTo(BeTrue()), "Backup did not successfully complete")
			})

			expectSolrBackupWithConsistentChecks(ctx, solrBackup, func(g Gomega, backup *solrv1beta1.SolrBackup) {
				g.Expect(backup.Status.IndividualSolrBackupStatus).To(Equal(foundSolrBackup.Status.IndividualSolrBackupStatus), "Backup status changed")
				g.Expect(backup.Status.History).To(BeEmpty(), "A non-recurring backup should have no history")
			})
		})
	})
})
