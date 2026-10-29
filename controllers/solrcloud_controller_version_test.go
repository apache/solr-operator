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
	"fmt"
	"testing"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/apache/solr-operator/controllers/util"
)

// TestReconcileSkipsSolrCloudWithUnknownSolrVersion verifies that a SolrCloud whose Solr version cannot be
// determined from its image tag gets an UnsupportedSolrVersion warning and no child resources.
func TestReconcileSkipsSolrCloudWithUnknownSolrVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := solrv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	solrCloud := &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solrv1beta1.SolrCloudSpec{
			SolrImage: &solrv1beta1.ContainerImage{Repository: "my-registry/solr", Tag: "latest"},
		},
	}
	// Store an already-defaulted SolrCloud, so that the reconcile gets past the defaulting step
	solrCloud.WithDefaults(logr.Discard())

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(solrCloud).Build()
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Client: fakeClient, Scheme: scheme, Recorder: rec}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %+v", result)
	}
	requireEvent(t, rec, corev1.EventTypeWarning, "UnsupportedSolrVersion")
	requireNoEvent(t, rec)

	statefulSets := &appsv1.StatefulSetList{}
	services := &corev1.ServiceList{}
	configMaps := &corev1.ConfigMapList{}
	for _, list := range []client.ObjectList{statefulSets, services, configMaps} {
		if err = fakeClient.List(context.Background(), list, client.InNamespace("default")); err != nil {
			t.Fatal(err)
		}
	}
	if len(statefulSets.Items)+len(services.Items)+len(configMaps.Items) > 0 {
		t.Errorf("expected no child resources, got %d StatefulSets, %d Services and %d ConfigMaps",
			len(statefulSets.Items), len(services.Items), len(configMaps.Items))
	}
}

// TestReconcileDeletesSolrCloudWithUnknownSolrVersion verifies that a SolrCloud with an unknown Solr major version
// can still be deleted: its storage finalizer deletes the PVCs and is removed, instead of blocking the deletion.
func TestReconcileDeletesSolrCloudWithUnknownSolrVersion(t *testing.T) {
	// Without a StatefulSet, e.g. with foreground cascading deletion, the PVCs must still be found and deleted
	for _, withStatefulSet := range []bool{true, false} {
		t.Run(fmt.Sprintf("withStatefulSet=%t", withStatefulSet), func(t *testing.T) {
			testReconcileDeletesSolrCloudWithUnknownSolrVersion(t, withStatefulSet)
		})
	}
}

func testReconcileDeletesSolrCloudWithUnknownSolrVersion(t *testing.T, withStatefulSet bool) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := solrv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	now := metav1.Now()
	solrCloud := &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "foo",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.SolrStorageFinalizer},
		},
		Spec: solrv1beta1.SolrCloudSpec{
			SolrImage: &solrv1beta1.ContainerImage{Repository: "my-registry/solr", Tag: "latest"},
			StorageOptions: solrv1beta1.SolrDataStorageOptions{
				PersistentStorage: &solrv1beta1.SolrPersistentDataStorageOptions{
					VolumeReclaimPolicy: solrv1beta1.VolumeReclaimPolicyDelete,
					PersistentVolumeClaimTemplate: solrv1beta1.PersistentVolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
							},
						},
					},
				},
			},
		},
	}
	solrCloud.WithDefaults(logr.Discard())

	selector := util.SolrCloudSelectorLabels(solrCloud)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data-foo-solrcloud-0", Namespace: "default", Labels: selector},
	}
	objects := []client.Object{solrCloud, pvc}
	if withStatefulSet {
		objects = append(objects, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: solrCloud.StatefulSetName(), Namespace: "default"},
			Spec:       appsv1.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: selector}},
		})
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	rec := record.NewFakeRecorder(8)
	r := &SolrCloudReconciler{Client: fakeClient, Scheme: scheme, Recorder: rec}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	requireNoEvent(t, rec)

	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pvc), &corev1.PersistentVolumeClaim{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected the PVC to be deleted, got %v", err)
	}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(solrCloud), &solrv1beta1.SolrCloud{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected the SolrCloud to be deleted once its finalizer is removed, got %v", err)
	}
}
