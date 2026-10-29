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

package util

import (
	"strings"
	"testing"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	psaapi "k8s.io/pod-security-admission/api"
	psapolicy "k8s.io/pod-security-admission/policy"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

// expectRestrictedProfile evaluates a generated pod template against the Kubernetes Pod Security
// Standards "restricted" profile, the same way the built-in Pod Security admission controller would.
//
// Only pods made up of operator-generated containers can pass. Containers that the user adds through
// podOptions carry whatever security context the user gave them, so the fixtures here add none.
func expectRestrictedProfile(t *testing.T, podTemplate *corev1.PodTemplateSpec, description string) {
	t.Helper()

	evaluator, err := psapolicy.NewEvaluator(psapolicy.DefaultChecks(), nil)
	require.NoError(t, err, "Could not build a Pod Security Standards evaluator")

	levelVersion := psaapi.LevelVersion{Level: psaapi.LevelRestricted, Version: psaapi.LatestVersion()}
	metadata := podTemplate.ObjectMeta.DeepCopy()
	podSpec := podTemplate.Spec.DeepCopy()

	var violations []string
	for _, result := range evaluator.EvaluatePod(levelVersion, metadata, podSpec) {
		if !result.Allowed {
			violations = append(violations, result.ForbiddenReason+": "+result.ForbiddenDetail)
		}
	}
	assert.Empty(t, violations, "%s does not satisfy the \"restricted\" profile: %s", description, strings.Join(violations, "; "))
}

func testSolrCloudForSecurityContext() *solr.SolrCloud {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec:       solr.SolrCloudSpec{},
	}
	solrCloud.WithDefaults(ctrl.Log)
	return solrCloud
}

// testStatefulSet generates the StatefulSet for a SolrCloud, with a ZooKeeper chroot so that the
// "setup-zk" init container is generated too.
func testStatefulSet(solrCloud *solr.SolrCloud, tls *TLSCerts, security *SecurityConfig) *appsv1.StatefulSet {
	status := &solr.SolrCloudStatus{
		ZookeeperConnectionInfo: solr.ZookeeperConnectionInfo{
			InternalConnectionString: "host:2181",
			ChRoot:                   "/foo",
		},
	}
	return GenerateStatefulSet(solrCloud, status, map[string]string{}, map[string]string{}, tls, security)
}

func TestGeneratedSolrCloudPodsSatisfyRestrictedProfile(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		statefulSet := testStatefulSet(testSolrCloudForSecurityContext(), nil, nil)
		expectRestrictedProfile(t, &statefulSet.Spec.Template, "The default SolrCloud pod")
	})

	t.Run("with a backup repository volume", func(t *testing.T) {
		solrCloud := testSolrCloudForSecurityContext()
		solrCloud.Spec.BackupRepositories = []solr.SolrBackupRepository{
			{
				Name:   "test-repo",
				Volume: &solr.VolumeRepository{Source: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		}
		statefulSet := testStatefulSet(solrCloud, nil, nil)
		expectRestrictedProfile(t, &statefulSet.Spec.Template, "A SolrCloud pod with a backup repository volume")
	})

	t.Run("with security.json", func(t *testing.T) {
		solrCloud := testSolrCloudForSecurityContext()
		security := &SecurityConfig{
			SolrSecurity: &solr.SolrSecurityOptions{AuthenticationType: solr.Basic},
			SecurityJson: "{}",
			SecurityJsonSrc: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "foo-security"},
					Key:                  "security.json",
				},
			},
		}
		statefulSet := testStatefulSet(solrCloud, nil, security)
		expectRestrictedProfile(t, &statefulSet.Spec.Template, "A SolrCloud pod bootstrapping security.json")
	})

	t.Run("with TLS from a secret", func(t *testing.T) {
		solrCloud := testSolrCloudForSecurityContext()
		solrCloud.Spec.SolrTLS = &solr.SolrTLSOptions{
			PKCS12Secret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "tls-secret"},
				Key:                  "keystore.p12",
			},
			KeyStorePasswordSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "tls-secret"},
				Key:                  "password",
			},
		}
		tls := TLSCertsForSolrCloud(solrCloud)
		// Force the keystore conversion init container, which is the other operator-generated container here
		tls.ServerConfig.NeedsPkcs12InitContainer = true
		statefulSet := testStatefulSet(solrCloud, tls, nil)

		initContainerNames := containerNames(statefulSet.Spec.Template.Spec.InitContainers)
		assert.Contains(t, initContainerNames, Pkcs12InitContainer, "The keystore conversion init container should be generated")
		expectRestrictedProfile(t, &statefulSet.Spec.Template, "A SolrCloud pod with TLS from a secret")
	})

	t.Run("with TLS from a mounted directory", func(t *testing.T) {
		solrCloud := testSolrCloudForSecurityContext()
		solrCloud.Spec.SolrTLS = &solr.SolrTLSOptions{
			MountedTLSDir: &solr.MountedTLSDirectory{Path: "/var/solr/tls"},
		}
		statefulSet := testStatefulSet(solrCloud, TLSCertsForSolrCloud(solrCloud), nil)

		initContainerNames := containerNames(statefulSet.Spec.Template.Spec.InitContainers)
		assert.Contains(t, initContainerNames, InitdbInitContainer, "The initdb init container should be generated")
		expectRestrictedProfile(t, &statefulSet.Spec.Template, "A SolrCloud pod with TLS from a mounted directory")
	})
}

func TestGeneratedPrometheusExporterPodsSatisfyRestrictedProfile(t *testing.T) {
	exporterWithDefaults := func() *solr.SolrPrometheusExporter {
		exporter := &solr.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		}
		exporter.WithDefaults()
		return exporter
	}

	t.Run("default", func(t *testing.T) {
		deployment := GenerateSolrPrometheusExporterDeployment(exporterWithDefaults(), SolrConnectionInfo{StandaloneAddress: "http://localhost:8983/solr"}, nil, "", nil, "")
		expectRestrictedProfile(t, &deployment.Spec.Template, "The default Prometheus exporter pod")
	})

	t.Run("with TLS from a mounted directory", func(t *testing.T) {
		exporter := exporterWithDefaults()
		exporter.Spec.SolrReference.SolrTLS = &solr.SolrTLSOptions{
			MountedTLSDir: &solr.MountedTLSDirectory{Path: "/var/solr/tls"},
		}
		tls := TLSCertsForExporter(exporter)
		deployment := GenerateSolrPrometheusExporterDeployment(exporter, SolrConnectionInfo{StandaloneAddress: "http://localhost:8983/solr"}, nil, "", tls, "")

		initContainerNames := containerNames(deployment.Spec.Template.Spec.InitContainers)
		assert.Contains(t, initContainerNames, TLSWrapperScriptInitContainer, "The TLS wrapper script init container should be generated")
		expectRestrictedProfile(t, &deployment.Spec.Template, "A Prometheus exporter pod with TLS from a mounted directory")
	})
}

func TestDefaultSecurityContextsAreOverridable(t *testing.T) {
	t.Run("the pod defaults are set", func(t *testing.T) {
		securityContext := defaultPodSecurityContext(nil, DefaultSolrGroup)
		assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, securityContext.SeccompProfile.Type, "The pod should use the default seccomp profile")
		assert.Equal(t, int64(DefaultSolrGroup), *securityContext.FSGroup, "The pod should use the default fsGroup")
	})

	t.Run("the user wins at the pod level", func(t *testing.T) {
		provided := &corev1.PodSecurityContext{
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
			FSGroup:        ptr.To(int64(1234)),
			RunAsUser:      ptr.To(int64(4321)),
		}
		securityContext := defaultPodSecurityContext(provided, DefaultSolrGroup)
		assert.Equal(t, corev1.SeccompProfileTypeUnconfined, securityContext.SeccompProfile.Type, "The provided seccomp profile should be kept")
		assert.Equal(t, int64(1234), *securityContext.FSGroup, "The provided fsGroup should be kept")
		assert.Equal(t, int64(4321), *securityContext.RunAsUser, "The provided runAsUser should be kept")
		assert.NotSame(t, provided, securityContext, "The provided security context should not be modified in place")
	})

	t.Run("the container defaults are set", func(t *testing.T) {
		securityContext := defaultContainerSecurityContext(nil, operatorContainerOptions{}, nil)
		assert.False(t, *securityContext.AllowPrivilegeEscalation, "Privilege escalation should be disallowed")
		assert.True(t, *securityContext.RunAsNonRoot, "The container should run as non-root")
		assert.Equal(t, []corev1.Capability{"ALL"}, securityContext.Capabilities.Drop, "All capabilities should be dropped")
		assert.Nil(t, securityContext.RunAsUser, "A container whose image has its own user should not be given a runAsUser")
		assert.Nil(t, securityContext.ReadOnlyRootFilesystem, "readOnlyRootFilesystem is not part of the restricted profile")
	})

	t.Run("a root image is given a user", func(t *testing.T) {
		securityContext := defaultContainerSecurityContext(nil, operatorContainerOptions{imageRunsAsRoot: true}, nil)
		assert.True(t, *securityContext.RunAsNonRoot, "The container should run as non-root")
		assert.Equal(t, int64(DefaultSolrUser), *securityContext.RunAsUser, "A root image needs an explicit user to run as non-root")
	})

	t.Run("a root image inherits the pod's user", func(t *testing.T) {
		securityContext := defaultContainerSecurityContext(nil, operatorContainerOptions{imageRunsAsRoot: true}, ptr.To(int64(1234)))
		assert.Nil(t, securityContext.RunAsUser, "The container should inherit the user given at the pod level")
	})

	t.Run("the user wins at the container level", func(t *testing.T) {
		provided := &corev1.SecurityContext{
			RunAsNonRoot:             ptr.To(false),
			AllowPrivilegeEscalation: ptr.To(true),
			Capabilities:             &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}},
		}
		securityContext := defaultContainerSecurityContext(provided, operatorContainerOptions{}, nil)
		assert.False(t, *securityContext.RunAsNonRoot, "The provided runAsNonRoot should be kept")
		assert.True(t, *securityContext.AllowPrivilegeEscalation, "The provided allowPrivilegeEscalation should be kept")
		assert.Empty(t, securityContext.Capabilities.Drop, "The provided capabilities should be kept as they are")
		assert.NotSame(t, provided, securityContext, "The provided security context should not be modified in place")
	})

	t.Run("asking for root turns off the conflicting defaults", func(t *testing.T) {
		provided := &corev1.SecurityContext{RunAsUser: ptr.To(int64(0))}
		securityContext := defaultContainerSecurityContext(provided, operatorContainerOptions{imageRunsAsRoot: true}, nil)
		assert.Nil(t, securityContext.RunAsNonRoot, "A container asked to run as root cannot also run as non-root")
		assert.Nil(t, securityContext.Capabilities, "A container asked to run as root keeps the capabilities it needs")
		assert.False(t, *securityContext.AllowPrivilegeEscalation, "Privilege escalation should still be disallowed")
	})

	t.Run("readOnlyRootFilesystem is kept for the containers that had it", func(t *testing.T) {
		statefulSet := testStatefulSet(testSolrCloudForSecurityContext(), nil, nil)
		setupZk := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, SetupZkInitContainer)
		assert.True(t, *setupZk.SecurityContext.ReadOnlyRootFilesystem, "The setup-zk init container should keep its read-only root filesystem")
	})
}

func TestBackupRepositoryPreparationRunsAsSolrByDefault(t *testing.T) {
	solrCloudWithRepo := func() *solr.SolrCloud {
		solrCloud := testSolrCloudForSecurityContext()
		solrCloud.Spec.BackupRepositories = []solr.SolrBackupRepository{
			{
				Name:   "test-repo",
				Volume: &solr.VolumeRepository{Source: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		}
		return solrCloud
	}

	t.Run("by default the directory is only checked", func(t *testing.T) {
		statefulSet := testStatefulSet(solrCloudWithRepo(), nil, nil)
		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, VolumePrepInitContainer)
		command := strings.Join(initContainer.Command, " ")

		assert.Contains(t, command, "test -w /var/solr/data/backup-restore/test-repo", "The backup directory should be checked for writability")
		assert.NotContains(t, command, "chown", "A non-root container cannot take ownership of the backup directory")
		assert.NotContains(t, command, "adduser", "A non-root container cannot add users")
		assert.Equal(t, int64(DefaultSolrUser), *initContainer.SecurityContext.RunAsUser, "The init container should run as the Solr user")
		assert.True(t, *initContainer.SecurityContext.RunAsNonRoot, "The init container should run as non-root")
	})

	t.Run("running as root takes ownership of the directory", func(t *testing.T) {
		solrCloud := solrCloudWithRepo()
		solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{
			DefaultInitContainerSecurityContext: &corev1.SecurityContext{RunAsUser: ptr.To(int64(0))},
		}
		statefulSet := testStatefulSet(solrCloud, nil, nil)
		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, VolumePrepInitContainer)
		command := strings.Join(initContainer.Command, " ")

		assert.Contains(t, command, "chown -R 8983:8983 /var/solr/data/backup-restore/test-repo", "A root container should take ownership of the backup directory")
		assert.Contains(t, command, "adduser", "A root container should add the Solr user to check writability as it")
		assert.Equal(t, int64(0), *initContainer.SecurityContext.RunAsUser, "The init container should run as root")
		assert.Nil(t, initContainer.SecurityContext.RunAsNonRoot, "A root init container cannot run as non-root")
	})

	t.Run("the pod's user is used for the check", func(t *testing.T) {
		solrCloud := solrCloudWithRepo()
		solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{
			PodSecurityContext: &corev1.PodSecurityContext{RunAsUser: ptr.To(int64(1234))},
		}
		statefulSet := testStatefulSet(solrCloud, nil, nil)
		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, VolumePrepInitContainer)

		assert.Contains(t, strings.Join(initContainer.Command, " "), "user 1234", "The error message should name the user that the check ran as")
		assert.Nil(t, initContainer.SecurityContext.RunAsUser, "The init container should inherit the user given at the pod level")
	})
}

func containerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i, container := range containers {
		names[i] = container.Name
	}
	return names
}

func findContainer(t *testing.T, containers []corev1.Container, name string) corev1.Container {
	t.Helper()
	for _, container := range containers {
		if container.Name == name {
			return container
		}
	}
	require.Fail(t, "Could not find the container "+name, "Containers: %v", containerNames(containers))
	return corev1.Container{}
}
