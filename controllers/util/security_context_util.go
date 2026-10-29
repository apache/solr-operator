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
	solr "github.com/apache/solr-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// Names of the containers and init containers that the Solr Operator generates.
const (
	VolumePrepInitContainer       = "volume-prep"
	SetupZkInitContainer          = "setup-zk"
	Pkcs12InitContainer           = "gen-pkcs12-keystore"
	TLSWrapperScriptInitContainer = "create-tls-wrapper-script"
	// InitdbInitContainer is defined in solr_tls_util.go
)

// operatorContainerOptions describes a container that the operator generates.
type operatorContainerOptions struct {
	// imageRunsAsRoot is true if the container's image has no non-root user of its own.
	// The "restricted" profile requires runAsNonRoot, which the kubelet refuses to honor for such
	// an image unless a runAsUser is also given, so these containers get one.
	imageRunsAsRoot bool

	// readOnlyRootFilesystem is true if the container is known not to write to its root filesystem.
	// This is not part of the "restricted" profile and is not applied to containers that did not
	// already have it, see https://github.com/apache/solr-operator/issues/801.
	readOnlyRootFilesystem bool
}

// operatorContainers lists every container and init container that the operator generates, and is
// therefore responsible for hardening. Containers the user supplies through podOptions are not
// listed: their security context is the user's to set.
//
// A container missing from this map gets no security context, which makes the pod fail "restricted"
// evaluation. The unit tests evaluate the generated pods against the profile, so a new operator
// container that is not listed here fails the build.
var operatorContainers = map[string]operatorContainerOptions{
	SolrNodeContainer:               {},
	SolrPrometheusExporterContainer: {},
	SetupZkInitContainer:            {readOnlyRootFilesystem: true},
	Pkcs12InitContainer:             {},
	VolumePrepInitContainer:         {imageRunsAsRoot: true},
	InitdbInitContainer:             {imageRunsAsRoot: true},
	TLSWrapperScriptInitContainer:   {imageRunsAsRoot: true},
}

// defaultPodSecurityContext builds the pod-level security context, starting from the one the user
// provided, if any, and filling in every field they left unset. User-provided fields always win.
//
// The "restricted" profile asks for a seccomp profile, which is set once here rather than on every
// container. fsGroup is not part of the profile, it is a long-standing operator default.
func defaultPodSecurityContext(userProvided *corev1.PodSecurityContext, defaultFSGroup int64) *corev1.PodSecurityContext {
	securityContext := &corev1.PodSecurityContext{}
	if userProvided != nil {
		securityContext = userProvided.DeepCopy()
	}

	if securityContext.SeccompProfile == nil {
		securityContext.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	}
	if securityContext.FSGroup == nil {
		securityContext.FSGroup = ptr.To(defaultFSGroup)
	}

	return securityContext
}

// defaultContainerSecurityContext builds the security context for one operator-generated container,
// starting from the one the user provided, if any, and filling in every field they left unset.
// User-provided fields always win, so a single field is enough to opt out of a single default.
//
// podRunAsUser is the pod-level runAsUser, which containers inherit when they do not set their own.
func defaultContainerSecurityContext(userProvided *corev1.SecurityContext, options operatorContainerOptions, podRunAsUser *int64) *corev1.SecurityContext {
	securityContext := &corev1.SecurityContext{}
	if userProvided != nil {
		securityContext = userProvided.DeepCopy()
	}

	// The user can ask for a container to run as root, which some setups need (see the backup
	// repository preparation in generateSolrSetupInitContainers). Such a container cannot also run
	// as non-root, and dropping all capabilities would leave it unable to do the work root was
	// asked for, so neither default is applied to it.
	effectiveRunAsUser := securityContext.RunAsUser
	if effectiveRunAsUser == nil {
		effectiveRunAsUser = podRunAsUser
	}
	runsAsRoot := effectiveRunAsUser != nil && *effectiveRunAsUser == 0

	if securityContext.AllowPrivilegeEscalation == nil {
		securityContext.AllowPrivilegeEscalation = ptr.To(false)
	}
	if !runsAsRoot {
		if securityContext.RunAsNonRoot == nil {
			securityContext.RunAsNonRoot = ptr.To(true)
		}
		if securityContext.Capabilities == nil {
			securityContext.Capabilities = &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}
		}
		if options.imageRunsAsRoot && effectiveRunAsUser == nil {
			securityContext.RunAsUser = ptr.To(int64(DefaultSolrUser))
		}
	}
	if options.readOnlyRootFilesystem && securityContext.ReadOnlyRootFilesystem == nil {
		securityContext.ReadOnlyRootFilesystem = ptr.To(true)
	}

	return securityContext
}

// setSecurityContextOnOperatorContainers sets the security context on every container and init
// container that the operator generated for the given pod, leaving user-supplied containers alone.
//
// Call this once the pod spec is otherwise complete, so that containers added along the way (for
// TLS, for example) are covered too.
func setSecurityContextOnOperatorContainers(podSpec *corev1.PodSpec, containerSecurityContext *corev1.SecurityContext, initContainerSecurityContext *corev1.SecurityContext) {
	var podRunAsUser *int64
	if podSpec.SecurityContext != nil {
		podRunAsUser = podSpec.SecurityContext.RunAsUser
	}

	for i, container := range podSpec.Containers {
		if options, isOperatorContainer := operatorContainers[container.Name]; isOperatorContainer {
			podSpec.Containers[i].SecurityContext = defaultContainerSecurityContext(containerSecurityContext, options, podRunAsUser)
		}
	}
	for i, container := range podSpec.InitContainers {
		if options, isOperatorContainer := operatorContainers[container.Name]; isOperatorContainer {
			podSpec.InitContainers[i].SecurityContext = defaultContainerSecurityContext(initContainerSecurityContext, options, podRunAsUser)
		}
	}
}

// operatorInitContainersRunAsRoot reports whether the user has asked the operator's init containers
// to run as root, which is the opt-in for init container work that non-root users cannot do.
func operatorInitContainersRunAsRoot(podOptions *solr.PodOptions) bool {
	if podOptions == nil {
		return false
	}
	if securityContext := podOptions.DefaultInitContainerSecurityContext; securityContext != nil && securityContext.RunAsUser != nil {
		return *securityContext.RunAsUser == 0
	}
	if securityContext := podOptions.PodSecurityContext; securityContext != nil && securityContext.RunAsUser != nil {
		return *securityContext.RunAsUser == 0
	}
	return false
}
