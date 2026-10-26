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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// backendCACertKey is the key, within the Solr TLS keystore secret and the operator-generated CA ConfigMap,
// that holds the PEM-encoded CA certificate used to validate the Solr backend certificate.
const backendCACertKey = "ca.crt"

func (r *SolrCloudReconciler) reconcileGatewayResources(ctx context.Context, cloud *solrv1beta1.SolrCloud, solrNodeNames []string, logger logr.Logger) error {
	extAddressabilityOpts := cloud.Spec.SolrAddressability.External

	// If not using Gateway method, clean up any existing HTTPRoutes and BackendTLSPolicy resources
	if extAddressabilityOpts == nil || extAddressabilityOpts.Method != solrv1beta1.Gateway {
		return r.cleanupGatewayResources(ctx, cloud, logger)
	}

	// Validate that gateway config is provided
	if extAddressabilityOpts.Gateway == nil || len(extAddressabilityOpts.Gateway.ParentRefs) == 0 {
		r.Recorder.Event(cloud, corev1.EventTypeWarning, "InvalidConfiguration", "gateway.parentRefs is required when using method=Gateway")
		return fmt.Errorf("gateway.parentRefs is required when using method=Gateway")
	}

	if err := r.reconcileGatewayHTTPRoutes(ctx, cloud, extAddressabilityOpts, solrNodeNames, logger); err != nil {
		return err
	}

	return r.reconcileGatewayBackendTLSPolicies(ctx, cloud, extAddressabilityOpts, solrNodeNames, logger)
}

func (r *SolrCloudReconciler) reconcileGatewayHTTPRoutes(ctx context.Context, cloud *solrv1beta1.SolrCloud, extAddressabilityOpts *solrv1beta1.ExternalAddressability, solrNodeNames []string, logger logr.Logger) error {
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
	listOps := &client.ListOptions{
		Namespace:     cloud.Namespace,
		LabelSelector: labels.SelectorFromSet(cloud.SharedLabels()),
	}
	if err := r.List(ctx, httpRouteList, listOps); err != nil {
		return err
	}
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
			if err := r.Delete(ctx, &httpRoute); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (r *SolrCloudReconciler) reconcileGatewayBackendTLSPolicies(ctx context.Context, cloud *solrv1beta1.SolrCloud, extAddressabilityOpts *solrv1beta1.ExternalAddressability, solrNodeNames []string, logger logr.Logger) error {
	listOps := &client.ListOptions{
		Namespace:     cloud.Namespace,
		LabelSelector: labels.SelectorFromSet(cloud.SharedLabels()),
	}

	// Backend TLS is only managed when Solr is running with TLS behind a Gateway. When it isn't, clean up
	// any resources previously created for it.
	if !cloud.UsesGatewayBackendTLS() {
		if err := r.deleteBackendTLSPolicies(ctx, cloud, listOps, "Cleaning up BackendTLSPolicy resources (backend TLS disabled)", logger); err != nil {
			return err
		}
		return r.deleteBackendTLSCAConfigMap(ctx, cloud, logger)
	}

	// Resolve the CA trust source once: an explicit override, or derived from the Solr TLS secret.
	caCertRefs, wellKnownCACertificates, err := r.resolveBackendTLSCA(ctx, cloud, logger)
	if err != nil {
		return err
	}

	// Reconcile Common BackendTLSPolicy (if not hidden)
	if !extAddressabilityOpts.HideCommon {
		commonPolicy := util.GenerateCommonBackendTLSPolicy(cloud, caCertRefs, wellKnownCACertificates)
		commonPolicyLogger := logger.WithValues("backendtlspolicy", commonPolicy.Name)
		foundCommonPolicy := &gatewayv1.BackendTLSPolicy{}
		err := r.Get(ctx, types.NamespacedName{Name: commonPolicy.Name, Namespace: commonPolicy.Namespace}, foundCommonPolicy)
		if err != nil && errors.IsNotFound(err) {
			commonPolicyLogger.Info("Creating common BackendTLSPolicy")
			if err = controllerutil.SetControllerReference(cloud, commonPolicy, r.Scheme); err == nil {
				err = r.Create(ctx, commonPolicy)
			}
		} else if err == nil {
			var needsUpdate bool
			needsUpdate, err = util.OvertakeControllerRef(cloud, foundCommonPolicy, r.Scheme)
			needsUpdate = util.CopyBackendTLSPolicyFields(commonPolicy, foundCommonPolicy, commonPolicyLogger) || needsUpdate

			if needsUpdate && err == nil {
				commonPolicyLogger.Info("Updating common BackendTLSPolicy")
				err = r.Update(ctx, foundCommonPolicy)
			}
		}
		if err != nil {
			return err
		}
	} else {
		// Delete common BackendTLSPolicy if it exists but should be hidden
		foundCommonPolicy := &gatewayv1.BackendTLSPolicy{}
		err := r.Get(ctx, types.NamespacedName{Name: cloud.CommonBackendTLSPolicyName(), Namespace: cloud.GetNamespace()}, foundCommonPolicy)
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
			nodePolicy := util.GenerateNodeBackendTLSPolicy(cloud, nodeName, caCertRefs, wellKnownCACertificates)
			nodePolicyLogger := logger.WithValues("backendtlspolicy", nodePolicy.Name)
			foundNodePolicy := &gatewayv1.BackendTLSPolicy{}
			err := r.Get(ctx, types.NamespacedName{Name: nodePolicy.Name, Namespace: nodePolicy.Namespace}, foundNodePolicy)
			if err != nil && errors.IsNotFound(err) {
				nodePolicyLogger.Info("Creating node BackendTLSPolicy")
				if err = controllerutil.SetControllerReference(cloud, nodePolicy, r.Scheme); err == nil {
					err = r.Create(ctx, nodePolicy)
				}
			} else if err == nil {
				var needsUpdate bool
				needsUpdate, err = util.OvertakeControllerRef(cloud, foundNodePolicy, r.Scheme)
				needsUpdate = util.CopyBackendTLSPolicyFields(nodePolicy, foundNodePolicy, nodePolicyLogger) || needsUpdate

				if needsUpdate && err == nil {
					nodePolicyLogger.Info("Updating node BackendTLSPolicy")
					err = r.Update(ctx, foundNodePolicy)
				}
			}
			if err != nil {
				return err
			}
		}
	}

	// Cleanup orphaned node BackendTLSPolicies (when hideNodes is true or when scaling down)
	backendTLSPolicyList := &gatewayv1.BackendTLSPolicyList{}
	if err := r.List(ctx, backendTLSPolicyList, listOps); err != nil {
		return err
	}
	for _, policy := range backendTLSPolicyList.Items {
		// Skip the common BackendTLSPolicy
		if policy.Name == cloud.CommonBackendTLSPolicyName() {
			continue
		}

		// Delete if hideNodes is true, or if this node no longer exists (scale-down)
		shouldDelete := extAddressabilityOpts.HideNodes
		if !shouldDelete {
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
			if err := r.Delete(ctx, &policy); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// resolveBackendTLSCA determines the CA trust source for the backend TLS policies. If the user supplied an
// explicit override it is used verbatim; otherwise the operator derives the CA from the `ca.crt` entry of the
// Solr TLS keystore secret, materializing it into a ConfigMap that the BackendTLSPolicies reference.
func (r *SolrCloudReconciler) resolveBackendTLSCA(ctx context.Context, cloud *solrv1beta1.SolrCloud, logger logr.Logger) ([]gatewayv1.LocalObjectReference, *string, error) {
	backendTLSPolicy := cloud.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy

	if backendTLSPolicy.HasCAOverride() {
		if len(backendTLSPolicy.CACertificateRefs) > 0 && backendTLSPolicy.WellKnownCACertificates != nil && *backendTLSPolicy.WellKnownCACertificates != "" {
			r.Recorder.Event(cloud, corev1.EventTypeWarning, "InvalidConfiguration", "gateway.backendTLSPolicy.caCertificateRefs and wellKnownCACertificates are mutually exclusive")
			return nil, nil, fmt.Errorf("invalid config: only one of `gateway.backendTLSPolicy.caCertificateRefs` or `wellKnownCACertificates` may be set")
		}
		// A user-provided CA supersedes the derived one, so remove any ConfigMap the operator previously materialized.
		if err := r.deleteBackendTLSCAConfigMap(ctx, cloud, logger); err != nil {
			return nil, nil, err
		}
		return util.ConvertCACertificateRefs(backendTLSPolicy.CACertificateRefs), backendTLSPolicy.WellKnownCACertificates, nil
	}

	// No override: derive the CA from the Solr TLS secret into an operator-managed ConfigMap.
	configMapName, err := r.reconcileBackendTLSCAConfigMap(ctx, cloud, logger)
	if err != nil {
		return nil, nil, err
	}
	caCertRefs := []gatewayv1.LocalObjectReference{
		{
			Group: gatewayv1.Group(""),
			Kind:  gatewayv1.Kind("ConfigMap"),
			Name:  gatewayv1.ObjectName(configMapName),
		},
	}
	return caCertRefs, nil, nil
}

// reconcileBackendTLSCAConfigMap materializes a ConfigMap containing the CA certificate (`ca.crt`) copied from
// the Solr TLS keystore secret, so the Gateway can validate the backend certificate. It returns the name of the
// ConfigMap. If the CA cannot be derived (e.g. mountedTLSDir, or the secret has no `ca.crt`), it records an event
// and returns an error instructing the user to configure gateway.backendTLSPolicy explicitly.
func (r *SolrCloudReconciler) reconcileBackendTLSCAConfigMap(ctx context.Context, cloud *solrv1beta1.SolrCloud, logger logr.Logger) (string, error) {
	tls := cloud.Spec.SolrTLS
	if tls == nil || tls.PKCS12Secret == nil {
		msg := "Cannot derive a backend CA certificate from the Solr TLS configuration; set gateway.backendTLSPolicy.caCertificateRefs or wellKnownCACertificates"
		r.Recorder.Event(cloud, corev1.EventTypeWarning, "InvalidConfiguration", msg)
		return "", fmt.Errorf("%s", msg)
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: tls.PKCS12Secret.Name, Namespace: cloud.Namespace}, secret); err != nil {
		return "", err
	}
	caCert, ok := secret.Data[backendCACertKey]
	if !ok || len(caCert) == 0 {
		msg := fmt.Sprintf("Solr TLS secret %q has no %q entry to derive a backend CA from; set gateway.backendTLSPolicy.caCertificateRefs or wellKnownCACertificates", tls.PKCS12Secret.Name, backendCACertKey)
		r.Recorder.Event(cloud, corev1.EventTypeWarning, "InvalidConfiguration", msg)
		return "", fmt.Errorf("%s", msg)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cloud.BackendTLSCACertConfigMapName(),
			Namespace: cloud.Namespace,
			Labels:    cloud.SharedLabelsWith(cloud.GetLabels()),
		},
		Data: map[string]string{backendCACertKey: string(caCert)},
	}

	configMapLogger := logger.WithValues("configMap", desired.Name)
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		configMapLogger.Info("Creating backend CA ConfigMap")
		if err = controllerutil.SetControllerReference(cloud, desired, r.Scheme); err == nil {
			err = r.Create(ctx, desired)
		}
	} else if err == nil {
		var needsUpdate bool
		needsUpdate, err = util.OvertakeControllerRef(cloud, found, r.Scheme)
		if !util.DeepEqualWithNils(found.Data, desired.Data) {
			found.Data = desired.Data
			needsUpdate = true
		}
		if !util.DeepEqualWithNils(found.Labels, desired.Labels) {
			found.Labels = desired.Labels
			needsUpdate = true
		}
		if needsUpdate && err == nil {
			configMapLogger.Info("Updating backend CA ConfigMap")
			err = r.Update(ctx, found)
		}
	}
	if err != nil {
		return "", err
	}
	return desired.Name, nil
}

// deleteBackendTLSCAConfigMap removes the operator-generated backend CA ConfigMap if it exists.
func (r *SolrCloudReconciler) deleteBackendTLSCAConfigMap(ctx context.Context, cloud *solrv1beta1.SolrCloud, logger logr.Logger) error {
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cloud.BackendTLSCACertConfigMapName(), Namespace: cloud.GetNamespace()}, found)
	if err == nil {
		logger.Info("Deleting backend CA ConfigMap")
		if err = r.Delete(ctx, found); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *SolrCloudReconciler) cleanupGatewayResources(ctx context.Context, cloud *solrv1beta1.SolrCloud, logger logr.Logger) error {
	listOps := &client.ListOptions{
		Namespace:     cloud.Namespace,
		LabelSelector: labels.SelectorFromSet(cloud.SharedLabels()),
	}

	// Clean up HTTPRoutes
	httpRouteList := &gatewayv1.HTTPRouteList{}
	if err := r.List(ctx, httpRouteList, listOps); err != nil {
		return err
	}
	if len(httpRouteList.Items) > 0 {
		logger.Info("Cleaning up HTTPRoutes (method changed from Gateway)")
		for _, httpRoute := range httpRouteList.Items {
			if err := r.Delete(ctx, &httpRoute); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	// Clean up BackendTLSPolicy resources and the derived CA ConfigMap
	if err := r.deleteBackendTLSPolicies(ctx, cloud, listOps, "Cleaning up BackendTLSPolicy resources (method changed from Gateway)", logger); err != nil {
		return err
	}
	return r.deleteBackendTLSCAConfigMap(ctx, cloud, logger)
}

// deleteBackendTLSPolicies deletes all BackendTLSPolicy resources owned by the cloud.
func (r *SolrCloudReconciler) deleteBackendTLSPolicies(ctx context.Context, cloud *solrv1beta1.SolrCloud, listOps *client.ListOptions, message string, logger logr.Logger) error {
	backendTLSPolicyList := &gatewayv1.BackendTLSPolicyList{}
	if err := r.List(ctx, backendTLSPolicyList, listOps); err != nil {
		return err
	}
	if len(backendTLSPolicyList.Items) > 0 {
		logger.Info(message)
		for _, policy := range backendTLSPolicyList.Items {
			if err := r.Delete(ctx, &policy); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}
