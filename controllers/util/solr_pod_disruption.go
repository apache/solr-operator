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
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GeneratePodDisruptionBudget(cloud *solr.SolrCloud, selector map[string]string) *policyv1.PodDisruptionBudget {
	// For this PDB, we can use an intOrString maxUnavailable (whatever the user provides),
	// because we are matching the labelSelector used by the statefulSet.
	var maxUnavailable intstr.IntOrString
	if cloud.Spec.UpdateStrategy.ManagedUpdateOptions.MaxPodsUnavailable != nil {
		maxUnavailable = *cloud.Spec.UpdateStrategy.ManagedUpdateOptions.MaxPodsUnavailable
	} else {
		maxUnavailable = intstr.FromString(DefaultMaxPodsUnavailable)
	}
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cloud.StatefulSetName(),
			Namespace: cloud.Namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			MaxUnavailable: &maxUnavailable,
		},
	}
}

/*
*
We cannot actually use the shard topology for PDBs, because Kubernetes does not currently support a pod
mapping to multiple PDBs. Since a Solr pod is sure to host replicas of multiple shards, then we would
have to create multiple PDBs that cover a single pod. Therefore we can only use the Cloud PDB defined in the method
above.

Whenever we can use this, when generating PDBs, we need to label them so that a list of all PDBs for a cloud can be found easily.
That way, when we have the list of PDBs to create/update, we will aslo know the list of PDBs that need to be deleted.

Kubernetes Documentation: https://kubernetes.io/docs/tasks/run-application/configure-pdb/#arbitrary-controllers-and-selectors
*/
func createPodDisruptionBudgetForShard(cloud *solr.SolrCloud, collection string, shard string, nodes []string) policyv1.PodDisruptionBudget {
	maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(
		intstr.ValueOrDefault(cloud.Spec.UpdateStrategy.ManagedUpdateOptions.MaxShardReplicasUnavailable, intstr.FromInt(DefaultMaxShardReplicasUnavailable)),
		len(nodes),
		false)
	if err != nil {
		maxUnavailable = 1
	}
	// From the documentation above, Kubernetes will only accept an int minAvailable for PDBs that use custom pod selectors.
	// Therefore, we cannot use the maxUnavailable straight from what the user provides.
	minAvailable := intstr.FromInt(len(nodes) - maxUnavailable)
	return policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cloud.Name + "-" + collection + "-" + shard,
			Namespace: cloud.Namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "statefulset.kubernetes.io/pod-name",
						Operator: "In",
						Values:   nodes,
					},
				},
			},
			MinAvailable: &minAvailable,
		},
	}
}
