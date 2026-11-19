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
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// fakeSolrCollectionsApi stands in for the Solr Collections API. REQUESTSTATUS answers with the
// next async state queued for the request id, or "notfound" when none is queued. BACKUP and
// DELETESTATUS always succeed, and every call is recorded.
type fakeSolrCollectionsApi struct {
	t *testing.T
	// Queued async states, by request id
	asyncStates map[string][]string
	// BACKUP calls for these collections fail.
	failBackupFor map[string]bool
	calls         []string
	backupParams  []map[string]string
}

func (f *fakeSolrCollectionsApi) roundTrip(request *http.Request) (*http.Response, error) {
	query := request.URL.Query()
	action := query.Get("action")
	f.calls = append(f.calls, action)

	var body string
	switch action {
	case "REQUESTSTATUS":
		state := "notfound"
		if queued := f.asyncStates[query.Get("requestid")]; len(queued) > 0 {
			state = queued[0]
			f.asyncStates[query.Get("requestid")] = queued[1:]
		}
		body = fmt.Sprintf(`{"responseHeader":{"status":0},"status":{"state":%q}}`, state)
	case "BACKUP":
		if f.failBackupFor[query.Get("collection")] {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("unavailable")),
			}, nil
		}
		f.backupParams = append(f.backupParams, map[string]string{
			"collection": query.Get("collection"),
			"name":       query.Get("name"),
			"async":      query.Get("async"),
		})
		body = `{"responseHeader":{"status":0}}`
	case "DELETESTATUS":
		body = `{"responseHeader":{"status":0},"status":"successfully removed stored response"}`
	case "LIST":
		// Only used to test a failure to list the collections.
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("unavailable")),
		}, nil
	default:
		f.t.Fatalf("Unexpected Collections API action %q", action)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// useFakeSolr routes all Solr API calls to the fake for the duration of the test. The async states
// are queued for the backup of "collection" by the "backup" SolrBackup, see notFoundTestFixtures.
func useFakeSolr(t *testing.T, asyncStates ...string) *fakeSolrCollectionsApi {
	fake := &fakeSolrCollectionsApi{t: t, asyncStates: map[string][]string{
		util.AsyncIdForCollectionBackup("collection", "backup"): asyncStates,
	}}
	solr_api.SetNoVerifyTLSHttpClient(&http.Client{Transport: roundTripFunc(fake.roundTrip)})
	t.Cleanup(func() {
		// Restore the client that solr_api's init() sets up.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		solr_api.SetNoVerifyTLSHttpClient(&http.Client{Transport: transport})
	})
	return fake
}

func notFoundTestFixtures(startedAgo time.Duration) (*solrv1beta1.SolrBackup, *solrv1beta1.IndividualSolrBackupStatus, *solrv1beta1.SolrCloud, *solrv1beta1.SolrBackupRepository) {
	backup := &solrv1beta1.SolrBackup{ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "ns"}}
	cloud := &solrv1beta1.SolrCloud{ObjectMeta: metav1.ObjectMeta{Name: "cloud", Namespace: "ns"}}
	repo := &solrv1beta1.SolrBackupRepository{Name: "repo", GCS: &solrv1beta1.GcsRepository{Bucket: "bucket"}}
	startTime := metav1.NewTime(time.Now().Add(-startedAgo))
	status := &solrv1beta1.IndividualSolrBackupStatus{
		CollectionBackupStatuses: []solrv1beta1.CollectionBackupStatus{{
			Collection: "collection",
			BackupName: util.FullCollectionBackupName("collection", backup.Name),
			InProgress: true,
			StartTime:  &startTime,
		}},
	}
	return backup, status, cloud, repo
}

// Solr can lose track of an accepted backup, e.g. when its async status is deleted while the
// operator is down, and then REQUESTSTATUS answers "notfound" forever. The operator must submit
// the backup again, rather than wait on it indefinitely, and must then see it through to the end.
func TestCollectionBackupIsResubmittedWhenSolrLosesTrackOfIt(t *testing.T) {
	// The second "notfound" is the check, before resubmitting, that Solr holds no request for the backup.
	fake := useFakeSolr(t, "notfound", "notfound", "completed")
	backup, status, cloud, repo := notFoundTestFixtures(time.Minute)
	originalStart := status.CollectionBackupStatuses[0].StartTime.DeepCopy()
	ctx := context.Background()

	// 1. Solr no longer knows about the backup: it is no longer considered in progress.
	asyncFinished, err := reconcileSolrCollectionBackup(ctx, backup, status, cloud, repo, "collection", logr.Discard())
	require.NoError(t, err)
	collectionStatus := status.CollectionBackupStatuses[0]
	assert.False(t, asyncFinished, "There is no async status to delete for a backup Solr lost track of")
	assert.False(t, collectionStatus.InProgress, "A backup Solr lost track of must not stay in progress")
	assert.False(t, collectionStatus.Finished)
	assert.Nil(t, collectionStatus.Successful)
	assert.Equal(t, "notfound", collectionStatus.AsyncBackupStatus, "The status should say why the backup was resubmitted")
	assert.False(t, util.UpdateStatusOfCollectionBackups(status), "The overall backup must not be finished")

	// 2. The next reconcile submits the backup again, with the same name and async id.
	asyncFinished, err = reconcileSolrCollectionBackup(ctx, backup, status, cloud, repo, "collection", logr.Discard())
	require.NoError(t, err)
	collectionStatus = status.CollectionBackupStatuses[0]
	assert.False(t, asyncFinished)
	assert.True(t, collectionStatus.InProgress, "The resubmitted backup should be in progress")
	require.Len(t, fake.backupParams, 1, "The backup should have been submitted again")
	assert.Equal(t, map[string]string{
		"collection": "collection",
		"name":       util.FullCollectionBackupName("collection", backup.Name),
		"async":      util.AsyncIdForCollectionBackup("collection", backup.Name),
	}, fake.backupParams[0])
	assert.True(t, originalStart.Equal(collectionStatus.StartTime), "Resubmitting must keep the original start time")

	// 3. The resubmitted backup completes, and the collection backup finishes successfully.
	// Its async status is left for the caller to delete, once this result has been saved.
	asyncFinished, err = reconcileSolrCollectionBackup(ctx, backup, status, cloud, repo, "collection", logr.Discard())
	require.NoError(t, err)
	collectionStatus = status.CollectionBackupStatuses[0]
	assert.True(t, asyncFinished, "The finished backup's async status should be reported for deletion")
	assert.True(t, collectionStatus.Finished)
	assert.False(t, collectionStatus.InProgress)
	require.NotNil(t, collectionStatus.Successful)
	assert.True(t, *collectionStatus.Successful)
	assert.Empty(t, collectionStatus.AsyncBackupStatus)
	assert.NotNil(t, collectionStatus.FinishTime)

	assert.True(t, util.UpdateStatusOfCollectionBackups(status), "The overall backup should now be finished")
	require.NotNil(t, status.Successful)
	assert.True(t, *status.Successful)

	assert.Equal(t, []string{"REQUESTSTATUS", "REQUESTSTATUS", "BACKUP", "REQUESTSTATUS"}, fake.calls, "The async status must not be deleted before the result is saved")
	assert.Empty(t, fake.asyncStates[util.AsyncIdForCollectionBackup("collection", backup.Name)], "Every queued async state should have been used")
}

// Right after a backup is submitted, a "notfound" must not trigger a resubmission straight away.
func TestCollectionBackupIsNotResubmittedRightAfterStarting(t *testing.T) {
	fake := useFakeSolr(t, "notfound")
	backup, status, cloud, repo := notFoundTestFixtures(time.Second)

	asyncFinished, err := reconcileSolrCollectionBackup(context.Background(), backup, status, cloud, repo, "collection", logr.Discard())
	require.NoError(t, err)
	collectionStatus := status.CollectionBackupStatuses[0]
	assert.False(t, asyncFinished)
	assert.True(t, collectionStatus.InProgress, "The backup should still be waited on")
	assert.Equal(t, "notfound", collectionStatus.AsyncBackupStatus)
	assert.Equal(t, []string{"REQUESTSTATUS"}, fake.calls)
}

// newInProgressBackupReconciler sets up a SolrBackup whose single collection backup is in progress,
// against a SolrCloud whose repository is ready. Status patches are recorded in fake.calls as
// "PATCH STATUS", so their order relative to the Solr calls can be checked. If failStatusPatch is
// set, saving the status fails.
func newInProgressBackupReconciler(t *testing.T, fake *fakeSolrCollectionsApi, failStatusPatch bool) (*SolrBackupReconciler, ctrl.Request) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, solrv1beta1.AddToScheme(scheme))

	cloud := &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud", Namespace: "ns"},
		Spec: solrv1beta1.SolrCloudSpec{
			BackupRepositories: []solrv1beta1.SolrBackupRepository{{Name: "repo", GCS: &solrv1beta1.GcsRepository{Bucket: "bucket"}}},
		},
		Status: solrv1beta1.SolrCloudStatus{BackupRepositoriesAvailable: map[string]bool{"repo": true}},
	}
	backup, status, _, _ := notFoundTestFixtures(time.Minute)
	backup.Spec = solrv1beta1.SolrBackupSpec{SolrCloud: "cloud", RepositoryName: "repo", Collections: []string{"collection"}}
	backup.WithDefaults()
	status.StartTime = metav1.Now()
	backup.Status.IndividualSolrBackupStatus = *status

	statusClient := fakeclient.NewClientBuilder().WithScheme(scheme).
		WithObjects(cloud, backup).
		WithStatusSubresource(&solrv1beta1.SolrBackup{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				fake.calls = append(fake.calls, "PATCH STATUS")
				if failStatusPatch {
					return errors.New("forced status patch failure")
				}
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	r := &SolrBackupReconciler{Client: statusClient, Recorder: record.NewFakeRecorder(16)}
	return r, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: backup.Namespace, Name: backup.Name}}
}

// Solr's async status for a finished collection backup must only be deleted once the backup's
// status, recording the result, has been saved. Otherwise a failed save would leave the backup
// in progress, with Solr reporting it as "notfound", and it would be taken again.
func TestSolrBackupAsyncStatusIsDeletedOnlyAfterTheStatusIsSaved(t *testing.T) {
	fake := useFakeSolr(t, "completed")
	r, req := newInProgressBackupReconciler(t, fake, false)

	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, []string{"REQUESTSTATUS", "PATCH STATUS", "DELETESTATUS"}, fake.calls)

	saved := &solrv1beta1.SolrBackup{}
	require.NoError(t, r.Get(context.Background(), req.NamespacedName, saved))
	require.Len(t, saved.Status.CollectionBackupStatuses, 1)
	assert.True(t, saved.Status.CollectionBackupStatuses[0].Finished, "The collection backup result should be saved")
	assert.True(t, saved.Status.Finished, "The backup should be saved as finished")
}

func TestSolrBackupAsyncStatusIsKeptWhenTheStatusCannotBeSaved(t *testing.T) {
	fake := useFakeSolr(t, "completed")
	r, req := newInProgressBackupReconciler(t, fake, true)

	_, err := r.Reconcile(context.Background(), req)
	require.Error(t, err, "The failed status save should be returned, so the reconcile is retried")
	assert.Equal(t, []string{"REQUESTSTATUS", "PATCH STATUS"}, fake.calls,
		"The async status must be kept, so the next reconcile can still find the backup's result")
}

// When the collections to back up have to be listed, and listing them fails, nothing can be backed
// up, so the error must end the reconcile of the SolrCloud backup.
func TestSolrCloudBackupStopsWhenCollectionsCannotBeListed(t *testing.T) {
	fake := useFakeSolr(t)
	backup, _, cloud, repo := notFoundTestFixtures(time.Minute)
	cloud.Spec.BackupRepositories = []solrv1beta1.SolrBackupRepository{*repo}
	status := &solrv1beta1.IndividualSolrBackupStatus{StartTime: metav1.Now()}

	r := &SolrBackupReconciler{Recorder: record.NewFakeRecorder(4)}
	asyncFinished, err := r.reconcileSolrCloudBackup(context.Background(), backup, cloud, status, logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not list the collections to back up")
	assert.Empty(t, asyncFinished)
	assert.Empty(t, status.CollectionBackupStatuses)
	assert.Equal(t, []string{"LIST"}, fake.calls, "No collection backups should be attempted")
}

// A collection whose backup cannot be started must still hold up the SolrBackup. Otherwise, once
// the other collections finish, the SolrBackup would be finished, and successful, without it.
func TestSolrBackupDoesNotFinishWithoutACollectionThatFailedToStart(t *testing.T) {
	fake := useFakeSolr(t)
	fake.failBackupFor = map[string]bool{"a": true}
	// b is checked before it is submitted, and then once in progress.
	fake.asyncStates[util.AsyncIdForCollectionBackup("b", "backup")] = []string{"notfound", "completed"}
	backup, _, cloud, repo := notFoundTestFixtures(time.Minute)
	backup.Spec.Collections = []string{"a", "b"}
	status := &solrv1beta1.IndividualSolrBackupStatus{StartTime: metav1.Now()}
	r := &SolrBackupReconciler{Recorder: record.NewFakeRecorder(4)}
	cloud.Spec.BackupRepositories = []solrv1beta1.SolrBackupRepository{*repo}

	// 1. a fails to start, b starts.
	_, err := r.reconcileSolrCloudBackup(context.Background(), backup, cloud, status, logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `collection "a"`)
	require.Len(t, status.CollectionBackupStatuses, 2, "Both collections should have a status, even the one that failed to start")
	assert.Equal(t, "a", status.CollectionBackupStatuses[0].Collection)
	assert.False(t, status.CollectionBackupStatuses[0].InProgress)
	assert.Nil(t, status.CollectionBackupStatuses[0].StartTime)
	assert.True(t, status.CollectionBackupStatuses[1].InProgress)

	// 2. a fails to start again, b completes. The SolrBackup must not finish without a.
	asyncFinished, err := r.reconcileSolrCloudBackup(context.Background(), backup, cloud, status, logr.Discard())
	require.Error(t, err)
	assert.Equal(t, []string{"b"}, asyncFinished)
	assert.True(t, status.CollectionBackupStatuses[1].Finished)
	assert.False(t, status.CollectionBackupStatuses[0].Finished)
	assert.False(t, status.Finished, "The SolrBackup must not finish while a collection has not been backed up")
	assert.Nil(t, status.Successful)
}
