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

package util

import (
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ConvertCACertificateRefs converts the operator's GatewayCertificateReference values into Gateway API
// LocalObjectReference values, applying the ConfigMap/core-group defaults for unspecified fields.
func ConvertCACertificateRefs(refs []solr.GatewayCertificateReference) []gatewayv1.LocalObjectReference {
	if len(refs) == 0 {
		return nil
	}
	converted := make([]gatewayv1.LocalObjectReference, len(refs))
	for i, ref := range refs {
		// Default to ConfigMap if kind is not specified
		kind := gatewayv1.Kind("ConfigMap")
		if ref.Kind != nil {
			kind = gatewayv1.Kind(*ref.Kind)
		}
		// Default to empty group (core API)
		group := gatewayv1.Group("")
		if ref.Group != nil {
			group = gatewayv1.Group(*ref.Group)
		}
		converted[i] = gatewayv1.LocalObjectReference{
			Group: group,
			Kind:  kind,
			Name:  gatewayv1.ObjectName(ref.Name),
		}
	}
	return converted
}

// GenerateCommonBackendTLSPolicy creates a BackendTLSPolicy for the common Solr service. The CA trust
// source (caCertRefs or wellKnownCACertificates) is resolved by the caller.
func GenerateCommonBackendTLSPolicy(solrCloud *solr.SolrCloud, caCertRefs []gatewayv1.LocalObjectReference, wellKnownCACertificates *string) *gatewayv1.BackendTLSPolicy {
	return generateBackendTLSPolicy(solrCloud, solrCloud.CommonBackendTLSPolicyName(), solrCloud.CommonServiceName(), solrCloud.InternalCommonUrl(false), caCertRefs, wellKnownCACertificates)
}

// GenerateNodeBackendTLSPolicy creates a BackendTLSPolicy for an individual Solr node service. The CA trust
// source (caCertRefs or wellKnownCACertificates) is resolved by the caller.
func GenerateNodeBackendTLSPolicy(solrCloud *solr.SolrCloud, nodeName string, caCertRefs []gatewayv1.LocalObjectReference, wellKnownCACertificates *string) *gatewayv1.BackendTLSPolicy {
	return generateBackendTLSPolicy(solrCloud, solrCloud.NodeBackendTLSPolicyName(nodeName), nodeName, solrCloud.InternalNodeUrl(nodeName, false), caCertRefs, wellKnownCACertificates)
}

// generateBackendTLSPolicy builds a BackendTLSPolicy targeting the given Service. The validation hostname
// defaults to the internal Solr service address (which the Solr server certificate must cover for TLS
// inter-node communication), and may be overridden via gateway.backendTLSPolicy.hostname.
func generateBackendTLSPolicy(solrCloud *solr.SolrCloud, name, serviceName, defaultHostname string, caCertRefs []gatewayv1.LocalObjectReference, wellKnownCACertificates *string) *gatewayv1.BackendTLSPolicy {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())

	hostname := defaultHostname
	if gateway := solrCloud.Spec.SolrAddressability.External.Gateway; gateway != nil && gateway.BackendTLSPolicy != nil && gateway.BackendTLSPolicy.Hostname != nil {
		hostname = *gateway.BackendTLSPolicy.Hostname
	}

	policy := &gatewayv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  gatewayv1.ObjectName(serviceName),
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname: gatewayv1.PreciseHostname(hostname),
			},
		},
	}

	// Set CA certificates or well-known CAs (exactly one is expected; the caller enforces this)
	if len(caCertRefs) > 0 {
		policy.Spec.Validation.CACertificateRefs = caCertRefs
	} else if wellKnownCACertificates != nil && *wellKnownCACertificates != "" {
		wellKnown := gatewayv1.WellKnownCACertificatesType(*wellKnownCACertificates)
		policy.Spec.Validation.WellKnownCACertificates = &wellKnown
	}

	return policy
}

// CopyBackendTLSPolicyFields copies fields from one BackendTLSPolicy to another
func CopyBackendTLSPolicyFields(from, to *gatewayv1.BackendTLSPolicy, logger logr.Logger) bool {
	requireUpdate := false

	if !DeepEqualWithNils(to.Labels, from.Labels) {
		logger.Info("BackendTLSPolicy labels have changed")
		requireUpdate = true
		to.Labels = from.Labels
	}

	if !DeepEqualWithNils(to.Spec.TargetRefs, from.Spec.TargetRefs) {
		logger.Info("BackendTLSPolicy targetRefs have changed")
		requireUpdate = true
		to.Spec.TargetRefs = from.Spec.TargetRefs
	}

	if !DeepEqualWithNils(to.Spec.Validation, from.Spec.Validation) {
		logger.Info("BackendTLSPolicy validation have changed")
		requireUpdate = true
		to.Spec.Validation = from.Spec.Validation
	}

	return requireUpdate
}
