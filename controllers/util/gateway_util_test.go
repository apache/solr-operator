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
	"testing"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func gatewayTestSolrCloud() *solr.SolrCloud {
	gatewayNs := "gateway-ns"
	return &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "solr-ns"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				CommonServicePort: 80,
				External: &solr.ExternalAddressability{
					Method:     solr.Gateway,
					DomainName: "solr.example.com",
					Gateway: &solr.SolrGatewayOptions{
						ParentRefs: []solr.GatewayParentReference{
							{Name: "my-gateway", Namespace: &gatewayNs},
						},
					},
				},
			},
		},
	}
}

func TestGenerateCommonHTTPRoute(t *testing.T) {
	sc := gatewayTestSolrCloud()
	route := GenerateCommonHTTPRoute(sc, []string{"example-solrcloud-0"})

	assert.Equal(t, sc.CommonHTTPRouteName(), route.Name, "Common HTTPRoute has wrong name")
	assert.Equal(t, sc.Namespace, route.Namespace, "Common HTTPRoute has wrong namespace")
	assert.Len(t, route.Spec.ParentRefs, 1, "Common HTTPRoute should have one parentRef")
	assert.Equal(t, "my-gateway", string(route.Spec.ParentRefs[0].Name), "Wrong parentRef name")
	assert.Equal(t, "gateway-ns", string(*route.Spec.ParentRefs[0].Namespace), "Wrong parentRef namespace")
	assert.NotEmpty(t, route.Spec.Hostnames, "Common HTTPRoute should advertise at least one hostname")
	assert.Equal(t, gatewayv1.Hostname(sc.ExternalCommonUrl(sc.Spec.SolrAddressability.External.DomainName, false)), route.Spec.Hostnames[0], "Common HTTPRoute hostname should be the external common URL")
	assert.Len(t, route.Spec.Rules, 1, "Common HTTPRoute should have one rule")
	assert.Equal(t, sc.CommonServiceName(), string(route.Spec.Rules[0].BackendRefs[0].Name), "Common HTTPRoute should target the common Service")
}

func TestGenerateCommonBackendTLSPolicy_DerivedDefaults(t *testing.T) {
	sc := gatewayTestSolrCloud()
	caCertRefs := []gatewayv1.LocalObjectReference{
		{Group: "", Kind: "ConfigMap", Name: gatewayv1.ObjectName(sc.BackendTLSCACertConfigMapName())},
	}

	policy := GenerateCommonBackendTLSPolicy(sc, caCertRefs, nil)

	assert.Equal(t, sc.CommonBackendTLSPolicyName(), policy.Name, "Common BackendTLSPolicy has wrong name")
	assert.Len(t, policy.Spec.TargetRefs, 1, "BackendTLSPolicy should have one target ref")
	assert.Equal(t, sc.CommonServiceName(), string(policy.Spec.TargetRefs[0].Name), "BackendTLSPolicy should target the common Service")
	assert.EqualValues(t, "Service", policy.Spec.TargetRefs[0].Kind, "BackendTLSPolicy should target a Service")

	// The validation hostname must default to the INTERNAL service address (which the Solr cert covers),
	// not the external addressability domain.
	assert.Equal(t, gatewayv1.PreciseHostname(sc.InternalCommonUrl(false)), policy.Spec.Validation.Hostname, "Backend TLS hostname should default to the internal service address")
	assert.NotEqual(t, gatewayv1.PreciseHostname(sc.ExternalCommonUrl(sc.Spec.SolrAddressability.External.DomainName, false)), policy.Spec.Validation.Hostname, "Backend TLS hostname must not be the external domain")

	assert.Equal(t, caCertRefs, policy.Spec.Validation.CACertificateRefs, "BackendTLSPolicy should use the resolved CA cert refs")
	assert.Nil(t, policy.Spec.Validation.WellKnownCACertificates, "WellKnownCACertificates should be unset when CA refs are provided")
}

func TestGenerateNodeBackendTLSPolicy_Hostname(t *testing.T) {
	sc := gatewayTestSolrCloud()
	nodeName := "example-solrcloud-0"
	caCertRefs := []gatewayv1.LocalObjectReference{{Kind: "ConfigMap", Name: "ca"}}

	policy := GenerateNodeBackendTLSPolicy(sc, nodeName, caCertRefs, nil)

	assert.Equal(t, sc.NodeBackendTLSPolicyName(nodeName), policy.Name, "Node BackendTLSPolicy has wrong name")
	assert.Equal(t, nodeName, string(policy.Spec.TargetRefs[0].Name), "Node BackendTLSPolicy should target the node Service")
	assert.Equal(t, gatewayv1.PreciseHostname(sc.InternalNodeUrl(nodeName, false)), policy.Spec.Validation.Hostname, "Node backend TLS hostname should default to the internal node service address")
}

func TestGenerateBackendTLSPolicy_HostnameOverride(t *testing.T) {
	sc := gatewayTestSolrCloud()
	override := "solr.internal.example.com"
	sc.Spec.SolrAddressability.External.Gateway.BackendTLSPolicy = &solr.SolrBackendTLSPolicy{Hostname: &override}

	policy := GenerateCommonBackendTLSPolicy(sc, []gatewayv1.LocalObjectReference{{Kind: "ConfigMap", Name: "ca"}}, nil)

	assert.Equal(t, gatewayv1.PreciseHostname(override), policy.Spec.Validation.Hostname, "Hostname override should be honored")
}

func TestGenerateBackendTLSPolicy_WellKnownCACertificates(t *testing.T) {
	sc := gatewayTestSolrCloud()
	wellKnown := "System"

	policy := GenerateCommonBackendTLSPolicy(sc, nil, &wellKnown)

	assert.Nil(t, policy.Spec.Validation.CACertificateRefs, "CACertificateRefs should be nil when using wellKnownCACertificates")
	if assert.NotNil(t, policy.Spec.Validation.WellKnownCACertificates, "WellKnownCACertificates should be set") {
		assert.EqualValues(t, "System", *policy.Spec.Validation.WellKnownCACertificates, "WellKnownCACertificates should be 'System'")
	}
}

func TestConvertCACertificateRefs(t *testing.T) {
	assert.Nil(t, ConvertCACertificateRefs(nil), "Converting no refs should yield nil")

	customKind := "Secret"
	customGroup := "example.com"
	refs := []solr.GatewayCertificateReference{
		{Name: "default-configmap"},
		{Name: "custom", Kind: &customKind, Group: &customGroup},
	}

	converted := ConvertCACertificateRefs(refs)
	assert.Len(t, converted, 2, "Should convert all refs")

	// First ref uses the ConfigMap / core-group defaults.
	assert.EqualValues(t, "ConfigMap", converted[0].Kind, "Kind should default to ConfigMap")
	assert.EqualValues(t, "", converted[0].Group, "Group should default to the core API group")
	assert.EqualValues(t, "default-configmap", converted[0].Name, "Name should be preserved")

	// Second ref honors explicit kind/group.
	assert.EqualValues(t, "Secret", converted[1].Kind, "Explicit kind should be honored")
	assert.EqualValues(t, "example.com", converted[1].Group, "Explicit group should be honored")
}
