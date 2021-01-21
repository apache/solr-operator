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
	solr "github.com/apache/lucene-solr-operator/api/v1beta1"
	"github.com/apache/lucene-solr-operator/controllers/util"
	"github.com/go-logr/logr"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
	"strings"
	"time"
)

// SolrCloudReconciler reconciles a SolrCloud object
type SolrCloudReconciler struct {
	client.Client
	scheme *runtime.Scheme
	Log    logr.Logger
}

var useZkCRD bool

func UseZkCRD(useCRD bool) {
	useZkCRD = useCRD
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.apache.org,resources=solrclouds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="cert-manager.io",resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="cert-manager.io",resources=clusterissuers,verbs=get;list
// +kubebuilder:rbac:groups="cert-manager.io",resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SolrCloudReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()

	logger := r.Log.WithValues("namespace", req.Namespace, "solrCloud", req.Name)
	// Fetch the SolrCloud instance
	instance := &solr.SolrCloud{}
	err := r.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return reconcile.Result{}, err
	}

	changed := instance.WithDefaults()
	if changed {
		logger.Info("Setting default settings for SolrCloud")
		if err := r.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// When working with the clouds, some actions outside of kube may need to be retried after a few seconds
	requeueOrNot := reconcile.Result{}

	newStatus := solr.SolrCloudStatus{}

	blockReconciliationOfStatefulSet := false
	if err := reconcileZk(r, logger, instance, &newStatus); err != nil {
		return requeueOrNot, err
	}

	// Generate Common Service
	commonService := util.GenerateCommonService(instance)
	if err := controllerutil.SetControllerReference(instance, commonService, r.scheme); err != nil {
		return requeueOrNot, err
	}

	// Check if the Common Service already exists
	commonServiceLogger := logger.WithValues("service", commonService.Name)
	foundCommonService := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: commonService.Name, Namespace: commonService.Namespace}, foundCommonService)
	if err != nil && errors.IsNotFound(err) {
		commonServiceLogger.Info("Creating Common Service")
		err = r.Create(context.TODO(), commonService)
	} else if err == nil && util.CopyServiceFields(commonService, foundCommonService, commonServiceLogger) {
		// Update the found Service and write the result back if there are any changes
		commonServiceLogger.Info("Updating Common Service")
		err = r.Update(context.TODO(), foundCommonService)
	}
	if err != nil {
		return requeueOrNot, err
	}

	solrNodeNames := instance.GetAllSolrNodeNames()

	hostNameIpMap := make(map[string]string)
	// Generate a service for every Node
	if instance.UsesIndividualNodeServices() {
		for _, nodeName := range solrNodeNames {
			err, ip := reconcileNodeService(r, logger, instance, nodeName)
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
		if err := controllerutil.SetControllerReference(instance, headless, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the HeadlessService already exists
		headlessServiceLogger := logger.WithValues("service", headless.Name)
		foundHeadless := &corev1.Service{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: headless.Name, Namespace: headless.Namespace}, foundHeadless)
		if err != nil && errors.IsNotFound(err) {
			headlessServiceLogger.Info("Creating HeadlessService")
			err = r.Create(context.TODO(), headless)
		} else if err == nil && util.CopyServiceFields(headless, foundHeadless, headlessServiceLogger) {
			// Update the found HeadlessService and write the result back if there are any changes
			headlessServiceLogger.Info("Updating HeadlessService")
			err = r.Update(context.TODO(), foundHeadless)
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	// Generate ConfigMap unless the user supplied a custom ConfigMap for solr.xml ... but the provided ConfigMap
	// might be for the Prometheus exporter, so we only care if they provide a solr.xml in the CM
	solrXmlConfigMapName := instance.ConfigMapName()
	solrXmlMd5 := ""
	if instance.Spec.CustomSolrKubeOptions.ConfigMapOptions != nil && instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap != "" {
		foundConfigMap := &corev1.ConfigMap{}
		nn := types.NamespacedName{Name: instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap, Namespace: instance.Namespace}
		err = r.Get(context.TODO(), nn, foundConfigMap)
		if err != nil {
			return requeueOrNot, err // if they passed a providedConfigMap name, then it must exist
		}

		// ConfigMap doesn't have to have a solr.xml, but if it does, then it needs to be valid!
		if foundConfigMap.Data != nil {
			solrXml, ok := foundConfigMap.Data["solr.xml"]
			if ok {
				if !strings.Contains(solrXml, "${hostPort:") {
					return requeueOrNot,
						fmt.Errorf("custom solr.xml in ConfigMap %s must contain a placeholder for the 'hostPort' variable, such as <int name=\"hostPort\">${hostPort:80}</int>",
							instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap)
				}
				// stored in the pod spec annotations on the statefulset so that we get a restart when solr.xml changes
				solrXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(solrXml)))
				solrXmlConfigMapName = foundConfigMap.Name
			} else {
				return requeueOrNot, fmt.Errorf("required 'solr.xml' key not found in provided ConfigMap %s",
					instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap)
			}
		} else {
			return requeueOrNot, fmt.Errorf("provided ConfigMap %s has no data",
				instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap)
		}
	} else {
		configMap := util.GenerateConfigMap(instance)
		if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
			return requeueOrNot, err
		}
		solrXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data["solr.xml"])))

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
			return requeueOrNot, err
		}
	}

	// Only create stateful set if zkConnectionString can be found (must contain host and port)
	if !strings.Contains(newStatus.ZkConnectionString(), ":") {
		blockReconciliationOfStatefulSet = true
	}

	tlsCertMd5 := ""
	needsPkcs12InitContainer := false // flag if the StatefulSet needs an additional initCont to create PKCS12 keystore
	// don't start reconciling TLS until we have ZK connectivity, avoids TLS code having to check for ZK
	if !blockReconciliationOfStatefulSet && instance.Spec.SolrTLS != nil {
		ctx := context.TODO()
		// Create the autogenerated TLS Cert and wait for it to be issued
		if instance.Spec.SolrTLS.AutoCreate != nil {
			tlsReady, err := r.reconcileAutoCreateTLS(ctx, instance)
			// don't create the StatefulSet until we have a cert, which can take a while for a Let's Encrypt Issuer
			if !tlsReady || err != nil {
				if err != nil {
					r.Log.Error(err, "Reconcile TLS Certificate failed")
				} else {
					wait := 30 * time.Second
					if instance.Spec.SolrTLS.AutoCreate.IssuerRef == nil {
						// this is a self-signed cert, so no need to wait very long for it to issue
						wait = 2 * time.Second
					}
					requeueOrNot.RequeueAfter = wait
				}
				return requeueOrNot, err
			}
		}

		// go get the current version of the TLS secret so changes get picked up by our STS spec (via env vars)
		foundTLSSecret := &corev1.Secret{}
		lookupErr := r.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, foundTLSSecret)
		if lookupErr != nil {
			r.Log.Info("TLS secret not found.", "secret", instance.Spec.SolrTLS.PKCS12Secret.Name)
			return requeueOrNot, lookupErr
		} else {
			// We have a watch on secrets, so will get notified when the secret changes (such as after cert renewal)
			// capture the resourceVersion of the secret and stash in an envVar so that the STS gets updates when the cert changes
			if instance.Spec.SolrTLS.RestartOnTLSSecretUpdate {
				tlsCertBytes, ok := foundTLSSecret.Data["tls.crt"]
				if ok {
					tlsCertMd5 = fmt.Sprintf("%x", md5.Sum(tlsCertBytes))
				} else {
					r.Log.Error(err, "tls.crt key not found TLS secret", "name", foundTLSSecret.Name)
					return requeueOrNot, nil
				}
			}

			if _, ok := foundTLSSecret.Data[instance.Spec.SolrTLS.PKCS12Secret.Key]; !ok {
				// the keystore.p12 key is not in the TLS secret, indicating we need to create it using an initContainer
				needsPkcs12InitContainer = true
			}
		}

		// create a job to set the cluster property but we need a ZK connection first
		urlSchemeJob := util.GenerateUrlSchemeJob(instance, &newStatus)
		foundJob := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Name: urlSchemeJob.Name, Namespace: instance.Namespace}, foundJob)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating one-time job to set urlScheme cluster property to 'https'", "job", urlSchemeJob.Name)
			if err := controllerutil.SetControllerReference(instance, &urlSchemeJob, r.scheme); err != nil {
				return requeueOrNot, err
			}
			err = r.Create(ctx, &urlSchemeJob)
			if err != nil {
				return requeueOrNot, err
			}
		} // else job exists
	}

	pvcLabelSelector := make(map[string]string, 0)
	var statefulSetStatus appsv1.StatefulSetStatus

	if !blockReconciliationOfStatefulSet {
		// Generate StatefulSet
		statefulSet := util.GenerateStatefulSet(instance, &newStatus, hostNameIpMap, solrXmlConfigMapName, solrXmlMd5, needsPkcs12InitContainer, tlsCertMd5)
		if err := controllerutil.SetControllerReference(instance, statefulSet, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the StatefulSet already exists
		statefulSetLogger := logger.WithValues("statefulSet", statefulSet.Name)
		foundStatefulSet := &appsv1.StatefulSet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, foundStatefulSet)
		if err != nil && errors.IsNotFound(err) {
			statefulSetLogger.Info("Creating StatefulSet")
			err = r.Create(context.TODO(), statefulSet)
			// Find which labels the PVCs will be using, to use for the finalizer
			pvcLabelSelector = statefulSet.Spec.Selector.MatchLabels
		} else if err == nil {
			statefulSetStatus = foundStatefulSet.Status
			if util.CopyStatefulSetFields(statefulSet, foundStatefulSet, statefulSetLogger) {
				// Update the found StatefulSet and write the result back if there are any changes
				statefulSetLogger.Info("Updating StatefulSet")
				err = r.Update(context.TODO(), foundStatefulSet)
			}
			// Find which labels the PVCs will be using, to use for the finalizer
			pvcLabelSelector = foundStatefulSet.Spec.Selector.MatchLabels
		}
		if err != nil {
			return requeueOrNot, err
		}
	} else {
		// If we are blocking the reconciliation of the statefulSet, we still want to find information about it.
		foundStatefulSet := &appsv1.StatefulSet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: instance.StatefulSetName(), Namespace: instance.Namespace}, foundStatefulSet)
		if err == nil {
			// Find the status
			statefulSetStatus = foundStatefulSet.Status
			// Find which labels the PVCs will be using, to use for the finalizer
			pvcLabelSelector = foundStatefulSet.Spec.Selector.MatchLabels
		} else if !errors.IsNotFound(err) {
			return requeueOrNot, err
		}
	}

	// Do not reconcile the storage finalizer unless we have PVC Labels that we know the Solr data PVCs are using.
	// Otherwise it will delete all PVCs possibly
	if len(pvcLabelSelector) > 0 {
		if err := r.reconcileStorageFinalizer(instance, pvcLabelSelector, logger); err != nil {
			return reconcile.Result{RequeueAfter: time.Second * 10}, nil
		}
	}

	var outOfDatePods, outOfDatePodsNotStarted []corev1.Pod
	var availableUpdatedPodCount int
	outOfDatePods, outOfDatePodsNotStarted, availableUpdatedPodCount, err = reconcileCloudStatus(r, instance, &newStatus, statefulSetStatus)
	if err != nil {
		return requeueOrNot, err
	}

	// Manage the updating of out-of-spec pods, if the Managed UpdateStrategy has been specified.
	totalPodCount := int(*instance.Spec.Replicas)
	if instance.Spec.UpdateStrategy.Method == solr.ManagedUpdate && len(outOfDatePods)+len(outOfDatePodsNotStarted) > 0 {
		updateLogger := logger.WithName("ManagedUpdateSelector")

		// The out of date pods that have not been started, should all be updated immediately.
		// There is no use "safely" updating pods which have not been started yet.
		podsToUpdate := outOfDatePodsNotStarted
		for _, pod := range outOfDatePodsNotStarted {
			logger.Info("Pod killed for update.", "pod", pod.Name, "reason", "The solr container in the pod has not yet started, thus it is safe to update.")
		}
		// Pick which pods should be deleted for an update.
		// Don't exit on an error, which would only occur because of an HTTP Exception. Requeue later instead.
		additionalPodsToUpdate, retryLater := util.DeterminePodsSafeToUpdate(instance, outOfDatePods, totalPodCount, int(newStatus.ReadyReplicas), availableUpdatedPodCount, len(outOfDatePodsNotStarted), updateLogger)
		podsToUpdate = append(podsToUpdate, additionalPodsToUpdate...)

		for _, pod := range podsToUpdate {
			err = r.Delete(context.Background(), &pod, client.Preconditions{
				UID: &pod.UID,
			})
			if err != nil {
				updateLogger.Error(err, "Error while killing solr pod for update", "pod", pod.Name)
			}
			// TODO: Create event for the CRD.
		}
		if err != nil || retryLater {
			if requeueOrNot.RequeueAfter <= 0 || requeueOrNot.RequeueAfter > time.Second*15 {
				requeueOrNot.RequeueAfter = time.Second * 15
			}
		}
	}

	extAddressabilityOpts := instance.Spec.SolrAddressability.External
	if extAddressabilityOpts != nil && extAddressabilityOpts.Method == solr.Ingress {
		// Generate Ingress
		ingress := util.GenerateIngress(instance, solrNodeNames)
		if err := controllerutil.SetControllerReference(instance, ingress, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the Ingress already exists
		ingressLogger := logger.WithValues("ingress", ingress.Name)
		foundIngress := &extv1.Ingress{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, foundIngress)
		if err != nil && errors.IsNotFound(err) {
			ingressLogger.Info("Creating Ingress")
			err = r.Create(context.TODO(), ingress)
		} else if err == nil && util.CopyIngressFields(ingress, foundIngress, ingressLogger) {
			// Update the found Ingress and write the result back if there are any changes
			ingressLogger.Info("Updating Ingress")
			err = r.Update(context.TODO(), foundIngress)
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	if !reflect.DeepEqual(instance.Status, newStatus) {
		instance.Status = newStatus
		r.Log.Info("Updating SolrCloud Status: ", "namespace", instance.Namespace, "name", instance.Name, "status", instance.Status)
		err = r.Status().Update(context.TODO(), instance)
		if err != nil {
			return requeueOrNot, err
		}
	}

	return requeueOrNot, nil
}

func reconcileCloudStatus(r *SolrCloudReconciler, solrCloud *solr.SolrCloud, newStatus *solr.SolrCloudStatus, statefulSetStatus appsv1.StatefulSetStatus) (outOfDatePods []corev1.Pod, outOfDatePodsNotStarted []corev1.Pod, availableUpdatedPodCount int, err error) {
	foundPods := &corev1.PodList{}
	selectorLabels := solrCloud.SharedLabels()
	selectorLabels["technology"] = solr.SolrTechnologyLabel

	labelSelector := labels.SelectorFromSet(selectorLabels)
	listOps := &client.ListOptions{
		Namespace:     solrCloud.Namespace,
		LabelSelector: labelSelector,
	}

	err = r.List(context.TODO(), foundPods, listOps)
	if err != nil {
		return outOfDatePods, outOfDatePodsNotStarted, availableUpdatedPodCount, err
	}

	var otherVersions []string
	nodeNames := make([]string, len(foundPods.Items))
	nodeStatusMap := map[string]solr.SolrNodeStatus{}
	backupRestoreReadyPods := 0

	updateRevision := statefulSetStatus.UpdateRevision

	newStatus.Replicas = statefulSetStatus.Replicas
	newStatus.UpToDateNodes = int32(0)
	newStatus.ReadyReplicas = int32(0)
	for idx, p := range foundPods.Items {
		nodeNames[idx] = p.Name
		nodeStatus := solr.SolrNodeStatus{}
		nodeStatus.Name = p.Name
		nodeStatus.NodeName = p.Spec.NodeName
		nodeStatus.InternalAddress = solrCloud.UrlScheme() + "://" + solrCloud.InternalNodeUrl(nodeStatus.Name, true)
		if solrCloud.Spec.SolrAddressability.External != nil && !solrCloud.Spec.SolrAddressability.External.HideNodes {
			nodeStatus.ExternalAddress = solrCloud.UrlScheme() + "://" + solrCloud.ExternalNodeUrl(nodeStatus.Name, solrCloud.Spec.SolrAddressability.External.DomainName, true)
		}
		if len(p.Status.ContainerStatuses) > 0 {
			// The first container should always be running solr
			nodeStatus.Version = solr.ImageVersion(p.Spec.Containers[0].Image)
			if nodeStatus.Version != solrCloud.Spec.SolrImage.Tag {
				otherVersions = append(otherVersions, nodeStatus.Version)
			}
		}

		// Check whether the node is considered "ready" by kubernetes
		nodeStatus.Ready = false
		for _, condition := range p.Status.Conditions {
			if condition.Type == corev1.PodReady {
				nodeStatus.Ready = condition.Status == corev1.ConditionTrue
			}
		}
		if nodeStatus.Ready {
			newStatus.ReadyReplicas += 1
		}

		// Get Volumes for backup/restore
		if solrCloud.Spec.StorageOptions.BackupRestoreOptions != nil {
			for _, volume := range p.Spec.Volumes {
				if volume.Name == util.BackupRestoreVolume {
					backupRestoreReadyPods += 1
				}
			}
		}

		// A pod is out of date if it's revision label is not equal to the statefulSetStatus' updateRevision.
		nodeStatus.SpecUpToDate = p.Labels["controller-revision-hash"] == updateRevision
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
			if containerNotStarted {
				outOfDatePodsNotStarted = append(outOfDatePodsNotStarted, p)
			} else {
				outOfDatePods = append(outOfDatePods, p)
			}
		}

		nodeStatusMap[nodeStatus.Name] = nodeStatus
	}
	sort.Strings(nodeNames)

	newStatus.SolrNodes = make([]solr.SolrNodeStatus, len(nodeNames))
	for idx, nodeName := range nodeNames {
		newStatus.SolrNodes[idx] = nodeStatusMap[nodeName]
	}

	if backupRestoreReadyPods == int(*solrCloud.Spec.Replicas) && backupRestoreReadyPods > 0 {
		newStatus.BackupRestoreReady = true
	}

	// If there are multiple versions of solr running, use the first otherVersion as the current running solr version of the cloud
	if len(otherVersions) > 0 {
		newStatus.TargetVersion = solrCloud.Spec.SolrImage.Tag
		newStatus.Version = otherVersions[0]
	} else {
		newStatus.TargetVersion = ""
		newStatus.Version = solrCloud.Spec.SolrImage.Tag
	}

	newStatus.InternalCommonAddress = solrCloud.UrlScheme() + "://" + solrCloud.InternalCommonUrl(true)
	if solrCloud.Spec.SolrAddressability.External != nil && !solrCloud.Spec.SolrAddressability.External.HideCommon {
		extAddress := solrCloud.UrlScheme() + "://" + solrCloud.ExternalCommonUrl(solrCloud.Spec.SolrAddressability.External.DomainName, true)
		newStatus.ExternalCommonAddress = &extAddress
	}

	return outOfDatePods, outOfDatePodsNotStarted, availableUpdatedPodCount, nil
}

func reconcileNodeService(r *SolrCloudReconciler, logger logr.Logger, instance *solr.SolrCloud, nodeName string) (err error, ip string) {
	// Generate Node Service
	service := util.GenerateNodeService(instance, nodeName)
	if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
		return err, ip
	}

	// Check if the Node Service already exists
	nodeServiceLogger := logger.WithValues("service", service.Name)
	foundService := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		nodeServiceLogger.Info("Creating Node Service")
		err = r.Create(context.TODO(), service)
	} else if err == nil {
		if util.CopyServiceFields(service, foundService, nodeServiceLogger) {
			// Update the found Node service because there are differences between our version and the existing version
			nodeServiceLogger.Info("Updating Node Service")
			err = r.Update(context.TODO(), foundService)
		}
		ip = foundService.Spec.ClusterIP
	}
	if err != nil {
		return err, ip
	}

	return nil, ip
}

func reconcileZk(r *SolrCloudReconciler, logger logr.Logger, instance *solr.SolrCloud, newStatus *solr.SolrCloudStatus) error {
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
		if err := controllerutil.SetControllerReference(instance, zkCluster, r.scheme); err != nil {
			return err
		}

		// Check if the ZookeeperCluster already exists
		zkLogger := logger.WithValues("zookeeperCluster", zkCluster.Name)
		foundZkCluster := &zk.ZookeeperCluster{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: zkCluster.Name, Namespace: zkCluster.Namespace}, foundZkCluster)
		if err != nil && errors.IsNotFound(err) {
			zkLogger.Info("Creating Zookeeer Cluster")
			err = r.Create(context.TODO(), zkCluster)
		} else if err == nil {
			if util.CopyZookeeperClusterFields(zkCluster, foundZkCluster, zkLogger) {
				// Update the found ZookeeperCluster and write the result back if there are any changes
				zkLogger.Info("Updating Zookeeer Cluster")
				err = r.Update(context.TODO(), foundZkCluster)
			}
			external := &foundZkCluster.Status.ExternalClientEndpoint
			if "" == *external {
				external = nil
			}
			internal := make([]string, zkCluster.Spec.Replicas)
			for i := range internal {
				internal[i] = fmt.Sprintf("%s-%d.%s-headless.%s:%d", foundZkCluster.Name, i, foundZkCluster.Name, foundZkCluster.Namespace, foundZkCluster.ZookeeperPorts().Client)
			}
			newStatus.ZookeeperConnectionInfo = solr.ZookeeperConnectionInfo{
				InternalConnectionString: strings.Join(internal, ","),
				ExternalConnectionString: external,
				ChRoot:                   pzk.ChRoot,
			}
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
func (r *SolrCloudReconciler) reconcileStorageFinalizer(cloud *solr.SolrCloud, pvcLabelSelector map[string]string, logger logr.Logger) error {
	// If persistentStorage is being used by the cloud, and the reclaim policy is set to "Delete",
	// then set a finalizer for the storage on the cloud, and delete the PVCs if the solrcloud has been deleted.

	if cloud.Spec.StorageOptions.PersistentStorage != nil && cloud.Spec.StorageOptions.PersistentStorage.VolumeReclaimPolicy == solr.VolumeReclaimPolicyDelete {
		if cloud.ObjectMeta.DeletionTimestamp.IsZero() {
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object
			if !util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
				cloud.ObjectMeta.Finalizers = append(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
				if err := r.Update(context.Background(), cloud); err != nil {
					return err
				}
			}
			return r.cleanupOrphanPVCs(cloud, pvcLabelSelector, logger)
		} else if util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
			// The object is being deleted
			logger.Info("Deleting PVCs for SolrCloud")

			// Our finalizer is present, so let's delete all existing PVCs
			if err := r.cleanUpAllPVCs(cloud, pvcLabelSelector, logger); err != nil {
				return err
			}
			logger.Info("Deleted PVCs for SolrCloud")

			// remove our finalizer from the list and update it.
			cloud.ObjectMeta.Finalizers = util.RemoveString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
			if err := r.Update(context.Background(), cloud); err != nil {
				return err
			}
		}
	} else if util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
		// remove our finalizer from the list and update it, because there is no longer a need to delete PVCs after the cloud is deleted.
		logger.Info("Removing storage finalizer for SolrCloud")
		cloud.ObjectMeta.Finalizers = util.RemoveString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
		if err := r.Update(context.Background(), cloud); err != nil {
			return err
		}
	}
	return nil
}

func (r *SolrCloudReconciler) getPVCCount(cloud *solr.SolrCloud, pvcLabelSelector map[string]string) (pvcCount int, err error) {
	pvcList, err := r.getPVCList(cloud, pvcLabelSelector)
	if err != nil {
		return -1, err
	}
	pvcCount = len(pvcList.Items)
	return pvcCount, nil
}

func (r *SolrCloudReconciler) cleanupOrphanPVCs(cloud *solr.SolrCloud, pvcLabelSelector map[string]string, logger logr.Logger) (err error) {
	// this check should make sure we do not delete the PVCs before the STS has scaled down
	if cloud.Status.ReadyReplicas == cloud.Status.Replicas {
		pvcList, err := r.getPVCList(cloud, pvcLabelSelector)
		if err != nil {
			return err
		}
		if len(pvcList.Items) > int(*cloud.Spec.Replicas) {
			for _, pvcItem := range pvcList.Items {
				// delete only Orphan PVCs
				if util.IsPVCOrphan(pvcItem.Name, *cloud.Spec.Replicas) {
					r.deletePVC(pvcItem, logger)
				}
			}
		}
	}
	return nil
}

func (r *SolrCloudReconciler) getPVCList(cloud *solr.SolrCloud, pvcLabelSelector map[string]string) (pvList corev1.PersistentVolumeClaimList, err error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: pvcLabelSelector,
	})
	pvclistOps := &client.ListOptions{
		Namespace:     cloud.Namespace,
		LabelSelector: selector,
	}
	pvcList := &corev1.PersistentVolumeClaimList{}
	err = r.Client.List(context.TODO(), pvcList, pvclistOps)
	return *pvcList, err
}

func (r *SolrCloudReconciler) cleanUpAllPVCs(cloud *solr.SolrCloud, pvcLabelSelector map[string]string, logger logr.Logger) (err error) {
	pvcList, err := r.getPVCList(cloud, pvcLabelSelector)
	if err != nil {
		return err
	}
	for _, pvcItem := range pvcList.Items {
		r.deletePVC(pvcItem, logger)
	}
	return nil
}

func (r *SolrCloudReconciler) deletePVC(pvcItem corev1.PersistentVolumeClaim, logger logr.Logger) {
	pvcDelete := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcItem.Name,
			Namespace: pvcItem.Namespace,
		},
	}
	logger.Info("Deleting PVC for SolrCloud", "PVC", pvcItem.Name)
	err := r.Client.Delete(context.TODO(), pvcDelete)
	if err != nil {
		logger.Error(err, "Error deleting PVC for SolrCloud", "PVC", pvcDelete.Name)
	}
}

func (r *SolrCloudReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndReconciler(mgr, r)
}

func (r *SolrCloudReconciler) SetupWithManagerAndReconciler(mgr ctrl.Manager, reconciler reconcile.Reconciler) error {
	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&solr.SolrCloud{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&extv1.Ingress{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&batchv1.Job{})

	var err error
	ctrlBuilder, err = r.indexAndWatchForProvidedConfigMaps(mgr, ctrlBuilder)
	if err != nil {
		return err
	}

	if useZkCRD {
		ctrlBuilder = ctrlBuilder.Owns(&zk.ZookeeperCluster{})
	}

	r.scheme = mgr.GetScheme()
	return ctrlBuilder.Complete(reconciler)
}

func (r *SolrCloudReconciler) indexAndWatchForProvidedConfigMaps(mgr ctrl.Manager, ctrlBuilder *builder.Builder) (*builder.Builder, error) {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &solr.SolrCloud{}, ".spec.customSolrKubeOptions.configMapOptions.providedConfigMap", func(rawObj runtime.Object) []string {
		// grab the SolrCloud object, extract the used configMap...
		solrCloud := rawObj.(*solr.SolrCloud)
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
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				foundClouds := &solr.SolrCloudList{}
				listOps := &client.ListOptions{
					FieldSelector: fields.OneTermEqualSelector(".spec.customSolrKubeOptions.configMapOptions.providedConfigMap", a.Meta.GetName()),
					Namespace:     a.Meta.GetNamespace(),
				}
				err := r.List(context.TODO(), foundClouds, listOps)
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
			}),
		},
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})), nil
}

// Reconciles the TLS cert, returns either a bool to indicate if the cert is ready or an error
func (r *SolrCloudReconciler) reconcileAutoCreateTLS(ctx context.Context, instance *solr.SolrCloud) (bool, error) {

	// short circuit this method with a quick check if the cert exists and is ready
	// this is useful b/c it may take many minutes for a cert to be issued, so we avoid
	// all the other checking that happens below while we're waiting for the cert
	foundCert := &certv1.Certificate{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.AutoCreate.Name, Namespace: instance.Namespace}, foundCert); err == nil {
		// cert exists, but is it ready? need to wait until we see the TLS secret
		if foundTLSSecret := r.isCertificateReady(ctx, foundCert, instance.Spec.SolrTLS); foundTLSSecret != nil {
			cert := util.GenerateCertificate(instance)
			return r.afterCertificateReady(ctx, instance, &cert, foundCert, foundTLSSecret)
		}
	}

	r.Log.Info("Reconciling TLS config", "tls", instance.Spec.SolrTLS)

	// cert not found, do full reconcile for TLS ...
	var err error
	var tlsReady bool

	// First, create the keystore password secret if needed
	keystoreSecret := util.GenerateKeystoreSecret(instance)
	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: keystoreSecret.Name, Namespace: keystoreSecret.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating keystore secret", "namespace", keystoreSecret.Namespace, "name", keystoreSecret.Name)
		if err := controllerutil.SetControllerReference(instance, &keystoreSecret, r.scheme); err != nil {
			return false, err
		}
		err = r.Create(ctx, &keystoreSecret)
	}
	if err != nil {
		return false, err
	}

	// Create a self-signed cert issuer if no issuerRef provided
	if instance.Spec.SolrTLS.AutoCreate.IssuerRef == nil {
		issuerName := fmt.Sprintf("%s-selfsigned-issuer", instance.Name)
		foundIssuer := &certv1.Issuer{}
		err = r.Get(ctx, types.NamespacedName{Name: issuerName, Namespace: instance.Namespace}, foundIssuer)
		if err != nil && errors.IsNotFound(err) {
			// specified Issuer not found, let's go create a self-signed for this
			issuer := util.GenerateSelfSignedIssuer(instance, issuerName)
			if err := controllerutil.SetControllerReference(instance, &issuer, r.scheme); err != nil {
				return false, err
			}
			r.Log.Info("Creating Self-signed Certificate Issuer", "issuer", issuer)
			err = r.Create(ctx, &issuer)
		} else if err == nil {
			r.Log.Info("Found Self-signed Certificate Issuer", "issuer", issuerName)
		}
		if err != nil {
			return false, err
		}
	} else {
		// real problems arise if we create the Certificate and the Issuer doesn't exist so make we have a good config here
		if instance.Spec.SolrTLS.AutoCreate.IssuerRef.Kind == "Issuer" {
			foundIssuer := &certv1.Issuer{}
			err = r.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.AutoCreate.IssuerRef.Name, Namespace: instance.Namespace}, foundIssuer)
			if err != nil {
				if errors.IsNotFound(err) {
					r.Log.Info("cert-manager Issuer not found in namespace, cannot create a TLS certificate without an Issuer",
						"issuer", instance.Spec.SolrTLS.AutoCreate.IssuerRef.Name, "ns", instance.Namespace)
				}
				return false, err
			}
		} // else assume ClusterIssuer and good luck
	}

	// Reconcile the Certificate to use for TLS ... A Certificate is a request to Issue the cert, the
	// actual cert lives in a TLS secret created by the Issuer
	cert := util.GenerateCertificate(instance)
	err = r.Get(ctx, types.NamespacedName{Name: cert.Name, Namespace: cert.Namespace}, foundCert)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating Certificate", "cert", cert)
		// Set the operator as the owner of the cert
		if err := controllerutil.SetControllerReference(instance, &cert, r.scheme); err != nil {
			return false, err
		}
		// Create the cert
		err = r.Create(ctx, &cert)
		if err != nil {
			return false, err
		}
	} else if err == nil {
		r.Log.Info("Found Certificate, checking if it is ready", "cert", foundCert.Name)
		if foundTLSSecret := r.isCertificateReady(ctx, foundCert, instance.Spec.SolrTLS); foundTLSSecret != nil {
			tlsReady, err = r.afterCertificateReady(ctx, instance, &cert, foundCert, foundTLSSecret)
			if tlsReady {
				r.Log.Info("TLS Certificate reconciled.", "cert", foundCert.Name)
			}
		} else {
			r.Log.Info("Certificate not ready, current status", "status", foundCert.Status)
		}
	}

	if err != nil {
		return false, err
	}

	return tlsReady, nil
}

func (r *SolrCloudReconciler) isCertificateReady(ctx context.Context, cert *certv1.Certificate, tlsOpts *solr.SolrTLSOptions) *corev1.Secret {
	// Cert is ready, lookup the secret holding the keystore
	foundTLSSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cert.Spec.SecretName, Namespace: cert.Namespace}, foundTLSSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("TLS secret not found", "name", cert.Spec.SecretName)
		} else {
			r.Log.Error(err, "TLS secret lookup failed", "name", cert.Spec.SecretName)
		}
		foundTLSSecret = nil
	}

	if foundTLSSecret == nil {
		if cert.Status.Conditions != nil {
			for _, cond := range cert.Status.Conditions {
				if cond.Type == certv1.CertificateConditionIssuing {
					r.Log.Info("Certificate is still issuing", "name", cert.Name, "status", cond.Status)
					break
				}
			}
		}
	}

	// Make sure the secret containing the keystore password exists as well
	if foundTLSSecret != nil {
		keyStorePasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: tlsOpts.KeyStorePasswordSecret.Name, Namespace: foundTLSSecret.Namespace}, keyStorePasswordSecret)
		if err != nil {
			r.Log.Error(err, "Keystore password secret not found", "name", tlsOpts.KeyStorePasswordSecret.Name)
			return nil
		}
	}

	return foundTLSSecret
}

// Once the cert is ready, apply any changes if needed, otherwise, stash the TLSSecretVersion
func (r *SolrCloudReconciler) afterCertificateReady(ctx context.Context, instance *solr.SolrCloud, cert *certv1.Certificate, foundCert *certv1.Certificate, foundTLSSecret *corev1.Secret) (bool, error) {
	// see if the create config changed, thus requiring a change to the underlying Certificate object
	var err error
	if util.CopyCreateCertificateFields(cert, foundCert) {
		r.Log.Info("Certificate fields changed, updating", "cert", foundCert)
		if instance.Spec.SolrTLS.RestartOnTLSSecretUpdate {
			// tricky ~ we have to delete the TLS secret or the cert won't get re-issued
			// but we only do this if we track the version
			r.Log.Info("Deleting TLS secret because the Certificate changed", "cert", foundCert.Name)
			err = r.Delete(ctx, foundTLSSecret)
			if err != nil {
				return false, err
			}
			r.Log.Info("Deleted TLS secret so it gets re-created after cert update", "secret", foundTLSSecret.Name)
		}

		// now update the existing Certificate to trigger the cert-manager to re-issue it
		err = r.Update(ctx, foundCert)
		if err != nil {
			return false, err
		}

		// just updated the cert, assume it's not ready and re-queue
		return false, nil
	} else {
		// cert exists, is ready and has no changes

		// let's add our controller ref to it so it gets cleaned up
		if foundTLSSecret.OwnerReferences == nil || len(foundTLSSecret.OwnerReferences) == 0 {
			if err := controllerutil.SetControllerReference(instance, foundTLSSecret, r.scheme); err != nil {
				return false, err
			}

			// have to update the secret because we didn't create it (the Issuer did)
			if err := r.Update(ctx, foundTLSSecret); err != nil {
				return false, err
			}
		}

		return true, nil
	}
}
