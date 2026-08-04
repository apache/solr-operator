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
	"fmt"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (r *SolrCloudReconciler) reconcileGatewayResources(ctx context.Context, cloud *solrv1beta1.SolrCloud, solrNodeNames []string, logger logr.Logger) error {
	extAddressabilityOpts := cloud.Spec.SolrAddressability.External
	if extAddressabilityOpts != nil && extAddressabilityOpts.Method == solrv1beta1.Gateway {
		// Validate that gateway config is provided
		if extAddressabilityOpts.Gateway == nil || len(extAddressabilityOpts.Gateway.ParentRefs) == 0 {
			r.Recorder.Event(cloud, corev1.EventTypeWarning, "InvalidConfiguration", "gateway.parentRefs is required when using method=Gateway")
			return fmt.Errorf("gateway.parentRefs is required when using method=Gateway")
		}

		// Validate that BackendTLSPolicy requires solrTLS to be configured
		if extAddressabilityOpts.HasBackendTLSPolicy() && cloud.Spec.SolrTLS == nil {
			r.Recorder.Event(cloud, corev1.EventTypeWarning, "InvalidConfiguration", "gateway.backendTLSPolicy requires spec.solrTLS to be configured, because BackendTLSPolicy instructs the gateway to use TLS when connecting to Solr backends")
			return fmt.Errorf("invalid config: `spec.customSolrKubeOptions.addressability.external.gateway.backendTLSPolicy` requires `spec.solrTLS` to be configured, because BackendTLSPolicy instructs the gateway to use TLS when connecting to Solr backends")
		}

		// Reconcile Common HTTPRoute (if not hidden)
		if !extAddressabilityOpts.HideCommon {
			commonHTTPRoute := util.GenerateCommonHTTPRoute(cloud, solrNodeNames)
			commonHTTPRouteLogger := logger.WithValues("httproute", commonHTTPRoute.Name)
			foundCommonHTTPRoute := &gatewayv1.HTTPRoute{}
			err := r.Get(ctx, types.NamespacedName{Name: commonHTTPRoute.Name, Namespace: commonHTTPRoute.Namespace}, foundCommonHTTPRoute)
			if err != nil && errors.IsNotFound(err) {
				commonHTTPRouteLogger.Info("Creating common HTTPRoute")
				if err = controllerutil.SetControllerReference(cloud, commonHTTPRoute, r.Scheme); err == nil {
					err = r.Create(ctx, commonHTTPRoute)
				}
			} else if err == nil {
				var needsUpdate bool
				needsUpdate, err = util.OvertakeControllerRef(cloud, foundCommonHTTPRoute, r.Scheme)
				needsUpdate = util.CopyHTTPRouteFields(commonHTTPRoute, foundCommonHTTPRoute, commonHTTPRouteLogger) || needsUpdate

				if needsUpdate && err == nil {
					commonHTTPRouteLogger.Info("Updating common HTTPRoute")
					err = r.Update(ctx, foundCommonHTTPRoute)
				}
			}
			if err != nil {
				return err
			}
		} else {
			// Delete common HTTPRoute if it exists but should be hidden
			foundCommonHTTPRoute := &gatewayv1.HTTPRoute{}
			err := r.Get(ctx, types.NamespacedName{Name: cloud.CommonHTTPRouteName(), Namespace: cloud.GetNamespace()}, foundCommonHTTPRoute)
			if err == nil {
				logger.Info("Deleting common HTTPRoute (hideCommon=true)")
				err = r.Delete(ctx, foundCommonHTTPRoute)
				if err != nil && !errors.IsNotFound(err) {
					return err
				}
			}
		}

		// Reconcile Node HTTPRoutes (if not hidden)
		if !extAddressabilityOpts.HideNodes {
			for _, nodeName := range solrNodeNames {
				nodeHTTPRoute := util.GenerateNodeHTTPRoute(cloud, nodeName)
				nodeHTTPRouteLogger := logger.WithValues("httproute", nodeHTTPRoute.Name)
				foundNodeHTTPRoute := &gatewayv1.HTTPRoute{}
				err := r.Get(ctx, types.NamespacedName{Name: nodeHTTPRoute.Name, Namespace: nodeHTTPRoute.Namespace}, foundNodeHTTPRoute)
				if err != nil && errors.IsNotFound(err) {
					nodeHTTPRouteLogger.Info("Creating node HTTPRoute")
					if err = controllerutil.SetControllerReference(cloud, nodeHTTPRoute, r.Scheme); err == nil {
						err = r.Create(ctx, nodeHTTPRoute)
					}
				} else if err == nil {
					var needsUpdate bool
					needsUpdate, err = util.OvertakeControllerRef(cloud, foundNodeHTTPRoute, r.Scheme)
					needsUpdate = util.CopyHTTPRouteFields(nodeHTTPRoute, foundNodeHTTPRoute, nodeHTTPRouteLogger) || needsUpdate

					if needsUpdate && err == nil {
						nodeHTTPRouteLogger.Info("Updating node HTTPRoute")
						err = r.Update(ctx, foundNodeHTTPRoute)
					}
				}
				if err != nil {
					return err
				}
			}
		}

		// Cleanup orphaned node HTTPRoutes (when scaling down or hideNodes=true)
		httpRouteList := &gatewayv1.HTTPRouteList{}
		labelSelector := labels.SelectorFromSet(cloud.SharedLabels())
		listOps := &client.ListOptions{
			Namespace:     cloud.Namespace,
			LabelSelector: labelSelector,
		}
		err := r.List(ctx, httpRouteList, listOps)
		if err == nil {
			for _, httpRoute := range httpRouteList.Items {
				// Skip the common HTTPRoute
				if httpRoute.Name == cloud.CommonHTTPRouteName() {
					continue
				}

				// Delete if hideNodes is true, or if this node no longer exists (scale-down)
				shouldDelete := extAddressabilityOpts.HideNodes
				if !shouldDelete {
					nodeExists := false
					for _, nodeName := range solrNodeNames {
						if httpRoute.Name == cloud.NodeHTTPRouteName(nodeName) {
							nodeExists = true
							break
						}
					}
					shouldDelete = !nodeExists
				}

				if shouldDelete {
					if extAddressabilityOpts.HideNodes {
						logger.Info("Deleting node HTTPRoute (hideNodes=true)", "httproute", httpRoute.Name)
					} else {
						logger.Info("Deleting orphaned node HTTPRoute", "httproute", httpRoute.Name)
					}
					err = r.Delete(ctx, &httpRoute)
					if err != nil && !errors.IsNotFound(err) {
						return err
					}
				}
			}
		}

		// Reconcile BackendTLSPolicy resources if configured
		if extAddressabilityOpts.HasBackendTLSPolicy() {
			// Reconcile Common BackendTLSPolicy (if not hidden)
			if !extAddressabilityOpts.HideCommon {
				commonPolicy := util.GenerateCommonBackendTLSPolicy(cloud)
				if commonPolicy != nil {
					commonPolicyLogger := logger.WithValues("backendtlspolicy", commonPolicy.Name)
					foundCommonPolicy := &gatewayv1.BackendTLSPolicy{}
					err = r.Get(ctx, types.NamespacedName{Name: commonPolicy.Name, Namespace: commonPolicy.Namespace}, foundCommonPolicy)

					if err != nil && errors.IsNotFound(err) {
						commonPolicyLogger.Info("Creating BackendTLSPolicy")
						if err = controllerutil.SetControllerReference(cloud, commonPolicy, r.Scheme); err == nil {
							err = r.Create(ctx, commonPolicy)
						}
					} else if err == nil {
						var needsUpdate bool
						needsUpdate, err = util.OvertakeControllerRef(cloud, foundCommonPolicy, r.Scheme)
						if err == nil && util.CopyBackendTLSPolicyFields(commonPolicy, foundCommonPolicy, commonPolicyLogger) {
							needsUpdate = true
						}
						if needsUpdate {
							commonPolicyLogger.Info("Updating BackendTLSPolicy")
							err = r.Update(ctx, foundCommonPolicy)
						}
					}
					if err != nil {
						return err
					}
				}
			} else {
				// Delete common BackendTLSPolicy if it exists but should be hidden
				foundCommonPolicy := &gatewayv1.BackendTLSPolicy{}
				err = r.Get(ctx, types.NamespacedName{Name: cloud.CommonBackendTLSPolicyName(), Namespace: cloud.GetNamespace()}, foundCommonPolicy)
				if err == nil {
					logger.Info("Deleting common BackendTLSPolicy (hideCommon=true)")
					err = r.Delete(ctx, foundCommonPolicy)
					if err != nil && !errors.IsNotFound(err) {
						return err
					}
				}
			}

			// Reconcile Node BackendTLSPolicies (if not hidden)
			if !extAddressabilityOpts.HideNodes {
				for _, nodeName := range solrNodeNames {
					nodePolicy := util.GenerateNodeBackendTLSPolicy(cloud, nodeName)
					if nodePolicy != nil {
						nodePolicyLogger := logger.WithValues("backendtlspolicy", nodePolicy.Name)
						foundNodePolicy := &gatewayv1.BackendTLSPolicy{}
						err = r.Get(ctx, types.NamespacedName{Name: nodePolicy.Name, Namespace: nodePolicy.Namespace}, foundNodePolicy)

						if err != nil && errors.IsNotFound(err) {
							nodePolicyLogger.Info("Creating BackendTLSPolicy")
							if err = controllerutil.SetControllerReference(cloud, nodePolicy, r.Scheme); err == nil {
								err = r.Create(ctx, nodePolicy)
							}
						} else if err == nil {
							var needsUpdate bool
							needsUpdate, err = util.OvertakeControllerRef(cloud, foundNodePolicy, r.Scheme)
							if err == nil && util.CopyBackendTLSPolicyFields(nodePolicy, foundNodePolicy, nodePolicyLogger) {
								needsUpdate = true
							}
							if needsUpdate {
								nodePolicyLogger.Info("Updating BackendTLSPolicy")
								err = r.Update(ctx, foundNodePolicy)
							}
						}

						if err != nil {
							return err
						}
					}
				}
			}

			// Cleanup node BackendTLSPolicies (when hideNodes is true or when scaling down)
			backendTLSPolicyList := &gatewayv1.BackendTLSPolicyList{}
			err = r.List(ctx, backendTLSPolicyList, listOps)
			if err == nil {
				for _, policy := range backendTLSPolicyList.Items {
					// Skip the common BackendTLSPolicy
					if policy.Name == cloud.CommonBackendTLSPolicyName() {
						continue
					}

					// Delete if hideNodes is true, or if this node no longer exists (scale-down)
					shouldDelete := extAddressabilityOpts.HideNodes
					if !shouldDelete {
						// Check if this node still exists
						nodeExists := false
						for _, nodeName := range solrNodeNames {
							if policy.Name == cloud.NodeBackendTLSPolicyName(nodeName) {
								nodeExists = true
								break
							}
						}
						shouldDelete = !nodeExists
					}

					if shouldDelete {
						if extAddressabilityOpts.HideNodes {
							logger.Info("Deleting node BackendTLSPolicy (hideNodes=true)", "backendtlspolicy", policy.Name)
						} else {
							logger.Info("Deleting orphaned node BackendTLSPolicy", "backendtlspolicy", policy.Name)
						}
						err = r.Delete(ctx, &policy)
						if err != nil && !errors.IsNotFound(err) {
							return err
						}
					}
				}
			}
		} else {
			// If BackendTLSPolicy is not configured, clean up any existing BackendTLSPolicy resources
			backendTLSPolicyList := &gatewayv1.BackendTLSPolicyList{}
			err = r.List(ctx, backendTLSPolicyList, listOps)
			if err == nil && len(backendTLSPolicyList.Items) > 0 {
				logger.Info("Cleaning up BackendTLSPolicy resources (BackendTLSPolicy disabled)")
				for _, policy := range backendTLSPolicyList.Items {
					err = r.Delete(ctx, &policy)
					if err != nil && !errors.IsNotFound(err) {
						return err
					}
				}
			}
		}
	} else {
		// If not using Gateway method, clean up any existing HTTPRoutes and BackendTLSPolicy resources
		labelSelector := labels.SelectorFromSet(cloud.SharedLabels())
		listOps := &client.ListOptions{
			Namespace:     cloud.Namespace,
			LabelSelector: labelSelector,
		}

		// Clean up HTTPRoutes
		httpRouteList := &gatewayv1.HTTPRouteList{}
		err := r.List(ctx, httpRouteList, listOps)
		if err == nil && len(httpRouteList.Items) > 0 {
			logger.Info("Cleaning up HTTPRoutes (method changed from Gateway)")
			for _, httpRoute := range httpRouteList.Items {
				err = r.Delete(ctx, &httpRoute)
				if err != nil && !errors.IsNotFound(err) {
					return err
				}
			}
		}

		// Clean up BackendTLSPolicy resources
		backendTLSPolicyList := &gatewayv1.BackendTLSPolicyList{}
		err = r.List(ctx, backendTLSPolicyList, listOps)
		if err == nil && len(backendTLSPolicyList.Items) > 0 {
			logger.Info("Cleaning up BackendTLSPolicy resources (method changed from Gateway)")
			for _, policy := range backendTLSPolicyList.Items {
				err = r.Delete(ctx, &policy)
				if err != nil && !errors.IsNotFound(err) {
					return err
				}
			}
		}
	}
}
