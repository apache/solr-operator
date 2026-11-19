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
	"github.com/apache/solr-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	FIt("does not keep ownership of the fields it defaults", func(ctx context.Context) {
		By("applying the SolrCloud as another field manager")
		applyAsHelm(ctx, applied)

		By("waiting for the operator to default the SolrCloud and release the defaulted fields")
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Spec.SolrImage).ToNot(BeNil(), "The SolrCloud should be defaulted")
			g.Expect(found.Spec.SolrImage.Tag).To(Equal(solrv1beta1.DefaultSolrVersion))
			expectNoOperatorFieldOwnership(g, found)
		})

		By("applying a different value for a defaulted field, without forcing conflicts")
		Expect(unstructured.SetNestedField(applied.Object, "9.10.0", "spec", "solrImage", "tag")).To(Succeed())
		applyAsHelm(ctx, applied)
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Spec.SolrImage.Tag).To(Equal("9.10.0"))
		})
	})

	FIt("does not keep ownership of the storage finalizer", func(ctx context.Context) {
		By("applying a SolrCloud whose PVCs are deleted with it")
		Expect(unstructured.SetNestedField(applied.Object, "5Gi", "spec", "dataStorage", "persistent", "pvcTemplate", "spec", "resources", "requests", "storage")).To(Succeed())
		Expect(unstructured.SetNestedField(applied.Object, "Delete", "spec", "dataStorage", "persistent", "reclaimPolicy")).To(Succeed())
		applyAsHelm(ctx, applied)
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Finalizers).To(ContainElement(util.SolrStorageFinalizer))
			expectNoOperatorFieldOwnership(g, found)
		})

		By("keeping the PVCs instead, which removes the finalizer")
		Expect(unstructured.SetNestedField(applied.Object, "Retain", "spec", "dataStorage", "persistent", "reclaimPolicy")).To(Succeed())
		applyAsHelm(ctx, applied)
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.Finalizers).ToNot(ContainElement(util.SolrStorageFinalizer))
			expectNoOperatorFieldOwnership(g, found)
		})
	})
})

var _ = FDescribe("SolrPrometheusExporter controller - Field ownership", func() {
	var exporter *solrv1beta1.SolrPrometheusExporter

	BeforeEach(func() {
		exporter = &solrv1beta1.SolrPrometheusExporter{}
		exporter.Name = "foo"
		exporter.Namespace = "default"
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, exporter)
	})

	FIt("does not keep ownership of the fields it defaults", func(ctx context.Context) {
		applied := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": solrv1beta1.GroupVersion.String(),
			"kind":       "SolrPrometheusExporter",
			"metadata": map[string]interface{}{
				"name":      exporter.Name,
				"namespace": exporter.Namespace,
			},
			"spec": map[string]interface{}{
				"solrReference": map[string]interface{}{
					"standalone": map[string]interface{}{
						"address": "http://solr:8983/solr",
					},
				},
			},
		}}

		By("applying the exporter as another field manager")
		applyAsHelm(ctx, applied)

		By("waiting for the operator to default the exporter and release the defaulted fields")
		expectSolrPrometheusExporterWithChecks(ctx, exporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.Spec.Image).ToNot(BeNil(), "The exporter should be defaulted")
			g.Expect(found.Spec.Image.Tag).To(Equal(solrv1beta1.DefaultSolrVersion))
			expectNoOperatorFieldOwnership(g, found)
		})

		By("applying a different value for a defaulted field, without forcing conflicts")
		Expect(unstructured.SetNestedField(applied.Object, "9.10.0", "spec", "image", "tag")).To(Succeed())
		applyAsHelm(ctx, applied)
		expectSolrPrometheusExporterWithChecks(ctx, exporter, func(g Gomega, found *solrv1beta1.SolrPrometheusExporter) {
			g.Expect(found.Spec.Image.Tag).To(Equal("9.10.0"))
		})
	})
})

func applyAsHelm(ctx context.Context, obj *unstructured.Unstructured) {
	ExpectWithOffset(1, k8sClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj.DeepCopy()), client.FieldOwner("helm"))).To(Succeed())
}

func expectNoOperatorFieldOwnership(g Gomega, obj metav1.Object) {
	for _, entry := range obj.GetManagedFields() {
		if entry.Manager == fieldManager {
			g.Expect(entry.Subresource).To(Equal("status"), "The operator should not own any spec or metadata fields")
		}
	}
}
