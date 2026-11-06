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
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	zkApi "github.com/pravega/zookeeper-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// errSolrUnreachable simulates the transient loss of connectivity to Solr that stalled cluster
// operations before they were guaranteed to be requeued.
var errSolrUnreachable = errors.New("simulated i/o timeout talking to Solr")

type unreachableSolrTransport struct{}

func (unreachableSolrTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errSolrUnreachable
}

// makeSolrUnreachable forces every Solr HTTP call to fail. The failing client goes in the mTLS slot,
// which the Solr API calls prefer over the default client. Only main.go ever fills that slot, so it
// is nil in tests and resetting it to nil restores exactly what was there before.
func makeSolrUnreachable(t *testing.T) {
	t.Helper()
	solr_api.SetMTLSHttpClient(&http.Client{Transport: unreachableSolrTransport{}})
	t.Cleanup(func() {
		solr_api.SetMTLSHttpClient(nil)
	})
}

// clusterOpFixture builds the state of a SolrCloud with 4 running pods, a requested size of 3 pods
// and the given cluster operation already locked in on its StatefulSet.
func clusterOpFixture(t *testing.T, clusterOp SolrClusterOp) (*SolrCloudReconciler, *solrv1beta1.SolrCloud, *appsv1.StatefulSet) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := solrv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := zkApi.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	instance := &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: solrv1beta1.SolrCloudSpec{
			Replicas: pointer.Int32(3),
			ZookeeperRef: &solrv1beta1.ZookeeperRef{
				ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
					InternalConnectionString: "zoo-headless:2181",
					ChRoot:                   "/example",
				},
			},
		},
	}
	// Apply the defaults up-front, otherwise the first reconcile would just persist them and requeue.
	instance.WithDefaults(logr.Discard())

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.StatefulSetName(),
			Namespace: instance.Namespace,
			UID:       "statefulset-uid",
			Annotations: map[string]string{
				util.ClusterOpsLockAnnotation: clusterOpAnnotation(t, clusterOp),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: pointer.Int32(4),
			Selector: &metav1.LabelSelector{MatchLabels: solrPodLabels(instance)},
		},
		Status: appsv1.StatefulSetStatus{Replicas: 4, ReadyReplicas: 4, UpdatedReplicas: 4},
	}

	objects := []client.Object{instance, statefulSet}
	// One pod per StatefulSet replica, i.e. including the pod that is about to be scaled down.
	for i := 0; i < int(*statefulSet.Spec.Replicas); i++ {
		objects = append(objects, readySolrPod(instance, statefulSet, instance.GetSolrPodName(i)))
	}

	r := &SolrCloudReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objects...).
			WithStatusSubresource(&solrv1beta1.SolrCloud{}, &appsv1.StatefulSet{}, &corev1.Pod{}).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(64),
	}
	return r, instance, statefulSet
}

func clusterOpAnnotation(t *testing.T, op SolrClusterOp) string {
	t.Helper()
	bytes, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

// solrPodLabels returns the labels that the operator uses to find the pods of a SolrCloud.
func solrPodLabels(instance *solrv1beta1.SolrCloud) map[string]string {
	labels := instance.SharedLabels()
	labels["technology"] = solrv1beta1.SolrTechnologyLabel
	return labels
}

// readySolrPod builds a ready Solr pod owned by the given StatefulSet.
func readySolrPod(instance *solrv1beta1.SolrCloud, statefulSet *appsv1.StatefulSet, podName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            podName,
			Namespace:       instance.Namespace,
			Labels:          solrPodLabels(instance),
			OwnerReferences: []metav1.OwnerReference{{UID: statefulSet.UID, Name: statefulSet.Name}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: util.SolrNodeContainer, Image: instance.Spec.SolrImage.ToImageName()}},
		},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: util.SolrNodeContainer, Started: pointer.Bool(true)}},
		},
	}
}

// TestClusterOpRequeuedWhenSolrIsUnreachable verifies that a cluster operation that fails because
// Solr cannot be reached is requeued, so that the operation continues once Solr is reachable again.
// Regression test for https://github.com/apache/solr-operator/issues/707, where a transiently failed
// scale down was never retried because nothing triggered another reconcile.
func TestClusterOpRequeuedWhenSolrIsUnreachable(t *testing.T) {
	makeSolrUnreachable(t)
	r, instance, statefulSet := clusterOpFixture(t, SolrClusterOp{
		Operation:     ScaleDownLock,
		Metadata:      "3",
		LastStartTime: metav1.Now(),
	})

	ctx := log.IntoContext(context.Background(), logr.Discard())
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}})
	if err != nil {
		t.Fatalf("did not expect the reconcile to fail, got %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Errorf("expected the failed scale-down to be requeued, got %+v", result)
	}

	// The operation should still hold the lock, so that it continues where it left off.
	if err = r.Get(ctx, client.ObjectKeyFromObject(statefulSet), statefulSet); err != nil {
		t.Fatal(err)
	}
	clusterOp, err := GetCurrentClusterOp(statefulSet)
	if err != nil {
		t.Fatal(err)
	}
	if clusterOp == nil || clusterOp.Operation != ScaleDownLock {
		t.Errorf("expected the scale-down lock to still be held, got %+v", clusterOp)
	}
	if *statefulSet.Spec.Replicas != 4 {
		t.Errorf("expected the StatefulSet to not be scaled down while Solr is unreachable, got %d replicas", *statefulSet.Spec.Replicas)
	}
}

// TestClusterOpQueuedForRetryAfterTimeoutWhenSolrIsUnreachable verifies that a cluster operation
// that keeps failing is eventually given up on, queued for a later retry and its lock released, so
// that other cluster operations are not blocked forever.
func TestClusterOpQueuedForRetryAfterTimeoutWhenSolrIsUnreachable(t *testing.T) {
	makeSolrUnreachable(t)
	r, instance, statefulSet := clusterOpFixture(t, SolrClusterOp{
		Operation:     ScaleDownLock,
		Metadata:      "3",
		LastStartTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
	})

	ctx := log.IntoContext(context.Background(), logr.Discard())
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}}); err != nil {
		t.Fatalf("did not expect the reconcile to fail, got %v", err)
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(statefulSet), statefulSet); err != nil {
		t.Fatal(err)
	}
	if clusterOp, err := GetCurrentClusterOp(statefulSet); err != nil {
		t.Fatal(err)
	} else if clusterOp != nil {
		t.Errorf("expected the scale-down lock to be released after the operation timed out, got %+v", clusterOp)
	}
	queue, err := GetClusterOpRetryQueue(statefulSet)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Operation != ScaleDownLock {
		t.Errorf("expected the scale-down to be queued for a later retry, got %+v", queue)
	}
}

// TestFailingLongRunningClusterOpQueuedForRetryAfterAMinute verifies that a cluster operation that
// is allowed to run for a long time (here a PVC expansion) is queued for a later retry as soon as it
// has been failing for a minute, instead of holding its lock for the full long timeout.
func TestFailingLongRunningClusterOpQueuedForRetryAfterAMinute(t *testing.T) {
	r, instance, statefulSet := clusterOpFixture(t, SolrClusterOp{
		Operation:     PvcExpansionLock,
		Metadata:      "not-a-size",
		LastStartTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
	})

	ctx := log.IntoContext(context.Background(), logr.Discard())
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}})
	if err != nil {
		t.Fatalf("did not expect the reconcile to fail, got %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Errorf("expected the failed PVC expansion to be requeued, got %+v", result)
	}

	if err = r.Get(ctx, client.ObjectKeyFromObject(statefulSet), statefulSet); err != nil {
		t.Fatal(err)
	}
	if clusterOp, opErr := GetCurrentClusterOp(statefulSet); opErr != nil {
		t.Fatal(opErr)
	} else if clusterOp != nil {
		t.Errorf("expected the PVC expansion lock to be released after a minute of errors, got %+v", clusterOp)
	}
	queue, err := GetClusterOpRetryQueue(statefulSet)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Operation != PvcExpansionLock {
		t.Errorf("expected the PVC expansion to be queued for a later retry, got %+v", queue)
	}
}
