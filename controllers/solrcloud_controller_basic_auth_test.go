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
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = FDescribe("SolrCloud controller - Basic Auth", func() {
	var (
		ctx context.Context

		solrCloud *solrv1beta1.SolrCloud
	)

	BeforeEach(func() {
		ctx = context.Background()

		replicas := int32(1)
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: &replicas,
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
						InternalConnectionString: "host:7271",
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

	FContext("Boostrap Security JSON", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
				ProbesRequireAuth:  true,
			}
		})
		FIt("has the correct resources", func() {
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)
		})
	})

	FContext("Boostrap Security JSON with Custom Probe Paths", func() {
		BeforeEach(func() {
			customHandler := corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Scheme: corev1.URISchemeHTTP,
					Path:   "/solr/readyz",
					Port:   intstr.FromInt(8983),
				},
			}

			// verify users can vary the probe path and the secure probe exec command uses them
			solrCloud.Spec.CustomSolrKubeOptions = solrv1beta1.CustomSolrKubeOptions{
				PodOptions: &solrv1beta1.PodOptions{
					LivenessProbe:  &corev1.Probe{ProbeHandler: customHandler},
					ReadinessProbe: &corev1.Probe{ProbeHandler: customHandler},
				},
			}

			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
				ProbesRequireAuth:  true,
			}
		})
		FIt("has the correct resources", func() {
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)
		})
	})

	FContext("Boostrap Security JSON with ZK ACLs", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
				ProbesRequireAuth:  true,
			}
			solrCloud.Spec.ZookeeperRef.ConnectionInfo.AllACL = &solrv1beta1.ZookeeperACL{
				SecretRef:   "secret-name",
				UsernameKey: "user",
				PasswordKey: "pass",
			}
			solrCloud.Spec.ZookeeperRef.ConnectionInfo.ReadOnlyACL = &solrv1beta1.ZookeeperACL{
				SecretRef:   "read-secret-name",
				UsernameKey: "read-only-user",
				PasswordKey: "read-only-pass",
			}
		})
		FIt("has the correct resources", func() {
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)
		})
	})

	FContext("Boostrap Security JSON - Delete Secret", func() {
		BeforeEach(func() {
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
			}
		})
		FIt("has the correct resources", func() {
			By("Testing that the statefulSet exists and has the correct information")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, true)

			By("Deleting bootstrapped secret")
			expectSecretWithChecks(ctx, solrCloud, solrCloud.SecurityBootstrapSecretName(), func(g Gomega, found *corev1.Secret) {
				g.Expect(k8sClient.Delete(ctx, found)).To(Succeed(), "Cannot delete bootstrapped secret")
			})

			By("Making sure the statefulSet is updated after deleting the bootstrap secret")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, false /* bootstrap secret not found */)
		})
	})

	FContext("User Provided Credentials", func() {
		BeforeEach(func() {
			basicAuthSecretName := "my-basic-auth-secret"
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
				BasicAuthSecret:    basicAuthSecretName,
			}
		})
		FIt("has the correct resources", func() {
			By("Making sure that no statefulSet exists until the BasicAuth Secret is created")
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			By("Create the basicAuth secret")
			basicAuthSecret := createBasicAuthSecret(solrCloud.Spec.SolrSecurity.BasicAuthSecret, solrv1beta1.DefaultBasicAuthUsername, solrCloud.Namespace)
			Expect(k8sClient.Create(ctx, basicAuthSecret)).To(Succeed(), "Could not create the necessary basicAuth secret")

			By("Make sure the StatefulSet is created and configured correctly")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, false)
		})
	})

	FContext("User Provided Credentials and security.json secret", func() {
		BeforeEach(func() {
			basicAuthSecretName := "my-basic-auth-secret"
			solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{
				AuthenticationType: solrv1beta1.Basic,
				BasicAuthSecret:    basicAuthSecretName,
				BootstrapSecurityJson: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-security-json"},
					Key:                  util.SecurityJsonFile,
				},
			}
		})
		FIt("has the correct resources", func() {
			By("Making sure that no statefulSet exists until the BasicAuth Secret is created")
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())

			By("Create the basicAuth secret")
			basicAuthSecret := createBasicAuthSecret(solrCloud.Spec.SolrSecurity.BasicAuthSecret, solrv1beta1.DefaultBasicAuthUsername, solrCloud.Namespace)
			Expect(k8sClient.Create(ctx, basicAuthSecret)).To(Succeed(), "Could not create the necessary basicAuth secret")

			By("Create the security.json Secret")
			createMockSecurityJsonSecret(ctx, "my-security-json", solrCloud.Namespace)

			By("Make sure the StatefulSet is created and configured correctly")
			expectStatefulSetBasicAuthConfig(ctx, solrCloud, false)
		})
	})
})

var boostrapedSecretKeys = []string{
	"admin",
	"solr",
	"security.json",
}

func expectStatefulSetBasicAuthConfig(ctx context.Context, sc *solrv1beta1.SolrCloud, expectBootstrapSecret bool) *appsv1.StatefulSet {
	Expect(sc.Spec.SolrSecurity).To(Not(BeNil()), "solrSecurity is not configured for this SolrCloud instance!")

	expProbePath := "/solr/admin/info/system"
	if sc.Spec.CustomSolrKubeOptions.PodOptions != nil && sc.Spec.CustomSolrKubeOptions.PodOptions.LivenessProbe != nil {
		expProbePath = sc.Spec.CustomSolrKubeOptions.PodOptions.LivenessProbe.HTTPGet.Path
	}

	stateful := expectStatefulSetWithChecks(ctx, sc, sc.StatefulSetName(), func(g Gomega, found *appsv1.StatefulSet) {
		expectBasicAuthConfigOnPodTemplateWithGomega(g, sc, &found.Spec.Template, expectBootstrapSecret, expProbePath)
	})

	expectSecretWithChecks(ctx, sc, sc.BasicAuthSecretName(), func(innerG Gomega, found *corev1.Secret) {
		innerG.Expect(found.Data).To(HaveKey(corev1.BasicAuthUsernameKey), "The Basic auth secret does not have a username entry for key %s", corev1.BasicAuthUsernameKey)
		innerG.Expect(string(found.Data[corev1.BasicAuthUsernameKey])).To(Equal(solrv1beta1.DefaultBasicAuthUsername), "password should be set for k8s-oper in the basic auth secret")
		innerG.Expect(found.Data).To(HaveKey(corev1.BasicAuthPasswordKey), "The Basic auth secret does not have a password entry for key %s", corev1.BasicAuthPasswordKey)
		innerG.Expect(found.Data[corev1.BasicAuthPasswordKey]).To(Not(BeEmpty()), "password should be set for k8s-oper in the basic auth secret and not be empty")
	})

	// verify a security.json gets bootstrapped if not using a user-provided secret
	if sc.Spec.SolrSecurity.BasicAuthSecret == "" && expectBootstrapSecret {
		bootstrapSecret := expectSecretWithChecks(ctx, sc, sc.SecurityBootstrapSecretName(), func(g Gomega, found *corev1.Secret) {
			for _, key := range boostrapedSecretKeys {
				g.Expect(found.Data).To(HaveKey(key), "The Boostrapped Security secret does not key %s", key)
				g.Expect(found.Data[key]).To(Not(BeEmpty()), "The Boostrapped Security secret key should not be empty: %s", key)
			}
		})

		if sc.Spec.CustomSolrKubeOptions.PodOptions != nil {
			probePaths := util.GetCustomProbePaths(sc)
			if len(probePaths) > 0 {
				securityJson := string(bootstrapSecret.Data["security.json"])
				Expect(securityJson).To(ContainSubstring(util.DefaultProbePath), "bootstrapped security.json should have an authz rule for probe path: %s", util.DefaultProbePath)
				for _, p := range probePaths {
					p = p[len("/solr"):] // drop the /solr part on the path
					Expect(securityJson).To(ContainSubstring(p), "bootstrapped security.json should have an authz rule for probe path: %s", p)
				}
			}
		}
	}

	return stateful
}

// Ensures config is setup for basic-auth enabled Solr pods
func expectBasicAuthConfigOnPodTemplateWithGomega(g Gomega, solrCloud *solrv1beta1.SolrCloud, podTemplate *corev1.PodTemplateSpec, expectBootstrapSecret bool, expProbePath string) *corev1.Container {
	// check the env vars needed for the probes to work with auth
	g.Expect(podTemplate.Spec.Containers).To(Not(BeEmpty()), "Solr Pod requires containers")
	mainContainer := podTemplate.Spec.Containers[0]
	g.Expect(mainContainer).To(Not(BeNil()), "Didn't find the main solrcloud-node container in the sts!")
	g.Expect(mainContainer.Env).To(Not(BeEmpty()), "Didn't find the main solrcloud-node container in the sts!")

	// probes with auth
	if solrCloud.Spec.SolrSecurity.ProbesRequireAuth {
		g.Expect(podTemplate.Spec.Volumes).To(Not(BeEmpty()), "Solr Pod requires volumes when using BasicAuth")
		secretName := solrCloud.BasicAuthSecretName()
		var basicAuthSecretVol *corev1.Volume = nil
		for _, vol := range podTemplate.Spec.Volumes {
			if vol.Name == secretName {
				basicAuthSecretVol = &vol
				break
			}
		}
		g.Expect(basicAuthSecretVol).To(Not(BeNil()), "No basic auth secret volume found in Pod spec")
		g.Expect(basicAuthSecretVol.VolumeSource.Secret).To(Not(BeNil()), "Didn't find the basic auth secret volume in sts config!")
		g.Expect(basicAuthSecretVol.VolumeSource.Secret.SecretName).To(Equal(solrCloud.BasicAuthSecretName()), "Wrong secret name used for basic auth volume")

		var basicAuthSecretVolMount *corev1.VolumeMount = nil
		for _, m := range podTemplate.Spec.Containers[0].VolumeMounts {
			if m.Name == secretName {
				basicAuthSecretVolMount = &m
				break
			}
		}
		g.Expect(basicAuthSecretVolMount).To(Not(BeNil()), "No Basic Auth volume mount used in Solr container")
		g.Expect(basicAuthSecretVolMount.MountPath).To(Equal("/etc/secrets/"+secretName), "Wrong path used to mount Basic Auth volume")

		expProbeCmd := fmt.Sprintf("JAVA_TOOL_OPTIONS=\"-Dbasicauth=$(cat /etc/secrets/%s-solrcloud-basic-auth/username):$(cat /etc/secrets/%s-solrcloud-basic-auth/password)\" java "+
			"-Dsolr.httpclient.builder.factory=org.apache.solr.client.solrj.impl.PreemptiveBasicAuthClientBuilderFactory "+
			"-Dsolr.install.dir=\"/opt/solr\" -Dlog4j.configurationFile=\"/opt/solr/server/resources/log4j2-console.xml\" "+
			"-classpath \"/opt/solr/server/solr-webapp/webapp/WEB-INF/lib/*:/opt/solr/server/lib/ext/*:/opt/solr/server/lib/*\" "+
			"org.apache.solr.util.SolrCLI api -get http://localhost:8983%s",
			solrCloud.Name, solrCloud.Name, expProbePath)
		g.Expect(mainContainer.LivenessProbe).To(Not(BeNil()), "main container should have a liveness probe defined")
		g.Expect(mainContainer.LivenessProbe.Exec).To(Not(BeNil()), "liveness probe should have an exec when auth is enabled")
		g.Expect(mainContainer.LivenessProbe.Exec.Command).To(Not(BeEmpty()), "liveness probe command cannot be empty")
		g.Expect(mainContainer.LivenessProbe.Exec.Command[2]).To(Equal(expProbeCmd), "liveness probe should invoke java with auth opts")
		g.Expect(mainContainer.LivenessProbe.TimeoutSeconds).To(BeEquivalentTo(5), "liveness probe default timeout should be increased when using basicAuth")
		g.Expect(mainContainer.ReadinessProbe).To(Not(BeNil()), "main container should have a readiness probe defined")
		g.Expect(mainContainer.ReadinessProbe.Exec).To(Not(BeNil()), "readiness probe should have an exec when auth is enabled")
		g.Expect(mainContainer.ReadinessProbe.Exec.Command).To(Not(BeEmpty()), "readiness probe command cannot be empty")
		g.Expect(mainContainer.ReadinessProbe.Exec.Command[2]).To(Equal(expProbeCmd), "readiness probe should invoke java with auth opts")
		g.Expect(mainContainer.ReadinessProbe.TimeoutSeconds).To(BeEquivalentTo(5), "readiness probe default timeout should be increased when using basicAuth")
	}

	// if no user-provided auth secret, then check that security.json gets bootstrapped correctly
	if solrCloud.Spec.SolrSecurity.BasicAuthSecret == "" || solrCloud.Spec.SolrSecurity.BootstrapSecurityJson != nil {
		// initContainers
		g.Expect(podTemplate.Spec.InitContainers).To(Not(BeEmpty()), "The Solr Pod template requires an init container to bootstrap the security.json")
		var expInitContainer *corev1.Container = nil
		for _, cnt := range podTemplate.Spec.InitContainers {
			if cnt.Name == "setup-zk" {
				expInitContainer = &cnt
				break
			}
		}

		if expectBootstrapSecret || solrCloud.Spec.SolrSecurity.BootstrapSecurityJson != nil {
			// if the zookeeperRef has ACLs set, verify the env vars were set correctly for this initContainer
			allACL, _ := solrCloud.Spec.ZookeeperRef.GetACLs()
			if allACL != nil {
				g.Expect(expInitContainer.Env).To(HaveLen(10), "Wrong number of env vars using ACLs and Basic Auth")
				g.Expect(expInitContainer.Env[len(expInitContainer.Env)-2].Name).To(Equal("SOLR_OPTS"), "Env var SOLR_OPTS is misplaced the Solr Pod env vars")
				g.Expect(expInitContainer.Env[len(expInitContainer.Env)-1].Name).To(Equal("SECURITY_JSON"), "Env var SECURITY_JSON is misplaced the Solr Pod env vars")
				testACLEnvVarsWithGomega(g, expInitContainer.Env[3:len(expInitContainer.Env)-2], true)
			} // else this ref not using ACLs

			expectPutSecurityJsonInZkCmd(g, expInitContainer)
		} else {
			g.Expect(expInitContainer).To(Or(
				BeNil(),
				WithTransform(
					func(container corev1.Container) string {
						if len(container.Command) < 3 {
							return ""
						}
						return container.Command[2]
					},
					Not(ContainSubstring("SECURITY_JSON")))), "setup-zk initContainer not reconciled after bootstrap secret deleted")
		}

	}

	return &mainContainer // return as a convenience in case tests want to do more checking on the main container
}

func expectPutSecurityJsonInZkCmd(g Gomega, expInitContainer *corev1.Container) {
	g.Expect(expInitContainer).To(Not(BeNil()), "Didn't find the setup-zk InitContainer in the sts!")
	expCmd := "ZK_SECURITY_JSON=$(/opt/solr/server/scripts/cloud-scripts/zkcli.sh -zkhost ${ZK_HOST} -cmd get /security.json); " +
		"if [ ${#ZK_SECURITY_JSON} -lt 3 ]; then " +
		"echo $SECURITY_JSON > /tmp/security.json; " +
		"/opt/solr/server/scripts/cloud-scripts/zkcli.sh -zkhost ${ZK_HOST} -cmd putfile /security.json /tmp/security.json; echo \"put security.json in ZK\"; fi"
	g.Expect(expInitContainer.Command[2]).To(ContainSubstring(expCmd), "setup-zk initContainer not configured to bootstrap security.json!")
}

func createMockSecurityJsonSecret(ctx context.Context, name string, ns string) corev1.Secret {
	secData := map[string]string{}
	secData[util.SecurityJsonFile] = "{}"
	sec := corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, StringData: secData}
	Expect(k8sClient.Create(ctx, &sec)).To(Succeed(), "Could not create mock security.json secret")
	return sec
}
