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
	"reflect"
	"time"

	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	solrv1beta1 "github.com/bloomberg/solr-operator/api/v1beta1"
)

// SolrBackupReconciler reconciles a SolrBackup object
type SolrBackupReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme
	config *rest.Config
}

// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds,verbs=get;list;watch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds/status,verbs=get
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrbackups/status,verbs=get;update;patch

func (r *SolrBackupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("solrbackup", req.NamespacedName)
	// Fetch the SolrBackup instance
	backup := &solrv1beta1.SolrBackup{}
	err := r.Get(context.TODO(), req.NamespacedName, backup)
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
		r.Log.Info("Setting default settings for solr-backup", "namespace", backup.Namespace, "name", backup.Name)
		if err := r.Update(context.TODO(), backup); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// When working with the collection backups, auto-requeue after 5 seconds
	// to check on the status of the async solr backup calls
	requeueOrNot := reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}

	solrCloud, allCollectionsComplete, collectionActionTaken, err := reconcileSolrCloudBackup(r, backup)
	if err != nil {
		r.Log.Error(err, "Error while taking SolrCloud backup")
	}
	if allCollectionsComplete && collectionActionTaken {
		// Requeue immediately to start the persisting job
		// From here on in the backup lifecycle, requeueing will not happen for the backup.
		requeueOrNot = reconcile.Result{RequeueAfter: time.Second * 10}
	} else if solrCloud == nil {
		requeueOrNot = reconcile.Result{}
	} else {
		// Only persist if the backup CRD is not finished (something bad happended)
		// and the collection backups are all complete (not necessarily successful)
		// Do not do this right after the collectionsBackup have been complete, wait till the next cycle
		if allCollectionsComplete && !backup.Status.Finished {
			// We will count on the Job updates to be notifified
			requeueOrNot = reconcile.Result{}
			err = persistSolrCloudBackups(r, backup, solrCloud)
			if err != nil {
				r.Log.Error(err, "Error while persisting SolrCloud backup")
			}
		}
	}

	if backup.Status.Finished && backup.Status.FinishTime == nil {
		now := metav1.Now()
		backup.Status.FinishTime = &now
		backup.Status.Successful = backup.Status.PersistenceStatus.Successful
	}

	if !reflect.DeepEqual(oldStatus, backup.Status) {
		r.Log.Info("Updating status for solr-backup", "namespace", backup.Namespace, "name", backup.Name)
		err = r.Status().Update(context.TODO(), backup)
	}

	if backup.Status.Finished {
		requeueOrNot = reconcile.Result{}
	}

	return requeueOrNot, err
}

func reconcileSolrCloudBackup(r *SolrBackupReconciler, backup *solrv1beta1.SolrBackup) (solrCloud *solrv1beta1.SolrCloud, collectionBackupsFinished bool, actionTaken bool, err error) {
	// Get the solrCloud that this backup is for.
	solrCloud = &solrv1beta1.SolrCloud{}

	err = r.Get(context.TODO(), types.NamespacedName{Namespace: backup.Namespace, Name: backup.Spec.SolrCloud}, solrCloud)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Error(err, "Could not find cloud to backup", "namespace", backup.Namespace, "backupName", backup.Name, "solrCloudName", backup.Spec.SolrCloud)
		return nil, collectionBackupsFinished, actionTaken, err
	} else if err != nil {
		return nil, collectionBackupsFinished, actionTaken, err
	}

	// First check if the collection backups have been completed
	collectionBackupsFinished = util.CheckStatusOfCollectionBackups(backup)

	// If the collectionBackups are complete, then nothing else has to be done here
	if collectionBackupsFinished {
		return solrCloud, collectionBackupsFinished, actionTaken, nil
	}

	actionTaken = true

	// This should only occur before the backup processes have been started
	if backup.Status.SolrVersion == "" {
		// Prep the backup directory in the persistentVolume
		err := util.EnsureDirectoryForBackup(solrCloud, backup.Name, r.config)
		if err != nil {
			return solrCloud, collectionBackupsFinished, actionTaken, err
		}

		// Make sure that all solr nodes are active and have the backupRestore shared volume mounted
		cloudReady := solrCloud.Status.BackupRestoreReady && (solrCloud.Status.Replicas == solrCloud.Status.ReadyReplicas)
		if !cloudReady {
			r.Log.Info("Cloud not ready for backup backup", "namespace", backup.Namespace, "cloud", solrCloud.Name, "backup", backup.Name)
			return solrCloud, collectionBackupsFinished, actionTaken, errors.NewServiceUnavailable("Cloud is not ready for backups or restores")
		}

		// Only set the solr version at the start of the backup. This shouldn't change throughout the backup.
		backup.Status.SolrVersion = solrCloud.Status.Version
	}

	// Go through each collection specified and reconcile the backup.
	for _, collection := range backup.Spec.Collections {
		_, err = reconcileSolrCollectionBackup(backup, solrCloud, collection)
	}

	// First check if the collection backups have been completed
	collectionBackupsFinished = util.CheckStatusOfCollectionBackups(backup)

	return solrCloud, collectionBackupsFinished, actionTaken, err
}

func reconcileSolrCollectionBackup(backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud, collection string) (finished bool, err error) {
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
		started, err := util.StartBackupForCollection(solrCloud.Name, collection, backup.Name, backup.Namespace)
		if err != nil {
			return true, err
		}
		collectionBackupStatus.InProgress = started
		if started && collectionBackupStatus.StartTime == nil {
			collectionBackupStatus.StartTime = &now
		}
	} else if collectionBackupStatus.InProgress {
		// Check the state of the backup, when it is in progress, and update the state accordingly
		finished, successful, asyncStatus, error := util.CheckBackupForCollection(solrCloud.Name, collection, backup.Name, backup.Namespace)
		if error != nil {
			return false, error
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

			err = util.DeleteAsyncInfoForBackup(solrCloud.Name, collection, backup.Name, backup.Namespace)
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

func persistSolrCloudBackups(r *SolrBackupReconciler, backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud) (err error) {
	if backup.Status.PersistenceStatus.Finished {
		return nil
	}
	now := metav1.Now()

	persistenceJob := util.GenerateBackupPersistenceJobForCloud(backup, solrCloud)
	if err := controllerutil.SetControllerReference(backup, persistenceJob, r.scheme); err != nil {
		return err
	}

	foundPersistenceJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: persistenceJob.Name, Namespace: persistenceJob.Namespace}, foundPersistenceJob)
	if err == nil && !backup.Status.PersistenceStatus.InProgress {

	}
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating Persistence Job", "namespace", persistenceJob.Namespace, "name", persistenceJob.Name)
		err = r.Create(context.TODO(), persistenceJob)
		backup.Status.PersistenceStatus.InProgress = true
		if backup.Status.PersistenceStatus.StartTime == nil {
			backup.Status.PersistenceStatus.StartTime = &now
		}
	} else if err != nil {
		return err
	} else {
		backup.Status.PersistenceStatus.FinishTime = foundPersistenceJob.Status.CompletionTime
		tru := true
		fals := false
		numFailLimit := int32(0)
		if foundPersistenceJob.Spec.BackoffLimit != nil {
			numFailLimit = *foundPersistenceJob.Spec.BackoffLimit
		}
		if foundPersistenceJob.Status.Succeeded > 0 {
			backup.Status.PersistenceStatus.Successful = &tru
		} else if foundPersistenceJob.Status.Failed > numFailLimit {
			backup.Status.PersistenceStatus.Successful = &fals
		}

		if backup.Status.PersistenceStatus.Successful != nil {
			backup.Status.PersistenceStatus.InProgress = false
			backup.Status.PersistenceStatus.Finished = true
			backup.Status.PersistenceStatus.FinishTime = &now
			backup.Status.Finished = true
			backup.Status.Successful = backup.Status.PersistenceStatus.Successful
		}
	}
	return err
}

func (r *SolrBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndReconciler(mgr, r)
}

func (r *SolrBackupReconciler) SetupWithManagerAndReconciler(mgr ctrl.Manager, reconciler reconcile.Reconciler) error {
	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrBackup{}).
		Owns(&batchv1.Job{})

	r.config = mgr.GetConfig()
	r.scheme = mgr.GetScheme()
	return ctrlBuilder.Complete(reconciler)
}
