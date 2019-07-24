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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

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

	// Watch for changes to SolrCloud
	err = c.Watch(&source.Kind{Type: &solrv1beta1.SolrCloud{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &solrv1beta1.SolrBackup{},
	})
	if err != nil {
		return err
	}

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
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
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

	// Define the desired PVC object
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name + "-backup-pvc",
			Namespace: backup.Namespace,
		},
	}
	pvc.Spec = backup.Spec.PersistentVolumeClaimSpec
	if err := controllerutil.SetControllerReference(backup, pvc, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the PVC already exists
	foundPVC := &corev1.PersistentVolumeClaim{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, foundPVC)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating PVC for backup data", "namespace", pvc.Namespace, "name", pvc.Name)
		err = r.Create(context.TODO(), pvc)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	started := true
	// Start the backup
	if !backup.Status.Started {
		started = false
		if backup.Spec.SolrCloud != "" {
			started, err = reconcileSolrCloudBackup(r, backup)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}


	err = reconcileSolrBackupStatus(r, backup, foundPVC, started)

	return reconcile.Result{}, err
}


func reconcileSolrBackupStatus(r *ReconcileSolrBackup, backup *solrv1beta1.SolrBackup, pvc *corev1.PersistentVolumeClaim, started bool) (err error) {
	update := false

	if backup.Status.PVC == "" {
		backup.Status.PVC = pvc.Name
		update = true
	}

	if backup.Status.PVCPhase != pvc.Status.Phase {
		backup.Status.PVCPhase = pvc.Status.Phase
		update = true
	}

	if backup.Status.Started != started {
		backup.Status.Started = started
		update = true
	}

	if update {
		err = r.Status().Update(context.Background(), backup)
	}

	return err
}

func reconcileSolrCloudBackup(r *ReconcileSolrBackup, backup *solrv1beta1.SolrBackup) (started bool, err error) {
	// Get list of backups for the given cloud
	solrCloud := &solrv1beta1.SolrCloud{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: backup.Namespace, Name: backup.Spec.SolrCloud}, solrCloud)
	if err != nil && errors.IsNotFound(err) {
		log.Error(err, "Could not find cloud to backup", "namespace", backup.Namespace, "backupName", backup.Name, "solrCloudName", backup.Spec.SolrCloud)
		return false, err
	} else if err != nil {
		return false, err
	}

	canStart := false
	for _, b := range solrCloud.Status.BackupsConnected {
		if b == backup.Name {
			canStart = true
		}
	}

	return canStart, nil
}