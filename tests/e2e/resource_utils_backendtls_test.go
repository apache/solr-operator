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

package e2e

import (
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func expectBackendTLSPolicy(ctx context.Context, parentResource client.Object, policyName string, additionalOffset ...int) *gatewayv1.BackendTLSPolicy {
	return expectBackendTLSPolicyWithChecks(ctx, parentResource, policyName, nil, resolveOffset(additionalOffset))
}

func expectBackendTLSPolicyWithChecks(ctx context.Context, parentResource client.Object, policyName string, additionalChecks func(Gomega, *gatewayv1.BackendTLSPolicy), additionalOffset ...int) *gatewayv1.BackendTLSPolicy {
	policy := &gatewayv1.BackendTLSPolicy{}
	EventuallyWithOffset(resolveOffset(additionalOffset), func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, resourceKey(parentResource, policyName), policy)).To(Succeed(), "Expected BackendTLSPolicy does not exist")

		if additionalChecks != nil {
			additionalChecks(g, policy)
		}
	}).Should(Succeed())
	return policy
}

func expectNoBackendTLSPolicy(ctx context.Context, parentResource client.Object, policyName string, additionalOffset ...int) {
	ConsistentlyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, policyName), &gatewayv1.BackendTLSPolicy{})
	}).Should(MatchError("backendtlspolicies.gateway.networking.k8s.io \""+policyName+"\" not found"), "BackendTLSPolicy exists when it should not")
}

func eventuallyExpectNoBackendTLSPolicy(ctx context.Context, parentResource client.Object, policyName string, additionalOffset ...int) {
	EventuallyWithOffset(resolveOffset(additionalOffset), func() error {
		return k8sClient.Get(ctx, resourceKey(parentResource, policyName), &gatewayv1.BackendTLSPolicy{})
	}).Should(MatchError("backendtlspolicies.gateway.networking.k8s.io \""+policyName+"\" not found"), "BackendTLSPolicy exists when it should not")
}
