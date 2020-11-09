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
	"crypto/md5"
	"fmt"
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
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
var IngressBaseUrl string

func UseZkCRD(useCRD bool) {
	useZkCRD = useCRD
}

func SetIngressBaseUrl(ingressBaseUrl string) {
	IngressBaseUrl = ingressBaseUrl
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
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
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters/status,verbs=get;update;patch

func (r *SolrCloudReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("solrcloud", req.NamespacedName)

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

	changed := instance.WithDefaults(IngressBaseUrl)
	if changed {
		r.Log.Info("Setting default settings for solr-cloud", "namespace", instance.Namespace, "name", instance.Name)
		if err := r.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// When working with the clouds, some actions outside of kube may need to be retried after a few seconds
	requeueOrNot := reconcile.Result{}

	newStatus := solr.SolrCloudStatus{}

	busyBoxImage := *instance.Spec.BusyBoxImage

	blockReconciliationOfStatefulSet := false

	if err := reconcileZk(r, req, instance, busyBoxImage, &newStatus); err != nil {
		return requeueOrNot, err
	}

	// Generate Common Service
	commonService := util.GenerateCommonService(instance)
	if err := controllerutil.SetControllerReference(instance, commonService, r.scheme); err != nil {
		return requeueOrNot, err
	}

	// Check if the Common Service already exists
	foundCommonService := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: commonService.Name, Namespace: commonService.Namespace}, foundCommonService)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating Common Service", "namespace", commonService.Namespace, "name", commonService.Name)
		err = r.Create(context.TODO(), commonService)
	} else if err == nil {
		if util.CopyServiceFields(commonService, foundCommonService) {
			// Update the found Service and write the result back if there are any changes
			r.Log.Info("Updating Common Service", "namespace", commonService.Namespace, "name", commonService.Name)
			err = r.Update(context.TODO(), foundCommonService)
		}
	} else {
		return requeueOrNot, err
	}

	solrNodeNames := instance.GetAllSolrNodeNames()

	hostNameIpMap := make(map[string]string)
	// Generate a service for every Node
	if instance.UsesIndividualNodeServices() {
		for _, nodeName := range solrNodeNames {
			err, ip := reconcileNodeService(r, instance, nodeName)
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
		foundHeadless := &corev1.Service{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: headless.Name, Namespace: headless.Namespace}, foundHeadless)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating HeadlessService", "namespace", headless.Namespace, "name", headless.Name)
			err = r.Create(context.TODO(), headless)
		} else if err == nil && util.CopyServiceFields(headless, foundHeadless) {
			// Update the found HeadlessService and write the result back if there are any changes
			r.Log.Info("Updating HeadlessService", "namespace", headless.Namespace, "name", headless.Name)
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
						fmt.Errorf("Custom solr.xml in ConfigMap %s must contain a placeholder for the 'hostPort' variable, such as <int name=\"hostPort\">${hostPort:80}</int>",
							instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap)
				}
				// stored in the pod spec annotations on the statefulset so that we get a restart when solr.xml changes
				solrXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(solrXml)))
				solrXmlConfigMapName = foundConfigMap.Name
			} else {
				return requeueOrNot, fmt.Errorf("Required 'solr.xml' key not found in provided ConfigMap %s",
					instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap)
			}
		} else {
			return requeueOrNot, fmt.Errorf("Provided ConfigMap %s has no data",
				instance.Spec.CustomSolrKubeOptions.ConfigMapOptions.ProvidedConfigMap)
		}
	} else {
		configMap := util.GenerateConfigMap(instance)
		if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the ConfigMap already exists
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			err = r.Create(context.TODO(), configMap)
			solrXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(configMap.Data["solr.xml"])))
		} else if err == nil && util.CopyConfigMapFields(configMap, foundConfigMap) {
			// Update the found ConfigMap and write the result back if there are any changes
			r.Log.Info("Updating ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			err = r.Update(context.TODO(), foundConfigMap)
			solrXmlMd5 = fmt.Sprintf("%x", md5.Sum([]byte(foundConfigMap.Data["solr.xml"])))
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	// Only create stateful set if zkConnectionString can be found (must contain host and port)
	if !strings.Contains(newStatus.ZkConnectionString(), ":") {
		blockReconciliationOfStatefulSet = true
	}

	pvcLabelSelector := make(map[string]string, 0)
	if !blockReconciliationOfStatefulSet {
		// Generate StatefulSet
		statefulSet := util.GenerateStatefulSet(instance, &newStatus, hostNameIpMap, solrXmlConfigMapName, solrXmlMd5)
		if err := controllerutil.SetControllerReference(instance, statefulSet, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the StatefulSet already exists
		foundStatefulSet := &appsv1.StatefulSet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, foundStatefulSet)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
			err = r.Create(context.TODO(), statefulSet)
			// Find which labels the PVCs will be using, to use for the finalizer
			pvcLabelSelector = statefulSet.Spec.Selector.MatchLabels
		} else if err == nil {
			if util.CopyStatefulSetFields(statefulSet, foundStatefulSet) {
				// Update the found StatefulSet and write the result back if there are any changes
				r.Log.Info("Updating StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
				err = r.Update(context.TODO(), foundStatefulSet)
			}
			newStatus.Replicas = foundStatefulSet.Status.Replicas
			newStatus.ReadyReplicas = foundStatefulSet.Status.ReadyReplicas
			// Find which labels the PVCs will be using, to use for the finalizer
			pvcLabelSelector = foundStatefulSet.Spec.Selector.MatchLabels
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	// Do not reconcile the storage finalizer unless we have PVC Labels that we know the Solr data PVCs are using.
	// Otherwise it will delete all PVCs possibly
	if len(pvcLabelSelector) > 0 {
		if err := r.reconcileStorageFinalizer(instance, pvcLabelSelector); err != nil {
			return reconcile.Result{RequeueAfter: time.Second * 10}, nil
		}
	}

	err = reconcileCloudStatus(r, instance, &newStatus)
	if err != nil {
		return requeueOrNot, err
	}

	extAddressabilityOpts := instance.Spec.SolrAddressability.External
	if extAddressabilityOpts != nil && extAddressabilityOpts.Method == solr.Ingress {
		// Generate Ingress
		ingress := util.GenerateIngress(instance, solrNodeNames, IngressBaseUrl)
		if err := controllerutil.SetControllerReference(instance, ingress, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the Ingress already exists
		foundIngress := &extv1.Ingress{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, foundIngress)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating Common Ingress", "namespace", ingress.Namespace, "name", ingress.Name)
			err = r.Create(context.TODO(), ingress)
		} else if err == nil && util.CopyIngressFields(ingress, foundIngress) {
			// Update the found Ingress and write the result back if there are any changes
			r.Log.Info("Updating Common Ingress", "namespace", ingress.Namespace, "name", ingress.Name)
			err = r.Update(context.TODO(), foundIngress)
		}
		if err != nil {
			return requeueOrNot, err
		}
	}

	if !reflect.DeepEqual(instance.Status, newStatus) {
		instance.Status = newStatus
		r.Log.Info("Updating SolrCloud Status: ", "namespace", instance.Namespace, "name", instance.Name)
		err = r.Status().Update(context.TODO(), instance)
		if err != nil {
			return requeueOrNot, err
		}
	}

	return requeueOrNot, nil
}

func reconcileCloudStatus(r *SolrCloudReconciler, solrCloud *solr.SolrCloud, newStatus *solr.SolrCloudStatus) (err error) {
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
		return err
	}

	otherVersions := make([]string, 0)
	nodeNames := make([]string, len(foundPods.Items))
	nodeStatusMap := map[string]solr.SolrNodeStatus{}
	backupRestoreReadyPods := 0
	for idx, p := range foundPods.Items {
		nodeNames[idx] = p.Name
		nodeStatus := solr.SolrNodeStatus{}
		nodeStatus.Name = p.Name
		nodeStatus.NodeName = p.Spec.NodeName
		nodeStatus.InternalAddress = "http://" + solrCloud.InternalNodeUrl(nodeStatus.Name, true)
		if solrCloud.Spec.SolrAddressability.External != nil && !solrCloud.Spec.SolrAddressability.External.HideNodes {
			nodeStatus.ExternalAddress = "http://" + solrCloud.ExternalNodeUrl(nodeStatus.Name, solrCloud.Spec.SolrAddressability.External.DomainName, true)
		}
		ready := false
		if len(p.Status.ContainerStatuses) > 0 {
			ready = true
			for _, c := range p.Status.ContainerStatuses {
				ready = ready && c.Ready
			}

			// The first container should always be running solr
			nodeStatus.Version = solr.ImageVersion(p.Spec.Containers[0].Image)
			if nodeStatus.Version != solrCloud.Spec.SolrImage.Tag {
				otherVersions = append(otherVersions, nodeStatus.Version)
			}
		}
		nodeStatus.Ready = ready

		nodeStatusMap[nodeStatus.Name] = nodeStatus

		// Get Volumes for backup/restore
		if solrCloud.Spec.StorageOptions.BackupRestoreOptions != nil {
			for _, volume := range p.Spec.Volumes {
				if volume.Name == util.BackupRestoreVolume {
					backupRestoreReadyPods += 1
				}
			}
		}
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

	newStatus.InternalCommonAddress = "http://" + solrCloud.InternalCommonUrl(true)
	if solrCloud.Spec.SolrAddressability.External != nil && !solrCloud.Spec.SolrAddressability.External.HideCommon {
		extAddress := "http://" + solrCloud.ExternalCommonUrl(solrCloud.Spec.SolrAddressability.External.DomainName, true)
		newStatus.ExternalCommonAddress = &extAddress
	}

	return nil
}

func reconcileNodeService(r *SolrCloudReconciler, instance *solr.SolrCloud, nodeName string) (err error, ip string) {
	// Generate Node Service
	service := util.GenerateNodeService(instance, nodeName)
	if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
		return err, ip
	}

	// Check if the Ingress already exists
	foundService := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating Node Service", "namespace", service.Namespace, "name", service.Name)
		err = r.Create(context.TODO(), service)
	} else if err == nil {
		if util.CopyServiceFields(service, foundService) {
			// Update the found Ingress and write the result back if there are any changes
			r.Log.Info("Updating Node Service", "namespace", service.Namespace, "name", service.Name)
			err = r.Update(context.TODO(), foundService)
		}
		ip = foundService.Spec.ClusterIP
	}
	if err != nil {
		return err, ip
	}

	return nil, ip
}

func reconcileZk(r *SolrCloudReconciler, request reconcile.Request, instance *solr.SolrCloud, busyBoxImage solr.ContainerImage, newStatus *solr.SolrCloudStatus) error {
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
		foundZkCluster := &zk.ZookeeperCluster{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: zkCluster.Name, Namespace: zkCluster.Namespace}, foundZkCluster)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating Zookeeer Cluster", "namespace", zkCluster.Namespace, "name", zkCluster.Name)
			err = r.Create(context.TODO(), zkCluster)
		} else if err == nil {
			if util.CopyZookeeperClusterFields(zkCluster, foundZkCluster) {
				// Update the found ZookeeperCluster and write the result back if there are any changes
				r.Log.Info("Updating Zookeeer Cluster", "namespace", zkCluster.Namespace, "name", zkCluster.Name)
				err = r.Update(context.TODO(), foundZkCluster)
			}
			external := &foundZkCluster.Status.ExternalClientEndpoint
			if "" == *external {
				external = nil
			}
			internal := make([]string, zkCluster.Spec.Replicas)
			for i, _ := range internal {
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
func (r *SolrCloudReconciler) reconcileStorageFinalizer(cloud *solr.SolrCloud, pvcLabelSelector map[string]string) error {
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
			return r.cleanupOrphanPVCs(cloud, pvcLabelSelector)
		} else if util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
			// The object is being deleted
			r.Log.Info("Deleting PVCs for SolrCloud", "cloud", cloud.Name, "namespace", cloud.Namespace)

			// Our finalizer is present, so let's delete all existing PVCs
			if err := r.cleanUpAllPVCs(cloud, pvcLabelSelector); err != nil {
				return err
			}
			r.Log.Info("Deleted PVCs for SolrCloud", "cloud", cloud.Name, "namespace", cloud.Namespace)

			// remove our finalizer from the list and update it.
			cloud.ObjectMeta.Finalizers = util.RemoveString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer)
			if err := r.Update(context.Background(), cloud); err != nil {
				return err
			}
		}
	} else if util.ContainsString(cloud.ObjectMeta.Finalizers, util.SolrStorageFinalizer) {
		// remove our finalizer from the list and update it, because there is no longer a need to delete PVCs after the cloud is deleted.
		r.Log.Info("Removing  SolrCloud", "cloud", cloud.Name, "namespace", cloud.Namespace)
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

func (r *SolrCloudReconciler) cleanupOrphanPVCs(cloud *solr.SolrCloud, pvcLabelSelector map[string]string) (err error) {
	// this check should make sure we do not delete the PVCs before the STS has scaled down
	if cloud.Status.ReadyReplicas == cloud.Status.Replicas {
		pvcList, err := r.getPVCList(cloud, pvcLabelSelector)
		if err != nil {
			return err
		}
		r.Log.Info("Checking for PVC Orphans in SolrCloud", "cloud", cloud.Name, "namespace", cloud.Namespace, "PVC Count", len(pvcList.Items), "ReadyReplicas Count", cloud.Status.ReadyReplicas)
		if len(pvcList.Items) > int(*cloud.Spec.Replicas) {
			if err != nil {
				return err
			}
			for _, pvcItem := range pvcList.Items {
				// delete only Orphan PVCs
				if util.IsPVCOrphan(pvcItem.Name, *cloud.Spec.Replicas) {
					r.deletePVC(cloud, pvcItem)
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

func (r *SolrCloudReconciler) cleanUpAllPVCs(cloud *solr.SolrCloud, pvcLabelSelector map[string]string) (err error) {
	pvcList, err := r.getPVCList(cloud, pvcLabelSelector)
	if err != nil {
		return err
	}
	for _, pvcItem := range pvcList.Items {
		r.deletePVC(cloud, pvcItem)
	}
	return nil
}

func (r *SolrCloudReconciler) deletePVC(cloud *solr.SolrCloud, pvcItem corev1.PersistentVolumeClaim) {
	pvcDelete := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcItem.Name,
			Namespace: pvcItem.Namespace,
		},
	}
	r.Log.Info("Deleting PVC for SolrCloud", "cloud", cloud.Name, "namespace", cloud.Namespace, "PVC", pvcItem.Name)
	err := r.Client.Delete(context.TODO(), pvcDelete)
	if err != nil {
		r.Log.Error(err, "Error deleting PVC for SolrCloud", "cloud", cloud.Name, "namespace", cloud.Namespace, "PVC", pvcDelete.Name)
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
		Owns(&extv1.Ingress{})

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
				r.List(context.TODO(), foundClouds, listOps)
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
