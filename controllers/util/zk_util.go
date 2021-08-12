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
	"github.com/go-logr/logr"
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
			Labels:   labels,
			Replicas: *zkSpec.Replicas,
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

	// Add storage information for the ZK Cluster
	if zkSpec.Persistence != nil {
		// If persistence is provided, then chose it.
		zkCluster.Spec.StorageType = "persistence"
	} else if zkSpec.Ephemeral != nil {
		// If ephemeral is provided, then chose it.
		zkCluster.Spec.StorageType = "ephemeral"
	} else {
		// If neither option is provided, default to the option used for solr (which defaults to ephemeral)
		if solrCloud.Spec.StorageOptions.PersistentStorage != nil {
			zkCluster.Spec.StorageType = "persistence"
		} else {
			zkCluster.Spec.StorageType = "ephemeral"
		}
	}

	// Set the persistence/ephemeral options if necessary
	if zkSpec.Persistence != nil && zkCluster.Spec.StorageType == "persistence" {
		zkCluster.Spec.Persistence = zkSpec.Persistence
	} else if zkSpec.Ephemeral != nil && zkCluster.Spec.StorageType == "ephemeral" {
		zkCluster.Spec.Ephemeral = zkSpec.Ephemeral
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

	if zkSpec.ZookeeperPod.Env != nil {
		zkCluster.Spec.Pod.Env = zkSpec.ZookeeperPod.Env
	}

	if solrCloud.Spec.SolrAddressability.KubeDomain != "" {
		zkCluster.Spec.KubernetesClusterDomain = solrCloud.Spec.SolrAddressability.KubeDomain
	}

	if zkSpec.ZookeeperPod.ServiceAccountName != "" {
		zkCluster.Spec.Pod.ServiceAccountName = zkSpec.ZookeeperPod.ServiceAccountName
	}

	// Add defaults that the ZK Operator should set itself, otherwise we will have problems with reconcile loops.
	// Also it will default the spec.Probes object which cannot be set to null.
	// TODO: Might be able to remove when the following is resolved and the dependency is upgraded:
	// https://github.com/pravega/zookeeper-operator/issues/378
	zkCluster.WithDefaults()
	return zkCluster
}

// CopyZookeeperClusterFields copies the owned fields from one ZookeeperCluster to another
// Returns true if the fields copied from don't match to.
func CopyZookeeperClusterFields(from, to *zk.ZookeeperCluster, logger logr.Logger) bool {
	logger = logger.WithValues("kind", "zookeeperCluster")
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta, logger)

	if !DeepEqualWithNils(to.Spec.Replicas, from.Spec.Replicas) {
		logger.Info("Update required because field changed", "field", "Spec.Replicas", "from", to.Spec.Replicas, "to", from.Spec.Replicas)
		requireUpdate = true
	}
	to.Spec.Replicas = from.Spec.Replicas

	if !DeepEqualWithNils(to.Spec.Image.Repository, from.Spec.Image.Repository) {
		logger.Info("Update required because field changed", "field", "Spec.Image.Repository", "from", to.Spec.Image.Repository, "to", from.Spec.Image.Repository)
		requireUpdate = true
	}
	to.Spec.Image.Repository = from.Spec.Image.Repository

	if from.Spec.Image.Tag != "" && !DeepEqualWithNils(to.Spec.Image.Tag, from.Spec.Image.Tag) {
		logger.Info("Update required because field changed", "field", "Spec.Image.Tag", "from", to.Spec.Image.Tag, "to", from.Spec.Image.Tag)
		requireUpdate = true
	}
	to.Spec.Image.Tag = from.Spec.Image.Tag

	if !DeepEqualWithNils(to.Spec.Image.PullPolicy, from.Spec.Image.PullPolicy) {
		logger.Info("Update required because field changed", "field", "Spec.Image.PullPolicy", "from", to.Spec.Image.PullPolicy, "to", from.Spec.Image.PullPolicy)
		requireUpdate = true
	}
	to.Spec.Image.PullPolicy = from.Spec.Image.PullPolicy

	if !DeepEqualWithNils(to.Spec.StorageType, from.Spec.StorageType) {
		logger.Info("Update required because field changed", "field", "Spec.StorageType", "from", to.Spec.StorageType, "to", from.Spec.StorageType)
		requireUpdate = true
		to.Spec.StorageType = from.Spec.StorageType
	}
	if to.Spec.StorageType == "persistence" {
		if to.Spec.Ephemeral != nil {
			logger.Info("Update required because field changed", "field", "Spec.Ephemeral", "from", to.Spec.Ephemeral, "to", nil)
			requireUpdate = true
			to.Spec.Ephemeral = nil
		}
		if from.Spec.Persistence != nil {
			if to.Spec.Persistence == nil {
				logger.Info("Update required because field changed", "field", "Spec.Persistence", "from", nil, "to", from.Spec.Persistence)
				requireUpdate = true
				to.Spec.Persistence = from.Spec.Persistence
			} else {
				if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests, from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests) {
					logger.Info("Update required because field changed", "field", "Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests", "from", to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests, "to", from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests)
					requireUpdate = true
					to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests = from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Requests
				}

				if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits, from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits) {
					logger.Info("Update required because field changed", "field", "Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits", "from", to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits, "to", from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits)
					requireUpdate = true
					to.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits = from.Spec.Persistence.PersistentVolumeClaimSpec.Resources.Limits
				}

				if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes, from.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes) {
					logger.Info("Update required because field changed", "field", "Spec.Persistence.PersistentVolumeClaimSpec.AccessModes", "from", to.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes, "to", from.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes)
					requireUpdate = true
					to.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes = from.Spec.Persistence.PersistentVolumeClaimSpec.AccessModes
				}

				if !DeepEqualWithNils(to.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName, from.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName) {
					logger.Info("Update required because field changed", "field", "Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName", "from", to.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName, "to", from.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName)
					requireUpdate = true
					to.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName = from.Spec.Persistence.PersistentVolumeClaimSpec.StorageClassName
				}

				if !DeepEqualWithNils(to.Spec.Persistence.VolumeReclaimPolicy, from.Spec.Persistence.VolumeReclaimPolicy) {
					logger.Info("Update required because field changed", "field", "Spec.Persistence.VolumeReclaimPolicy", "from", to.Spec.Persistence.VolumeReclaimPolicy, "to", from.Spec.Persistence.VolumeReclaimPolicy)
					requireUpdate = true
					to.Spec.Persistence.VolumeReclaimPolicy = from.Spec.Persistence.VolumeReclaimPolicy
				}

				if !DeepEqualWithNils(to.Spec.Persistence.Annotations, from.Spec.Persistence.Annotations) {
					logger.Info("Update required because field changed", "field", "Spec.Persistence.Annotations", "from", to.Spec.Persistence.Annotations, "to", from.Spec.Persistence.Annotations)
					requireUpdate = true
					to.Spec.Persistence.Annotations = from.Spec.Persistence.Annotations
				}
			}
		} else if to.Spec.Persistence != nil {
			logger.Info("Update required because field changed", "field", "Spec.Persistence", "from", to.Spec.Persistence, "to", nil)
			requireUpdate = true
			to.Spec.Persistence = nil
		}
	} else if to.Spec.StorageType == "ephemeral" {
		if to.Spec.Persistence != nil {
			logger.Info("Update required because field changed", "field", "Spec.Persistence", "from", to.Spec.Persistence, "to", nil)
			requireUpdate = true
			to.Spec.Persistence = nil
		}
		if from.Spec.Ephemeral != nil {
			if to.Spec.Ephemeral == nil {
				logger.Info("Update required because field changed", "field", "Spec.Ephemeral", "from", nil, "to", from.Spec.Ephemeral)
				requireUpdate = true
				to.Spec.Ephemeral = from.Spec.Ephemeral
			} else {
				if !DeepEqualWithNils(to.Spec.Ephemeral.EmptyDirVolumeSource.Medium, from.Spec.Ephemeral.EmptyDirVolumeSource.Medium) {
					logger.Info("Update required because field changed", "field", "Spec.Ephemeral.EmptyDirVolumeSource.Medium", "from", to.Spec.Ephemeral.EmptyDirVolumeSource.Medium, "to", from.Spec.Ephemeral.EmptyDirVolumeSource.Medium)
					requireUpdate = true
					to.Spec.Ephemeral.EmptyDirVolumeSource.Medium = from.Spec.Ephemeral.EmptyDirVolumeSource.Medium
				}

				if !DeepEqualWithNils(to.Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit, from.Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit) {
					logger.Info("Update required because field changed", "field", "Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit", "from", to.Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit, "to", from.Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit)
					requireUpdate = true
					to.Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit = from.Spec.Ephemeral.EmptyDirVolumeSource.SizeLimit
				}
			}
		} else if to.Spec.Ephemeral != nil {
			logger.Info("Update required because field changed", "field", "Spec.Ephemeral", "from", to.Spec.Ephemeral, "to", nil)
			requireUpdate = true
			to.Spec.Ephemeral = nil
		}
	}

	if !DeepEqualWithNils(to.Spec.Pod.Resources, from.Spec.Pod.Resources) {
		logger.Info("Update required because field changed", "field", "Spec.Pod.Resources", "from", to.Spec.Pod.Resources, "to", from.Spec.Pod.Resources)
		requireUpdate = true
		to.Spec.Pod.Resources = from.Spec.Pod.Resources
	}

	if !DeepEqualWithNils(to.Spec.Pod.Env, from.Spec.Pod.Env) {
		logger.Info("Update required because field changed", "field", "Spec.Pod.Env", "from", to.Spec.Pod.Env, "to", from.Spec.Pod.Env)
		requireUpdate = true
		to.Spec.Pod.Env = from.Spec.Pod.Env
	}

	if !DeepEqualWithNils(to.Spec.Pod.Tolerations, from.Spec.Pod.Tolerations) {
		logger.Info("Update required because field changed", "field", "Spec.Pod.Tolerations", "from", to.Spec.Pod.Tolerations, "to", from.Spec.Pod.Tolerations)
		requireUpdate = true
		to.Spec.Pod.Tolerations = from.Spec.Pod.Tolerations
	}

	if !DeepEqualWithNils(to.Spec.Pod.NodeSelector, from.Spec.Pod.NodeSelector) {
		logger.Info("Update required because field changed", "field", "Spec.Pod.NodeSelector", "from", to.Spec.Pod.NodeSelector, "to", from.Spec.Pod.NodeSelector)
		requireUpdate = true
		to.Spec.Pod.NodeSelector = from.Spec.Pod.NodeSelector
	}

	// The Zookeeper operator defaults the pod affinity, so we only want to require an update if the requested affinity is not null
	// But always change it so that the change will be picked up if another change is done.
	if !DeepEqualWithNils(to.Spec.Pod.Affinity, from.Spec.Pod.Affinity) && from.Spec.Pod.Affinity != nil {
		logger.Info("Update required because field changed", "field", "Spec.Pod.Affinity", "from", to.Spec.Pod.Affinity, "to", from.Spec.Pod.Affinity)
		requireUpdate = true
	}
	to.Spec.Pod.Affinity = from.Spec.Pod.Affinity

	// The Zookeeper Operator defaults the serviceAccountName to "default", therefore only update if either of the following
	//   - The new serviceAccountName is not empty
	//   - The old serviceAccountName is not "default", so we know we want to switch to the default value.
	if !DeepEqualWithNils(to.Spec.Pod.ServiceAccountName, from.Spec.Pod.ServiceAccountName) && (from.Spec.Pod.ServiceAccountName != "" || to.Spec.Pod.ServiceAccountName != "default") {
		logger.Info("Update required because field changed", "field", "Spec.Pod.ServiceAccountName", "from", to.Spec.Pod.ServiceAccountName, "to", from.Spec.Pod.ServiceAccountName)
		requireUpdate = true
		to.Spec.Pod.ServiceAccountName = from.Spec.Pod.ServiceAccountName
	}

	if !DeepEqualWithNils(to.Spec.KubernetesClusterDomain, from.Spec.KubernetesClusterDomain) && from.Spec.KubernetesClusterDomain != "" {
		logger.Info("Update required because field changed", "field", "Spec.KubernetesClusterDomain", "from", to.Spec.KubernetesClusterDomain, "to", from.Spec.KubernetesClusterDomain)
		requireUpdate = true
	}
	to.Spec.KubernetesClusterDomain = from.Spec.KubernetesClusterDomain

	if !DeepEqualWithNils(to.Spec.Probes, from.Spec.Probes) {
		logger.Info("Update required because field changed", "field", "Spec.Probes", "from", to.Spec.Probes, "to", from.Spec.Probes)
		requireUpdate = true
		to.Spec.Probes = from.Spec.Probes
	}

	return requireUpdate
}

// AddACLsToEnv creates the neccessary environment variables for using ZK ACLs, and returns whether ACLs were provided.
// info: Zookeeper Connection Information
func AddACLsToEnv(allACL *solr.ZookeeperACL, readOnlyACL *solr.ZookeeperACL) (hasACLs bool, envVars []corev1.EnvVar) {
	if allACL == nil && readOnlyACL == nil {
		return false, envVars
	}

	f := false
	var zkDigests []string
	if allACL != nil {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: "ZK_ALL_ACL_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: allACL.SecretRef,
						},
						Key:      allACL.UsernameKey,
						Optional: &f,
					},
				},
			},
			corev1.EnvVar{
				Name: "ZK_ALL_ACL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: allACL.SecretRef,
						},
						Key:      allACL.PasswordKey,
						Optional: &f,
					},
				},
			})
		zkDigests = append(zkDigests, "-DzkDigestUsername=$(ZK_ALL_ACL_USERNAME)", "-DzkDigestPassword=$(ZK_ALL_ACL_PASSWORD)")
	}
	if readOnlyACL != nil {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: "ZK_READ_ACL_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: readOnlyACL.SecretRef,
						},
						Key:      readOnlyACL.UsernameKey,
						Optional: &f,
					},
				},
			},
			corev1.EnvVar{
				Name: "ZK_READ_ACL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: readOnlyACL.SecretRef,
						},
						Key:      readOnlyACL.PasswordKey,
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
