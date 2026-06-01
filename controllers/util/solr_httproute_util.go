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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GenerateHTTPRoutes returns a list of HTTPRoute pointers generated for the SolrCloud,
// one for each entry in Spec.CustomSolrKubeOptions.HTTPRouteOptions.
func GenerateHTTPRoutes(solrCloud *solr.SolrCloud) []*gatewayv1.HTTPRoute {
	routes := make([]*gatewayv1.HTTPRoute, 0, len(solrCloud.Spec.CustomSolrKubeOptions.HTTPRouteOptions))
	for i := range solrCloud.Spec.CustomSolrKubeOptions.HTTPRouteOptions {
		routes = append(routes, GenerateHTTPRoute(solrCloud, &solrCloud.Spec.CustomSolrKubeOptions.HTTPRouteOptions[i]))
	}
	return routes
}

// GenerateHTTPRoute returns an HTTPRoute pointer generated for the SolrCloud based on the given HTTPRouteOptions.
// The routing rule routes all traffic (PathPrefix "/") to the Solr common service.
func GenerateHTTPRoute(solrCloud *solr.SolrCloud, opts *solr.HTTPRouteOptions) *gatewayv1.HTTPRoute {
	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	labels = MergeLabelsOrAnnotations(labels, opts.Labels)

	var annotations map[string]string
	if len(opts.Annotations) > 0 {
		annotations = MergeLabelsOrAnnotations(annotations, opts.Annotations)
	}

	// Build Gateway API ParentReferences from the options
	parentRefs := make([]gatewayv1.ParentReference, 0, len(opts.ParentRefs))
	for _, pr := range opts.ParentRefs {
		ref := gatewayv1.ParentReference{
			Name: gatewayv1.ObjectName(pr.Name),
		}
		if pr.Group != nil {
			g := gatewayv1.Group(*pr.Group)
			ref.Group = &g
		}
		if pr.Kind != nil {
			k := gatewayv1.Kind(*pr.Kind)
			ref.Kind = &k
		}
		if pr.Namespace != nil {
			ns := gatewayv1.Namespace(*pr.Namespace)
			ref.Namespace = &ns
		}
		if pr.SectionName != nil {
			sn := gatewayv1.SectionName(*pr.SectionName)
			ref.SectionName = &sn
		}
		if pr.Port != nil {
			pn := gatewayv1.PortNumber(*pr.Port)
			ref.Port = &pn
		}
		parentRefs = append(parentRefs, ref)
	}

	// Build hostnames
	hostnames := make([]gatewayv1.Hostname, 0, len(opts.Hostnames))
	for _, h := range opts.Hostnames {
		hostnames = append(hostnames, gatewayv1.Hostname(h))
	}

	// Build routing rules: route all traffic (PathPrefix "/") to the Solr common service.
	// This mirrors the common Ingress rule that routes to the common service.
	pathPrefix := "/"
	pathPrefixType := gatewayv1.PathMatchPathPrefix
	commonServicePort := gatewayv1.PortNumber(solrCloud.Spec.SolrAddressability.CommonServicePort)

	rules := []gatewayv1.HTTPRouteRule{
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathPrefixType,
						Value: &pathPrefix,
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName(solrCloud.CommonServiceName()),
							Port: &commonServicePort,
						},
					},
				},
			},
		},
	}

	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Hostnames: hostnames,
			Rules:     rules,
		},
	}
}
