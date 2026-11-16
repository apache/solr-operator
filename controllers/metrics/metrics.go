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

// Package metrics holds the Prometheus metrics that the Solr Operator exposes about its own work, on top of the
// generic controller-runtime, workqueue, client-go and Go runtime metrics.
//
// The counters here are incremented by the controllers as things happen. Gauges that describe the current state
// of the managed resources are computed at scrape time from the operator's cache instead, by the collector in the
// controllers package, so that they never go stale when a resource is deleted.
package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Prefix of every metric exposed by the Solr Operator itself.
const Namespace = "solr_operator"

// Label values for the "result" label of ReconcilesTotal.
const (
	ReconcileResultSuccess      = "success"
	ReconcileResultError        = "error"
	ReconcileResultRequeue      = "requeue"
	ReconcileResultRequeueAfter = "requeue_after"
)

// ClusterOperationResult is how a cluster operation on a SolrCloud ended, for ClusterOperationsTotal.
type ClusterOperationResult string

const (
	// ClusterOperationCompleted means the operation finished its work.
	ClusterOperationCompleted ClusterOperationResult = "completed"
	// ClusterOperationFailed means the operation hit an error and was queued to be retried later.
	ClusterOperationFailed ClusterOperationResult = "failed"
	// ClusterOperationPaused means the operation ran out of time and was queued to be continued later, so that
	// other operations get a turn.
	ClusterOperationPaused ClusterOperationResult = "paused"
)

// Label values for the "result" label of SolrBackupRunsTotal.
const (
	BackupResultSucceeded = "succeeded"
	BackupResultFailed    = "failed"
)

// Label values for the "result" label of SolrAPIRequestsTotal.
const (
	SolrAPIResultSuccess = "success"
	SolrAPIResultError   = "error"
)

var (
	// ReconcilesTotal counts the reconciles of every individual Solr resource, by result. The controller-runtime
	// metrics only count per controller, this tells which resource is being reconciled, or failing.
	ReconcilesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "reconciles_total",
		Help: "Number of reconciles of each Solr resource, by result. \"error\" reconciles are retried with backoff, " +
			"\"requeue\" and \"requeue_after\" reconciles asked to be run again.",
	}, []string{"kind", "namespace", "name", "result"})

	// ClusterOperationsTotal counts the cluster operations that ended on each SolrCloud, by result.
	ClusterOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "solrcloud_cluster_operations_total",
		Help: "Number of cluster operations (RollingUpdate, ScalingUp, ScalingDown, BalanceReplicas, PVCExpansion) that " +
			"have ended on each SolrCloud, by result. \"completed\" operations finished their work, \"failed\" operations " +
			"hit an error and were queued to be retried later, \"paused\" operations ran out of time and were queued to " +
			"be continued later.",
	}, []string{"namespace", "name", "operation", "result"})

	// SolrBackupRunsTotal counts the finished backup runs of each SolrBackup, by result. A recurring SolrBackup
	// has many runs, a one-off SolrBackup has one.
	SolrBackupRunsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "solrbackup_runs_total",
		Help:      "Number of finished backup runs of each SolrBackup, by result (\"succeeded\" or \"failed\").",
	}, []string{"namespace", "name", "solrcloud", "result"})

	// SolrAPIRequestsTotal counts the requests the operator makes to the Solr API of each SolrCloud, by result.
	SolrAPIRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "solr_api_requests_total",
		Help: "Number of requests the operator has made to the Solr API of each SolrCloud, by result. An \"error\" " +
			"is a failed connection, a 4xx or 5xx response or an unreadable response.",
	}, []string{"namespace", "solrcloud", "result"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(ReconcilesTotal, ClusterOperationsTotal, SolrBackupRunsTotal, SolrAPIRequestsTotal)
}

// RecordClusterOperation counts a cluster operation that ended on the given SolrCloud.
func RecordClusterOperation(namespace string, solrCloud string, operation string, result ClusterOperationResult) {
	ClusterOperationsTotal.WithLabelValues(namespace, solrCloud, operation, string(result)).Inc()
}

// RecordBackupRun counts a finished run of the given SolrBackup.
func RecordBackupRun(namespace string, solrBackup string, solrCloud string, successful bool) {
	result := BackupResultFailed
	if successful {
		result = BackupResultSucceeded
	}
	SolrBackupRunsTotal.WithLabelValues(namespace, solrBackup, solrCloud, result).Inc()
}

// RecordSolrAPIRequest counts a request to the Solr API of the given SolrCloud.
func RecordSolrAPIRequest(namespace string, solrCloud string, err error) {
	result := SolrAPIResultSuccess
	if err != nil {
		result = SolrAPIResultError
	}
	SolrAPIRequestsTotal.WithLabelValues(namespace, solrCloud, result).Inc()
}

// InstrumentReconciler wraps a Reconciler so that every reconcile is counted in ReconcilesTotal, labelled with the
// kind, namespace and name of the reconciled resource and the result of the reconcile.
//
// When a reconcile finds that the resource is gone, read through the given reader (the operator's cache) as an
// object of the given type, the counter series of that resource are removed instead, so that deleted resources do
// not leave series behind for the lifetime of the operator.
func InstrumentReconciler(kind string, reader client.Reader, resourceType client.Object, reconciler reconcile.Reconciler) reconcile.Reconciler {
	return &instrumentedReconciler{kind: kind, reader: reader, resourceType: resourceType, Reconciler: reconciler}
}

type instrumentedReconciler struct {
	reconcile.Reconciler
	kind         string
	reader       client.Reader
	resourceType client.Object
}

func (r *instrumentedReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	result, err := r.Reconciler.Reconcile(ctx, req)
	if r.resourceIsGone(ctx, req) {
		ForgetResource(r.kind, req.Namespace, req.Name)
	} else {
		ReconcilesTotal.WithLabelValues(r.kind, req.Namespace, req.Name, reconcileResult(result, err)).Inc()
	}
	return result, err
}

func (r *instrumentedReconciler) resourceIsGone(ctx context.Context, req reconcile.Request) bool {
	resource := r.resourceType.DeepCopyObject().(client.Object)
	return errors.IsNotFound(r.reader.Get(ctx, req.NamespacedName, resource))
}

// ForgetResource removes the counter series of a resource that has been deleted.
func ForgetResource(kind string, namespace string, name string) {
	ReconcilesTotal.DeletePartialMatch(prometheus.Labels{"kind": kind, "namespace": namespace, "name": name})
	switch kind {
	case "SolrCloud":
		ClusterOperationsTotal.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "name": name})
		SolrAPIRequestsTotal.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "solrcloud": name})
	case "SolrBackup":
		SolrBackupRunsTotal.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "name": name})
	}
}

// reconcileResult classifies the outcome of a reconcile the same way controller-runtime does for its own
// controller_runtime_reconcile_total metric.
func reconcileResult(result reconcile.Result, err error) string {
	switch {
	case err != nil:
		return ReconcileResultError
	case result.RequeueAfter > 0:
		return ReconcileResultRequeueAfter
	case result.Requeue: //nolint:staticcheck // Requeue is still what the controllers return
		return ReconcileResultRequeue
	default:
		return ReconcileResultSuccess
	}
}
