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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

// releaseFieldOwnership drops the operator's ownership of the spec and metadata fields it has written, such as defaults,
// so that a server-side apply of another value, e.g. by Helm, does not conflict with the operator.
func releaseFieldOwnership(ctx context.Context, c client.Client, obj client.Object) error {
	managedFields := obj.GetManagedFields()
	kept := make([]metav1.ManagedFieldsEntry, 0, len(managedFields))
	for _, entry := range managedFields {
		if entry.Manager != FieldManager || entry.Subresource != "" {
			kept = append(kept, entry)
		}
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
