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

package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type stubReconciler struct {
	result reconcile.Result
	err    error
}

func (s stubReconciler) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return s.result, s.err
}

// The wrapper only needs to know whether the resource exists, so any object type does for the test.
func readerWith(objects ...client.Object) client.Reader {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
}

func TestInstrumentReconcilerCountsEveryResult(t *testing.T) {
	ReconcilesTotal.Reset()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "cloud"}}
	reader := readerWith(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cloud"}})

	cases := map[string]stubReconciler{
		ReconcileResultSuccess:      {},
		ReconcileResultError:        {err: errors.New("boom")},
		ReconcileResultRequeue:      {result: reconcile.Result{Requeue: true}}, //nolint:staticcheck
		ReconcileResultRequeueAfter: {result: reconcile.Result{RequeueAfter: time.Second}},
	}
	for expected, stub := range cases {
		result, err := InstrumentReconciler("SolrCloud", reader, &corev1.ConfigMap{}, stub).Reconcile(context.Background(), req)
		assert.Equal(t, stub.result, result, "the wrapper must pass the result through")
		assert.Equal(t, stub.err, err, "the wrapper must pass the error through")
		assert.Equal(t, float64(1), counterValue(t, ReconcilesTotal, "SolrCloud", "ns", "cloud", expected), "count for result %q", expected)
	}

	// An error wins over a requeue, like it does in controller-runtime
	_, _ = InstrumentReconciler("SolrCloud", reader, &corev1.ConfigMap{}, stubReconciler{result: reconcile.Result{RequeueAfter: time.Second}, err: errors.New("boom")}).Reconcile(context.Background(), req)
	assert.Equal(t, float64(2), counterValue(t, ReconcilesTotal, "SolrCloud", "ns", "cloud", ReconcileResultError))
	assert.Equal(t, float64(1), counterValue(t, ReconcilesTotal, "SolrCloud", "ns", "cloud", ReconcileResultRequeueAfter))
}

func TestInstrumentReconcilerForgetsDeletedResources(t *testing.T) {
	ReconcilesTotal.Reset()
	ClusterOperationsTotal.Reset()
	SolrAPIRequestsTotal.Reset()
	SolrBackupRunsTotal.Reset()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "cloud"}}

	// Series for the cloud, for a backup of it, and for an unrelated cloud
	RecordClusterOperation("ns", "cloud", "RollingUpdate", ClusterOperationCompleted)
	RecordSolrAPIRequest("ns", "cloud", nil)
	RecordBackupRun("ns", "backup", "cloud", true)
	RecordClusterOperation("ns", "other", "RollingUpdate", ClusterOperationCompleted)
	ReconcilesTotal.WithLabelValues("SolrCloud", "ns", "cloud", ReconcileResultSuccess).Inc()
	ReconcilesTotal.WithLabelValues("SolrBackup", "ns", "cloud", ReconcileResultSuccess).Inc()

	// A reconcile of the cloud while it is gone from the cache
	_, _ = InstrumentReconciler("SolrCloud", readerWith(), &corev1.ConfigMap{}, stubReconciler{}).Reconcile(context.Background(), req)

	assert.Equal(t, 0, seriesCount(ReconcilesTotal, prometheus.Labels{"kind": "SolrCloud", "namespace": "ns", "name": "cloud"}), "the reconciles of the deleted cloud are forgotten, and the reconcile of its deletion is not counted")
	assert.Equal(t, 0, seriesCount(ClusterOperationsTotal, prometheus.Labels{"namespace": "ns", "name": "cloud"}), "the cluster operations of the deleted cloud are forgotten")
	assert.Equal(t, 0, seriesCount(SolrAPIRequestsTotal, prometheus.Labels{"namespace": "ns", "solrcloud": "cloud"}), "the Solr API requests of the deleted cloud are forgotten")
	assert.Equal(t, 1, seriesCount(ReconcilesTotal, prometheus.Labels{"kind": "SolrBackup", "namespace": "ns", "name": "cloud"}), "a backup with the same name is a different resource")
	assert.Equal(t, 1, seriesCount(SolrBackupRunsTotal, prometheus.Labels{"namespace": "ns", "name": "backup"}), "backups of the deleted cloud keep their series")
	assert.Equal(t, 1, seriesCount(ClusterOperationsTotal, prometheus.Labels{"namespace": "ns", "name": "other"}), "other clouds keep their series")
}

// seriesCount counts the series of a counter that carry the given labels.
func seriesCount(vec *prometheus.CounterVec, labels prometheus.Labels) int {
	count := 0
	metrics := make(chan prometheus.Metric, 100)
	vec.Collect(metrics)
	close(metrics)
	for metric := range metrics {
		written := &dto.Metric{}
		if metric.Write(written) != nil {
			continue
		}
		matches := 0
		for _, label := range written.GetLabel() {
			if value, wanted := labels[label.GetName()]; wanted && value == label.GetValue() {
				matches++
			}
		}
		if matches == len(labels) {
			count++
		}
	}
	return count
}

func TestRecordHelpers(t *testing.T) {
	ClusterOperationsTotal.Reset()
	SolrBackupRunsTotal.Reset()
	SolrAPIRequestsTotal.Reset()

	RecordClusterOperation("ns", "cloud", "RollingUpdate", ClusterOperationCompleted)
	RecordClusterOperation("ns", "cloud", "RollingUpdate", ClusterOperationFailed)
	RecordClusterOperation("ns", "cloud", "RollingUpdate", ClusterOperationFailed)
	assert.Equal(t, float64(1), counterValue(t, ClusterOperationsTotal, "ns", "cloud", "RollingUpdate", "completed"))
	assert.Equal(t, float64(2), counterValue(t, ClusterOperationsTotal, "ns", "cloud", "RollingUpdate", "failed"))

	RecordBackupRun("ns", "backup", "cloud", true)
	RecordBackupRun("ns", "backup", "cloud", false)
	RecordBackupRun("ns", "backup", "cloud", false)
	assert.Equal(t, float64(1), counterValue(t, SolrBackupRunsTotal, "ns", "backup", "cloud", "succeeded"))
	assert.Equal(t, float64(2), counterValue(t, SolrBackupRunsTotal, "ns", "backup", "cloud", "failed"))

	RecordSolrAPIRequest("ns", "cloud", nil)
	RecordSolrAPIRequest("ns", "cloud", errors.New("connection refused"))
	assert.Equal(t, float64(1), counterValue(t, SolrAPIRequestsTotal, "ns", "cloud", "success"))
	assert.Equal(t, float64(1), counterValue(t, SolrAPIRequestsTotal, "ns", "cloud", "error"))
}

func counterValue(t *testing.T, vec *prometheus.CounterVec, labelValues ...string) float64 {
	t.Helper()
	counter, err := vec.GetMetricWithLabelValues(labelValues...)
	require.NoError(t, err)
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	return metric.GetCounter().GetValue()
}
