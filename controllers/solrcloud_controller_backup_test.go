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
)

var _ = FDescribe("SolrCloud controller - Backup Repositories", func() {
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

	FContext("S3 Repository", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
						Volumes:      extraVolumes,
						Annotations:  testPodAnnotations,
					},
				},
				BackupRepositories: []solrv1beta1.SolrBackupRepository{
					{
						Name: "test-repo",
						S3: &solrv1beta1.S3Repository{
							Region: "test-region",
							Bucket: "test-bucket",
						},
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr ConfigMap")
			configMap := expectConfigMap(ctx, solrCloud, solrCloud.ConfigMapName(), map[string]string{"solr.xml": util.GenerateSolrXMLStringForCloud(solrCloud)})

			By("testing the Solr StatefulSet with explicit volumes and envVars before adding S3Repo credentials")
			// Make sure envVars and Volumes are correct be
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Annotations for the solrxml configMap
			solrXmlMd5 := fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))
			Expect(statefulSet.Spec.Template.Annotations).To(Equal(util.MergeLabelsOrAnnotations(testPodAnnotations, map[string]string{
				"solr.apache.org/solrXmlMd5":          solrXmlMd5,
				util.SolrBackupRepositoriesAnnotation: "test-repo",
			})), "Incorrect pod annotations")

			// Env Variable Tests
			expectedEnvVars := map[string]string{
				"ZK_HOST":        "host:7271/",
				"SOLR_HOST":      "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName() + "." + solrCloud.Namespace,
				"SOLR_PORT":      "8983",
				"SOLR_NODE_PORT": "8983",
				"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
			}
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env

			// First test the extraVars, then the default Solr Operator vars
			Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
			testPodEnvVariables(expectedEnvVars, append(foundEnv[:len(foundEnv)-3], foundEnv[len(foundEnv)-1]))

			// Check Volumes
			extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
			Expect(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(len(extraVolumes)+1), "Container has wrong number of volumeMounts")
			Expect(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[1]).To(Equal(*extraVolumes[0].DefaultContainerMount), "Additional Volume from podOptions not mounted into container properly.")
			Expect(statefulSet.Spec.Template.Spec.Volumes).To(HaveLen(len(extraVolumes)+2), "Pod has wrong number of volumes")
			Expect(statefulSet.Spec.Template.Spec.Volumes[2].Name).To(Equal(extraVolumes[0].Name), "Additional Volume from podOptions not loaded into pod properly.")
			Expect(statefulSet.Spec.Template.Spec.Volumes[2].VolumeSource).To(Equal(extraVolumes[0].Source), "Additional Volume from podOptions not loaded into pod properly.")

			By("adding credentials to the S3 repository (envVars)")
			s3Credentials := &solrv1beta1.S3Credentials{
				AccessKeyIdSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "aws-secret-1"},
					Key:                  "accessKeyId",
				},
				SecretAccessKeySecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "aws-secret-2"},
					Key:                  "secretAccessKey",
				},
				SessionTokenSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "aws-secret-3"},
					Key:                  "sessionToken",
				},
			}
			foundSolrCloud := expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.BackupRepositories[0].S3.Credentials = s3Credentials
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Change the s3 credentials for the SolrCloud")
			})

			By("testing the Solr StatefulSet after adding S3Repo envVar credentials")
			// Make sure envVars and Volumes are correct be
			statefulSet = expectStatefulSetWithChecks(ctx, foundSolrCloud, foundSolrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				// Annotations for the solrxml configMap
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))), "Wrong solr.xml MD5 annotation in the pod template!")

				// Env Variable Tests
				expectedEnvVars := map[string]string{
					"ZK_HOST":        "host:7271/",
					"SOLR_HOST":      "$(POD_HOSTNAME)." + foundSolrCloud.HeadlessServiceName() + "." + foundSolrCloud.Namespace,
					"SOLR_PORT":      "8983",
					"SOLR_NODE_PORT": "8983",
					"SOLR_LOG_LEVEL": "INFO",
					"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
				}
				foundEnv := found.Spec.Template.Spec.Containers[0].Env

				// First test the extraVars, then the default Solr Operator vars, then the S3 vars
				g.Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
				testPodEnvVariablesWithGomega(g, expectedEnvVars, append(append([]corev1.EnvVar{}, foundEnv[:len(foundEnv)-6]...), foundEnv[len(foundEnv)-1]))
				g.Expect(foundEnv[len(foundEnv)-6:len(foundEnv)-3]).To(Equal([]corev1.EnvVar{
					{
						Name:      "AWS_ACCESS_KEY_ID",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: s3Credentials.AccessKeyIdSecret},
					},
					{
						Name:      "AWS_SECRET_ACCESS_KEY",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: s3Credentials.SecretAccessKeySecret},
					},
					{
						Name:      "AWS_SESSION_TOKEN",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: s3Credentials.SessionTokenSecret},
					},
				}), "Wrong envVars for the S3 Credentials")

				// Check that no additional volumes have been added
				extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(len(extraVolumes)+1), "Container has wrong number of volumeMounts")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[1]).To(Equal(*extraVolumes[0].DefaultContainerMount), "Additional Volume from podOptions not mounted into container properly.")
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(len(extraVolumes)+2), "Pod has wrong number of volumes")
				g.Expect(found.Spec.Template.Spec.Volumes[2].Name).To(Equal(extraVolumes[0].Name), "Additional Volume from podOptions not loaded into pod properly.")
				g.Expect(found.Spec.Template.Spec.Volumes[2].VolumeSource).To(Equal(extraVolumes[0].Source), "Additional Volume from podOptions not loaded into pod properly.")
			})

			By("adding credentials to the S3 repository (envVars & credentials file)")
			s3Credentials.CredentialsFileSecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "aws-credentials"},
				Key:                  "credentials-file",
			}
			foundSolrCloud = expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.BackupRepositories[0].S3.Credentials = s3Credentials
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Change the s3 credentials for the SolrCloud")
			})

			By("testing the Solr StatefulSet after adding S3Repo envVar credentials & credentials file")
			// Make sure envVars and Volumes are correct be
			statefulSet = expectStatefulSetWithChecks(ctx, foundSolrCloud, foundSolrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				// Annotations for the solrxml configMap
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))), "Wrong solr.xml MD5 annotation in the pod template!")

				// Env Variable Tests
				expectedEnvVars := map[string]string{
					"ZK_HOST":        "host:7271/",
					"SOLR_HOST":      "$(POD_HOSTNAME)." + foundSolrCloud.HeadlessServiceName() + "." + foundSolrCloud.Namespace,
					"SOLR_PORT":      "8983",
					"SOLR_NODE_PORT": "8983",
					"SOLR_LOG_LEVEL": "INFO",
					"SOLR_OPTS":      "-DhostPort=$(SOLR_NODE_PORT)",
				}
				foundEnv := found.Spec.Template.Spec.Containers[0].Env

				// First test the extraVars, then the default Solr Operator vars, then the S3 vars
				g.Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
				testPodEnvVariablesWithGomega(g, expectedEnvVars, append(append([]corev1.EnvVar{}, foundEnv[:len(foundEnv)-7]...), foundEnv[len(foundEnv)-1]))
				g.Expect(foundEnv[len(foundEnv)-7:len(foundEnv)-3]).To(Equal([]corev1.EnvVar{
					{
						Name:      "AWS_ACCESS_KEY_ID",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: s3Credentials.AccessKeyIdSecret},
					},
					{
						Name:      "AWS_SECRET_ACCESS_KEY",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: s3Credentials.SecretAccessKeySecret},
					},
					{
						Name:      "AWS_SESSION_TOKEN",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: s3Credentials.SessionTokenSecret},
					},
					{
						Name:  "AWS_SHARED_CREDENTIALS_FILE",
						Value: "/var/solr/data/backup-restore/test-repo/s3credential/credentials",
					},
				}), "Wrong envVars for the S3 Credentials")

				// Check that no additional volumes have been added
				extraVolumes[0].DefaultContainerMount.Name = extraVolumes[0].Name
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(len(extraVolumes)+2), "Container has wrong number of volumeMounts")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name).To(Equal("backup-repository-test-repo"), "S3Credentials file volumeMount has wrong name.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[1].MountPath).To(Equal("/var/solr/data/backup-restore/test-repo/s3credential"), "S3Credentials file volumeMount has wrong mount path.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[1].ReadOnly).To(BeTrue(), "S3Credentials file volumeMount must be read-only.")
				g.Expect(found.Spec.Template.Spec.Containers[0].VolumeMounts[2]).To(Equal(*extraVolumes[0].DefaultContainerMount), "Additional Volume from podOptions not mounted into container properly.")
				g.Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(len(extraVolumes)+3), "Pod has wrong number of volumes")
				g.Expect(found.Spec.Template.Spec.Volumes[2].Name).To(Equal("backup-repository-test-repo"), "S3Credentials file volume has wrong name.")
				g.Expect(found.Spec.Template.Spec.Volumes[2].VolumeSource.Secret).To(Not(BeNil()), "S3Credentials file has to be loaded via a secret volume.")
				g.Expect(found.Spec.Template.Spec.Volumes[2].VolumeSource.Secret.SecretName).To(Equal(s3Credentials.CredentialsFileSecret.Name), "S3Credentials file is loaded into the pod using the wrong secret name.")
				g.Expect(found.Spec.Template.Spec.Volumes[2].VolumeSource.Secret.Items).To(Equal(
					[]corev1.KeyToPath{{Key: s3Credentials.CredentialsFileSecret.Key, Path: util.S3CredentialFileName}}), "S3Credentials file pod volume has the wrong items.")
				g.Expect(found.Spec.Template.Spec.Volumes[2].VolumeSource.Secret.DefaultMode).To(BeEquivalentTo(&util.SecretReadOnlyPermissions), "S3Credentials file pod volume has the wrong default mode.")
				g.Expect(found.Spec.Template.Spec.Volumes[3].Name).To(Equal(extraVolumes[0].Name), "Additional Volume from podOptions not loaded into pod properly.")
				g.Expect(found.Spec.Template.Spec.Volumes[3].VolumeSource).To(Equal(extraVolumes[0].Source), "Additional Volume from podOptions not loaded into pod properly.")
			})

			By("adding extra options to the S3 repository")
			s3Credentials.CredentialsFileSecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "aws-credentials"},
				Key:                  "credentials-file",
			}
			foundSolrCloud = expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.BackupRepositories[0].S3.Endpoint = "https://test-endpoint:3223"
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Change the s3 endpoint for the SolrCloud")
			})

			By("checking the solr.xml configMap is updated with the new S3 options, and a solrCloud rolling update follows")
			newConfigMap := expectConfigMap(ctx, foundSolrCloud, foundSolrCloud.ConfigMapName(), map[string]string{"solr.xml": util.GenerateSolrXMLStringForCloud(foundSolrCloud)})

			updateSolrXmlMd5 := fmt.Sprintf("%x", md5.Sum([]byte(newConfigMap.Data[util.SolrXmlFile])))
			Expect(updateSolrXmlMd5).To(Not(Equal(solrXmlMd5)), "New configMap hash should not equal initial configMap hash")
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
				g.Expect(found.Spec.Template.Annotations).To(HaveKeyWithValue(util.SolrXmlMd5Annotation, updateSolrXmlMd5), "Custom solr.xml MD5 annotation should be updated on the pod template.")
			})
		})
	})

	FContext("Multiple Repositories - Annotations", func() {
		BeforeEach(func() {
			solrCloud.Spec = solrv1beta1.SolrCloudSpec{
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
					},
				},
				CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
					PodOptions: &solrv1beta1.PodOptions{
						EnvVariables: extraVars,
						Volumes:      extraVolumes,
					},
				},
				BackupRepositories: []solrv1beta1.SolrBackupRepository{
					{
						Name: "test-repo",
						S3: &solrv1beta1.S3Repository{
							Region: "test-region",
							Bucket: "test-bucket",
						},
					},
					{
						Name: "another",
						S3: &solrv1beta1.S3Repository{
							Region: "test-region-2",
							Bucket: "test-bucket-2",
						},
					},
				},
			}
		})
		FIt("has the correct resources", func() {
			By("testing the Solr ConfigMap")
			configMap := expectConfigMap(ctx, solrCloud, solrCloud.ConfigMapName(), map[string]string{"solr.xml": util.GenerateSolrXMLStringForCloud(solrCloud)})

			By("testing the Solr StatefulSet with explicit volumes and envVars before adding S3Repo credentials")
			// Make sure envVars and Volumes are correct be
			statefulSet := expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			// Annotations for the solrxml configMap
			Expect(statefulSet.Spec.Template.Annotations).To(Equal(map[string]string{
				"solr.apache.org/solrXmlMd5":          fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data["solr.xml"]))),
				util.SolrBackupRepositoriesAnnotation: "another,test-repo",
			}), "Incorrect pod annotations")
		})
	})
})
