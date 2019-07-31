/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package solrbackup

import (
	"context"
	solrv1beta1 "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	"github.com/bloomberg/solr-operator/pkg/controller/util"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

var log = logf.Log.WithName("controller")

var (
	Config *rest.Config
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SolrBackup Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSolrBackup{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("solrbackup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to SolrBackup
	err = c.Watch(&source.Kind{Type: &solrv1beta1.SolrBackup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Jobs
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &solrv1beta1.SolrBackup{},
	})
	if err != nil {
		return err
	}
	Config = mgr.GetConfig()

	return nil
}

var _ reconcile.Reconciler = &ReconcileSolrBackup{}

// ReconcileSolrBackup reconciles a SolrBackup object
type ReconcileSolrBackup struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a SolrBackup object and makes changes based on the state read
// and what is in the SolrBackup.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=persistentvolumeclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrbackups/status,verbs=get;update;patch
func (r *ReconcileSolrBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the SolrBackup instance
	backup := &solrv1beta1.SolrBackup{}
	err := r.Get(context.TODO(), request.NamespacedName, backup)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	changed := backup.WithDefaults()
	if changed {
		log.Info("Setting default settings for solr-backup", "namespace", backup.Namespace, "name", backup.Name)
		if err := r.Update(context.TODO(), backup); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	solrCloud, err := reconcileSolrCloudBackup(r, backup)

	if err == nil {
		err = persistSolrCloudBackups(r, backup, solrCloud)
	}

	if backup.Status.Finished && backup.Status.FinishTime == nil {
		now := metav1.Now()
		backup.Status.FinishTime = &now
	}
	log.Info("Updating status for solr-backup", "namespace", backup.Namespace, "name", backup.Name)
	err = r.Status().Update(context.TODO(), backup)

	return requeueOrFinish(backup), err
}

func requeueOrFinish(backup *solrv1beta1.SolrBackup) reconcile.Result {
	if backup.Status.Finished {
		return reconcile.Result{}
	} else {
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}
	}
}

func reconcileSolrCloudBackup(r *ReconcileSolrBackup, backup *solrv1beta1.SolrBackup) (solrCloud *solrv1beta1.SolrCloud, err error) {
	// Get the solrCloud that this backup is for.
	solrCloud = &solrv1beta1.SolrCloud{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: backup.Namespace, Name: backup.Spec.SolrCloud}, solrCloud)
	if err != nil && errors.IsNotFound(err) {
		log.Error(err, "Could not find cloud to backup", "namespace", backup.Namespace, "backupName", backup.Name, "solrCloudName", backup.Spec.SolrCloud)
		return nil, err
	} else if err != nil {
		return nil, err
	}
	if backup.Status.BackupsFinished {
		return solrCloud, nil
	}

	// Only set the solr version at the start of the backup. This shouldn't change throughout the backup.
	if backup.Status.SolrVersion == "" {
		backup.Status.SolrVersion = solrCloud.Status.Version
		// Prep the backup directory in the persistentVolume
		err := util.EnsureDirectoryForBackup(solrCloud, backup.Name, Config)
		if err != nil {
			return solrCloud, err
		}
	}

	// Make sure that all solr nodes are active and have the backupRestore shared volume mounted
	cloudReady := solrCloud.Status.BackupRestoreReady && (solrCloud.Status.Replicas == solrCloud.Status.ReadyReplicas)
	if !cloudReady {
		log.Info("Cloud not ready for backup backup", "namespace", backup.Namespace, "cloud", solrCloud.Name, "backup", backup.Name)
		return solrCloud, errors.NewServiceUnavailable("Cloud is not ready for backups or restores");
	}

	// Go through each collection specified and reconcile the backup.
	// If all are finished, nothing needs to be done here
	if !backup.Status.BackupsFinished {
		allFinished := true
		for _, collection := range backup.Spec.Collections {
			finished, error := reconcileSolrCollectionBackup(backup, solrCloud, collection)
			err = error
			allFinished = allFinished && finished
		}
		backup.Status.BackupsFinished = allFinished
	}

	// Check if persistence should be skipped if the backups didn't all complete successfuly
	if backup.Status.BackupsFinished {
		allSuccessful := true
		for _, collectionStatus := range backup.Status.CollectionBackupStatuses {
			if collectionStatus.Successful == nil {
				allSuccessful = false
			} else {
				allSuccessful = allSuccessful && *collectionStatus.Successful
			}
		}
		if !allSuccessful {
			backup.Status.Finished = true
		}
	}

	return solrCloud, err
}

func reconcileSolrCollectionBackup(backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud, collection string) (finished bool, err error) {
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
		if started {
			now := metav1.Now()
			collectionBackupStatus.StartTime = &now
		}
	} else if collectionBackupStatus.InProgress {
		// Check the state of the backup, when it is in progress, and update the state accordingly
		finished, successful, asyncStatus, error := util.CheckBackupForCollection(solrCloud.Name, collection, backup.Name, backup.Namespace)
		if error != nil {
			err = error
		}
		collectionBackupStatus.Finished = finished
		if finished {
			collectionBackupStatus.InProgress = false
			collectionBackupStatus.Successful = &successful
			collectionBackupStatus.AsyncBackupStatus = ""
			now := metav1.Now()
			collectionBackupStatus.FinishTime = &now

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


func persistSolrCloudBackups(r *ReconcileSolrBackup, backup *solrv1beta1.SolrBackup, solrCloud *solrv1beta1.SolrCloud) (err error) {
	if !backup.Status.BackupsFinished || backup.Status.PersistenceStatus.Finished || backup.Status.Finished {
		return nil
	}

	persistenceJob := util.GenerateBackupPersistenceJobForCloud(backup, solrCloud)
	if err := controllerutil.SetControllerReference(backup, persistenceJob, r.scheme); err != nil {
		return err
	}

	foundPersistenceJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: persistenceJob.Name, Namespace: persistenceJob.Namespace}, foundPersistenceJob)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Persistence Job", "namespace", persistenceJob.Namespace, "name", persistenceJob.Name)
		err = r.Create(context.TODO(), persistenceJob)
		backup.Status.PersistenceStatus.InProgress = true
	} else if err != nil {
		return err
	} else {
		backup.Status.PersistenceStatus.StartTime = foundPersistenceJob.Status.StartTime
		backup.Status.PersistenceStatus.FinishTime = foundPersistenceJob.Status.CompletionTime
		tru := true
		if foundPersistenceJob.Status.Succeeded > 0 {
			backup.Status.PersistenceStatus.Successful = &tru
			backup.Status.PersistenceStatus.Finished = true
			backup.Status.Finished = true
		}
		if foundPersistenceJob.Status.Failed > 0 {
			backup.Status.PersistenceStatus.Successful = &tru
			backup.Status.Finished = true
		}
	}
	return err
}