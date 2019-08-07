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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	SolrPrometheusExporterTechnologyLabel = "solr-prometheus-exporter"
)

// SolrPrometheusExporterSpec defines the desired state of SolrPrometheusExporter
type SolrPrometheusExporterSpec struct {
	// Reference of the Solr instance to collect metrics for
	SolrReference `json:"solrReference"`

	// Image of Solr Prometheus Exporter to run.
	// +optional
	Image *ContainerImage `json:"image,omitempty"`

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
}

func (ps *SolrPrometheusExporterSpec) withDefaults(namespace string) (changed bool) {
	changed = ps.SolrReference.withDefaults(namespace) || changed

	if ps.Image == nil {
		ps.Image = &ContainerImage{}
	}
	changed = ps.Image.withDefaults(DefaultSolrRepo, DefaultSolrVersion, DefaultPullPolicy) || changed

	if ps.NumThreads == 0 {
		ps.NumThreads = 1
		changed = true
	}

	if ps.ScrapeInterval == 0 {
		ps.ScrapeInterval = 60
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
}

func (sr *SolrReference) withDefaults(namespace string) (changed bool) {
	if sr.Cloud == nil {
		changed = sr.Cloud.withDefaults(namespace) || changed
	}
	return changed
}

// SolrCloudReference defines a reference to an internal or external solrCloud
type SolrCloudReference struct {
	// The name of a solr cloud running within the kubernetes cluster
	// +optional
	KubeSolr *types.NamespacedName `json:",inline"`

	// The ZK Connection information for a cloud, could be used for solr's outside of the kube cluster
	// +optional
	ZookeeperConnectionInfo *ZookeeperConnectionInfo `json:"zkConnectionInfo,omitempty"`
}

func (scr *SolrCloudReference) withDefaults(namespace string) (changed bool) {
	if scr.KubeSolr != nil {
		if scr.KubeSolr.Namespace == "" {
			scr.KubeSolr.Namespace = namespace
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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SolrPrometheusExporter is the Schema for the solrprometheusexporters API
// +k8s:openapi-gen=true
// +kubebuilder:resource:shortName=solrmetrics
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready",description="Whether the prometheus exporter is ready"
// +kubebuilder:printcolumn:name="Scrape Interval",type="integer",JSONPath=".spec.scrapeInterval",description="Scrape interval for metrics (in ms)"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SolrPrometheusExporterList contains a list of SolrPrometheusExporter
type SolrPrometheusExporterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolrPrometheusExporter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrPrometheusExporter{}, &SolrPrometheusExporterList{})
}
