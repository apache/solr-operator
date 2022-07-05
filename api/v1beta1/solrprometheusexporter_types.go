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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SolrPrometheusExporterTechnologyLabel = "solr-prometheus-exporter"
)

// SolrPrometheusExporterSpec defines the desired state of SolrPrometheusExporter
type SolrPrometheusExporterSpec struct {
	// Reference of the Solr instance to collect metrics for
	SolrReference SolrReference `json:"solrReference"`

	// Image of Solr Prometheus Exporter to run.
	// +optional
	Image *ContainerImage `json:"image,omitempty"`

	// Provide custom options for kubernetes objects created for the SolrPrometheusExporter.
	// +optional
	CustomKubeOptions CustomExporterKubeOptions `json:"customKubeOptions,omitempty"`

	// The entrypoint into the exporter. Defaults to the official docker-solr location.
	// +optional
	ExporterEntrypoint string `json:"exporterEntrypoint,omitempty"`

	// Number of threads to use for the prometheus exporter
	// Defaults to 1
	// +optional
	NumThreads int32 `json:"numThreads,omitempty"`

	// The interval to scrape Solr at (in seconds)
	// Defaults to 60 seconds
	// +optional
	ScrapeInterval int32 `json:"scrapeInterval,omitempty"`

	// The xml config for the metrics
	// +optional
	Config string `json:"metricsConfig,omitempty"`

	// An initContainer is needed to create a wrapper script around the exporter entrypoint when TLS is enabled
	// with the `spec.solrReference.solrTLS.mountedTLSDir` option
	// +optional
	BusyBoxImage *ContainerImage `json:"busyBoxImage,omitempty"`

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

func (ps *SolrPrometheusExporterSpec) withDefaults(namespace string) (changed bool) {
	changed = ps.SolrReference.withDefaults(namespace) || changed

	if ps.Image == nil {
		// Only instantiate the Image variable if the Solr reference is a SolrCloud resource.
		// If so, then the image will be defaulted to the same image that the Solr reference uses.
		if ps.SolrReference.Cloud == nil || ps.SolrReference.Cloud.Name == "" {
			ps.Image = &ContainerImage{}
		}
	}

	// Do not change this to an else, as the Image might be instantiated in the if
	if ps.Image != nil {
		changed = ps.Image.withDefaults(DefaultSolrRepo, DefaultSolrVersion, DefaultPullPolicy) || changed
	}

	if ps.NumThreads == 0 {
		ps.NumThreads = 1
		changed = true
	}

	return changed
}

// SolrReference defines a reference to an internal or external solrCloud or standalone solr
// One, and only one, of Cloud or Standalone must be provided.
type SolrReference struct {
	// Reference of a solrCloud instance
	// +optional
	Cloud *SolrCloudReference `json:"cloud,omitempty"`

	// Reference of a standalone solr instance
	// +optional
	Standalone *StandaloneSolrReference `json:"standalone,omitempty"`

	// Settings to configure the SolrJ client used to request metrics from TLS enabled Solr pods
	// +optional
	SolrTLS *SolrTLSOptions `json:"solrTLS,omitempty"`

	// If Solr is secured, you'll need to provide credentials for the Prometheus exporter to authenticate via a
	// kubernetes.io/basic-auth secret which must contain a username and password. If basic auth is enabled on the
	// SolrCloud instance, the default secret (unless you are supplying your own) is named using the pattern:
	// <SOLR_CLOUD_NAME>-solrcloud-basic-auth. If using the security.json bootstrapped by the Solr operator,
	// then the username is "k8s-oper".
	// +optional
	BasicAuthSecret string `json:"basicAuthSecret,omitempty"`
}

func (sr *SolrReference) withDefaults(namespace string) (changed bool) {
	if sr.Cloud != nil {
		changed = sr.Cloud.withDefaults(namespace) || changed
	}
	return changed
}

// SolrCloudReference defines a reference to an internal or external solrCloud.
// Internal (to the kube cluster) clouds should be specified via the Name and Namespace options.
// External clouds should be specified by their Zookeeper connection information.
type SolrCloudReference struct {
	// The name of a solr cloud running within the kubernetes cluster
	// +optional
	Name string `json:"name,omitempty"`

	// The namespace of a solr cloud running within the kubernetes cluster
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// The ZK Connection information for a cloud, could be used for solr's running outside of the kube cluster
	// +optional
	ZookeeperConnectionInfo *ZookeeperConnectionInfo `json:"zkConnectionInfo,omitempty"`
}

func (scr *SolrCloudReference) withDefaults(namespace string) (changed bool) {
	if scr.Name != "" {
		if scr.Namespace == "" {
			scr.Namespace = namespace
			changed = true
		}
	}

	if scr.ZookeeperConnectionInfo != nil {
		changed = scr.ZookeeperConnectionInfo.withDefaults() || changed
	}
	return changed
}

// SolrPrometheusExporterStatus defines the observed state of SolrPrometheusExporter
type StandaloneSolrReference struct {
	// The address of the standalone solr
	Address string `json:"address"`
}

type CustomExporterKubeOptions struct {
	// SolrPodOptions defines the custom options for the solrPrometheusExporter pods.
	// +optional
	PodOptions *PodOptions `json:"podOptions,omitempty"`

	// DeploymentOptions defines the custom options for the solrPrometheusExporter Deployment.
	// +optional
	DeploymentOptions *DeploymentOptions `json:"deploymentOptions,omitempty"`

	// ServiceOptions defines the custom options for the solrPrometheusExporter Service.
	// +optional
	ServiceOptions *ServiceOptions `json:"serviceOptions,omitempty"`

	// ServiceOptions defines the custom options for the solrPrometheusExporter ConfigMap.
	// +optional
	ConfigMapOptions *ConfigMapOptions `json:"configMapOptions,omitempty"`
}

// SolrPrometheusExporterStatus defines the observed state of SolrPrometheusExporter
type SolrPrometheusExporterStatus struct {
	// An address the prometheus exporter can be connected to from within the Kube cluster
	// InternalAddress string `json:"internalAddress"`

	// An address the prometheus exporter can be connected to from outside of the Kube cluster
	// Will only be provided when an ingressUrl is provided for the cloud
	// +optional
	// ExternalAddress string `json:"externalAddress,omitempty"`

	// Is the prometheus exporter up and running
	Ready bool `json:"ready"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:resource:shortName=solrmetrics
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready",description="Whether the prometheus exporter is ready"
//+kubebuilder:printcolumn:name="Scrape Interval",type="integer",JSONPath=".spec.scrapeInterval",description="Scrape interval for metrics (in ms)"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SolrPrometheusExporter is the Schema for the solrprometheusexporters API
type SolrPrometheusExporter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SolrPrometheusExporterSpec   `json:"spec,omitempty"`
	Status SolrPrometheusExporterStatus `json:"status,omitempty"`
}

// WithDefaults set default values when not defined in the spec.
func (spe *SolrPrometheusExporter) WithDefaults() bool {
	return spe.Spec.withDefaults(spe.Namespace)
}

func (spe *SolrPrometheusExporter) SharedLabels() map[string]string {
	return spe.SharedLabelsWith(map[string]string{})
}

func (spe *SolrPrometheusExporter) SharedLabelsWith(labels map[string]string) map[string]string {
	newLabels := map[string]string{}

	if labels != nil {
		for k, v := range labels {
			newLabels[k] = v
		}
	}

	newLabels[SolrPrometheusExporterTechnologyLabel] = spe.Name
	return newLabels
}

// MetricsDeploymentName returns the name of the metrics deployment for the cloud
func (sc *SolrPrometheusExporter) MetricsDeploymentName() string {
	return fmt.Sprintf("%s-solr-metrics", sc.GetName())
}

// MetricsConfigMapName returns the name of the metrics service for the cloud
func (sc *SolrPrometheusExporter) MetricsConfigMapName() string {
	return fmt.Sprintf("%s-solr-metrics", sc.GetName())
}

// MetricsServiceName returns the name of the metrics service for the cloud
func (sc *SolrPrometheusExporter) MetricsServiceName() string {
	return fmt.Sprintf("%s-solr-metrics", sc.GetName())
}

func (sc *SolrPrometheusExporter) MetricsIngressPrefix() string {
	return fmt.Sprintf("%s-%s-solr-metrics", sc.Namespace, sc.Name)
}

func (sc *SolrPrometheusExporter) MetricsIngressUrl(ingressBaseUrl string) string {
	return fmt.Sprintf("%s.%s", sc.MetricsIngressPrefix(), ingressBaseUrl)
}

func (sc *SolrPrometheusExporter) BusyBoxImage() *ContainerImage {
	c := sc.Spec.BusyBoxImage
	if c == nil {
		c = &ContainerImage{}
		c.Repository = DefaultBusyBoxImageRepo
		c.Tag = DefaultBusyBoxImageVersion
		c.PullPolicy = DefaultPullPolicy
	}
	return c
}

//+kubebuilder:object:root=true

// SolrPrometheusExporterList contains a list of SolrPrometheusExporter
type SolrPrometheusExporterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrPrometheusExporter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrPrometheusExporter{}, &SolrPrometheusExporterList{})
}
