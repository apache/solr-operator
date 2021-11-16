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

package util

import (
	"bytes"
	"context"
	"fmt"
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"net/url"
	"strconv"
	"time"
)

func GetBackupRepositoryByName(backupRepos []solr.SolrBackupRepository, repositoryName string) *solr.SolrBackupRepository {
	// If no name is given and only 1 repo exists, return the repo
	if repositoryName == "" && len(backupRepos) == 1 {
		return &backupRepos[0]
	}
	//Build map of string->BackupRepository
	for _, repo := range backupRepos {
		if repo.Name == repositoryName {
			return &repo
		}
	}
	return nil
}

func FullCollectionBackupName(collection string, backupName string) string {
	return fmt.Sprintf("%s-%s", backupName, collection)
}

func AsyncIdForCollectionBackup(collection string, backupName string) string {
	return fmt.Sprintf("%s-%s", backupName, collection)
}

func UpdateStatusOfCollectionBackups(backupStatus *solr.IndividualSolrBackupStatus) (allFinished bool) {
	// Check if all collection backups have been completed, this is updated in the loop
	allFinished = len(backupStatus.CollectionBackupStatuses) > 0

	allSuccessful := len(backupStatus.CollectionBackupStatuses) > 0

	for _, collectionStatus := range backupStatus.CollectionBackupStatuses {
		allFinished = allFinished && collectionStatus.Finished
		allSuccessful = allSuccessful && (collectionStatus.Successful != nil && *collectionStatus.Successful)
	}

	backupStatus.Finished = allFinished
	if allFinished && backupStatus.Successful == nil {
		backupStatus.Successful = &allSuccessful
	}
	return
}

func GenerateQueryParamsForBackup(backupRepository *solr.SolrBackupRepository, backup *solr.SolrBackup, collection string) url.Values {
	queryParams := url.Values{}
	queryParams.Add("action", "BACKUP")
	queryParams.Add("collection", collection)
	queryParams.Add("name", FullCollectionBackupName(collection, backup.Name))
	queryParams.Add("async", AsyncIdForCollectionBackup(collection, backup.Name))
	queryParams.Add("location", BackupLocationPath(backupRepository, backup.Spec.Location))
	queryParams.Add("repository", backup.Spec.RepositoryName)

	if backup.Spec.Recurrence.IsEnabled() {
		queryParams.Add("maxNumBackupPoints", strconv.Itoa(backup.Spec.Recurrence.MaxSaved))
	}

	return queryParams
}

func StartBackupForCollection(ctx context.Context, cloud *solr.SolrCloud, backupRepository *solr.SolrBackupRepository, backup *solr.SolrBackup, collection string, logger logr.Logger) (success bool, err error) {
	queryParams := GenerateQueryParamsForBackup(backupRepository, backup, collection)
	resp := &solr_api.SolrAsyncResponse{}

	logger.Info("Calling to start collection backup", "solrCloud", cloud.Name, "collection", collection)
	err = solr_api.CallCollectionsApi(ctx, cloud, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			success = true
		}
	} else {
		logger.Error(err, "Error starting collection backup", "solrCloud", cloud.Name, "collection", collection)
	}

	return success, err
}

func CheckBackupForCollection(ctx context.Context, cloud *solr.SolrCloud, collection string, backupName string, logger logr.Logger) (finished bool, success bool, asyncStatus string, err error) {
	logger.Info("Calling to check on collection backup", "solrCloud", cloud.Name, "collection", collection)

	var message string
	asyncStatus, message, err = solr_api.CheckAsyncRequest(ctx, cloud, AsyncIdForCollectionBackup(collection, backupName))

	if err == nil {
		if asyncStatus == "completed" {
			finished = true
			success = true
		}
		if asyncStatus == "failed" {
			finished = true
			success = false
		}
	} else {
		logger.Error(err, "Error checking on collection backup", "solrCloud", cloud.Name, "collection", collection, "message", message)
	}

	return finished, success, asyncStatus, err
}

func DeleteAsyncInfoForBackup(ctx context.Context, cloud *solr.SolrCloud, collection string, backupName string, logger logr.Logger) (err error) {
	logger.Info("Calling to delete async info for backup command.", "solrCloud", cloud.Name, "collection", collection)
	var message string
	message, err = solr_api.DeleteAsyncRequest(ctx, cloud, AsyncIdForCollectionBackup(collection, backupName))

	if err != nil {
		logger.Error(err, "Error deleting async data for collection backup", "solrCloud", cloud.Name, "collection", collection, "message", message)
	}

	return err
}

func EnsureDirectoryForBackup(solrCloud *solr.SolrCloud, backupRepository *solr.SolrBackupRepository, backup *solr.SolrBackup, config *rest.Config) (err error) {
	// Directory creation only required/possible for volume (i.e. local) backups
	if IsRepoVolume(backupRepository) {
		backupPath := BackupLocationPath(backupRepository, backup.Spec.Location)
		return RunExecForPod(
			solrCloud.GetAllSolrPodNames()[0],
			solrCloud.Namespace,
			[]string{"/bin/bash", "-c", "mkdir -p " + backupPath},
			config,
		)
	}
	return nil
}

func RunExecForPod(podName string, namespace string, command []string, config *rest.Config) (err error) {
	client := &kubernetes.Clientset{}
	if client, err = kubernetes.NewForConfig(config); err != nil {
		return err
	}
	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err = corev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   command,
		Container: "solrcloud-node",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	var exec remotecommand.Executor
	exec, err = remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error while creating Executor: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return fmt.Errorf("error in Stream: %v", err)
	}

	return nil
}

func ScheduleNextBackup(restartSchedule string, lastBackupTime time.Time) (nextBackup time.Time, err error) {
	if parsedSchedule, parseErr := cron.ParseStandard(restartSchedule); parseErr != nil {
		err = parseErr
	} else {
		nextBackup = parsedSchedule.Next(lastBackupTime)
	}
	return
}
