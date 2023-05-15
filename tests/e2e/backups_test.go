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
	"github.com/apache/solr-operator/controllers/util/solr_api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"time"
)

var _ = FDescribe("E2E - Backups", Ordered, func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrBackup *solrv1beta1.SolrBackup

		solrCollection = "e2e"

		localBackupRepository = "local"
	)

	/*
		Create a single SolrCloud that all PrometheusExporter tests in this "Describe" will use.
	*/
	BeforeAll(func(ctx context.Context) {
		solrCloud = generateBaseSolrCloud(2)
		solrCloud.Spec.BackupRepositories = []solrv1beta1.SolrBackupRepository{
			{
				Name: localBackupRepository,
				Volume: &solrv1beta1.VolumeRepository{
					Source: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: backupDirHostPath,
						},
					},
				},
			},
		}

		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a Solr Collection to backup")
		createAndQueryCollection(ctx, solrCloud, solrCollection, 1, 2)
	})

	BeforeEach(func() {
		solrBackup = &solrv1beta1.SolrBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: solrCloud.Namespace,
			},
			Spec: solrv1beta1.SolrBackupSpec{
				SolrCloud: "foo",
				Collections: []string{
					solrCollection,
				},
				Location: "test-dir",
			},
		}
	})

	JustBeforeEach(func(ctx context.Context) {
		backupName := rand.String(5)
		solrBackup.Name += backupName
		// We are using one cloud for each Solr backup, make sure that the location is different for each
		solrBackup.Spec.Location += "/dir-" + backupName

		By("creating a SolrBackup")
		Expect(k8sClient.Create(ctx, solrBackup)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			deleteAndWait(ctx, solrBackup)
		})
	})

	FContext("Local Directory - Recurring", func() {
		BeforeEach(func() {
			solrBackup.Spec.RepositoryName = localBackupRepository
			solrBackup.Spec.Recurrence = &solrv1beta1.BackupRecurrence{
				Schedule: "@every 10s",
				MaxSaved: 3,
			}
		})

		FIt("Takes a backup correctly", func(ctx context.Context) {
			By("waiting until more backups have been taken than can be saved")
			time.Sleep(time.Second * 45)
			foundSolrBackup := expectSolrBackup(ctx, solrBackup)
			Expect(foundSolrBackup.Status.History).To(HaveLen(solrBackup.Spec.Recurrence.MaxSaved), "The SolrBackup does not have the correct number of saved backups in its status")
			Expect(foundSolrBackup.Status.History[len(foundSolrBackup.Status.History)-1].Successful).To(PointTo(BeTrue()), "The latest backup was not successful")

			lastBackupId := 0
			checkBackup(ctx, solrCloud, solrBackup, func(g Gomega, collection string, backupListResponse *solr_api.SolrBackupListResponse) {
				g.Expect(backupListResponse.Backups).To(HaveLen(3), "The wrong number of recurring backups have been saved")
				lastBackupId = backupListResponse.Backups[len(backupListResponse.Backups)-1].BackupId
				g.Expect(lastBackupId).To(BeNumerically(">", 3), "The last backup ID is too low")
			})

			By("disabling further backup recurrence")
			foundSolrBackup = expectSolrBackupWithChecks(ctx, solrBackup, func(g Gomega, backup *solrv1beta1.SolrBackup) {
				backup.Spec.Recurrence.Disabled = true
				g.Expect(k8sClient.Update(ctx, backup)).To(Succeed(), "Could not update SolrBackup to disable recurrence")
			})
			time.Sleep(time.Second * 15)
			nextFoundSolrBackup := expectSolrBackup(ctx, solrBackup)
			// Use start time because we might have disabled the recurrence mid-backup, and the finish time might not have been set
			Expect(nextFoundSolrBackup.Status.StartTime).To(Equal(foundSolrBackup.Status.StartTime), "The last backup start time should be unchanged after recurrence is disabled")

			checkBackup(ctx, solrCloud, solrBackup, func(g Gomega, collection string, backupListResponse *solr_api.SolrBackupListResponse) {
				g.Expect(backupListResponse.Backups).To(HaveLen(3), "The wrong number of recurring backups have been saved")
				newLastBackupId := backupListResponse.Backups[len(backupListResponse.Backups)-1].BackupId
				g.Expect(newLastBackupId).To(Equal(lastBackupId), "The last backup ID should not have been changed since the backup recurrence was disabled")
			})
		})
	})

	FContext("Local Directory - Single", func() {
		BeforeEach(func() {
			solrBackup.Spec.RepositoryName = localBackupRepository
			solrBackup.Spec.Recurrence = nil
		})

		FIt("Takes a backup correctly", func(ctx context.Context) {
			By("waiting until more backups have been taken than can be saved")
			foundSolrBackup := expectSolrBackupWithChecks(ctx, solrBackup, func(g Gomega, backup *solrv1beta1.SolrBackup) {
				g.Expect(backup.Status.Successful).To(PointTo(BeTrue()), "Backup did not successfully complete")
			})

			checkBackup(ctx, solrCloud, solrBackup, func(g Gomega, collection string, backupListResponse *solr_api.SolrBackupListResponse) {
				g.Expect(backupListResponse.Backups).To(HaveLen(1), "A non-recurring backupList should have a length of 1")
			})

			// Make sure nothing else happens after the backup is complete
			expectSolrBackupWithConsistentChecks(ctx, solrBackup, func(g Gomega, backup *solrv1beta1.SolrBackup) {
				g.Expect(backup.Status.IndividualSolrBackupStatus).To(Equal(foundSolrBackup.Status.IndividualSolrBackupStatus), "Backup status changed")
				g.Expect(backup.Status.History).To(BeEmpty(), "A non-recurring backup should have no history")
				g.Expect(backup.Status.NextScheduledTime).To(BeNil(), "There should be no nextScheduledTime for a non-recurring backup")
			})

			checkBackup(ctx, solrCloud, solrBackup, func(g Gomega, collection string, backupListResponse *solr_api.SolrBackupListResponse) {
				g.Expect(backupListResponse.Backups).To(HaveLen(1), "A non-recurring backupList should have a length of 1")
				g.Expect(backupListResponse.Backups[0].BackupId).To(Equal(0), "A non-recurring backup should have an ID of 1")
			})
		})
	})
})
