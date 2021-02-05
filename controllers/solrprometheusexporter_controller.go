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
	"crypto/md5"
	"fmt"
	solrv1beta1 "github.com/apache/lucene-solr-operator/api/v1beta1"
	"github.com/apache/lucene-solr-operator/controllers/util"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SolrPrometheusExporterReconciler reconciles a SolrPrometheusExporter object
type SolrPrometheusExporterReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=configmaps/status,verbs=get
// +kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=services/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
// +kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds,verbs=get;list;watch
// +kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds/status,verbs=get
// +kubebuilder:rbac:groups=solr.apache.org,resources=solrprometheusexporters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.apache.org,resources=solrprometheusexporters/status,verbs=get;update;patch

func (r *SolrPrometheusExporterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()

	logger := r.Log.WithValues("namespace", req.Namespace, "solrPrometheusExporter", req.Name)

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
		logger.Info("Setting default settings for Solr PrometheusExporter")
		if err := r.Update(context.TODO(), prometheusExporter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	configMapKey := util.PrometheusExporterConfigMapKey
	configXmlMd5 := ""
	if prometheusExporter.Spec.Config == "" && prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions != nil && prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap != "" {
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap, Namespace: prometheusExporter.Namespace}, foundConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}

		if foundConfigMap.Data != nil {
			configXml, ok := foundConfigMap.Data[configMapKey]
			if ok {
				configXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(configXml)))
			} else {
				return ctrl.Result{}, fmt.Errorf("required '%s' key not found in provided ConfigMap %s",
					configMapKey, prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("provided ConfigMap %s has no data",
				prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap)
		}
	}

	if prometheusExporter.Spec.Config != "" {
		// Generate ConfigMap
		configMap := util.GenerateMetricsConfigMap(prometheusExporter)
		if err := controllerutil.SetControllerReference(prometheusExporter, configMap, r.scheme); err != nil {
			return ctrl.Result{}, err
		}

		// capture the MD5 for the default config XML, otherwise we already computed it above
		if configXmlMd5 == "" {
			configXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[configMapKey])))
		}

		// Check if the ConfigMap already exists
		configMapLogger := logger.WithValues("configMap", configMap.Name)
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			configMapLogger.Info("Creating ConfigMap")
			err = r.Create(context.TODO(), configMap)
		} else if err == nil && util.CopyConfigMapFields(configMap, foundConfigMap, configMapLogger) {
			// Update the found ConfigMap and write the result back if there are any changes
			configMapLogger.Info("Updating ConfigMap")
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
	serviceLogger := logger.WithValues("service", metricsService.Name)
	foundMetricsService := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: metricsService.Name, Namespace: metricsService.Namespace}, foundMetricsService)
	if err != nil && errors.IsNotFound(err) {
		serviceLogger.Info("Creating Service")
		err = r.Create(context.TODO(), metricsService)
	} else if err == nil && util.CopyServiceFields(metricsService, foundMetricsService, serviceLogger) {
		// Update the found Metrics Service and write the result back if there are any changes
		serviceLogger.Info("Updating Service")
		err = r.Update(context.TODO(), foundMetricsService)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get the ZkConnectionString to connect to
	solrConnectionInfo := util.SolrConnectionInfo{}
	// If TLS is enabled for this SolrCloud, extract the TLS options and pkcs12 keystore initcontainer if applicable
	// as the Prom exporter will need these to make requests to the SolrCloud pods
	var tlsEnabled *util.TLSEnabled = nil
	if solrConnectionInfo, tlsEnabled, err = getSolrConnectionInfo(r, prometheusExporter); err != nil {
		return ctrl.Result{}, err
	}

	deploy := util.GenerateSolrPrometheusExporterDeployment(prometheusExporter, solrConnectionInfo, configXmlMd5, tlsEnabled)
	if err := controllerutil.SetControllerReference(prometheusExporter, deploy, r.scheme); err != nil {
		return ctrl.Result{}, err
	}

	ready := false
	// Check if the Metrics Deployment already exists
	deploymentLogger := logger.WithValues("service", metricsService.Name)
	foundDeploy := &appsv1.Deployment{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, foundDeploy)
	if err != nil && errors.IsNotFound(err) {
		deploymentLogger.Info("Creating Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
		err = r.Create(context.TODO(), deploy)
	} else if err == nil {
		if util.CopyDeploymentFields(deploy, foundDeploy, deploymentLogger) {
			deploymentLogger.Info("Updating Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
			err = r.Update(context.TODO(), foundDeploy)
		}
		ready = foundDeploy.Status.ReadyReplicas > 0
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if ready != prometheusExporter.Status.Ready {
		prometheusExporter.Status.Ready = ready
		logger.Info("Updating status for solr-prometheus-exporter", "namespace", prometheusExporter.Namespace, "name", prometheusExporter.Name)
		err = r.Status().Update(context.TODO(), prometheusExporter)
	}

	return ctrl.Result{}, err
}

func getSolrConnectionInfo(r *SolrPrometheusExporterReconciler, prometheusExporter *solrv1beta1.SolrPrometheusExporter) (solrConnectionInfo util.SolrConnectionInfo, tlsEnabled *util.TLSEnabled, err error) {
	solrConnectionInfo = util.SolrConnectionInfo{}

	if prometheusExporter.Spec.SolrReference.Standalone != nil {
		solrConnectionInfo.StandaloneAddress = prometheusExporter.Spec.SolrReference.Standalone.Address
	}
	if prometheusExporter.Spec.SolrReference.Cloud != nil {
		cloudRef := prometheusExporter.Spec.SolrReference.Cloud
		if cloudRef.ZookeeperConnectionInfo != nil {
			solrConnectionInfo.CloudZkConnnectionInfo = cloudRef.ZookeeperConnectionInfo
		} else if cloudRef.Name != "" {
			solrCloud := &solrv1beta1.SolrCloud{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: prometheusExporter.Spec.SolrReference.Cloud.Name, Namespace: prometheusExporter.Spec.SolrReference.Cloud.Namespace}, solrCloud)
			if err == nil {
				solrConnectionInfo.CloudZkConnnectionInfo = &solrCloud.Status.ZookeeperConnectionInfo

				if solrCloud.Spec.SolrTLS != nil {
					tlsEnabled = &util.TLSEnabled{}
					tlsEnabled.TLSOptions = solrCloud.Spec.SolrTLS

					foundTLSSecret := &corev1.Secret{}
					err = r.Get(context.TODO(), types.NamespacedName{Name: solrCloud.Spec.SolrTLS.PKCS12Secret.Name, Namespace: solrCloud.Namespace}, foundTLSSecret)
					if err != nil {
						return solrConnectionInfo, nil, err
					}

					if solrCloud.Spec.SolrTLS.RestartOnTLSSecretUpdate {
						tlsCertBytes, ok := foundTLSSecret.Data["tls.crt"]
						if ok {
							tlsEnabled.TLSCertMd5 = fmt.Sprintf("%x", md5.Sum(tlsCertBytes))
						}
					}

					foundStatefulSet := &appsv1.StatefulSet{}
					err = r.Get(context.TODO(), types.NamespacedName{Name: solrCloud.StatefulSetName(), Namespace: solrCloud.GetNamespace()}, foundStatefulSet)
					if err != nil {
						// can't find the sts for the SolrCloud ... just bail
						return solrConnectionInfo, nil, err
					}

					for _, cnt := range foundStatefulSet.Spec.Template.Spec.InitContainers {
						if cnt.Name == "gen-pkcs12-keystore" {
							tlsEnabled.Pkcs12InitContainer = &cnt
							break
						}
					}
				}
			}
		}
	}
	return solrConnectionInfo, tlsEnabled, err
}

func (r *SolrPrometheusExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndReconciler(mgr, r)
}

func (r *SolrPrometheusExporterReconciler) SetupWithManagerAndReconciler(mgr ctrl.Manager, reconciler reconcile.Reconciler) error {
	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrPrometheusExporter{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{})

	var err error
	ctrlBuilder, err = r.indexAndWatchForProvidedConfigMaps(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	r.scheme = mgr.GetScheme()
	return ctrlBuilder.Complete(reconciler)
}

func (r *SolrPrometheusExporterReconciler) indexAndWatchForProvidedConfigMaps(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	providedConfigMapField := ".spec.customKubeOptions.configMapOptions.providedConfigMap"

	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &solrv1beta1.SolrPrometheusExporter{}, providedConfigMapField, func(rawObj runtime.Object) []string {
		// grab the SolrCloud object, extract the used configMap...
		exporter := rawObj.(*solrv1beta1.SolrPrometheusExporter)
		if exporter.Spec.CustomKubeOptions.ConfigMapOptions == nil {
			return nil
		}
		if exporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap == "" {
			return nil
		}
		// ...and if so, return it
		return []string{exporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap}
	}); err != nil {
		return ctrlBuilder, err
	}

	return ctrlBuilder.Watches(
		&source.Kind{Type: &corev1.ConfigMap{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				foundExporters := &solrv1beta1.SolrPrometheusExporterList{}
				listOps := &client.ListOptions{
					FieldSelector: fields.OneTermEqualSelector(providedConfigMapField, a.Meta.GetName()),
					Namespace:     a.Meta.GetNamespace(),
				}
				err := r.List(context.TODO(), foundExporters, listOps)
				if err != nil {
					// if no exporters found, just no-op this
					return []reconcile.Request{}
				}

				requests := make([]reconcile.Request, len(foundExporters.Items))
				for i, item := range foundExporters.Items {
					requests[i] = reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      item.GetName(),
							Namespace: item.GetNamespace(),
						},
					}
				}
				return requests
			}),
		},
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}
