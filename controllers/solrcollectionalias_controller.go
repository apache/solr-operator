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

package controllers

import (
	"context"
	"reflect"
	"time"

	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	solrv1beta1 "github.com/bloomberg/solr-operator/api/v1beta1"
)

// SolrCollectionAliasReconciler reconciles a SolrCollectionAlias object
type SolrCollectionAliasReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcollectionaliases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcollectionaliases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcloud,verbs=get;list;watch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcloud/status,verbs=get

func (r *SolrCollectionAliasReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("solrcollectionalias", req.NamespacedName)

	alias := &solrv1beta1.SolrCollectionAlias{}
	aliasFinalizer := "alias.finalizers.bloomberg.com"

	err := r.Get(context.TODO(), req.NamespacedName, alias)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return reconcile.Result{}, err
	}

	oldStatus := alias.Status.DeepCopy()
	requeueOrNot := reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}

	aliasCreationStatus := reconcileSolrCollectionAlias(r, alias, alias.Spec.SolrCloud, alias.Name, alias.Spec.AliasType, alias.Spec.Collections, alias.Namespace)

	if err != nil {
		r.Log.Error(err, "Error while creating SolrCloud alias")
	}

	if alias.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object
		if !util.ContainsString(alias.ObjectMeta.Finalizers, aliasFinalizer) {
			alias.ObjectMeta.Finalizers = append(alias.ObjectMeta.Finalizers, aliasFinalizer)
			if err := r.Update(context.Background(), alias); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted, get associated SolrCloud
		solrCloud := &solrv1beta1.SolrCloud{}
		err = r.Get(context.TODO(), types.NamespacedName{Namespace: alias.Namespace, Name: alias.Spec.SolrCloud}, solrCloud)

		if util.ContainsString(alias.ObjectMeta.Finalizers, aliasFinalizer) && solrCloud != nil && aliasCreationStatus {
			r.Log.Info("Deleting Solr collection alias", "cloud", alias.Spec.SolrCloud, "namespace", alias.Namespace, "Collection Name", alias.Name)
			// our finalizer is present, along with the associated SolrCloud and alias lets delete alias
			delete, err := util.DeleteCollectionAlias(alias.Spec.SolrCloud, alias.Name, alias.Namespace)
			if err != nil {
				r.Log.Error(err, "Failed to delete Solr collection")
				return reconcile.Result{}, err
			}

			r.Log.Info("Deleted Solr collection", "cloud", alias.Spec.SolrCloud, "namespace", alias.Namespace, "Alias", alias.Name, "Deleted", delete)

		}

		// remove our finalizer from the list and update it.
		alias.ObjectMeta.Finalizers = util.RemoveString(alias.ObjectMeta.Finalizers, aliasFinalizer)
		if err := r.Update(context.Background(), alias); err != nil {
			return reconcile.Result{}, err
		}
	}

	if alias.Status.CreatedTime == nil {
		now := metav1.Now()
		alias.Status.CreatedTime = &now
		alias.Status.Created = aliasCreationStatus
	}

	if !reflect.DeepEqual(oldStatus, alias.Status) {
		r.Log.Info("Updating status for collection alias", "alias", alias, "namespace", alias.Namespace, "name", alias.Name)
		err = r.Status().Update(context.TODO(), alias)
	}

	if aliasCreationStatus {
		requeueOrNot = reconcile.Result{}
	}

	return requeueOrNot, nil
}

func reconcileSolrCollectionAlias(r *SolrCollectionAliasReconciler, alias *solrv1beta1.SolrCollectionAlias, solrCloudName string, aliasName string, aliasType string, collections []string, namespace string) (aliasCreationStatus bool) {
	success, aliasCollections := util.CurrentCollectionAliasDetails(solrCloudName, aliasName, namespace)

	// If not created, or if alias status differs from spec requirements create alias
	if !success || !reflect.DeepEqual(alias.Status.Collections, aliasCollections) {
		r.Log.Info("Applying collection alias", "alias", alias)
		err := util.CreateCollectionAlias(solrCloudName, aliasName, aliasType, collections, namespace)
		if err == nil {
			alias.Status.Created = true
			alias.Status.Collections = collections
			return alias.Status.Created
		}
	}

	return alias.Status.Created
}

func (r *SolrCollectionAliasReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrCollectionAlias{}).
		Complete(r)
}
