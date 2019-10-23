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
	etcd "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller")

// GenerateZookeeperCluster returns a new ZookeeperCluster pointer generated for the SolrCloud instance
// object: SolrCloud instance
// zkSpec: the spec of the ZookeeperCluster to generate
func GenerateZookeeperCluster(solrCloud *solr.SolrCloud, zkSpec solr.ZookeeperSpec) *zk.ZookeeperCluster {
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
			Labels:                    labels,
			Replicas:                  *zkSpec.Replicas,
			PersistentVolumeClaimSpec: *zkSpec.PersistentVolumeClaimSpec,
		},
	}
	return zkCluster
}

// CopyZookeeperClusterFields copies the owned fields from one ZookeeperCluster to another
// Returns true if the fields copied from don't match to.
func CopyZookeeperClusterFields(from, to *zk.ZookeeperCluster) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			log.Info("Updating Zookeeper label ", k, v)
			requireUpdate = true
		}
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			log.Info("Updating Zk annotation", k, v)
			requireUpdate = true
		}
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec.Replicas, from.Spec.Replicas) {
		log.Info("Updating Zk replicas")
		requireUpdate = true
	}
	to.Spec.Replicas = from.Spec.Replicas

	if !reflect.DeepEqual(to.Spec.Image.Repository, from.Spec.Image.Repository) {
		log.Info("Updating Zk image repository")
		requireUpdate = true
	}
	to.Spec.Image.Repository = from.Spec.Image.Repository

	if !reflect.DeepEqual(to.Spec.Image.Tag, from.Spec.Image.Tag) {
		log.Info("Updating Zk image tag")
		requireUpdate = true
	}
	to.Spec.Image.Tag = from.Spec.Image.Tag

	if !reflect.DeepEqual(to.Spec.PersistentVolumeClaimSpec.Resources.Requests, from.Spec.PersistentVolumeClaimSpec.Resources.Requests) {
		requireUpdate = true
	}
	to.Spec.PersistentVolumeClaimSpec.Resources.Requests = from.Spec.PersistentVolumeClaimSpec.Resources.Requests

	return requireUpdate
}

// GenerateEtcdCluster returns a new EtcdCluster pointer generated for the SolrCloud instance
// object: SolrCloud instance
// etcdSpec: the spec of the EtcdCluster to generate
// busyBoxImage: the image of busyBox to use
func GenerateEtcdCluster(solrCloud *solr.SolrCloud, etcdSpec solr.EtcdSpec, busyBoxImage solr.ContainerImage) *etcd.EtcdCluster {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["technology"] = solr.ZookeeperTechnologyLabel

	etcdCluster := &etcd.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.ProvidedZetcdName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: etcd.ClusterSpec{
			Version:    etcdSpec.Image.Tag,
			Repository: etcdSpec.Image.Repository,
			Size:       *etcdSpec.Replicas,
			Pod: &etcd.PodPolicy{
				Labels:       labels,
				BusyboxImage: busyBoxImage.ToImageName(),
			},
		},
	}
	return etcdCluster
}

// CopyEtcdClusterFields copies the owned fields from one EtcdCluster to another
// Returns true if the fields copied from don't match to.
func CopyEtcdClusterFields(from, to *etcd.EtcdCluster) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec, from.Spec) {
		requireUpdate = true
	}
	to.Spec = from.Spec

	return requireUpdate
}

// GenerateZetcdDeployment returns a new appsv1.Deployment for Zetcd
// solrCloud: SolrCloud instance
// spec: ZetcdSpec
func GenerateZetcdDeployment(solrCloud *solr.SolrCloud, spec solr.ZetcdSpec) *appsv1.Deployment {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	selectorLabels := solrCloud.SharedLabels()

	labels["technology"] = solr.ZookeeperTechnologyLabel
	selectorLabels["technology"] = solr.ZookeeperTechnologyLabel

	labels["app"] = "zetcd"
	selectorLabels["app"] = "zetcd"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.ProvidedZetcdName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "zetcd",
							Image:           spec.Image.ToImageName(),
							ImagePullPolicy: spec.Image.PullPolicy,
							Ports:           []corev1.ContainerPort{{ContainerPort: 2181}},
						},
					},
				},
			},
		},
	}
	return deployment
}

// CopyDeploymentFields copies the owned fields from one Deployment to another
// Returns true if the fields copied from don't match to.
func CopyDeploymentFields(from, to *appsv1.Deployment) bool {
	requireUpdate := false
	for k, v := range from.Labels {
		if to.Labels[k] != v {
			requireUpdate = true
		}
		to.Labels[k] = v
	}

	for k, v := range from.Annotations {
		if to.Annotations[k] != v {
			requireUpdate = true
		}
		to.Annotations[k] = v
	}

	if !reflect.DeepEqual(to.Spec.Replicas, from.Spec.Replicas) {
		requireUpdate = true
		to.Spec.Replicas = from.Spec.Replicas
	}

	if !reflect.DeepEqual(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
		to.Spec.Selector = from.Spec.Selector
	}

	if !reflect.DeepEqual(to.Spec.Template.Labels, from.Spec.Template.Labels) {
		requireUpdate = true
		to.Spec.Template.Labels = from.Spec.Template.Labels
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Volumes, from.Spec.Template.Spec.Volumes) {
		requireUpdate = true
		to.Spec.Template.Spec.Volumes = from.Spec.Template.Spec.Volumes
	}

	if len(to.Spec.Template.Spec.Containers) != len(from.Spec.Template.Spec.Containers) {
		requireUpdate = true
		to.Spec.Template.Spec.Containers = from.Spec.Template.Spec.Containers
	} else if !reflect.DeepEqual(to.Spec.Template.Spec.Containers, from.Spec.Template.Spec.Containers) {
		for i := 0; i < len(to.Spec.Template.Spec.Containers); i++ {
			if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[i].Name, from.Spec.Template.Spec.Containers[i].Name) {
				requireUpdate = true
				to.Spec.Template.Spec.Containers[i].Name = from.Spec.Template.Spec.Containers[i].Name
			}

			if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[i].Image, from.Spec.Template.Spec.Containers[i].Image) {
				requireUpdate = true
				to.Spec.Template.Spec.Containers[i].Image = from.Spec.Template.Spec.Containers[i].Image
			}

			if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[i].ImagePullPolicy, from.Spec.Template.Spec.Containers[i].ImagePullPolicy) {
				requireUpdate = true
				to.Spec.Template.Spec.Containers[i].ImagePullPolicy = from.Spec.Template.Spec.Containers[i].ImagePullPolicy
			}

			if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[i].Command, from.Spec.Template.Spec.Containers[i].Command) {
				requireUpdate = true
				to.Spec.Template.Spec.Containers[i].Command = from.Spec.Template.Spec.Containers[i].Command
			}

			if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[i].Args, from.Spec.Template.Spec.Containers[i].Args) {
				requireUpdate = true
				to.Spec.Template.Spec.Containers[i].Args = from.Spec.Template.Spec.Containers[i].Args
			}

			if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[i].Env, from.Spec.Template.Spec.Containers[i].Env) {
				requireUpdate = true
				to.Spec.Template.Spec.Containers[i].Env = from.Spec.Template.Spec.Containers[i].Env
			}
		}
	}

	return requireUpdate
}

// GenerateZetcdService returns a new corev1.Service pointer generated for the Zetcd deployment
// solrCloud: SolrCloud instance
// spec: ZetcdSpec
func GenerateZetcdService(solrCloud *solr.SolrCloud, spec solr.ZetcdSpec) *corev1.Service {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	selectorLabels := solrCloud.SharedLabels()

	labels["technology"] = solr.ZookeeperTechnologyLabel
	selectorLabels["technology"] = solr.ZookeeperTechnologyLabel

	labels["app"] = "zetcd"
	selectorLabels["app"] = "zetcd"

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.ProvidedZetcdName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 2181, TargetPort: intstr.IntOrString{IntVal: 2181, Type: intstr.Int}},
			},
			Selector: selectorLabels,
		},
	}
	return service
}
