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
	"io"
	"net/http"
	"strings"
	"testing"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeSolrAsync tracks async request states by ID and, like Solr, rejects a BACKUP whose async ID is already tracked.
type fakeSolrAsync struct {
	states  map[string]string
	actions []string
}

func (f *fakeSolrAsync) RoundTrip(request *http.Request) (*http.Response, error) {
	query := request.URL.Query()
	action := query.Get("action")
	f.actions = append(f.actions, action)

	status := http.StatusOK
	var body interface{}
	switch action {
	case "BACKUP":
		asyncId := query.Get("async")
		if _, exists := f.states[asyncId]; exists {
			status = http.StatusBadRequest
			body = map[string]interface{}{"responseHeader": map[string]int{"status": 400}}
		} else {
			f.states[asyncId] = "submitted"
			body = map[string]interface{}{"responseHeader": map[string]int{"status": 0}, "requestid": asyncId}
		}
	case "REQUESTSTATUS":
		state, exists := f.states[query.Get("requestid")]
		if !exists {
			state = "notfound"
		}
		body = map[string]interface{}{"responseHeader": map[string]int{"status": 0}, "status": map[string]string{"state": state}}
	case "DELETESTATUS":
		delete(f.states, query.Get("requestid"))
		body = map[string]interface{}{"responseHeader": map[string]int{"status": 0}, "status": "successfully removed"}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
}

func backupAsyncFixture(t *testing.T, existingState string) (*fakeSolrAsync, *solrv1beta1.SolrBackup, *solrv1beta1.SolrCloud, *solrv1beta1.SolrBackupRepository) {
	t.Helper()
	backup := &solrv1beta1.SolrBackup{ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "default"}}
	solr := &fakeSolrAsync{states: map[string]string{}}
	if existingState != "" {
		solr.states[util.AsyncIdForCollectionBackup("books", backup.Name)] = existingState
	}
	// Only main.go fills the mTLS slot, which the Solr API calls prefer, so resetting it to nil restores it.
	solr_api.SetMTLSHttpClient(&http.Client{Transport: solr})
	t.Cleanup(func() {
		solr_api.SetMTLSHttpClient(nil)
	})

	cloud := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "cloud", Namespace: "default"}}
	repo := &solrv1beta1.SolrBackupRepository{Name: "gcs", GCS: &solrv1beta1.GcsRepository{Bucket: "bucket"}}
	return solr, backup, cloud, repo
}

// A backup Solr accepted, but whose InProgress status never reached the apiserver, must be tracked rather than resubmitted.
func TestCollectionBackupAdoptsRequestSolrAlreadyAccepted(t *testing.T) {
	solr, backup, cloud, repo := backupAsyncFixture(t, "running")
	status := &solrv1beta1.IndividualSolrBackupStatus{}

	_, err := reconcileSolrCollectionBackup(context.Background(), backup, status, cloud, repo, "books", logr.Discard())
	require.NoError(t, err)
	require.Len(t, status.CollectionBackupStatuses, 1)
	assert.True(t, status.CollectionBackupStatuses[0].InProgress)
	assert.NotNil(t, status.CollectionBackupStatuses[0].StartTime)
	assert.NotContains(t, solr.actions, "BACKUP")

	solr.states[util.AsyncIdForCollectionBackup("books", backup.Name)] = "completed"
	finished, err := reconcileSolrCollectionBackup(context.Background(), backup, status, cloud, repo, "books", logr.Discard())
	require.NoError(t, err)
	assert.True(t, finished)
	require.NotNil(t, status.CollectionBackupStatuses[0].Successful)
	assert.True(t, *status.CollectionBackupStatuses[0].Successful)
	assert.Empty(t, solr.states, "the async request should be deleted once the backup finishes")
}

// A finished request may be left over from a previous recurrence, so it must not count as this backup.
func TestCollectionBackupResubmitsOverFinishedRequest(t *testing.T) {
	for _, state := range []string{"completed", "failed"} {
		t.Run(state, func(t *testing.T) {
			solr, backup, cloud, repo := backupAsyncFixture(t, state)
			status := &solrv1beta1.IndividualSolrBackupStatus{}

			finished, err := reconcileSolrCollectionBackup(context.Background(), backup, status, cloud, repo, "books", logr.Discard())
			require.NoError(t, err)
			assert.False(t, finished)
			assert.True(t, status.CollectionBackupStatuses[0].InProgress)
			assert.Equal(t, []string{"REQUESTSTATUS", "DELETESTATUS", "BACKUP"}, solr.actions)
			assert.Equal(t, "submitted", solr.states[util.AsyncIdForCollectionBackup("books", backup.Name)])
		})
	}
}

func TestCollectionBackupRejectsUnknownAsyncState(t *testing.T) {
	solr, backup, cloud, repo := backupAsyncFixture(t, "")
	solr.states[util.AsyncIdForCollectionBackup("books", backup.Name)] = ""
	status := &solrv1beta1.IndividualSolrBackupStatus{}

	_, err := reconcileSolrCollectionBackup(context.Background(), backup, status, cloud, repo, "books", logr.Discard())
	assert.Error(t, err)
	assert.Empty(t, status.CollectionBackupStatuses)
	assert.NotContains(t, solr.actions, "BACKUP")
}

func TestCollectionBackupSubmitsWhenSolrHasNoRequest(t *testing.T) {
	solr, backup, cloud, repo := backupAsyncFixture(t, "")
	status := &solrv1beta1.IndividualSolrBackupStatus{}

	_, err := reconcileSolrCollectionBackup(context.Background(), backup, status, cloud, repo, "books", logr.Discard())
	require.NoError(t, err)
	assert.True(t, status.CollectionBackupStatuses[0].InProgress)
	assert.Equal(t, []string{"REQUESTSTATUS", "BACKUP"}, solr.actions)
}
