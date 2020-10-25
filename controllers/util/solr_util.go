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
	"sort"
	"strconv"
	"strings"
	"time"

	b64 "encoding/base64"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	"math/rand"
	"regexp"

	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	SolrClientPortName  = "solr-client"
	BackupRestoreVolume = "backup-restore"

	SolrZKConnectionStringAnnotation = "solr.apache.org/zkConnectionString"

	DefaultLivenessProbeInitialDelaySeconds = 20
	DefaultLivenessProbeTimeoutSeconds      = 1
	DefaultLivenessProbeSuccessThreshold    = 1
	DefaultLivenessProbeFailureThreshold    = 3
	DefaultLivenessProbePeriodSeconds       = 10

	DefaultReadinessProbeInitialDelaySeconds = 15
	DefaultReadinessProbeTimeoutSeconds      = 1
	DefaultReadinessProbeSuccessThreshold    = 1
	DefaultReadinessProbeFailureThreshold    = 3
	DefaultReadinessProbePeriodSeconds       = 5

	DefaultStartupProbeInitialDelaySeconds = 20
	DefaultStartupProbeTimeoutSeconds      = 30
	DefaultStartupProbeSuccessThreshold    = 1
	DefaultStartupProbeFailureThreshold    = 15
	DefaultStartupProbePeriodSeconds       = 10
)

// GenerateStatefulSet returns a new appsv1.StatefulSet pointer generated for the SolrCloud instance
// object: SolrCloud instance
// replicas: the number of replicas for the SolrCloud instance
// storage: the size of the storage for the SolrCloud instance (e.g. 100Gi)
// zkConnectionString: the connectionString of the ZK instance to connect to
func GenerateStatefulSet(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus, hostNameIPs map[string]string, createPkcs12InitContainer bool) *appsv1.StatefulSet {
	gracePeriodTerm := int64(10)
	solrPodPort := solrCloud.Spec.SolrAddressability.PodPort
	fsGroup := int64(solrPodPort)
	defaultMode := int32(420)
	probeScheme := corev1.URISchemeHTTP
	if solrCloud.Spec.SolrTLS != nil {
		probeScheme = corev1.URISchemeHTTPS
	}
	defaultHandler := corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Scheme: probeScheme,
			Path:   "/solr/admin/info/system",
			Port:   intstr.FromInt(solrPodPort),
		},
	}

	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	selectorLabels := solrCloud.SharedLabels()

	labels["technology"] = solr.SolrTechnologyLabel
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	annotations := map[string]string{
		SolrZKConnectionStringAnnotation: solrCloudStatus.ZkConnectionString(),
	}

	podLabels := labels

	customSSOptions := solrCloud.Spec.CustomSolrKubeOptions.StatefulSetOptions
	if nil != customSSOptions {
		labels = MergeLabelsOrAnnotations(labels, customSSOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customSSOptions.Annotations)
	}

	customPodOptions := solrCloud.Spec.CustomSolrKubeOptions.PodOptions
	var podAnnotations map[string]string
	if nil != customPodOptions {
		podLabels = MergeLabelsOrAnnotations(podLabels, customPodOptions.Labels)
		podAnnotations = customPodOptions.Annotations
	}

	// Volumes & Mounts
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

	if solrCloud.Spec.SolrTLS != nil {
		solrVolumes = append(solrVolumes, tlsVolumes(solrCloud.Spec.SolrTLS, createPkcs12InitContainer)...)
	}

	solrDataVolumeName := "data"
	volumeMounts := []corev1.VolumeMount{{Name: solrDataVolumeName, MountPath: "/var/solr/data"}}

	if solrCloud.Spec.SolrTLS != nil {
		volumeMounts = append(volumeMounts, tlsVolumeMounts(createPkcs12InitContainer)...)
	}

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

	if nil != customPodOptions {
		// Add Custom Volumes to pod
		for _, volume := range customPodOptions.Volumes {
			volume.DefaultContainerMount.Name = volume.Name
			volumeMounts = append(volumeMounts, volume.DefaultContainerMount)

			solrVolumes = append(solrVolumes, corev1.Volume{
				Name:         volume.Name,
				VolumeSource: volume.Source,
			})
		}
	}

	// Host Aliases
	hostAliases := make([]corev1.HostAlias, len(hostNameIPs))
	if len(hostAliases) == 0 {
		hostAliases = nil
	} else {
		hostNames := make([]string, len(hostNameIPs))
		index := 0
		for hostName := range hostNameIPs {
			hostNames[index] = hostName
			index += 1
		}

		sort.Strings(hostNames)

		for index, hostName := range hostNames {
			hostAliases[index] = corev1.HostAlias{
				IP:        hostNameIPs[hostName],
				Hostnames: []string{hostName},
			}
			index++
		}
	}

	// if an ingressBaseDomain is provided, the node should be addressable outside of the cluster
	solrHostName := solrCloud.AdvertisedNodeHost("$(POD_HOSTNAME)")
	solrAdressingPort := solrCloud.NodePort()

	zkConnectionStr, zkServer, zkChroot := solrCloudStatus.DissectZkInfo()

	// Only have a postStart command to create the chRoot, if it is not '/' (which does not need to be created)
	var postStart *corev1.Handler
	if len(zkChroot) > 1 {
		postStart = &corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"sh", "-c", "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}"},
			},
		}
	}

	// Environment Variables
	envVars := []corev1.EnvVar{
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
			Value: strconv.Itoa(solrPodPort),
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
			Value: zkConnectionStr,
		},
		{
			Name:  "ZK_SERVER",
			Value: zkServer,
		},
		{
			Name:  "ZK_CHROOT",
			Value: zkChroot,
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
	}

	// Append TLS related env vars if enabled
	if solrCloud.Spec.SolrTLS != nil {
		envVars = append(envVars, TLSEnvVars(solrCloud.Spec.SolrTLS, createPkcs12InitContainer)...)
	}

	// Add Custom EnvironmentVariables to the solr container
	if nil != customPodOptions {
		envVars = append(envVars, customPodOptions.EnvVariables...)
	}

	// Create the Stateful Set
	stateful := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.StatefulSetName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			ServiceName: solrCloud.HeadlessServiceName(),
			Replicas:    solrCloud.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: podAnnotations,
				},

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
									ContainerPort: int32(solrPodPort),
									Name:          SolrClientPortName,
									Protocol:      "TCP",
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: DefaultLivenessProbeInitialDelaySeconds,
								TimeoutSeconds:      DefaultLivenessProbeTimeoutSeconds,
								SuccessThreshold:    DefaultLivenessProbeSuccessThreshold,
								FailureThreshold:    DefaultLivenessProbeFailureThreshold,
								PeriodSeconds:       DefaultLivenessProbePeriodSeconds,
								Handler:             defaultHandler,
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: DefaultReadinessProbeInitialDelaySeconds,
								TimeoutSeconds:      DefaultReadinessProbeTimeoutSeconds,
								SuccessThreshold:    DefaultReadinessProbeSuccessThreshold,
								FailureThreshold:    DefaultReadinessProbeFailureThreshold,
								PeriodSeconds:       DefaultReadinessProbePeriodSeconds,
								Handler:             defaultHandler,
							},
							VolumeMounts:             volumeMounts,
							Args:                     []string{"-DhostPort=" + strconv.Itoa(solrAdressingPort)},
							Env:                      envVars,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							Lifecycle: &corev1.Lifecycle{
								PostStart: postStart,
								PreStop: &corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"solr", "stop", "-p", strconv.Itoa(solrPodPort)},
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

	if createPkcs12InitContainer {
		stateful.Spec.Template.Spec.InitContainers = append(stateful.Spec.Template.Spec.InitContainers, generatePkcs12InitContainer(solrCloud))
	}

	if solrCloud.Spec.SolrImage.ImagePullSecret != "" {
		stateful.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: solrCloud.Spec.SolrImage.ImagePullSecret},
		}
	}

	// DEPRECATED: Replaced by the options below
	if solrCloud.Spec.SolrPod.Affinity != nil {
		stateful.Spec.Template.Spec.Affinity = solrCloud.Spec.SolrPod.Affinity
	}

	// DEPRECATED: Replaced by the options below
	if solrCloud.Spec.SolrPod.Resources.Limits != nil || solrCloud.Spec.SolrPod.Resources.Requests != nil {
		stateful.Spec.Template.Spec.Containers[0].Resources = solrCloud.Spec.SolrPod.Resources
	}

	if nil != customPodOptions {
		if customPodOptions.Affinity != nil {
			stateful.Spec.Template.Spec.Affinity = customPodOptions.Affinity
		}

		if customPodOptions.Resources.Limits != nil || customPodOptions.Resources.Requests != nil {
			stateful.Spec.Template.Spec.Containers[0].Resources = customPodOptions.Resources
		}

		if customPodOptions.PodSecurityContext != nil {
			stateful.Spec.Template.Spec.SecurityContext = customPodOptions.PodSecurityContext
		}

		if customPodOptions.Tolerations != nil {
			stateful.Spec.Template.Spec.Tolerations = customPodOptions.Tolerations
		}

		if customPodOptions.NodeSelector != nil {
			stateful.Spec.Template.Spec.NodeSelector = customPodOptions.NodeSelector
		}

		if customPodOptions.LivenessProbe != nil {
			stateful.Spec.Template.Spec.Containers[0].LivenessProbe = fillProbe(*customPodOptions.LivenessProbe, DefaultLivenessProbeInitialDelaySeconds, DefaultLivenessProbeTimeoutSeconds, DefaultLivenessProbeSuccessThreshold, DefaultLivenessProbeFailureThreshold, DefaultLivenessProbePeriodSeconds, &defaultHandler)
		}

		if customPodOptions.ReadinessProbe != nil {
			stateful.Spec.Template.Spec.Containers[0].ReadinessProbe = fillProbe(*customPodOptions.ReadinessProbe, DefaultReadinessProbeInitialDelaySeconds, DefaultReadinessProbeTimeoutSeconds, DefaultReadinessProbeSuccessThreshold, DefaultReadinessProbeFailureThreshold, DefaultReadinessProbePeriodSeconds, &defaultHandler)
		}

		if customPodOptions.StartupProbe != nil {
			stateful.Spec.Template.Spec.Containers[0].StartupProbe = fillProbe(*customPodOptions.StartupProbe, DefaultStartupProbeInitialDelaySeconds, DefaultStartupProbeTimeoutSeconds, DefaultStartupProbeSuccessThreshold, DefaultStartupProbeFailureThreshold, DefaultStartupProbePeriodSeconds, &defaultHandler)
		}

	}

	return stateful
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

	if !DeepEqualWithNils(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Selector changed from", to.Spec.Selector, "To:", from.Spec.Selector)
		to.Spec.Selector = from.Spec.Selector
	}

	requireVolumeUpdate := false
	if len(from.Spec.VolumeClaimTemplates) != len(to.Spec.VolumeClaimTemplates) {
		requireVolumeUpdate = true
		log.Info("Update required because:", "Spec.VolumeClaimTemplates changed from", to.Spec.VolumeClaimTemplates, "To:", from.Spec.VolumeClaimTemplates)
	}
	for i, fromVct := range from.Spec.VolumeClaimTemplates {
		if !DeepEqualWithNils(to.Spec.VolumeClaimTemplates[i].Spec, fromVct.Spec) {
			requireVolumeUpdate = true
			log.Info("Update required because:", "Spec.VolumeClaimTemplates.Spec changed from", to.Spec.VolumeClaimTemplates[i].Spec, "To:", fromVct.Spec)
		}
	}
	if requireVolumeUpdate {
		to.Spec.Template.Labels = from.Spec.Template.Labels
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

	if !DeepEqualWithNils(to.Spec.Template.Spec.Containers, from.Spec.Template.Spec.Containers) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Containers changed from", to.Spec.Template.Spec.Containers, "To:", from.Spec.Template.Spec.Containers)
		to.Spec.Template.Spec.Containers = from.Spec.Template.Spec.Containers
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.InitContainers, from.Spec.Template.Spec.InitContainers) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.InitContainers changed from", to.Spec.Template.Spec.InitContainers, "To:", from.Spec.Template.Spec.InitContainers)
		to.Spec.Template.Spec.InitContainers = from.Spec.Template.Spec.InitContainers
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.HostAliases, from.Spec.Template.Spec.HostAliases) {
		requireUpdate = true
		to.Spec.Template.Spec.HostAliases = from.Spec.Template.Spec.HostAliases
		log.Info("Update required because:", "Spec.Template.Spec.HostAliases changed from", to.Spec.Template.Spec.HostAliases, "To:", from.Spec.Template.Spec.HostAliases)
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.Volumes, from.Spec.Template.Spec.Volumes) {
		requireUpdate = true
		to.Spec.Template.Spec.Volumes = from.Spec.Template.Spec.Volumes
		log.Info("Update required because:", "Spec.Template.Spec.Volumes changed from", to.Spec.Template.Spec.Volumes, "To:", from.Spec.Template.Spec.Volumes)
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.ImagePullSecrets, from.Spec.Template.Spec.ImagePullSecrets) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.ImagePullSecrets changed from", to.Spec.Template.Spec.ImagePullSecrets, "To:", from.Spec.Template.Spec.ImagePullSecrets)
		to.Spec.Template.Spec.ImagePullSecrets = from.Spec.Template.Spec.ImagePullSecrets
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.Containers[0].Resources, from.Spec.Template.Spec.Containers[0].Resources) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Containers[0].Resources changed from", to.Spec.Template.Spec.Containers[0].Resources, "To:", from.Spec.Template.Spec.Containers[0].Resources)
		to.Spec.Template.Spec.Containers[0].Resources = from.Spec.Template.Spec.Containers[0].Resources
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

	if !DeepEqualWithNils(to.Spec.Template.Spec.NodeSelector, from.Spec.Template.Spec.NodeSelector) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.NodeSelector changed from", to.Spec.Template.Spec.NodeSelector, "To:", from.Spec.Template.Spec.NodeSelector)
		to.Spec.Template.Spec.NodeSelector = from.Spec.Template.Spec.NodeSelector
	}

	if !DeepEqualWithNils(to.Spec.Template.Spec.Tolerations, from.Spec.Template.Spec.Tolerations) {
		requireUpdate = true
		log.Info("Update required because:", "Spec.Template.Spec.Tolerations changed from", to.Spec.Template.Spec.Tolerations, "To:", from.Spec.Template.Spec.Tolerations)
		to.Spec.Template.Spec.Tolerations = from.Spec.Template.Spec.Tolerations
	}

	return requireUpdate
}

// GenerateConfigMap returns a new corev1.ConfigMap pointer generated for the SolrCloud instance solr.xml
// solrCloud: SolrCloud instance
func GenerateConfigMap(solrCloud *solr.SolrCloud) *corev1.ConfigMap {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	var annotations map[string]string

	customOptions := solrCloud.Spec.CustomSolrKubeOptions.ConfigMapOptions
	if nil != customOptions {
		labels = MergeLabelsOrAnnotations(labels, customOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customOptions.Annotations)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.ConfigMapName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Data: map[string]string{
			"solr.xml": `<?xml version="1.0" encoding="UTF-8" ?>
<solr>
  <solrcloud>
    <str name="host">${host:}</str>
    <int name="hostPort">${hostPort:80}</int>
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
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !DeepEqualWithNils(to.Data, from.Data) {
		requireUpdate = true
	}
	to.Data = from.Data

	return requireUpdate
}

// fillProbe builds the probe logic used for pod liveness, readiness, startup checks
func fillProbe(customSolrKubeOptions corev1.Probe, defaultInitialDelaySeconds int32, defaultTimeoutSeconds int32, defaultSuccessThreshold int32, defaultFailureThreshold int32, defaultPeriodSeconds int32, defaultHandler *corev1.Handler) *corev1.Probe {
	probe := &corev1.Probe{
		InitialDelaySeconds: defaultInitialDelaySeconds,
		TimeoutSeconds:      defaultTimeoutSeconds,
		SuccessThreshold:    defaultSuccessThreshold,
		FailureThreshold:    defaultFailureThreshold,
		PeriodSeconds:       defaultPeriodSeconds,
		Handler:             *defaultHandler,
	}

	if customSolrKubeOptions.InitialDelaySeconds != 0 {
		probe.InitialDelaySeconds = customSolrKubeOptions.InitialDelaySeconds
	}

	if customSolrKubeOptions.TimeoutSeconds != 0 {
		probe.TimeoutSeconds = customSolrKubeOptions.TimeoutSeconds
	}

	if customSolrKubeOptions.SuccessThreshold != 0 {
		probe.SuccessThreshold = customSolrKubeOptions.SuccessThreshold
	}

	if customSolrKubeOptions.FailureThreshold != 0 {
		probe.FailureThreshold = customSolrKubeOptions.FailureThreshold
	}

	if customSolrKubeOptions.PeriodSeconds != 0 {
		probe.PeriodSeconds = customSolrKubeOptions.PeriodSeconds
	}

	if customSolrKubeOptions.Handler.Exec != nil || customSolrKubeOptions.Handler.HTTPGet != nil {
		probe.Handler = customSolrKubeOptions.Handler
	}

	return probe
}

// GenerateCommonService returns a new corev1.Service pointer generated for the entire SolrCloud instance
// solrCloud: SolrCloud instance
func GenerateCommonService(solrCloud *solr.SolrCloud) *corev1.Service {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["service-type"] = "common"

	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	var annotations map[string]string

	// Add externalDNS annotation if necessary
	extOpts := solrCloud.Spec.SolrAddressability.External
	if extOpts != nil && extOpts.Method == solr.ExternalDNS && !extOpts.HideCommon {
		annotations = make(map[string]string, 1)
		urls := []string{solrCloud.ExternalDnsDomain(extOpts.DomainName)}
		for _, domain := range extOpts.AdditionalDomainNames {
			urls = append(urls, solrCloud.ExternalDnsDomain(domain))
		}
		annotations["external-dns.alpha.kubernetes.io/hostname"] = strings.Join(urls, ",")
	}

	customOptions := solrCloud.Spec.CustomSolrKubeOptions.CommonServiceOptions
	if nil != customOptions {
		labels = MergeLabelsOrAnnotations(labels, customOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customOptions.Annotations)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.CommonServiceName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: SolrClientPortName, Port: int32(solrCloud.Spec.SolrAddressability.CommonServicePort), Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(SolrClientPortName)},
			},
			Selector: selectorLabels,
		},
	}
	return service
}

// GenerateHeadlessService returns a new Headless corev1.Service pointer generated for the SolrCloud instance
// The PublishNotReadyAddresses option is set as true, because we want each pod to be reachable no matter the readiness of the pod.
// solrCloud: SolrCloud instance
func GenerateHeadlessService(solrCloud *solr.SolrCloud) *corev1.Service {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["service-type"] = "headless"

	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	var annotations map[string]string

	// Add externalDNS annotation if necessary
	extOpts := solrCloud.Spec.SolrAddressability.External
	if extOpts != nil && extOpts.Method == solr.ExternalDNS && !extOpts.HideNodes {
		annotations = make(map[string]string, 1)
		urls := []string{solrCloud.ExternalDnsDomain(extOpts.DomainName)}
		for _, domain := range extOpts.AdditionalDomainNames {
			urls = append(urls, solrCloud.ExternalDnsDomain(domain))
		}
		annotations["external-dns.alpha.kubernetes.io/hostname"] = strings.Join(urls, ",")
	}

	customOptions := solrCloud.Spec.CustomSolrKubeOptions.HeadlessServiceOptions
	if nil != customOptions {
		labels = MergeLabelsOrAnnotations(labels, customOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customOptions.Annotations)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.HeadlessServiceName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: SolrClientPortName, Port: int32(solrCloud.NodePort()), Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(SolrClientPortName)},
			},
			Selector:                 selectorLabels,
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
		},
	}
	return service
}

// GenerateNodeService returns a new External corev1.Service pointer generated for the given Solr Node.
// The PublishNotReadyAddresses option is set as true, because we want each pod to be reachable no matter the readiness of the pod.
// solrCloud: SolrCloud instance
// nodeName: string node
func GenerateNodeService(solrCloud *solr.SolrCloud, nodeName string) *corev1.Service {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels["service-type"] = "external"

	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel
	selectorLabels["statefulset.kubernetes.io/pod-name"] = nodeName

	var annotations map[string]string

	customOptions := solrCloud.Spec.CustomSolrKubeOptions.NodeServiceOptions
	if nil != customOptions {
		labels = MergeLabelsOrAnnotations(labels, customOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customOptions.Annotations)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nodeName,
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{Name: SolrClientPortName, Port: int32(solrCloud.NodePort()), Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(SolrClientPortName)},
			},
			PublishNotReadyAddresses: true,
		},
	}
	return service
}

// CopyServiceFields copies the owned fields from one Service to another
func CopyServiceFields(from, to *corev1.Service) bool {
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !DeepEqualWithNils(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
	}
	to.Spec.Selector = from.Spec.Selector

	if !DeepEqualWithNils(to.Spec.Ports, from.Spec.Ports) {
		requireUpdate = true
	}
	to.Spec.Ports = from.Spec.Ports

	if !DeepEqualWithNils(to.Spec.ExternalName, from.Spec.ExternalName) {
		requireUpdate = true
	}
	to.Spec.ExternalName = from.Spec.ExternalName

	if !DeepEqualWithNils(to.Spec.PublishNotReadyAddresses, from.Spec.PublishNotReadyAddresses) {
		requireUpdate = true
	}
	to.Spec.PublishNotReadyAddresses = from.Spec.PublishNotReadyAddresses

	return requireUpdate
}

// GenerateIngress returns a new Ingress pointer generated for the entire SolrCloud, pointing to all instances
// solrCloud: SolrCloud instance
// nodeStatuses: []SolrNodeStatus the nodeStatuses
// ingressBaseDomain: string baseDomain of the ingress
func GenerateIngress(solrCloud *solr.SolrCloud, nodeNames []string, ingressBaseDomain string) (ingress *extv1.Ingress) {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	var annotations map[string]string

	customOptions := solrCloud.Spec.CustomSolrKubeOptions.IngressOptions
	if nil != customOptions {
		labels = MergeLabelsOrAnnotations(labels, customOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customOptions.Annotations)
	}

	extOpts := solrCloud.Spec.SolrAddressability.External

	// Create advertised domain name and possible additional domain names
	rules := CreateSolrIngressRules(solrCloud, nodeNames, append([]string{extOpts.DomainName}, extOpts.AdditionalDomainNames...))

	var ingressTLS []extv1.IngressTLS
	if solrCloud.Spec.SolrTLS != nil {
		if annotations == nil {
			annotations = make(map[string]string, 1)
		}
		_, ok := annotations["nginx.ingress.kubernetes.io/backend-protocol"]
		if !ok {
			annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"
		}
		ingressTLS = append(ingressTLS, extv1.IngressTLS{SecretName: solrCloud.Spec.SolrTLS.PKCS12Secret.Name})
	}

	ingress = &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.CommonIngressName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: extv1.IngressSpec{
			Rules: rules,
			TLS:   ingressTLS,
		},
	}
	return ingress
}

// CreateSolrIngressRules returns all applicable ingress rules for a cloud.
// solrCloud: SolrCloud instance
// nodeNames: the names for each of the solr pods
// domainName: string Domain for the ingress rule to use
func CreateSolrIngressRules(solrCloud *solr.SolrCloud, nodeNames []string, domainNames []string) []extv1.IngressRule {
	var ingressRules []extv1.IngressRule
	if !solrCloud.Spec.SolrAddressability.External.HideCommon {
		for _, domainName := range domainNames {
			ingressRules = append(ingressRules, CreateCommonIngressRule(solrCloud, domainName))
		}
	}
	if !solrCloud.Spec.SolrAddressability.External.HideNodes {
		for _, nodeName := range nodeNames {
			for _, domainName := range domainNames {
				ingressRules = append(ingressRules, CreateNodeIngressRule(solrCloud, nodeName, domainName))
			}
		}
	}
	return ingressRules
}

// CreateCommonIngressRule returns a new Ingress Rule generated for a SolrCloud under the given domainName
// solrCloud: SolrCloud instance
// domainName: string Domain for the ingress rule to use
func CreateCommonIngressRule(solrCloud *solr.SolrCloud, domainName string) (ingressRule extv1.IngressRule) {
	ingressRule = extv1.IngressRule{
		Host: solrCloud.ExternalCommonUrl(domainName, false),
		IngressRuleValue: extv1.IngressRuleValue{
			HTTP: &extv1.HTTPIngressRuleValue{
				Paths: []extv1.HTTPIngressPath{
					{
						Backend: extv1.IngressBackend{
							ServiceName: solrCloud.CommonServiceName(),
							ServicePort: intstr.FromInt(solrCloud.Spec.SolrAddressability.CommonServicePort),
						},
					},
				},
			},
		},
	}
	return ingressRule
}

// CreateNodeIngressRule returns a new Ingress Rule generated for a specific Solr Node under the given domainName
// solrCloud: SolrCloud instance
// nodeName: string Name of the node
// domainName: string Domain for the ingress rule to use
func CreateNodeIngressRule(solrCloud *solr.SolrCloud, nodeName string, domainName string) (ingressRule extv1.IngressRule) {
	ingressRule = extv1.IngressRule{
		Host: solrCloud.ExternalNodeUrl(nodeName, domainName, false),
		IngressRuleValue: extv1.IngressRuleValue{
			HTTP: &extv1.HTTPIngressRuleValue{
				Paths: []extv1.HTTPIngressPath{
					{
						Backend: extv1.IngressBackend{
							ServiceName: nodeName,
							ServicePort: intstr.FromInt(solrCloud.NodePort()),
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
	requireUpdate := CopyLabelsAndAnnotations(&from.ObjectMeta, &to.ObjectMeta)

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !DeepEqualWithNils(to.Spec.Rules, from.Spec.Rules) {
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

func GenerateCertificate(solrCloud *solr.SolrCloud) certv1.Certificate {

	dnsNames := findDNSNamesForCertificate(solrCloud)

	// convert something like: CN=localhost, OU=Organizational Unit, O=Organization, L=Location, ST=State, C=Country
	// into an X509Subject to be used during cert creation
	subjectMap := parseSubjectDName(solrCloud.Spec.SolrTLS.AutoCreate.SubjectDistinguishedName)
	subject := toX509Subject(subjectMap)
	commonName := subjectMap["CN"]

	var issuerRef certmetav1.ObjectReference
	if solrCloud.Spec.SolrTLS.AutoCreate.IssuerRef != nil {
		issuerRef = certmetav1.ObjectReference{
			Name: solrCloud.Spec.SolrTLS.AutoCreate.IssuerRef.Name,
			Kind: solrCloud.Spec.SolrTLS.AutoCreate.IssuerRef.Kind,
		}
	} else {
		issuerRef = certmetav1.ObjectReference{
			Name: fmt.Sprintf("%s-selfsigned-issuer", solrCloud.Name),
			Kind: "Issuer",
		}
	}

	cert := certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.Spec.SolrTLS.AutoCreate.Name,
			Namespace: solrCloud.Namespace,
		},
		Spec: certv1.CertificateSpec{
			Subject:    subject,
			CommonName: commonName,
			SecretName: solrCloud.Spec.SolrTLS.PKCS12Secret.Name,
			DNSNames:   dnsNames,
			Keystores: &certv1.CertificateKeystores{
				PKCS12: &certv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: certmetav1.SecretKeySelector{
						Key: solrCloud.Spec.SolrTLS.KeyStorePasswordSecret.Key,
						LocalObjectReference: certmetav1.LocalObjectReference{
							Name: solrCloud.Spec.SolrTLS.KeyStorePasswordSecret.Name,
						},
					},
				},
			},
			IssuerRef: issuerRef,
		},
	}

	return cert
}

func toX509Subject(subjectMap map[string]string) *certv1.X509Subject {
	var provinces []string
	// sometimes it's ST and other times it's S ... handle both
	if subjectMap["ST"] != "" {
		provinces = append(provinces, subjectMap["ST"])
	}
	if subjectMap["S"] != "" {
		provinces = append(provinces, subjectMap["S"])
	}
	return &certv1.X509Subject{
		Organizations:       toX509SubjectField(subjectMap["O"]),
		Countries:           toX509SubjectField(subjectMap["C"]),
		OrganizationalUnits: toX509SubjectField(subjectMap["OU"]),
		Localities:          toX509SubjectField(subjectMap["L"]),
		Provinces:           provinces,
		StreetAddresses:     toX509SubjectField(subjectMap["STREET"]),
		PostalCodes:         toX509SubjectField(subjectMap["PC"]),
		SerialNumber:        subjectMap["SERIALNUMBER"],
	}
}

func toX509SubjectField(fieldValue string) []string {
	if fieldValue != "" {
		return []string{fieldValue}
	} else {
		return nil
	}
}

func parseSubjectDName(dname string) map[string]string {
	parsed := make(map[string]string)
	fields := []string{"CN", "O", "OU", "L", "ST", "S", "C", "SERIALNUMBER", "STREET", "PC"}
	for _, f := range fields {
		regex := fmt.Sprintf("(?i)%s=([^,]+)", f)
		match := regexp.MustCompile(regex).FindStringSubmatch(dname)
		if len(match) == 2 {
			parsed[f] = match[1]
		}
	}
	return parsed
}

// used to generate a random password to be used for the keystore
func randomPasswordB64() []byte {
	rand.Seed(time.Now().UnixNano())
	lower := "abcdefghijklmnpqrstuvwxyz" // no 'o'
	chars := lower + strings.ToUpper(lower) + "0123456789()[]~%$!#"
	pass := make([]byte, 12)
	perm := rand.Perm(len(chars))
	for i := 0; i < len(pass); i++ {
		pass[i] = chars[perm[i]]
	}
	return []byte(b64.StdEncoding.EncodeToString(pass))
}

func GenerateKeystoreSecret(solrCloud *solr.SolrCloud) corev1.Secret {
	secretName := solrCloud.Spec.SolrTLS.KeyStorePasswordSecret.Name
	data := map[string][]byte{}
	data["password-key"] = randomPasswordB64()
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: solrCloud.Namespace},
		Data:       data,
		Type:       corev1.SecretTypeOpaque,
	}
}

func GenerateSelfSignedIssuer(solrCloud *solr.SolrCloud, issuerName string) certv1.Issuer {
	return certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{Name: issuerName, Namespace: solrCloud.Namespace},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				SelfSigned: &certv1.SelfSignedIssuer{},
			},
		},
	}
}

func TLSEnvVars(opts *solr.SolrTLSOptions, createPkcs12InitContainer bool) []corev1.EnvVar {

	// Determine the correct values for the SOLR_SSL_WANT_CLIENT_AUTH and SOLR_SSL_NEED_CLIENT_AUTH vars
	wantClientAuth := "false"
	needClientAuth := "false"
	if opts.ClientAuth == solr.Need {
		needClientAuth = "true"
	} else if opts.ClientAuth == solr.Want {
		wantClientAuth = "true"
	}

	// the keystore path depends on whether we're just loading it from the secret or whether
	// our initContainer has to generate it from the TLS secret using openssl
	// this complexity is due to the secret mount directory not being writable
	var keystorePath string
	if createPkcs12InitContainer {
		keystorePath = solr.DefaultWritableKeyStorePath
	} else {
		keystorePath = solr.DefaultKeyStorePath
	}
	keystorePath += ("/" + solr.DefaultKeyStoreFile)

	passwordValueFrom := &corev1.EnvVarSource{SecretKeyRef: opts.KeyStorePasswordSecret}

	envVars := []corev1.EnvVar{
		{
			Name:  "SOLR_SSL_ENABLED",
			Value: "true",
		},
		{
			Name:  "SOLR_SSL_KEY_STORE",
			Value: keystorePath,
		},
		{
			Name:      "SOLR_SSL_KEY_STORE_PASSWORD",
			ValueFrom: passwordValueFrom,
		},
		{
			Name:  "SOLR_SSL_TRUST_STORE",
			Value: keystorePath,
		},
		{
			Name:      "SOLR_SSL_TRUST_STORE_PASSWORD",
			ValueFrom: passwordValueFrom,
		},
		{
			Name:  "SOLR_SSL_WANT_CLIENT_AUTH",
			Value: wantClientAuth,
		},
		{
			Name:  "SOLR_SSL_NEED_CLIENT_AUTH",
			Value: needClientAuth,
		},
		{
			Name:  "SOLR_SSL_CLIENT_HOSTNAME_VERIFICATION",
			Value: strconv.FormatBool(opts.VerifyClientHostname),
		},
		{
			Name:  "SOLR_SSL_CHECK_PEER_NAME",
			Value: strconv.FormatBool(opts.CheckPeerName),
		},
	}

	if opts.RestartOnTLSSecretUpdate && opts.TLSSecretVersion != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "SOLR_TLS_SECRET_VERS", Value: opts.TLSSecretVersion})
	}

	return envVars
}

func tlsVolumeMounts(createPkcs12InitContainer bool) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "keystore",
			ReadOnly:  true,
			MountPath: solr.DefaultKeyStorePath,
		},
	}

	// We need an initContainer to convert a TLS cert into the pkcs12 format Java wants (using openssl)
	// but openssl cannot write to the /var/solr/tls directory because of the way secret mounts work
	// so we need to mount an empty directory to write pkcs12 keystore into
	if createPkcs12InitContainer {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "pkcs12",
			ReadOnly:  false,
			MountPath: solr.DefaultWritableKeyStorePath,
		})
	}

	return mounts
}

func tlsVolumes(opts *solr.SolrTLSOptions, createPkcs12InitContainer bool) []corev1.Volume {
	optional := false
	defaultMode := int32(0664)
	vols := []corev1.Volume{
		{
			Name: "keystore",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  opts.PKCS12Secret.Name,
					DefaultMode: &defaultMode,
					Optional:    &optional,
				},
			},
		},
	}

	if createPkcs12InitContainer {
		vols = append(vols, corev1.Volume{
			Name: "pkcs12",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return vols
}

func findDNSNamesForCertificate(solrCloud *solr.SolrCloud) []string {
	var dnsNames []string
	extOpts := solrCloud.Spec.SolrAddressability.External
	if extOpts != nil {
		rules := CreateSolrIngressRules(solrCloud, []string{}, append([]string{extOpts.DomainName}, extOpts.AdditionalDomainNames...))
		for _, rule := range rules {
			dnsNames = append(dnsNames, rule.Host)
		}
		if extOpts.DomainName != "" {
			dnsNames = append(dnsNames, extOpts.DomainName, "*."+extOpts.DomainName)
		}
	} else {
		dnsNames = append(dnsNames, solrCloud.AdvertisedNodeHost("*"))
	}
	if len(dnsNames) > 1 {
		dnsNames = deDupeDNSNames(dnsNames)
		sort.Strings(dnsNames)
	}
	return dnsNames
}

func deDupeDNSNames(dnsNames []string) []string {
	keys := make(map[string]bool)
	var set []string
	for _, name := range dnsNames {
		if _, exists := keys[name]; !exists {
			keys[name] = true
			set = append(set, name)
		}
	}
	return set
}

func CopyCreateCertificateFields(from, to *certv1.Certificate) bool {
	requireUpdate := false

	// from is the desired state, built using the user-supplied config
	// to is the current state, returned from an API Get call
	// for changed fields, we take the value in the "from" object and send the "to" object back to the API update call

	if from.Name != to.Name {
		to.Name = from.Name
		requireUpdate = true
	}

	if from.Spec.CommonName != to.Spec.CommonName {
		to.Spec.CommonName = from.Spec.CommonName
		requireUpdate = true
	}

	if from.Spec.SecretName != to.Spec.SecretName {
		to.Spec.SecretName = from.Spec.SecretName
		requireUpdate = true
	}

	if !DeepEqualWithNils(from.Spec.Subject, to.Spec.Subject) {
		to.Spec.Subject = from.Spec.Subject
		requireUpdate = true
	}

	if !DeepEqualWithNils(from.Spec.IssuerRef, to.Spec.IssuerRef) {
		to.Spec.IssuerRef = from.Spec.IssuerRef
		requireUpdate = true
	}

	if !DeepEqualWithNils(from.Spec.DNSNames, to.Spec.DNSNames) {
		to.Spec.DNSNames = from.Spec.DNSNames
		requireUpdate = true
	}

	if !DeepEqualWithNils(from.Spec.Keystores, to.Spec.Keystores) {
		to.Spec.Keystores = from.Spec.Keystores
		requireUpdate = true
	}

	return requireUpdate
}

func generatePkcs12InitContainer(solrCloud *solr.SolrCloud) corev1.Container {
	// get the keystore password from the env for generating the keystore using openssl
	passwordValueFrom := &corev1.EnvVarSource{SecretKeyRef: solrCloud.Spec.SolrTLS.KeyStorePasswordSecret}
	envVars := []corev1.EnvVar{
		{
			Name:      "SOLR_SSL_KEY_STORE_PASSWORD",
			ValueFrom: passwordValueFrom,
		},
	}

	cmd := "openssl pkcs12 -export -in " + solr.DefaultKeyStorePath + "/tls.crt -in " + solr.DefaultKeyStorePath +
		"/ca.crt -inkey " + solr.DefaultKeyStorePath + "/tls.key -out " + solr.DefaultKeyStorePath +
		"/pkcs12/keystore.p12 -passout pass:${SOLR_SSL_KEY_STORE_PASSWORD}"
	return corev1.Container{
		Name:                     "gen-pkcs12-keystore",
		Image:                    solrCloud.Spec.SolrImage.ToImageName(),
		ImagePullPolicy:          solrCloud.Spec.SolrImage.PullPolicy,
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: "File",
		Command:                  []string{"sh", "-c", cmd},
		VolumeMounts:             tlsVolumeMounts(true),
		Env:                      envVars,
	}
}
