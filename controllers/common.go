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
	"bytes"
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)

// FieldManager is the field manager of all the operator's writes. It is set as the default field owner of the
// operator's client, so that its managedFields entries can be told apart from those of other clients.
const FieldManager = "solr-operator"

// Set the requeueAfter if it has not been set, or is greater than the new time to requeue at
func updateRequeueAfter(requeueOrNot *reconcile.Result, newWait time.Duration) {
	if newWait <= 0 {
		requeueOrNot.RequeueAfter = 0
	}
	if requeueOrNot.RequeueAfter <= 0 || requeueOrNot.RequeueAfter > newWait {
		requeueOrNot.RequeueAfter = newWait
	}
}

// patchResource writes only the changes made to obj since orig was read, failing if obj was modified in between.
// An Update would also write the fields that the Go types fill in, making the operator their owner.
func patchResource(ctx context.Context, c client.Client, obj client.Object, orig client.Object) error {
	return c.Patch(ctx, obj, client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{}))
}

// takenOverLists are the atomic lists that operators before v0.10 took over from the applier, by writing them back with
// the empty fields that the Go types fill in, such as a resources block on every container.
var takenOverLists = []fieldpath.Path{
	fieldpath.MakePathOrDie("spec", "customSolrKubeOptions", "podOptions", "initContainers"),
	fieldpath.MakePathOrDie("spec", "customSolrKubeOptions", "podOptions", "sidecarContainers"),
	fieldpath.MakePathOrDie("spec", "zookeeperRef", "provided", "zookeeperPodPolicy", "initContainers"),
	fieldpath.MakePathOrDie("spec", "zookeeperRef", "provided", "zookeeperPodPolicy", "containers"),
	fieldpath.MakePathOrDie("spec", "customKubeOptions", "podOptions", "initContainers"),
	fieldpath.MakePathOrDie("spec", "customKubeOptions", "podOptions", "sidecarContainers"),
}

// releaseFieldOwnership drops the operator's ownership of the spec and metadata fields it has written, such as defaults,
// so that a server-side apply of another value, e.g. by Helm, does not conflict with the operator.
func releaseFieldOwnership(ctx context.Context, c client.Client, obj client.Object) error {
	managedFields := obj.GetManagedFields()
	kept, err := withoutOperatorFieldOwnership(managedFields)
	if err != nil {
		return err
	}
	if len(kept) == len(managedFields) {
		return nil
	}
	if len(kept) == 0 {
		// An empty list leaves managedFields unchanged, a single empty entry clears them
		kept = []metav1.ManagedFieldsEntry{{}}
	}
	orig := obj.DeepCopyObject().(client.Object)
	obj.SetManagedFields(kept)
	return patchResource(ctx, c, obj, orig)
}

// withoutOperatorFieldOwnership removes the operator's spec and metadata entries from managedFields. Server-side apply
// never removes a field that nobody owns, so the takenOverLists that only the operator owns are handed back to the
// object's Apply manager, if it has exactly one.
func withoutOperatorFieldOwnership(managedFields []metav1.ManagedFieldsEntry) ([]metav1.ManagedFieldsEntry, error) {
	kept := make([]metav1.ManagedFieldsEntry, 0, len(managedFields))
	operatorOwned := &fieldpath.Set{}
	otherOwned := &fieldpath.Set{}
	applyIndex := -1
	applyEntries := 0
	for _, entry := range managedFields {
		if entry.Subresource != "" {
			kept = append(kept, entry)
			continue
		}
		fields, err := fieldSet(entry)
		if err != nil {
			return nil, err
		}
		if entry.Manager == FieldManager {
			operatorOwned = operatorOwned.Union(fields)
			continue
		}
		otherOwned = otherOwned.Union(fields)
		if entry.Operation == metav1.ManagedFieldsOperationApply {
			applyIndex = len(kept)
			applyEntries++
		}
		kept = append(kept, entry)
	}
	if len(kept) == len(managedFields) || applyEntries != 1 {
		return kept, nil
	}

	handBack := &fieldpath.Set{}
	for _, list := range takenOverLists {
		if operatorOwned.Has(list) && !otherOwned.Has(list) {
			handBack.Insert(list)
		}
	}
	if handBack.Empty() {
		return kept, nil
	}
	applied, err := fieldSet(kept[applyIndex])
	if err != nil {
		return nil, err
	}
	raw, err := applied.Union(handBack).ToJSON()
	if err != nil {
		return nil, err
	}
	kept[applyIndex].FieldsV1 = &metav1.FieldsV1{Raw: raw}
	return kept, nil
}

func fieldSet(entry metav1.ManagedFieldsEntry) (*fieldpath.Set, error) {
	fields := &fieldpath.Set{}
	if entry.FieldsV1 == nil {
		return fields, nil
	}
	return fields, fields.FromJSON(bytes.NewReader(entry.FieldsV1.Raw))
}
