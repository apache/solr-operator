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
	"github.com/apache/solr-operator/controllers/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
)

// SolrPrometheusExporterReconciler reconciles a SolrPrometheusExporter object
type SolrPrometheusExporterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=,resources=configmaps/status,verbs=get
//+kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=,resources=services/status,verbs=get
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds,verbs=get;list;watch
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds/status,verbs=get
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrprometheusexporters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrprometheusexporters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrprometheusexporters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *SolrPrometheusExporterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the SolrPrometheusExporter instance
	prometheusExporter := &solrv1beta1.SolrPrometheusExporter{}
	err := r.Get(ctx, req.NamespacedName, prometheusExporter)
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
		if err := r.Update(ctx, prometheusExporter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	requeueOrNot := ctrl.Result{}

	configMapKey := util.PrometheusExporterConfigMapKey
	configXmlMd5 := ""
	if prometheusExporter.Spec.Config == "" && prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions != nil && prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap != "" {
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap, Namespace: prometheusExporter.Namespace}, foundConfigMap)
		if err != nil {
			return requeueOrNot, err
		}

		if foundConfigMap.Data != nil {
			configXml, ok := foundConfigMap.Data[configMapKey]
			if ok {
				configXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(configXml)))
			} else {
				return requeueOrNot, fmt.Errorf("required '%s' key not found in provided ConfigMap %s",
					configMapKey, prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap)
			}
		} else {
			return requeueOrNot, fmt.Errorf("provided ConfigMap %s has no data",
				prometheusExporter.Spec.CustomKubeOptions.ConfigMapOptions.ProvidedConfigMap)
		}
	}

	if prometheusExporter.Spec.Config != "" {
		// Generate ConfigMap
		configMap := util.GenerateMetricsConfigMap(prometheusExporter)

		// capture the MD5 for the default config XML, otherwise we already computed it above
		if configXmlMd5 == "" {
			configXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[configMapKey])))
		}

		// Check if the ConfigMap already exists
		configMapLogger := logger.WithValues("configMap", configMap.Name)
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			configMapLogger.Info("Creating ConfigMap")
			if err = controllerutil.SetControllerReference(prometheusExporter, configMap, r.Scheme); err == nil {
				err = r.Create(ctx, configMap)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(prometheusExporter, foundConfigMap, r.Scheme)
			needsUpdate = util.CopyConfigMapFields(configMap, foundConfigMap, configMapLogger) || needsUpdate

			// Update the found ConfigMap and write the result back if there are any changes
			if needsUpdate && err == nil {
				configMapLogger.Info("Updating ConfigMap")
				err = r.Update(ctx, foundConfigMap)
			}
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	// Generate Metrics Service
	metricsService := util.GenerateSolrMetricsService(prometheusExporter)

	// Check if the Metrics Service already exists
	serviceLogger := logger.WithValues("service", metricsService.Name)
	foundMetricsService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: metricsService.Name, Namespace: metricsService.Namespace}, foundMetricsService)
	if err != nil && errors.IsNotFound(err) {
		serviceLogger.Info("Creating Service")
		if err = controllerutil.SetControllerReference(prometheusExporter, metricsService, r.Scheme); err == nil {
			err = r.Create(ctx, metricsService)
		}
	} else if err == nil {
		var needsUpdate bool
		needsUpdate, err = util.OvertakeControllerRef(prometheusExporter, foundMetricsService, r.Scheme)
		needsUpdate = util.CopyServiceFields(metricsService, foundMetricsService, serviceLogger) || needsUpdate

		// Update the found Metrics Service and write the result back if there are any changes
		if needsUpdate && err == nil {
			serviceLogger.Info("Updating Service")
			err = r.Update(ctx, foundMetricsService)
		}
	}
	if err != nil {
		return requeueOrNot, err
	}

	// Get the ZkConnectionString to connect to
	solrConnectionInfo := util.SolrConnectionInfo{}
	var solrCloudImage *solrv1beta1.ContainerImage
	if solrConnectionInfo, solrCloudImage, err = getSolrConnectionInfo(ctx, r, prometheusExporter); err != nil {
		return requeueOrNot, err
	}

	// Make sure the TLS config is in order
	var tls *util.TLSCerts = nil
	if prometheusExporter.Spec.SolrReference.SolrTLS != nil {
		tls, err = r.reconcileTLSConfig(prometheusExporter)
		if err != nil {
			return requeueOrNot, err
		}
	}

	basicAuthMd5 := ""
	if prometheusExporter.Spec.SolrReference.BasicAuthSecret != "" {
		basicAuthSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: prometheusExporter.Spec.SolrReference.BasicAuthSecret, Namespace: prometheusExporter.Namespace}, basicAuthSecret)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = util.ValidateBasicAuthSecret(basicAuthSecret)
		if err != nil {
			return reconcile.Result{}, err
		}
		creds := fmt.Sprintf("%s:%s", basicAuthSecret.Data[corev1.BasicAuthUsernameKey], basicAuthSecret.Data[corev1.BasicAuthPasswordKey])
		basicAuthMd5 = fmt.Sprintf("%x", md5.Sum([]byte(creds)))
	}

	deploy := util.GenerateSolrPrometheusExporterDeployment(prometheusExporter, solrConnectionInfo, solrCloudImage, configXmlMd5, tls, basicAuthMd5)

	ready := false
	// Check if the Metrics Deployment already exists
	deploymentLogger := logger.WithValues("deployment", deploy.Name)
	foundDeploy := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, foundDeploy)

	// Set the annotation for a scheduled restart, if necessary.
	if nextRestartAnnotation, reconcileWaitDuration, err := util.ScheduleNextRestart(prometheusExporter.Spec.RestartSchedule, foundDeploy.Spec.Template.Annotations); err != nil {
		logger.Error(err, "Cannot parse restartSchedule cron", "cron", prometheusExporter.Spec.RestartSchedule)
	} else {
		if nextRestartAnnotation != "" {
			if deploy.Spec.Template.Annotations == nil {
				deploy.Spec.Template.Annotations = make(map[string]string, 1)
			}
			// Set the new restart time annotation
			deploy.Spec.Template.Annotations[util.SolrScheduledRestartAnnotation] = nextRestartAnnotation
			// TODO: Create event for the CRD.
		} else if existingRestartAnnotation, exists := foundDeploy.Spec.Template.Annotations[util.SolrScheduledRestartAnnotation]; exists {
			if deploy.Spec.Template.Annotations == nil {
				deploy.Spec.Template.Annotations = make(map[string]string, 1)
			}
			// Keep the existing nextRestart annotation if it exists and we aren't setting a new one.
			deploy.Spec.Template.Annotations[util.SolrScheduledRestartAnnotation] = existingRestartAnnotation
		}
		if reconcileWaitDuration != nil {
			// Set the requeueAfter if it has not been set, or is greater than the time we need to wait to restart again
			updateRequeueAfter(&requeueOrNot, *reconcileWaitDuration)
		}
	}

	if err != nil && errors.IsNotFound(err) {
		deploymentLogger.Info("Creating Deployment")
		if err = controllerutil.SetControllerReference(prometheusExporter, deploy, r.Scheme); err == nil {
			err = r.Create(ctx, deploy)
		}
	} else if err == nil {
		var needsUpdate bool
		needsUpdate, err = util.OvertakeControllerRef(prometheusExporter, foundDeploy, r.Scheme)
		needsUpdate = util.CopyDeploymentFields(deploy, foundDeploy, deploymentLogger) || needsUpdate

		// Update the found Metrics Service and write the result back if there are any changes
		if needsUpdate && err == nil {
			deploymentLogger.Info("Updating Deployment")
			err = r.Update(ctx, foundDeploy)
		}
		ready = foundDeploy.Status.ReadyReplicas > 0
	}
	if err != nil {
		return requeueOrNot, err
	}

	if ready != prometheusExporter.Status.Ready {
		prometheusExporter.Status.Ready = ready
		logger.Info("Updating status for solr-prometheus-exporter")
		err = r.Status().Update(ctx, prometheusExporter)
	}

	return requeueOrNot, err
}

func getSolrConnectionInfo(ctx context.Context, r *SolrPrometheusExporterReconciler, prometheusExporter *solrv1beta1.SolrPrometheusExporter) (solrConnectionInfo util.SolrConnectionInfo, solrCloudImage *solrv1beta1.ContainerImage, err error) {
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
			solrNamespace := prometheusExporter.Spec.SolrReference.Cloud.Namespace
			if solrNamespace == "" {
				solrNamespace = prometheusExporter.Namespace
			}
			err = r.Get(ctx, types.NamespacedName{Name: prometheusExporter.Spec.SolrReference.Cloud.Name, Namespace: solrNamespace}, solrCloud)
			if err == nil {
				solrConnectionInfo.CloudZkConnnectionInfo = &solrCloud.Status.ZookeeperConnectionInfo
				solrCloudImage = solrCloud.Spec.SolrImage
			}
		}
	}
	return
}

// reconcileTLSConfig Reconciles the various options for configuring TLS for the exporter
// The exporter is a client to Solr pods, so can either just have a truststore so it trusts Solr certs
// Or it can have its own client auth cert when Solr mTLS is required
func (r *SolrPrometheusExporterReconciler) reconcileTLSConfig(prometheusExporter *solrv1beta1.SolrPrometheusExporter) (*util.TLSCerts, error) {
	tls := util.TLSCertsForExporter(prometheusExporter)
	opts := tls.ClientConfig.Options

	if opts.PKCS12Secret != nil {
		// Ensure one or the other have been configured, but not both
		if opts.MountedTLSDir != nil {
			return nil, fmt.Errorf("invalid TLS config, either supply `solrTLS.pkcs12Secret` or `solrTLS.mountedTLSDir` but not both")
		}

		// make sure the PKCS12Secret and corresponding keystore password exist and agree with the supplied config
		_, err := tls.ClientConfig.VerifyKeystoreAndTruststoreSecretConfig(&r.Client)
		if err != nil {
			return nil, err
		}
	} else if opts.TrustStoreSecret != nil {
		// no client cert, but we have truststore for the exporter, configure it ...
		// Ensure one or the other have been configured, but not both
		if opts.MountedTLSDir != nil {
			return nil, fmt.Errorf("invalid TLS config, either supply `solrTLS.trustStoreSecret` or `solrTLS.mountedTLSDir` but not both")
		}

		// make sure the TrustStoreSecret and corresponding password exist and agree with the supplied config
		err := tls.ClientConfig.VerifyTruststoreOnly(&r.Client)
		if err != nil {
			return nil, err
		}
	} else {
		// per-pod TLS files get mounted into a dir on the pod dynamically using some external agent / CSI driver type mechanism
		if opts.MountedTLSDir == nil {
			return nil, fmt.Errorf("invalid TLS config, the 'solrTLS.mountedTLSDir' option is required unless you specify a keystore and/or truststore secret")
		}

		if opts.MountedTLSDir.KeystoreFile == "" && opts.MountedTLSDir.TruststoreFile == "" {
			return nil, fmt.Errorf("invalid TLS config, the 'solrTLS.mountedTLSDir' option must specify a keystoreFile and/or truststoreFile")
		}
	}

	return tls, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolrPrometheusExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrPrometheusExporter{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{})

	var err error
	ctrlBuilder, err = r.indexAndWatchForSolrClouds(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	ctrlBuilder, err = r.indexAndWatchForProvidedConfigMaps(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	// Get notified when the TLS secret updates (such as when the cert gets renewed)
	ctrlBuilder, err = r.indexAndWatchForKeystoreSecret(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	// Exporter may only have a truststore w/o a keystore
	ctrlBuilder, err = r.indexAndWatchForTruststoreSecret(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	// Get notified when the basic auth secret updates; exporter pods must be restarted if the basic auth password
	// changes b/c the credentials are loaded from a Java system property at startup and not watched for changes.
	ctrlBuilder, err = r.indexAndWatchForBasicAuthSecret(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	return ctrlBuilder.Complete(r)
}

func (r *SolrPrometheusExporterReconciler) indexAndWatchForSolrClouds(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	solrCloudField := ".spec.solrReference.cloud.name"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrPrometheusExporter{}, solrCloudField, func(rawObj client.Object) []string {
		// grab the SolrPrometheusExporter object, extract the used SolrCloud...
		exporter := rawObj.(*solrv1beta1.SolrPrometheusExporter)
		if exporter.Spec.SolrReference.Cloud == nil {
			return nil
		}
		if exporter.Spec.SolrReference.Cloud.Name == "" {
			return nil
		}
		// ...and if so, return it
		return []string{exporter.Spec.SolrReference.Cloud.Name}
	}); err != nil {
		return ctrlBuilder, err
	}

	return ctrlBuilder.Watches(
		&source.Kind{Type: &solrv1beta1.SolrCloud{}},
		handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			foundExporters := &solrv1beta1.SolrPrometheusExporterList{}
			listOps := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(solrCloudField, obj.GetName()),
				Namespace:     obj.GetNamespace(),
			}
			err := r.List(context.Background(), foundExporters, listOps)
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
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}

func (r *SolrPrometheusExporterReconciler) indexAndWatchForProvidedConfigMaps(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	providedConfigMapField := ".spec.customKubeOptions.configMapOptions.providedConfigMap"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrPrometheusExporter{}, providedConfigMapField, func(rawObj client.Object) []string {
		// grab the SolrPrometheusExporter object, extract the used configMap...
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
		handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			foundExporters := &solrv1beta1.SolrPrometheusExporterList{}
			listOps := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(providedConfigMapField, obj.GetName()),
				Namespace:     obj.GetNamespace(),
			}
			err := r.List(context.Background(), foundExporters, listOps)
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
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}

func (r *SolrPrometheusExporterReconciler) indexAndWatchForKeystoreSecret(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	tlsSecretField := ".spec.solrReference.solrTLS.pkcs12Secret"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrPrometheusExporter{}, tlsSecretField, func(rawObj client.Object) []string {
		// grab the SolrPrometheusExporter object, extract the referenced TLS secret...
		exporter := rawObj.(*solrv1beta1.SolrPrometheusExporter)
		if exporter.Spec.SolrReference.SolrTLS == nil || exporter.Spec.SolrReference.SolrTLS.PKCS12Secret == nil {
			return nil
		}
		// ...and if so, return it
		return []string{exporter.Spec.SolrReference.SolrTLS.PKCS12Secret.Name}
	}); err != nil {
		return ctrlBuilder, err
	}

	return r.buildSecretWatch(tlsSecretField, ctrlBuilder)
}

func (r *SolrPrometheusExporterReconciler) indexAndWatchForTruststoreSecret(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	tlsSecretField := ".spec.solrReference.solrTLS.trustStoreSecret"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrPrometheusExporter{}, tlsSecretField, func(rawObj client.Object) []string {
		// grab the SolrPrometheusExporter object, extract the referenced truststore secret...
		exporter := rawObj.(*solrv1beta1.SolrPrometheusExporter)
		if exporter.Spec.SolrReference.SolrTLS == nil || exporter.Spec.SolrReference.SolrTLS.TrustStoreSecret == nil {
			return nil
		}
		// ...and if so, return it
		return []string{exporter.Spec.SolrReference.SolrTLS.TrustStoreSecret.Name}
	}); err != nil {
		return ctrlBuilder, err
	}

	return r.buildSecretWatch(tlsSecretField, ctrlBuilder)
}

func (r *SolrPrometheusExporterReconciler) indexAndWatchForBasicAuthSecret(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	secretField := ".spec.solrReference.basicAuthSecret"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrPrometheusExporter{}, secretField, func(rawObj client.Object) []string {
		// grab the SolrPrometheusExporter object, extract the referenced BasicAuth secret...
		exporter := rawObj.(*solrv1beta1.SolrPrometheusExporter)
		if exporter.Spec.SolrReference.BasicAuthSecret == "" {
			return nil
		}
		// ...and if so, return it
		return []string{exporter.Spec.SolrReference.BasicAuthSecret}
	}); err != nil {
		return ctrlBuilder, err
	}

	return r.buildSecretWatch(secretField, ctrlBuilder)
}

func (r *SolrPrometheusExporterReconciler) buildSecretWatch(secretField string, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	return ctrlBuilder.Watches(
		&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			foundExporters := &solrv1beta1.SolrPrometheusExporterList{}
			listOps := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(secretField, obj.GetName()),
				Namespace:     obj.GetNamespace(),
			}
			err := r.List(context.Background(), foundExporters, listOps)
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
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}
