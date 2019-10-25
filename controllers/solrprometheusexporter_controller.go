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
	solrv1beta1 "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// SolrPrometheusExporterReconciler reconciles a SolrPrometheusExporter object
type SolrPrometheusExporterReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a SolrPrometheusExporter object and makes changes based on the state read
// and what is in the SolrPrometheusExporter.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments & config maps and read solrClouds
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=configmaps/status,verbs=get
// +kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=services/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds,verbs=get;list;watch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds/status,verbs=get
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrprometheusexporters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrprometheusexporters/status,verbs=get;update;patch
func (r *SolrPrometheusExporterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("solrprometheusexporter", req.NamespacedName)

	// Fetch the SolrPrometheusExporter instance
	prometheusExporter := &solrv1beta1.SolrPrometheusExporter{}
	err := r.Get(context.TODO(), req.NamespacedName, prometheusExporter)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return ctrl.Result{}, err
	}

	changed := prometheusExporter.WithDefaults()
	if changed {
		r.Log.Info("Setting default settings for Solr PrometheusExporter", "namespace", prometheusExporter.Namespace, "name", prometheusExporter.Name)
		if err := r.Update(context.TODO(), prometheusExporter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if prometheusExporter.Spec.Config != "" {
		// Generate ConfigMap
		configMap := util.GenerateMetricsConfigMap(prometheusExporter)
		if err := controllerutil.SetControllerReference(prometheusExporter, configMap, r.scheme); err != nil {
			return ctrl.Result{}, err
		}

		// Check if the ConfigMap already exists
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating PrometheusExporter ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			err = r.Create(context.TODO(), configMap)
		} else if err == nil && util.CopyConfigMapFields(configMap, foundConfigMap) {
			// Update the found ConfigMap and write the result back if there are any changes
			r.Log.Info("Updating PrometheusExporter ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			err = r.Update(context.TODO(), foundConfigMap)
		}
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Generate Metrics Service
	metricsService := util.GenerateSolrMetricsService(prometheusExporter)
	if err := controllerutil.SetControllerReference(prometheusExporter, metricsService, r.scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the Metrics Service already exists
	foundMetricsService := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: metricsService.Name, Namespace: metricsService.Namespace}, foundMetricsService)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating PrometheusExporter Service", "namespace", metricsService.Namespace, "name", metricsService.Name)
		err = r.Create(context.TODO(), metricsService)
	} else if err == nil && util.CopyServiceFields(metricsService, foundMetricsService) {
		// Update the found Metrics Service and write the result back if there are any changes
		r.Log.Info("Updating PrometheusExporter Service", "namespace", metricsService.Namespace, "name", metricsService.Name)
		err = r.Update(context.TODO(), foundMetricsService)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get the ZkConnectionString to connect to
	solrConnectionInfo := util.SolrConnectionInfo{}
	if solrConnectionInfo, err = getSolrConnectionInfo(r, prometheusExporter); err != nil {
		return ctrl.Result{}, err
	}

	deploy := util.GenerateSolrPrometheusExporterDeployment(prometheusExporter, solrConnectionInfo)
	if err := controllerutil.SetControllerReference(prometheusExporter, deploy, r.scheme); err != nil {
		return ctrl.Result{}, err
	}

	foundDeploy := &appsv1.Deployment{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, foundDeploy)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating PrometheusExporter Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
		err = r.Create(context.TODO(), deploy)
	} else if err == nil {
		if util.CopyDeploymentFields(deploy, foundDeploy) {
			r.Log.Info("Updating PrometheusExporter Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
			err = r.Update(context.TODO(), foundDeploy)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		ready := foundDeploy.Status.ReadyReplicas > 0

		if ready != prometheusExporter.Status.Ready {
			prometheusExporter.Status.Ready = ready
			r.Log.Info("Updating status for solr-prometheus-exporter", "namespace", prometheusExporter.Namespace, "name", prometheusExporter.Name)
			err = r.Status().Update(context.TODO(), prometheusExporter)
		}
	}
	return ctrl.Result{}, err
}

func getSolrConnectionInfo(r *SolrPrometheusExporterReconciler, prometheusExporter *solrv1beta1.SolrPrometheusExporter) (solrConnectionInfo util.SolrConnectionInfo, err error) {
	solrConnectionInfo = util.SolrConnectionInfo{}

	if prometheusExporter.Spec.SolrReference.Standalone != nil {
		solrConnectionInfo.StandaloneAddress = prometheusExporter.Spec.SolrReference.Standalone.Address
	}
	if prometheusExporter.Spec.SolrReference.Cloud != nil {
		if prometheusExporter.Spec.SolrReference.Cloud.ZookeeperConnectionInfo != nil {
			solrConnectionInfo.CloudZkConnnectionString = prometheusExporter.Spec.SolrReference.Cloud.ZookeeperConnectionInfo.ZkConnectionString()
		} else if prometheusExporter.Spec.SolrReference.Cloud.Name != "" {
			solrCloud := &solrv1beta1.SolrCloud{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: prometheusExporter.Spec.SolrReference.Cloud.Name, Namespace: prometheusExporter.Spec.SolrReference.Cloud.Namespace}, solrCloud)
			if err == nil {
				solrConnectionInfo.CloudZkConnnectionString = solrCloud.Status.ZookeeperConnectionInfo.ZkConnectionString()
			}
		}
	}
	return solrConnectionInfo, err
}

func (r *SolrPrometheusExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrPrometheusExporter{}).
		Complete(r)
}
