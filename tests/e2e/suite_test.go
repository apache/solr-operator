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
	"github.com/apache/solr-operator/version"
	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2/types"
	zkApi "github.com/pravega/zookeeper-operator/api/v1beta1"
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

	defaultOperatorImage = "apache/solr-operator:" + version.FullVersion()
	defaultSolrImage     = "solr:8.11"

	operatorImage = getEnvWithDefault(operatorImageEnv, defaultOperatorImage)
	solrImage     = getEnvWithDefault(solrImageEnv, defaultSolrImage)
)

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting Solr Operator E2E suite\n")
	RunSpecs(t, "Solr Operator e2e suite")
}

var _ = SynchronizedBeforeSuite(func(ctx context.Context) {
	// Run this once before all tests, not per-test-process
	By("starting the test solr operator")
	solrOperatorRelease = runSolrOperator(ctx)
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

	var err error
	k8sConfig, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Could not load in default kubernetes config")

	rawK8sClient, err = kubernetes.NewForConfig(k8sConfig)
	Expect(err).NotTo(HaveOccurred(), "Could not create raw Kubernetes client")

	By("setting up the k8s clients")
	Expect(solrv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(zkApi.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred(), "Could not create controllerRuntime Kubernetes client")

	// Delete the testing namespace if it already exists, then recreate it below
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace()}}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace()}, namespace)
	if err == nil {
		By("deleting the existing namespace for this parallel test process before recreating it")
		deleteAndWait(ctx, namespace)
	}

	By("creating a namespace for this parallel test process")
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace()}})).
		To(Succeed(), "Failed to create testing namespace %s", testNamespace())
})

var _ = SynchronizedAfterSuite(func(ctx context.Context) {
	// Run these in each parallel test process after the tests
	By("deleting the namespace for this parallel test process")
	Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace()}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).
		To(Or(Succeed(), MatchError(HaveSuffix("%q not found", testNamespace()))), "Failed to delete testing namespace %s", testNamespace())
}, func() {
	// Run this once after all tests, not per-test-process
	if solrOperatorRelease != nil {
		By("tearing down the test solr operator")
		stopSolrOperator(solrOperatorRelease)
	}
})

type RetryCommand struct {
	report        SpecReport
	parallelism   int
	randomSeed    int64
	operatorImage string
	solrImage     string
}

// ColorableString for ReportEntry to use
func (rc RetryCommand) ColorableString() string {
	return fmt.Sprintf("{{orange}}%s{{/}}", rc)
}

// non-colorable String() is used by go's string formatting support but ignored by ReportEntry
func (rc RetryCommand) String() string {
	return fmt.Sprintf(
		"make e2e-tests TEST_FILES=%q TEST_FILTER=%q TEST_SEED=%d TEST_PARALLELISM=%d %s=%q %s=%q",
		rc.report.FileName(),
		rc.report.FullText(),
		rc.randomSeed,
		rc.parallelism,
		solrImageEnv, rc.solrImage,
		operatorImageEnv, rc.operatorImage,
	)
}

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		ginkgoConfig, _ := GinkgoConfiguration()
		AddReportEntry(
			"Re-Run Failed Test Using Command",
			types.CodeLocation{},
			RetryCommand{
				report:        report,
				parallelism:   ginkgoConfig.ParallelTotal,
				randomSeed:    GinkgoRandomSeed(),
				operatorImage: operatorImage,
				solrImage:     solrImage,
			},
		)
	}
})
