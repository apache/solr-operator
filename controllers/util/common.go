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

package util

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strconv"
	"strings"
)

// CopyLabelsAndAnnotations copies the labels and annotations from one object to another.
// Additional Labels and Annotations in the 'to' object will not be removed.
// Returns true if there are updates required to the object.
func CopyLabelsAndAnnotations(from, to *metav1.ObjectMeta) (requireUpdate bool) {
	if len(to.Labels) == 0 && len(from.Labels) > 0 {
		to.Labels = make(map[string]string, len(from.Labels))
	}
	for k, v := range from.Labels {
		if to.Labels[k] != v {
			requireUpdate = true
			log.Info("Update Label", "label", k, "newValue", v, "oldValue", to.Labels[k])
			to.Labels[k] = v
		}
	}

	if len(to.Annotations) == 0 && len(from.Annotations) > 0 {
		to.Annotations = make(map[string]string, len(from.Annotations))
	}
	for k, v := range from.Annotations {
		if to.Annotations[k] != v {
			requireUpdate = true
			log.Info("Update Annotation", "annotation", k, "newValue", v, "oldValue", to.Annotations[k])
			to.Annotations[k] = v
		}
	}

	return requireUpdate
}

func DuplicateLabelsOrAnnotations(from map[string]string) map[string]string {
	to := make(map[string]string, len(from))
	for k, v := range from {
		to[k] = v
	}
	return to
}

func MergeLabelsOrAnnotations(base, additional map[string]string) map[string]string {
	merged := DuplicateLabelsOrAnnotations(base)
	for k, v := range additional {
		if _, alreadyExists := merged[k]; !alreadyExists {
			merged[k] = v
		}
	}
	return merged
}

// DeepEqualWithNils returns a deepEquals call that treats nil and zero-length maps, arrays and slices as the same.
func DeepEqualWithNils(x, y interface{}) bool {
	if (x == nil) != (y == nil) {
		// Make sure that x is not the nil value
		if x == nil {
			x = y
		}
		v := reflect.ValueOf(x)
		switch v.Kind() {
		case reflect.Array:
		case reflect.Map:
		case reflect.Slice:
			return v.Len() == 0
		}
	}
	return reflect.DeepEqual(x, y)
}

// ContainsString helper function to test string contains
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString helper function to remove string
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// IsPVCOrphan determines whether the given name represents a PVC that is an orphan, or no longer has a pod associated with it.
func IsPVCOrphan(pvcName string, replicas int32) bool {
	index := strings.LastIndexAny(pvcName, "-")
	if index == -1 {
		return false
	}

	ordinal, err := strconv.Atoi(pvcName[index+1:])
	if err != nil {
		return false
	}

	return int32(ordinal) >= replicas
}

// CopyConfigMapFields copies the owned fields from one ConfigMap to another
func CopyConfigMapFields(from, to *corev1.ConfigMap) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !DeepEqualWithNils(to.Data, from.Data) {
		requireUpdate = true
		log.Info("Update required because:", "Data changed from", to.Data, "To:", from.Data)
	}
	to.Data = from.Data

	return requireUpdate
}

// CopyServiceFields copies the owned fields from one Service to another
func CopyServiceFields(from, to *corev1.Service) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !DeepEqualWithNils(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Selector changed from", to.Spec.Selector, "To:", from.Spec.Selector)
	}
	to.Spec.Selector = from.Spec.Selector

	if !DeepEqualWithNils(to.Spec.Ports, from.Spec.Ports) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Ports changed from", to.Spec.Ports, "To:", from.Spec.Ports)
	}
	to.Spec.Ports = from.Spec.Ports

	if !DeepEqualWithNils(to.Spec.ExternalName, from.Spec.ExternalName) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.ExternalName changed from", to.Spec.ExternalName, "To:", from.Spec.ExternalName)
	}
	to.Spec.ExternalName = from.Spec.ExternalName

	if !DeepEqualWithNils(to.Spec.PublishNotReadyAddresses, from.Spec.PublishNotReadyAddresses) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.PublishNotReadyAddresses changed from", to.Spec.PublishNotReadyAddresses, "To:", from.Spec.PublishNotReadyAddresses)
	}
	to.Spec.PublishNotReadyAddresses = from.Spec.PublishNotReadyAddresses

	return requireUpdate
}

// CopyIngressFields copies the owned fields from one Ingress to another
func CopyIngressFields(from, to *extv1.Ingress) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	if !DeepEqualWithNils(to.Spec.Rules, from.Spec.Rules) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Rules changed from", to.Spec.Rules, "To:", from.Spec.Rules)
	}
	to.Spec.Rules = from.Spec.Rules

	return requireUpdate
}

// CopyStatefulSetFields copies the owned fields from one StatefulSet to another
// Returns true if the fields copied from don't match to.
func CopyStatefulSetFields(from, to *appsv1.StatefulSet) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	if !DeepEqualWithNils(to.Spec.Replicas, from.Spec.Replicas) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Replicas changed from", to.Spec.Replicas, "To:", from.Spec.Replicas)
		to.Spec.Replicas = from.Spec.Replicas
	}

	if !DeepEqualWithNils(to.Spec.UpdateStrategy, from.Spec.UpdateStrategy) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.UpdateStrategy changed from", to.Spec.UpdateStrategy, "To:", from.Spec.UpdateStrategy)
		to.Spec.UpdateStrategy = from.Spec.UpdateStrategy
	}

	/*
			Kubernetes does not currently support updates to these fields: Selector and PodManagementPolicy

		if !DeepEqualWithNils(to.Spec.Selector, from.Spec.Selector) {
			requireUpdate = true
			log.Info("Update required because:", "Spec.Selector changed from", to.Spec.Selector, "To:", from.Spec.Selector)
			to.Spec.Selector = from.Spec.Selector
		}

		if !DeepEqualWithNils(to.Spec.PodManagementPolicy, from.Spec.PodManagementPolicy) {
			requireUpdate = true
			log.Info("Update required because:", "Spec.PodManagementPolicy changed from", to.Spec.PodManagementPolicy, "To:", from.Spec.PodManagementPolicy)
			to.Spec.PodManagementPolicy = from.Spec.PodManagementPolicy
		}
	*/

	/*
			Kubernetes does not support modification of VolumeClaimTemplates currently. See:
		    https://github.com/kubernetes/enhancements/issues/661

		if len(from.Spec.VolumeClaimTemplates) > len(to.Spec.VolumeClaimTemplates) {
			requireUpdate = true
			log.Info("Update required because:", "Spec.VolumeClaimTemplates changed from", to.Spec.VolumeClaimTemplates, "To:", from.Spec.VolumeClaimTemplates)
			to.Spec.VolumeClaimTemplates = from.Spec.VolumeClaimTemplates
		}
		for i, fromVct := range from.Spec.VolumeClaimTemplates {
			if !DeepEqualWithNils(to.Spec.VolumeClaimTemplates[i].Name, fromVct.Name) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.VolumeClaimTemplates["+strconv.Itoa(i)+"].Name changed from", to.Spec.VolumeClaimTemplates[i].Name, "To:", fromVct.Name)
				to.Spec.VolumeClaimTemplates[i].Name = fromVct.Name
			}
			if !DeepEqualWithNils(to.Spec.VolumeClaimTemplates[i].Labels, fromVct.Labels) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.VolumeClaimTemplates["+strconv.Itoa(i)+"].Labels changed from", to.Spec.VolumeClaimTemplates[i].Labels, "To:", fromVct.Labels)
				to.Spec.VolumeClaimTemplates[i].Labels = fromVct.Labels
			}
			if !DeepEqualWithNils(to.Spec.VolumeClaimTemplates[i].Annotations, fromVct.Annotations) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.VolumeClaimTemplates["+strconv.Itoa(i)+"].Annotations changed from", to.Spec.VolumeClaimTemplates[i].Annotations, "To:", fromVct.Annotations)
				to.Spec.VolumeClaimTemplates[i].Annotations = fromVct.Annotations
			}
			if !DeepEqualWithNils(to.Spec.VolumeClaimTemplates[i].Spec, fromVct.Spec) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.VolumeClaimTemplates["+strconv.Itoa(i)+"].Spec changed from", to.Spec.VolumeClaimTemplates[i].Spec, "To:", fromVct.Spec)
				to.Spec.VolumeClaimTemplates[i].Spec = fromVct.Spec
			}
		}
	*/

	requireUpdate = requireUpdate || CopyPodTemplates(&from.Spec.Template, &to.Spec.Template, "Spec.Template")

	return requireUpdate
}

// CopyDeploymentFields copies the owned fields from one Deployment to another
// Returns true if the fields copied from don't match to.
func CopyDeploymentFields(from, to *appsv1.Deployment) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	if !DeepEqualWithNils(to.Spec.Replicas, from.Spec.Replicas) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Replicas changed from", to.Spec.Replicas, "To:", from.Spec.Replicas)
		to.Spec.Replicas = from.Spec.Replicas
	}

	if !DeepEqualWithNils(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Selector changed from", to.Spec.Selector, "To:", from.Spec.Selector)
		to.Spec.Selector = from.Spec.Selector
	}

	requireUpdate = requireUpdate || CopyPodTemplates(&from.Spec.Template, &to.Spec.Template, "Spec.Template")

	return requireUpdate
}

func CopyPodTemplates(from, to *corev1.PodTemplateSpec, basePath string) (requireUpdate bool) {
	if !DeepEqualWithNils(to.Labels, from.Labels) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Labels changed from", to.Labels, "To:", from.Labels)
		to.Labels = from.Labels
	}

	if !DeepEqualWithNils(to.Annotations, from.Annotations) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Annotations changed from", to.Annotations, "To:", from.Annotations)
		to.Annotations = from.Annotations
	}

	requireUpdate = requireUpdate || CopyPodContainers(from.Spec.Containers, to.Spec.Containers, basePath+".Spec.Containers")

	requireUpdate = requireUpdate || CopyPodContainers(from.Spec.InitContainers, to.Spec.InitContainers, basePath+".Spec.InitContainers")

	if !DeepEqualWithNils(to.Spec.HostAliases, from.Spec.HostAliases) {
		requireUpdate = true
		to.Spec.HostAliases = from.Spec.HostAliases
		log.Info("Update required because:", basePath+".Spec.HostAliases changed from", to.Spec.HostAliases, "To:", from.Spec.HostAliases)
	}

	if !DeepEqualWithNils(to.Spec.Volumes, from.Spec.Volumes) {
		requireUpdate = true
		to.Spec.Volumes = from.Spec.Volumes
		log.Info("Update required because:", basePath+".Spec.Volumes changed from", to.Spec.Volumes, "To:", from.Spec.Volumes)
	}

	if !DeepEqualWithNils(to.Spec.ImagePullSecrets, from.Spec.ImagePullSecrets) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Spec.ImagePullSecrets changed from", to.Spec.ImagePullSecrets, "To:", from.Spec.ImagePullSecrets)
		to.Spec.ImagePullSecrets = from.Spec.ImagePullSecrets
	}

	if !DeepEqualWithNils(to.Spec.Affinity, from.Spec.Affinity) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Spec.Affinity changed from", to.Spec.Affinity, "To:", from.Spec.Affinity)
		to.Spec.Affinity = from.Spec.Affinity
	}

	if !DeepEqualWithNils(to.Spec.SecurityContext, from.Spec.SecurityContext) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Spec.SecurityContext changed from", to.Spec.SecurityContext, "To:", from.Spec.SecurityContext)
		to.Spec.SecurityContext = from.Spec.SecurityContext
	}

	if !DeepEqualWithNils(to.Spec.NodeSelector, from.Spec.NodeSelector) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Spec.NodeSelector changed from", to.Spec.NodeSelector, "To:", from.Spec.NodeSelector)
		to.Spec.NodeSelector = from.Spec.NodeSelector
	}

	if !DeepEqualWithNils(to.Spec.Tolerations, from.Spec.Tolerations) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Spec.Tolerations changed from", to.Spec.Tolerations, "To:", from.Spec.Tolerations)
		to.Spec.Tolerations = from.Spec.Tolerations
	}

	if !DeepEqualWithNils(to.Spec.PriorityClassName, from.Spec.PriorityClassName) {
		requireUpdate = true
		log.Info("Update required because:", basePath+".Spec.PriorityClassName changed from", to.Spec.PriorityClassName, "To:", from.Spec.PriorityClassName)
		to.Spec.PriorityClassName = from.Spec.PriorityClassName
	}

	return requireUpdate
}

func CopyPodContainers(from, to []corev1.Container, basePath string) (requireUpdate bool) {
	if len(to) < len(from) {
		requireUpdate = true
		to = from
	} else {
		for i := 0; i < len(from); i++ {
			if !DeepEqualWithNils(to[i].Name, from[i].Name) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Name changed from", to[i].Name, "To:", from[i].Name)
				to[i].Name = from[i].Name
			}

			if !DeepEqualWithNils(to[i].Image, from[i].Image) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Image changed from", to[i].Image, "To:", from[i].Image)
				to[i].Image = from[i].Image
			}

			if from[i].ImagePullPolicy != "" && !DeepEqualWithNils(to[i].ImagePullPolicy, from[i].ImagePullPolicy) {
				// Only request an update if the requestedPullPolicy is not empty
				// Otherwise kubernetes will specify a defaultPollPolicy and the operator will endlessly recurse, trying to unset the default policy.
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].ImagePullPolicy changed from", to[i].ImagePullPolicy, "To:", from[i].ImagePullPolicy)
				to[i].ImagePullPolicy = from[i].ImagePullPolicy
			}

			if !DeepEqualWithNils(to[i].Command, from[i].Command) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Command changed from", to[i].Command, "To:", from[i].Command)
				to[i].Command = from[i].Command
			}

			if !DeepEqualWithNils(to[i].Args, from[i].Args) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Args changed from", to[i].Args, "To:", from[i].Args)
				to[i].Args = from[i].Args
			}

			if !DeepEqualWithNils(to[i].Env, from[i].Env) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Env changed from", to[i].Env, "To:", from[i].Env)
				to[i].Env = from[i].Env
			}

			if !DeepEqualWithNils(to[i].Resources, from[i].Resources) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Resources changed from", to[i].Resources, "To:", from[i].Resources)
				to[i].Resources = from[i].Resources
			}

			if !DeepEqualWithNils(to[i].VolumeMounts, from[i].VolumeMounts) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].VolumeMounts changed from", to[i].VolumeMounts, "To:", from[i].VolumeMounts)
				to[i].VolumeMounts = from[i].VolumeMounts
			}

			if !DeepEqualWithNils(to[i].Ports, from[i].Ports) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Ports changed from", to[i].Ports, "To:", from[i].Ports)
				to[i].Ports = from[i].Ports
			}

			if !DeepEqualWithNils(to[i].Lifecycle, from[i].Lifecycle) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].Lifecycle changed from", to[i].Lifecycle, "To:", from[i].Lifecycle)
				to[i].Lifecycle = from[i].Lifecycle
			}

			if !DeepEqualWithNils(to[i].LivenessProbe, from[i].LivenessProbe) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].LivenessProbe changed from", to[i].LivenessProbe, "To:", from[i].LivenessProbe)
				to[i].LivenessProbe = from[i].LivenessProbe
			}

			if !DeepEqualWithNils(to[i].ReadinessProbe, from[i].ReadinessProbe) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].ReadinessProbe changed from", to[i].ReadinessProbe, "To:", from[i].ReadinessProbe)
				to[i].LivenessProbe = from[i].ReadinessProbe
			}

			if !DeepEqualWithNils(to[i].StartupProbe, from[i].StartupProbe) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].StartupProbe changed from", to[i].StartupProbe, "To:", from[i].StartupProbe)
				to[i].StartupProbe = from[i].StartupProbe
			}

			if from[i].TerminationMessagePath != "" && !DeepEqualWithNils(to[i].TerminationMessagePath, from[i].TerminationMessagePath) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].TerminationMessagePath changed from", to[i].TerminationMessagePath, "To:", from[i].TerminationMessagePath)
				to[i].TerminationMessagePath = from[i].TerminationMessagePath
			}

			if from[i].TerminationMessagePolicy != "" && !DeepEqualWithNils(to[i].TerminationMessagePolicy, from[i].TerminationMessagePolicy) {
				requireUpdate = true
				log.Info("Update required because:", basePath+".Containers["+strconv.Itoa(i)+")].TerminationMessagePolicy changed from", to[i].TerminationMessagePolicy, "To:", from[i].TerminationMessagePolicy)
				to[i].TerminationMessagePolicy = from[i].TerminationMessagePolicy
			}
		}
	}
	return requireUpdate
}
