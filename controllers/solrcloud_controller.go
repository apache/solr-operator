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
	"encoding/json"
	"fmt"
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util"
	etcd "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/go-logr/logr"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	zk "github.com/pravega/zookeeper-operator/pkg/apis/zookeeper/v1beta1"
	gozk "github.com/samuel/go-zookeeper/zk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
var useEtcdCRD bool
var IngressBaseUrl string

func UseZkCRD(useCRD bool) {
	useZkCRD = useCRD
}

func UseEtcdCRD(useCRD bool) {
	useEtcdCRD = useCRD
}

func SetIngressBaseUrl(ingressBaseUrl string) {
	IngressBaseUrl = ingressBaseUrl
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solr.bloomberg.com,resources=solrclouds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zookeeper.pravega.io,resources=zookeeperclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etcd.database.coreos.com,resources=etcdclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etcd.database.coreos.com,resources=etcdclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="cert-manager.io",resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="cert-manager.io",resources=clusterissuers,verbs=get;list
// +kubebuilder:rbac:groups="cert-manager.io",resources=certificates,verbs=get;list;watch;create;update;patch;delete

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

	// Generate ConfigMap
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
	} else if err == nil && util.CopyConfigMapFields(configMap, foundConfigMap) {
		// Update the found ConfigMap and write the result back if there are any changes
		r.Log.Info("Updating ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
		err = r.Update(context.TODO(), foundConfigMap)
	}
	if err != nil {
		return requeueOrNot, err
	}

	needsPkcs12InitContainer := false
	if instance.Spec.SolrTLS != nil {
		// Create the autogenerated TLS Cert and wait for it to be issued
		if instance.Spec.SolrTLS.AutoCreate != nil {
			tlsReady, err := r.reconcileAutoCreateTLS(context.TODO(), instance)
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
					r.Log.Info("Certificate is not ready, will requeue after brief wait")
					requeueOrNot.RequeueAfter = wait
				}
				return requeueOrNot, err
			}
		}

		// go get the current version of the TLS secret so changes get picked up by our STS spec (via env vars)
		foundTLSSecret := &corev1.Secret{}
		lookupErr := r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.SolrTLS.PKCS12Secret.Name, Namespace: instance.Namespace}, foundTLSSecret)
		if lookupErr != nil {
			r.Log.Info("TLS secret not found.", "secret", instance.Spec.SolrTLS.PKCS12Secret.Name)
			return requeueOrNot, lookupErr
		} else {
			// We have a watch on secrets, so will get notified when the secret changes (such as after cert renewal)
			// capture the resourceVersion of the secret and stash in an envVar so that the STS gets updates when the cert changes
			if instance.Spec.SolrTLS.RestartOnTLSSecretUpdate {
				instance.Spec.SolrTLS.TLSSecretVersion = foundTLSSecret.ResourceVersion
			}

			if _, ok := foundTLSSecret.Data[instance.Spec.SolrTLS.PKCS12Secret.Key]; !ok {
				// the keystore.p12 key is not in the TLS secret, indicating we need to create it using an initContainer
				needsPkcs12InitContainer = true
			}
		}

		// see if we need to set the urlScheme cluster prop for enabling TLS
		newStatus.UrlSchemeClusterProperty = instance.Status.UrlSchemeClusterProperty
		if !newStatus.UrlSchemeClusterProperty && strings.Contains(newStatus.ZkConnectionString(), ":") && newStatus.ZkConnectionString() != "host:7271/" {
			updated, err := r.setUrlSchemeClusterProperty(&newStatus)
			if !updated && err == nil {
				// no error, so just requeue and wait a bit to see the zk host come online
				requeueOrNot.RequeueAfter = 5 * time.Second
				return requeueOrNot, nil
			} else if err != nil {
				return requeueOrNot, err
			} else {
				newStatus.UrlSchemeClusterProperty = true
			}
		}
	}

	// Only create stateful set if zkConnectionString can be found (must contain host and port)
	if !strings.Contains(newStatus.ZkConnectionString(), ":") {
		blockReconciliationOfStatefulSet = true
	}

	if !blockReconciliationOfStatefulSet {
		// Generate StatefulSet
		statefulSet := util.GenerateStatefulSet(instance, &newStatus, hostNameIpMap, needsPkcs12InitContainer)
		if err := controllerutil.SetControllerReference(instance, statefulSet, r.scheme); err != nil {
			return requeueOrNot, err
		}

		// Check if the StatefulSet already exists
		foundStatefulSet := &appsv1.StatefulSet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, foundStatefulSet)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("Creating StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
			err = r.Create(context.TODO(), statefulSet)
		} else if err == nil {
			if util.CopyStatefulSetFields(statefulSet, foundStatefulSet) {
				// Update the found StatefulSet and write the result back if there are any changes
				r.Log.Info("Updating StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
				err = r.Update(context.TODO(), foundStatefulSet)
			}
			newStatus.Replicas = foundStatefulSet.Status.Replicas
			newStatus.ReadyReplicas = foundStatefulSet.Status.ReadyReplicas
		}
		if err != nil {
			return requeueOrNot, err
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
		r.Log.Info("Updating SolrCloud Status: ", "namespace", instance.Namespace, "name", instance.Name, "status", instance.Status)
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

	otherVersions := []string{}
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
		if solrCloud.Spec.BackupRestoreVolume != nil {
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
	} else {
		pzk := zkRef.ProvidedZookeeper
		if pzk == nil {
			return errors.NewBadRequest("No Zookeeper reference information provided.")
		}
		if pzk.Zetcd != nil {
			if !useEtcdCRD {
				return errors.NewBadRequest("Cannot create an Etcd Cluster, as the Solr Operator is not configured to use the Etcd CRD")
			}
			// Generate EtcdCluster
			etcdCluster := util.GenerateEtcdCluster(instance, *pzk.Zetcd.EtcdSpec, busyBoxImage)
			if err := controllerutil.SetControllerReference(instance, etcdCluster, r.scheme); err != nil {
				return err
			}

			// Check if the EtcdCluster already exists
			foundEtcdCluster := &etcd.EtcdCluster{}
			err := r.Get(context.TODO(), types.NamespacedName{Name: etcdCluster.Name, Namespace: etcdCluster.Namespace}, foundEtcdCluster)
			if err != nil && errors.IsNotFound(err) {
				r.Log.Info("Creating EtcdCluster", "namespace", etcdCluster.Namespace, "name", etcdCluster.Name)
				err = r.Create(context.TODO(), etcdCluster)
			} else if err == nil && util.CopyEtcdClusterFields(etcdCluster, foundEtcdCluster) {
				// Update the found EtcdCluster and write the result back if there are any changes
				r.Log.Info("Updating EtcdCluster", "namespace", etcdCluster.Namespace, "name", etcdCluster.Name)
				err = r.Update(context.TODO(), foundEtcdCluster)
			}
			if err != nil {
				return err
			}

			// Generate Zetcd Deployment
			deployment := util.GenerateZetcdDeployment(instance, *pzk.Zetcd.ZetcdSpec)
			if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
				return err
			}

			// Check if the Zetcd Deployment already exists
			foundDeployment := &appsv1.Deployment{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
			if err != nil && errors.IsNotFound(err) {
				r.Log.Info("Creating Zetcd Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
				err = r.Create(context.TODO(), foundDeployment)
			} else if err == nil && util.CopyDeploymentFields(deployment, foundDeployment) {
				// Update the found Zetcd Deployment and write the result back if there are any changes
				r.Log.Info("Updating Zetcd Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
				err = r.Update(context.TODO(), foundDeployment)
			}
			if err != nil {
				return err
			}

			// Generate Zetcd Service
			service := util.GenerateZetcdService(instance, *pzk.Zetcd.ZetcdSpec)
			if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
				return err
			}

			// Check if the Zetcd Service already exists
			foundService := &corev1.Service{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
			if err != nil && errors.IsNotFound(err) {
				r.Log.Info("Creating Zetcd Service", "namespace", service.Namespace, "name", service.Name)
				err = r.Create(context.TODO(), service)
			} else if err == nil {
				if util.CopyServiceFields(service, foundService) {
					// Update the found Zetcd Service and write the result back if there are any changes
					r.Log.Info("Updating Zetcd Service", "namespace", service.Namespace, "name", service.Name)
					err = r.Update(context.TODO(), foundService)
				}
				newStatus.ZookeeperConnectionInfo = solr.ZookeeperConnectionInfo{
					InternalConnectionString: service.Name + "." + service.Namespace + ":2181",
					ChRoot:                   pzk.ChRoot,
				}
			}
			return err
		} else if pzk.Zookeeper != nil {
			// Generate ZookeeperCluster
			if !useZkCRD {
				return errors.NewBadRequest("Cannot create a Zookeeper Cluster, as the Solr Operator is not configured to use the Zookeeper CRD")
			}
			zkCluster := util.GenerateZookeeperCluster(instance, *pzk.Zookeeper)
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
		}
	}
	return nil
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
		Owns(&corev1.Secret{})

	if useZkCRD {
		ctrlBuilder = ctrlBuilder.Owns(&zk.ZookeeperCluster{})
	}
	if useEtcdCRD {
		ctrlBuilder = ctrlBuilder.Owns(&etcd.EtcdCluster{})
	}

	r.scheme = mgr.GetScheme()
	return ctrlBuilder.Complete(reconciler)
}

// Reconciles the TLS cert, returns either a bool to indicate if the cert is ready or an error
func (r *SolrCloudReconciler) reconcileAutoCreateTLS(ctx context.Context, instance *solr.SolrCloud) (bool, error) {

	// short circuit this method with a quick check if the cert exists and is ready
	// this is useful b/c it may take many minutes for a cert to be issued, so we avoid
	// all the other checking that happens below while we're waiting for the cert
	foundCert := &certv1.Certificate{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.SolrTLS.AutoCreate.Name, Namespace: instance.Namespace}, foundCert); err == nil {
		// cert exists, but is it ready? need to wait until we see the TLS secret
		if foundTLSSecret := r.isCertificateReady(ctx, foundCert); foundTLSSecret != nil {
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
	} // else is a ClusterIssuer

	// Reconcile the Certificate to use for TLS
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
		if foundTLSSecret := r.isCertificateReady(ctx, foundCert); foundTLSSecret != nil {
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

func (r *SolrCloudReconciler) isCertificateReady(ctx context.Context, cert *certv1.Certificate) *corev1.Secret {
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

	return foundTLSSecret
}

// Once the cert is ready, apply any changes if needed, otherwise, stash the TLSSecretVersion
func (r *SolrCloudReconciler) afterCertificateReady(ctx context.Context, instance *solr.SolrCloud, cert *certv1.Certificate, foundCert *certv1.Certificate, foundTLSSecret *corev1.Secret) (bool, error) {
	// see if the create config changed, thus requiring a change to the underlying Certificate object
	var err error
	if util.CopyCreateCertificateFields(cert, foundCert) {
		r.Log.Info("Certificate fields changed, updating", "cert", foundCert)
		// tricky ~ we have to delete the TLS secret or the cert won't get re-issued
		err = r.Delete(ctx, foundTLSSecret)
		if err != nil {
			return false, err
		}
		r.Log.Info("Deleted TLS secret so it gets re-created after cert update", "secret", foundTLSSecret.Name)

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

func (r *SolrCloudReconciler) setUrlSchemeClusterProperty(newStatus *solr.SolrCloudStatus) (bool, error) {
	clusterPropsPath := "/clusterprops.json"

	chroot := newStatus.ZookeeperConnectionInfo.ChRoot
	// Go ZK client doesn't like the chroot on the connection string!
	if chroot != "" {
		clusterPropsPath = chroot + clusterPropsPath
	}
	// set the "https" cluster prop
	zkHosts := strings.Split(newStatus.ZookeeperConnectionInfo.InternalConnectionString, ",")
	r.Log.Info("Connecting to ZooKeeper", "zkHosts", zkHosts)
	zkConn, _, zkErr := gozk.Connect(zkHosts, time.Second*5)
	if zkErr != nil {
		if strings.Contains(zkErr.Error(), "no such host") {
			r.Log.Info("ZooKeeper has not provisioned yet, will try to connect again after a brief wait ...", "zkErr", zkErr)
			return false, nil // zk just hasn't provisioned yet (we hope)
		}
		r.Log.Error(zkErr, "Failed to connect to ZooKeeper", "zkHosts", zkHosts)
		return false, zkErr
	}
	defer zkConn.Close()

	data, stat, zkErr := zkConn.Get(clusterPropsPath)
	if zkErr == nil && data != nil {
		var clusterProps map[string]interface{}
		parseErr := json.Unmarshal(data, &clusterProps)
		if parseErr != nil {
			r.Log.Error(parseErr, "Failed to parse /clusterprops.json")
			clusterProps = make(map[string]interface{})
		}
		if clusterProps["urlScheme"] != "https" {
			clusterProps["urlScheme"] = "https"
			clusterPropsJson, _ := json.Marshal(clusterProps)
			znodeVers := int32(0)
			if stat != nil {
				znodeVers = stat.Version
			}
			stat, zkErr = zkConn.Set(clusterPropsPath, clusterPropsJson, znodeVers)
			if zkErr != nil {
				r.Log.Error(zkErr, "Failed to update /clusterprops.json")
			} else {
				r.Log.Info("Updated urlScheme=https in /clusterprops.json", "stat", stat)
			}
		} else {
			r.Log.Info("urlScheme is already set to https, cluster properties reconciled")
		}
	} else {
		// Does the chroot znode exist?
		if chroot != "" {
			exists, _, zkErr := zkConn.Exists(chroot)
			if !exists {
				r.Log.Error(zkErr, "Get chroot failed", "path", chroot)
				_, zkErr = zkConn.Create(chroot, nil, 0, gozk.WorldACL(gozk.PermAll))
				if zkErr != nil {
					r.Log.Error(zkErr, "Failed to create ZK chroot", "path", chroot)
				} else {
					r.Log.Info("Created chroot", "path", chroot)
				}
			}
		}

		// Create the znode
		clusterProps := make(map[string]interface{})
		clusterProps["urlScheme"] = "https"
		clusterPropsJson, _ := json.Marshal(clusterProps)
		r.Log.Info("Creating /clusterprops.json", "json", clusterProps, "path", clusterPropsPath)

		resp, zkErr := zkConn.Create(clusterPropsPath, clusterPropsJson, 0, gozk.WorldACL(gozk.PermAll))
		if zkErr != nil {
			r.Log.Error(zkErr, "Failed to create /clusterprops.json to set urlScheme=https", "resp", resp)
			return false, zkErr
		} else {
			r.Log.Info("Set urlScheme to https in /clusterprops.json", "clusterProps", clusterProps, "resp", resp)
		}
	}
	return true, nil
}
