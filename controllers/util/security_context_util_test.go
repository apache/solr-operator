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

	t.Run("with TLS from a secret", func(t *testing.T) {
		exporter := exporterWithDefaults()
		exporter.Spec.SolrReference.SolrTLS = &solr.SolrTLSOptions{
			PKCS12Secret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "tls-secret"},
				Key:                  "keystore.p12",
			},
			KeyStorePasswordSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "tls-secret"},
				Key:                  "password",
			},
		}
		tls := TLSCertsForExporter(exporter)
		// Force the keystore conversion init container, the exporter's other generated container
		tls.ClientConfig.NeedsPkcs12InitContainer = true
		deployment := GenerateSolrPrometheusExporterDeployment(exporter, SolrConnectionInfo{StandaloneAddress: "http://localhost:8983/solr"}, nil, "", tls, "")

		initContainerNames := containerNames(deployment.Spec.Template.Spec.InitContainers)
		assert.Contains(t, initContainerNames, Pkcs12InitContainer, "The keystore conversion init container should be generated")
		expectRestrictedProfile(t, &deployment.Spec.Template, "A Prometheus exporter pod with TLS from a secret")
	})

	t.Run("the exporter container is hardened and overridable", func(t *testing.T) {
		deployment := GenerateSolrPrometheusExporterDeployment(exporterWithDefaults(), SolrConnectionInfo{StandaloneAddress: "http://localhost:8983/solr"}, nil, "", nil, "")
		exporterContainer := findContainer(t, deployment.Spec.Template.Spec.Containers, SolrPrometheusExporterContainer)
		assert.True(t, *exporterContainer.SecurityContext.RunAsNonRoot, "The exporter container should run as non-root")
		assert.Equal(t, []corev1.Capability{"ALL"}, exporterContainer.SecurityContext.Capabilities.Drop, "The exporter container should drop all capabilities")

		overridden := exporterWithDefaults()
		overridden.Spec.CustomKubeOptions.PodOptions = &solr.PodOptions{
			ContainerSecurityContext: &corev1.SecurityContext{RunAsNonRoot: ptr.To(false)},
		}
		deployment = GenerateSolrPrometheusExporterDeployment(overridden, SolrConnectionInfo{StandaloneAddress: "http://localhost:8983/solr"}, nil, "", nil, "")
		exporterContainer = findContainer(t, deployment.Spec.Template.Spec.Containers, SolrPrometheusExporterContainer)
		assert.False(t, *exporterContainer.SecurityContext.RunAsNonRoot, "The provided runAsNonRoot should reach the exporter container")
		assert.Equal(t, []corev1.Capability{"ALL"}, exporterContainer.SecurityContext.Capabilities.Drop, "The other defaults should still apply to the exporter container")
	})
}

// TestOverridingOneFieldKeepsThePodCompliant is the common case for the merging behaviour: a user
// sets something unrelated to the profile and the pod the operator generates still satisfies it.
func TestOverridingOneFieldKeepsThePodCompliant(t *testing.T) {
	solrCloud := testSolrCloudForSecurityContext()
	solrCloud.Spec.BackupRepositories = []solr.SolrBackupRepository{
		{
			Name:   "test-repo",
			Volume: &solr.VolumeRepository{Source: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}
	solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{
		PodSecurityContext:                  &corev1.PodSecurityContext{RunAsGroup: ptr.To(int64(2345))},
		ContainerSecurityContext:            &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(true)},
		DefaultInitContainerSecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(true)},
	}
	statefulSet := testStatefulSet(solrCloud, nil, nil)

	assert.Equal(t, int64(2345), *statefulSet.Spec.Template.Spec.SecurityContext.RunAsGroup, "The provided runAsGroup should be kept")
	expectRestrictedProfile(t, &statefulSet.Spec.Template, "A SolrCloud pod with unrelated security context fields set")
}

// TestInitContainerOverrideReachesTheTLSInitContainers covers the init containers that the TLS code
// adds after the rest of the pod has been assembled, which the override never reached before.
func TestInitContainerOverrideReachesTheTLSInitContainers(t *testing.T) {
	provided := &corev1.SecurityContext{RunAsUser: ptr.To(int64(4321))}

	t.Run("the keystore conversion container", func(t *testing.T) {
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
		solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{DefaultInitContainerSecurityContext: provided}
		tls := TLSCertsForSolrCloud(solrCloud)
		tls.ServerConfig.NeedsPkcs12InitContainer = true
		statefulSet := testStatefulSet(solrCloud, tls, nil)

		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, Pkcs12InitContainer)
		assert.Equal(t, int64(4321), *initContainer.SecurityContext.RunAsUser, "The override should reach the keystore conversion init container")
		assert.True(t, *initContainer.SecurityContext.RunAsNonRoot, "The defaults should still apply to it")
	})

	t.Run("the initdb container", func(t *testing.T) {
		solrCloud := testSolrCloudForSecurityContext()
		solrCloud.Spec.SolrTLS = &solr.SolrTLSOptions{
			MountedTLSDir: &solr.MountedTLSDirectory{Path: "/var/solr/tls"},
		}
		solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{DefaultInitContainerSecurityContext: provided}
		statefulSet := testStatefulSet(solrCloud, TLSCertsForSolrCloud(solrCloud), nil)

		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, InitdbInitContainer)
		assert.Equal(t, int64(4321), *initContainer.SecurityContext.RunAsUser, "The override should reach the initdb init container")
		assert.True(t, *initContainer.SecurityContext.RunAsNonRoot, "The defaults should still apply to it")
	})

	t.Run("the exporter's TLS wrapper script container", func(t *testing.T) {
		exporter := &solr.SolrPrometheusExporter{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"}}
		exporter.Spec.SolrReference.SolrTLS = &solr.SolrTLSOptions{
			MountedTLSDir: &solr.MountedTLSDirectory{Path: "/var/solr/tls"},
		}
		exporter.Spec.CustomKubeOptions.PodOptions = &solr.PodOptions{DefaultInitContainerSecurityContext: provided}
		exporter.WithDefaults()
		deployment := GenerateSolrPrometheusExporterDeployment(exporter, SolrConnectionInfo{StandaloneAddress: "http://localhost:8983/solr"}, nil, "", TLSCertsForExporter(exporter), "")

		initContainer := findContainer(t, deployment.Spec.Template.Spec.InitContainers, TLSWrapperScriptInitContainer)
		assert.Equal(t, int64(4321), *initContainer.SecurityContext.RunAsUser, "The override should reach the TLS wrapper script init container")
		assert.True(t, *initContainer.SecurityContext.RunAsNonRoot, "The defaults should still apply to it")
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

	t.Run("capabilities are overridden as a whole", func(t *testing.T) {
		// "capabilities" is one field of a securityContext, so providing it replaces the default
		// rather than merging into it. An empty block is therefore how a user keeps the container's
		// default capabilities, which the "restricted" profile does not allow.
		provided := &corev1.SecurityContext{Capabilities: &corev1.Capabilities{}}
		securityContext := defaultContainerSecurityContext(provided, operatorContainerOptions{}, nil)
		assert.Empty(t, securityContext.Capabilities.Drop, "An empty capabilities block should not have the default dropped capabilities added to it")
	})

	t.Run("asking for privileged mode turns off the conflicting default", func(t *testing.T) {
		provided := &corev1.SecurityContext{Privileged: ptr.To(true)}
		securityContext := defaultContainerSecurityContext(provided, operatorContainerOptions{}, nil)
		assert.Nil(t, securityContext.AllowPrivilegeEscalation, "Kubernetes rejects allowPrivilegeEscalation: false on a privileged container")
	})

	t.Run("adding CAP_SYS_ADMIN turns off the conflicting default", func(t *testing.T) {
		provided := &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"CAP_SYS_ADMIN"}}}
		securityContext := defaultContainerSecurityContext(provided, operatorContainerOptions{}, nil)
		assert.Nil(t, securityContext.AllowPrivilegeEscalation, "Kubernetes rejects allowPrivilegeEscalation: false alongside an added CAP_SYS_ADMIN")
	})

	t.Run("an explicit allowPrivilegeEscalation is kept alongside privileged mode", func(t *testing.T) {
		provided := &corev1.SecurityContext{Privileged: ptr.To(true), AllowPrivilegeEscalation: ptr.To(true)}
		securityContext := defaultContainerSecurityContext(provided, operatorContainerOptions{}, nil)
		assert.True(t, *securityContext.AllowPrivilegeEscalation, "The provided allowPrivilegeEscalation should be kept")
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

	t.Run("the failure reason reaches the pod status", func(t *testing.T) {
		statefulSet := testStatefulSet(solrCloudWithRepo(), nil, nil)
		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, VolumePrepInitContainer)

		assert.Contains(t, strings.Join(initContainer.Command, " "), ">&2", "The failure message should go to stderr")
		assert.Equal(t, corev1.TerminationMessageFallbackToLogsOnError, initContainer.TerminationMessagePolicy, "The failure message should show up in the pod status, not only in the container log")
	})

	t.Run("the Solr user is only added once for several repositories", func(t *testing.T) {
		solrCloud := solrCloudWithRepo()
		solrCloud.Spec.BackupRepositories = append(solrCloud.Spec.BackupRepositories, solr.SolrBackupRepository{
			Name:   "second-repo",
			Volume: &solr.VolumeRepository{Source: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		})
		solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{
			DefaultInitContainerSecurityContext: &corev1.SecurityContext{RunAsUser: ptr.To(int64(0))},
		}
		statefulSet := testStatefulSet(solrCloud, nil, nil)
		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, VolumePrepInitContainer)
		command := strings.Join(initContainer.Command, " ")

		assert.Equal(t, 1, strings.Count(command, "adduser"), "The Solr user should only be added once, however many repositories there are")
		assert.Contains(t, command, "chown -R 8983:8983 /var/solr/data/backup-restore/test-repo", "The first repository directory should be prepared")
		assert.Contains(t, command, "chown -R 8983:8983 /var/solr/data/backup-restore/second-repo", "The second repository directory should be prepared")
		assert.Len(t, initContainer.VolumeMounts, 2, "Both repository volumes should be mounted")
	})

	t.Run("every repository is checked when running as the Solr user", func(t *testing.T) {
		solrCloud := solrCloudWithRepo()
		solrCloud.Spec.BackupRepositories = append(solrCloud.Spec.BackupRepositories, solr.SolrBackupRepository{
			Name:   "second-repo",
			Volume: &solr.VolumeRepository{Source: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		})
		statefulSet := testStatefulSet(solrCloud, nil, nil)
		initContainer := findContainer(t, statefulSet.Spec.Template.Spec.InitContainers, VolumePrepInitContainer)
		command := strings.Join(initContainer.Command, " ")

		assert.Contains(t, command, "test -w /var/solr/data/backup-restore/test-repo", "The first repository directory should be checked")
		assert.Contains(t, command, "test -w /var/solr/data/backup-restore/second-repo", "The second repository directory should be checked")
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

// TestUserSuppliedContainersKeepTheirSecurityContext covers the case where a container the user adds
// happens to be named after one the operator generates for other pods. The name alone must not be
// enough for the operator's defaults to replace the security context the user gave it.
func TestUserSuppliedContainersKeepTheirSecurityContext(t *testing.T) {
	userSecurityContext := &corev1.SecurityContext{
		RunAsUser:    ptr.To(int64(1234)),
		Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}},
	}

	solrCloud := testSolrCloudForSecurityContext()
	solrCloud.Spec.CustomSolrKubeOptions.PodOptions = &solr.PodOptions{
		SidecarContainers: []corev1.Container{
			{Name: SolrPrometheusExporterContainer, SecurityContext: userSecurityContext},
		},
		// "volume-prep" is only generated for a SolrCloud with a volume backupRepository, which
		// this one does not have, so the name is free for the user to take.
		InitContainers: []corev1.Container{
			{Name: VolumePrepInitContainer, SecurityContext: userSecurityContext},
		},
		ContainerSecurityContext:            &corev1.SecurityContext{RunAsUser: ptr.To(int64(4321))},
		DefaultInitContainerSecurityContext: &corev1.SecurityContext{RunAsUser: ptr.To(int64(4321))},
	}
	statefulSet := testStatefulSet(solrCloud, nil, nil)
	podSpec := statefulSet.Spec.Template.Spec

	sidecar := findContainer(t, podSpec.Containers, SolrPrometheusExporterContainer)
	assert.Equal(t, userSecurityContext, sidecar.SecurityContext, "A user-supplied sidecar should keep the security context it was given")

	initContainer := findContainer(t, podSpec.InitContainers, VolumePrepInitContainer)
	assert.Equal(t, userSecurityContext, initContainer.SecurityContext, "A user-supplied init container should keep the security context it was given")

	solrContainer := findContainer(t, podSpec.Containers, SolrNodeContainer)
	assert.Equal(t, int64(4321), *solrContainer.SecurityContext.RunAsUser, "The operator's own containers should still be hardened")
	setupZk := findContainer(t, podSpec.InitContainers, SetupZkInitContainer)
	assert.Equal(t, int64(4321), *setupZk.SecurityContext.RunAsUser, "The operator's own init containers should still be hardened")
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
