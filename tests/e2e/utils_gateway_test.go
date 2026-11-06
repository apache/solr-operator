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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// gatewayTestClientImage must be kept in sync with the image preloaded in manage_e2e_tests.sh.
	gatewayTestClientImage   = "curlimages/curl:8.11.1"
	gatewayTestClientPodName = "gateway-test-client"
	envoyGatewaySystemNs     = "envoy-gateway-system"
)

// deployGatewayTestClientPod creates a long-lived curl pod in the given namespace, used to issue in-cluster HTTP
// requests through the Gateway (whose Envoy proxy is only reachable from within the cluster). It waits for the pod
// to be running and registers cleanup. The pod name is returned.
func deployGatewayTestClientPod(ctx context.Context, namespace string) string {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: gatewayTestClientPodName, Namespace: namespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   gatewayTestClientImage,
					Command: []string{"sleep", "infinity"},
				},
			},
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, pod)).To(Succeed(), "Failed to create the gateway test client pod")
	DeferCleanup(func(ctx context.Context) {
		_ = k8sClient.Delete(ctx, pod)
	})

	EventuallyWithOffset(1, func(g Gomega) {
		found := &corev1.Pod{}
		g.Expect(k8sClient.Get(ctx, resourceKey(pod, gatewayTestClientPodName), found)).To(Succeed())
		g.Expect(found.Status.Phase).To(Equal(corev1.PodRunning), "gateway test client pod is not running yet")
	}).Should(Succeed(), "The gateway test client pod never became ready")
	return gatewayTestClientPodName
}

// envoyProxyAddressForGateway returns the in-cluster address (clusterIP:port) of the Envoy proxy that Envoy
// Gateway provisions for the given Gateway, for the given listener port.
func envoyProxyAddressForGateway(ctx context.Context, gatewayName, gatewayNamespace string, listenerPort int32) string {
	var address string
	EventuallyWithOffset(1, func(g Gomega) {
		serviceList := &corev1.ServiceList{}
		g.Expect(k8sClient.List(ctx, serviceList,
			client.InNamespace(envoyGatewaySystemNs),
			client.MatchingLabels{
				"gateway.envoyproxy.io/owning-gateway-name":      gatewayName,
				"gateway.envoyproxy.io/owning-gateway-namespace": gatewayNamespace,
			},
		)).To(Succeed(), "Could not list Envoy proxy services")
		g.Expect(serviceList.Items).ToNot(BeEmpty(), "No Envoy proxy Service found for Gateway %s/%s", gatewayNamespace, gatewayName)

		svc := serviceList.Items[0]
		g.Expect(svc.Spec.ClusterIP).ToNot(BeElementOf("", corev1.ClusterIPNone), "Envoy proxy Service has no ClusterIP")
		var port int32
		for _, p := range svc.Spec.Ports {
			if p.Port == listenerPort {
				port = p.Port
				break
			}
		}
		g.Expect(port).ToNot(BeZero(), "Envoy proxy Service %s has no port %d", svc.Name, listenerPort)
		address = fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, port)
	}).Should(Succeed(), "Could not resolve the Envoy proxy address for Gateway %s/%s", gatewayNamespace, gatewayName)
	return address
}

// callSolrThroughGateway issues an HTTP request from the client pod to the Envoy proxy, setting the Host header so
// the Gateway routes it to the correct HTTPRoute. It returns the combined response body followed by an
// "HTTP_STATUS:<code>" line.
func callSolrThroughGateway(ctx context.Context, namespace, clientPod, proxyAddress, host, path string) (string, error) {
	url := fmt.Sprintf("http://%s%s", proxyAddress, path)
	command := []string{
		"curl", "-sS", "-m", "15",
		"-o", "-",
		"-w", fmt.Sprintf("%q", "\\nHTTP_STATUS:%{http_code}"),
		"-H", fmt.Sprintf("%q", "Host: "+host),
		fmt.Sprintf("%q", url),
	}
	return runExecForContainer(ctx, "curl", clientPod, namespace, command)
}
