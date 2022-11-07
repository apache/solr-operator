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

package v1beta1

import (
	"fmt"
	"github.com/go-logr/logr"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultPullPolicy = "" // This will use the default pullPolicy of Always when the tag is "latest" and IfNotPresent for all other tags.

	DefaultSolrReplicas = int32(3)
	DefaultSolrRepo     = "library/solr"
	DefaultSolrVersion  = "8.11"
	DefaultSolrJavaMem  = "-Xms1g -Xmx2g"
	DefaultSolrOpts     = ""
	DefaultSolrLogLevel = "INFO"
	DefaultSolrGCTune   = ""

	DefaultBusyBoxImageRepo    = "library/busybox"
	DefaultBusyBoxImageVersion = "1.28.0-glibc"

	DefaultZkReplicas                                = int32(3)
	DefaultZkStorage                                 = "5Gi"
	DefaultZkRepo                                    = "pravega/zookeeper"
	DefaultZkVersion                                 = ""
	DefaultZkVolumeReclaimPolicy VolumeReclaimPolicy = "Retain"

	SolrTechnologyLabel      = "solr-cloud"
	ZookeeperTechnologyLabel = "zookeeper"

	DefaultBasicAuthUsername = "k8s-oper"

	LegacyBackupRepositoryName = "legacy_volume_repository"
)

// SolrCloudSpec defines the desired state of SolrCloud
type SolrCloudSpec struct {
	// The number of solr nodes to run
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// The information for the Zookeeper this SolrCloud should connect to
	// Can be a zookeeper that is running, or one that is created by the solr operator
	// +optional
	ZookeeperRef *ZookeeperRef `json:"zookeeperRef,omitempty"`

	// +optional
	SolrImage *ContainerImage `json:"solrImage,omitempty"`

	// Customize how the cloud data is stored.
	// If neither "persistent" or "ephemeral" is provided, then ephemeral storage will be used by default.
	//
	// +optional
	StorageOptions SolrDataStorageOptions `json:"dataStorage,omitempty"`

	// Provide custom options for kubernetes objects created for the Solr Cloud.
	// +optional
	CustomSolrKubeOptions CustomSolrKubeOptions `json:"customSolrKubeOptions,omitempty"`

	// Customize how Solr is addressed both internally and externally in Kubernetes.
	// +optional
	SolrAddressability SolrAddressabilityOptions `json:"solrAddressability,omitempty"`

	// Define how Solr rolling updates are executed.
	// +optional
	UpdateStrategy SolrUpdateStrategy `json:"updateStrategy,omitempty"`

	// +optional
	BusyBoxImage *ContainerImage `json:"busyBoxImage,omitempty"`

	// +optional
	SolrJavaMem string `json:"solrJavaMem,omitempty"`

	// You can add common system properties to the SOLR_OPTS environment variable
	// SolrOpts is the string interface for these optional settings
	// +optional
	SolrOpts string `json:"solrOpts,omitempty"`

	// This will add java system properties for connecting to Zookeeper.
	// SolrZkOpts is the string interface for these optional settings
	// +optional
	SolrZkOpts string `json:"solrZkOpts,omitempty"`

	// Set the Solr Log level, defaults to INFO
	// +optional
	SolrLogLevel string `json:"solrLogLevel,omitempty"`

	// Set GC Tuning configuration through GC_TUNE environment variable
	// +optional
	SolrGCTune string `json:"solrGCTune,omitempty"`

	// Options to enable the server TLS certificate for Solr pods
	// +optional
	SolrTLS *SolrTLSOptions `json:"solrTLS,omitempty"`

	// Options to configure client TLS certificate for Solr pods
	// +optional
	SolrClientTLS *SolrTLSOptions `json:"solrClientTLS,omitempty"`

	// Options to enable Solr security
	// +optional
	SolrSecurity *SolrSecurityOptions `json:"solrSecurity,omitempty"`

	// Allows specification of multiple different "repositories" for Solr to use when backing up data.
	//+optional
	//+listType:=map
	//+listMapKey:=name
	BackupRepositories []SolrBackupRepository `json:"backupRepositories,omitempty"`

	// List of Solr Modules to be loaded when starting Solr
	// Note: You do not need to specify a module if it is required by another property (e.g. backupRepositories[].gcs)
	//
	//+optional
	SolrModules []string `json:"solrModules,omitempty"`

	// List of paths in the Solr Docker image to load in the classpath.
	// Note: Solr Modules will be auto-loaded if specified in the "solrModules" property. There is no need to specify them here as well.
	//
	//+optional
	AdditionalLibs []string `json:"additionalLibs,omitempty"`
}

func (spec *SolrCloudSpec) withDefaults(logger logr.Logger) (changed bool) {
	if spec.Replicas == nil {
		changed = true
		r := DefaultSolrReplicas
		spec.Replicas = &r
	}

	if spec.SolrJavaMem == "" && DefaultSolrJavaMem != "" {
		changed = true
		spec.SolrJavaMem = DefaultSolrJavaMem
	}

	if spec.SolrOpts == "" && DefaultSolrOpts != "" {
		changed = true
		spec.SolrOpts = DefaultSolrOpts
	}

	if spec.SolrLogLevel == "" && DefaultSolrLogLevel != "" {
		changed = true
		spec.SolrLogLevel = DefaultSolrLogLevel
	}

	if spec.SolrGCTune == "" && DefaultSolrGCTune != "" {
		changed = true
		spec.SolrGCTune = DefaultSolrGCTune
	}

	changed = spec.SolrAddressability.withDefaults(spec.SolrTLS != nil, logger) || changed

	changed = spec.UpdateStrategy.withDefaults() || changed

	if spec.ZookeeperRef == nil {
		spec.ZookeeperRef = &ZookeeperRef{}
	}
	changed = spec.ZookeeperRef.withDefaults() || changed

	if spec.SolrImage == nil {
		spec.SolrImage = &ContainerImage{}
	}
	changed = spec.SolrImage.withDefaults(DefaultSolrRepo, DefaultSolrVersion, DefaultPullPolicy) || changed

	changed = spec.StorageOptions.withDefaults() || changed

	if spec.BusyBoxImage == nil {
		c := ContainerImage{}
		spec.BusyBoxImage = &c
	}
	changed = spec.BusyBoxImage.withDefaults(DefaultBusyBoxImageRepo, DefaultBusyBoxImageVersion, DefaultPullPolicy) || changed

	return changed
}

type CustomSolrKubeOptions struct {
	// SolrPodOptions defines the custom options for solrCloud pods.
	// +optional
	PodOptions *PodOptions `json:"podOptions,omitempty"`

	// StatefulSetOptions defines the custom options for the solrCloud StatefulSet.
	// +optional
	StatefulSetOptions *StatefulSetOptions `json:"statefulSetOptions,omitempty"`

	// CommonServiceOptions defines the custom options for the common solrCloud Service.
	// +optional
	CommonServiceOptions *ServiceOptions `json:"commonServiceOptions,omitempty"`

	// HeadlessServiceOptions defines the custom options for the headless solrCloud Service.
	// +optional
	HeadlessServiceOptions *ServiceOptions `json:"headlessServiceOptions,omitempty"`

	// NodeServiceOptions defines the custom options for the individual solrCloud Node services, if they are created.
	// These services will only be created when exposing SolrNodes externally via an Ingress in the AddressabilityOptions.
	// +optional
	NodeServiceOptions *ServiceOptions `json:"nodeServiceOptions,omitempty"`

	// ServiceOptions defines the custom options for the solrCloud ConfigMap.
	// +optional
	ConfigMapOptions *ConfigMapOptions `json:"configMapOptions,omitempty"`

	// IngressOptions defines the custom options for the solrCloud Ingress.
	// +optional
	IngressOptions *IngressOptions `json:"ingressOptions,omitempty"`
}

type SolrDataStorageOptions struct {

	// PersistentStorage is the specification for how the persistent Solr data storage should be configured.
	//
	// This option cannot be used with the "ephemeral" option.
	//
	// +optional
	PersistentStorage *SolrPersistentDataStorageOptions `json:"persistent,omitempty"`

	// EphemeralStorage is the specification for how the ephemeral Solr data storage should be configured.
	//
	// This option cannot be used with the "persistent" option.
	// Ephemeral storage is used by default if neither "persistent" or "ephemeral" is provided.
	//
	// +optional
	EphemeralStorage *SolrEphemeralDataStorageOptions `json:"ephemeral,omitempty"`
}

func (opts *SolrDataStorageOptions) withDefaults() (changed bool) {
	if opts.PersistentStorage != nil {
		changed = changed || opts.PersistentStorage.withDefaults()
	}

	return changed
}

type SolrPersistentDataStorageOptions struct {

	// VolumeReclaimPolicy determines how the Solr Cloud's PVCs will be treated after the cloud is deleted.
	//   - Retain: This is the default Kubernetes policy, where PVCs created for StatefulSets are not deleted when the StatefulSet is deleted.
	//   - Delete: The PVCs will be deleted by the Solr Operator after the SolrCloud object is deleted.
	// The default value is Retain, so no data will be deleted unless explicitly configured.
	// +optional
	VolumeReclaimPolicy VolumeReclaimPolicy `json:"reclaimPolicy,omitempty"`

	// PersistentVolumeClaimTemplate is the PVC object for the solr node to store its data.
	// Within metadata, the Name, Labels and Annotations are able to be specified, but defaults will be provided if necessary.
	// The entire Spec is customizable, however there will be defaults provided if necessary.
	// This field is optional. If no PVC spec is provided, then a default will be provided.
	// +optional
	PersistentVolumeClaimTemplate PersistentVolumeClaimTemplate `json:"pvcTemplate,omitempty"`
}

func (opts *SolrPersistentDataStorageOptions) withDefaults() (changed bool) {
	if opts.VolumeReclaimPolicy == "" {
		changed = true
		opts.VolumeReclaimPolicy = VolumeReclaimPolicyRetain
	}

	return changed
}

// VolumeReclaimPolicy is a string enumeration type that enumerates
// all possible ways that a SolrCloud can treat it's PVCs after its death
// +kubebuilder:validation:Enum=Retain;Delete
type VolumeReclaimPolicy string

const (
	// All pod PVCs are retained after the SolrCloud is deleted.
	VolumeReclaimPolicyRetain VolumeReclaimPolicy = "Retain"

	// All pod PVCs are deleted after the SolrCloud is deleted.
	VolumeReclaimPolicyDelete VolumeReclaimPolicy = "Delete"
)

// PersistentVolumeClaimTemplate is used to produce
// PersistentVolumeClaim objects as part of an EphemeralVolumeSource.
type PersistentVolumeClaimTemplate struct {
	// May contain labels and annotations that will be copied into the PVC
	// when creating it. No other fields are allowed and will be rejected during
	// validation.
	//
	// +optional
	ObjectMeta TemplateMeta `json:"metadata,omitempty"`

	// The specification for the PersistentVolumeClaim. The entire content is
	// copied unchanged into the PVC that gets created from this
	// template. The same fields as in a PersistentVolumeClaim
	// are also valid here.
	//
	// +optional
	Spec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

// TemplateMeta is metadata for templated resources.
type TemplateMeta struct {
	// Name must be unique within a namespace. Is required when creating resources, although
	// some resources may allow a client to request the generation of an appropriate name
	// automatically. Name is primarily intended for creation idempotence and configuration
	// definition.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	// +optional
	Name string `json:"name,omitempty"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,11,rep,name=labels"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}

type SolrEphemeralDataStorageOptions struct {
	// HostPathVolumeSource is an optional config to specify a path on the host machine to store Solr data.
	//
	// If hostPath is omitted, then the default EmptyDir is used, otherwise hostPath takes precedence over EmptyDir.
	// +optional
	HostPath *corev1.HostPathVolumeSource `json:"hostPath,omitempty"`
	//EmptyDirVolumeSource is an optional config for the emptydir volume that will store Solr data.
	// +optional
	EmptyDir *corev1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`
}

// +kubebuilder:validation:MinProperties:=2
// +kubebuilder:validation:MaxProperties:=2
type SolrBackupRepository struct {
	// A name used to identify this local storage profile.  Values should follow RFC-1123.  (See here for more details:
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names)
	//
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=100
	Name string `json:"name"`

	// A GCSRepository for Solr to use when backing up and restoring collections.
	//+optional
	GCS *GcsRepository `json:"gcs,omitempty"`

	// An S3Repository for Solr to use when backing up and restoring collections.
	//+optional
	S3 *S3Repository `json:"s3,omitempty"`

	// Allows specification of a "repository" for Solr to use when backing up data "locally".
	//+optional
	Volume *VolumeRepository `json:"volume,omitempty"`
}

type GcsRepository struct {
	// The name of the GCS bucket that all backup data will be stored in
	Bucket string `json:"bucket"`

	// The name & key of a Kubernetes secret holding a Google cloud service account key.  Must be set unless deployed in
	// GKE and making use of Google's "Workplace Identity" feature.
	//+optional
	GcsCredentialSecret *corev1.SecretKeySelector `json:"gcsCredentialSecret,omitempty"`

	// An already-created chroot within the bucket to store data in. Defaults to the root path "/" if not specified.
	// +optional
	BaseLocation string `json:"baseLocation,omitempty"`
}

type S3Repository struct {
	// The S3 region to store the backup data in
	Region string `json:"region"`

	// The name of the S3 bucket that all backup data will be stored in
	Bucket string `json:"bucket"`

	// Options for specifying S3Credentials. This is optional in case you want to mount this information yourself.
	// However, if you do not include these credentials, and you do not load them yourself via a mount or EnvVars,
	// you will likely see errors when taking s3 backups.
	//
	// If running in EKS, you can create an IAMServiceAccount that uses a role permissioned for this S3 bucket.
	// Then use that serviceAccountName for your SolrCloud, and the credentials should be auto-populated.
	//
	// +optional
	Credentials *S3Credentials `json:"credentials,omitempty"`

	// An already-created chroot within the bucket to store data in. Defaults to the root path "/" if not specified.
	// +optional
	BaseLocation string `json:"baseLocation,omitempty"`

	// The full endpoint URL to use when connecting with S3 (or a supported S3 compatible interface)
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// The full proxy URL to use when connecting with S3
	// +optional
	ProxyUrl string `json:"proxyUrl,omitempty"`
}

type S3Credentials struct {
	// The name & key of a Kubernetes secret holding an AWS Access Key ID
	// +optional
	AccessKeyIdSecret *corev1.SecretKeySelector `json:"accessKeyIdSecret,omitempty"`

	// The name & key of a Kubernetes secret holding an AWS Secret Access Key
	// +optional
	SecretAccessKeySecret *corev1.SecretKeySelector `json:"secretAccessKeySecret,omitempty"`

	// The name & key of a Kubernetes secret holding an AWS Session Token
	// +optional
	SessionTokenSecret *corev1.SecretKeySelector `json:"sessionTokenSecret,omitempty"`

	// The name & key of a Kubernetes secret holding an AWS credentials file
	// +optional
	CredentialsFileSecret *corev1.SecretKeySelector `json:"credentialsFileSecret,omitempty"`
}

type VolumeRepository struct {
	// This is a volumeSource for a volume that will be mounted to all solrNodes to store backups and load restores.
	// The data within the volume will be namespaced for this instance, so feel free to use the same volume for multiple clouds.
	// Since the volume will be mounted to all solrNodes, it must be able to be written from multiple pods.
	// If a PVC reference is given, the PVC must have `accessModes: - ReadWriteMany`.
	// Other options are to use a NFS volume.
	Source corev1.VolumeSource `json:"source"`

	// Select a custom directory name to mount the backup/restore data in the given volume.
	// If not specified, then the name of the solrcloud will be used by default.
	// +optional
	Directory string `json:"directory,omitempty"`
}

type SolrAddressabilityOptions struct {
	// External defines the way in which this SolrCloud nodes should be made addressable externally, from outside the Kubernetes cluster.
	// If none is provided, the Solr Cloud will not be made addressable externally.
	// +optional
	External *ExternalAddressability `json:"external,omitempty"`

	// PodPort defines the port to have the Solr Pod listen on.
	// Defaults to 8983
	// +optional
	PodPort int `json:"podPort,omitempty"`

	// CommonServicePort defines the port to have the common Solr service listen on.
	// Defaults to 80 (when not using TLS) or 443 (when using TLS)
	// +optional
	CommonServicePort int `json:"commonServicePort,omitempty"`

	// KubeDomain allows for the specification of an override of the default "cluster.local" Kubernetes cluster domain.
	// Only use this option if the Kubernetes cluster has been setup with a custom domain.
	// +optional
	KubeDomain string `json:"kubeDomain,omitempty"`
}

func (opts *SolrAddressabilityOptions) withDefaults(usesTLS bool, logger logr.Logger) (changed bool) {
	if opts.External != nil {
		changed = opts.External.withDefaults(usesTLS, logger)
	}
	if opts.PodPort == 0 {
		changed = true
		opts.PodPort = 8983
	}
	if opts.CommonServicePort == 0 {
		changed = true
		if usesTLS {
			opts.CommonServicePort = 443
		} else {
			opts.CommonServicePort = 80
		}
	}
	return changed
}

// ExternalAddressability defines the config for making Solr services available externally to kubernetes.
// Be careful when using LoadBalanced and includeNodes, as many IP addresses could be created if you are running many large solrClouds.
type ExternalAddressability struct {
	// The way in which this SolrCloud's service(s) should be made addressable externally.
	Method ExternalAddressabilityMethod `json:"method"`

	// Use the external address to advertise the SolrNode, defaults to false.
	//
	// If false, the external address will be available, however Solr (and clients using the CloudSolrClient in SolrJ) will only be aware of the internal URLs.
	// If true, Solr will startup with the hostname of the external address.
	//
	// NOTE: This option cannot be true when hideNodes is set to true. So it will be auto-set to false if that is the case.
	//
	// +optional
	UseExternalAddress bool `json:"useExternalAddress"`

	// Do not expose the common Solr service externally. This affects a single service.
	// Defaults to false.
	// +optional
	HideCommon bool `json:"hideCommon,omitempty"`

	// Do not expose each of the Solr Node services externally.
	// The number of services this affects could range from 1 (a headless service for ExternalDNS) to the number of Solr pods your cloud contains (individual node services for Ingress/LoadBalancer).
	// Defaults to false.
	// +optional
	HideNodes bool `json:"hideNodes,omitempty"`

	// Override the domainName provided as startup parameters to the operator, used by ingresses and externalDNS.
	// The common and/or node services will be addressable by unique names under the given domain.
	// e.g. given.domain.name.com -> default-example-solrcloud.given.domain.name.com
	//
	// For the LoadBalancer method, this field is optional and will only be used when useExternalAddress=true.
	// If used with the LoadBalancer method, you will need DNS routing to the LoadBalancer IP address through the url template given above.
	DomainName string `json:"domainName"`

	// Provide additional domainNames that the Ingress or ExternalDNS should listen on.
	// This option is ignored with the LoadBalancer method.
	// +optional
	AdditionalDomainNames []string `json:"additionalDomainNames,omitempty"`

	// Provide additional domainNames that the Ingress or ExternalDNS should listen on.
	// This option is ignored with the LoadBalancer method.
	//
	// DEPRECATED: Please use additionalDomainNames instead. This will be removed in a future version.
	//
	// +optional
	AdditionalDomains []string `json:"additionalDomains,omitempty"`

	// NodePortOverride defines the port to have all Solr node service(s) listen on and advertise itself as if advertising through an Ingress or LoadBalancer.
	// This overrides the default usage of the podPort.
	//
	// This is option is only used when HideNodes=false, otherwise the the port each Solr Node will advertise itself with the podPort.
	// This option is also unavailable with the ExternalDNS method.
	//
	// If using method=Ingress, your ingress controller is required to listen on this port.
	// If your ingress controller is not listening on the podPort, then this option is required for solr to be addressable via an Ingress.
	//
	// Defaults to 80 (without TLS) or 443 (with TLS) if HideNodes=false and method=Ingress, otherwise this is optional.
	// +optional
	NodePortOverride int `json:"nodePortOverride,omitempty"`

	// IngressTLSTerminationSecret defines a TLS Secret to use for TLS termination of all exposed addresses in the ingress.
	//
	// This is option is only available when Method=Ingress, because ExternalDNS and LoadBalancer Services do not support TLS termination.
	// This option is also unavailable when the SolrCloud has TLS enabled via `spec.solrTLS`, in this case the Ingress cannot terminate TLS before reaching Solr.
	//
	// When using this option, the UseExternalAddress option will be disabled, since Solr cannot be running in HTTP mode and making internal requests in HTTPS.
	//
	// DEPRECATED: Use ingressTLSTermination.tlsSecret instead
	//
	// +optional
	IngressTLSTerminationSecret string `json:"ingressTLSTerminationSecret,omitempty"`

	// IngressTLSTermination tells the SolrCloud Ingress to terminate TLS on incoming connections.
	//
	// This is option is only available when Method=Ingress, because ExternalDNS and LoadBalancer Services do not support TLS termination.
	// This option is also unavailable when the SolrCloud has TLS enabled via `spec.solrTLS`, in this case the Ingress cannot terminate TLS before reaching Solr.
	//
	// When using this option, the UseExternalAddress option will be disabled, since Solr cannot be running in HTTP mode and making internal requests in HTTPS.
	//
	// +optional
	IngressTLSTermination *SolrIngressTLSTermination `json:"ingressTLSTermination,omitempty"`
}

// ExternalAddressabilityMethod is a string enumeration type that enumerates
// all possible ways that a SolrCloud can be made addressable external to the kubernetes cluster.
// +kubebuilder:validation:Enum=Ingress;ExternalDNS
type ExternalAddressabilityMethod string

const (
	// Use an ingress to make the Solr service(s) externally addressable
	Ingress ExternalAddressabilityMethod = "Ingress"

	// Use ExternalDNS to make the Solr service(s) externally addressable
	ExternalDNS ExternalAddressabilityMethod = "ExternalDNS"

	// Make Solr service(s) type:LoadBalancer to make them externally addressable
	// NOTE: This option is not currently supported.
	LoadBalancer ExternalAddressabilityMethod = "LoadBalancer"
)

func (opts *ExternalAddressability) withDefaults(usesTLS bool, logger logr.Logger) (changed bool) {
	// TODO: Remove in v0.7.0
	// If the deprecated IngressTLSTerminationSecret exists, use it to default the new location of the value.
	// If that location already exists, then merely remove the deprecated option.
	if opts.IngressTLSTerminationSecret != "" {
		terminationSecretLogger := logger.WithValues("option", "spec.solrAddressability.external.ingressTLSTerminationSecret").WithValues("newLocation", "spec.solrAddressability.external.ingressTLSTermination.tlsSecret")
		var loggingAction string
		if !opts.HasIngressTLSTermination() {
			opts.IngressTLSTermination = &SolrIngressTLSTermination{
				TLSSecret: opts.IngressTLSTerminationSecret,
			}
			loggingAction = "Moving"
		} else {
			terminationSecretLogger = terminationSecretLogger.WithValues("reason", "Cannot move deprecated option because ingressTLSTermination is already defined")
			loggingAction = "Removing"
		}
		opts.IngressTLSTerminationSecret = ""
		terminationSecretLogger.Info(loggingAction + " deprecated CRD option")
		changed = true
	}

	// You can't use an externalAddress for Solr Nodes if the Nodes are hidden externally
	if opts.UseExternalAddress && (opts.HideNodes || opts.IngressTLSTermination != nil) {
		changed = true
		opts.UseExternalAddress = false
	}

	// Add the values from the deprecated "additionalDomains" to the new "additionalDomainNames" field
	// But make sure you aren't creating duplicates
	// TODO: Remove in v0.7.0
	if opts.AdditionalDomains != nil {
		// Only modify AdditionalDomainNames if AdditionalDomains is empty
		// But if it is non-nil and empty, still set AdditionalDomains to nil
		if len(opts.AdditionalDomains) > 0 {
			if len(opts.AdditionalDomainNames) == 0 {
				opts.AdditionalDomainNames = opts.AdditionalDomains
			} else {
				for _, domain := range opts.AdditionalDomains {
					hasDomain := false
					for _, containsDomain := range opts.AdditionalDomainNames {
						if domain == containsDomain {
							hasDomain = true
							break
						}
					}
					if !hasDomain {
						opts.AdditionalDomainNames = append(opts.AdditionalDomainNames, domain)
					}
				}
			}
		}
		logger.Info("Moving deprecated CRD option", "option", "spec.solrAddressability.external.additionalDomains", "newLocation", "spec.solrAddressability.external.additionalDomainNames")
		changed = true
		opts.AdditionalDomains = nil
	}

	// If the Ingress method is used, default the nodePortOverride to 80 or 443, since that is the port that most ingress controllers listen on.
	if !opts.HideNodes && opts.Method == Ingress && opts.NodePortOverride == 0 {
		changed = true
		if usesTLS {
			opts.NodePortOverride = 443
		} else {
			opts.NodePortOverride = 80
		}
	}
	// If a headless service is used, aka not using individual node services, then a nodePortOverride is not allowed.
	if !opts.UsesIndividualNodeServices() && opts.NodePortOverride > 0 {
		changed = true
		opts.NodePortOverride = 0
	}

	return changed
}

// SolrIngressTLSTermination defines how a SolrCloud should have TLS Termination enabled.
// Only one option can be provided.
//
// +kubebuilder:validation:MaxProperties=1
type SolrIngressTLSTermination struct {

	// UseDefaultTLSSecret determines whether the ingress should use the default TLS secret provided by the Ingress implementation.
	//
	// For example, using nginx: https://kubernetes.github.io/ingress-nginx/user-guide/tls/#default-ssl-certificate
	//
	// +optional
	UseDefaultTLSSecret bool `json:"useDefaultTLSSecret,omitempty"`

	// TLSSecret defines a TLS Secret to use for TLS termination of all exposed addresses for this SolrCloud in the Ingress.
	//
	// +optional
	TLSSecret string `json:"tlsSecret,omitempty"`
}

type SolrUpdateStrategy struct {
	// Method defines the way in which SolrClouds should be updated when the podSpec changes.
	// +optional
	Method SolrUpdateMethod `json:"method,omitempty"`

	// Options for Solr Operator Managed rolling updates.
	// +optional
	ManagedUpdateOptions ManagedUpdateOptions `json:"managed,omitempty"`

	// Perform a scheduled restart on the given schedule, in CRON format.
	//
	// Multiple CRON syntaxes are supported
	//   - Standard CRON (e.g. "CRON_TZ=Asia/Seoul 0 6 * * ?")
	//   - Predefined Schedules (e.g. "@yearly", "@weekly", etc.)
	//   - Intervals (e.g. "@every 10h30m")
	//
	// For more information please check this reference:
	// https://pkg.go.dev/github.com/robfig/cron/v3?utm_source=godoc#hdr-CRON_Expression_Format
	//
	// +optional
	RestartSchedule string `json:"restartSchedule,omitempty"`
}

// SolrUpdateMethod is a string enumeration type that enumerates
// all possible ways that a SolrCloud can having rolling updates managed.
// +kubebuilder:validation:Enum=Managed;StatefulSet;Manual
type SolrUpdateMethod string

const (
	// Let the Solr Operator manage rolling updates to keep collections/shards available while updating pods in parallel.
	// This is the default option.
	ManagedUpdate SolrUpdateMethod = "Managed"

	// Use the default StatefulSet rolling updates logic. One pod at a time, starting with the highest ordinal.
	StatefulSetUpdate SolrUpdateMethod = "StatefulSet"

	// The Solr Operator and Kubernetes will not delete pods for updates. The user will be responsible for this.
	ManualUpdate SolrUpdateMethod = "Manual"
)

func (opts *SolrUpdateStrategy) withDefaults() (changed bool) {
	// You can't use an externalAddress for Solr Nodes if the Nodes are hidden externally
	if opts.Method == "" {
		changed = true
		opts.Method = ManagedUpdate
	}

	return changed
}

// Spec to control the desired behavior of managed rolling update.
type ManagedUpdateOptions struct {

	// The maximum number of pods that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of the desired number of pods (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// If the provided number is 0 or negative, then all pods will be allowed to be updated in unison.
	//
	// Defaults to 25%.
	//
	// +optional
	MaxPodsUnavailable *intstr.IntOrString `json:"maxPodsUnavailable,omitempty"`

	// The maximum number of replicas for each shard that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of replicas in a shard (ex: 25%).
	// Absolute number is calculated from percentage by rounding down.
	// If the provided number is 0 or negative, then all replicas will be allowed to be updated in unison.
	//
	// Defaults to 1.
	//
	// +optional
	MaxShardReplicasUnavailable *intstr.IntOrString `json:"maxShardReplicasUnavailable,omitempty"`
}

// ZookeeperRef defines the zookeeper ensemble for solr to connect to
// If no ConnectionString is provided, the solr-cloud controller will create and manage an internal ensemble
type ZookeeperRef struct {
	// A zookeeper ensemble that is run independently of the solr operator
	// If an externalConnectionString is provided, but no internalConnectionString is, the external will be used as the internal
	// +optional
	ConnectionInfo *ZookeeperConnectionInfo `json:"connectionInfo,omitempty"`

	// Create a new Zookeeper Ensemble with the following spec
	// Note: This option will not allow the SolrCloud to run across kube-clusters.
	// Note: Requires
	//   - The zookeeperOperator flag to be provided to the Solr Operator
	//   - A zookeeper operator to be running
	// +optional
	ProvidedZookeeper *ZookeeperSpec `json:"provided,omitempty"`
}

func (ref *ZookeeperRef) withDefaults() (changed bool) {
	if ref.ProvidedZookeeper == nil && ref.ConnectionInfo == nil {
		changed = true
		ref.ProvidedZookeeper = &ZookeeperSpec{}
	} else if ref.ConnectionInfo != nil {
		if ref.ProvidedZookeeper != nil {
			ref.ProvidedZookeeper = nil
			changed = true
		}
		changed = ref.ConnectionInfo.withDefaults() || changed
	}
	if ref.ProvidedZookeeper != nil {
		changed = ref.ProvidedZookeeper.WithDefaults() || changed
	}
	return changed
}

func (ref *ZookeeperRef) GetACLs() (allACL *ZookeeperACL, readOnlyACL *ZookeeperACL) {
	if ref.ConnectionInfo != nil {
		allACL = ref.ConnectionInfo.AllACL
		readOnlyACL = ref.ConnectionInfo.ReadOnlyACL
	} else if ref.ProvidedZookeeper != nil {
		allACL = ref.ProvidedZookeeper.AllACL
		readOnlyACL = ref.ProvidedZookeeper.ReadOnlyACL
	}
	return
}

// ZookeeperSpec defines the internal zookeeper ensemble to run with the given spec
type ZookeeperSpec struct {

	// Number of members to create up for the ZK ensemble
	// Defaults to 3
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image of Zookeeper to run
	// +optional
	Image *ContainerImage `json:"image,omitempty"`

	// Persistence is the configuration for zookeeper persistent layer.
	// PersistentVolumeClaimSpec and VolumeReclaimPolicy can be specified in here.
	// At anypoint only one of Persistence or Ephemeral should be present in the manifest
	// +optional
	Persistence *ZKPersistence `json:"persistence,omitempty"`

	// Ephemeral is the configuration which helps create ephemeral storage
	// At anypoint only one of Persistence or Ephemeral should be present in the manifest
	// +optional
	Ephemeral *ZKEphemeral `json:"ephemeral,omitempty"`

	// Customization options for the Zookeeper Pod
	// +optional
	ZookeeperPod ZookeeperPodPolicy `json:"zookeeperPodPolicy,omitempty"`

	// The ChRoot to connect solr at
	// +optional
	ChRoot string `json:"chroot,omitempty"`

	// ZooKeeper ACL to use when connecting with ZK.
	// This ACL should have ALL permission in the given chRoot.
	// +optional
	AllACL *ZookeeperACL `json:"acl,omitempty"`

	// ZooKeeper ACL to use when connecting with ZK for reading operations.
	// This ACL should have READ permission in the given chRoot.
	// +optional
	ReadOnlyACL *ZookeeperACL `json:"readOnlyAcl,omitempty"`

	// Additional Zookeeper Configuration settings
	// +optional
	Config ZookeeperConfig `json:"config,omitempty"`
}

type ZKPersistence struct {
	// VolumeReclaimPolicy is a zookeeper operator configuration. If it's set to Delete,
	// the corresponding PVCs will be deleted by the operator when zookeeper cluster is deleted.
	// The default value is Retain.
	VolumeReclaimPolicy VolumeReclaimPolicy `json:"reclaimPolicy,omitempty"`
	// PersistentVolumeClaimSpec is the spec to describe PVC for the container
	// This field is optional. If no PVC is specified default persistentvolume
	// will get created.
	PersistentVolumeClaimSpec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
	// Annotations specifies the annotations to attach to pvc the operator
	// creates.
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ZKEphemeral struct {
	//EmptyDirVolumeSource is optional and this will create the emptydir volume
	//It has two parameters Medium and SizeLimit which are optional as well
	//Medium specifies What type of storage medium should back this directory.
	//SizeLimit specifies Total amount of local storage required for this EmptyDir volume.
	EmptyDirVolumeSource corev1.EmptyDirVolumeSource `json:"emptydirvolumesource,omitempty"`
}

// ZookeeperConfig is the current configuration of each Zookeeper node, which
// sets these values in the config-map
type ZookeeperConfig struct {
	// InitLimit is the amount of time, in ticks, to allow followers to connect
	// and sync to a leader.
	//
	// Default value is 10.
	InitLimit int `json:"initLimit,omitempty"`

	// TickTime is the length of a single tick, which is the basic time unit used
	// by Zookeeper, as measured in milliseconds
	//
	// The default value is 2000.
	TickTime int `json:"tickTime,omitempty"`

	// SyncLimit is the amount of time, in ticks, to allow followers to sync with
	// Zookeeper.
	//
	// The default value is 2.
	SyncLimit int `json:"syncLimit,omitempty"`

	// Clients can submit requests faster than ZooKeeper can process them, especially
	// if there are a lot of clients. Zookeeper will throttle Clients so that requests
	// won't exceed global outstanding limit.
	//
	// The default value is 1000
	GlobalOutstandingLimit int `json:"globalOutstandingLimit,omitempty"`

	// To avoid seeks ZooKeeper allocates space in the transaction log file in
	// blocks of preAllocSize kilobytes
	//
	// The default value is 64M
	PreAllocSize int `json:"preAllocSize,omitempty"`

	// ZooKeeper records its transactions using snapshots and a transaction log
	// The number of transactions recorded in the transaction log before a snapshot
	// can be taken is determined by snapCount
	//
	// The default value is 100,000
	SnapCount int `json:"snapCount,omitempty"`

	// Zookeeper maintains an in-memory list of last committed requests for fast
	// synchronization with followers
	//
	// The default value is 500
	CommitLogCount int `json:"commitLogCount,omitempty"`

	// Snapshot size limit in Kb
	//
	// The defult value is 4GB
	SnapSizeLimitInKb int `json:"snapSizeLimitInKb,omitempty"`

	// Limits the total number of concurrent connections that can be made to a
	//zookeeper server
	//
	// The defult value is 0, indicating no limit
	MaxCnxns int `json:"maxCnxns,omitempty"`

	// Limits the number of concurrent connections that a single client, identified
	// by IP address, may make to a single member of the ZooKeeper ensemble.
	//
	// The default value is 60
	MaxClientCnxns int `json:"maxClientCnxns,omitempty"`

	// The minimum session timeout in milliseconds that the server will allow the
	// client to negotiate
	//
	// The default value is 4000
	MinSessionTimeout int `json:"minSessionTimeout,omitempty"`

	// The maximum session timeout in milliseconds that the server will allow the
	// client to negotiate.
	//
	// The default value is 40000
	MaxSessionTimeout int `json:"maxSessionTimeout,omitempty"`

	// Retain the snapshots according to retain count
	//
	// The default value is 3
	AutoPurgeSnapRetainCount int `json:"autoPurgeSnapRetainCount,omitempty"`

	// The time interval in hours for which the purge task has to be triggered
	//
	// Disabled by default
	AutoPurgePurgeInterval int `json:"autoPurgePurgeInterval,omitempty"`

	// QuorumListenOnAllIPs when set to true the ZooKeeper server will listen for
	// connections from its peers on all available IP addresses, and not only the
	// address configured in the server list of the configuration file. It affects
	// the connections handling the ZAB protocol and the Fast Leader Election protocol.
	//
	// The default value is false.
	QuorumListenOnAllIPs bool `json:"quorumListenOnAllIPs,omitempty"`

	// key-value map of additional zookeeper configuration parameters
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	AdditionalConfig map[string]string `json:"additionalConfig,omitempty"`
}

func (z *ZookeeperSpec) WithDefaults() (changed bool) {
	if z.Replicas == nil {
		changed = true
		r := DefaultZkReplicas
		z.Replicas = &r
	}

	if z.Image == nil {
		z.Image = &ContainerImage{}
	}
	changed = z.Image.withDefaults(DefaultZkRepo, DefaultZkVersion, corev1.PullIfNotPresent) || changed

	if z.Persistence != nil {
		if z.Persistence.VolumeReclaimPolicy == "" {
			z.Persistence.VolumeReclaimPolicy = DefaultZkVolumeReclaimPolicy
			changed = true
		}

		if len(z.Persistence.PersistentVolumeClaimSpec.AccessModes) == 0 {
			z.Persistence.PersistentVolumeClaimSpec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
			changed = true
		}

		if len(z.Persistence.PersistentVolumeClaimSpec.Resources.Requests) == 0 {
			z.Persistence.PersistentVolumeClaimSpec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(DefaultZkStorage),
			}
			changed = true
		}
	}

	if z.ChRoot == "" {
		changed = true
		z.ChRoot = "/"
	} else if !strings.HasPrefix(z.ChRoot, "/") {
		changed = true
		z.ChRoot = "/" + z.ChRoot
	}
	return changed
}

// ZookeeperPodPolicy defines the common pod configuration for Pods, including when used
// in deployments, stateful-sets, etc.
type ZookeeperPodPolicy struct {
	// The scheduling constraints on pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Node Selector to be added on pods.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations to be added on pods.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// List of environment variables to set in the main ZK container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources is the resource requirements for the Zookeeper container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Optional Service Account to run the zookeeper pods under.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Labels specifies the labels to attach to pods the operator creates for
	// the zookeeper cluster.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations specifies the annotations to attach to zookeeper pods
	// creates.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// SecurityContext specifies the security context for the entire zookeeper pod
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/security-context
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// TerminationGracePeriodSeconds is the amount of time that kubernetes will
	// give for a zookeeper pod instance to shutdown normally.
	// The default value is 30.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TerminationGracePeriodSeconds int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// ImagePullSecrets is a list of references to secrets in the same namespace to use for pulling any images
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// SolrCloudStatus defines the observed state of SolrCloud
type SolrCloudStatus struct {
	// SolrNodes contain the statuses of each solr node running in this solr cloud.
	SolrNodes []SolrNodeStatus `json:"solrNodes"`

	// Replicas is the number of desired replicas in the cluster
	Replicas int32 `json:"replicas"`

	// PodSelector for SolrCloud pods, required by the HPA
	PodSelector string `json:"podSelector"`

	// ReadyReplicas is the number of ready replicas in the cluster
	ReadyReplicas int32 `json:"readyReplicas"`

	// UpToDateNodes is the number of Solr Node pods that are running the latest pod spec
	UpToDateNodes int32 `json:"upToDateNodes"`

	// The version of solr that the cloud is running
	Version string `json:"version"`

	// The version of solr that the cloud is meant to be running.
	// Will only be provided when the cloud is migrating between versions
	// +optional
	TargetVersion string `json:"targetVersion,omitempty"`

	// InternalCommonAddress is the internal common http address for all solr nodes
	InternalCommonAddress string `json:"internalCommonAddress"`

	// ExternalCommonAddress is the external common http address for all solr nodes.
	// Will only be provided when an ingressUrl is provided for the cloud
	// +optional
	ExternalCommonAddress *string `json:"externalCommonAddress,omitempty"`

	// ZookeeperConnectionInfo is the information on how to connect to the used Zookeeper
	ZookeeperConnectionInfo ZookeeperConnectionInfo `json:"zookeeperConnectionInfo"`

	// BackupRestoreReady announces whether the solrCloud has the backupRestorePVC mounted to all pods
	// and therefore is ready for backups and restores.
	BackupRestoreReady bool `json:"backupRestoreReady"`

	// BackupRepositoriesAvailable lists the backupRepositories specified in the SolrCloud and whether they are available across all Pods.
	// +optional
	BackupRepositoriesAvailable map[string]bool `json:"backupRepositoriesAvailable,omitempty"`
}

// SolrNodeStatus is the status of a solrNode in the cloud, with readiness status
// and internal and external addresses
type SolrNodeStatus struct {
	// The name of the pod running the node
	Name string `json:"name"`

	// The name of the Kubernetes Node which the pod is running on
	NodeName string `json:"nodeName"`

	// An address the node can be connected to from within the Kube cluster
	InternalAddress string `json:"internalAddress"`

	// An address the node can be connected to from outside of the Kube cluster
	// Will only be provided when an ingressUrl is provided for the cloud
	// +optional
	ExternalAddress string `json:"externalAddress,omitempty"`

	// Is the node up and running
	Ready bool `json:"ready"`

	// The version of solr that the node is running
	Version string `json:"version"`

	// This Solr Node pod is using the latest version of solrcloud pod spec.
	SpecUpToDate bool `json:"specUpToDate"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:resource:shortName=solr
//+kubebuilder:categories=all
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.readyReplicas,selectorpath=.status.podSelector
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version",description="Solr Version of the cloud"
//+kubebuilder:printcolumn:name="TargetVersion",type="string",JSONPath=".status.targetVersion",description="Target Solr Version of the cloud"
//+kubebuilder:printcolumn:name="DesiredNodes",type="integer",JSONPath=".spec.replicas",description="Number of solr nodes configured to run in the cloud"
//+kubebuilder:printcolumn:name="Nodes",type="integer",JSONPath=".status.replicas",description="Number of solr nodes running"
//+kubebuilder:printcolumn:name="ReadyNodes",type="integer",JSONPath=".status.readyReplicas",description="Number of solr nodes connected to the cloud"
//+kubebuilder:printcolumn:name="UpToDateNodes",type="integer",JSONPath=".status.upToDateNodes",description="Number of solr nodes running the latest SolrCloud pod spec"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SolrCloud is the Schema for the solrclouds API
type SolrCloud struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrCloudSpec   `json:"spec,omitempty"`
	Status SolrCloudStatus `json:"status,omitempty"`
}

// WithDefaults set default values when not defined in the spec.
func (sc *SolrCloud) WithDefaults(logger logr.Logger) bool {
	return sc.Spec.withDefaults(logger)
}

func (sc *SolrCloud) GetAllSolrPodNames() []string {
	replicas := 1
	if sc.Spec.Replicas != nil {
		replicas = int(*sc.Spec.Replicas)
	}
	podNames := make([]string, replicas)
	statefulSetName := sc.StatefulSetName()
	for i := range podNames {
		podNames[i] = fmt.Sprintf("%s-%d", statefulSetName, i)
	}
	return podNames
}

func (sc *SolrCloud) BasicAuthSecretName() string {
	if sc.Spec.SolrSecurity != nil && sc.Spec.SolrSecurity.BasicAuthSecret != "" {
		return sc.Spec.SolrSecurity.BasicAuthSecret
	} else {
		return fmt.Sprintf("%s-solrcloud-basic-auth", sc.Name)
	}
}

func (sc *SolrCloud) SecurityBootstrapSecretName() string {
	return fmt.Sprintf("%s-solrcloud-security-bootstrap", sc.Name)
}

// ConfigMapName returns the name of the cloud config-map
func (sc *SolrCloud) ConfigMapName() string {
	return fmt.Sprintf("%s-solrcloud-configmap", sc.GetName())
}

// StatefulSetName returns the name of the statefulset for the cloud
func (sc *SolrCloud) StatefulSetName() string {
	return fmt.Sprintf("%s-solrcloud", sc.GetName())
}

// CommonServiceName returns the name of the common service for the cloud
func (sc *SolrCloud) CommonServiceName() string {
	return fmt.Sprintf("%s-solrcloud-common", sc.GetName())
}

// InternalURLForCloud returns the name of the common service for the cloud
func InternalURLForCloud(sc *SolrCloud) string {
	return fmt.Sprintf("%s://%s-solrcloud-common.%s%s", sc.UrlScheme(false), sc.Name, sc.Namespace, sc.CommonPortSuffix(false))
}

// HeadlessServiceName returns the name of the headless service for the cloud
func (sc *SolrCloud) HeadlessServiceName() string {
	return fmt.Sprintf("%s-solrcloud-headless", sc.GetName())
}

// CommonIngressName returns the name of the common ingress for the cloud
func (sc *SolrCloud) CommonIngressName() string {
	return fmt.Sprintf("%s-solrcloud-common", sc.GetName())
}

// ProvidedZookeeperName returns the provided zk cluster
func (sc *SolrCloud) ProvidedZookeeperName() string {
	return fmt.Sprintf("%s-solrcloud-zookeeper", sc.GetName())
}

// ProvidedZookeeperAddress returns the client address of the provided zk cluster
func (sc *SolrCloud) ProvidedZookeeperAddress() string {
	return fmt.Sprintf("%s-solrcloud-zookeeper-client:2181", sc.GetName())
}

// ZkConnectionString returns the zkConnectionString for the cloud
func (sc *SolrCloud) ZkConnectionString() string {
	return sc.Status.ZkConnectionString()
}
func (scs SolrCloudStatus) ZkConnectionString() string {
	return scs.ZookeeperConnectionInfo.ZkConnectionString()
}
func (scs SolrCloudStatus) DissectZkInfo() (zkConnectionString string, zkServer string, zkChRoot string) {
	zkConnectionString = scs.ZookeeperConnectionInfo.ZkConnectionString()
	zkParts := strings.SplitN(zkConnectionString, "/", 2)
	zkServer = zkParts[0]
	zkChRoot = "/" + zkParts[1]

	return zkConnectionString, zkServer, zkChRoot
}

func (zkInfo ZookeeperConnectionInfo) ZkConnectionString() string {
	return zkInfo.InternalConnectionString + zkInfo.ChRoot
}

// UsesHeadlessService returns whether the given solrCloud requires a headless service to be created for it.
// solrCloud: SolrCloud instance
func (sc *SolrCloud) UsesHeadlessService() bool {
	return !sc.Spec.SolrAddressability.External.UsesIndividualNodeServices()
}

// UsesIndividualNodeServices returns whether the given solrCloud requires a individual node services to be created for it.
// solrCloud: SolrCloud instance
func (sc *SolrCloud) UsesIndividualNodeServices() bool {
	return sc.Spec.SolrAddressability.External.UsesIndividualNodeServices()
}

func (extOpts *ExternalAddressability) UsesIndividualNodeServices() bool {
	// LoadBalancer and Ingress will not work with headless services if each pod needs to be exposed externally.
	return extOpts != nil && !extOpts.HideNodes && (extOpts.Method == Ingress || extOpts.Method == LoadBalancer)
}

func (sc *SolrCloud) CommonExternalPrefix() string {
	return fmt.Sprintf("%s-%s-solrcloud", sc.Namespace, sc.Name)
}

func (sc *SolrCloud) NodeIngressPrefix(nodeName string) string {
	return fmt.Sprintf("%s-%s", sc.Namespace, nodeName)
}

func (sc *SolrCloud) ExternalDnsDomain(domainName string) string {
	return fmt.Sprintf("%s.%s", sc.Namespace, domainName)
}

func (sc *SolrCloud) customKubeDomain() string {
	if sc.Spec.SolrAddressability.KubeDomain != "" {
		return ".svc." + sc.Spec.SolrAddressability.KubeDomain
	} else {
		return ""
	}
}

func (sc *SolrCloud) NodeHeadlessUrl(nodeName string, withPort bool) (url string) {
	url = fmt.Sprintf("%s.%s.%s", nodeName, sc.HeadlessServiceName(), sc.Namespace) + sc.customKubeDomain()
	if withPort {
		url += sc.NodePortSuffix(false)
	}
	return url
}

func (sc *SolrCloud) NodeServiceUrl(nodeName string, withPort bool) (url string) {
	url = fmt.Sprintf("%s.%s", nodeName, sc.Namespace) + sc.customKubeDomain()
	if withPort {
		url += sc.NodePortSuffix(false)
	}
	return url
}

func (sc *SolrCloud) CommonPortSuffix(external bool) string {
	return sc.PortToSuffix(sc.Spec.SolrAddressability.CommonServicePort, external)
}

func (sc *SolrCloud) NodePortSuffix(external bool) string {
	return sc.PortToSuffix(sc.NodePort(), external)
}

func (sc *SolrCloud) NodePort() int {
	port := sc.Spec.SolrAddressability.PodPort
	external := sc.Spec.SolrAddressability.External
	// The nodePort is different than the podPort ONLY if the nodes are exposed externally and a nodePortOverride has been set.
	if external.UsesIndividualNodeServices() && external.NodePortOverride > 0 {
		port = sc.Spec.SolrAddressability.External.NodePortOverride
	}
	return port
}

func (sc *SolrCloud) PortToSuffix(port int, external bool) string {
	suffix := ""
	if sc.UrlScheme(external) == "https" {
		if port != 443 {
			suffix = ":" + strconv.Itoa(port)
		}
	} else {
		if port != 80 {
			suffix = ":" + strconv.Itoa(port)
		}
	}
	return suffix
}

func (sc *SolrCloud) InternalNodeUrl(nodeName string, withPort bool) string {
	if sc.UsesHeadlessService() {
		return sc.NodeHeadlessUrl(nodeName, withPort)
	} else if sc.UsesIndividualNodeServices() {
		return sc.NodeServiceUrl(nodeName, withPort)
	} else {
		return ""
	}
}

func (sc *SolrCloud) InternalCommonUrl(withPort bool) (url string) {
	url = fmt.Sprintf("%s.%s", sc.CommonServiceName(), sc.Namespace) + sc.customKubeDomain()
	if withPort {
		url += sc.CommonPortSuffix(false)
	}
	return url
}

func (sc *SolrCloud) ExternalNodeUrl(nodeName string, domainName string, withPort bool) (url string) {
	if sc.Spec.SolrAddressability.External.Method == Ingress {
		url = fmt.Sprintf("%s.%s", sc.NodeIngressPrefix(nodeName), domainName)
	} else if sc.Spec.SolrAddressability.External.Method == ExternalDNS {
		url = fmt.Sprintf("%s.%s", nodeName, sc.ExternalDnsDomain(domainName))
	}
	// TODO: Add LoadBalancer stuff here

	if withPort && sc.Spec.SolrAddressability.External.Method != Ingress {
		// Ingress does not require a port, since the port is whatever the ingress is listening on (80 and 443)
		url += sc.NodePortSuffix(true)
	}
	return url
}

func (sc *SolrCloud) ExternalCommonUrl(domainName string, withPort bool) (url string) {
	if sc.Spec.SolrAddressability.External.Method == Ingress {
		url = fmt.Sprintf("%s.%s", sc.CommonExternalPrefix(), domainName)
	} else if sc.Spec.SolrAddressability.External.Method == ExternalDNS {
		url = fmt.Sprintf("%s.%s", sc.CommonServiceName(), sc.ExternalDnsDomain(domainName))
	}
	// TODO: Add LoadBalancer stuff here

	if withPort && sc.Spec.SolrAddressability.External.Method != Ingress {
		// Ingress does not require a port, since the port is whatever the ingress is listening on (80 and 443)
		url += sc.CommonPortSuffix(true)
	}
	return url
}

func (ea *ExternalAddressability) HasIngressTLSTermination() bool {
	if ea != nil && ea.Method == Ingress && ea.IngressTLSTermination != nil {
		return ea.IngressTLSTermination.UseDefaultTLSSecret || ea.IngressTLSTermination.TLSSecret != ""
	}
	return false
}

func (sc *SolrCloud) UrlScheme(external bool) string {
	urlScheme := "http"
	if sc.Spec.SolrTLS != nil {
		urlScheme = "https"
	} else if external && sc.Spec.SolrAddressability.External.HasIngressTLSTermination() {
		urlScheme = "https"
	}
	return urlScheme
}

func (sc *SolrCloud) AdvertisedNodeHost(nodeName string) string {
	external := sc.Spec.SolrAddressability.External
	if external != nil && external.UseExternalAddress {
		return sc.ExternalNodeUrl(nodeName, sc.Spec.SolrAddressability.External.DomainName, false)
	} else {
		return sc.InternalNodeUrl(nodeName, false)
	}
}

func (sc *SolrCloud) UsesPersistentStorage() bool {
	return sc.Spec.StorageOptions.PersistentStorage != nil
}

func (sc *SolrCloud) DataVolumeName() string {
	// Set the default name of the pvc
	if sc.Spec.StorageOptions.PersistentStorage != nil &&
		sc.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name != "" {
		return sc.Spec.StorageOptions.PersistentStorage.PersistentVolumeClaimTemplate.ObjectMeta.Name
	} else {
		return "data"
	}
}

func (sc *SolrCloud) SharedLabels() map[string]string {
	return sc.SharedLabelsWith(map[string]string{})
}

func (sc *SolrCloud) SharedLabelsWith(labels map[string]string) map[string]string {
	newLabels := map[string]string{}

	if labels != nil {
		for k, v := range labels {
			newLabels[k] = v
		}
	}

	newLabels["solr-cloud"] = sc.Name
	return newLabels
}

//+kubebuilder:object:root=true

// SolrCloudList contains a list of SolrCloud
type SolrCloudList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrCloud `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrCloud{}, &SolrCloudList{})
}

// +kubebuilder:validation:Enum=None;Want;Need
type ClientAuthType string

const (
	None ClientAuthType = "None"
	Want ClientAuthType = "Want"
	Need ClientAuthType = "Need"
)

type MountedTLSDirectory struct {
	// The path on the main Solr container where the TLS files are mounted by some external agent or CSI Driver
	Path string `json:"path"`

	// Override the name of the keystore file; no default, if you don't supply this setting, then the corresponding
	// env vars and Java system properties will not be configured for the pod template
	// +optional
	KeystoreFile string `json:"keystoreFile,omitempty"`

	// Override the name of the keystore password file; defaults to keystore-password
	// +optional
	KeystorePasswordFile string `json:"keystorePasswordFile,omitempty"`

	// Override the name of the truststore file; no default, if you don't supply this setting, then the corresponding
	// env vars and Java system properties will not be configured for the pod template
	// +optional
	TruststoreFile string `json:"truststoreFile,omitempty"`

	// Override the name of the truststore password file; defaults to the same value as the KeystorePasswordFile
	// +optional
	TruststorePasswordFile string `json:"truststorePasswordFile,omitempty"`
}

type SolrTLSOptions struct {
	// TLS Secret containing a pkcs12 keystore; required for Solr pods unless mountedTLSDir is used
	// +optional
	PKCS12Secret *corev1.SecretKeySelector `json:"pkcs12Secret,omitempty"`

	// Secret containing the key store password; this field is required unless mountedTLSDir is used, as most JVMs do not support pkcs12 keystores without a password
	// +optional
	KeyStorePasswordSecret *corev1.SecretKeySelector `json:"keyStorePasswordSecret,omitempty"`

	// TLS Secret containing a pkcs12 truststore; if not provided, then the keystore and password are used for the truststore
	// The specified key is used as the truststore file name when mounted into Solr pods
	// +optional
	TrustStoreSecret *corev1.SecretKeySelector `json:"trustStoreSecret,omitempty"`

	// Secret containing the trust store password; if not provided the keyStorePassword will be used
	// +optional
	TrustStorePasswordSecret *corev1.SecretKeySelector `json:"trustStorePasswordSecret,omitempty"`

	// Determines the client authentication method, either None, Want, or Need;
	// this affects K8s ability to call liveness / readiness probes so use cautiously.
	// Only applies for server certificates, has no effect on client certificates
	// +kubebuilder:default=None
	ClientAuth ClientAuthType `json:"clientAuth,omitempty"`

	// Verify client's hostname during SSL handshake
	// Only applies for server configuration
	// +optional
	VerifyClientHostname bool `json:"verifyClientHostname,omitempty"`

	// TLS certificates contain host/ip "peer name" information that is validated by default.
	// +optional
	CheckPeerName bool `json:"checkPeerName,omitempty"`

	// Opt-in flag to restart Solr pods after TLS secret updates, such as if the cert is renewed; default is false.
	// This option only applies when using the `spec.solrTLS.pkcs12Secret` option; when using the `spec.solrTLS.mountedTLSDir` option,
	// you need to ensure pods get restarted before the certs expire, see `spec.updateStrategy.restartSchedule` for scheduling restarts.
	// +optional
	RestartOnTLSSecretUpdate bool `json:"restartOnTLSSecretUpdate,omitempty"`

	// Used to specify a path where the keystore, truststore, and password files for the TLS certificate are mounted by an external agent or CSI driver.
	// This option is typically used with `spec.updateStrategy.restartSchedule` to restart Solr pods before the mounted TLS cert expires.
	// +optional
	MountedTLSDir *MountedTLSDirectory `json:"mountedTLSDir,omitempty"`
}

// +kubebuilder:validation:Enum=Basic
type AuthenticationType string

const (
	Basic AuthenticationType = "Basic"
)

type SolrSecurityOptions struct {
	// Indicates the authentication plugin type that is being used by Solr; for now only "Basic" is supported by the
	// Solr operator but support for other authentication plugins may be added in the future.
	AuthenticationType AuthenticationType `json:"authenticationType,omitempty"`

	// Secret (kubernetes.io/basic-auth) containing credentials the operator should use for API requests to secure Solr pods.
	// If you provide this secret, then the operator assumes you've also configured your own security.json file and
	// uploaded it to Solr. If you change the password for this user using the Solr security API, then you *must* update
	// the secret with the new password or the operator will be  locked out of Solr and API requests will fail,
	// ultimately causing a CrashBackoffLoop for all pods if probe endpoints are secured (see 'probesRequireAuth' setting).
	//
	// If you don't supply this secret, then the operator creates a kubernetes.io/basic-auth secret containing the password
	// for the "k8s-oper" user. All API requests from the operator are made as the "k8s-oper" user, which is configured
	// with read-only access to a minimal set of endpoints. In addition, the operator bootstraps a default security.json
	// file and credentials for two additional users: admin and solr. The 'solr' user has basic read access to Solr
	// resources. Once the security.json is bootstrapped, the operator will not update it! You're expected to use the
	// 'admin' user to access the Security API to make further changes. It's strictly a bootstrapping operation.
	// +optional
	BasicAuthSecret string `json:"basicAuthSecret,omitempty"`

	// Flag to indicate if the configured HTTP endpoint(s) used for the probes require authentication; defaults
	// to false. If you set to true, then probes will use a local command on the main container to hit the secured
	// endpoints with credentials sourced from an env var instead of HTTP directly.
	// +optional
	ProbesRequireAuth bool `json:"probesRequireAuth,omitempty"`

	// Configure a user-provided security.json from a secret to allow for advanced security config.
	// If not specified, the operator bootstraps a security.json with basic auth enabled.
	// This is a bootstrapping config only; once Solr is initialized, the security config should be managed by the security API.
	// +optional
	BootstrapSecurityJson *corev1.SecretKeySelector `json:"bootstrapSecurityJson,omitempty"`
}
