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

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helm 4 applies server-side, so it fails on any field the operator has taken ownership of
var _ = FDescribe("SolrCloud controller - Field ownership", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud
		applied   *unstructured.Unstructured
	)

	BeforeEach(func() {
		solrCloud = &solrv1beta1.SolrCloud{}
		solrCloud.Name = "foo"
		solrCloud.Namespace = "default"

		applied = &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": solrv1beta1.GroupVersion.String(),
			"kind":       "SolrCloud",
			"metadata": map[string]interface{}{
				"name":      solrCloud.Name,
				"namespace": solrCloud.Namespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"zookeeperRef": map[string]interface{}{
					"connectionInfo": map[string]interface{}{
						"internalConnectionString": "host:7271",
					},
				},
				"customSolrKubeOptions": map[string]interface{}{
					"podOptions": map[string]interface{}{
						"initContainers": []interface{}{
							map[string]interface{}{
								"name":    "custom-init",
								"image":   "busybox:1.37",
								"command": []interface{}{"true"},
							},
						},
					},
				},
			},
		}}
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FIt("does not take ownership of fields set by a server-side apply", func(ctx context.Context) {
		By("applying the SolrCloud as another field manager")
		Expect(k8sClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(applied.DeepCopy()), client.FieldOwner("helm"))).To(Succeed())

		By("waiting for the operator to default the SolrCloud")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.WithDefaults(logger)).To(BeFalse(), "The SolrCloud spec should not need to be defaulted eventually")
		})

		By("applying the same SolrCloud again, without forcing conflicts")
		Expect(k8sClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(applied.DeepCopy()), client.FieldOwner("helm"))).To(Succeed())
	})
})
