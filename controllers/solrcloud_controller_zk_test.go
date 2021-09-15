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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.WithDefaults()).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
		})
	})

	AfterEach(func() {
		cleanupTest(ctx, solrCloud)
	})

	Context("ZK Connection String - Admin ACL", func() {
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
		It("has the correct resources", func() {
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
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			testACLEnvVars(foundEnv[len(foundEnv)-6:len(foundEnv)-3], false)
			Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
			// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
			// TODO: Make this not invalidate foundEnv
			testPodEnvVariables(expectedEnvVars, append(foundEnv[:len(foundEnv)-6], foundEnv[len(foundEnv)-1]))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), "Incorrect pre-stop command")
		})
	})

	Context("ZK Connection String - Admin & Read ACL", func() {
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
						ReadOnlyACL: &solrv1beta1.ZookeeperACL{
							SecretRef:   "read-secret-name",
							UsernameKey: "read-only-user",
							PasswordKey: "read-only-pass",
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
		It("has the correct resources", func() {
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
			foundEnv := statefulSet.Spec.Template.Spec.Containers[0].Env
			testACLEnvVars(foundEnv[len(foundEnv)-8:len(foundEnv)-3], true)
			Expect(foundEnv[len(foundEnv)-3:len(foundEnv)-1]).To(Equal(extraVars), "Extra Env Vars are not the same as the ones provided in podOptions")
			// Note that this check changes the variable foundEnv, so the values are no longer valid afterwards.
			// TODO: Make this not invalidate foundEnv
			testPodEnvVariables(expectedEnvVars, append(foundEnv[:len(foundEnv)-8], foundEnv[len(foundEnv)-1]))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"solr", "stop", "-p", "8983"}), "Incorrect pre-stop command")
		})
	})
})