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
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"sort"
	"strings"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	zkApi "github.com/pravega/zookeeper-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SolrCloudReconciler reconciles a SolrCloud object
type SolrCloudReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var useZkCRD bool

func UseZkCRD(useCRD bool) {
	useZkCRD = useCRD
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services/status,verbs=get
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters/status,verbs=get
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *SolrCloudReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &solrv1beta1.SolrCloud{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return reconcile.Result{}, err
	}

	changed := instance.WithDefaults(logger)
	if changed {
		logger.Info("Setting default settings for SolrCloud")
		if err = r.Update(ctx, instance); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// When working with the clouds, some actions outside of kube may need to be retried after a few seconds
	requeueOrNot := reconcile.Result{}

	newStatus := solrv1beta1.SolrCloudStatus{}

	blockReconciliationOfStatefulSet := false
	if err = r.reconcileZk(ctx, logger, instance, &newStatus); err != nil {
		return requeueOrNot, err
	}

	// Generate Common Service
	commonService := util.GenerateCommonService(instance)

	// Check if the Common Service already exists
	commonServiceLogger := logger.WithValues("service", commonService.Name)
	foundCommonService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: commonService.Name, Namespace: commonService.Namespace}, foundCommonService)
	if err != nil && errors.IsNotFound(err) {
		commonServiceLogger.Info("Creating Common Service")
		if err = controllerutil.SetControllerReference(instance, commonService, r.Scheme); err == nil {
			err = r.Create(ctx, commonService)
		}
	} else if err == nil {
		var needsUpdate bool
		needsUpdate, err = util.OvertakeControllerRef(instance, foundCommonService, r.Scheme)
		needsUpdate = util.CopyServiceFields(commonService, foundCommonService, commonServiceLogger) || needsUpdate

		// Update the found Service and write the result back if there are any changes
		if needsUpdate && err == nil {
			commonServiceLogger.Info("Updating Common Service")
			err = r.Update(ctx, foundCommonService)
		}
	}
	if err != nil {
		return requeueOrNot, err
	}

	solrNodeNames := instance.GetAllSolrPodNames()

	hostNameIpMap := make(map[string]string)
	// Generate a service for every Node
	if instance.UsesIndividualNodeServices() {
		for _, nodeName := range solrNodeNames {
			err, ip := r.reconcileNodeService(ctx, logger, instance, nodeName)
			if err != nil {
				return requeueOrNot, err
			}
			// This IP Address only needs to be used in the hostname map if the SolrCloud is advertising the external address.
			if instance.Spec.SolrAddressability.External.UseExternalAddress {
				if ip == "" {
					// If we are using this IP in the hostAliases of the statefulSet, it needs to be set for every service before trying to update the statefulSet
					blockReconciliationOfStatefulSet = true
				} else {
					hostNameIpMap[instance.AdvertisedNodeHost(nodeName)] = ip
				}
			}
		}
	}

	// Generate HeadlessService
	if instance.UsesHeadlessService() {
		headless := util.GenerateHeadlessService(instance)

		// Check if the HeadlessService already exists
		headlessServiceLogger := logger.WithValues("service", headless.Name)
		foundHeadless := &corev1.Service{}
		err = r.Get(ctx, types.NamespacedName{Name: headless.Name, Namespace: headless.Namespace}, foundHeadless)
		if err != nil && errors.IsNotFound(err) {
			headlessServiceLogger.Info("Creating Headless Service")
			if err = controllerutil.SetControllerReference(instance, headless, r.Scheme); err == nil {
				err = r.Create(ctx, headless)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(instance, foundHeadless, r.Scheme)
			needsUpdate = util.CopyServiceFields(headless, foundHeadless, headlessServiceLogger) || needsUpdate

			// Update the found HeadlessService and write the result back if there are any changes
			if needsUpdate && err == nil {
				headlessServiceLogger.Info("Updating Headless Service")
				err = r.Update(ctx, foundHeadless)
			}
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	// Use a map to hold additional config info that gets determined during reconcile
	// needed for creating the STS and supporting objects (secrets, config maps, and so on)
	reconcileConfigInfo := make(map[string]string)

	// Generate ConfigMap unless the user supplied a custom ConfigMap for solr.xml
	if instance.Spec.CustomSolrKubeOptions.ConfigMapOptions != nil && instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap != "" {
		providedConfigMapName := instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap
		foundConfigMap := &corev1.ConfigMap{}
		nn := types.NamespacedName{Name: providedConfigMapName, Namespace: instance.Namespace}
		err = r.Get(ctx, nn, foundConfigMap)
		if err != nil {
			return requeueOrNot, err // if they passed a providedConfigMap name, then it must exist
		}

		if foundConfigMap.Data != nil {
			logXml, hasLogXml := foundConfigMap.Data[util.LogXmlFile]
			solrXml, hasSolrXml := foundConfigMap.Data[util.SolrXmlFile]

			// if there's a user-provided config, it must have one of the expected keys
			if !hasLogXml && !hasSolrXml {
				// TODO: Create event for the CRD.
				return requeueOrNot, fmt.Errorf("user provided ConfigMap %s must have one of 'solr.xml' and/or 'log4j2.xml'",
					providedConfigMapName)
			}

			if hasSolrXml {
				// make sure the user-provided solr.xml is valid
				if !strings.Contains(solrXml, "${hostPort:") {
					return requeueOrNot,
						fmt.Errorf("custom solr.xml in ConfigMap %s must contain a placeholder for the 'hostPort' variable, such as <int name=\"hostPort\">${hostPort:80}</int>",
							providedConfigMapName)
				}
				// stored in the pod spec annotations on the statefulset so that we get a restart when solr.xml changes
				reconcileConfigInfo[util.SolrXmlMd5Annotation] = fmt.Sprintf("%x", md5.Sum([]byte(solrXml)))
				reconcileConfigInfo[util.SolrXmlFile] = foundConfigMap.Name
			}

			if hasLogXml {
				if !strings.Contains(logXml, "monitorInterval=") {
					// stored in the pod spec annotations on the statefulset so that we get a restart when the log config changes
					reconcileConfigInfo[util.LogXmlMd5Annotation] = fmt.Sprintf("%x", md5.Sum([]byte(logXml)))
				} // else log4j will automatically refresh for us, so no restart needed
				reconcileConfigInfo[util.LogXmlFile] = foundConfigMap.Name
			}

		} else {
			return requeueOrNot, fmt.Errorf("provided ConfigMap %s has no data", providedConfigMapName)
		}
	}

	if reconcileConfigInfo[util.SolrXmlFile] == "" {
		// no user provided solr.xml, so create the default
		configMap := util.GenerateConfigMap(instance)

		reconcileConfigInfo[util.SolrXmlMd5Annotation] = fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data[util.SolrXmlFile])))
		reconcileConfigInfo[util.SolrXmlFile] = configMap.Name

		// Check if the ConfigMap already exists
		configMapLogger := logger.WithValues("configMap", configMap.Name)
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			configMapLogger.Info("Creating ConfigMap")
			if err = controllerutil.SetControllerReference(instance, configMap, r.Scheme); err == nil {
				err = r.Create(ctx, configMap)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(instance, foundConfigMap, r.Scheme)
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

	// Holds security config info needed during construction of the StatefulSet
	var security *util.SecurityConfig = nil
	if instance.Spec.SolrSecurity != nil {
		security, err = util.ReconcileSecurityConfig(ctx, &r.Client, instance)
		if err == nil && security != nil {
			// If authn enabled on Solr, we need to pass the auth header when making requests
			ctx, err = security.AddAuthToContext(ctx)
			if err != nil {
				logger.Error(err, "failed to create Authorization header when reconciling")
			}
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	// Only create stateful set if zkConnectionString can be found (must contain a host before the chroot)
	zkConnectionString := newStatus.ZkConnectionString()
	if len(zkConnectionString) < 2 || strings.HasPrefix(zkConnectionString, "/") {
		blockReconciliationOfStatefulSet = true
		logger.Info("Will not create/update the StatefulSet because the zookeeperConnectionString has no host", "zookeeperConnectionString", zkConnectionString)
	}

	// Holds TLS config info for a server cert and optionally a client cert as well
	var tls *util.TLSCerts = nil

	// can't have a solrClientTLS w/o solrTLS!
	if instance.Spec.SolrTLS == nil && instance.Spec.SolrClientTLS != nil {
		return requeueOrNot, fmt.Errorf("invalid TLS config, `spec.solrTLS` is not defined; `spec.solrClientTLS` can only be used in addition to `spec.solrTLS`")
	}

	// don't start reconciling TLS until we have ZK connectivity, avoids TLS code having to check for ZK
	if !blockReconciliationOfStatefulSet && instance.Spec.SolrTLS != nil {
		tls, err = r.reconcileTLSConfig(instance)
		if err != nil {
			return requeueOrNot, err
		}
	}

	extAddressabilityOpts := instance.Spec.SolrAddressability.External
	if extAddressabilityOpts != nil && extAddressabilityOpts.Method == solrv1beta1.Ingress {
		// Generate Ingress
		ingress := util.GenerateIngress(instance, solrNodeNames)

		// Check if the Ingress already exists
		ingressLogger := logger.WithValues("ingress", ingress.Name)
		foundIngress := &netv1.Ingress{}
		err = r.Get(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, foundIngress)
		if err != nil && errors.IsNotFound(err) {
			ingressLogger.Info("Creating Ingress")
			if err = controllerutil.SetControllerReference(instance, ingress, r.Scheme); err == nil {
				err = r.Create(ctx, ingress)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(instance, foundIngress, r.Scheme)
			needsUpdate = util.CopyIngressFields(ingress, foundIngress, ingressLogger) || needsUpdate

			// Update the found Ingress and write the result back if there are any changes
			if needsUpdate && err == nil {
				ingressLogger.Info("Updating Ingress")
				err = r.Update(ctx, foundIngress)
			}
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	var statefulSet *appsv1.StatefulSet

	if !blockReconciliationOfStatefulSet {
		// Generate StatefulSet that should exist
		expectedStatefulSet := util.GenerateStatefulSet(instance, &newStatus, hostNameIpMap, reconcileConfigInfo, tls, security)

		// Check if the StatefulSet already exists
		statefulSetLogger := logger.WithValues("statefulSet", expectedStatefulSet.Name)
		foundStatefulSet := &appsv1.StatefulSet{}
		err = r.Get(ctx, types.NamespacedName{Name: expectedStatefulSet.Name, Namespace: expectedStatefulSet.Namespace}, foundStatefulSet)

		// TODO: Move this logic down to the cluster ops and save the existing annotation in util.MaintainPreservedStatefulSetFields()
		// Set the annotation for a scheduled restart, if necessary.
		if nextRestartAnnotation, reconcileWaitDuration, schedulingErr := util.ScheduleNextRestart(instance.Spec.UpdateStrategy.RestartSchedule, foundStatefulSet.Spec.Template.Annotations); schedulingErr != nil {
			logger.Error(schedulingErr, "Cannot parse restartSchedule cron", "cron", instance.Spec.UpdateStrategy.RestartSchedule)
		} else {
			if nextRestartAnnotation != "" {
				// Set the new restart time annotation
				expectedStatefulSet.Spec.Template.Annotations[util.SolrScheduledRestartAnnotation] = nextRestartAnnotation
				// TODO: Create event for the CRD.
			} else if existingRestartAnnotation, exists := foundStatefulSet.Spec.Template.Annotations[util.SolrScheduledRestartAnnotation]; exists {
				// Keep the existing nextRestart annotation if it exists and we aren't setting a new one.
				expectedStatefulSet.Spec.Template.Annotations[util.SolrScheduledRestartAnnotation] = existingRestartAnnotation
			}
			if reconcileWaitDuration != nil {
				// Set the requeueAfter if it has not been set, or is greater than the time we need to wait to restart again
				updateRequeueAfter(&requeueOrNot, *reconcileWaitDuration)
			}
		}

		// Update or Create the StatefulSet
		if err != nil && errors.IsNotFound(err) {
			statefulSetLogger.Info("Creating StatefulSet")
			if err = controllerutil.SetControllerReference(instance, expectedStatefulSet, r.Scheme); err == nil {
				err = r.Create(ctx, expectedStatefulSet)
			}
			statefulSet = expectedStatefulSet
		} else if err == nil {
			util.MaintainPreservedStatefulSetFields(expectedStatefulSet, foundStatefulSet)

			// Check to see if the StatefulSet needs an update
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(instance, foundStatefulSet, r.Scheme)
			needsUpdate = util.CopyStatefulSetFields(expectedStatefulSet, foundStatefulSet, statefulSetLogger) || needsUpdate

			// Update the found StatefulSet and write the result back if there are any changes
			if needsUpdate && err == nil {
				statefulSetLogger.Info("Updating StatefulSet")
				err = r.Update(ctx, foundStatefulSet)
			}
			statefulSet = foundStatefulSet
		}
		if err != nil {
			return requeueOrNot, err
		}
	} else {
		// If we are blocking the reconciliation of the statefulSet, we still want to find information about it.
		err = r.Get(ctx, types.NamespacedName{Name: instance.StatefulSetName(), Namespace: instance.Namespace}, statefulSet)
		if err != nil {
			if !errors.IsNotFound(err) {
				return requeueOrNot, err
			} else {
				statefulSet = nil
			}
		}
	}

	// *********************************************************
	// The operations after this require a statefulSet to exist,
	// including updating the solrCloud status
	// *********************************************************
	if statefulSet == nil {
		return requeueOrNot, err
	}

	// Do not reconcile the storage finalizer unless we have PVC Labels that we know the Solr data PVCs are using.
	// Otherwise it will delete all PVCs possibly
	if len(statefulSet.Spec.Selector.MatchLabels) > 0 {
		if err := r.reconcileStorageFinalizer(ctx, instance, statefulSet.Spec.Selector.MatchLabels, logger); err != nil {
			logger.Error(err, "Cannot delete PVCs while garbage collecting after deletion.")
			updateRequeueAfter(&requeueOrNot, time.Second*15)
		}
	}

	// Get the SolrCloud's Pods and initialize them if necessary
	var podList []corev1.Pod
	var podSelector labels.Selector
	if podSelector, podList, err = r.initializePods(ctx, instance, logger); err != nil {
		return requeueOrNot, err
	}

	// Make sure the SolrCloud status is up-to-date with the state of the cluster
	var outOfDatePods util.OutOfDatePodSegmentation
	var availableUpdatedPodCount int
	outOfDatePods, availableUpdatedPodCount, err = createCloudStatus(instance, &newStatus, statefulSet.Status, podSelector, podList)
	if err != nil {
		return requeueOrNot, err
	}

	// We only want to do one cluster operation at a time, so we use a lock to ensure that.
	// Update or Scale, one-at-a-time. We do not want to do both.
	hasReadyPod := newStatus.ReadyReplicas > 0
	var retryLaterDuration time.Duration
	if clusterOp, opErr := GetCurrentClusterOp(statefulSet); clusterOp != nil && opErr == nil {
		switch clusterOp.Operation {
		case UpdateLock:
			retryLaterDuration, err = handleManagedCloudRollingUpdate(ctx, r, instance, statefulSet, clusterOp, outOfDatePods, hasReadyPod, availableUpdatedPodCount, logger)
		case ScaleDownLock:
			retryLaterDuration, err = handleManagedCloudScaleDown(ctx, r, instance, statefulSet, clusterOp, podList, logger)
		case ScaleUpLock:
			retryLaterDuration, err = handleManagedCloudScaleUp(ctx, r, instance, statefulSet, clusterOp, logger)
		default:
			// This shouldn't happen, but we don't want to be stuck if it does.
			// Just remove the cluster Op, because the solr operator version running does not support it.
			err = clearClusterOpLockWithPatch(ctx, r, statefulSet, "clusterOp not supported", logger)
		}
	} else if opErr == nil {
		if clusterOpQueue, opErr := GetClusterOpRetryQueue(statefulSet); opErr == nil {
			queuedRetryOps := map[SolrClusterOperationType]bool{}

			for _, op := range clusterOpQueue {
				queuedRetryOps[op.Operation] = true
			}
			lockAcquired := false
			// Start cluster operations if needed.
			// The operations will be actually run in future reconcile loops, but a clusterOpLock will be acquired here.
			// And that lock will tell future reconcile loops that the operation needs to be done.
			// Do not attempt a rolling restart if a rolling restart is already queued for retry.
			if !queuedRetryOps[UpdateLock] {
				lockAcquired, retryLaterDuration, err = determineRollingUpdateClusterOpLockIfNecessary(ctx, r, instance, statefulSet, outOfDatePods, logger)
			}

			// If a non-managed scale needs to take place, this method will update the StatefulSet without starting
			// a "locked" cluster operation
			// Do not attempt a scale, if a scale operation is already queued for retry
			if !lockAcquired && !queuedRetryOps[ScaleDownLock] && !queuedRetryOps[ScaleUpLock] {
				lockAcquired, retryLaterDuration, err = determineScaleClusterOpLockIfNecessary(ctx, r, instance, statefulSet, podList, logger)
			}

			// If no lock has been acquired, retry the next queued clusterOp, if there are any operations in the retry queue.
			if !lockAcquired {
				err = retryNextQueuedClusterOpWithPatch(ctx, r, statefulSet, logger)
			}

			// After a lock is acquired, the reconcile will be started again because the StatefulSet is being watched for changes
		} else {
			err = opErr
		}
	} else {
		err = opErr
	}
	if err != nil && retryLaterDuration == 0 {
		retryLaterDuration = time.Second * 5
	}
	if retryLaterDuration > 0 {
		updateRequeueAfter(&requeueOrNot, retryLaterDuration)
	}
	if err != nil {
		return requeueOrNot, err
	}

	// Upsert or delete solrcloud-wide PodDisruptionBudget(s) based on 'Enabled' flag.
	pdb := util.GeneratePodDisruptionBudget(instance, statefulSet.Spec.Selector.MatchLabels)
	if instance.Spec.Availability.PodDisruptionBudget.Enabled != nil && *instance.Spec.Availability.PodDisruptionBudget.Enabled {
		// Check if the PodDistruptionBudget already exists
		pdbLogger := logger.WithValues("podDisruptionBudget", pdb.Name)
		foundPDB := &policyv1.PodDisruptionBudget{}
		err = r.Get(ctx, types.NamespacedName{Name: pdb.Name, Namespace: pdb.Namespace}, foundPDB)
		if err != nil && errors.IsNotFound(err) {
			pdbLogger.Info("Creating PodDisruptionBudget")
			if err = controllerutil.SetControllerReference(instance, pdb, r.Scheme); err == nil {
				err = r.Create(ctx, pdb)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(instance, foundPDB, r.Scheme)
			needsUpdate = util.CopyPodDisruptionBudgetFields(pdb, foundPDB, pdbLogger) || needsUpdate

			// Update the found PodDistruptionBudget and write the result back if there are any changes
			if needsUpdate && err == nil {
				pdbLogger.Info("Updating PodDisruptionBudget")
				err = r.Update(ctx, foundPDB)
			}
		}
		if err != nil {
			return requeueOrNot, err
		}
	} else { // PDB is disabled, make sure that we delete any previously created pdb that might exist.
		err = r.Client.Delete(ctx, pdb)
		if err != nil && !errors.IsNotFound(err) {
			return requeueOrNot, err
		}
	}

	if !reflect.DeepEqual(instance.Status, newStatus) {
		logger.Info("Updating SolrCloud Status", "status", newStatus)
		oldInstance := instance.DeepCopy()
		instance.Status = newStatus
		err = r.Status().Patch(ctx, instance, client.MergeFrom(oldInstance))
		if err != nil {
			return requeueOrNot, err
		}
	}

	return requeueOrNot, err
}

// InitializePods Ensure that all SolrCloud Pods are initialized
func (r *SolrCloudReconciler) initializePods(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, logger logr.Logger) (podSelector labels.Selector, podList []corev1.Pod, err error) {
	foundPods := &corev1.PodList{}
	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solrv1beta1.SolrTechnologyLabel

	podSelector = labels.SelectorFromSet(selectorLabels)
	listOps := &client.ListOptions{
		Namespace:     solrCloud.Namespace,
		LabelSelector: podSelector,
	}

	if err = r.List(ctx, foundPods, listOps); err != nil {
		logger.Error(err, "Error listing pods for SolrCloud")
		return
	}
	podList = foundPods.Items

	// Initialize the pod's notStopped readinessCondition so that they can receive traffic until they are stopped
	for i, pod := range podList {
		if updatedPod, podError := r.initializePod(ctx, &pod, logger); podError != nil {
			err = podError
		} else if updatedPod != nil {
			podList[i] = *updatedPod
		}
	}
	return
}

// InitializePod Initialize Status Conditions for a SolrCloud Pod
func (r *SolrCloudReconciler) initializePod(ctx context.Context, pod *corev1.Pod, logger logr.Logger) (updatedPod *corev1.Pod, err error) {
	shouldPatchPod := false

	updatedPod = pod.DeepCopy()

	// Initialize all the readiness gates found for the pod
	for _, readinessGate := range pod.Spec.ReadinessGates {
		if InitializePodReadinessCondition(updatedPod, readinessGate.ConditionType) {
			shouldPatchPod = true
		}
	}

	if shouldPatchPod {
		if err = r.Status().Patch(ctx, updatedPod, client.StrategicMergeFrom(pod)); err != nil {
			logger.Error(err, "Could not patch pod-stopped readiness condition for pod to start traffic", "pod", pod.Name)
			// set the pod back to its original state since the patch failed
			updatedPod = pod

			// TODO: Create event for the CRD.
		}
	}
	return
}

// Initialize the SolrCloud.Status object
func createCloudStatus(solrCloud *solrv1beta1.SolrCloud,
	newStatus *solrv1beta1.SolrCloudStatus, statefulSetStatus appsv1.StatefulSetStatus, podSelector labels.Selector,
	podList []corev1.Pod) (outOfDatePods util.OutOfDatePodSegmentation, availableUpdatedPodCount int, err error) {
	var otherVersions []string
	nodeNames := make([]string, len(podList))
	nodeStatusMap := map[string]solrv1beta1.SolrNodeStatus{}

	newStatus.Replicas = statefulSetStatus.Replicas
	newStatus.UpToDateNodes = int32(0)
	newStatus.ReadyReplicas = int32(0)

	newStatus.PodSelector = podSelector.String()
	backupReposAvailable := make(map[string]bool, len(solrCloud.Spec.BackupRepositories))
	for _, repo := range solrCloud.Spec.BackupRepositories {
		backupReposAvailable[repo.Name] = false
	}
	for podIdx, p := range podList {
		nodeNames[podIdx] = p.Name
		nodeStatus := solrv1beta1.SolrNodeStatus{}
		nodeStatus.Name = p.Name
		nodeStatus.NodeName = p.Spec.NodeName
		nodeStatus.InternalAddress = solrCloud.UrlScheme(false) + "://" + solrCloud.InternalNodeUrl(nodeStatus.Name, true)
		if solrCloud.Spec.SolrAddressability.External != nil && !solrCloud.Spec.SolrAddressability.External.HideNodes {
			nodeStatus.ExternalAddress = solrCloud.UrlScheme(true) + "://" + solrCloud.ExternalNodeUrl(nodeStatus.Name, solrCloud.Spec.SolrAddressability.External.DomainName, true)
		}
		if len(p.Status.ContainerStatuses) > 0 {
			// The first container should always be running solr
			nodeStatus.Version = solrv1beta1.ImageVersion(p.Spec.Containers[0].Image)
			if nodeStatus.Version != solrCloud.Spec.SolrImage.Tag {
				otherVersions = append(otherVersions, nodeStatus.Version)
			}
		}

		// Check whether the node is considered "ready" by kubernetes
		nodeStatus.Ready = false
		nodeStatus.ScheduledForDeletion = false
		podIsScheduledForUpdate := false
		for _, condition := range p.Status.Conditions {
			if condition.Type == corev1.PodReady {
				nodeStatus.Ready = condition.Status == corev1.ConditionTrue
			} else if condition.Type == util.SolrIsNotStoppedReadinessCondition {
				nodeStatus.ScheduledForDeletion = condition.Status == corev1.ConditionFalse
				podIsScheduledForUpdate = nodeStatus.ScheduledForDeletion && condition.Reason == string(PodUpdate)
			}
		}
		if nodeStatus.Ready {
			newStatus.ReadyReplicas += 1
		}

		// Merge BackupRepository availability for this pod
		backupReposAvailableForPod := util.GetAvailableBackupRepos(&p)
		for repo, availableSoFar := range backupReposAvailable {
			backupReposAvailable[repo] = (availableSoFar || podIdx == 0) && backupReposAvailableForPod[repo]
		}

		// A pod is out of date if it's revision label is not equal to the statefulSetStatus' updateRevision.
		// Assume the pod is up-to-date if we don't have an updateRevision from the statefulSet status.
		// This should only happen when the statefulSet has just been created, so it's not a big deal.
		// NOTE: This is usually because the statefulSet status wasn't fetched, not because the fetched updateRevision
		//       is empty.
		updateRevision := statefulSetStatus.UpdateRevision
		nodeStatus.SpecUpToDate = updateRevision == "" || p.Labels["controller-revision-hash"] == updateRevision
		if nodeStatus.SpecUpToDate {
			newStatus.UpToDateNodes += 1
			if nodeStatus.Ready {
				// If the pod is up-to-date and is available, increase the counter
				availableUpdatedPodCount += 1
			}
		} else {
			containerNotStarted := false
			if !nodeStatus.Ready {
				containerNotStarted = true
				// Gather whether the solr container has started or not.
				// If it hasn't, then the pod can safely be deleted irrespective of maxNodesUnavailable.
				// This is useful for podTemplate updates that override pod specs that failed to start, such as containers with images that do not exist.
				for _, containerStatus := range p.Status.ContainerStatuses {
					if containerStatus.Name == util.SolrNodeContainer {
						containerNotStarted = containerStatus.Started == nil || !*containerStatus.Started
					}
				}
			}
			if podIsScheduledForUpdate {
				outOfDatePods.ScheduledForDeletion = append(outOfDatePods.ScheduledForDeletion, p)
			} else if containerNotStarted {
				outOfDatePods.NotStarted = append(outOfDatePods.NotStarted, p)
			} else {
				outOfDatePods.Running = append(outOfDatePods.Running, p)
			}
		}

		nodeStatusMap[nodeStatus.Name] = nodeStatus
	}
	sort.Strings(nodeNames)

	newStatus.SolrNodes = make([]solrv1beta1.SolrNodeStatus, len(nodeNames))
	for idx, nodeName := range nodeNames {
		newStatus.SolrNodes[idx] = nodeStatusMap[nodeName]
	}
	if len(backupReposAvailable) > 0 {
		newStatus.BackupRepositoriesAvailable = backupReposAvailable
		allPodsBackupReady := len(backupReposAvailable) > 0
		for _, backupRepo := range solrCloud.Spec.BackupRepositories {
			allPodsBackupReady = allPodsBackupReady && backupReposAvailable[backupRepo.Name]
			if !allPodsBackupReady {
				break
			}
		}
		newStatus.BackupRestoreReady = allPodsBackupReady
	}

	// If there are multiple versions of solr running, use the first otherVersion as the current running solr version of the cloud
	if len(otherVersions) > 0 {
		newStatus.TargetVersion = solrCloud.Spec.SolrImage.Tag
		newStatus.Version = otherVersions[0]
	} else {
		newStatus.TargetVersion = ""
		newStatus.Version = solrCloud.Spec.SolrImage.Tag
	}

	newStatus.InternalCommonAddress = solrCloud.UrlScheme(false) + "://" + solrCloud.InternalCommonUrl(true)
	if solrCloud.Spec.SolrAddressability.External != nil && !solrCloud.Spec.SolrAddressability.External.HideCommon {
		extAddress := solrCloud.UrlScheme(true) + "://" + solrCloud.ExternalCommonUrl(solrCloud.Spec.SolrAddressability.External.DomainName, true)
		newStatus.ExternalCommonAddress = &extAddress
	}

	return outOfDatePods, availableUpdatedPodCount, nil
}

func (r *SolrCloudReconciler) reconcileNodeService(ctx context.Context, logger logr.Logger, instance *solrv1beta1.SolrCloud, nodeName string) (err error, ip string) {
	// Generate Node Service
	service := util.GenerateNodeService(instance, nodeName)

	// Check if the Node Service already exists
	nodeServiceLogger := logger.WithValues("service", service.Name)
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		nodeServiceLogger.Info("Creating Node Service")
		if err = controllerutil.SetControllerReference(instance, service, r.Scheme); err == nil {
			err = r.Create(ctx, service)
		}
	} else if err == nil {
		ip = foundService.Spec.ClusterIP

		// Check to see if the Service needs an update
		var needsUpdate bool
		needsUpdate, err = util.OvertakeControllerRef(instance, foundService, r.Scheme)
		needsUpdate = util.CopyServiceFields(service, foundService, nodeServiceLogger) || needsUpdate

		if needsUpdate && err == nil {
			// Update the found Node service because there are differences between our version and the existing version
			nodeServiceLogger.Info("Updating Node Service")
			err = r.Update(ctx, foundService)
		}
	}
	if err != nil {
		return err, ip
	}

	return nil, ip
}
func (r *SolrCloudReconciler) reconcileZk(ctx context.Context, logger logr.Logger, instance *solrv1beta1.SolrCloud, newStatus *solrv1beta1.SolrCloudStatus) error {
	zkRef := instance.Spec.ZookeeperRef

	if zkRef.ConnectionInfo != nil {
		newStatus.ZookeeperConnectionInfo = *zkRef.ConnectionInfo
	} else if zkRef.ProvidedZookeeper != nil {
		pzk := zkRef.ProvidedZookeeper
		// Generate ZookeeperCluster
		if !useZkCRD {
			return errors.NewBadRequest("Cannot create a Zookeeper Cluster, as the Solr Operator is not configured to use the Zookeeper CRD")
		}
		zkCluster := util.GenerateZookeeperCluster(instance, pzk)

		// Check if the ZookeeperCluster already exists
		zkLogger := logger.WithValues("zookeeperCluster", zkCluster.Name)
		foundZkCluster := &zkApi.ZookeeperCluster{}
		err := r.Get(ctx, types.NamespacedName{Name: zkCluster.Name, Namespace: zkCluster.Namespace}, foundZkCluster)
		if err != nil && errors.IsNotFound(err) {
			zkLogger.Info("Creating Zookeeer Cluster")
			if err = controllerutil.SetControllerReference(instance, zkCluster, r.Scheme); err == nil {
				err = r.Create(ctx, zkCluster)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(instance, foundZkCluster, r.Scheme)
			needsUpdate = util.CopyZookeeperClusterFields(zkCluster, foundZkCluster, zkLogger) || needsUpdate

			// Update the found ZookeeperCluster and write the result back if there are any changes
			if needsUpdate && err == nil {
				zkLogger.Info("Updating Zookeeer Cluster")
				err = r.Update(ctx, foundZkCluster)
			}
		}
		external := &foundZkCluster.Status.ExternalClientEndpoint
		if "" == *external {
			external = nil
		}
		internal := make([]string, zkCluster.Spec.Replicas)
		kubeDomain := zkCluster.GetKubernetesClusterDomain()
		for i := range internal {
			internal[i] = fmt.Sprintf("%s-%d.%s-headless.%s.svc.%s:%d", zkCluster.Name, i, zkCluster.Name, zkCluster.Namespace, kubeDomain, zkCluster.ZookeeperPorts().Client)
		}
		newStatus.ZookeeperConnectionInfo = solrv1beta1.ZookeeperConnectionInfo{
			InternalConnectionString: strings.Join(internal, ","),
			ExternalConnectionString: external,
			ChRoot:                   pzk.ChRoot,
		}
		return err
	} else {
		return errors.NewBadRequest("No Zookeeper reference information provided.")
	}
	return nil
}

// Logic derived from:
// - https://book.kubebuilder.io/reference/using-finalizers.html
// - https://github.com/pravega/zookeeper-operator/blob/v0.2.9/pkg/controller/zookeepercluster/zookeepercluster_controller.go#L629
func (r *SolrCloudReconciler) reconcileStorageFinalizer(ctx context.Context, cloud *solrv1beta1.SolrCloud, pvcLabelSelector map[string]string, logger logr.Logger) error {
	// If persistentStorage is being used by the cloud, and the reclaim policy is set to "Delete",
	// then set a finalizer for the storage on the cloud, and delete the PVCs if the solrcloud has been deleted.

	if cloud.Spec.StorageOptions.PersistentStorage != nil && cloud.Spec.StorageOptions.PersistentStorage.VolumeReclaimPolicy == solrv1beta1.VolumeReclaimPolicyDelete {
		if cloud.ObjectMeta.DeletionTimestamp.IsZero() {
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object
			if !util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
				cloud.ObjectMeta.Finalizers = append(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
				if err := r.Update(ctx, cloud); err != nil {
					return err
				}
			}
			return r.cleanupOrphanPVCs(ctx, cloud, pvcLabelSelector, logger)
		} else if util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
			// The object is being deleted
			logger.Info("Deleting PVCs for SolrCloud")

			// Our finalizer is present, so let's delete all existing PVCs
			if err := r.cleanUpAllPVCs(ctx, cloud, pvcLabelSelector, logger); err != nil {
				return err
			}
			logger.Info("Deleted PVCs for SolrCloud")

			// remove our finalizer from the list and update it.
			cloud.ObjectMeta.Finalizers = util.RemoveString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
			if err := r.Update(ctx, cloud); err != nil {
				return err
			}
		}
	} else if util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
		// remove our finalizer from the list and update it, because there is no longer a need to delete PVCs after the cloud is deleted.
		logger.Info("Removing storage finalizer for SolrCloud")
		cloud.ObjectMeta.Finalizers = util.RemoveString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
		if err := r.Update(ctx, cloud); err != nil {
			return err
		}
	}
	return nil
}

func (r *SolrCloudReconciler) getPVCCount(ctx context.Context, cloud *solrv1beta1.SolrCloud, pvcLabelSelector map[string]string) (pvcCount int, err error) {
	pvcList, err := r.getPVCList(ctx, cloud, pvcLabelSelector)
	if err != nil {
		return -1, err
	}
	pvcCount = len(pvcList.Items)
	return pvcCount, nil
}

func (r *SolrCloudReconciler) cleanupOrphanPVCs(ctx context.Context, cloud *solrv1beta1.SolrCloud, pvcLabelSelector map[string]string, logger logr.Logger) (err error) {
	// this check should make sure we do not delete the PVCs before the STS has scaled down
	if cloud.Status.ReadyReplicas == cloud.Status.Replicas {
		pvcList, err := r.getPVCList(ctx, cloud, pvcLabelSelector)
		if err != nil {
			return err
		}
		if len(pvcList.Items) > int(*cloud.Spec.Replicas) {
			for _, pvcItem := range pvcList.Items {
				// delete only Orphan PVCs
				if util.IsPVCOrphan(pvcItem.Name, *cloud.Spec.Replicas) {
					r.deletePVC(ctx, pvcItem, logger)
				}
			}
		}
	}
	return nil
}

func (r *SolrCloudReconciler) getPVCList(ctx context.Context, cloud *solrv1beta1.SolrCloud, pvcLabelSelector map[string]string) (pvList corev1.PersistentVolumeClaimList, err error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: pvcLabelSelector,
	})
	pvclistOps := &client.ListOptions{
		Namespace:     cloud.Namespace,
		LabelSelector: selector,
	}
	pvcList := &corev1.PersistentVolumeClaimList{}
	err = r.Client.List(ctx, pvcList, pvclistOps)
	return *pvcList, err
}

func (r *SolrCloudReconciler) cleanUpAllPVCs(ctx context.Context, cloud *solrv1beta1.SolrCloud, pvcLabelSelector map[string]string, logger logr.Logger) (err error) {
	pvcList, err := r.getPVCList(ctx, cloud, pvcLabelSelector)
	if err != nil {
		return err
	}
	for _, pvcItem := range pvcList.Items {
		r.deletePVC(ctx, pvcItem, logger)
	}
	return nil
}

func (r *SolrCloudReconciler) deletePVC(ctx context.Context, pvcItem corev1.PersistentVolumeClaim, logger logr.Logger) {
	pvcDelete := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcItem.Name,
			Namespace: pvcItem.Namespace,
		},
	}
	logger.Info("Deleting PVC for SolrCloud", "PVC", pvcItem.Name)
	err := r.Client.Delete(ctx, pvcDelete)
	if err != nil {
		logger.Error(err, "Error deleting PVC for SolrCloud", "PVC", pvcDelete.Name)
	}
}

// Ensure the TLS config is ready, such as verifying the TLS secret exists, to enable TLS on SolrCloud pods
func (r *SolrCloudReconciler) reconcileTLSConfig(instance *solrv1beta1.SolrCloud) (*util.TLSCerts, error) {
	tls := util.TLSCertsForSolrCloud(instance)

	// Has the user configured a secret containing the TLS cert files that we need to mount into the Solr pods?
	serverCert := tls.ServerConfig.Options
	if serverCert.PKCS12Secret != nil {
		// Ensure one or the other have been configured, but not both
		if serverCert.MountedTLSDir != nil {
			return nil, fmt.Errorf("invalid TLS config, either supply `solrTLS.pkcs12Secret` or `solrTLS.mountedTLSDir` but not both")
		}

		_, err := tls.ServerConfig.VerifyKeystoreAndTruststoreSecretConfig(&r.Client)
		if err != nil {
			return nil, err
		}

		// is there a client TLS config too?
		if tls.ClientConfig != nil {
			if tls.ClientConfig.Options.PKCS12Secret == nil {
				// cannot mix options with the client cert, if the server cert comes from a secret, so too must the client, not a mountedTLSDir
				return nil, fmt.Errorf("invalid TLS config, the 'solrClientTLS.pkcs12Secret' option is required when using a secret for server cert")
			}

			// shouldn't configure a client cert if it's the same as the server cert
			if tls.ClientConfig.Options.PKCS12Secret == tls.ServerConfig.Options.PKCS12Secret {
				return nil, fmt.Errorf("invalid TLS config, the 'solrClientTLS.pkcs12Secret' option should not be the same as the 'solrTLS.pkcs12Secret'")
			}

			_, err := tls.ClientConfig.VerifyKeystoreAndTruststoreSecretConfig(&r.Client)
			if err != nil {
				return nil, err
			}
		}
	} else if serverCert.MountedTLSDir != nil {
		// per-pod TLS files get mounted into a dir on the pod dynamically using some external agent / CSI driver type mechanism
		// make sure the client cert, if configured, is also using the mounted dir option as mixing the two approaches is not supported
		if tls.ClientConfig != nil && tls.ClientConfig.Options.MountedTLSDir == nil {
			return nil, fmt.Errorf("invalid TLS config, client cert must also use 'mountedTLSDir' when using 'solrTLS.mountedTLSDir'")
		}
	} else {
		return nil, fmt.Errorf("invalid TLS config, must supply either 'pkcs12Secret' or 'mountedTLSDir' for the server cert")
	}

	return tls, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolrCloudReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&solrv1beta1.SolrCloud{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}). /* for authentication */
		Owns(&netv1.Ingress{}).
		Owns(&policyv1.PodDisruptionBudget{})

	var err error
	ctrlBuilder, err = r.indexAndWatchForProvidedConfigMaps(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	ctrlBuilder, err = r.indexAndWatchForTLSSecret(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	ctrlBuilder, err = r.indexAndWatchForClientTLSSecret(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	if useZkCRD {
		ctrlBuilder = ctrlBuilder.Owns(&zkApi.ZookeeperCluster{})
	}

	return ctrlBuilder.Complete(r)
}

func (r *SolrCloudReconciler) indexAndWatchForProvidedConfigMaps(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrCloud{}, ".spec.customSolrKubeOptions.configMapOptions.providedConfigMap", func(rawObj client.Object) []string {
		// grab the SolrCloud object, extract the used configMap...
		solrCloud := rawObj.(*solrv1beta1.SolrCloud)
		if solrCloud.Spec.CustomSolrKubeOptions.ConfigMapOptions == nil {
			return nil
		}
		if solrCloud.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap == "" {
			return nil
		}
		// ...and if so, return it
		return []string{solrCloud.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap}
	}); err != nil {
		return ctrlBuilder, err
	}

	return ctrlBuilder.Watches(
		&source.Kind{Type: &corev1.ConfigMap{}},
		r.findSolrCloudByFieldValueFunc(".spec.customSolrKubeOptions.configMapOptions.providedConfigMap"),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}

func (r *SolrCloudReconciler) indexAndWatchForTLSSecret(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	field := ".spec.solrTLS.pkcs12Secret"
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrCloud{}, field, func(rawObj client.Object) []string {
		// grab the SolrCloud object, extract the used configMap...
		solrCloud := rawObj.(*solrv1beta1.SolrCloud)
		if solrCloud.Spec.SolrTLS == nil || solrCloud.Spec.SolrTLS.PKCS12Secret == nil {
			return nil
		}
		// ...and if so, return it
		return []string{solrCloud.Spec.SolrTLS.PKCS12Secret.Name}
	}); err != nil {
		return ctrlBuilder, err
	}

	return ctrlBuilder.Watches(
		&source.Kind{Type: &corev1.Secret{}},
		r.findSolrCloudByFieldValueFunc(field),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}

func (r *SolrCloudReconciler) indexAndWatchForClientTLSSecret(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	field := ".spec.solrClientTLS.pkcs12Secret"
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &solrv1beta1.SolrCloud{}, field, func(rawObj client.Object) []string {
		// grab the SolrCloud object, extract the used configMap...
		solrCloud := rawObj.(*solrv1beta1.SolrCloud)
		if solrCloud.Spec.SolrClientTLS == nil || solrCloud.Spec.SolrClientTLS.PKCS12Secret == nil {
			return nil
		}
		// ...and if so, return it
		return []string{solrCloud.Spec.SolrClientTLS.PKCS12Secret.Name}
	}); err != nil {
		return ctrlBuilder, err
	}

	return ctrlBuilder.Watches(
		&source.Kind{Type: &corev1.Secret{}},
		r.findSolrCloudByFieldValueFunc(field),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}

func (r *SolrCloudReconciler) findSolrCloudByFieldValueFunc(field string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(
		func(obj client.Object) []reconcile.Request {
			foundClouds := &solrv1beta1.SolrCloudList{}
			listOps := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(field, obj.GetName()),
				Namespace:     obj.GetNamespace(),
			}
			err := r.List(context.Background(), foundClouds, listOps)
			if err != nil {
				return []reconcile.Request{}
			}

			requests := make([]reconcile.Request, len(foundClouds.Items))
			for i, item := range foundClouds.Items {
				requests[i] = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				}
			}
			return requests
		})
}
