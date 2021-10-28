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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
)

// SolrBackupReconciler reconciles a SolrBackup object
type SolrBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *rest.Config
}

//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
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
		if err = r.Update(ctx, backup); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	requeueOrNot := reconcile.Result{}

	// Check if we should start the next backup
	if backup.Status.NextScheduledTime != nil {
		if backup.Status.NextScheduledTime.UTC().After(time.Now().UTC()) {
			// We have hit the next scheduled restart time.
			// Set the next scheduled time to nil, and continue through to start the next backup.
			requeueOrNot = reconcile.Result{Requeue: true}
		} else {
			// If we have not hit the next scheduled restart, wait to requeue until that is true.
			updateRequeueAfter(&requeueOrNot, backup.Status.NextScheduledTime.UTC().Sub(time.Now().UTC()))
		}
	}

	// Do backup work if a nextScheduledTime is not set and there is no current finish time.
	// The nextScheduledTime is only set when the last backup is finished and we are waiting to do the next
	if backup.Status.NextScheduledTime == nil && backup.Status.Current.FinishTime == nil {
		// When working with the collection backups, auto-requeue after 5 seconds
		// to check on the status of the async solr backup calls
		updateRequeueAfter(&requeueOrNot, time.Second*5)

		solrCloud, allCollectionsComplete, collectionActionTaken, err1 := r.reconcileSolrCloudBackup(ctx, backup, logger)
		if err1 != nil {
			// TODO Should we be failing the backup for some sub-set of errors here?
			logger.Error(err1, "Error while taking SolrCloud backup")
		}
		if allCollectionsComplete && collectionActionTaken {
			// Requeue immediately to start the persisting job
			// From here on in the backup lifecycle, requeueing will not happen for the backup.
			updateRequeueAfter(&requeueOrNot, time.Second*10)
		} else if solrCloud == nil {
			requeueOrNot = reconcile.Result{}
		} else {
			// Only persist if the backup CRD is not finished (something bad happened)
			// and the collection backups are all complete (not necessarily successful)
			// Do not do this right after the collectionsBackup have been complete, wait till the next cycle
			if allCollectionsComplete && !backup.Status.Current.Finished {
				if backup.Spec.Persistence != nil {
					// We will count on the Job updates to be notified
					requeueOrNot = reconcile.Result{}
					err = r.persistSolrCloudBackups(ctx, backup, solrCloud, logger)
					if err != nil {
						logger.Error(err, "Error while persisting SolrCloud backup")
					}
				} else {
					tru := true
					backup.Status.Current.Successful = &tru
					now := metav1.Now()
					backup.Status.Current.FinishTime = &now
					// No need to requeue for backup purposes (unless for the next backup, handled at the end of the reconcile)
					requeueOrNot = reconcile.Result{}
					if backup.Spec.Recurrence != nil {
						// Add the current backup to the front of the history.
						// If there is no max
						backup.Status.History = append([]solrv1beta1.IndividualSolrBackupStatus{backup.Status.Current}, backup.Status.History...)

						// Remove history if we have too much saved
						if len(backup.Status.History) > backup.Spec.Recurrence.MaxSaved {
							backup.Status.History = backup.Status.History[:backup.Spec.Recurrence.MaxSaved]
						}

						if nextRestartTime, err1 := util.ScheduleNextBackup(backup.Spec.Recurrence.Schedule, backup.Status.Current.FinishTime.Time); err1 != nil {
							logger.Error(err1, "Could not schedule new backup due to back schedule")
						} else {
							convTime := metav1.NewTime(nextRestartTime)
							backup.Status.NextScheduledTime = &convTime
						}

						// Reset Current, which is fine since it is now in the history.
						backup.Status.Current = solrv1beta1.IndividualSolrBackupStatus{}
					}
				}
			}
		}

		if backup.Status.Current.Finished && backup.Status.Current.FinishTime == nil {
			now := metav1.Now()
			backup.Status.Current.FinishTime = &now
			if backup.Spec.Persistence != nil {
				backup.Status.Current.Successful = backup.Status.Current.PersistenceStatus.Successful
			}
		}

		if !reflect.DeepEqual(oldStatus, &backup.Status) {
			logger.Info("Updating status for solr-backup")
			err = r.Status().Update(ctx, backup)
		}

		if backup.Status.NextScheduledTime != nil {
			updateRequeueAfter(&requeueOrNot, backup.Status.NextScheduledTime.Sub(time.Now()))
		}
	}

	return requeueOrNot, err
}

func (r *SolrBackupReconciler) reconcileSolrCloudBackup(ctx context.Context, backup *solrv1beta1.SolrBackup, logger logr.Logger) (solrCloud *solrv1beta1.SolrCloud, collectionBackupsFinished bool, actionTaken bool, err error) {
	// Get the solrCloud that this backup is for.
	solrCloud = &solrv1beta1.SolrCloud{}

	err = r.Get(ctx, types.NamespacedName{Namespace: backup.Namespace, Name: backup.Spec.SolrCloud}, solrCloud)
	if err != nil && errors.IsNotFound(err) {
		logger.Error(err, "Could not find cloud to backup", "solrCloud", backup.Spec.SolrCloud)
		return nil, collectionBackupsFinished, actionTaken, err
	} else if err != nil {
		return nil, collectionBackupsFinished, actionTaken, err
	}

	var httpHeaders map[string]string
	if solrCloud.Spec.SolrSecurity != nil {
		basicAuthSecret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: solrCloud.BasicAuthSecretName(), Namespace: solrCloud.Namespace}, basicAuthSecret); err != nil {
			return nil, collectionBackupsFinished, actionTaken, err
		}
		httpHeaders = map[string]string{"Authorization": util.BasicAuthHeader(basicAuthSecret)}
	}

	// First check if the collection backups have been completed
	collectionBackupsFinished = util.CheckStatusOfCollectionBackups(backup)

	// If the collectionBackups are complete, then nothing else has to be done here
	if collectionBackupsFinished {
		return solrCloud, collectionBackupsFinished, actionTaken, nil
	}

	actionTaken = true
	backupRepository := util.GetBackupRepositoryByName(solrCloud.Spec.BackupRepositories, backup.Spec.RepositoryName)
	if backupRepository == nil {
		err = fmt.Errorf("Unable to find backup repository to use for backup [%s] (which specified the repository"+
			" [%s]).  solrcloud must define a repository matching that name (or have only 1 repository defined).",
			backup.Name, backup.Spec.RepositoryName)
		return solrCloud, collectionBackupsFinished, actionTaken, err
	}

	// This should only occur before the backup processes have been started
	if backup.Status.Current.StartTime.IsZero() {
		// Prep the backup directory in the persistentVolume
		err = util.EnsureDirectoryForBackup(solrCloud, backupRepository, backup, r.Config)
		if err != nil {
			return solrCloud, collectionBackupsFinished, actionTaken, err
		}

		// Make sure that all solr nodes are active and have the backupRestore shared volume mounted
		// TODO: we do not need all replicas to be healthy. We should just check that leaders exist for all shards. (or just let Solr do that)
		cloudReady := solrCloud.Status.BackupRestoreReady && (solrCloud.Status.Replicas == solrCloud.Status.ReadyReplicas)
		if !cloudReady {
			logger.Info("Cloud not ready for backup backup", "solrCloud", solrCloud.Name)
			return solrCloud, collectionBackupsFinished, actionTaken, errors.NewServiceUnavailable("Cloud is not ready for backups or restores")
		}

		// Only set the solr version at the start of the backup. This shouldn't change throughout the backup.
		backup.Status.Current.SolrVersion = solrCloud.Status.Version
		backup.Status.Current.StartTime = metav1.Now()
	}

	// Go through each collection specified and reconcile the backup.
	for _, collection := range backup.Spec.Collections {
		_, err = reconcileSolrCollectionBackup(backup, solrCloud, backupRepository, collection, httpHeaders, logger)
	}

	// First check if the collection backups have been completed
	collectionBackupsFinished = util.CheckStatusOfCollectionBackups(backup)

	return solrCloud, collectionBackupsFinished, actionTaken, err
}

func reconcileSolrCollectionBackup(backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud, backupRepository *solrv1beta1.SolrBackupRepository, collection string, httpHeaders map[string]string, logger logr.Logger) (finished bool, err error) {
	now := metav1.Now()
	collectionBackupStatus := solrv1beta1.CollectionBackupStatus{}
	collectionBackupStatus.Collection = collection
	backupIndex := -1
	// Get the backup status for this collection, if one exists
	for i, status := range backup.Status.Current.CollectionBackupStatuses {
		if status.Collection == collection {
			collectionBackupStatus = status
			backupIndex = i
		}
	}

	// If the collection backup hasn't started, start it
	if !collectionBackupStatus.InProgress && !collectionBackupStatus.Finished {
		// Start the backup by calling solr
		started, err := util.StartBackupForCollection(solrCloud, backupRepository, backup, collection, httpHeaders, logger)
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
		finished, successful, asyncStatus, err := util.CheckBackupForCollection(solrCloud, collection, backup.Name, httpHeaders, logger)
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

			err = util.DeleteAsyncInfoForBackup(solrCloud, collection, backup.Name, httpHeaders, logger)
		} else {
			collectionBackupStatus.AsyncBackupStatus = asyncStatus
		}
	}

	if backupIndex < 0 {
		backup.Status.Current.CollectionBackupStatuses = append(backup.Status.Current.CollectionBackupStatuses, collectionBackupStatus)
	} else {
		backup.Status.Current.CollectionBackupStatuses[backupIndex] = collectionBackupStatus
	}

	return collectionBackupStatus.Finished, err
}

func (r *SolrBackupReconciler) persistSolrCloudBackups(ctx context.Context, backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud, logger logr.Logger) (err error) {
	if backup.Status.Current.PersistenceStatus == nil {
		backup.Status.Current.PersistenceStatus = &solrv1beta1.BackupPersistenceStatus{}
	}
	if backup.Status.Current.PersistenceStatus.Finished {
		return nil
	}
	now := metav1.Now()

	backupRepository := util.GetBackupRepositoryByName(solrCloud.Spec.BackupRepositories, backup.Spec.RepositoryName)
	if backupRepository == nil {
		err = fmt.Errorf("Unable to find backup repository to use for backup [%s] (which specified the repository"+
			" [%s]).  solrcloud must define a repository matching that name (or have only 1 repository defined).",
			backup.Name, backup.Spec.RepositoryName)
		return err
	}

	if util.IsRepoManaged(backupRepository) {
		persistenceJob := util.GenerateBackupPersistenceJobForCloud(backupRepository, backup, solrCloud)
		if err := controllerutil.SetControllerReference(backup, persistenceJob, r.Scheme); err != nil {
			return err
		}

		foundPersistenceJob := &batchv1.Job{}
		err = r.Get(ctx, types.NamespacedName{Name: persistenceJob.Name, Namespace: persistenceJob.Namespace}, foundPersistenceJob)
		if err == nil && !backup.Status.Current.PersistenceStatus.InProgress {
		} else if err != nil && errors.IsNotFound(err) {
			logger.Info("Creating Persistence Job", "job", persistenceJob.Name)
			err = r.Create(ctx, persistenceJob)
			backup.Status.Current.PersistenceStatus.InProgress = true
			if backup.Status.Current.PersistenceStatus.StartTime == nil {
				backup.Status.Current.PersistenceStatus.StartTime = &now
			}
		} else if err != nil {
			return err
		} else {
			backup.Status.Current.PersistenceStatus.FinishTime = foundPersistenceJob.Status.CompletionTime
			tru := true
			fals := false
			numFailLimit := int32(0)
			if foundPersistenceJob.Spec.BackoffLimit != nil {
				numFailLimit = *foundPersistenceJob.Spec.BackoffLimit
			}
			if foundPersistenceJob.Status.Succeeded > 0 {
				backup.Status.Current.PersistenceStatus.Successful = &tru
			} else if foundPersistenceJob.Status.Failed > numFailLimit {
				backup.Status.Current.PersistenceStatus.Successful = &fals
			}

			if backup.Status.Current.PersistenceStatus.Successful != nil {
				backup.Status.Current.PersistenceStatus.InProgress = false
				backup.Status.Current.PersistenceStatus.Finished = true
				backup.Status.Current.PersistenceStatus.FinishTime = &now
				backup.Status.Current.Finished = true
				backup.Status.Current.Successful = backup.Status.Current.PersistenceStatus.Successful
			}
		}
		return err
	} else {
		return nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolrBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
