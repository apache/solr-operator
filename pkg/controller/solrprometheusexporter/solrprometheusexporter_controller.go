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

package solrprometheusexporter

import (
	"context"
	solrv1beta1 "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	"github.com/bloomberg/solr-operator/pkg/controller/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

// Add creates a new SolrPrometheusExporter Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSolrPrometheusExporter{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("solrprometheusexporter-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to SolrPrometheusExporter
	err = c.Watch(&source.Kind{Type: &solrv1beta1.SolrPrometheusExporter{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &solrv1beta1.SolrPrometheusExporter{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSolrPrometheusExporter{}

// ReconcileSolrPrometheusExporter reconciles a SolrPrometheusExporter object
type ReconcileSolrPrometheusExporter struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a SolrPrometheusExporter object and makes changes based on the state read
// and what is in the SolrPrometheusExporter.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments & config maps and read solrClouds
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrprometheusexporters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrprometheusexporters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcloud,verbs=get;list;watch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrcloud/status,verbs=get
func (r *ReconcileSolrPrometheusExporter) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the SolrPrometheusExporter instance
	prometheusExporter := &solrv1beta1.SolrPrometheusExporter{}
	err := r.Get(context.TODO(), request.NamespacedName, prometheusExporter)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if prometheusExporter.Spec.Config != "" {
		// Generate ConfigMap
		configMap := util.GenerateMetricsConfigMap(prometheusExporter)
		if err := controllerutil.SetControllerReference(prometheusExporter, configMap, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		// Check if the ConfigMap already exists
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating PrometheusExporter ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			err = r.Create(context.TODO(), configMap)
		} else if err == nil && util.CopyConfigMapFields(configMap, foundConfigMap) {
			// Update the found ConfigMap and write the result back if there are any changes
			log.Info("Updating PrometheusExporter ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			err = r.Update(context.TODO(), foundConfigMap)
		}
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Get the ZkConnectionString to connect to
	solrConnectionInfo := util.SolrConnectionInfo{}
	if solrConnectionInfo, err = getSolrConnectionInfo(r, prometheusExporter); err != nil {
		return reconcile.Result{}, err
	}

	deploy := util.GenerateSolrPrometheusExporterDeployment(prometheusExporter, solrConnectionInfo)
	if err := controllerutil.SetControllerReference(prometheusExporter, deploy, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	foundDeploy := &appsv1.Deployment{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, foundDeploy)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating PrometheusExporter Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
		err = r.Create(context.TODO(), deploy)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
		if !util.CopyDeploymentFields(deploy, foundDeploy) {
			log.Info("Updating PrometheusExporter Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
			err = r.Update(context.TODO(), foundDeploy)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		ready := foundDeploy.Status.ReadyReplicas > 0

		if ready != prometheusExporter.Status.Ready {
			prometheusExporter.Status.Ready = ready
			log.Info("Updating status for solr-prometheus-exporter", "namespace", prometheusExporter.Namespace, "name", prometheusExporter.Name)
			err = r.Status().Update(context.TODO(), prometheusExporter)
		}
	}
	return reconcile.Result{}, err
}

func getSolrConnectionInfo(r *ReconcileSolrPrometheusExporter, prometheusExporter *solrv1beta1.SolrPrometheusExporter) (solrConnectionInfo util.SolrConnectionInfo, err error) {
	solrConnectionInfo = util.SolrConnectionInfo{}

	if prometheusExporter.Spec.SolrReference.Standalone != nil {
		solrConnectionInfo.StandaloneAddress = prometheusExporter.Spec.SolrReference.Standalone.Address
	}
	if prometheusExporter.Spec.SolrReference.Cloud != nil {
		if prometheusExporter.Spec.SolrReference.Cloud.ZookeeperConnectionInfo != nil {
			solrConnectionInfo.CloudZkConnnectionString = prometheusExporter.Spec.SolrReference.Cloud.ZookeeperConnectionInfo.ZkConnectionString()
		} else if prometheusExporter.Spec.SolrReference.Cloud.KubeSolr != nil {
			solrCloud := &solrv1beta1.SolrCloud{}
			err = r.Get(context.TODO(), *prometheusExporter.Spec.SolrReference.Cloud.KubeSolr, solrCloud)
			if err == nil {
				solrConnectionInfo.CloudZkConnnectionString = solrCloud.Status.ZookeeperConnectionInfo.ZkConnectionString()
			}
		}
	}
	return solrConnectionInfo, err
}
