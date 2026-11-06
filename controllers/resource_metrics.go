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

package controllers

import (
	"context"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/metrics"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// How long a scrape may spend reading the cache. Reads from a synced cache take microseconds, this only matters
// while the cache is still syncing after startup.
const resourceMetricsTimeout = 10 * time.Second

// Label values of solr_operator_solrbackup_state
const (
	backupStateRunning   = "running"
	backupStateSucceeded = "succeeded"
	backupStateFailed    = "failed"
)

// Label values of solr_operator_solrcloud_cluster_operations
const (
	clusterOpStateRunning = "running"
	clusterOpStateQueued  = "queued"
)

var (
	managedResourcesDesc = prometheus.NewDesc(metrics.Namespace+"_managed_resources",
		"Number of Solr resources managed by the operator, by kind (SolrCloud, SolrBackup, SolrPrometheusExporter) and namespace.",
		[]string{"kind", "namespace"}, nil)

	solrCloudInfoDesc = prometheus.NewDesc(metrics.Namespace+"_solrcloud_info",
		"Information about a SolrCloud, always 1. The version label is the version of Solr the SolrCloud is running.",
		[]string{"namespace", "name", "version"}, nil)
	solrCloudDesiredReplicasDesc = prometheus.NewDesc(metrics.Namespace+"_solrcloud_desired_replicas",
		"Number of Solr pods the SolrCloud asks for (spec.replicas).",
		[]string{"namespace", "name"}, nil)
	solrCloudReadyReplicasDesc = prometheus.NewDesc(metrics.Namespace+"_solrcloud_ready_replicas",
		"Number of Solr pods of the SolrCloud that are ready.",
		[]string{"namespace", "name"}, nil)
	solrCloudUpToDateReplicasDesc = prometheus.NewDesc(metrics.Namespace+"_solrcloud_up_to_date_replicas",
		"Number of Solr pods of the SolrCloud that run the latest pod spec.",
		[]string{"namespace", "name"}, nil)
	solrCloudReadyDesc = prometheus.NewDesc(metrics.Namespace+"_solrcloud_ready",
		"Whether all the Solr pods the SolrCloud asks for are ready (1) or not (0).",
		[]string{"namespace", "name"}, nil)
	solrCloudClusterOperationsDesc = prometheus.NewDesc(metrics.Namespace+"_solrcloud_cluster_operations",
		"Number of cluster operations on the SolrCloud, by operation (RollingUpdate, ScalingUp, ScalingDown, BalanceReplicas, PVCExpansion) "+
			"and state: \"running\" for the operation holding the cluster operation lock, \"queued\" for operations waiting to be retried. "+
			"Only present while such operations exist.",
		[]string{"namespace", "name", "operation", "state"}, nil)

	solrBackupStateDesc = prometheus.NewDesc(metrics.Namespace+"_solrbackup_state",
		"State of the latest run of the SolrBackup: 1 for its current state out of \"running\", \"succeeded\" and \"failed\", 0 for the other states.",
		[]string{"namespace", "name", "solrcloud", "state"}, nil)

	solrPrometheusExporterReadyDesc = prometheus.NewDesc(metrics.Namespace+"_solrprometheusexporter_ready",
		"Whether the SolrPrometheusExporter is ready (1) or not (0).",
		[]string{"namespace", "name", "solrcloud"}, nil)
)

// ResourceMetricsCollector exposes gauges describing the Solr resources that the operator manages. They are computed
// at scrape time from the operator's cache, so they reflect what the operator sees and never go stale when a
// resource is deleted. With several operator replicas every replica reports them, not just the leader.
type ResourceMetricsCollector struct {
	reader client.Reader
}

// NewResourceMetricsCollector creates a collector that reads the managed resources through the given reader,
// which should be the operator's cache.
func NewResourceMetricsCollector(reader client.Reader) *ResourceMetricsCollector {
	return &ResourceMetricsCollector{reader: reader}
}

// RegisterResourceMetrics registers the gauges describing the managed Solr resources with the registry behind the
// operator's metrics endpoint, reading the resources through the given reader (the operator's cache).
func RegisterResourceMetrics(reader client.Reader) error {
	return ctrlmetrics.Registry.Register(NewResourceMetricsCollector(reader))
}

func (c *ResourceMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- managedResourcesDesc
	ch <- solrCloudInfoDesc
	ch <- solrCloudDesiredReplicasDesc
	ch <- solrCloudReadyReplicasDesc
	ch <- solrCloudUpToDateReplicasDesc
	ch <- solrCloudReadyDesc
	ch <- solrCloudClusterOperationsDesc
	ch <- solrBackupStateDesc
	ch <- solrPrometheusExporterReadyDesc
}

func (c *ResourceMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), resourceMetricsTimeout)
	defer cancel()

	// kind -> namespace -> number of resources
	counts := map[string]map[string]int{}
	count := func(kind string, namespace string) {
		if counts[kind] == nil {
			counts[kind] = map[string]int{}
		}
		counts[kind][namespace]++
	}

	c.collectSolrClouds(ctx, ch, count)
	c.collectSolrBackups(ctx, ch, count)
	c.collectSolrPrometheusExporters(ctx, ch, count)

	for kind, namespaces := range counts {
		for namespace, n := range namespaces {
			ch <- prometheus.MustNewConstMetric(managedResourcesDesc, prometheus.GaugeValue, float64(n), kind, namespace)
		}
	}
}

func (c *ResourceMetricsCollector) collectSolrClouds(ctx context.Context, ch chan<- prometheus.Metric, count func(kind string, namespace string)) {
	solrClouds := &solrv1beta1.SolrCloudList{}
	if err := c.reader.List(ctx, solrClouds); err != nil {
		resourceMetricsLog().Error(err, "Could not list SolrClouds for the operator metrics")
		return
	}
	for i := range solrClouds.Items {
		solrCloud := &solrClouds.Items[i]
		count("SolrCloud", solrCloud.Namespace)
		labels := []string{solrCloud.Namespace, solrCloud.Name}

		desiredReplicas := int32(0)
		if solrCloud.Spec.Replicas != nil {
			desiredReplicas = *solrCloud.Spec.Replicas
		}
		ready := 0.0
		if solrCloud.Status.ReadyReplicas >= desiredReplicas {
			ready = 1
		}

		ch <- prometheus.MustNewConstMetric(solrCloudInfoDesc, prometheus.GaugeValue, 1, solrCloud.Namespace, solrCloud.Name, solrCloud.Status.Version)
		ch <- prometheus.MustNewConstMetric(solrCloudDesiredReplicasDesc, prometheus.GaugeValue, float64(desiredReplicas), labels...)
		ch <- prometheus.MustNewConstMetric(solrCloudReadyReplicasDesc, prometheus.GaugeValue, float64(solrCloud.Status.ReadyReplicas), labels...)
		ch <- prometheus.MustNewConstMetric(solrCloudUpToDateReplicasDesc, prometheus.GaugeValue, float64(solrCloud.Status.UpToDateNodes), labels...)
		ch <- prometheus.MustNewConstMetric(solrCloudReadyDesc, prometheus.GaugeValue, ready, labels...)

		c.collectClusterOperations(ctx, ch, solrCloud)
	}
}

// collectClusterOperations reports the cluster operations of a SolrCloud, which live in annotations on its StatefulSet.
func (c *ResourceMetricsCollector) collectClusterOperations(ctx context.Context, ch chan<- prometheus.Metric, solrCloud *solrv1beta1.SolrCloud) {
	statefulSet := &appsv1.StatefulSet{}
	if err := c.reader.Get(ctx, types.NamespacedName{Namespace: solrCloud.Namespace, Name: solrCloud.StatefulSetName()}, statefulSet); err != nil {
		if !errors.IsNotFound(err) {
			resourceMetricsLog().Error(err, "Could not get the StatefulSet of a SolrCloud for the operator metrics", "namespace", solrCloud.Namespace, "solrCloud", solrCloud.Name)
		}
		return
	}

	// operation -> state -> number of operations
	operations := map[SolrClusterOperationType]map[string]int{}
	add := func(operation SolrClusterOperationType, state string) {
		if operations[operation] == nil {
			operations[operation] = map[string]int{}
		}
		operations[operation][state]++
	}
	if clusterOp, err := GetCurrentClusterOp(statefulSet); err != nil {
		resourceMetricsLog().Error(err, "Could not read the cluster operation of a SolrCloud for the operator metrics", "namespace", solrCloud.Namespace, "solrCloud", solrCloud.Name)
	} else if clusterOp != nil {
		add(clusterOp.Operation, clusterOpStateRunning)
	}
	if queue, err := GetClusterOpRetryQueue(statefulSet); err != nil {
		resourceMetricsLog().Error(err, "Could not read the cluster operation retry queue of a SolrCloud for the operator metrics", "namespace", solrCloud.Namespace, "solrCloud", solrCloud.Name)
	} else {
		for _, queuedOp := range queue {
			add(queuedOp.Operation, clusterOpStateQueued)
		}
	}

	for operation, states := range operations {
		for state, n := range states {
			ch <- prometheus.MustNewConstMetric(solrCloudClusterOperationsDesc, prometheus.GaugeValue, float64(n), solrCloud.Namespace, solrCloud.Name, string(operation), state)
		}
	}
}

func (c *ResourceMetricsCollector) collectSolrBackups(ctx context.Context, ch chan<- prometheus.Metric, count func(kind string, namespace string)) {
	solrBackups := &solrv1beta1.SolrBackupList{}
	if err := c.reader.List(ctx, solrBackups); err != nil {
		resourceMetricsLog().Error(err, "Could not list SolrBackups for the operator metrics")
		return
	}
	for i := range solrBackups.Items {
		solrBackup := &solrBackups.Items[i]
		count("SolrBackup", solrBackup.Namespace)

		currentState := backupStateRunning
		if status := solrBackup.Status.IndividualSolrBackupStatus; status.Finished {
			if status.Successful != nil && *status.Successful {
				currentState = backupStateSucceeded
			} else {
				currentState = backupStateFailed
			}
		}
		for _, state := range []string{backupStateRunning, backupStateSucceeded, backupStateFailed} {
			value := 0.0
			if state == currentState {
				value = 1
			}
			ch <- prometheus.MustNewConstMetric(solrBackupStateDesc, prometheus.GaugeValue, value, solrBackup.Namespace, solrBackup.Name, solrBackup.Spec.SolrCloud, state)
		}
	}
}

func (c *ResourceMetricsCollector) collectSolrPrometheusExporters(ctx context.Context, ch chan<- prometheus.Metric, count func(kind string, namespace string)) {
	exporters := &solrv1beta1.SolrPrometheusExporterList{}
	if err := c.reader.List(ctx, exporters); err != nil {
		resourceMetricsLog().Error(err, "Could not list SolrPrometheusExporters for the operator metrics")
		return
	}
	for i := range exporters.Items {
		exporter := &exporters.Items[i]
		count("SolrPrometheusExporter", exporter.Namespace)

		solrCloud := ""
		if exporter.Spec.SolrReference.Cloud != nil {
			solrCloud = exporter.Spec.SolrReference.Cloud.Name
		}
		ready := 0.0
		if exporter.Status.Ready {
			ready = 1
		}
		ch <- prometheus.MustNewConstMetric(solrPrometheusExporterReadyDesc, prometheus.GaugeValue, ready, exporter.Namespace, exporter.Name, solrCloud)
	}
}

func resourceMetricsLog() logr.Logger {
	return ctrl.Log.WithName("metrics")
}
