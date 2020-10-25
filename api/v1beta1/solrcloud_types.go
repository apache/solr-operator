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

package v1beta1

import (
	"fmt"
	"strconv"
	"strings"

	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultPullPolicy = corev1.PullIfNotPresent

	DefaultSolrReplicas = int32(3)
	DefaultSolrRepo     = "library/solr"
	DefaultSolrVersion  = "7.7.0"
	DefaultSolrStorage  = "5Gi"
	DefaultSolrJavaMem  = "-Xms1g -Xmx2g"
	DefaultSolrOpts     = ""
	DefaultSolrLogLevel = "INFO"
	DefaultSolrGCTune   = ""

	DefaultBusyBoxImageRepo    = "library/busybox"
	DefaultBusyBoxImageVersion = "1.28.0-glibc"

	DefaultZkReplicas            = int32(3)
	DefaultZkStorage             = "5Gi"
	DefaultZkRepo                = "pravega/zookeeper"
	DefaultZkVersion             = "0.2.6"
	DefaultZkVolumeReclaimPolicy = zk.VolumeReclaimPolicyRetain

	DefaultEtcdReplicas = 3
	DefaultEtcdRepo     = "quay.io/coreos/etcd"
	DefaultEtcdVersion  = "3.2.13"

	DefaultZetcdReplicas = int32(1)
	DefaultZetcdRepo     = "quay.io/etcd-io/zetcd"
	DefaultZetcdVersion  = "0.0.5"

	SolrTechnologyLabel      = "solr-cloud"
	ZookeeperTechnologyLabel = "zookeeper"

	DefaultKeyStorePath         = "/var/solr/tls"
	DefaultKeyStoreFile         = "keystore.p12"
	DefaultWritableKeyStorePath = "/var/solr/tls/pkcs12"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

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

	// DEPRECATED: Please use the options provided in customSolrKubeOptions.podOptions
	//
	// Pod defines the policy to create pod for the SolrCloud.
	// Updating the Pod does not take effect on any existing pods.
	// +optional
	SolrPod SolrPodPolicy `json:"solrPodPolicy,omitempty"`

	// DataPvcSpec is the spec to describe PVC for the solr node to store its data.
	// This field is optional. If no PVC spec is provided, each solr node will use emptyDir as the data volume
	// +optional
	DataPvcSpec *corev1.PersistentVolumeClaimSpec `json:"dataPvcSpec,omitempty"`

	// Required for backups & restores to be enabled.
	// This is a volumeSource for a volume that will be mounted to all solrNodes to store backups and load restores.
	// The data within the volume will be namespaces for this instance, so feel free to use the same volume for multiple clouds.
	// Since the volume will be mounted to all solrNodes, it must be able to be written from multiple pods.
	// If a PVC reference is given, the PVC must have `accessModes: - ReadWriteMany`.
	// Other options are to use a NFS volume.
	// +optional
	BackupRestoreVolume *corev1.VolumeSource `json:"backupRestoreVolume,omitempty"`

	// Provide custom options for kubernetes objects created for the Solr Cloud.
	// +optional
	CustomSolrKubeOptions CustomSolrKubeOptions `json:"customSolrKubeOptions,omitempty"`

	// Customize how Solr is addressed both internally and externally in Kubernetes.
	// +optional
	SolrAddressability SolrAddressabilityOptions `json:"solrAddressability,omitempty"`

	// +optional
	BusyBoxImage *ContainerImage `json:"busyBoxImage,omitempty"`

	// +optional
	SolrJavaMem string `json:"solrJavaMem,omitempty"`

	// You can add common system properties to the SOLR_OPTS environment variable
	// SolrOpts is the string interface for these optional settings
	// +optional
	SolrOpts string `json:"solrOpts,omitempty"`

	// Set the Solr Log level, defaults to INFO
	// +optional
	SolrLogLevel string `json:"solrLogLevel,omitempty"`

	// Set GC Tuning configuration through GC_TUNE environment variable
	// +optional
	SolrGCTune string `json:"solrGCTune,omitempty"`

	// Options to enable TLS between Solr pods
	// +optional
	SolrTLS *SolrTLSOptions `json:"solrTLS,omitempty"`
}

func (spec *SolrCloudSpec) withDefaults(instanceName string, ingressBaseDomain string) (changed bool) {
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

	changed = spec.SolrAddressability.withDefaults(ingressBaseDomain) || changed

	if spec.SolrTLS != nil {
		changed = spec.SolrTLS.withDefaults(instanceName) || changed
	}

	if spec.ZookeeperRef == nil {
		spec.ZookeeperRef = &ZookeeperRef{}
	}
	changed = spec.ZookeeperRef.withDefaults() || changed

	if spec.SolrImage == nil {
		spec.SolrImage = &ContainerImage{}
	}
	changed = spec.SolrImage.withDefaults(DefaultSolrRepo, DefaultSolrVersion, DefaultPullPolicy) || changed

	if spec.DataPvcSpec != nil {
		spec.DataPvcSpec.AccessModes = []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		}
		if len(spec.DataPvcSpec.Resources.Requests) == 0 {
			spec.DataPvcSpec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(DefaultSolrStorage),
			}
			changed = true
		}
		if spec.DataPvcSpec.VolumeMode == nil {
			temp := corev1.PersistentVolumeFilesystem
			spec.DataPvcSpec.VolumeMode = &temp
		}
	}

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
	// Defaults to 80
	// +optional
	CommonServicePort int `json:"commonServicePort,omitempty"`

	// KubeDomain allows for the specification of an override of the default "cluster.local" Kubernetes cluster domain.
	// Only use this option if the Kubernetes cluster has been setup with a custom domain.
	// +optional
	KubeDomain string `json:"kubeDomain,omitempty"`
}

func (opts *SolrAddressabilityOptions) withDefaults(ingressBaseDomain string) (changed bool) {
	// DEPRECATED: ingressBaseDomain will be removed in v0.3.0
	if opts.External == nil && ingressBaseDomain != "" {
		changed = true
		opts.External = &ExternalAddressability{
			Method:             Ingress,
			DomainName:         ingressBaseDomain,
			UseExternalAddress: true,
			NodePortOverride:   80,
		}
	} else if opts.External != nil {
		changed = opts.External.withDefaults()
	}
	if opts.PodPort == 0 {
		changed = true
		opts.PodPort = 8983
	}
	if opts.CommonServicePort == 0 {
		changed = true
		opts.CommonServicePort = 80
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
	// Deprecation warning: When an ingress-base-domain is passed in to the operator, this value defaults to true.
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
	// e.g. default-example-solrcloud.given.domain.name.com
	//
	// This options will be required for the Ingress and ExternalDNS methods once the ingressBaseDomain startup parameter is removed.
	//
	// For the LoadBalancer method, this field is optional and will only be used when useExternalAddress=true.
	// If used with the LoadBalancer method, you will need DNS routing to the LoadBalancer IP address through the url template given above.
	// +optional
	DomainName string `json:"domainName,omitempty"`

	// Provide additional domainNames that the Ingress or ExternalDNS should listen on.
	// This option is ignored with the LoadBalancer method.
	// +optional
	AdditionalDomainNames []string `json:"additionalDomains,omitempty"`

	// NodePortOverride defines the port to have all Solr node service(s) listen on and advertise itself as if advertising through an Ingress or LoadBalancer.
	// This overrides the default usage of the podPort.
	//
	// This is option is only used when HideNodes=false, otherwise the the port each Solr Node will advertise itself with the podPort.
	// This option is also unavailable with the ExternalDNS method.
	//
	// If using method=Ingress, your ingress controller is required to listen on this port.
	// If your ingress controller is not listening on the podPort, then this option is required for solr to be addressable via an Ingress.
	//
	// Defaults to 80 if HideNodes=false and method=Ingress, otherwise this is optional.
	// +optional
	NodePortOverride int `json:"nodePortOverride,omitempty"`
}

// ExternalAddressability is a string enumeration type that enumerates
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

func (opts *ExternalAddressability) withDefaults() (changed bool) {
	// You can't use an externalAddress for Solr Nodes if the Nodes are hidden externally
	if opts.UseExternalAddress && opts.HideNodes {
		changed = true
		opts.UseExternalAddress = false
	}
	// If the Ingress method is used, default the nodePortOverride to 80, since that is the port that most ingress controllers listen on.
	if !opts.HideNodes && opts.Method == Ingress && opts.NodePortOverride == 0 {
		changed = true
		opts.NodePortOverride = 80
	}
	// If a headless service is used, aka not using individual node services, then a nodePortOverride is not allowed.
	if !opts.UsesIndividualNodeServices() && opts.NodePortOverride > 0 {
		changed = true
		opts.NodePortOverride = 0
	}

	return changed
}

type CertificateIssuerRef struct {
	// Name of the resource being referred to.
	Name string `json:"name"`
	// Kind of the resource being referred to.
	// +optional
	Kind string `json:"kind,omitempty"`
}

type CreateCertificate struct {
	// Name of the certificate to create for Solr TLS; defaults to the name of the SolrCloud + "-solr-tls"
	// +optional
	Name string `json:"name,omitempty"`

	// Subject distinguished name for the auto-generated cert;
	// use format: CN=localhost, OU=Organizational Unit, O=Organization, L=Location, ST=State, C=Country
	// If not provided, then the operator uses the SolrCloud instance name as the Organization
	// +optional
	SubjectDistinguishedName string `json:"subjectDName,omitempty"`

	// Specify the cert-manager Issuer or ClusterIssuer to use to issue the certificate;
	// for creating a self-signed cert, don't supply an issuerRef.
	// +optional
	IssuerRef *CertificateIssuerRef `json:"issuerRef,omitempty"`
}

type ClientAuthType string

const (
	None ClientAuthType = "None"
	Want ClientAuthType = "Want"
	Need ClientAuthType = "Need"
)

type SolrTLSOptions struct {
	// Solr operator should just create a TLS cert automatically using the specified Issuer
	// If issuerRef is not specified, the operator generates a self-signed certificate.
	// +optional
	AutoCreate *CreateCertificate `json:"autoCreate,omitempty"`

	// Secret containing the key store password
	// +optional
	KeyStorePasswordSecret *corev1.SecretKeySelector `json:"keyStorePasswordSecret,omitempty"`

	// TLS Secret containing a pkcs12 keystore created by cert-manager; required unless autoCreate is requested.
	// +optional
	PKCS12Secret *corev1.SecretKeySelector `json:"pkcs12Secret,omitempty"`

	// Determines the client authentication method, either None, Want, or Need;
	// this affects K8s ability to call liveness / readiness probes so use cautiously.
	// +optional
	ClientAuth ClientAuthType `json:"clientAuth,omitempty"`

	// Verify client's hostname during SSL handshake
	// +optional
	VerifyClientHostname bool `json:"verifyClientHostname,omitempty"`

	// TLS certificates contain host/ip "peer name" information that is validated by default.
	// +optional
	CheckPeerName bool `json:"checkPeerName,omitempty"`

	// Opt-in flag to restart Solr pods after TLS secret updates, such as if the cert is renewed; default is false.
	// +optional
	RestartOnTLSSecretUpdate bool `json:"restartOnTLSSecretUpdate,omitempty"`

	// Set at runtime during reconcile, once the TLS secret is issued
	TLSSecretVersion string `json:"-"`
}

func (opts *SolrTLSOptions) withDefaults(instanceName string) (changed bool) {

	if opts.AutoCreate != nil {
		// User wants us to create a Certificate and Keystore password on-the-fly for them
		// Fill-in any missing information with defaults so that the reconciliation process
		// doesn't have to account for missing values with defaults
		if opts.AutoCreate.Name == "" {
			opts.AutoCreate.Name = fmt.Sprintf("%s-solr-tls", instanceName)
			changed = true
		}

		if opts.AutoCreate.SubjectDistinguishedName == "" {
			opts.AutoCreate.SubjectDistinguishedName = "O=" + instanceName
			changed = true
		}

		if opts.KeyStorePasswordSecret == nil {
			secretName := fmt.Sprintf("%s-pkcs12-keystore", instanceName)
			opts.KeyStorePasswordSecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: "password-key",
			}
			changed = true
		}

		if opts.PKCS12Secret == nil {
			var issuerName string
			if opts.AutoCreate.IssuerRef != nil {
				issuerName = strings.Replace(opts.AutoCreate.IssuerRef.Name, "-issuer", "", -1)
			} else {
				issuerName = "selfsigned"
			}
			certSecretName := fmt.Sprintf("%s-%s-%s", instanceName, issuerName, "solr-tls")
			opts.PKCS12Secret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: certSecretName,
				},
				Key: DefaultKeyStoreFile,
			}
			changed = true
		}
	}

	if opts.ClientAuth == "" {
		opts.ClientAuth = None
		changed = true
	}

	return changed
}

// DEPRECATED: Please use the options provided in SolrCloud.Spec.customSolrKubeOptions.podOptions
//
// SolrPodPolicy defines the common pod configuration for Pods, including when used
// in deployments, stateful-sets, etc.
type SolrPodPolicy struct {
	// The scheduling constraints on pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Resources is the resource requirements for the container.
	// This field cannot be updated once the cluster is created.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ZookeeperRef defines the zookeeper ensemble for solr to connect to
// If no ConnectionString is provided, the solr-cloud controller will create and manage an internal ensemble
type ZookeeperRef struct {
	// A zookeeper ensemble that is run independently of the solr operator
	// If an externalConnectionString is provided, but no internalConnectionString is, the external will be used as the internal
	// +optional
	ConnectionInfo *ZookeeperConnectionInfo `json:"connectionInfo,omitempty"`

	// A zookeeper that is created by the solr operator
	// Note: This option will not allow the SolrCloud to run across kube-clusters.
	// +optional
	ProvidedZookeeper *ProvidedZookeeper `json:"provided,omitempty"`
}

func (ref *ZookeeperRef) withDefaults() (changed bool) {
	if ref.ProvidedZookeeper == nil && ref.ConnectionInfo == nil {
		changed = true
		ref.ProvidedZookeeper = &ProvidedZookeeper{}
	} else if ref.ConnectionInfo != nil {
		if ref.ProvidedZookeeper != nil {
			ref.ProvidedZookeeper = nil
			changed = true
		}
		changed = ref.ConnectionInfo.withDefaults() || changed
	}
	if ref.ProvidedZookeeper != nil {
		changed = ref.ProvidedZookeeper.withDefaults() || changed
	}
	return changed
}

func (ci *ZookeeperConnectionInfo) withDefaults() (changed bool) {
	if ci.InternalConnectionString == "" {
		if ci.ExternalConnectionString != nil {
			changed = true
			ci.InternalConnectionString = *ci.ExternalConnectionString
		}
	}
	if ci.ChRoot == "" {
		changed = true
		ci.ChRoot = "/"
	} else if !strings.HasPrefix(ci.ChRoot, "/") {
		changed = true
		ci.ChRoot = "/" + ci.ChRoot
	}
	return changed
}

// ProvidedZookeeper defines the internal zookeeper ensemble to run
type ProvidedZookeeper struct {
	// Create a new Zookeeper Ensemble with the following spec
	// Note: Requires
	//   - The zookeeperOperator flag to be provided to the Solr Operator
	//   - A zookeeper operator to be running
	// +optional
	Zookeeper *ZookeeperSpec `json:"zookeeper,omitempty"`

	// Create a new Etcd Cluster and a Zetcd proxy to connect the cluster to solr
	// Note: Requires
	//   - The etcdOperator flag to be provided to the Solr Operator
	//   - An etcd operator to be running
	// +optional
	Zetcd *FullZetcdSpec `json:"zetcd,inline"`

	// The ChRoot to connect solr at
	// +optional
	ChRoot string `json:"chroot,omitempty"`
}

func (z *ProvidedZookeeper) withDefaults() (changed bool) {
	if z.Zookeeper == nil && z.Zetcd == nil {
		changed = true
		z.Zookeeper = &ZookeeperSpec{}
	}
	if z.Zookeeper != nil {
		changed = z.Zookeeper.withDefaults() || changed
	}
	if z.Zetcd != nil {
		changed = z.Zetcd.withDefaults() || changed
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

// ZookeeperSpec defines the internal zookeeper ensemble to run for solr
type ZookeeperSpec struct {
	// Number of members to create up for the ZK ensemble
	// Defaults to 3
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image of Zookeeper to run
	// +optional
	Image *ContainerImage `json:"image,omitempty"`

	// PersistentVolumeClaimSpec is the spec to describe PVC for the zk container
	// This field is optional. If no PVC spec is provided, etcd container will use emptyDir as volume.
	// WARNING: This field is DEPRECATED, please use the Persistence option
	// +optional
	PersistentVolumeClaimSpec *corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec,omitempty"`

	// Persistence is the configuration for zookeeper persistent layer.
	// PersistentVolumeClaimSpec and VolumeReclaimPolicy can be specified in here.
	// +optional
	Persistence *zk.Persistence `json:"persistence,omitempty"`

	// Pod resources for zookeeper pod
	// +optional
	ZookeeperPod ZookeeperPodPolicy `json:"zookeeperPodPolicy,omitempty"`
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

	// Resources is the resource requirements for the container.
	// This field cannot be updated once the cluster is created.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

func (z *ZookeeperSpec) withDefaults() (changed bool) {
	if z.Replicas == nil {
		changed = true
		r := DefaultZkReplicas
		z.Replicas = &r
	}

	if z.Image == nil {
		z.Image = &ContainerImage{}
	}
	changed = z.Image.withDefaults(DefaultZkRepo, DefaultZkVersion, DefaultPullPolicy) || changed

	// Backwards compatibility with old ZK Persistence options.
	// This will be removed eventually
	if z.Persistence == nil && z.PersistentVolumeClaimSpec != nil {
		z.Persistence = &zk.Persistence{
			PersistentVolumeClaimSpec: *z.PersistentVolumeClaimSpec,
		}
		changed = true
	}

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
	return changed
}

// FullZetcdSpec defines the internal etcd ensemble and zetcd server to run for solr (spoofing zookeeper)
type FullZetcdSpec struct {
	// +optional
	EtcdSpec *EtcdSpec `json:"etcdSpec,omitempty"`

	// +optional
	ZetcdSpec *ZetcdSpec `json:"zetcdSpec,omitempty"`
}

func (z *FullZetcdSpec) withDefaults() (changed bool) {
	if z.EtcdSpec == nil {
		z.EtcdSpec = &EtcdSpec{}
	}
	changed = z.EtcdSpec.withDefaults() || changed

	if z.ZetcdSpec == nil {
		z.ZetcdSpec = &ZetcdSpec{}
	}
	changed = z.ZetcdSpec.withDefaults() || changed

	return changed
}

// EtcdSpec defines the internal etcd ensemble to run for solr (spoofing zookeeper)
type EtcdSpec struct {
	// The number of EtcdReplicas to create
	// +optional
	Replicas *int `json:"replicas,omitempty"`

	// +optional
	Image *ContainerImage `json:"image,omitempty"`

	// PersistentVolumeClaimSpec is the spec to describe PVC for the zk container
	// This field is optional. If no PVC spec, etcd container will use emptyDir as volume
	PersistentVolumeClaimSpec *corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec,omitempty"`

	// Pod resources for etcd pods
	// +optional
	EtcdPod EtcdPodPolicy `json:"etcdPodPolicy,omitempty"`
}

// EtcdPodPolicy defines the common pod configuration for Pods, including when used
// in deployments, stateful-sets, etc.
type EtcdPodPolicy struct {
	// The scheduling constraints on pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Resources is the resource requirements for the container.
	// This field cannot be updated once the cluster is created.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

func (s *EtcdSpec) withDefaults() (changed bool) {
	if s.Replicas == nil {
		changed = true
		r := DefaultEtcdReplicas
		s.Replicas = &r
	}

	if s.Image == nil {
		s.Image = &ContainerImage{}
	}
	changed = s.Image.withDefaults(DefaultEtcdRepo, DefaultEtcdVersion, DefaultPullPolicy) || changed

	return changed
}

// ZetcdSpec defines the zetcd proxy to run connection solr and etcd
type ZetcdSpec struct {
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Image *ContainerImage `json:"image,omitempty"`

	// Pod resources for zetcd pods
	// +optional
	ZetcdPod ZetcdPodPolicy `json:"zetcdPodPolicy,omitempty"`
}

// EtcdPodPolicy defines the common pod configuration for Pods, including when used
// in deployments, stateful-sets, etc.
type ZetcdPodPolicy struct {
	// The scheduling constraints on pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Resources is the resource requirements for the container.
	// This field cannot be updated once the cluster is created.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

func (s *ZetcdSpec) withDefaults() (changed bool) {
	if s.Replicas == nil {
		changed = true
		r := DefaultZetcdReplicas
		s.Replicas = &r
	}

	if s.Image == nil {
		s.Image = &ContainerImage{}
	}
	changed = s.Image.withDefaults(DefaultZetcdRepo, DefaultZetcdVersion, DefaultPullPolicy) || changed

	return changed
}

// SolrCloudStatus defines the observed state of SolrCloud
type SolrCloudStatus struct {
	// SolrNodes contain the statuses of each solr node running in this solr cloud.
	SolrNodes []SolrNodeStatus `json:"solrNodes"`

	// Replicas is the number of number of desired replicas in the cluster
	Replicas int32 `json:"replicas"`

	// ReadyReplicas is the number of number of ready replicas in the cluster
	ReadyReplicas int32 `json:"readyReplicas"`

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

	// Flag to mark that we've set this cluster-wide property so we don't reconnect to ZK for every loop
	UrlSchemeClusterProperty bool `json:"urlSchemeClusterProperty"`
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
}

// SolrNodeStatus is the status of a solrNode in the cloud, with readiness status
// and internal and external addresses
type ZookeeperConnectionInfo struct {
	// The connection string to connect to the ensemble from within the Kubernetes cluster
	// +optional
	InternalConnectionString string `json:"internalConnectionString,omitempty"`

	// The connection string to connect to the ensemble from outside of the Kubernetes cluster
	// If external and no internal connection string is provided, the external cnx string will be used as the internal cnx string
	// +optional
	ExternalConnectionString *string `json:"externalConnectionString,omitempty"`

	// The ChRoot to connect solr at
	// +optional
	ChRoot string `json:"chroot,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced

// SolrCloud is the Schema for the solrclouds API
// +kubebuilder:resource:shortName=solr
// +kubebuilder:categories=all
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.readyReplicas
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version",description="Solr Version of the cloud"
// +kubebuilder:printcolumn:name="TargetVersion",type="string",JSONPath=".status.targetVersion",description="Target Solr Version of the cloud"
// +kubebuilder:printcolumn:name="DesiredNodes",type="integer",JSONPath=".spec.replicas",description="Number of solr nodes configured to run in the cloud"
// +kubebuilder:printcolumn:name="Nodes",type="integer",JSONPath=".status.replicas",description="Number of solr nodes running"
// +kubebuilder:printcolumn:name="ReadyNodes",type="integer",JSONPath=".status.readyReplicas",description="Number of solr nodes connected to the cloud"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type SolrCloud struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrCloudSpec   `json:"spec,omitempty"`
	Status SolrCloudStatus `json:"status,omitempty"`
}

// WithDefaults set default values when not defined in the spec.
func (sc *SolrCloud) WithDefaults(ingressBaseDomain string) bool {
	return sc.Spec.withDefaults(sc.Name, ingressBaseDomain)
}

func (sc *SolrCloud) GetAllSolrNodeNames() []string {
	replicas := 1
	if sc.Spec.Replicas != nil {
		replicas = int(*sc.Spec.Replicas)
	}
	nodeNames := make([]string, replicas)
	statefulSetName := sc.StatefulSetName()
	for i := range nodeNames {
		nodeNames[i] = fmt.Sprintf("%s-%d", statefulSetName, i)
	}
	return nodeNames
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
func InternalURLForCloud(cloudName string, namespace string) string {
	return fmt.Sprintf("http://%s-solrcloud-common.%s", cloudName, namespace)
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

// ProvidedZetcdName returns the name of the zetcd cluster
func (sc *SolrCloud) ProvidedZetcdName() string {
	return fmt.Sprintf("%s-solrcloud-zetcd", sc.GetName())
}

// IngressName returns the name of the ingress for the cloud
func (sc *SolrCloud) ProvidedZetcdAddress() string {
	return fmt.Sprintf("%s-solrcloud-zetcd:2181", sc.GetName())
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

func (sc *SolrCloud) CommonExternalUrl(domainName string) string {
	return fmt.Sprintf("%s.%s", sc.CommonExternalPrefix(), domainName)
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
		url += sc.NodePortSuffix()
	}
	return url
}

func (sc *SolrCloud) NodeServiceUrl(nodeName string, withPort bool) (url string) {
	url = fmt.Sprintf("%s.%s", nodeName, sc.Namespace) + sc.customKubeDomain()
	if withPort {
		url += sc.NodePortSuffix()
	}
	return url
}

func (sc *SolrCloud) CommonPortSuffix() string {
	return PortToSuffix(sc.Spec.SolrAddressability.CommonServicePort)
}

func (sc *SolrCloud) NodePortSuffix() string {
	return PortToSuffix(sc.NodePort())
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

// PortToSuffix returns the url suffix for a port.
// Port 80 does not require a suffix, as it is the default port for HTTP.
func PortToSuffix(port int) string {
	if port == 80 {
		return ""
	}
	return ":" + strconv.Itoa(port)
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
		url += sc.CommonPortSuffix()
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
	if withPort {
		url += sc.NodePortSuffix()
	}
	return url
}

func (sc *SolrCloud) ExternalCommonUrl(domainName string, withPort bool) (url string) {
	if sc.Spec.SolrAddressability.External.Method == Ingress {
		url = fmt.Sprintf("%s.%s", sc.CommonExternalPrefix(), domainName)
	} else if sc.Spec.SolrAddressability.External.Method == ExternalDNS {
		url = fmt.Sprintf("%s.%s", sc.CommonServiceName(), sc.ExternalDnsDomain(domainName))
	}
	if withPort {
		url += sc.CommonPortSuffix()
	}
	return url
}

func (sc *SolrCloud) AdvertisedNodeHost(nodeName string) string {
	external := sc.Spec.SolrAddressability.External
	if external != nil && external.UseExternalAddress {
		return sc.ExternalNodeUrl(nodeName, sc.Spec.SolrAddressability.External.DomainName, false)
	} else {
		return sc.InternalNodeUrl(nodeName, false)
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

// +kubebuilder:object:root=true

// SolrCloudList contains a list of SolrCloud
type SolrCloudList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrCloud `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrCloud{}, &SolrCloudList{})
}
