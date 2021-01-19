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
	"fmt"
	"sort"
	"strconv"
	"strings"

	solr "github.com/apache/lucene-solr-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	SolrClientPortName  = "solr-client"
	BackupRestoreVolume = "backup-restore"

	SolrNodeContainer = "solrcloud-node"

	DefaultSolrUser  = 8983
	DefaultSolrGroup = 8983

	SolrStorageFinalizer             = "storage.finalizers.solr.apache.org"
	SolrZKConnectionStringAnnotation = "solr.apache.org/zkConnectionString"
	SolrPVCTechnologyLabel           = "solr.apache.org/technology"
	SolrCloudPVCTechnology           = "solr-cloud"
	SolrPVCStorageLabel              = "solr.apache.org/storage"
	SolrCloudPVCDataStorage          = "data"
	SolrPVCInstanceLabel             = "solr.apache.org/instance"
	SolrXmlMd5Annotation             = "solr.apache.org/solrXmlMd5"

	DefaultStatefulSetPodManagementPolicy = appsv1.ParallelPodManagement

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
func GenerateStatefulSet(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus, hostNameIPs map[string]string, solrXmlConfigMapName string, solrXmlMd5 string) *appsv1.StatefulSet {
	gracePeriodTerm := int64(10)
	solrPodPort := solrCloud.Spec.SolrAddressability.PodPort
	fsGroup := int64(DefaultSolrGroup)
	defaultMode := int32(420)
	defaultHandler := corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Scheme: corev1.URISchemeHTTP,
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

	// Keep track of the SolrOpts that the Solr Operator needs to set
	// These will be added to the SolrOpts given by the user.
	allSolrOpts := []string{"-DhostPort=$(SOLR_NODE_PORT)"}

	// Volumes & Mounts
	solrVolumes := []corev1.Volume{
		{
			Name: "solr-xml",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: solrXmlConfigMapName,
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
	if solrCloud.UsesPersistentStorage() {
		pvc := solrCloud.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.DeepCopy()

		// Set the default name of the pvc
		if pvc.ObjectMeta.Name == "" {
			pvc.ObjectMeta.Name = solrDataVolumeName
		}

		// Set some defaults in the PVC Spec
		if len(pvc.Spec.AccessModes) == 0 {
			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
		}
		if pvc.Spec.VolumeMode == nil {
			temp := corev1.PersistentVolumeFilesystem
			pvc.Spec.VolumeMode = &temp
		}

		//  Add internally-used labels.
		internalLabels := map[string]string{
			SolrPVCTechnologyLabel: SolrCloudPVCTechnology,
			SolrPVCStorageLabel:    SolrCloudPVCDataStorage,
			SolrPVCInstanceLabel:   solrCloud.Name,
		}
		pvc.ObjectMeta.Labels = MergeLabelsOrAnnotations(internalLabels, pvc.ObjectMeta.Labels)

		pvcs = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:        pvc.ObjectMeta.Name,
					Labels:      pvc.ObjectMeta.Labels,
					Annotations: pvc.ObjectMeta.Annotations,
				},
				Spec: pvc.Spec,
			},
		}
	} else {
		emptyDirVolume := corev1.Volume{
			Name: solrDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		if solrCloud.Spec.StorageOptions.EphemeralStorage != nil {
			emptyDirVolume.VolumeSource.EmptyDir = &solrCloud.Spec.StorageOptions.EphemeralStorage.EmptyDir
		}
		solrVolumes = append(solrVolumes, emptyDirVolume)
	}
	// Add backup volumes
	if solrCloud.Spec.StorageOptions.BackupRestoreOptions != nil {
		solrVolumes = append(solrVolumes, corev1.Volume{
			Name:         BackupRestoreVolume,
			VolumeSource: solrCloud.Spec.StorageOptions.BackupRestoreOptions.Volume,
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      BackupRestoreVolume,
			MountPath: BaseBackupRestorePath,
			SubPath:   BackupRestoreSubPathForCloud(solrCloud.Spec.StorageOptions.BackupRestoreOptions.Directory, solrCloud.Name),
			ReadOnly:  false,
		})
	}

	if nil != customPodOptions {
		// Add Custom Volumes to pod
		for _, volume := range customPodOptions.Volumes {
			volume.DefaultContainerMount.Name = volume.Name

			// Only add the container mount if one has been provided.
			if volume.DefaultContainerMount != nil {
				volumeMounts = append(volumeMounts, *volume.DefaultContainerMount)
			}

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
			// This is the port that jetty will listen on
			Name:  "SOLR_PORT",
			Value: strconv.Itoa(solrPodPort),
		},
		{
			// This is the port that the Solr Node will advertise itself as listening on in live_nodes
			Name:  "SOLR_NODE_PORT",
			Value: strconv.Itoa(solrAdressingPort),
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
			Name:  "GC_TUNE",
			Value: solrCloud.Spec.SolrGCTune,
		},
	}

	// Add ACL information, if given, through Env Vars
	if hasACLs, aclEnvs := AddACLsToEnv(solrCloud.Spec.ZookeeperRef.ConnectionInfo); hasACLs {
		envVars = append(envVars, aclEnvs...)

		// The $SOLR_ZK_CREDS_AND_ACLS parameter does not get picked up when running solr, it must be added to the SOLR_OPTS.
		allSolrOpts = append(allSolrOpts, "$(SOLR_ZK_CREDS_AND_ACLS)")
	}

	// Add Custom EnvironmentVariables to the solr container
	if nil != customPodOptions {
		envVars = append(envVars, customPodOptions.EnvVariables...)
	}

	// track the MD5 of the custom solr.xml in the pod spec annotations,
	// so we get a rolling restart when the configMap changes
	if solrXmlMd5 != "" {
		if podAnnotations == nil {
			podAnnotations = make(map[string]string, 1)
		}
		podAnnotations[SolrXmlMd5Annotation] = solrXmlMd5
	}

	if solrCloud.Spec.SolrOpts != "" {
		allSolrOpts = append(allSolrOpts, solrCloud.Spec.SolrOpts)
	}

	// Add SOLR_OPTS last, so that it can use values from all of the other ENV_VARS
	envVars = append(envVars, corev1.EnvVar{
		Name:  "SOLR_OPTS",
		Value: strings.Join(allSolrOpts, " "),
	})

	initContainers := generateSolrSetupInitContainers(solrCloud, solrDataVolumeName)

	// Add user defined additional init containers
	if customPodOptions != nil && len(customPodOptions.InitContainers) > 0 {
		initContainers = append(initContainers, customPodOptions.InitContainers...)
	}

	containers := []corev1.Container{
		{
			Name:            SolrNodeContainer,
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
			VolumeMounts: volumeMounts,
			Env:          envVars,
			Lifecycle: &corev1.Lifecycle{
				PostStart: postStart,
				PreStop: &corev1.Handler{
					Exec: &corev1.ExecAction{
						Command: []string{"solr", "stop", "-p", strconv.Itoa(solrPodPort)},
					},
				},
			},
		},
	}

	// Add user defined additional sidecar containers
	if customPodOptions != nil && len(customPodOptions.SidecarContainers) > 0 {
		containers = append(containers, customPodOptions.SidecarContainers...)
	}

	// Decide which update strategy to use
	updateStrategy := appsv1.OnDeleteStatefulSetStrategyType
	if solrCloud.Spec.UpdateStrategy.Method == solr.StatefulSetUpdate {
		// Only use the rolling update strategy if the StatefulSetUpdate method is specified.
		updateStrategy = appsv1.RollingUpdateStatefulSetStrategyType
	}

	// Determine which podManagementPolicy to use for the statefulSet
	podManagementPolicy := DefaultStatefulSetPodManagementPolicy
	if solrCloud.Spec.CustomSolrKubeOptions.StatefulSetOptions != nil && solrCloud.Spec.CustomSolrKubeOptions.StatefulSetOptions.PodManagementPolicy != "" {
		podManagementPolicy = solrCloud.Spec.CustomSolrKubeOptions.StatefulSetOptions.PodManagementPolicy
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
			ServiceName:         solrCloud.HeadlessServiceName(),
			Replicas:            solrCloud.Spec.Replicas,
			PodManagementPolicy: podManagementPolicy,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: updateStrategy,
			},
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
					Volumes:        solrVolumes,
					InitContainers: initContainers,
					HostAliases:    hostAliases,
					Containers:     containers,
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

		if customPodOptions.PriorityClassName != "" {
			stateful.Spec.Template.Spec.PriorityClassName = customPodOptions.PriorityClassName
		}
	}

	return stateful
}

func generateSolrSetupInitContainers(solrCloud *solr.SolrCloud, solrDataVolumeName string) []corev1.Container {
	// The setup of the solr.xml will always be necessary
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "solr-xml",
			MountPath: "/tmp",
		},
		{
			Name:      solrDataVolumeName,
			MountPath: "/tmp-config",
		},
	}
	setupCommands := []string{"cp /tmp/solr.xml /tmp-config/solr.xml"}

	// Add prep for backup-restore volume
	// This entails setting the correct permissions for the directory
	if solrCloud.Spec.StorageOptions.BackupRestoreOptions != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      BackupRestoreVolume,
			MountPath: "/backup-restore",
			SubPath:   BackupRestoreSubPathForCloud(solrCloud.Spec.StorageOptions.BackupRestoreOptions.Directory, solrCloud.Name),
			ReadOnly:  false,
		})

		setupCommands = append(setupCommands, fmt.Sprintf("chown -R %d:%d /backup-restore", DefaultSolrUser, DefaultSolrGroup))
	}

	volumePrepInitContainer := corev1.Container{
		Name:            "cp-solr-xml",
		Image:           solrCloud.Spec.BusyBoxImage.ToImageName(),
		ImagePullPolicy: solrCloud.Spec.BusyBoxImage.PullPolicy,
		Command:         []string{"sh", "-c", strings.Join(setupCommands, " && ")},
		VolumeMounts:    volumeMounts,
	}

	return []corev1.Container{volumePrepInitContainer}
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

// GenerateIngress returns a new Ingress pointer generated for the entire SolrCloud, pointing to all instances
// solrCloud: SolrCloud instance
// nodeStatuses: []SolrNodeStatus the nodeStatuses
// ingressBaseDomain: string baseDomain of the ingress
func GenerateIngress(solrCloud *solr.SolrCloud, nodeNames []string) (ingress *extv1.Ingress) {
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

	ingress = &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.CommonIngressName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: extv1.IngressSpec{
			Rules: rules,
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
	pathType := extv1.PathTypeImplementationSpecific
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
						PathType: &pathType,
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
	pathType := extv1.PathTypeImplementationSpecific
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
						PathType: &pathType,
					},
				},
			},
		},
	}
	return ingressRule
}
