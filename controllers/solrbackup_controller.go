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
	"reflect"
	"time"

	"github.com/apache/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
)

// SolrBackupReconciler reconciles a SolrBackup object
type SolrBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	config *rest.Config
}

//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds,verbs=get;list;watch
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds/status,verbs=get
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrbackups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *SolrBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the SolrBackup instance
	backup := &solrv1beta1.SolrBackup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return reconcile.Result{}, err
	}

	oldStatus := backup.Status.DeepCopy()

	changed := backup.WithDefaults()
	if changed {
		logger.Info("Setting default settings for solr-backup")
		if err := r.Update(ctx, backup); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// When working with the collection backups, auto-requeue after 5 seconds
	// to check on the status of the async solr backup calls
	requeueOrNot := reconcile.Result{}

	solrCloud, _, err := r.reconcileSolrCloudBackup(ctx, backup, logger)
	if err != nil {
		// TODO Should we be failing the backup for some sub-set of errors here?
		logger.Error(err, "Error while taking SolrCloud backup")

		// Requeue after 10 seconds for errors.
		requeueOrNot = reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}
	} else if solrCloud != nil && !backup.Status.Finished {
		// Only requeue if the SolrCloud we are backing up exists and we are not finished with the backups.
		requeueOrNot = reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}
	} else if backup.Status.Finished && backup.Status.FinishTime == nil {
		now := metav1.Now()
		backup.Status.FinishTime = &now
	}

	if !reflect.DeepEqual(oldStatus, &backup.Status) {
		logger.Info("Updating status for solr-backup")
		err = r.Status().Update(ctx, backup)
	}

	return requeueOrNot, err
}

func (r *SolrBackupReconciler) reconcileSolrCloudBackup(ctx context.Context, backup *solrv1beta1.SolrBackup, logger logr.Logger) (solrCloud *solrv1beta1.SolrCloud, actionTaken bool, err error) {
	// Get the solrCloud that this backup is for.
	solrCloud = &solrv1beta1.SolrCloud{}

	err = r.Get(ctx, types.NamespacedName{Namespace: backup.Namespace, Name: backup.Spec.SolrCloud}, solrCloud)
	if err != nil && errors.IsNotFound(err) {
		logger.Error(err, "Could not find cloud to backup", "solrCloud", backup.Spec.SolrCloud)
		return nil, actionTaken, err
	} else if err != nil {
		return nil, actionTaken, err
	}

	// Add any additional values needed to Authn to Solr to the Context used when invoking the API
	if solrCloud.Spec.SolrSecurity != nil {
		ctx, err = util.AddAuthToContext(ctx, &r.Client, solrCloud.Spec.SolrSecurity, solrCloud.Namespace)
		if err != nil {
			return nil, actionTaken, err
		}
	}

	// First check if the collection backups have been completed
	collectionBackupsFinished := util.UpdateStatusOfCollectionBackups(backup)

	// If the collectionBackups are complete, then nothing else has to be done here
	if collectionBackupsFinished {
		return solrCloud, actionTaken, nil
	}

	actionTaken = true
	backupRepository := util.GetBackupRepositoryByName(solrCloud.Spec.BackupRepositories, backup.Spec.RepositoryName)
	if backupRepository == nil {
		err = fmt.Errorf("Unable to find backup repository to use for backup [%s] (which specified the repository"+
			" [%s]).  solrcloud must define a repository matching that name (or have only 1 repository defined).",
			backup.Name, backup.Spec.RepositoryName)
		return solrCloud, actionTaken, err
	}

	// This should only occur before the backup processes have been started
	if backup.Status.SolrVersion == "" {
		// Prep the backup directory in the persistentVolume
		err = util.EnsureDirectoryForBackup(solrCloud, backupRepository, backup, r.config)
		if err != nil {
			return solrCloud, actionTaken, err
		}

		// Make sure that all solr nodes are active and have the backupRestore shared volume mounted
		// TODO: we do not need all replicas to be healthy. We should just check that leaders exist for all shards. (or just let Solr do that)
		cloudReady := solrCloud.Status.BackupRestoreReady && (solrCloud.Status.Replicas == solrCloud.Status.ReadyReplicas)
		if !cloudReady {
			logger.Info("Cloud not ready for backup backup", "solrCloud", solrCloud.Name)
			return solrCloud, actionTaken, errors.NewServiceUnavailable("Cloud is not ready for backups or restores")
		}

		// Only set the solr version at the start of the backup. This shouldn't change throughout the backup.
		backup.Status.SolrVersion = solrCloud.Status.Version
	}

	// Go through each collection specified and reconcile the backup.
	for _, collection := range backup.Spec.Collections {
		// This will in-place update the CollectionBackupStatus in the backup object
		_, err = reconcileSolrCollectionBackup(ctx, backup, solrCloud, backupRepository, collection, logger)
	}

	// First check if the collection backups have been completed
	util.UpdateStatusOfCollectionBackups(backup)

	return solrCloud, actionTaken, err
}

func reconcileSolrCollectionBackup(ctx context.Context, backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud, backupRepository *solrv1beta1.SolrBackupRepository, collection string, logger logr.Logger) (finished bool, err error) {
	now := metav1.Now()
	collectionBackupStatus := solrv1beta1.CollectionBackupStatus{}
	collectionBackupStatus.Collection = collection
	backupIndex := -1
	// Get the backup status for this collection, if one exists
	for i, status := range backup.Status.CollectionBackupStatuses {
		if status.Collection == collection {
			collectionBackupStatus = status
			backupIndex = i
		}
	}

	// If the collection backup hasn't started, start it
	if !collectionBackupStatus.InProgress && !collectionBackupStatus.Finished {
		// Start the backup by calling solr
		started, err := util.StartBackupForCollection(ctx, solrCloud, backupRepository, backup, collection, logger)
		if err != nil {
			return true, err
		}
		collectionBackupStatus.InProgress = started
		if started && collectionBackupStatus.StartTime == nil {
			collectionBackupStatus.StartTime = &now
		}
		collectionBackupStatus.BackupName = util.FullCollectionBackupName(collection, backup.Name)
	} else if collectionBackupStatus.InProgress {
		// Check the state of the backup, when it is in progress, and update the state accordingly
		finished, successful, asyncStatus, err := util.CheckBackupForCollection(ctx, solrCloud, collection, backup.Name, logger)
		if err != nil {
			return false, err
		}
		collectionBackupStatus.Finished = finished
		if finished {
			collectionBackupStatus.InProgress = false
			if collectionBackupStatus.Successful == nil {
				collectionBackupStatus.Successful = &successful
			}
			collectionBackupStatus.AsyncBackupStatus = ""
			if collectionBackupStatus.FinishTime == nil {
				collectionBackupStatus.FinishTime = &now
			}

			err = util.DeleteAsyncInfoForBackup(ctx, solrCloud, collection, backup.Name, logger)
		} else {
			collectionBackupStatus.AsyncBackupStatus = asyncStatus
		}
	}

	if backupIndex < 0 {
		backup.Status.CollectionBackupStatuses = append(backup.Status.CollectionBackupStatuses, collectionBackupStatus)
	} else {
		backup.Status.CollectionBackupStatuses[backupIndex] = collectionBackupStatus
	}

	return collectionBackupStatus.Finished, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolrBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()

	return ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
