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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
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

// AddACLsToEnv creates the neccessary environment variables for using ZK ACLs, and returns whether ACLs were provided.
// info: Zookeeper Connection Information
func AddACLsToEnv(info *solr.ZookeeperConnectionInfo) (hasACLs bool, envVars []corev1.EnvVar) {
	if info == nil || (info.AllACL == nil && info.ReadOnlyACL == nil) {
		return false, envVars
	}
	f := false
	var zkDigests []string
	if info.AllACL != nil {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: "ZK_ALL_ACL_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: info.AllACL.SecretRef,
						},
						Key:      info.AllACL.UsernameKey,
						Optional: &f,
					},
				},
			},
			corev1.EnvVar{
				Name: "ZK_ALL_ACL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: info.AllACL.SecretRef,
						},
						Key:      info.AllACL.PasswordKey,
						Optional: &f,
					},
				},
			})
		zkDigests = append(zkDigests, "-DzkDigestUsername=$(ZK_ALL_ACL_USERNAME)", "-DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD)")
	}
	if info.ReadOnlyACL != nil {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: "ZK_READ_ACL_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: info.ReadOnlyACL.SecretRef,
						},
						Key:      info.ReadOnlyACL.UsernameKey,
						Optional: &f,
					},
				},
			},
			corev1.EnvVar{
				Name: "ZK_READ_ACL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: info.ReadOnlyACL.SecretRef,
						},
						Key:      info.ReadOnlyACL.PasswordKey,
						Optional: &f,
					},
				},
			})
		zkDigests = append(zkDigests, "-DzkDigestReadonlyUsername=$(ZK_READ_ACL_USERNAME)", "-DzkDigestReadonlyPassword=$(ZK_READ_ACL_PASSWORD)")
	}
	envVars = append(envVars,
		corev1.EnvVar{
			Name:  "SOLR_ZK_CREDS_AND_ACLS",
			Value: "-DzkACLProvider=org.apache.solr.common.cloud.VMParamsAllAndReadonlyDigestZkACLProvider -DzkCredentialsProvider=org.apache.solr.common.cloud.VMParamsSingleSetCredentialsDigestZkCredentialsProvider " + strings.Join(zkDigests, " "),
		})

	return true, envVars
}
