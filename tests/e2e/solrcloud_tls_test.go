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
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("E2E - SolrCloud - TLS - Secrets", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection = "e2e"
	)

	/*
		Create a single SolrCloud that has TLS Enabled
	*/
	BeforeEach(func(ctx context.Context) {
		installSolrIssuer(ctx, testNamespace())
	})

	/*
		Start the SolrCloud and ensure that it is running
	*/
	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a Solr Collection to query metrics for")
		createAndQueryCollection(ctx, solrCloud, solrCollection, 1, 2)
	})

	FContext("No Client TLS", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, false)

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func() {})
	})

	FContext("No Client TLS - Just a Keystore", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, false)

			solrCloud.Spec.SolrTLS.TrustStoreSecret = nil
			solrCloud.Spec.SolrTLS.TrustStorePasswordSecret = nil

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func() {})
	})

	FContext("No Client TLS - CheckPeerName", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, false)

			solrCloud.Spec.SolrTLS.CheckPeerName = true

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"

			//solrCloud.Spec.CustomSolrKubeOptions.PodOptions.EnvVariables =
			//	append(solrCloud.Spec.CustomSolrKubeOptions.PodOptions.EnvVariables, corev1.EnvVar{
			//		Name:  "SOLR_TOOL_OPTS",
			//		Value: "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake",
			//	})
		})

		FIt("Can run", func(ctx context.Context) {
			By("Checking that using the wrong peer name does not fail")
			response, err := callSolrApiInPod(
				ctx,
				solrCloud,
				"get",
				"/solr/admin/info/health",
				nil,
				"localhost",
			)
			Expect(err).To(HaveOccurred(), "Error should have occurred while calling Solr API - Bad server hostname for TLS")
			Expect(response).To(Or(ContainSubstring("Invalid SNI"), ContainSubstring("doesn't match any of the subject alternative names"), ContainSubstring("No subject alternative DNS name matching")), "Wrong error when calling Solr - Bad hostname for TLS expected")
		})
	})

	FContext("With Client TLS - VerifyClientHostname", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, true)

			solrCloud.Spec.SolrTLS.VerifyClientHostname = true

			solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func(ctx context.Context) {
			By("Checking that using the wrong peer name does not fail")
			_, err := callSolrApiInPod(
				ctx,
				solrCloud,
				"get",
				"/solr/admin/info/health",
				nil,
				"localhost",
			)
			Expect(err).ToNot(HaveOccurred(), "Error occurred while calling Solr API - Server Hostname checking should not be on")
		})
	})

	FContext("With Client TLS - CheckPeerName", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, true)

			solrCloud.Spec.SolrTLS.CheckPeerName = true

			solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func(ctx context.Context) {
			By("Checking that using the wrong peer name fails")
			response, err := callSolrApiInPod(
				ctx,
				solrCloud,
				"get",
				"/solr/admin/info/health",
				nil,
				"localhost",
			)
			Expect(err).To(HaveOccurred(), "Error should have occurred while calling Solr API - Bad server hostname for TLS")
			Expect(response).To(Or(ContainSubstring("Invalid SNI"), ContainSubstring("doesn't match any of the subject alternative names"), ContainSubstring("No subject alternative DNS name matching")), "Wrong error when calling Solr - Bad hostname for TLS expected")
		})
	})

	FContext("With Client TLS - Client Auth Need", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, true)

			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Need

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func() {})
	})

	FContext("With Client TLS - Client Auth Want", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithSecretTLS(ctx, 2, true)

			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Want

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func() {})
	})
})

var _ = FDescribe("E2E - SolrCloud - TLS - Mounted Dir", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection = "e2e"
	)

	/*
		Create a single SolrCloud that has TLS Enabled
	*/
	BeforeEach(func(ctx context.Context) {
		installSolrIssuer(ctx, testNamespace())
	})

	/*
		Start the SolrCloud and ensure that it is running
	*/
	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a Solr Collection to query metrics for")
		createAndQueryCollection(ctx, solrCloud, solrCollection, 1, 2)
	})

	FContext("ClientAuth - Want", func() {

		BeforeEach(func(ctx context.Context) {
			solrCloud = generateBaseSolrCloudWithCSITLS(1, false, false)

			solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Want

			//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
		})

		FIt("Can run", func() {})
	})

	//FContext("ClientAuth - Need", func() {
	//
	//	BeforeEach(func(ctx context.Context) {
	//		solrCloud = generateBaseSolrCloudWithCSITLS(1, false, true)
	//
	//		solrCloud.Spec.SolrTLS.ClientAuth = solrv1beta1.Need
	//
	//		//solrCloud.Spec.SolrOpts = "-Djavax.net.debug=SSL,keymanager,trustmanager,ssl:handshake"
	//	})
	//
	//	FIt("Can run", func() {})
	//})
})
