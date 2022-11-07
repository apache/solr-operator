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
	solr "github.com/apache/solr-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sort"
	"strconv"
	"strings"
)

const (
	SolrClientPortName = "solr-client"

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
	SolrXmlFile                      = "solr.xml"
	LogXmlMd5Annotation              = "solr.apache.org/logXmlMd5"
	LogXmlFile                       = "log4j2.xml"

	DefaultStatefulSetPodManagementPolicy = appsv1.ParallelPodManagement

	DistLibs    = "/opt/solr/dist"
	ContribLibs = "/opt/solr/contrib/%s/lib"
)

var (
	DefaultSolrVolumePrepInitContainerMemory = resource.NewScaledQuantity(50, 6)
	DefaultSolrVolumePrepInitContainerCPU    = resource.NewMilliQuantity(50, resource.DecimalExponent)
	DefaultSolrZKPrepInitContainerMemory     = resource.NewScaledQuantity(200, 6)
	DefaultSolrZKPrepInitContainerCPU        = resource.NewMilliQuantity(400, resource.DecimalExponent)
)

// GenerateStatefulSet returns a new appsv1.StatefulSet pointer generated for the SolrCloud instance
// object: SolrCloud instance
// replicas: the number of replicas for the SolrCloud instance
// storage: the size of the storage for the SolrCloud instance (e.g. 100Gi)
// zkConnectionString: the connectionString of the ZK instance to connect to
func GenerateStatefulSet(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus, hostNameIPs map[string]string, reconcileConfigInfo map[string]string, tls *TLSCerts, security *SecurityConfig) *appsv1.StatefulSet {
	terminationGracePeriod := int64(60)
	solrPodPort := solrCloud.Spec.SolrAddressability.PodPort
	fsGroup := int64(DefaultSolrGroup)

	probeScheme := corev1.URISchemeHTTP
	if tls != nil {
		probeScheme = corev1.URISchemeHTTPS
	}

	defaultProbeTimeout := int32(1)
	defaultHandler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Scheme: probeScheme,
			Path:   "/solr" + DefaultProbePath,
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

	customPodOptions := solrCloud.Spec.CustomSolrKubeOptions.PodOptions.DeepCopy()
	var podAnnotations map[string]string
	if nil != customPodOptions {
		podLabels = MergeLabelsOrAnnotations(podLabels, customPodOptions.Labels)
		podAnnotations = customPodOptions.Annotations

		if customPodOptions.TerminationGracePeriodSeconds != nil {
			terminationGracePeriod = *customPodOptions.TerminationGracePeriodSeconds
		}
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
						Name: reconcileConfigInfo[SolrXmlFile],
					},
					Items: []corev1.KeyToPath{
						{
							Key:  SolrXmlFile,
							Path: SolrXmlFile,
						},
					},
					DefaultMode: &PublicReadOnlyPermissions,
				},
			},
		},
	}

	solrDataVolumeName := solrCloud.DataVolumeName()

	var pvcs []corev1.PersistentVolumeClaim
	if solrCloud.UsesPersistentStorage() {
		pvc := solrCloud.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.DeepCopy()

		// Set the default name of the pvc
		pvc.ObjectMeta.Name = solrDataVolumeName

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
		ephemeralVolume := corev1.Volume{
			Name:         solrDataVolumeName,
			VolumeSource: corev1.VolumeSource{},
		}
		if solrCloud.Spec.StorageOptions.EphemeralStorage != nil {
			if nil != solrCloud.Spec.StorageOptions.EphemeralStorage.HostPath {
				ephemeralVolume.VolumeSource.HostPath = solrCloud.Spec.StorageOptions.EphemeralStorage.HostPath
			} else if nil != solrCloud.Spec.StorageOptions.EphemeralStorage.EmptyDir {
				ephemeralVolume.VolumeSource.EmptyDir = solrCloud.Spec.StorageOptions.EphemeralStorage.EmptyDir
			} else {
				ephemeralVolume.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
			}
		} else {
			ephemeralVolume.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
		}
		solrVolumes = append(solrVolumes, ephemeralVolume)
	}

	volumeMounts := []corev1.VolumeMount{{Name: solrDataVolumeName, MountPath: "/var/solr/data"}}

	// Add necessary specs for backupRepos
	backupEnvVars := make([]corev1.EnvVar, 0)
	for _, repo := range solrCloud.Spec.BackupRepositories {
		volumeSource, mount := RepoVolumeSourceAndMount(&repo, solrCloud.Name)
		if volumeSource != nil {
			solrVolumes = append(solrVolumes, corev1.Volume{
				Name:         RepoVolumeName(&repo),
				VolumeSource: *volumeSource,
			})
			volumeMounts = append(volumeMounts, *mount)
		}
		repoEnvVars := RepoEnvVars(&repo)
		if len(repoEnvVars) > 0 {
			backupEnvVars = append(backupEnvVars, repoEnvVars...)
		}
	}
	// Add annotation specifying the backupRepositories available with this version of the Pod.
	podAnnotations = SetAvailableBackupRepos(solrCloud, podAnnotations)

	if nil != customPodOptions {
		// Add Custom Volumes to pod
		for _, volume := range customPodOptions.Volumes {
			// Only add the container mount if one has been provided.
			if volume.DefaultContainerMount != nil {
				volume.DefaultContainerMount.Name = volume.Name
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

	solrHostName := solrCloud.AdvertisedNodeHost("$(POD_HOSTNAME)")
	solrAdressingPort := solrCloud.NodePort()

	// Solr can take longer than SOLR_STOP_WAIT to run solr stop, give it a few extra seconds before forcefully killing the pod.
	solrStopWait := terminationGracePeriod - 5
	if solrStopWait < 0 {
		solrStopWait = 0
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
			Name:  "SOLR_LOG_LEVEL",
			Value: solrCloud.Spec.SolrLogLevel,
		},
		{
			Name:  "GC_TUNE",
			Value: solrCloud.Spec.SolrGCTune,
		},
		{
			Name:  "SOLR_STOP_WAIT",
			Value: strconv.FormatInt(solrStopWait, 10),
		},
	}

	// Add all necessary information for connection to Zookeeper
	zkEnvVars, zkSolrOpt, hasChroot := createZkConnectionEnvVars(solrCloud, solrCloudStatus)
	if zkSolrOpt != "" {
		allSolrOpts = append(allSolrOpts, zkSolrOpt)
	}
	envVars = append(envVars, zkEnvVars...)

	// Add envVars for backupRepos if any are needed
	if len(backupEnvVars) > 0 {
		envVars = append(envVars, backupEnvVars...)
	}

	// Only have a postStart command to create the chRoot, if it is not '/' (which does not need to be created)
	var postStart *corev1.LifecycleHandler
	if hasChroot {
		postStart = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"sh", "-c", "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}"},
			},
		}
	}

	// Default preStop hook
	preStop := &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{
			Command: []string{"solr", "stop", "-p", strconv.Itoa(solrPodPort)},
		},
	}

	// Add Custom EnvironmentVariables to the solr container
	if nil != customPodOptions {
		envVars = append(envVars, customPodOptions.EnvVariables...)
	}

	// Did the user provide a custom log config?
	if reconcileConfigInfo[LogXmlFile] != "" {
		if reconcileConfigInfo[LogXmlMd5Annotation] != "" {
			if podAnnotations == nil {
				podAnnotations = make(map[string]string, 1)
			}
			podAnnotations[LogXmlMd5Annotation] = reconcileConfigInfo[LogXmlMd5Annotation]
		}

		// cannot use /var/solr as a mountPath, so mount the custom log config
		// in a sub-dir named after the user-provided ConfigMap
		volMount, envVar, newVolume := setupVolumeMountForUserProvidedConfigMapEntry(reconcileConfigInfo, LogXmlFile, solrVolumes, "LOG4J_PROPS")
		volumeMounts = append(volumeMounts, *volMount)
		envVars = append(envVars, *envVar)
		if newVolume != nil {
			solrVolumes = append(solrVolumes, *newVolume)
		}
	}

	// track the MD5 of the custom solr.xml in the pod spec annotations,
	// so we get a rolling restart when the configMap changes
	if reconcileConfigInfo[SolrXmlMd5Annotation] != "" {
		if podAnnotations == nil {
			podAnnotations = make(map[string]string, 1)
		}
		podAnnotations[SolrXmlMd5Annotation] = reconcileConfigInfo[SolrXmlMd5Annotation]
	}

	if solrCloud.Spec.SolrOpts != "" {
		allSolrOpts = append(allSolrOpts, solrCloud.Spec.SolrOpts)
	}

	// Add SOLR_OPTS last, so that it can use values from all of the other ENV_VARS
	envVars = append(envVars, corev1.EnvVar{
		Name:  "SOLR_OPTS",
		Value: strings.Join(allSolrOpts, " "),
	})

	initContainers := generateSolrSetupInitContainers(solrCloud, solrCloudStatus, solrDataVolumeName, security)

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
				InitialDelaySeconds: 20,
				TimeoutSeconds:      defaultProbeTimeout,
				SuccessThreshold:    1,
				FailureThreshold:    3,
				PeriodSeconds:       10,
				ProbeHandler:        defaultHandler,
			},
			ReadinessProbe: &corev1.Probe{
				InitialDelaySeconds: 15,
				TimeoutSeconds:      defaultProbeTimeout,
				SuccessThreshold:    1,
				FailureThreshold:    3,
				PeriodSeconds:       5,
				ProbeHandler:        defaultHandler,
			},
			VolumeMounts: volumeMounts,
			Env:          envVars,
			Lifecycle: &corev1.Lifecycle{
				PostStart: postStart,
				PreStop:   preStop,
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
					TerminationGracePeriodSeconds: &terminationGracePeriod,
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

	var imagePullSecrets []corev1.LocalObjectReference

	if customPodOptions != nil {
		imagePullSecrets = customPodOptions.ImagePullSecrets
	}

	if solrCloud.Spec.SolrImage.ImagePullSecret != "" {
		imagePullSecrets = append(
			imagePullSecrets,
			corev1.LocalObjectReference{Name: solrCloud.Spec.SolrImage.ImagePullSecret},
		)
	}

	stateful.Spec.Template.Spec.ImagePullSecrets = imagePullSecrets

	if nil != customPodOptions {
		solrContainer := &stateful.Spec.Template.Spec.Containers[0]

		if customPodOptions.ServiceAccountName != "" {
			stateful.Spec.Template.Spec.ServiceAccountName = customPodOptions.ServiceAccountName
		}

		if customPodOptions.Affinity != nil {
			stateful.Spec.Template.Spec.Affinity = customPodOptions.Affinity
		}

		if customPodOptions.Resources.Limits != nil || customPodOptions.Resources.Requests != nil {
			solrContainer.Resources = customPodOptions.Resources
		}

		if customPodOptions.PodSecurityContext != nil {
			stateful.Spec.Template.Spec.SecurityContext = customPodOptions.PodSecurityContext
		}

		if customPodOptions.Lifecycle != nil {
			solrContainer.Lifecycle = customPodOptions.Lifecycle
		}

		if customPodOptions.Tolerations != nil {
			stateful.Spec.Template.Spec.Tolerations = customPodOptions.Tolerations
		}

		if customPodOptions.NodeSelector != nil {
			stateful.Spec.Template.Spec.NodeSelector = customPodOptions.NodeSelector
		}

		if customPodOptions.StartupProbe != nil {
			// Default Solr container does not contain a startupProbe, so copy the livenessProbe
			baseProbe := solrContainer.LivenessProbe.DeepCopy()
			// Two options are different by default from the livenessProbe
			baseProbe.TimeoutSeconds = 30
			baseProbe.FailureThreshold = 15
			solrContainer.StartupProbe = customizeProbe(baseProbe, *customPodOptions.StartupProbe)
		}

		if customPodOptions.LivenessProbe != nil {
			solrContainer.LivenessProbe = customizeProbe(solrContainer.LivenessProbe, *customPodOptions.LivenessProbe)
		}

		if customPodOptions.ReadinessProbe != nil {
			solrContainer.ReadinessProbe = customizeProbe(solrContainer.ReadinessProbe, *customPodOptions.ReadinessProbe)
		}

		if customPodOptions.PriorityClassName != "" {
			stateful.Spec.Template.Spec.PriorityClassName = customPodOptions.PriorityClassName
		}

		if len(customPodOptions.TopologySpreadConstraints) > 0 {
			stateful.Spec.Template.Spec.TopologySpreadConstraints = customPodOptions.TopologySpreadConstraints

			// Set the label selector for constraints to the statefulSet label selector, if none is provided
			for i := range stateful.Spec.Template.Spec.TopologySpreadConstraints {
				if stateful.Spec.Template.Spec.TopologySpreadConstraints[i].LabelSelector == nil {
					stateful.Spec.Template.Spec.TopologySpreadConstraints[i].LabelSelector = stateful.Spec.Selector.DeepCopy()
				}
			}
		}
	}

	// Enrich the StatefulSet config to enable TLS on Solr pods if needed
	if tls != nil {
		tls.enableTLSOnSolrCloudStatefulSet(stateful)
	}

	// If probes require auth is set OR tls is configured to want / need client auth, then reconfigure the probes to use an exec
	if (solrCloud.Spec.SolrSecurity != nil && solrCloud.Spec.SolrSecurity.ProbesRequireAuth) || (tls != nil && tls.ServerConfig != nil && tls.ServerConfig.Options.ClientAuth != solr.None) {
		enableSecureProbesOnSolrCloudStatefulSet(solrCloud, stateful)
	}

	return stateful
}

func generateSolrSetupInitContainers(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus, solrDataVolumeName string, security *SecurityConfig) (containers []corev1.Container) {
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

	// Add prep for backup-restore Repositories
	// This entails setting the correct permissions for the directory
	for _, repo := range solrCloud.Spec.BackupRepositories {
		if IsRepoVolume(&repo) {
			if _, volumeMount := RepoVolumeSourceAndMount(&repo, solrCloud.Name); volumeMount != nil {
				volumeMounts = append(volumeMounts, *volumeMount)

				setupCommands = append(setupCommands, fmt.Sprintf(
					"chown -R %d:%d %s",
					DefaultSolrUser,
					DefaultSolrGroup,
					volumeMount.MountPath))
			}
		}
	}

	volumePrepResources := corev1.ResourceList{
		corev1.ResourceCPU:    *DefaultSolrVolumePrepInitContainerCPU,
		corev1.ResourceMemory: *DefaultSolrVolumePrepInitContainerMemory,
	}
	volumePrepInitContainer := corev1.Container{
		Name:            "cp-solr-xml",
		Image:           solrCloud.Spec.BusyBoxImage.ToImageName(),
		ImagePullPolicy: solrCloud.Spec.BusyBoxImage.PullPolicy,
		Command:         []string{"sh", "-c", strings.Join(setupCommands, " && ")},
		VolumeMounts:    volumeMounts,
		Resources: corev1.ResourceRequirements{
			Requests: volumePrepResources,
			Limits:   volumePrepResources,
		},
	}

	containers = append(containers, volumePrepInitContainer)

	if hasZKSetupContainer, zkSetupContainer := generateZKInteractionInitContainer(solrCloud, solrCloudStatus, security); hasZKSetupContainer {
		containers = append(containers, zkSetupContainer)
	}

	// If the user has provided custom resources for the default init containers, use them
	customPodOptions := solrCloud.Spec.CustomSolrKubeOptions.PodOptions
	if nil != customPodOptions {
		resources := customPodOptions.DefaultInitContainerResources
		if resources.Limits != nil || resources.Requests != nil {
			for i := range containers {
				containers[i].Resources = resources
			}
		}
	}

	return containers
}

const DefaultSolrXML = `<?xml version="1.0" encoding="UTF-8" ?>
<solr>
  %s
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
  %s
</solr>
`

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
			"solr.xml": GenerateSolrXMLStringForCloud(solrCloud),
		},
	}

	return configMap
}

func GenerateSolrXMLStringForCloud(solrCloud *solr.SolrCloud) string {
	backupSection, solrModules, additionalLibs := GenerateBackupRepositoriesForSolrXml(solrCloud.Spec.BackupRepositories)
	solrModules = append(solrModules, solrCloud.Spec.SolrModules...)
	additionalLibs = append(additionalLibs, solrCloud.Spec.AdditionalLibs...)
	return GenerateSolrXMLString(backupSection, solrModules, additionalLibs)
}

func GenerateSolrXMLString(backupSection string, solrModules []string, additionalLibs []string) string {
	return fmt.Sprintf(DefaultSolrXML, GenerateAdditionalLibXMLPart(solrModules, additionalLibs), backupSection)
}

func GenerateAdditionalLibXMLPart(solrModules []string, additionalLibs []string) string {
	libs := make(map[string]bool, 0)

	// Add all module library locations
	if len(solrModules) > 0 {
		libs[DistLibs] = true
	}
	for _, module := range solrModules {
		libs[fmt.Sprintf(ContribLibs, module)] = true
	}

	// Add all custom library locations
	for _, libPath := range additionalLibs {
		libs[libPath] = true
	}

	libXml := ""
	if len(libs) > 0 {
		libList := make([]string, 0)
		for lib := range libs {
			libList = append(libList, lib)
		}
		sort.Strings(libList)
		libXml = fmt.Sprintf("<str name=\"sharedLib\">%s</str>", strings.Join(libList, ","))
	}
	return libXml
}

func getAppProtocol(solrCloud *solr.SolrCloud) *string {
	appProtocol := "http"
	if solrCloud.Spec.SolrTLS != nil {
		appProtocol = "https"
	}
	return &appProtocol
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
				{
					Name:        SolrClientPortName,
					Port:        int32(solrCloud.Spec.SolrAddressability.CommonServicePort),
					Protocol:    corev1.ProtocolTCP,
					TargetPort:  intstr.FromString(SolrClientPortName),
					AppProtocol: getAppProtocol(solrCloud),
				},
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
				{
					Name:        SolrClientPortName,
					Port:        int32(solrCloud.NodePort()),
					Protocol:    corev1.ProtocolTCP,
					TargetPort:  intstr.FromString(SolrClientPortName),
					AppProtocol: getAppProtocol(solrCloud),
				},
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
				{
					Name:        SolrClientPortName,
					Port:        int32(solrCloud.NodePort()),
					Protocol:    corev1.ProtocolTCP,
					TargetPort:  intstr.FromString(SolrClientPortName),
					AppProtocol: getAppProtocol(solrCloud),
				},
			},
			PublishNotReadyAddresses: true,
		},
	}
	return service
}

// GenerateIngress returns a new Ingress pointer generated for the entire SolrCloud, pointing to all instances
// solrCloud: SolrCloud instance
// nodeStatuses: []SolrNodeStatus the nodeStatuses
func GenerateIngress(solrCloud *solr.SolrCloud, nodeNames []string) (ingress *netv1.Ingress) {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	var annotations map[string]string

	customOptions := solrCloud.Spec.CustomSolrKubeOptions.IngressOptions
	if nil != customOptions {
		labels = MergeLabelsOrAnnotations(labels, customOptions.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, customOptions.Annotations)
	}

	extOpts := solrCloud.Spec.SolrAddressability.External

	// Create advertised domain name and possible additional domain names'
	allDomains := append([]string{extOpts.DomainName}, extOpts.AdditionalDomainNames...)
	rules, allHosts := CreateSolrIngressRules(solrCloud, nodeNames, allDomains)

	var ingressTLS []netv1.IngressTLS
	if solrCloud.Spec.SolrTLS != nil && solrCloud.Spec.SolrTLS.PKCS12Secret != nil {
		ingressTLS = append(ingressTLS, netv1.IngressTLS{SecretName: solrCloud.Spec.SolrTLS.PKCS12Secret.Name})
	} // else if using mountedTLSDir, it's likely they'll have an auto-wired TLS solution for Ingress as well via annotations

	if extOpts.HasIngressTLSTermination() {
		newIngressTLS := netv1.IngressTLS{
			Hosts: allHosts,
		}
		if extOpts.IngressTLSTermination.TLSSecret != "" {
			newIngressTLS.SecretName = extOpts.IngressTLSTermination.TLSSecret
		}
		ingressTLS = append(ingressTLS, newIngressTLS)
	}
	solrNodesRequireTLS := solrCloud.Spec.SolrTLS != nil
	ingressFrontedByTLS := len(ingressTLS) > 0

	// TLS Passthrough annotations
	if solrNodesRequireTLS {
		if annotations == nil {
			annotations = make(map[string]string, 1)
		}
		_, ok := annotations["nginx.ingress.kubernetes.io/backend-protocol"]
		if !ok {
			annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"
		}
	} else {
		if annotations == nil {
			annotations = make(map[string]string, 1)
		}
		_, ok := annotations["nginx.ingress.kubernetes.io/backend-protocol"]
		if !ok {
			annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTP"
		}
	}

	// TLS Accept annotations
	if ingressFrontedByTLS {
		_, ok := annotations["nginx.ingress.kubernetes.io/ssl-redirect"]
		if !ok {
			annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
		}
	}

	ingress = &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.CommonIngressName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: netv1.IngressSpec{
			Rules: rules,
			TLS:   ingressTLS,
		},
	}

	if nil != customOptions && customOptions.IngressClassName != nil {
		ingress.Spec.IngressClassName = customOptions.IngressClassName
	}

	return ingress
}

// CreateSolrIngressRules returns all applicable ingress rules for a cloud.
// solrCloud: SolrCloud instance
// nodeNames: the names for each of the solr pods
// domainName: string Domain for the ingress rule to use
func CreateSolrIngressRules(solrCloud *solr.SolrCloud, nodeNames []string, domainNames []string) (ingressRules []netv1.IngressRule, allHosts []string) {
	if !solrCloud.Spec.SolrAddressability.External.HideCommon {
		for _, domainName := range domainNames {
			rule := CreateCommonIngressRule(solrCloud, domainName)
			ingressRules = append(ingressRules, rule)
			allHosts = append(allHosts, rule.Host)
		}
	}
	if !solrCloud.Spec.SolrAddressability.External.HideNodes {
		for _, nodeName := range nodeNames {
			for _, domainName := range domainNames {
				rule := CreateNodeIngressRule(solrCloud, nodeName, domainName)
				ingressRules = append(ingressRules, rule)
				allHosts = append(allHosts, rule.Host)
			}
		}
	}
	return
}

// CreateCommonIngressRule returns a new Ingress Rule generated for a SolrCloud under the given domainName
// solrCloud: SolrCloud instance
// domainName: string Domain for the ingress rule to use
func CreateCommonIngressRule(solrCloud *solr.SolrCloud, domainName string) (ingressRule netv1.IngressRule) {
	pathType := netv1.PathTypeImplementationSpecific
	ingressRule = netv1.IngressRule{
		Host: solrCloud.ExternalCommonUrl(domainName, false),
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{
					{
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: solrCloud.CommonServiceName(),
								Port: netv1.ServiceBackendPort{
									Number: int32(solrCloud.Spec.SolrAddressability.CommonServicePort),
								},
							},
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
func CreateNodeIngressRule(solrCloud *solr.SolrCloud, nodeName string, domainName string) (ingressRule netv1.IngressRule) {
	pathType := netv1.PathTypeImplementationSpecific
	ingressRule = netv1.IngressRule{
		Host: solrCloud.ExternalNodeUrl(nodeName, domainName, false),
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{
					{
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: nodeName,
								Port: netv1.ServiceBackendPort{
									Number: int32(solrCloud.NodePort()),
								},
							},
						},
						PathType: &pathType,
					},
				},
			},
		},
	}
	return ingressRule
}

// TODO: Have this replace the postStart hook for creating the chroot
func generateZKInteractionInitContainer(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus, security *SecurityConfig) (bool, corev1.Container) {
	allSolrOpts := make([]string, 0)

	// Add all necessary ZK Info
	envVars, zkSolrOpt, hasChroot := createZkConnectionEnvVars(solrCloud, solrCloudStatus)
	if zkSolrOpt != "" {
		allSolrOpts = append(allSolrOpts, zkSolrOpt)
	}

	if solrCloud.Spec.SolrOpts != "" {
		allSolrOpts = append(allSolrOpts, solrCloud.Spec.SolrOpts)
	}

	// Add SOLR_OPTS last, so that it can use values from all of the other ENV_VARS
	if len(allSolrOpts) > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SOLR_OPTS",
			Value: strings.Join(allSolrOpts, " "),
		})
	}

	cmd := ""

	if hasChroot {
		cmd += "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}; "
	}

	if solrCloud.Spec.SolrTLS != nil {
		cmd += setUrlSchemeClusterPropCmd()
	}

	if security != nil && security.SecurityJson != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "SECURITY_JSON", ValueFrom: security.SecurityJsonSrc})
		if solrCloud.Spec.SolrZkOpts != "" {
			envVars = append(envVars, corev1.EnvVar{Name: "ZKCLI_JVM_FLAGS", Value: solrCloud.Spec.SolrZkOpts})
		}
		cmd += cmdToPutSecurityJsonInZk()
	}

	zkSetupResources := corev1.ResourceList{
		corev1.ResourceCPU:    *DefaultSolrZKPrepInitContainerCPU,
		corev1.ResourceMemory: *DefaultSolrZKPrepInitContainerMemory,
	}
	if cmd != "" {
		return true, corev1.Container{
			Name:                     "setup-zk",
			Image:                    solrCloud.Spec.SolrImage.ToImageName(),
			ImagePullPolicy:          solrCloud.Spec.SolrImage.PullPolicy,
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: "File",
			Command:                  []string{"sh", "-c", cmd},
			Env:                      envVars,
			Resources: corev1.ResourceRequirements{
				Requests: zkSetupResources,
				Limits:   zkSetupResources,
			},
		}
	}

	return false, corev1.Container{}
}

func createZkConnectionEnvVars(solrCloud *solr.SolrCloud, solrCloudStatus *solr.SolrCloudStatus) (envVars []corev1.EnvVar, solrOpt string, hasChroot bool) {
	zkConnectionStr, zkServer, zkChroot := solrCloudStatus.DissectZkInfo()
	envVars = []corev1.EnvVar{
		{
			Name:  "ZK_HOST",
			Value: zkConnectionStr,
		},
		{
			Name:  "ZK_CHROOT",
			Value: zkChroot,
		},
		{
			Name:  "ZK_SERVER",
			Value: zkServer,
		},
	}

	solrOpts := make([]string, 0)

	// Add ACL information, if given, through Env Vars
	allACL, readOnlyACL := solrCloud.Spec.ZookeeperRef.GetACLs()
	if hasACLs, aclEnvs := AddACLsToEnv(allACL, readOnlyACL); hasACLs {
		envVars = append(envVars, aclEnvs...)

		// The $SOLR_ZK_CREDS_AND_ACLS parameter does not get picked up when running solr, it must be added to the SOLR_OPTS.
		solrOpts = append(solrOpts, "$(SOLR_ZK_CREDS_AND_ACLS)")
	}

	// Add ZK Connection System Properties to Solr
	if solrCloud.Spec.SolrZkOpts != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SOLR_ZK_OPTS",
			Value: solrCloud.Spec.SolrZkOpts,
		})

		// The $SOLR_ZK_OPTS parameter does not get picked up when running solr, it must be added to the SOLR_OPTS.
		solrOpts = append(solrOpts, "$(SOLR_ZK_OPTS)")
	}

	return envVars, strings.Join(solrOpts, " "), len(zkChroot) > 1
}

func setupVolumeMountForUserProvidedConfigMapEntry(reconcileConfigInfo map[string]string, fileKey string, solrVolumes []corev1.Volume, envVar string) (*corev1.VolumeMount, *corev1.EnvVar, *corev1.Volume) {
	volName := strings.ReplaceAll(fileKey, ".", "-")
	mountPath := fmt.Sprintf("/var/solr/%s", reconcileConfigInfo[fileKey])
	appendedToExisting := false
	if reconcileConfigInfo[fileKey] == reconcileConfigInfo[SolrXmlFile] {
		// the user provided a custom log4j2.xml and solr.xml, append to the volume for solr.xml created above
		for _, vol := range solrVolumes {
			if vol.Name == "solr-xml" {
				vol.ConfigMap.Items = append(vol.ConfigMap.Items, corev1.KeyToPath{Key: fileKey, Path: fileKey})
				appendedToExisting = true
				volName = vol.Name
				break
			}
		}
	}

	var vol *corev1.Volume = nil
	if !appendedToExisting {
		vol = &corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: reconcileConfigInfo[fileKey]},
					Items:                []corev1.KeyToPath{{Key: fileKey, Path: fileKey}},
					DefaultMode:          &PublicReadOnlyPermissions,
				},
			},
		}
	}
	pathToFile := fmt.Sprintf("%s/%s", mountPath, fileKey)

	return &corev1.VolumeMount{Name: volName, MountPath: mountPath}, &corev1.EnvVar{Name: envVar, Value: pathToFile}, vol
}

func GetCustomProbePaths(solrCloud *solr.SolrCloud) []string {
	probePaths := []string{}

	podOptions := solrCloud.Spec.CustomSolrKubeOptions.PodOptions
	if podOptions == nil {
		return probePaths
	}

	// include any custom paths
	if podOptions.ReadinessProbe != nil && podOptions.ReadinessProbe.HTTPGet != nil {
		probePaths = append(probePaths, podOptions.ReadinessProbe.HTTPGet.Path)
	}

	if podOptions.LivenessProbe != nil && podOptions.LivenessProbe.HTTPGet != nil {
		probePaths = append(probePaths, podOptions.LivenessProbe.HTTPGet.Path)
	}

	if podOptions.StartupProbe != nil && podOptions.StartupProbe.HTTPGet != nil {
		probePaths = append(probePaths, podOptions.StartupProbe.HTTPGet.Path)
	}

	return probePaths
}
