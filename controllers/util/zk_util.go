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
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strconv"
)

var log = logf.Log.WithName("controller")

// GenerateZookeeperCluster returns a new ZookeeperCluster pointer generated for the SolrCloud instance
// object: SolrCloud instance
// zkSpec: the spec of the ZookeeperCluster to generate
func GenerateZookeeperCluster(solrCloud *solr.SolrCloud, zkSpec *solr.ZookeeperSpec) *zk.ZookeeperCluster {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["technology"] = solr.ZookeeperTechnologyLabel

	zkCluster := &zk.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.ProvidedZookeeperName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: zk.ZookeeperClusterSpec{
			Image: zk.ContainerImage{
				Repository: zkSpec.Image.Repository,
				Tag:        zkSpec.Image.Tag,
				PullPolicy: zkSpec.Image.PullPolicy,
			},
			Labels:      labels,
			Replicas:    *zkSpec.Replicas,
			Persistence: zkSpec.Persistence,
			Ports: []corev1.ContainerPort{
				{
					Name:          "client",
					ContainerPort: 2181,
				},
				{
					Name:          "quorum",
					ContainerPort: 2888,
				},
				{
					Name:          "leader-election",
					ContainerPort: 3888,
				},
			},
		},
	}

	// Append Pod Policies if provided by user
	if zkSpec.ZookeeperPod.Affinity != nil {
		zkCluster.Spec.Pod.Affinity = zkSpec.ZookeeperPod.Affinity
	}

	if zkSpec.ZookeeperPod.Resources.Limits != nil || zkSpec.ZookeeperPod.Resources.Requests != nil {
		zkCluster.Spec.Pod.Resources = zkSpec.ZookeeperPod.Resources
	}

	if zkSpec.ZookeeperPod.Tolerations != nil {
		zkCluster.Spec.Pod.Tolerations = zkSpec.ZookeeperPod.Tolerations
	}

	if zkSpec.ZookeeperPod.NodeSelector != nil {
		zkCluster.Spec.Pod.NodeSelector = zkSpec.ZookeeperPod.NodeSelector
	}

	return zkCluster
}

// CopyZookeeperClusterFields copies the owned fields from one ZookeeperCluster to another
// Returns true if the fields copied from don't match to.
func CopyZookeeperClusterFields(from, to *zk.ZookeeperCluster) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	if !DeepEqualWithNils(to.Spec.Replicas, from.Spec.Replicas) {
		log.Info("Updating Zk replicas")
		requireUpdate = true
	}
	to.Spec.Replicas = from.Spec.Replicas

	if !DeepEqualWithNils(to.Spec.Image.Repository, from.Spec.Image.Repository) {
		log.Info("Updating Zk image repository")
		requireUpdate = true
	}
	to.Spec.Image.Repository = from.Spec.Image.Repository

	if !DeepEqualWithNils(to.Spec.Image.Tag, from.Spec.Image.Tag) {
		log.Info("Updating Zk image tag")
		requireUpdate = true
	}
	to.Spec.Image.Tag = from.Spec.Image.Tag

	if from.Spec.Persistence != nil {
		if to.Spec.Persistence == nil {
			log.Info("Updating Zk Persistence")
			requireUpdate = true
			to.Spec.Persistence = from.Spec.Persistence
		} else {
			if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests, from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests) {
				log.Info("Updating Zk Persistence PVC Requests")
				requireUpdate = true
				to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests = from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests
			}

			if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes, from.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes) {
				log.Info("Updating Zk Persistence PVC AccessModes")
				requireUpdate = true
				to.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes = from.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes
			}

			if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName, from.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName) {
				log.Info("Updating Zk Persistence PVC StorageClassName")
				requireUpdate = true
				to.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName = from.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName
			}

			if !DeepEqualWithNils(to.Spec.Persistence.VolumeReclaimPolicy, from.Spec.Persistence.VolumeReclaimPolicy) {
				log.Info("Updating Zk Persistence VolumeReclaimPolicy")
				requireUpdate = true
				to.Spec.Persistence.VolumeReclaimPolicy = from.Spec.Persistence.VolumeReclaimPolicy
			}
		}
	}
	/* Uncomment when the following PR is merged in: https://github.com/pravega/zookeeper-operator/pull/64
	   Otherwise the ZK Operator will create persistence when none is given, and this will infinitely loop.
	else if to.Spec.Persistence != nil {
		requireUpdate = true
		to.Spec.Persistence = nil
	}*/

	if !DeepEqualWithNils(to.Spec.Pod.Resources, from.Spec.Pod.Resources) {
		log.Info("Updating Zk pod resources")
		requireUpdate = true
		to.Spec.Pod.Resources = from.Spec.Pod.Resources
	}

	if !DeepEqualWithNils(to.Spec.Pod.Tolerations, from.Spec.Pod.Tolerations) {
		log.Info("Updating Zk tolerations")
		log.Info("Update required because:", "Spec.Pod.Tolerations canged from", to.Spec.Pod.Tolerations, "To:", from.Spec.Pod.Tolerations)
		requireUpdate = true
		to.Spec.Pod.Tolerations = from.Spec.Pod.Tolerations
	}

	if !DeepEqualWithNils(to.Spec.Pod.NodeSelector, from.Spec.Pod.NodeSelector) {
		log.Info("Updating Zk nodeSelector")
		log.Info("Update required because:", "Spec.Pod.NodeSelector canged from", to.Spec.Pod.NodeSelector, "To:", from.Spec.Pod.NodeSelector)
		requireUpdate = true
		to.Spec.Pod.NodeSelector = from.Spec.Pod.NodeSelector
	}

	if !DeepEqualWithNils(to.Spec.Pod.Affinity, from.Spec.Pod.Affinity) {
		log.Info("Updating Zk pod affinity")
		log.Info("Update required because:", "Spec.Pod.Affinity canged from", to.Spec.Pod.Affinity, "To:", from.Spec.Pod.Affinity)
		requireUpdate = true
		to.Spec.Pod.Affinity = from.Spec.Pod.Affinity
	}

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

	if !DeepEqualWithNils(to.Spec.Template.Labels, from.Spec.Template.Labels) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Labels changed from", to.Spec.Template.Labels, "To:", from.Spec.Template.Labels)
		to.Spec.Template.Labels = from.Spec.Template.Labels
	}

	if !DeepEqualWithNils(to.Spec.Template.Annotations, from.Spec.Template.Annotations) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Annotations changed from", to.Spec.Template.Annotations, "To:", from.Spec.Template.Annotations)
		to.Spec.Template.Annotations = from.Spec.Template.Annotations
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.Volumes, from.Spec.Template.Spec.Volumes) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Volumes changed from", to.Spec.Template.Spec.Volumes, "To:", from.Spec.Template.Spec.Volumes)
		to.Spec.Template.Spec.Volumes = from.Spec.Template.Spec.Volumes
	}

	if len(to.Spec.Template.Spec.Containers) != len(from.Spec.Template.Spec.Containers) {
		requireUpdate = true
		to.Spec.Template.Spec.Containers = from.Spec.Template.Spec.Containers
	} else if !DeepEqualWithNils(to.Spec.Template.Spec.Containers, from.Spec.Template.Spec.Containers) {
		for i := 0; i < len(to.Spec.Template.Spec.Containers); i++ {
			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].Name, from.Spec.Template.Spec.Containers[i].Name) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].Name changed from", to.Spec.Template.Spec.Containers[i].Name, "To:", from.Spec.Template.Spec.Containers[i].Name)
				to.Spec.Template.Spec.Containers[i].Name = from.Spec.Template.Spec.Containers[i].Name
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].Image, from.Spec.Template.Spec.Containers[i].Image) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].Image changed from", to.Spec.Template.Spec.Containers[i].Image, "To:", from.Spec.Template.Spec.Containers[i].Image)
				to.Spec.Template.Spec.Containers[i].Image = from.Spec.Template.Spec.Containers[i].Image
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].ImagePullPolicy, from.Spec.Template.Spec.Containers[i].ImagePullPolicy) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].ImagePullPolicy changed from", to.Spec.Template.Spec.Containers[i].ImagePullPolicy, "To:", from.Spec.Template.Spec.Containers[i].ImagePullPolicy)
				to.Spec.Template.Spec.Containers[i].ImagePullPolicy = from.Spec.Template.Spec.Containers[i].ImagePullPolicy
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].Command, from.Spec.Template.Spec.Containers[i].Command) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].Command changed from", to.Spec.Template.Spec.Containers[i].Command, "To:", from.Spec.Template.Spec.Containers[i].Command)
				to.Spec.Template.Spec.Containers[i].Command = from.Spec.Template.Spec.Containers[i].Command
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].Args, from.Spec.Template.Spec.Containers[i].Args) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].Args changed from", to.Spec.Template.Spec.Containers[i].Args, "To:", from.Spec.Template.Spec.Containers[i].Args)
				to.Spec.Template.Spec.Containers[i].Args = from.Spec.Template.Spec.Containers[i].Args
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].Env, from.Spec.Template.Spec.Containers[i].Env) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].Env changed from", to.Spec.Template.Spec.Containers[i].Env, "To:", from.Spec.Template.Spec.Containers[i].Env)
				to.Spec.Template.Spec.Containers[i].Env = from.Spec.Template.Spec.Containers[i].Env
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].Resources, from.Spec.Template.Spec.Containers[i].Resources) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].Resources changed from", to.Spec.Template.Spec.Containers[i].Resources, "To:", from.Spec.Template.Spec.Containers[i].Resources)
				to.Spec.Template.Spec.Containers[i].Resources = from.Spec.Template.Spec.Containers[i].Resources
			}

			if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[i].VolumeMounts, from.Spec.Template.Spec.Containers[i].VolumeMounts) {
				requireUpdate = true
				log.Info("Update required because:", "Spec.Template.Spec.Containers["+strconv.Itoa(i)+")].VolumeMounts changed from", to.Spec.Template.Spec.Containers[i].VolumeMounts, "To:", from.Spec.Template.Spec.Containers[i].VolumeMounts)
				to.Spec.Template.Spec.Containers[i].VolumeMounts = from.Spec.Template.Spec.Containers[i].VolumeMounts
			}
		}
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.ImagePullSecrets, from.Spec.Template.Spec.ImagePullSecrets) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.ImagePullSecrets changed from", to.Spec.Template.Spec.ImagePullSecrets, "To:", from.Spec.Template.Spec.ImagePullSecrets)
		to.Spec.Template.Spec.ImagePullSecrets = from.Spec.Template.Spec.ImagePullSecrets
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.Affinity, from.Spec.Template.Spec.Affinity) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Affinity changed from", to.Spec.Template.Spec.Affinity, "To:", from.Spec.Template.Spec.Affinity)
		to.Spec.Template.Spec.Affinity = from.Spec.Template.Spec.Affinity
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.SecurityContext, from.Spec.Template.Spec.SecurityContext) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.SecurityContext changed from", to.Spec.Template.Spec.SecurityContext, "To:", from.Spec.Template.Spec.SecurityContext)
		to.Spec.Template.Spec.SecurityContext = from.Spec.Template.Spec.SecurityContext
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.Tolerations, from.Spec.Template.Spec.Tolerations) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Tolerations canged from", to.Spec.Template.Spec.Tolerations, "To:", from.Spec.Template.Spec.Tolerations)
		to.Spec.Template.Spec.Tolerations = from.Spec.Template.Spec.Tolerations
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.NodeSelector, from.Spec.Template.Spec.NodeSelector) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.NodeSelector canged from", to.Spec.Template.Spec.NodeSelector, "To:", from.Spec.Template.Spec.NodeSelector)
		to.Spec.Template.Spec.NodeSelector = from.Spec.Template.Spec.NodeSelector
	}

	return requireUpdate
}
