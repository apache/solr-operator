/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// StatefulSetOptions defines custom options for StatefulSets
type StatefulSetOptions struct {
	// Annotations to be added for the StatefulSet.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added for the StatefulSet.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// DeploymentOptions defines custom options for Deployments
type DeploymentOptions struct {
	// Annotations to be added for the Deployment.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added for the Deployment.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// PodOptions defines the common pod configuration for Pods, including when used
// in deployments, stateful-sets, etc.
type PodOptions struct {
	// The scheduling constraints on pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Resources is the resource requirements for the container.
	// This field cannot be updated once the cluster is created.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Additional non-data volumes to load into the default container.
	// +optional
	Volumes []AdditionalVolume `json:"volumes,omitempty"`

	// PodSecurityContext is the security context for the pod.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// Additional environment variables to pass to the default container.
	// +optional
	EnvVariables []corev1.EnvVar `json:"envVars,omitempty"`

	// Annotations to be added for pods.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added for pods.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Tolerations to be added for the StatefulSet.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Node Selector to be added for the StatefulSet.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Liveness probe parameters
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// Readiness probe parameters
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Startup probe parameters
	// +optional
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`
}

// ServiceOptions defines custom options for services
type ServiceOptions struct {
	// Annotations to be added for the Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added for the Service.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// IngressOptions defines custom options for ingresses
type IngressOptions struct {
	// Annotations to be added for the Ingress.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added for the Ingress.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// ConfigMapOptions defines custom options for configMaps
type ConfigMapOptions struct {
	// Annotations to be added for the ConfigMap.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added for the ConfigMap.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// AdditionalVolume provides information on additional volumes that should be loaded into pods
type AdditionalVolume struct {
	// Name of the volume
	Name string `json:"name"`

	// Source is the source of the Volume to be loaded into the solrCloud Pod
	Source corev1.VolumeSource `json:"source,omitempty"`

	// DefaultContainerMount defines how to mount this volume into the default container.
	DefaultContainerMount corev1.VolumeMount `json:"defaultContainerMount"`
}

// ContainerImage defines the fields needed for a Docker repository image. The
// format here matches the predominant format used in Helm charts.
type ContainerImage struct {
	// +optional
	Repository string `json:"repository,omitempty"`
	// +optional
	Tag string `json:"tag,omitempty"`
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
}

func (c *ContainerImage) withDefaults(repo string, version string, policy corev1.PullPolicy) (changed bool) {
	if c.Repository == "" {
		changed = true
		c.Repository = repo
	}
	if c.Tag == "" {
		changed = true
		c.Tag = version
	}
	if c.PullPolicy == "" {
		changed = true
		c.PullPolicy = policy
	}
	return changed
}

func (c *ContainerImage) ToImageName() (name string) {
	return c.Repository + ":" + c.Tag
}

func ImageVersion(image string) (version string) {
	split := strings.Split(image, ":")
	if len(split) < 2 {
		return ""
	} else {
		return split[1]
	}
}
