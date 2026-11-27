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
	fieldpath.MakePathOrDie("spec", "zookeeperRef", "provided", "initContainers"),
	fieldpath.MakePathOrDie("spec", "zookeeperRef", "provided", "containers"),
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
// object's Apply manager, if it has exactly one and no other client has written the spec with an Update.
func withoutOperatorFieldOwnership(managedFields []metav1.ManagedFieldsEntry) ([]metav1.ManagedFieldsEntry, error) {
	kept := make([]metav1.ManagedFieldsEntry, 0, len(managedFields))
	var operatorEntries []metav1.ManagedFieldsEntry
	for _, entry := range managedFields {
		if entry.Manager == FieldManager && entry.Subresource == "" {
			operatorEntries = append(operatorEntries, entry)
		} else {
			kept = append(kept, entry)
		}
	}
	if len(operatorEntries) == 0 {
		return kept, nil
	}

	// Migration of SolrClouds last written by operators before v0.10, can be removed once upgrades from v0.9 are not supported
	operatorOwned, err := unionFieldSets(operatorEntries)
	if err != nil {
		return nil, err
	}
	var ownedLists []fieldpath.Path
	for _, list := range takenOverLists {
		if operatorOwned.Has(list) {
			ownedLists = append(ownedLists, list)
		}
	}
	if len(ownedLists) == 0 {
		return kept, nil
	}

	applyIndex := -1
	var others []metav1.ManagedFieldsEntry
	for i, entry := range kept {
		if entry.Subresource != "" {
			continue
		}
		if entry.Operation == metav1.ManagedFieldsOperationApply {
			if applyIndex >= 0 {
				return kept, nil
			}
			applyIndex = i
		}
		others = append(others, entry)
	}
	if applyIndex < 0 {
		return kept, nil
	}
	// A list that another client wrote with an Update, e.g. Helm 3 or kubectl client-side apply, would otherwise be
	// handed to an unrelated applier of a few fields, whose next apply would remove it
	for _, entry := range others {
		if entry.Operation == metav1.ManagedFieldsOperationApply || entry.Manager == kept[applyIndex].Manager {
			continue
		}
		fields, err := fieldSet(entry)
		if err != nil {
			return nil, err
		}
		if !fields.WithPrefix(fieldpath.PathElement{FieldName: &specField}).Empty() {
			return kept, nil
		}
	}
	otherOwned, err := unionFieldSets(others)
	if err != nil {
		return nil, err
	}
	handBack := &fieldpath.Set{}
	for _, list := range ownedLists {
		if !otherOwned.Has(list) {
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

var specField = "spec"

func unionFieldSets(entries []metav1.ManagedFieldsEntry) (*fieldpath.Set, error) {
	union := &fieldpath.Set{}
	for _, entry := range entries {
		fields, err := fieldSet(entry)
		if err != nil {
			return nil, err
		}
		union = union.Union(fields)
	}
	return union, nil
}

func fieldSet(entry metav1.ManagedFieldsEntry) (*fieldpath.Set, error) {
	fields := &fieldpath.Set{}
	if entry.FieldsV1 == nil {
		return fields, nil
	}
	return fields, fields.FromJSON(bytes.NewReader(entry.FieldsV1.Raw))
}
