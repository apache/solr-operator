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
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/zk_api"
	"github.com/apache/solr-operator/version"
	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// Available environment variables to customize tests
	operatorImageEnv = "OPERATOR_IMAGE"
	solrImageEnv     = "SOLR_IMAGE"

	backupDirHostPath = "/tmp/backup"
)

var (
	solrOperatorRelease *release.Release
	k8sClient           client.Client
	rawK8sClient        *kubernetes.Clientset
	k8sConfig           *rest.Config
	logger              logr.Logger

	defaultOperatorImage = "apache/solr-operator:" + version.Version
	defaultSolrImage     = "solr:8.11"
)

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting Solr Operator E2E suite\n")
	RunSpecs(t, "Solr Operator e2e suite")
}

var _ = SynchronizedBeforeSuite(func() {
	// Run this once before all tests, not per-test-process
	By("starting the test solr operator")
	solrOperatorRelease = runSolrOperator()
	Expect(solrOperatorRelease).ToNot(BeNil())
}, func(ctx context.Context) {
	// Run these in each parallel test process before the tests
	rand.Seed(GinkgoRandomSeed() + int64(GinkgoParallelProcess()))

	// Define testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 180
		duration = time.Millisecond * 500
		interval = time.Millisecond * 250
	)
	SetDefaultConsistentlyDuration(duration)
	SetDefaultConsistentlyPollingInterval(interval)
	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(logger)

	By("setting up the k8s clients")
	Expect(solrv1beta1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(zk_api.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	var err error
	k8sConfig, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Could not load in default kubernetes config")
	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred(), "Could not create controllerRuntime Kubernetes client")

	rawK8sClient, err = kubernetes.NewForConfig(k8sConfig)
	Expect(err).NotTo(HaveOccurred(), "Could not create raw Kubernetes client")

	By("creating a namespace for this parallel test process")
	_, err = rawK8sClient.CoreV1().Namespaces().Create(
		ctx,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace(),
			},
		},
		metav1.CreateOptions{},
	)
	Expect(err).To(Not(HaveOccurred()), "Failed to create testing namespace %s", testNamespace())
})

var _ = SynchronizedAfterSuite(func(ctx context.Context) {
	// Run these in each parallel test process after the tests
	By("deleting the namespace for this parallel test process")
	deletePolicy := metav1.DeletePropagationForeground
	Expect(rawK8sClient.CoreV1().Namespaces().Delete(ctx, testNamespace(), metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})).To(Succeed(), "Failed to delete testing namespace %s", testNamespace())
}, func() {
	// Run this once after all tests, not per-test-process
	if solrOperatorRelease != nil {
		By("tearing down the test solr operator")
		stopSolrOperator(solrOperatorRelease)
	}
})
