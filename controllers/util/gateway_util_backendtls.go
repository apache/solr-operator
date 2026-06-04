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

// GenerateCommonBackendTLSPolicy creates a BackendTLSPolicy for the common Solr service
func GenerateCommonBackendTLSPolicy(solrCloud *solr.SolrCloud) *gatewayv1.BackendTLSPolicy {
	if solrCloud.Spec.SolrAddressability.External.Gateway == nil ||
		solrCloud.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy == nil {
		return nil
	}

	// Get the full FQDN for hostname validation
	domainName := solrCloud.Spec.SolrAddressability.External.DomainName
	fqdn := solrCloud.ExternalCommonUrl(domainName, false)

	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	backendTLSConfig := solrCloud.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy

	// Convert CA certificate refs
	var caCertRefs []gatewayv1.LocalObjectReference
	if backendTLSConfig.CACertificateRefs != nil {
		caCertRefs = make([]gatewayv1.LocalObjectReference, len(backendTLSConfig.CACertificateRefs))
		for i, ref := range backendTLSConfig.CACertificateRefs {
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
			certRef := gatewayv1.LocalObjectReference{
				Group: group,
				Kind:  kind,
				Name:  gatewayv1.ObjectName(ref.Name),
			}
			caCertRefs[i] = certRef
		}
	}

	policy := &gatewayv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.CommonBackendTLSPolicyName(),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  gatewayv1.ObjectName(solrCloud.CommonServiceName()),
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname: gatewayv1.PreciseHostname(fqdn),
			},
		},
	}

	// Set CA certificates or well-known CAs
	if caCertRefs != nil {
		policy.Spec.Validation.CACertificateRefs = caCertRefs
	} else if backendTLSConfig.WellKnownCACertificates != nil {
		wellKnown := gatewayv1.WellKnownCACertificatesType(*backendTLSConfig.WellKnownCACertificates)
		policy.Spec.Validation.WellKnownCACertificates = &wellKnown
	}

	return policy
}

// GenerateNodeBackendTLSPolicy creates a BackendTLSPolicy for individual Solr node service
func GenerateNodeBackendTLSPolicy(solrCloud *solr.SolrCloud, nodeName string) *gatewayv1.BackendTLSPolicy {
	if solrCloud.Spec.SolrAddressability.External.Gateway == nil ||
		solrCloud.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy == nil {
		return nil
	}

	// Get the full FQDN for hostname validation
	domainName := solrCloud.Spec.SolrAddressability.External.DomainName
	fqdn := solrCloud.ExternalNodeUrl(nodeName, domainName, false)

	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	backendTLSConfig := solrCloud.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy

	// Convert CA certificate refs
	var caCertRefs []gatewayv1.LocalObjectReference
	if backendTLSConfig.CACertificateRefs != nil {
		caCertRefs = make([]gatewayv1.LocalObjectReference, len(backendTLSConfig.CACertificateRefs))
		for i, ref := range backendTLSConfig.CACertificateRefs {
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
			certRef := gatewayv1.LocalObjectReference{
				Group: group,
				Kind:  kind,
				Name:  gatewayv1.ObjectName(ref.Name),
			}
			caCertRefs[i] = certRef
		}
	}

	policy := &gatewayv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.NodeBackendTLSPolicyName(nodeName),
			Namespace: solrCloud.GetNamespace(),
			Labels:    labels,
		},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  gatewayv1.ObjectName(nodeName),
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname: gatewayv1.PreciseHostname(fqdn),
			},
		},
	}

	// Set CA certificates or well-known CAs
	if caCertRefs != nil {
		policy.Spec.Validation.CACertificateRefs = caCertRefs
	} else if backendTLSConfig.WellKnownCACertificates != nil {
		wellKnown := gatewayv1.WellKnownCACertificatesType(*backendTLSConfig.WellKnownCACertificates)
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
