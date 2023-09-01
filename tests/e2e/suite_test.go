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
	"bufio"
	"bytes"
	"context"
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/version"
	certManagerApi "github.com/cert-manager/cert-manager/pkg/api"
	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2/types"
	zkApi "github.com/pravega/zookeeper-operator/api/v1beta1"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"math/rand"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// Available environment variables to customize tests
	operatorImageEnv = "OPERATOR_IMAGE"
	solrImageEnv     = "SOLR_IMAGE"
	kubeVersionEnv   = "KUBERNETES_VERSION"

	backupDirHostPath = "/tmp/backup"

	// Shared testing specs
	timeout  = time.Second * 180
	duration = time.Millisecond * 500
	interval = time.Millisecond * 250
)

var (
	k8sClient    client.Client
	rawK8sClient *kubernetes.Clientset
	k8sConfig    *rest.Config
	logger       logr.Logger

	defaultOperatorImage = "apache/solr-operator:" + version.FullVersion()
	defaultSolrImage     = "solr:8.11"

	operatorImage = getEnvWithDefault(operatorImageEnv, defaultOperatorImage)
	solrImage     = getEnvWithDefault(solrImageEnv, defaultSolrImage)
	kubeVersion   = getEnvWithDefault(kubeVersionEnv, defaultSolrImage)

	outputDir = "output"
)

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting Solr Operator E2E suite\n")
	RunSpecs(t, "Solr Operator e2e suite")
}

var _ = SynchronizedBeforeSuite(func(ctx context.Context) {
	// Define testing timeouts/durations and intervals.
	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(logger)

	Expect(os.RemoveAll(outputDir+"/")).To(Succeed(), "Could not delete existing output directory before tests start: %s", outputDir)
	Expect(os.Mkdir(outputDir, os.ModeDir|os.ModePerm)).To(Succeed(), "Could not create directory for test output: %s", outputDir)

	var err error
	k8sConfig, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Could not load in default kubernetes config")
	Expect(zkApi.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(certManagerApi.AddToScheme(scheme.Scheme)).To(Succeed())
	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred(), "Could not create controllerRuntime Kubernetes client")

	// Set up a shared Zookeeper Cluster to be used for most SolrClouds
	// This will significantly speed up tests.
	// The zookeeper will not be healthy until after the zookeeper operator is released with the solr operator
	By("creating a shared zookeeper cluster")
	zookeeper := runSharedZookeeperCluster(ctx)

	// Set up a shared Bootstrap issuer for creating CAs for Solr
	By("creating a boostrap cert issuer")
	installBootstrapIssuer(ctx)

	// Run this once before all tests, not per-test-process
	By("starting the test solr operator")
	solrOperatorRelease := runSolrOperator(ctx)
	Expect(solrOperatorRelease).ToNot(BeNil())

	By("Waiting for the Zookeeper to come up healthy")
	// This check must be done after the solr operator is installed
	waitForSharedZookeeperCluster(ctx, zookeeper)
}, func(ctx context.Context) {
	// Run these in each parallel test process before the tests
	rand.Seed(GinkgoRandomSeed() + int64(GinkgoParallelProcess()))

	// Define testing timeouts/durations and intervals.
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
	Expect(certManagerApi.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred(), "Could not create controllerRuntime Kubernetes client")

	By("creating a namespace for this parallel test process")
	createOrRecreateNamespace(ctx, testNamespace())
})

type RetryCommand struct {
	report        SpecReport
	parallelism   int
	randomSeed    int64
	operatorImage string
	solrImage     string
	kubeVersion   string
}

// ColorableString for ReportEntry to use
func (rc RetryCommand) ColorableString() string {
	return fmt.Sprintf("{{orange}}%s{{/}}", rc)
}

// non-colorable String() is used by go's string formatting support but ignored by ReportEntry
func (rc RetryCommand) String() string {
	return fmt.Sprintf(
		"make e2e-tests TEST_FILES=%q TEST_FILTER=%q TEST_SEED=%d TEST_PARALLELISM=%d %s=%q %s=%q %s=%q",
		rc.report.FileName(),
		rc.report.FullText(),
		rc.randomSeed,
		rc.parallelism,
		solrImageEnv, rc.solrImage,
		operatorImageEnv, rc.operatorImage,
		kubeVersionEnv, rc.kubeVersion,
	)
}

type FailureInformation struct {
	namespace       string
	outputDirectory string
}

// ColorableString for ReportEntry to use
func (fi FailureInformation) ColorableString() string {
	return fmt.Sprintf("{{yellow}}%s{{/}}", fi)
}

// non-colorable String() is used by go's string formatting support but ignored by ReportEntry
func (fi FailureInformation) String() string {
	return fmt.Sprintf(
		"Namespace: %s\nLogs Directory: %s\n",
		fi.namespace,
		fi.outputDirectory,
	)
}

func outputDirForTest(testText string) string {
	testName := cases.Title(language.AmericanEnglish, cases.NoLower).String(testText)
	return outputDir + "/" + strings.ReplaceAll(strings.ReplaceAll(testName, "  ", "-"), " ", "")
}

var _ = JustAfterEach(func() {
	testOutputDir := outputDirForTest(CurrentSpecReport().FullText())

	// We count "ran" as "passed" or "failed"
	if CurrentSpecReport().State.Is(types.SpecStatePassed | types.SpecStateFailureStates) {
		Expect(os.Mkdir(testOutputDir, os.ModeDir|os.ModePerm)).To(Succeed(), "Could not create directory for test output: %s", testOutputDir)
		testOutputDir += "/"
		// Always save the logs of the Solr Operator for the test
		startTime := CurrentSpecReport().StartTime
		writePodLogsToFile(
			testOutputDir+"solr-operator.log",
			getSolrOperatorPodName(solrOperatorReleaseNamespace),
			solrOperatorReleaseNamespace,
			&startTime,
			fmt.Sprintf("%q: %q", "namespace", testNamespace()),
		)
		// Always save the logs of the Solr Operator for the test
		writeAllSolrLogsToFiles(
			testOutputDir,
			testNamespace(),
		)
	}
})

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		ginkgoConfig, _ := GinkgoConfiguration()
		testOutputDir, _ := filepath.Abs(outputDirForTest(report.FullText()))
		AddReportEntry(
			"Failure Information",
			types.CodeLocation{},
			FailureInformation{
				namespace:       testNamespace(),
				outputDirectory: testOutputDir,
			},
		)
		AddReportEntry(
			"Re-Run Failed Test Using Command",
			types.CodeLocation{},
			RetryCommand{
				report:        report,
				parallelism:   ginkgoConfig.ParallelTotal,
				randomSeed:    GinkgoRandomSeed(),
				operatorImage: operatorImage,
				solrImage:     solrImage,
				kubeVersion:   kubeVersion,
			},
		)
	}
})

func getSolrOperatorPodName(namespace string) string {
	labelSelector := labels.SelectorFromSet(map[string]string{"control-plane": "solr-operator"})
	listOps := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		Limit:         int64(1),
	}

	foundPods := &corev1.PodList{}
	Expect(k8sClient.List(context.TODO(), foundPods, listOps)).To(Succeed(), "Could not fetch Solr Operator pod")
	Expect(foundPods).ToNot(BeNil(), "No Solr Operator pods could be found")
	Expect(foundPods.Items).ToNot(BeEmpty(), "No Solr Operator pods could be found")
	return foundPods.Items[0].Name
}

func writeAllSolrLogsToFiles(directory string, namespace string) {
	req, err := labels.NewRequirement("technology", selection.In, []string{solrv1beta1.SolrTechnologyLabel, solrv1beta1.SolrPrometheusExporterTechnologyLabel})
	Expect(err).ToNot(HaveOccurred())

	labelSelector := labels.Everything().Add(*req)
	listOps := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	foundPods := &corev1.PodList{}
	Expect(k8sClient.List(context.TODO(), foundPods, listOps)).To(Succeed(), "Could not fetch Solr Operator pod")
	Expect(foundPods).ToNot(BeNil(), "No Solr pods could be found")
	for _, pod := range foundPods.Items {
		writePodLogsToFile(
			directory+pod.Name+".log",
			pod.Name,
			namespace,
			nil,
			"",
		)
	}
}

func writePodLogsToFile(filename string, podName string, podNamespace string, startTimeRaw *time.Time, filterLinesWithString string) {
	logFile, err := os.Create(filename)
	defer logFile.Close()
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save logs: %s", filename)

	podLogOpts := corev1.PodLogOptions{}
	if startTimeRaw != nil {
		startTime := metav1.NewTime(*startTimeRaw)
		podLogOpts.SinceTime = &startTime
	}

	req := rawK8sClient.CoreV1().Pods(podNamespace).GetLogs(podName, &podLogOpts)
	podLogs, logsErr := req.Stream(context.Background())
	defer podLogs.Close()
	Expect(logsErr).ToNot(HaveOccurred(), "Could not open stream to fetch pod logs. namespace: %s, pod: %s", podNamespace, podName)

	var logReader io.Reader
	logReader = podLogs

	if filterLinesWithString != "" {
		filteredWriter := bytes.NewBufferString("")
		scanner := bufio.NewScanner(podLogs)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, filterLinesWithString) {
				io.WriteString(filteredWriter, line)
				io.WriteString(filteredWriter, "\n")
			}
		}
		logReader = filteredWriter
	}

	_, err = io.Copy(logFile, logReader)
	Expect(err).ToNot(HaveOccurred(), "Could not write podLogs to file: %s", filename)
}
