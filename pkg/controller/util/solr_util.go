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
	solr "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"strconv"
)

const (
	SolrClientPort        = 8983
	SolrClientPortName    = "solr-client"
	ExtSolrClientPort     = 80
	ExtSolrClientPortName = "ext-solr-client"
)

// GenerateStatefulSet returns a new appsv1.StatefulSet pointer generated for the SolrCloud instance
// object: SolrCloud instance
// replicas: the number of replicas for the SolrCloud instance
// storage: the size of the storage for the SolrCloud instance (e.g. 100Gi)
// zkConnectionString: the connectionString of the ZK instance to connect to
func GenerateStatefulSet(solrCloud *solr.SolrCloud, ingressBaseDomain string) *appsv1.StatefulSet {
	gracePeriodTerm := int64(10)
	fsGroup := int64(SolrClientPort)

	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	selectorLabels := solrCloud.SharedLabels()

	labels["technology"] = solr.SolrTechnologyLabel
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	solrVolumes := []corev1.Volume{
		{
			Name: "solr-xml",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: solrCloud.ConfigMapName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "solr.xml",
							Path: "solr.xml",
						},
					},
				},
			},
		},
	}

	solrDataVolumeName := "solrcloud-storage"
	pvcs := []corev1.PersistentVolumeClaim(nil)
	if solrCloud.Spec.PersistentVolumeClaimSpec != nil {
		solrDataVolumeName = "solrcloud-persistent-storage"

		pvcs = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{Name: solrDataVolumeName},
				Spec:       *solrCloud.Spec.PersistentVolumeClaimSpec,
			},
		}
	} else {
		solrVolumes = append(solrVolumes, corev1.Volume{
			Name: solrDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// if an ingressBaseDomain is provided, the node should be addressable outside of the cluster
	solrHostName := "$(POD_HOSTNAME)." + solrCloud.HeadlessServiceName()
	if ingressBaseDomain != "" {
		solrHostName = solrCloud.NodeIngressUrl("$(POD_HOSTNAME)", ingressBaseDomain)
	}

	stateful := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.StatefulSetName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			ServiceName: solrCloud.HeadlessServiceName(),
			Replicas:    solrCloud.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},

				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &gracePeriodTerm,
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: &fsGroup,
					},
					Volumes: solrVolumes,
					InitContainers: []corev1.Container{
						{
							Name:            "cp-solr-xml",
							Image:           solrCloud.Spec.BusyBoxImage.ToImageName(),
							ImagePullPolicy: solrCloud.Spec.BusyBoxImage.PullPolicy,
							Command:         []string{"sh", "-c", "cp /tmp/solr.xml /tmp-config/solr.xml"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "solr-xml",
									MountPath: "/tmp",
								},
								{
									Name:      solrDataVolumeName,
									MountPath: "/tmp-config",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "solrcloud-node",
							Image:           solrCloud.Spec.SolrImage.ToImageName(),
							ImagePullPolicy: solrCloud.Spec.SolrImage.PullPolicy,
							Ports: []corev1.ContainerPort{
								{ContainerPort: SolrClientPort, Name: SolrClientPortName},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: solrDataVolumeName, MountPath: "/opt/solr/server/home"}},
							Env: []corev1.EnvVar{
								{
									Name:  "SOLR_JAVA_MEM",
									Value: "-Xms1g -Xmx2g",
								},
								{
									Name:  "SOLR_HOME",
									Value: "/opt/solr/server/home",
								},
								{
									Name:  "SOLR_PORT",
									Value: strconv.Itoa(SolrClientPort),
								},
								{
									Name: "POD_HOSTNAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name:  "SOLR_HOST",
									Value: solrHostName,
								},
								{
									Name:  "ZK_HOST",
									Value: solrCloud.ZkConnectionString(),
								},
								{
									Name:  "SOLR_LOG_LEVEL",
									Value: "INFO",
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 20,
								PeriodSeconds:       10,
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Scheme: corev1.URISchemeHTTP,
										Path:   "/solr/admin/info/system",
										Port:   intstr.FromInt(SolrClientPort),
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 15,
								PeriodSeconds:       5,
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Scheme: corev1.URISchemeHTTP,
										Path:   "/solr/admin/info/system",
										Port:   intstr.FromInt(SolrClientPort),
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: pvcs,
		},
	}
	return stateful
}

// CopyStatefulSetFields copies the owned fields from one StatefulSet to another
// Returns true if the fields copied from don't match to.
func CopyStatefulSetFields(from, to *appsv1.StatefulSet) bool {

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

	if !reflect.DeepEqual(to.Spec.VolumeClaimTemplates, from.Spec.VolumeClaimTemplates) {
		requireUpdate = true
		to.Spec.VolumeClaimTemplates = from.Spec.VolumeClaimTemplates
	}

	if !reflect.DeepEqual(to.Spec.Template.Labels, from.Spec.Template.Labels) {
		requireUpdate = true
		to.Spec.Template.Labels = from.Spec.Template.Labels
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec, from.Spec.Template.Spec) {
		requireUpdate = true
		to.Spec.Template.Spec = from.Spec.Template.Spec
	}

	return requireUpdate
}

// GenerateConfigMap returns a new corev1.ConfigMap pointer generated for the SolrCloud instance solr.xml
// solrCloud: SolrCloud instance
func GenerateConfigMap(solrCloud *solr.SolrCloud) *corev1.ConfigMap {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.ConfigMapName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Data: map[string]string{
			"solr.xml": `<?xml version="1.0" encoding="UTF-8" ?>
<solr>
  <solrcloud>
    <str name="host">${host:}</str>
    <int name="hostPort">80</int>
    <str name="hostContext">${hostContext:solr}</str>
    <bool name="genericCoreNodeNames">${genericCoreNodeNames:true}</bool>
    <int name="zkClientTimeout">${zkClientTimeout:30000}</int>
    <int name="distribUpdateSoTimeout">${distribUpdateSoTimeout:600000}</int>
    <int name="distribUpdateConnTimeout">${distribUpdateConnTimeout:60000}</int>
    <str name="zkCredentialsProvider">${zkCredentialsProvider:org.apache.solr.common.cloud.DefaultZkCredentialsProvider}</str>
    <str name="zkACLProvider">${zkACLProvider:org.apache.solr.common.cloud.DefaultZkACLProvider}</str>
  </solrcloud>
  <shardHandlerFactory name="shardHandlerFactory"
    class="HttpShardHandlerFactory">
    <int name="socketTimeout">${socketTimeout:600000}</int>
    <int name="connTimeout">${connTimeout:60000}</int>
  </shardHandlerFactory>
</solr>
`,
		},
	}
	return configMap
}

// CopyConfigMapFields copies the owned fields from one ConfigMap to another
func CopyConfigMapFields(from, to *corev1.ConfigMap) bool {
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

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !reflect.DeepEqual(to.Data, from.Data) {
		requireUpdate = true
	}
	to.Data = from.Data

	return requireUpdate
}

// GenerateService returns a new corev1.Service pointer generated for the SolrCloud instance
// solrCloud: SolrCloud instance
func GenerateService(solrCloud *solr.SolrCloud) *corev1.Service {
	// TODO: Default and Validate these with Webhooks
	copyLabels := solrCloud.GetLabels()
	if copyLabels == nil {
		copyLabels = map[string]string{}
	}
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["service-type"] = "common"

	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.CommonServiceName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: ExtSolrClientPortName, Port: ExtSolrClientPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(SolrClientPort)},
			},
			Selector: selectorLabels,
		},
	}
	return service
}

// GenerateHeadlessService returns a new Headless corev1.Service pointer generated for the SolrCloud instance
// solrCloud: SolrCloud instance
func GenerateHeadlessService(solrCloud *solr.SolrCloud) *corev1.Service {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["service-type"] = "headless"

	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.HeadlessServiceName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: ExtSolrClientPortName, Port: ExtSolrClientPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(SolrClientPort)},
			},
			Selector:  selectorLabels,
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	return service
}

// GenerateNodeService returns a new External corev1.Service pointer generated for the given Solr Node
// solrCloud: SolrCloud instance
// nodeName: string node
func GenerateNodeService(solrCloud *solr.SolrCloud, nodeName string) *corev1.Service {
	// TODO: Default and Validate these with Webhooks
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["service-type"] = "external"

	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel
	selectorLabels["statefulset.kubernetes.io/pod-name"] = nodeName

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{Name: ExtSolrClientPortName, Port: ExtSolrClientPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(SolrClientPort)},
			},
		},
	}
	return service
}

// CopyServiceFields copies the owned fields from one Service to another
func CopyServiceFields(from, to *corev1.Service) bool {
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

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !reflect.DeepEqual(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
	}
	to.Spec.Selector = from.Spec.Selector

	if !reflect.DeepEqual(to.Spec.Ports, from.Spec.Ports) {
		requireUpdate = true
	}
	to.Spec.Ports = from.Spec.Ports

	if !reflect.DeepEqual(to.Spec.ExternalName, from.Spec.ExternalName) {
		requireUpdate = true
	}
	to.Spec.ExternalName = from.Spec.ExternalName

	return requireUpdate
}

// GenerateCommonIngress returns a new Ingress pointer generated for the entire SolrCloud, pointing to all instances
// solrCloud: SolrCloud instance
// nodeStatuses: []SolrNodeStatus the nodeStatuses
// ingressBaseDomain: string baseDomain of the ingress
func GenerateCommonIngress(solrCloud *solr.SolrCloud, nodeStatuses []solr.SolrNodeStatus, ingressBaseDomain string) *extv1.Ingress {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())

	rules := []extv1.IngressRule{
		{
			Host: solrCloud.CommonIngressUrl(ingressBaseDomain),
			IngressRuleValue: extv1.IngressRuleValue{
				HTTP: &extv1.HTTPIngressRuleValue{
					Paths: []extv1.HTTPIngressPath{
						{
							Backend: extv1.IngressBackend{
								ServiceName: solrCloud.CommonServiceName(),
								ServicePort: intstr.FromInt(ExtSolrClientPort),
							},
						},
					},
				},
			},
		},
	}

	for idx, _ := range nodeStatuses {
		rules = append(rules, CreateNodeIngressRule(solrCloud, &nodeStatuses[idx], ingressBaseDomain))
	}

	ingress := &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.CommonIngressName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: extv1.IngressSpec{
			Rules: rules,
		},
	}
	return ingress
}

// CreateNodeIngressRule returns a new Ingress Rule generated for a specific Solr Node
// solrCloud: SolrCloud instance
// nodeName: string Name of the node
// ingressBaseDomain: string base domain for the ingress controller
func CreateNodeIngressRule(solrCloud *solr.SolrCloud, nodeStatus *solr.SolrNodeStatus, ingressBaseDomain string) extv1.IngressRule {
	externalAddress := solrCloud.NodeIngressUrl(nodeStatus.NodeName, ingressBaseDomain)
	nodeStatus.ExternalAddress = "http://" + externalAddress
	return extv1.IngressRule{
		Host: externalAddress,
		IngressRuleValue: extv1.IngressRuleValue{
			HTTP: &extv1.HTTPIngressRuleValue{
				Paths: []extv1.HTTPIngressPath{
					{
						Backend: extv1.IngressBackend{
							ServiceName: nodeStatus.NodeName,
							ServicePort: intstr.FromInt(ExtSolrClientPort),
						},
					},
				},
			},
		},
	}
}

// CopyIngressFields copies the owned fields from one Ingress to another
func CopyIngressFields(from, to *extv1.Ingress) bool {
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

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !reflect.DeepEqual(to.Spec.Rules, from.Spec.Rules) {
		requireUpdate = true
	}
	to.Spec.Rules = from.Spec.Rules

	return requireUpdate
}
