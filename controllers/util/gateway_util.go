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

// GenerateCommonHTTPRoute creates an HTTPRoute for the common Solr service
func GenerateCommonHTTPRoute(solrCloud *solr.SolrCloud, nodeNames []string) *gatewayv1.HTTPRoute {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	var annotations map[string]string

	gatewayOpts := solrCloud.Spec.SolrAddressability.External.Gateway
	if gatewayOpts != nil {
		labels = MergeLabelsOrAnnotations(labels, gatewayOpts.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, gatewayOpts.Annotations)
	}

	extOpts := solrCloud.Spec.SolrAddressability.External

	// Create advertised domain name and possible additional domain names
	allDomains := append([]string{extOpts.DomainName}, extOpts.AdditionalDomainNames...)
	hostnames := make([]gatewayv1.Hostname, 0, len(allDomains)+len(gatewayOpts.AdditionalHostnames))
	for _, domain := range allDomains {
		hostname := gatewayv1.Hostname(solrCloud.ExternalCommonUrl(domain, false))
		hostnames = append(hostnames, hostname)
	}
	// Append user-specified additional hostnames for the common route
	for _, h := range gatewayOpts.AdditionalHostnames {
		hostnames = append(hostnames, gatewayv1.Hostname(h))
	}

	// Convert parentRefs from our type to Gateway API type
	parentRefs := make([]gatewayv1.ParentReference, len(gatewayOpts.ParentRefs))
	for i, ref := range gatewayOpts.ParentRefs {
		parentRef := gatewayv1.ParentReference{
			Name: gatewayv1.ObjectName(ref.Name),
		}
		if ref.Namespace != nil {
			namespace := gatewayv1.Namespace(*ref.Namespace)
			parentRef.Namespace = &namespace
		}
		if ref.SectionName != nil {
			sectionName := gatewayv1.SectionName(*ref.SectionName)
			parentRef.SectionName = &sectionName
		}
		parentRefs[i] = parentRef
	}

	// Determine backend port
	backendPort := gatewayv1.PortNumber(solrCloud.Spec.SolrAddressability.CommonServicePort)

	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.CommonHTTPRouteName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Hostnames: hostnames,
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(solrCloud.CommonServiceName()),
									Port: &backendPort,
								},
							},
						},
					},
				},
			},
		},
	}

	return httpRoute
}

// GenerateNodeHTTPRoute creates an HTTPRoute for individual Solr nodes
func GenerateNodeHTTPRoute(solrCloud *solr.SolrCloud, nodeName string) *gatewayv1.HTTPRoute {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	var annotations map[string]string

	gatewayOpts := solrCloud.Spec.SolrAddressability.External.Gateway
	if gatewayOpts != nil {
		labels = MergeLabelsOrAnnotations(labels, gatewayOpts.Labels)
		annotations = MergeLabelsOrAnnotations(annotations, gatewayOpts.Annotations)
	}

	extOpts := solrCloud.Spec.SolrAddressability.External

	// Create hostnames for all domains
	allDomains := append([]string{extOpts.DomainName}, extOpts.AdditionalDomainNames...)
	hostnames := make([]gatewayv1.Hostname, 0, len(allDomains))
	for _, domain := range allDomains {
		hostname := gatewayv1.Hostname(solrCloud.ExternalNodeUrl(nodeName, domain, false))
		hostnames = append(hostnames, hostname)
	}

	// Convert parentRefs
	parentRefs := make([]gatewayv1.ParentReference, len(gatewayOpts.ParentRefs))
	for i, ref := range gatewayOpts.ParentRefs {
		parentRef := gatewayv1.ParentReference{
			Name: gatewayv1.ObjectName(ref.Name),
		}
		if ref.Namespace != nil {
			namespace := gatewayv1.Namespace(*ref.Namespace)
			parentRef.Namespace = &namespace
		}
		if ref.SectionName != nil {
			sectionName := gatewayv1.SectionName(*ref.SectionName)
			parentRef.SectionName = &sectionName
		}
		parentRefs[i] = parentRef
	}

	// Determine backend port (uses NodePort which may be overridden)
	backendPort := gatewayv1.PortNumber(solrCloud.NodePort())

	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.NodeHTTPRouteName(nodeName),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Hostnames: hostnames,
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(nodeName),
									Port: &backendPort,
								},
							},
						},
					},
				},
			},
		},
	}

	return httpRoute
}

// CopyHTTPRouteFields copies the fields from one HTTPRoute to another
// Returns true if there are differences between the two objects
func CopyHTTPRouteFields(from, to *gatewayv1.HTTPRoute, logger logr.Logger) bool {
	requireUpdate := false

	// Copy labels
	if !DeepEqualWithNils(to.Labels, from.Labels) {
		logger.Info("HTTPRoute labels have changed")
		requireUpdate = true
		to.Labels = from.Labels
	}

	// Copy annotations
	if !DeepEqualWithNils(to.Annotations, from.Annotations) {
		logger.Info("HTTPRoute annotations have changed")
		requireUpdate = true
		to.Annotations = from.Annotations
	}

	// Copy spec
	if !DeepEqualWithNils(to.Spec.ParentRefs, from.Spec.ParentRefs) {
		logger.Info("HTTPRoute parentRefs have changed")
		requireUpdate = true
		to.Spec.ParentRefs = from.Spec.ParentRefs
	}

	if !DeepEqualWithNils(to.Spec.Hostnames, from.Spec.Hostnames) {
		logger.Info("HTTPRoute hostnames have changed")
		requireUpdate = true
		to.Spec.Hostnames = from.Spec.Hostnames
	}

	if !DeepEqualWithNils(to.Spec.Rules, from.Spec.Rules) {
		logger.Info("HTTPRoute rules have changed")
		requireUpdate = true
		to.Spec.Rules = from.Spec.Rules
	}

	return requireUpdate
}
