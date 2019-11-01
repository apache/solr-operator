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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	solr "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	SolrClientPort        = 8983
	SolrClientPortName    = "solr-client"
	ExtSolrClientPort     = 80
	ExtSolrClientPortName = "ext-solr-client"
	BackupRestoreVolume   = "backup-restore"
)

// GenerateStatefulSet returns a new appsv1.StatefulSet pointer generated for the SolrCloud instance
// object: SolrCloud instance
// replicas: the number of replicas for the SolrCloud instance
// storage: the size of the storage for the SolrCloud instance (e.g. 100Gi)
// zkConnectionString: the connectionString of the ZK instance to connect to
func GenerateStatefulSet(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus, ingressBaseDomain string, hostNameIPs map[string]string) *appsv1.StatefulSet {
	gracePeriodTerm := int64(10)
	fsGroup := int64(SolrClientPort)
	defaultMode := int32(420)

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
					DefaultMode: &defaultMode,
				},
			},
		},
	}

	solrDataVolumeName := "data"
	volumeMounts := []corev1.VolumeMount{{Name: solrDataVolumeName, MountPath: "/var/solr/data"}}
	var pvcs []corev1.PersistentVolumeClaim
	if solrCloud.Spec.DataPvcSpec != nil {
		pvcs = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{Name: solrDataVolumeName},
				Spec:       *solrCloud.Spec.DataPvcSpec,
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
	// Add backup volumes
	if solrCloud.Spec.BackupRestoreVolume != nil {
		solrVolumes = append(solrVolumes, corev1.Volume{
			Name:         BackupRestoreVolume,
			VolumeSource: *solrCloud.Spec.BackupRestoreVolume,
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: BackupRestoreVolume, MountPath: BaseBackupRestorePath, SubPath: BackupRestoreSubPathForCloud(solrCloud.Name)})
	}

	hostAliases := make([]corev1.HostAlias, len(hostNameIPs))
	index := 0
	for hostName, ip := range hostNameIPs {
		hostAliases[index] = corev1.HostAlias{
			IP:        ip,
			Hostnames: []string{hostName},
		}
		index++
	}
	if len(hostAliases) == 0 {
		hostAliases = nil
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
							Name:                     "cp-solr-xml",
							Image:                    solrCloud.Spec.BusyBoxImage.ToImageName(),
							ImagePullPolicy:          solrCloud.Spec.BusyBoxImage.PullPolicy,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							Command:                  []string{"sh", "-c", "cp /tmp/solr.xml /tmp-config/solr.xml"},
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
					HostAliases: hostAliases,
					Containers: []corev1.Container{
						{
							Name:            "solrcloud-node",
							Image:           solrCloud.Spec.SolrImage.ToImageName(),
							ImagePullPolicy: solrCloud.Spec.SolrImage.PullPolicy,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: SolrClientPort,
									Name:          SolrClientPortName,
									Protocol:      "TCP",
								},
							},
							VolumeMounts: volumeMounts,
							Env: []corev1.EnvVar{
								{
									Name:  "SOLR_JAVA_MEM",
									Value: solrCloud.Spec.SolrJavaMem,
								},
								{
									Name:  "SOLR_HOME",
									Value: "/var/solr/data",
								},
								{
									Name:  "SOLR_PORT",
									Value: strconv.Itoa(SolrClientPort),
								},
								{
									Name: "POD_HOSTNAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath:  "metadata.name",
											APIVersion: "v1",
										},
									},
								},
								{
									Name:  "SOLR_HOST",
									Value: solrHostName,
								},
								{
									Name:  "ZK_HOST",
									Value: solrCloudStatus.ZkConnectionString(),
								},
								{
									Name:  "SOLR_LOG_LEVEL",
									Value: solrCloud.Spec.SolrLogLevel,
								},
								{
									Name:  "SOLR_OPTS",
									Value: solrCloud.Spec.SolrOpts,
								},
								{
									Name:  "GC_TUNE",
									Value: solrCloud.Spec.SolrGCTune,
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 20,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
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
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
								PeriodSeconds:       5,
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Scheme: corev1.URISchemeHTTP,
										Path:   "/solr/admin/info/system",
										Port:   intstr.FromInt(SolrClientPort),
									},
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
						},
					},
				},
			},
			VolumeClaimTemplates: pvcs,
		},
	}

	if solrCloud.Spec.SolrImage.ImagePullSecret != "" {
		stateful.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: solrCloud.Spec.SolrImage.ImagePullSecret},
		}
	}

	if solrCloud.Spec.SolrPod.Affinity != nil {
		stateful.Spec.Template.Spec.Affinity = solrCloud.Spec.SolrPod.Affinity
	}

	if solrCloud.Spec.SolrPod.Resources.Limits != nil || solrCloud.Spec.SolrPod.Resources.Requests != nil {
		stateful.Spec.Template.Spec.Containers[0].Resources = solrCloud.Spec.SolrPod.Resources
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
			log.Info("Update SS", "diff", "labels", "label", k, v, to.Labels[k])
		}
		to.Labels[k] = v
	}

	for k, v := range from.Annotations {
		if to.Annotations[k] != v {
			requireUpdate = true
			log.Info("Update SS", "diff", "annotations", "annotation", k, v, to.Annotations[k])
		}
		to.Annotations[k] = v
	}

	if !reflect.DeepEqual(to.Spec.Replicas, from.Spec.Replicas) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Replicas changed from", from.Spec.Replicas, "To:", to.Spec.Replicas)
		to.Spec.Replicas = from.Spec.Replicas
	}

	if !reflect.DeepEqual(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Selector changed from", from.Spec.Selector, "To:", to.Spec.Selector)
		to.Spec.Selector = from.Spec.Selector
	}

	if !reflect.DeepEqual(to.Spec.VolumeClaimTemplates, from.Spec.VolumeClaimTemplates) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.VolumeClaimTemplates changed from", from.Spec.VolumeClaimTemplates, "To:", to.Spec.VolumeClaimTemplates)
		to.Spec.VolumeClaimTemplates = from.Spec.VolumeClaimTemplates
	}

	if !reflect.DeepEqual(to.Spec.Template.Labels, from.Spec.Template.Labels) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Labels changed from", from.Spec.Template.Labels, "To:", to.Spec.Template.Labels)
		to.Spec.Template.Labels = from.Spec.Template.Labels
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers, from.Spec.Template.Spec.Containers) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Containers changed from", from.Spec.Template.Spec.Containers, "To:", to.Spec.Template.Spec.Containers)
		to.Spec.Template.Spec.Containers = from.Spec.Template.Spec.Containers
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.InitContainers, from.Spec.Template.Spec.InitContainers) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.InitContainers changed from", from.Spec.Template.Spec.InitContainers, "To:", to.Spec.Template.Spec.InitContainers)
		to.Spec.Template.Spec.InitContainers = from.Spec.Template.Spec.InitContainers
	}

	/*if !reflect.DeepEqual(to.Spec.Template.Spec.HostAliases, from.Spec.Template.Spec.HostAliases) {
		requireUpdate = true
		to.Spec.Template.Spec.HostAliases = from.Spec.Template.Spec.HostAliases
	}*/

	if !reflect.DeepEqual(to.Spec.Template.Spec.Volumes, from.Spec.Template.Spec.Volumes) {
		requireUpdate = true
		to.Spec.Template.Spec.Volumes = from.Spec.Template.Spec.Volumes
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.ImagePullSecrets, from.Spec.Template.Spec.ImagePullSecrets) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.ImagePullSecrets changed from", from.Spec.Template.Spec.ImagePullSecrets, "To:", to.Spec.Template.Spec.ImagePullSecrets)
		to.Spec.Template.Spec.ImagePullSecrets = from.Spec.Template.Spec.ImagePullSecrets
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].Resources, from.Spec.Template.Spec.Containers[0].Resources) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Containers[0].Resources changed from", from.Spec.Template.Spec.Containers[0].Resources, "To:", to.Spec.Template.Spec.Containers[0].Resources)
		to.Spec.Template.Spec.Containers[0].Resources = from.Spec.Template.Spec.Containers[0].Resources
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Affinity, from.Spec.Template.Spec.Affinity) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Affinity changed from", from.Spec.Template.Spec.Affinity, "To:", to.Spec.Template.Spec.Affinity)
		to.Spec.Template.Spec.Affinity = from.Spec.Template.Spec.Affinity
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
func GenerateCommonIngress(solrCloud *solr.SolrCloud, nodeNames []string, ingressBaseDomain string) (ingress *extv1.Ingress) {
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

	for _, nodeName := range nodeNames {
		ingressRule := CreateNodeIngressRule(solrCloud, nodeName, ingressBaseDomain)
		rules = append(rules, ingressRule)
	}

	ingress = &extv1.Ingress{
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
func CreateNodeIngressRule(solrCloud *solr.SolrCloud, nodeName string, ingressBaseDomain string) (ingressRule extv1.IngressRule) {
	ingressRule = extv1.IngressRule{
		Host: solrCloud.NodeIngressUrl(nodeName, ingressBaseDomain),
		IngressRuleValue: extv1.IngressRuleValue{
			HTTP: &extv1.HTTPIngressRuleValue{
				Paths: []extv1.HTTPIngressPath{
					{
						Backend: extv1.IngressBackend{
							ServiceName: nodeName,
							ServicePort: intstr.FromInt(ExtSolrClientPort),
						},
					},
				},
			},
		},
	}
	return ingressRule
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

func CallCollectionsApi(cloud string, namespace string, urlParams url.Values, response interface{}) (err error) {
	cloudUrl := solr.InternalURLForCloud(cloud, namespace)

	urlParams.Set("wt", "json")

	cloudUrl = cloudUrl + "/solr/admin/collections?" + urlParams.Encode()

	resp := &http.Response{}
	if resp, err = http.Get(cloudUrl); err != nil {
		return err
	}

	defer resp.Body.Close()

	if err == nil && resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		err = errors.NewServiceUnavailable(fmt.Sprintf("Recieved bad response code of %d from solr with response: %s", resp.StatusCode, string(b)))
	}

	if err == nil {
		json.NewDecoder(resp.Body).Decode(&response)
	}

	return err
}
