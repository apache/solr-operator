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
	"strings"
	"testing"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// errForcedPatchFailure is returned by the fake client to force the status patch to fail,
// so that the "could not patch readiness condition" event path is exercised.
var errForcedPatchFailure = errors.New("forced patch failure")

// requireEvent asserts that one event was recorded whose type and reason match.
// record.FakeRecorder formats each event as "<eventtype> <reason> <message>".
func requireEvent(t *testing.T, rec *record.FakeRecorder, wantType, wantReason string) {
	t.Helper()
	select {
	case got := <-rec.Events:
		if !strings.HasPrefix(got, wantType+" "+wantReason+" ") {
			t.Errorf("expected event of type %q with reason %q, got %q", wantType, wantReason, got)
		}
	default:
		t.Fatalf("expected an event with reason %q to be recorded, but none was", wantReason)
	}
}

// requireNoEvent asserts that no event is currently buffered on the recorder.
func requireNoEvent(t *testing.T, rec *record.FakeRecorder) {
	t.Helper()
	select {
	case got := <-rec.Events:
		t.Fatalf("expected no event to be recorded, but got %q", got)
	default:
	}
}

// solrCloudWithStorageRequest builds a minimal SolrCloud requesting the given persistent data size.
func solrCloudWithStorageRequest(size string) *solrv1beta1.SolrCloud {
	return &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
		Spec: solrv1beta1.SolrCloudSpec{
			StorageOptions: solrv1beta1.SolrDataStorageOptions{
				PersistentStorage: &solrv1beta1.SolrPersistentDataStorageOptions{
					PersistentVolumeClaimTemplate: solrv1beta1.PersistentVolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse(size),
								},
							},
						},
					},
				},
			},
		},
	}
}

// podWithReadinessGate builds a Pod that advertises the given readiness gate (and no status yet).
func podWithReadinessGate(condType corev1.PodConditionType) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test"},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{{ConditionType: condType}},
		},
	}
}

// failingStatusPatchClient returns a fake client whose Status().Patch always fails, so that
// readiness-condition patch failures (and their events) can be exercised deterministically.
func failingStatusPatchClient() client.Client {
	return fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(_ context.Context, _ client.Client, _ string, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
				return errForcedPatchFailure
			},
		}).
		Build()
}

// TestDeterminePvcExpansionEmitsErrorEventOnBadAnnotation verifies a PVCExpansionError warning is
// emitted when the existing minimum-size annotation recorded on the StatefulSet cannot be parsed.
func TestDeterminePvcExpansionEmitsErrorEventOnBadAnnotation(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Recorder: rec}
	instance := solrCloudWithStorageRequest("5Gi")
	sts := &appsv1.StatefulSet{}
	sts.Annotations = map[string]string{util.StorageMinimumSizeAnnotation: "not-a-quantity"}

	clusterOp, _, err := determinePvcExpansionClusterOpLockIfNecessary(context.Background(), r, instance, sts, logr.Discard())
	if err == nil {
		t.Error("expected an error parsing the existing PVC size annotation, got nil")
	}
	if clusterOp != nil {
		t.Errorf("expected no cluster operation to be started, got %+v", clusterOp)
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "PVCExpansionError")
}

// TestDeterminePvcExpansionEmitsForbiddenEventOnShrink verifies a PVCExpansionForbidden warning is
// emitted (and no cluster op started) when the requested size is smaller than the existing size.
func TestDeterminePvcExpansionEmitsForbiddenEventOnShrink(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Recorder: rec}
	instance := solrCloudWithStorageRequest("5Gi")
	sts := &appsv1.StatefulSet{}
	sts.Annotations = map[string]string{util.StorageMinimumSizeAnnotation: "10Gi"}

	clusterOp, _, err := determinePvcExpansionClusterOpLockIfNecessary(context.Background(), r, instance, sts, logr.Discard())
	if err != nil {
		t.Errorf("did not expect an error for a shrink request, got %v", err)
	}
	if clusterOp != nil {
		t.Errorf("expected no cluster operation for a shrink request, got %+v", clusterOp)
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "PVCExpansionForbidden")
}

// TestDeterminePvcExpansionNoEventWhenSizeUnchanged verifies that the steady-state path (requested
// size matches the recorded size) neither starts a cluster op nor records an event.
func TestDeterminePvcExpansionNoEventWhenSizeUnchanged(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Recorder: rec}
	instance := solrCloudWithStorageRequest("5Gi")
	sts := &appsv1.StatefulSet{}
	sts.Annotations = map[string]string{util.StorageMinimumSizeAnnotation: "5Gi"}

	clusterOp, _, err := determinePvcExpansionClusterOpLockIfNecessary(context.Background(), r, instance, sts, logr.Discard())
	if err != nil {
		t.Errorf("did not expect an error, got %v", err)
	}
	if clusterOp != nil {
		t.Errorf("expected no cluster operation, got %+v", clusterOp)
	}
	requireNoEvent(t, rec)
}

// TestHandleManagedScaleDownEmitsEventOnBadMetadata verifies a ClusterOperationError warning is
// emitted when the scale-down target stored in the cluster-operation metadata cannot be parsed.
func TestHandleManagedScaleDownEmitsEventOnBadMetadata(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}
	clusterOp := &SolrClusterOp{Operation: ScaleDownLock, Metadata: "not-an-int"}

	_, _, _, err := handleManagedCloudScaleDown(context.Background(), r, instance, &appsv1.StatefulSet{}, clusterOp, nil, logr.Discard())
	if err == nil {
		t.Error("expected an error parsing the scale-down metadata, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "ClusterOperationError")
}

// TestInitializePodEmitsEventOnPatchFailure verifies that a failed readiness-condition patch while
// starting traffic on a pod surfaces a PodReadinessConditionUpdateFailed warning.
func TestInitializePodEmitsEventOnPatchFailure(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Client: failingStatusPatchClient(), Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}
	pod := podWithReadinessGate(util.SolrIsNotStoppedReadinessCondition)

	if _, err := r.initializePod(context.Background(), instance, pod, logr.Discard()); err == nil {
		t.Error("expected the forced patch failure to be returned, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "PodReadinessConditionUpdateFailed")
}

// TestEnsurePodReadinessConditionsEmitsEventOnPatchFailure verifies that a failed readiness-condition
// patch while stopping traffic on a pod surfaces a PodReadinessConditionUpdateFailed warning.
func TestEnsurePodReadinessConditionsEmitsEventOnPatchFailure(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Client: failingStatusPatchClient(), Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}

	pod := podWithReadinessGate(util.SolrIsNotStoppedReadinessCondition)
	// Seed an existing condition with a different reason so a change (and thus a patch) is required.
	pod.Status.Conditions = []corev1.PodCondition{{
		Type:   util.SolrIsNotStoppedReadinessCondition,
		Status: corev1.ConditionTrue,
		Reason: string(PodStarted),
	}}
	ensureConditions := map[corev1.PodConditionType]podReadinessConditionChange{
		util.SolrIsNotStoppedReadinessCondition: {
			reason:  PodUpdate,
			message: "Pod is being deleted, traffic to the pod must be stopped",
			status:  false,
		},
	}

	if _, err := EnsurePodReadinessConditions(context.Background(), r, instance, pod, ensureConditions, logr.Discard()); err == nil {
		t.Error("expected the forced patch failure to be returned, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "PodReadinessConditionUpdateFailed")
}

// TestHandleManagedScaleUpEmitsEventOnBadMetadata verifies a ClusterOperationError warning is
// emitted when the scale-up target stored in the cluster-operation metadata cannot be parsed.
func TestHandleManagedScaleUpEmitsEventOnBadMetadata(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}
	clusterOp := &SolrClusterOp{Operation: ScaleUpLock, Metadata: "not-an-int"}

	if _, _, err := handleManagedCloudScaleUp(context.Background(), r, instance, &appsv1.StatefulSet{}, clusterOp, nil, logr.Discard()); err == nil {
		t.Error("expected an error parsing the scale-up metadata, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "ClusterOperationError")
}

// TestHandlePvcExpansionEmitsEventOnBadMetadata verifies a PVCExpansionError warning is emitted
// when the target PVC size stored in the cluster-operation metadata cannot be parsed.
func TestHandlePvcExpansionEmitsEventOnBadMetadata(t *testing.T) {
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}
	clusterOp := &SolrClusterOp{Operation: PvcExpansionLock, Metadata: "not-a-quantity"}

	if _, _, err := handlePvcExpansion(context.Background(), r, instance, &appsv1.StatefulSet{}, clusterOp, logr.Discard()); err == nil {
		t.Error("expected an error parsing the PVC expansion metadata, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "PVCExpansionError")
}

// TestDeletePodForUpdateTreatsNotFoundAsSuccess verifies that when the target
// pod is already gone, DeletePodForUpdate does not report a PodUpdateError
// warning. A NotFound from the delete is the desired postcondition (the pod is
// absent and will be recreated), so it should surface a normal PodUpdate event.
func TestDeletePodForUpdateTreatsNotFoundAsSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("could not add client-go types to scheme: %v", err)
	}
	if err := solrv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("could not add solr types to scheme: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test",
			UID:       "test-uid",
		},
	}

	// Force the fake client's Delete to return NotFound, simulating a pod that
	// was already removed by a prior reconcile.
	notFoundClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "test-pod")
			},
		}).
		Build()

	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Client: notFoundClient, Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}

	if _, _, err := DeletePodForUpdate(context.Background(), r, instance, pod, false, logr.Discard()); err == nil {
		t.Error("expected DeletePodForUpdate to return the NotFound delete error, got nil")
	}
	// A NotFound delete must surface a normal PodUpdate event, never a PodUpdateError warning.
	requireEvent(t, rec, corev1.EventTypeNormal, "PodUpdate")
}

// TestDeletePodForUpdateReportsRealDeleteError verifies that a non-NotFound
// delete failure is still surfaced as a PodUpdateError warning.
func TestDeletePodForUpdateReportsRealDeleteError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("could not add client-go types to scheme: %v", err)
	}
	if err := solrv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("could not add solr types to scheme: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test",
			UID:       "test-uid",
		},
	}

	realErrClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return errors.New("boom")
			},
		}).
		Build()

	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Client: realErrClient, Recorder: rec}
	instance := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}

	if _, _, err := DeletePodForUpdate(context.Background(), r, instance, pod, false, logr.Discard()); err == nil {
		t.Error("expected DeletePodForUpdate to return the delete error, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "PodUpdateError")
}

// emitted when a backup is attempted against a repository that the SolrCloud has not yet marked
// available. A GCS repository is used so that EnsureDirectoryForBackup is a no-op (no pod exec).
func TestReconcileSolrCloudBackupEmitsCloudNotReadyEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("could not add client-go types to scheme: %v", err)
	}
	if err := solrv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("could not add solr types to scheme: %v", err)
	}

	solrCloud := &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud", Namespace: "test"},
		Spec: solrv1beta1.SolrCloudSpec{
			BackupRepositories: []solrv1beta1.SolrBackupRepository{{
				Name: "test-repo",
				GCS:  &solrv1beta1.GcsRepository{Bucket: "test-bucket"},
			}},
		},
		Status: solrv1beta1.SolrCloudStatus{
			BackupRepositoriesAvailable: map[string]bool{"test-repo": false},
		},
	}
	backup := &solrv1beta1.SolrBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "test"},
		Spec: solrv1beta1.SolrBackupSpec{
			SolrCloud:      "cloud",
			RepositoryName: "test-repo",
		},
	}

	rec := record.NewFakeRecorder(8)
	r := &SolrBackupReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(solrCloud).Build(),
		Recorder: rec,
	}

	if _, _, err := r.reconcileSolrCloudBackup(context.Background(), backup, &backup.Status.IndividualSolrBackupStatus, logr.Discard()); err == nil {
		t.Error("expected a 'cloud not ready' error, got nil")
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "BackupCloudNotReady")
}
