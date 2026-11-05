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
	"testing"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
)

// podMonitorFixture builds a reconciler with a fake client holding the given SolrCloud, with the
// operator configured to use the Prometheus Operator CRDs.
func podMonitorFixture(t *testing.T, instance *solrv1beta1.SolrCloud, extraObjects ...client.Object) *SolrCloudReconciler {
	t.Helper()
	return podMonitorFixtureWithInterceptors(t, instance, interceptor.Funcs{}, extraObjects...)
}

// podMonitorFixtureWithInterceptors is podMonitorFixture with a fake client whose calls can be made to fail
func podMonitorFixtureWithInterceptors(t *testing.T, instance *solrv1beta1.SolrCloud, interceptors interceptor.Funcs, extraObjects ...client.Object) *SolrCloudReconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, solrv1beta1.AddToScheme(scheme))
	require.NoError(t, monitoringv1.AddToScheme(scheme))

	// Only main.go ever sets this, so it is false in tests unless a test turns it on
	UsePrometheusCRD(true)
	t.Cleanup(func() { UsePrometheusCRD(false) })

	return &SolrCloudReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(instance).WithObjects(extraObjects...).WithInterceptorFuncs(interceptors).Build(),
		Scheme:   scheme,
		Recorder: noOpEventRecorder{},
	}
}

// podMonitorCloud returns a defaulted SolrCloud, with a PodMonitor enabled when wanted
func podMonitorCloud(t *testing.T, podMonitor *solrv1beta1.SolrPodMonitorOptions) *solrv1beta1.SolrCloud {
	t.Helper()
	instance := &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: solrv1beta1.SolrCloudSpec{
			ZookeeperRef: &solrv1beta1.ZookeeperRef{
				ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{InternalConnectionString: "zoo-headless:2181"},
			},
			Prometheus: solrv1beta1.SolrPrometheusOptions{PodMonitor: podMonitor},
		},
	}
	instance.WithDefaults(logr.Discard())
	return instance
}

func getPodMonitor(t *testing.T, r *SolrCloudReconciler, instance *solrv1beta1.SolrCloud) (*monitoringv1.PodMonitor, error) {
	t.Helper()
	podMonitor := &monitoringv1.PodMonitor{}
	err := r.Get(context.Background(), types.NamespacedName{Name: instance.PodMonitorName(), Namespace: instance.Namespace}, podMonitor)
	return podMonitor, err
}

func TestPodMonitorIsCreatedAndOwnedByTheCloud(t *testing.T) {
	instance := podMonitorCloud(t, &solrv1beta1.SolrPodMonitorOptions{Enable: ptr.To(true), Interval: "30s"})
	r := podMonitorFixture(t, instance)

	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))

	podMonitor, err := getPodMonitor(t, r, instance)
	require.NoError(t, err, "The PodMonitor should have been created")
	assert.Equal(t, monitoringv1.Duration("30s"), podMonitor.Spec.PodMetricsEndpoints[0].Interval, "Wrong scrape interval on the created PodMonitor")

	// The owner reference is what deletes the PodMonitor along with the SolrCloud
	require.Len(t, podMonitor.OwnerReferences, 1, "The PodMonitor should be owned by the SolrCloud")
	assert.Equal(t, instance.Name, podMonitor.OwnerReferences[0].Name, "The PodMonitor should be owned by the SolrCloud")
	assert.Equal(t, "SolrCloud", podMonitor.OwnerReferences[0].Kind, "The PodMonitor should be owned by the SolrCloud")
}

func TestPodMonitorIsUpdatedWhenItDriftsFromTheSpec(t *testing.T) {
	instance := podMonitorCloud(t, &solrv1beta1.SolrPodMonitorOptions{Enable: ptr.To(true), Interval: "30s"})
	r := podMonitorFixture(t, instance)
	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))

	// Someone edited the PodMonitor behind the operator's back
	podMonitor, err := getPodMonitor(t, r, instance)
	require.NoError(t, err)
	podMonitor.Spec.PodMetricsEndpoints[0].Interval = monitoringv1.Duration("10m")
	podMonitor.Spec.PodMetricsEndpoints[0].Path = "/not/the/metrics/path"
	require.NoError(t, r.Update(context.Background(), podMonitor))

	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))

	podMonitor, err = getPodMonitor(t, r, instance)
	require.NoError(t, err)
	assert.Equal(t, monitoringv1.Duration("30s"), podMonitor.Spec.PodMetricsEndpoints[0].Interval, "The scrape interval should have been reconciled back")
	assert.Equal(t, util.SolrMetricsPath, podMonitor.Spec.PodMetricsEndpoints[0].Path, "The metrics path should have been reconciled back")
}

func TestPodMonitorIsDeletedWhenNoLongerEnabled(t *testing.T) {
	instance := podMonitorCloud(t, &solrv1beta1.SolrPodMonitorOptions{Enable: ptr.To(true)})
	r := podMonitorFixture(t, instance)
	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))
	_, err := getPodMonitor(t, r, instance)
	require.NoError(t, err, "The PodMonitor should have been created")

	// The user turns the PodMonitor off again
	instance.Spec.Prometheus.PodMonitor.Enable = ptr.To(false)
	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))

	_, err = getPodMonitor(t, r, instance)
	assert.True(t, errors.IsNotFound(err), "The PodMonitor should have been deleted, got error: %v", err)

	// Reconciling a cloud that never wanted a PodMonitor must not fail on the missing object
	instance.Spec.Prometheus.PodMonitor = nil
	assert.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))
}

func TestPodMonitorRequiresTheOperatorToUseThePrometheusCRDs(t *testing.T) {
	instance := podMonitorCloud(t, &solrv1beta1.SolrPodMonitorOptions{Enable: ptr.To(true)})
	r := podMonitorFixture(t, instance)

	// An operator that was not started with --prometheus-operator=true cannot create a PodMonitor, but
	// must keep managing the rest of the SolrCloud, warning through an event instead of failing.
	UsePrometheusCRD(false)
	recorder := record.NewFakeRecorder(5)
	r.Recorder = recorder

	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()),
		"A PodMonitor that cannot be created must not fail the reconcile of the SolrCloud")

	_, getErr := getPodMonitor(t, r, instance)
	assert.True(t, errors.IsNotFound(getErr), "No PodMonitor should have been created")

	require.Len(t, recorder.Events, 1, "The SolrCloud should have gotten an event about the missing CRDs")
	assert.Contains(t, <-recorder.Events, "MissingPrometheusCRD", "Wrong event for a PodMonitor that cannot be created")
}

func TestPodMonitorFailuresDoNotStallTheSolrCloud(t *testing.T) {
	instance := podMonitorCloud(t, &solrv1beta1.SolrPodMonitorOptions{Enable: ptr.To(true), Labels: map[string]string{"release": "not a valid label value!"}})
	// The SolrCloud schema does not validate the labels, so the API server rejects the PodMonitor instead
	rejectPodMonitors := interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if podMonitor, isPodMonitor := obj.(*monitoringv1.PodMonitor); isPodMonitor {
				return errors.NewInvalid(monitoringv1.SchemeGroupVersion.WithKind(monitoringv1.PodMonitorsKind).GroupKind(), podMonitor.Name, nil)
			}
			return c.Create(ctx, obj, opts...)
		},
	}
	r := podMonitorFixtureWithInterceptors(t, instance, rejectPodMonitors)
	recorder := record.NewFakeRecorder(5)
	r.Recorder = recorder

	// Metrics are not worth stalling the Solr cluster over, so the failure is reported and retried instead
	requeueOrNot := reconcile.Result{}
	r.reconcilePodMonitorBestEffort(context.Background(), instance, logr.Discard(), &requeueOrNot)

	_, getErr := getPodMonitor(t, r, instance)
	assert.True(t, errors.IsNotFound(getErr), "The rejected PodMonitor should not exist")
	assert.Positive(t, requeueOrNot.RequeueAfter, "The SolrCloud should be requeued to retry the PodMonitor")
	require.Len(t, recorder.Events, 1, "The SolrCloud should have gotten an event about the failed PodMonitor")
	assert.Contains(t, <-recorder.Events, "PodMonitorError", "Wrong event for a PodMonitor that cannot be created")
}

func TestPrometheusCRDInstalledChecksTheCluster(t *testing.T) {
	podMonitor := monitoringv1.SchemeGroupVersion.WithKind(monitoringv1.PodMonitorsKind)

	// A cluster without the Prometheus Operator CRDs. Watching PodMonitors there would make the
	// manager give up after its cache sync timeout, crash looping the operator.
	withoutCRDs := meta.NewDefaultRESTMapper([]schema.GroupVersion{monitoringv1.SchemeGroupVersion})
	assert.False(t, PrometheusCRDInstalled(withoutCRDs, logr.Discard()), "PodMonitors cannot be managed without the PodMonitor CRD")

	withCRDs := meta.NewDefaultRESTMapper([]schema.GroupVersion{monitoringv1.SchemeGroupVersion})
	withCRDs.Add(podMonitor, meta.RESTScopeNamespace)
	assert.True(t, PrometheusCRDInstalled(withCRDs, logr.Discard()), "PodMonitors can be managed when the PodMonitor CRD is installed")
}

func TestPodMonitorIsNotTouchedForCloudsThatDoNotWantOne(t *testing.T) {
	instance := podMonitorCloud(t, nil)
	r := podMonitorFixture(t, instance)

	require.NoError(t, r.reconcilePodMonitor(context.Background(), instance, logr.Discard()))

	podMonitors := &monitoringv1.PodMonitorList{}
	require.NoError(t, r.List(context.Background(), podMonitors, client.InNamespace(instance.Namespace)))
	assert.Empty(t, podMonitors.Items, "No PodMonitor should exist for a SolrCloud that does not ask for one")
}
