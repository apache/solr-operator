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
	"encoding/json"
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
	appsv1 "k8s.io/api/apps/v1"
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

var _ = JustAfterEach(func(ctx context.Context) {
	testOutputDir := outputDirForTest(CurrentSpecReport().FullText())

	// We count "ran" as "passed" or "failed"
	if CurrentSpecReport().State.Is(types.SpecStatePassed | types.SpecStateFailureStates) {
		Expect(os.Mkdir(testOutputDir, os.ModeDir|os.ModePerm)).To(Succeed(), "Could not create directory for test output: %s", testOutputDir)
		testOutputDir += "/"
		// Always save the logs of the Solr Operator for the test
		startTime := CurrentSpecReport().StartTime
		writePodLogsToFile(
			ctx,
			testOutputDir+"solr-operator.log",
			getSolrOperatorPodName(ctx, solrOperatorReleaseNamespace),
			solrOperatorReleaseNamespace,
			&startTime,
			fmt.Sprintf("%q: %q", "namespace", testNamespace()),
		)
		// Always save the logs of the Solr Operator for the test
		writeAllSolrInfoToFiles(
			ctx,
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

func getSolrOperatorPodName(ctx context.Context, namespace string) string {
	labelSelector := labels.SelectorFromSet(map[string]string{"control-plane": "solr-operator"})
	listOps := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		Limit:         int64(1),
	}

	foundPods := &corev1.PodList{}
	Expect(k8sClient.List(ctx, foundPods, listOps)).To(Succeed(), "Could not fetch Solr Operator pod")
	Expect(foundPods).ToNot(BeNil(), "No Solr Operator pods could be found")
	Expect(foundPods.Items).ToNot(BeEmpty(), "No Solr Operator pods could be found")
	return foundPods.Items[0].Name
}

func writeAllSolrInfoToFiles(ctx context.Context, directory string, namespace string) {
	req, err := labels.NewRequirement("technology", selection.In, []string{solrv1beta1.SolrTechnologyLabel, solrv1beta1.SolrPrometheusExporterTechnologyLabel})
	Expect(err).ToNot(HaveOccurred())

	labelSelector := labels.Everything().Add(*req)
	listOps := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	foundPods := &corev1.PodList{}
	Expect(k8sClient.List(ctx, foundPods, listOps)).To(Succeed(), "Could not fetch Solr pods")
	Expect(foundPods).ToNot(BeNil(), "No Solr pods could be found")
	for _, pod := range foundPods.Items {
		writeAllPodInfoToFiles(
			ctx,
			directory+pod.Name,
			&pod,
		)
	}

	foundStatefulSets := &appsv1.StatefulSetList{}
	Expect(k8sClient.List(ctx, foundStatefulSets, listOps)).To(Succeed(), "Could not fetch Solr statefulSets")
	Expect(foundStatefulSets).ToNot(BeNil(), "No Solr statefulSet could be found")
	for _, statefulSet := range foundStatefulSets.Items {
		writeAllStatefulSetInfoToFiles(
			directory+statefulSet.Name+".statefulSet",
			&statefulSet,
		)
	}

	// Unfortunately the services don't have the technology label
	req, err = labels.NewRequirement("solr-cloud", selection.Exists, make([]string, 0))
	Expect(err).ToNot(HaveOccurred())

	labelSelector = labels.Everything().Add(*req)
	listOps = &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	foundServices := &corev1.ServiceList{}
	Expect(k8sClient.List(ctx, foundServices, listOps)).To(Succeed(), "Could not fetch Solr pods")
	Expect(foundServices).ToNot(BeNil(), "No Solr services could be found")
	for _, service := range foundServices.Items {
		writeAllServiceInfoToFiles(
			directory+service.Name+".service",
			&service,
		)
	}
}

// writeAllStatefulSetInfoToFiles writes the following each to a separate file with the given base name & directory.
//   - StatefulSet Spec/Status
//   - StatefulSet Events
func writeAllStatefulSetInfoToFiles(baseFilename string, statefulSet *appsv1.StatefulSet) {
	// Write statefulSet to a file
	statusFile, err := os.Create(baseFilename + ".status.json")
	defer statusFile.Close()
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save statefulSet status: %s", baseFilename+".status.json")
	jsonBytes, marshErr := json.MarshalIndent(statefulSet, "", "\t")
	Expect(marshErr).ToNot(HaveOccurred(), "Could not serialize statefulSet json")
	_, writeErr := statusFile.Write(jsonBytes)
	Expect(writeErr).ToNot(HaveOccurred(), "Could not write statefulSet json to file")

	// Write events for statefulSet to a file
	eventsFile, err := os.Create(baseFilename + ".events.json")
	defer eventsFile.Close()
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save statefulSet events: %s", baseFilename+".events.yaml")

	eventList, err := rawK8sClient.CoreV1().Events(statefulSet.Namespace).Search(scheme.Scheme, statefulSet)
	Expect(err).ToNot(HaveOccurred(), "Could not find events for statefulSet: %s", statefulSet.Name)
	jsonBytes, marshErr = json.MarshalIndent(eventList, "", "\t")
	Expect(marshErr).ToNot(HaveOccurred(), "Could not serialize statefulSet events json")
	_, writeErr = eventsFile.Write(jsonBytes)
	Expect(writeErr).ToNot(HaveOccurred(), "Could not write statefulSet events json to file")
}

// writeAllServiceInfoToFiles writes the following each to a separate file with the given base name & directory.
//   - Service
func writeAllServiceInfoToFiles(baseFilename string, service *corev1.Service) {
	// Write service to a file
	statusFile, err := os.Create(baseFilename + ".json")
	defer statusFile.Close()
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save service status: %s", baseFilename+".json")
	jsonBytes, marshErr := json.MarshalIndent(service, "", "\t")
	Expect(marshErr).ToNot(HaveOccurred(), "Could not serialize service json")
	_, writeErr := statusFile.Write(jsonBytes)
	Expect(writeErr).ToNot(HaveOccurred(), "Could not write service json to file")
}

// writeAllPodInfoToFile writes the following each to a separate file with the given base name & directory.
//   - Pod Spec/Status
//   - Pod Events
//   - Pod logs
func writeAllPodInfoToFiles(ctx context.Context, baseFilename string, pod *corev1.Pod) {
	// Write pod to a file
	statusFile, err := os.Create(baseFilename + ".status.json")
	defer statusFile.Close()
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save pod status: %s", baseFilename+".status.json")
	jsonBytes, marshErr := json.MarshalIndent(pod, "", "\t")
	Expect(marshErr).ToNot(HaveOccurred(), "Could not serialize pod json")
	_, writeErr := statusFile.Write(jsonBytes)
	Expect(writeErr).ToNot(HaveOccurred(), "Could not write pod json to file")

	// Write events for pod to a file
	eventsFile, err := os.Create(baseFilename + ".events.json")
	defer eventsFile.Close()
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save pod events: %s", baseFilename+".events.yaml")

	eventList, err := rawK8sClient.CoreV1().Events(pod.Namespace).Search(scheme.Scheme, pod)
	Expect(err).ToNot(HaveOccurred(), "Could not find events for pod: %s", pod.Name)
	jsonBytes, marshErr = json.MarshalIndent(eventList, "", "\t")
	Expect(marshErr).ToNot(HaveOccurred(), "Could not serialize pod events json")
	_, writeErr = eventsFile.Write(jsonBytes)
	Expect(writeErr).ToNot(HaveOccurred(), "Could not write pod events json to file")

	// Write pod logs to a file
	if len(pod.Status.ContainerStatuses) > 0 && pod.Status.ContainerStatuses[0].Started != nil && *pod.Status.ContainerStatuses[0].Started {
		writePodLogsToFile(
			ctx,
			baseFilename+".log",
			pod.Name,
			pod.Namespace,
			nil,
			"",
		)
	}
}

func writePodLogsToFile(ctx context.Context, filename string, podName string, podNamespace string, startTimeRaw *time.Time, filterLinesWithString string) {
	logFile, err := os.Create(filename)
	Expect(err).ToNot(HaveOccurred(), "Could not open file to save logs: %s", filename)
	defer logFile.Close()

	podLogOpts := corev1.PodLogOptions{}
	if startTimeRaw != nil {
		startTime := metav1.NewTime(*startTimeRaw)
		podLogOpts.SinceTime = &startTime
	}

	req := rawK8sClient.CoreV1().Pods(podNamespace).GetLogs(podName, &podLogOpts)
	podLogs, logsErr := req.Stream(ctx)
	Expect(logsErr).ToNot(HaveOccurred(), "Could not open stream to fetch pod logs. namespace: %s, pod: %s", podNamespace, podName)
	defer podLogs.Close()

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
