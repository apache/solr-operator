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
	"strconv"
	"strings"
	"time"

	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	etcd "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	zkCli "github.com/samuel/go-zookeeper/zk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	}
	to.Spec.Pod.Resources = from.Spec.Pod.Resources

	if from.Spec.Pod.Affinity != nil {
		if !DeepEqualWithNils(to.Spec.Pod.Affinity.NodeAffinity, from.Spec.Pod.Affinity.NodeAffinity) {
			log.Info("Updating Zk pod node affinity")
			log.Info("Update required because:", "Spec.Pod.Affinity.NodeAffinity changed from", to.Spec.Pod.Affinity.NodeAffinity, "To:", from.Spec.Pod.Affinity.NodeAffinity)
			requireUpdate = true
		}

		if !DeepEqualWithNils(to.Spec.Pod.Affinity.PodAffinity, from.Spec.Pod.Affinity.PodAffinity) {
			log.Info("Updating Zk pod node affinity")
			log.Info("Update required because:", "Spec.Pod.Affinity.PodAffinity changed from", to.Spec.Pod.Affinity.PodAffinity, "To:", from.Spec.Pod.Affinity.PodAffinity)
			requireUpdate = true
		}
		to.Spec.Pod.Affinity = from.Spec.Pod.Affinity
	}

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

	// Append Pod Policies if provided by user
	if etcdSpec.EtcdPod.Affinity != nil {
		etcdCluster.Spec.Pod.Affinity = etcdSpec.EtcdPod.Affinity
	}

	if etcdSpec.EtcdPod.Resources.Limits != nil || etcdSpec.EtcdPod.Resources.Requests != nil {
		etcdCluster.Spec.Pod.Resources = etcdSpec.EtcdPod.Resources
	}

	return etcdCluster
}

// CopyEtcdClusterFields copies the owned fields from one EtcdCluster to another
// Returns true if the fields copied from don't match to.
func CopyEtcdClusterFields(from, to *etcd.EtcdCluster) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	if !DeepEqualWithNils(to.Spec, from.Spec) {
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

	if spec.ZetcdPod.Resources.Limits != nil || spec.ZetcdPod.Resources.Requests != nil {
		deployment.Spec.Template.Spec.Containers[0].Resources = spec.ZetcdPod.Resources
	}

	if spec.ZetcdPod.Affinity != nil {
		deployment.Spec.Template.Spec.Affinity = spec.ZetcdPod.Affinity
	}

	return deployment
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

func CreateChRootIfNecessary(info solr.ZookeeperConnectionInfo) error {
	if info.InternalConnectionString != "" && info.ChRoot != "/" {
		zkClient, _, err := zkCli.Connect(strings.Split(info.InternalConnectionString, ","), time.Second)
		if err != nil {
			log.Error(err, "Could not connect to Zookeeper", "connectionString", info.InternalConnectionString)
			return err
		}

		pathParts := strings.Split(strings.TrimPrefix(info.ChRoot, "/"), "/")
		pathToCreate := ""
		// Loop through each parent of the ZNode, and make sure that they exist recursively
		for _, part := range pathParts {
			if part == "" {
				continue
			}
			pathToCreate += "/" + part

			// Make sure that this part of the chRoot exists
			exists, _, err := zkClient.Exists(pathToCreate)
			if err != nil {
				log.Error(err, "Could not check existence of Znode", "path", pathToCreate)
				return err
			} else if !exists {
				log.Info("Creating Znode for chRoot of SolrCloud", "path", pathToCreate)
				_, err = zkClient.Create(pathToCreate, []byte(""), 0, zkCli.WorldACL(zkCli.PermAll))

				if err != nil {
					log.Error(err, "Could not create Znode for chRoot of SolrCloud", "path", pathToCreate)
					return err
				}
			}
		}
		return err
	}
	return nil
}
