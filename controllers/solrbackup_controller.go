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
	"k8s.io/apimachinery/pkg/fields"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	"github.com/apache/solr-operator/controllers/util"
	"github.com/go-logr/logr"
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
	Config *rest.Config
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

	changed := backup.WithDefaults()
	if changed {
		logger.Info("Setting default settings for solr-backup")
		if err = r.Update(ctx, backup); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	oldStatus := backup.Status.DeepCopy()

	requeueOrNot := reconcile.Result{}

	var backupNeedsToWait bool

	// Check if we should start the next backup
	if backup.Status.NextScheduledTime != nil {
		// If the backup no longer enabled, remove the next scheduled time
		if !backup.Spec.Recurrence.IsEnabled() {
			backup.Status.NextScheduledTime = nil
			backupNeedsToWait = false
		} else if backup.Status.NextScheduledTime.UTC().Before(time.Now().UTC()) {
			// We have hit the next scheduled restart time.
			backupNeedsToWait = false
			backup.Status.NextScheduledTime = nil

			// Add the current backup to the front of the history.
			// If there is no max
			backup.Status.History = append([]solrv1beta1.IndividualSolrBackupStatus{backup.Status.IndividualSolrBackupStatus}, backup.Status.History...)

			// Remove history if we have too much saved
			if len(backup.Status.History) > backup.Spec.Recurrence.MaxSaved {
				backup.Status.History = backup.Status.History[:backup.Spec.Recurrence.MaxSaved]
			}

			// Reset Current, which is fine since it is now in the history.
			backup.Status.IndividualSolrBackupStatus = solrv1beta1.IndividualSolrBackupStatus{}
		} else {
			// If we have not hit the next scheduled restart, wait to requeue until that is true.
			updateRequeueAfter(&requeueOrNot, backup.Status.NextScheduledTime.UTC().Sub(time.Now().UTC()))
			backupNeedsToWait = true
		}
	} else {
		backupNeedsToWait = false
	}

	// Do backup work if we are not waiting and the current backup is not finished
	if !backupNeedsToWait && !backup.Status.IndividualSolrBackupStatus.Finished {
		solrCloud, _, err1 := r.reconcileSolrCloudBackup(ctx, backup, &backup.Status.IndividualSolrBackupStatus, logger)
		if err1 != nil {
			// TODO Should we be failing the backup for some sub-set of errors here?
			logger.Error(err1, "Error while taking SolrCloud backup")

			// Requeue after 10 seconds for errors.
			updateRequeueAfter(&requeueOrNot, time.Second*10)
		} else if backup.Status.IndividualSolrBackupStatus.Finished {
			// Set finish time
			now := metav1.Now()
			backup.Status.IndividualSolrBackupStatus.FinishTime = &now
		} else if solrCloud != nil {
			// When working with the collection backups, auto-requeue after 5 seconds
			// to check on the status of the async solr backup calls
			updateRequeueAfter(&requeueOrNot, time.Second*5)
		}
	}

	// Schedule the next backupTime, if it doesn't have a next scheduled time, it has recurrence and the current backup is finished
	if backup.Status.IndividualSolrBackupStatus.Finished {
		if nextBackupTime, err1 := util.ScheduleNextBackup(backup.Spec.Recurrence.Schedule, backup.Status.IndividualSolrBackupStatus.StartTime.Time); err1 != nil {
			logger.Error(err1, "Could not update backup scheduling due to bad cron schedule", "cron", backup.Spec.Recurrence.Schedule)
		} else {
			convTime := metav1.NewTime(nextBackupTime)
			if backup.Status.NextScheduledTime == nil || convTime != *backup.Status.NextScheduledTime {
				// Only log out the message if there is a change in NextScheduled
				logger.Info("(Re)scheduling Next Backup", "time", nextBackupTime)
				backup.Status.NextScheduledTime = &convTime
				updateRequeueAfter(&requeueOrNot, backup.Status.NextScheduledTime.Sub(time.Now()))
			}
		}
	}

	if !reflect.DeepEqual(*oldStatus, backup.Status) {
		logger.Info("Updating status for solr-backup", "newStatus", backup.Status, "oldStatus", oldStatus)
		err = r.Status().Update(ctx, backup)
	}

	return requeueOrNot, err
}

func (r *SolrBackupReconciler) reconcileSolrCloudBackup(ctx context.Context, backup *solrv1beta1.SolrBackup, currentBackupStatus *solrv1beta1.IndividualSolrBackupStatus, logger logr.Logger) (solrCloud *solrv1beta1.SolrCloud, actionTaken bool, err error) {
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
		ctx, err = util.AddAuthToContext(ctx, &r.Client, solrCloud)
		if err != nil {
			return nil, actionTaken, err
		}
	}

	// First check if the collection backups have been completed
	collectionBackupsFinished := util.UpdateStatusOfCollectionBackups(currentBackupStatus)

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
	if currentBackupStatus.StartTime.IsZero() {
		// Prep the backup directory in the persistentVolume
		err = util.EnsureDirectoryForBackup(solrCloud, backupRepository, backup, r.Config)
		if err != nil {
			return solrCloud, actionTaken, err
		}

		// Make sure that all solr living Solr pods have the backupRepo configured
		if !solrCloud.Status.BackupRepositoriesAvailable[backupRepository.Name] {
			logger.Info("Cloud not ready for backup", "solrCloud", solrCloud.Name, "repository", backupRepository.Name)
			return solrCloud, actionTaken, errors.NewServiceUnavailable(fmt.Sprintf("Cloud is not ready for backups in the %s repository", backupRepository.Name))
		}

		// Only set the solr version at the start of the backup. This shouldn't change throughout the backup.
		currentBackupStatus.SolrVersion = solrCloud.Status.Version
		currentBackupStatus.StartTime = metav1.Now()
	}

	// Go through each collection specified and reconcile the backup.
	for _, collection := range backup.Spec.Collections {
		// This will in-place update the CollectionBackupStatus in the backup object
		if _, err = reconcileSolrCollectionBackup(ctx, backup, currentBackupStatus, solrCloud, backupRepository, collection, logger); err != nil {
			break
		}
	}

	// First check if the collection backups have been completed
	util.UpdateStatusOfCollectionBackups(currentBackupStatus)

	return solrCloud, actionTaken, err
}

func reconcileSolrCollectionBackup(ctx context.Context, backup *solrv1beta1.SolrBackup, currentBackupStatus *solrv1beta1.IndividualSolrBackupStatus, solrCloud *solrv1beta1.SolrCloud, backupRepository *solrv1beta1.SolrBackupRepository, collection string, logger logr.Logger) (finished bool, err error) {
	now := metav1.Now()
	collectionBackupStatus := solrv1beta1.CollectionBackupStatus{}
	collectionBackupStatus.Collection = collection
	backupIndex := -1
	// Get the backup status for this collection, if one exists
	for i, status := range currentBackupStatus.CollectionBackupStatuses {
		if status.Collection == collection {
			collectionBackupStatus = status
			backupIndex = i
		}
	}

	// If the collection backup hasn't started, start it
	if collectionBackupStatus.Finished {
		return true, nil
	} else if !collectionBackupStatus.InProgress {
		// Start the backup by calling solr
		var started bool
		started, err = util.StartBackupForCollection(ctx, solrCloud, backupRepository, backup, collection, logger)
		if err != nil {
			return true, err
		}
		collectionBackupStatus.InProgress = started
		if started && collectionBackupStatus.StartTime == nil {
			collectionBackupStatus.StartTime = &now
		}
		collectionBackupStatus.BackupName = util.FullCollectionBackupName(collection, backup.Name)
	} else if collectionBackupStatus.InProgress {
		var successful bool
		var asyncStatus string
		// Check the state of the backup, when it is in progress, and update the state accordingly
		finished, successful, asyncStatus, err = util.CheckBackupForCollection(ctx, solrCloud, collection, backup.Name, logger)
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
		currentBackupStatus.CollectionBackupStatuses = append(currentBackupStatus.CollectionBackupStatuses, collectionBackupStatus)
	} else {
		currentBackupStatus.CollectionBackupStatuses[backupIndex] = collectionBackupStatus
	}

	return collectionBackupStatus.Finished, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolrBackupReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	r.Config = mgr.GetConfig()

	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrBackup{})

	ctrlBuilder, err = r.indexAndWatchForSolrClouds(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	return ctrlBuilder.Complete(r)
}

func (r *SolrBackupReconciler) indexAndWatchForSolrClouds(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	solrCloudField := ".spec.solrCloud"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrBackup{}, solrCloudField, func(rawObj client.Object) []string {
		// grab the SolrBackup object, extract the used SolrCloud...
		return []string{rawObj.(*solrv1beta1.SolrBackup).Spec.SolrCloud}
	}); err != nil {
		return ctrlBuilder, err
	}

	return ctrlBuilder.Watches(
		&source.Kind{Type: &solrv1beta1.SolrCloud{}},
		handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			solrCloud := obj.(*solrv1beta1.SolrCloud)
			foundBackups := &solrv1beta1.SolrBackupList{}
			listOps := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(solrCloudField, obj.GetName()),
				Namespace:     obj.GetNamespace(),
			}
			err := r.List(context.Background(), foundBackups, listOps)
			if err != nil {
				// if no exporters found, just no-op this
				return []reconcile.Request{}
			}

			requests := make([]reconcile.Request, 0)
			for _, item := range foundBackups.Items {
				// Only queue the request if the Cloud is ready.
				cloudIsReady := solrCloud.Status.BackupRestoreReady
				if item.Spec.RepositoryName != "" {
					cloudIsReady = solrCloud.Status.BackupRepositoriesAvailable[item.Spec.RepositoryName]
				}
				if cloudIsReady {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      item.GetName(),
							Namespace: item.GetNamespace(),
						},
					})
				}
			}
			return requests
		}),
		builder.WithPredicates(predicate.GenerationChangedPredicate{})), nil
}
