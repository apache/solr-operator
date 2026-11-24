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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)

const (
	initContainersFields = `{"f:spec":{"f:customSolrKubeOptions":{"f:podOptions":{"f:initContainers":{}}}}}`
	replicasFields       = `{"f:spec":{"f:replicas":{}}}`
	backupRepoFields     = `{"f:spec":{"f:backupRepositories":{}}}`
)

func managedFieldsEntry(manager string, operation metav1.ManagedFieldsOperationType, fields string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{Manager: manager, Operation: operation, FieldsV1: &metav1.FieldsV1{Raw: []byte(fields)}}
}

func owns(t *testing.T, entry metav1.ManagedFieldsEntry, path fieldpath.Path) bool {
	fields, err := fieldSet(entry)
	if err != nil {
		t.Fatal(err)
	}
	return fields.Has(path)
}

func TestWithoutOperatorFieldOwnership(t *testing.T) {
	operator := managedFieldsEntry(FieldManager, metav1.ManagedFieldsOperationUpdate,
		`{"f:spec":{"f:customSolrKubeOptions":{"f:podOptions":{"f:initContainers":{}}},"f:solrImage":{"f:tag":{}}}}`)
	status := metav1.ManagedFieldsEntry{Manager: FieldManager, Operation: metav1.ManagedFieldsOperationUpdate, Subresource: "status"}

	tests := []struct {
		name          string
		others        []metav1.ManagedFieldsEntry
		expectedOwned bool
	}{
		{
			name:          "a single Apply manager gets the taken over list",
			others:        []metav1.ManagedFieldsEntry{managedFieldsEntry("helm", metav1.ManagedFieldsOperationApply, replicasFields)},
			expectedOwned: true,
		},
		{
			name: "nothing is handed back with several Apply managers",
			others: []metav1.ManagedFieldsEntry{
				managedFieldsEntry("helm", metav1.ManagedFieldsOperationApply, replicasFields),
				managedFieldsEntry("argocd", metav1.ManagedFieldsOperationApply, backupRepoFields),
			},
			expectedOwned: false,
		},
		{
			name: "nothing is handed back when another manager owns the list",
			others: []metav1.ManagedFieldsEntry{
				managedFieldsEntry("helm", metav1.ManagedFieldsOperationApply, replicasFields),
				managedFieldsEntry("kubectl-edit", metav1.ManagedFieldsOperationUpdate, initContainersFields),
			},
			expectedOwned: false,
		},
		{
			name:          "nothing is handed back to an Update manager",
			others:        []metav1.ManagedFieldsEntry{managedFieldsEntry("kubectl-client-side-apply", metav1.ManagedFieldsOperationUpdate, replicasFields)},
			expectedOwned: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			managedFields := append([]metav1.ManagedFieldsEntry{operator, status}, test.others...)
			kept, err := withoutOperatorFieldOwnership(managedFields)
			if err != nil {
				t.Fatal(err)
			}
			if len(kept) != len(managedFields)-1 {
				t.Fatalf("expected only the operator's spec entry to be removed, got %v", kept)
			}
			if kept[0].Subresource != "status" {
				t.Errorf("expected the operator's status entry to be kept, got %v", kept[0])
			}
			helm := kept[1]
			if owns(t, helm, takenOverLists[0]) != test.expectedOwned {
				t.Errorf("expected %s to own the init containers: %v", helm.Manager, test.expectedOwned)
			}
			if !owns(t, helm, fieldpath.MakePathOrDie("spec", "replicas")) {
				t.Errorf("expected %s to keep its own fields", helm.Manager)
			}
			if owns(t, helm, fieldpath.MakePathOrDie("spec", "solrImage", "tag")) {
				t.Errorf("expected the defaulted fields not to be handed to %s", helm.Manager)
			}
		})
	}
}
