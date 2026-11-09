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
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestResourceMetricsCollectorReportsManagedResources(t *testing.T) {
	runningUpdate, err := clusterOpAnnotations(&SolrClusterOp{Operation: UpdateLock}, []SolrClusterOp{{Operation: ScaleDownLock}, {Operation: ScaleDownLock}, {Operation: BalanceReplicasLock}})
	require.NoError(t, err)

	objects := []client.Object{
		// A healthy SolrCloud with nothing going on
		&solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "healthy"},
			Spec:       solrv1beta1.SolrCloudSpec{Replicas: pointer.Int32(3)},
			Status:     solrv1beta1.SolrCloudStatus{Version: "9.9.0", ReadyReplicas: 3, UpToDateNodes: 3},
		},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "healthy-solrcloud"}},
		// A SolrCloud mid rolling update, with a pod down and operations queued behind the update
		&solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "updating"},
			Spec:       solrv1beta1.SolrCloudSpec{Replicas: pointer.Int32(4)},
			Status:     solrv1beta1.SolrCloudStatus{Version: "9.8.0", ReadyReplicas: 3, UpToDateNodes: 1},
		},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "updating-solrcloud", Annotations: runningUpdate}},
		// A SolrCloud in another namespace whose StatefulSet does not exist yet, and whose defaults are not
		// persisted yet, so that spec.replicas falls back to the API default
		&solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b", Name: "new"},
		},
		// Backups in every state
		&solrv1beta1.SolrBackup{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "running"},
			Spec:       solrv1beta1.SolrBackupSpec{SolrCloud: "healthy"},
		},
		&solrv1beta1.SolrBackup{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "succeeded"},
			Spec:       solrv1beta1.SolrBackupSpec{SolrCloud: "healthy"},
			Status:     solrv1beta1.SolrBackupStatus{IndividualSolrBackupStatus: solrv1beta1.IndividualSolrBackupStatus{Finished: true, Successful: pointer.Bool(true)}},
		},
		&solrv1beta1.SolrBackup{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b", Name: "failed"},
			Spec:       solrv1beta1.SolrBackupSpec{SolrCloud: "new"},
			Status:     solrv1beta1.SolrBackupStatus{IndividualSolrBackupStatus: solrv1beta1.IndividualSolrBackupStatus{Finished: true, Successful: pointer.Bool(false)}},
		},
		// Exporters, one for a SolrCloud and one for a standalone Solr
		&solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "exporter"},
			Spec:       solrv1beta1.SolrPrometheusExporterSpec{SolrReference: solrv1beta1.SolrReference{Cloud: &solrv1beta1.SolrCloudReference{Name: "healthy"}}},
			Status:     solrv1beta1.SolrPrometheusExporterStatus{Ready: true},
		},
		&solrv1beta1.SolrPrometheusExporter{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b", Name: "standalone-exporter"},
			Spec:       solrv1beta1.SolrPrometheusExporterSpec{SolrReference: solrv1beta1.SolrReference{Standalone: &solrv1beta1.StandaloneSolrReference{Address: "http://solr:8983"}}},
		},
	}

	gathered := gatherResourceMetrics(t, fake.NewClientBuilder().WithScheme(resourceMetricsScheme(t)).WithObjects(objects...).Build())

	assert.Equal(t, map[string]float64{
		`solr_operator_managed_resources{kind="SolrBackup",namespace="ns-a"}`:             2,
		`solr_operator_managed_resources{kind="SolrBackup",namespace="ns-b"}`:             1,
		`solr_operator_managed_resources{kind="SolrCloud",namespace="ns-a"}`:              2,
		`solr_operator_managed_resources{kind="SolrCloud",namespace="ns-b"}`:              1,
		`solr_operator_managed_resources{kind="SolrPrometheusExporter",namespace="ns-a"}`: 1,
		`solr_operator_managed_resources{kind="SolrPrometheusExporter",namespace="ns-b"}`: 1,

		`solr_operator_solrcloud_info{name="healthy",namespace="ns-a",version="9.9.0"}`:  1,
		`solr_operator_solrcloud_info{name="updating",namespace="ns-a",version="9.8.0"}`: 1,
		`solr_operator_solrcloud_info{name="new",namespace="ns-b",version=""}`:           1,

		`solr_operator_solrcloud_desired_replicas{name="healthy",namespace="ns-a"}`:  3,
		`solr_operator_solrcloud_desired_replicas{name="updating",namespace="ns-a"}`: 4,
		`solr_operator_solrcloud_desired_replicas{name="new",namespace="ns-b"}`:      3,

		`solr_operator_solrcloud_ready_replicas{name="healthy",namespace="ns-a"}`:  3,
		`solr_operator_solrcloud_ready_replicas{name="updating",namespace="ns-a"}`: 3,
		`solr_operator_solrcloud_ready_replicas{name="new",namespace="ns-b"}`:      0,

		`solr_operator_solrcloud_up_to_date_replicas{name="healthy",namespace="ns-a"}`:  3,
		`solr_operator_solrcloud_up_to_date_replicas{name="updating",namespace="ns-a"}`: 1,
		`solr_operator_solrcloud_up_to_date_replicas{name="new",namespace="ns-b"}`:      0,

		`solr_operator_solrcloud_ready{name="healthy",namespace="ns-a"}`:  1,
		`solr_operator_solrcloud_ready{name="updating",namespace="ns-a"}`: 0,
		`solr_operator_solrcloud_ready{name="new",namespace="ns-b"}`:      0,

		`solr_operator_solrcloud_cluster_operations{name="updating",namespace="ns-a",operation="RollingUpdate",state="running"}`:  1,
		`solr_operator_solrcloud_cluster_operations{name="updating",namespace="ns-a",operation="ScalingDown",state="queued"}`:     2,
		`solr_operator_solrcloud_cluster_operations{name="updating",namespace="ns-a",operation="BalanceReplicas",state="queued"}`: 1,

		`solr_operator_solrbackup_state{name="running",namespace="ns-a",solrcloud="healthy",state="running"}`:     1,
		`solr_operator_solrbackup_state{name="running",namespace="ns-a",solrcloud="healthy",state="succeeded"}`:   0,
		`solr_operator_solrbackup_state{name="running",namespace="ns-a",solrcloud="healthy",state="failed"}`:      0,
		`solr_operator_solrbackup_state{name="succeeded",namespace="ns-a",solrcloud="healthy",state="running"}`:   0,
		`solr_operator_solrbackup_state{name="succeeded",namespace="ns-a",solrcloud="healthy",state="succeeded"}`: 1,
		`solr_operator_solrbackup_state{name="succeeded",namespace="ns-a",solrcloud="healthy",state="failed"}`:    0,
		`solr_operator_solrbackup_state{name="failed",namespace="ns-b",solrcloud="new",state="running"}`:          0,
		`solr_operator_solrbackup_state{name="failed",namespace="ns-b",solrcloud="new",state="succeeded"}`:        0,
		`solr_operator_solrbackup_state{name="failed",namespace="ns-b",solrcloud="new",state="failed"}`:           1,

		`solr_operator_solrprometheusexporter_ready{name="exporter",namespace="ns-a",solrcloud="healthy"}`:     1,
		`solr_operator_solrprometheusexporter_ready{name="standalone-exporter",namespace="ns-b",solrcloud=""}`: 0,
	}, gathered)
}

func TestResourceMetricsCollectorWithNoResources(t *testing.T) {
	gathered := gatherResourceMetrics(t, fake.NewClientBuilder().WithScheme(resourceMetricsScheme(t)).Build())
	assert.Empty(t, gathered, "no resources means no series, not zero-valued ones")
}

func TestResourceMetricsCollectorSurvivesReadErrors(t *testing.T) {
	reader := fake.NewClientBuilder().WithScheme(resourceMetricsScheme(t)).
		WithObjects(&solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "cloud"}}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, isBackupList := list.(*solrv1beta1.SolrBackupList); isBackupList {
					return errors.New("the cache has not started yet")
				}
				return c.List(ctx, list, opts...)
			},
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return errors.New("the cache has not started yet")
			},
		}).Build()

	gathered := gatherResourceMetrics(t, reader)

	assert.Equal(t, float64(1), gathered[`solr_operator_managed_resources{kind="SolrCloud",namespace="ns-a"}`], "the SolrClouds that could be read are still reported")
	for series := range gathered {
		assert.NotContains(t, series, "solrbackup", "nothing is reported for the kind that could not be listed")
		assert.NotContains(t, series, "cluster_operations", "nothing is reported about cluster operations when the StatefulSet could not be read")
	}
}

func clusterOpAnnotations(current *SolrClusterOp, queue []SolrClusterOp) (map[string]string, error) {
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
	if current != nil {
		if err := setClusterOpLock(statefulSet, *current); err != nil {
			return nil, err
		}
	}
	if err := setClusterOpRetryQueue(statefulSet, queue); err != nil {
		return nil, err
	}
	return statefulSet.Annotations, nil
}

func resourceMetricsScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, solrv1beta1.AddToScheme(scheme))
	return scheme
}

// gatherResourceMetrics scrapes a ResourceMetricsCollector reading through the given reader, and returns every
// series as `name{label="value",...}` -> value.
func gatherResourceMetrics(t *testing.T, reader client.Reader) map[string]float64 {
	t.Helper()
	registry := prometheus.NewPedanticRegistry()
	require.NoError(t, registry.Register(NewResourceMetricsCollector(reader)))

	families, err := registry.Gather()
	require.NoError(t, err)
	return metricSeries(families)
}

// metricSeries flattens gathered metric families into `name{label="value",...}` -> value, for the gauges and
// counters prefixed with the Solr Operator metrics namespace.
func metricSeries(families []*dto.MetricFamily) map[string]float64 {
	series := map[string]float64{}
	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), metrics.Namespace+"_") {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make([]string, 0, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels = append(labels, fmt.Sprintf("%s=%q", label.GetName(), label.GetValue()))
			}
			sort.Strings(labels)
			value := metric.GetGauge().GetValue()
			if metric.GetCounter() != nil {
				value = metric.GetCounter().GetValue()
			}
			series[fmt.Sprintf("%s{%s}", family.GetName(), strings.Join(labels, ","))] = value
		}
	}
	return series
}
