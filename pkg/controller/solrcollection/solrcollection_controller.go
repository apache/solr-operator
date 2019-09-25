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

package solrcollection

import (
	"context"
	"reflect"
	"time"

	solrv1beta1 "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	"github.com/bloomberg/solr-operator/pkg/controller/util"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new SolrCollection Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSolrCollection{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("solrcollection-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to SolrCollection
	err = c.Watch(&source.Kind{Type: &solrv1beta1.SolrCollection{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSolrCollection{}

// ReconcileSolrCollection reconciles a SolrCollection object
type ReconcileSolrCollection struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a SolrCollection object and makes changes based on the state read
// and what is in the SolrCollection.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcollections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcollections/status,verbs=get;update;patch
func (r *ReconcileSolrCollection) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the SolrCollection collection
	collection := &solrv1beta1.SolrCollection{}
	collectionFinalizer := "collection.finalizers.bloomberg.com"

	err := r.Get(context.TODO(), request.NamespacedName, collection)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	oldStatus := collection.Status.DeepCopy()
	requeueOrNot := reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}

	solrCloud, collectionCreationStatus, err := reconcileSolrCollection(r, collection, collection.Spec.NumShards, collection.Spec.ReplicationFactor, collection.Spec.AutoAddReplicas, collection.Spec.MaxShardsPerNode, collection.Spec.RouterName, collection.Spec.Shards, collection.Namespace)

	if err != nil {
		log.Error(err, "Error while creating SolrCloud collection")
	}

	if collection.Status.CreatedTime == nil {
		now := metav1.Now()
		collection.Status.CreatedTime = &now
	}

	if solrCloud != nil && collectionCreationStatus == false {
		log.Info("Collections update failed")
	}

	if collection.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object
		if !util.ContainsString(collection.ObjectMeta.Finalizers, collectionFinalizer) {
			collection.ObjectMeta.Finalizers = append(collection.ObjectMeta.Finalizers, collectionFinalizer)
			if err := r.Update(context.Background(), collection); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if util.ContainsString(collection.ObjectMeta.Finalizers, collectionFinalizer) {
			log.Info("Deleting Solr collection", "cloud", collection.Spec.SolrCloud, "namespace", collection.Namespace, "Collection Name", collection.Name)
			// our finalizer is present, so lets handle our external dependency
			delete, err := util.DeleteCollection(collection.Spec.SolrCloud, collection.Name, collection.Namespace)
			if err != nil {
				log.Error(err, "Failed to delete Solr collection")
				return reconcile.Result{}, err
			}

			log.Info("Deleted Solr collection", "cloud", collection.Spec.SolrCloud, "namespace", collection.Namespace, "Collection Name", collection.Name, "Deleted", delete)

		}

		// remove our finalizer from the list and update it.
		collection.ObjectMeta.Finalizers = util.RemoveString(collection.ObjectMeta.Finalizers, collectionFinalizer)
		if err := r.Update(context.Background(), collection); err != nil {
			return reconcile.Result{}, err
		}
	}

	if !reflect.DeepEqual(oldStatus, collection.Status) {
		log.Info("Updating status for solr-collection", "collection", collection, "namespace", collection.Namespace, "name", collection.Name)
		err = r.Status().Update(context.TODO(), collection)
	}

	if collection.Status.InProgressCreation {
		if util.CheckIfCollectionExists(collection.Spec.SolrCloud, collection.Spec.Collection, collection.Namespace) {
			log.Info("Collection exists, creation complete", "collection", collection, "namespace", collection.Namespace, "name", collection.Name)
			collection.Status.InProgressCreation = false
			requeueOrNot = reconcile.Result{}
		} else {
			log.Info("Collection creation still in progress", "collection", collection, "namespace", collection.Namespace, "name", collection.Name)
			requeueOrNot = reconcile.Result{Requeue: true}
		}
	}
	if collection.Status.Created {
		requeueOrNot = reconcile.Result{}
	}

	return requeueOrNot, nil
}

func reconcileSolrCollection(r *ReconcileSolrCollection, collection *solrv1beta1.SolrCollection, numShards int64, replicationFactor int64, autoAddReplicas bool, maxShardsPerNode int64, routerName string, shards string, namespace string) (solrCloud *solrv1beta1.SolrCloud, collectionCreationStatus bool, err error) {
	// Get the solrCloud that this collection is for.
	solrCloud = &solrv1beta1.SolrCloud{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: collection.Namespace, Name: collection.Spec.SolrCloud}, solrCloud)

	// If the collection has already been created already and requires modification
	if collection.Status.Created {
		modificationRequired, err := util.CheckIfCollectionModificationRequired(solrCloud.Name, collection.Name, replicationFactor, autoAddReplicas, maxShardsPerNode, namespace)

		if err != nil {
			return nil, false, err
		}

		if modificationRequired {
			modify, err := util.ModifyCollection(solrCloud.Name, collection.Name, replicationFactor, autoAddReplicas, maxShardsPerNode, namespace)

			if err != nil {
				return nil, false, err
			}

			log.Info("Modified Solr collection", "SolrCloud", collection.Spec.SolrCloud, "namespace", collection.Namespace, "Collection Name", collection.Name, "Modified", modify)
		} else {
			log.Info("Modification on Solr collection not required", "SolrCloud", collection.Spec.SolrCloud, "namespace", collection.Namespace, "Collection Name", collection.Name, "Modified", nil)
		}

	}

	// If the collection collection hasn't been created or is in progress, start it creation
	if !collection.Status.Created && !collection.Status.InProgressCreation {

		// Request the creation of collection by calling solr
		collection.Status.InProgressCreation = true
		create, err := util.CreateCollection(solrCloud.Name, collection.Name, numShards, replicationFactor, autoAddReplicas, routerName, shards, namespace)
		if err != nil {
			collection.Status.InProgressCreation = false
			return nil, false, err
		}
		collection.Status.Created = create
		collection.Status.InProgressCreation = false
	}

	err = r.Get(context.TODO(), types.NamespacedName{Namespace: collection.Namespace, Name: collection.Spec.SolrCloud}, solrCloud)

	return solrCloud, collection.Status.Created, err
}
