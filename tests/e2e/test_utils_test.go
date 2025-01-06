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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	zkApi "github.com/pravega/zookeeper-operator/api/v1beta1"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	helmDriver = "configmap"

	solrOperatorReleaseName      = "solr-operator"
	solrOperatorReleaseNamespace = "solr-operator"

	sharedZookeeperName             = "shared"
	sharedZookeeperNamespace        = "zk"
	sharedZookeeperConnectionString = sharedZookeeperName + "-client." + sharedZookeeperNamespace + ":2181"
)

var (
	settings = cli.New()
)

func testNamespace() string {
	return fmt.Sprintf("solr-e2e-%d", GinkgoParallelProcess())
}

// Run Solr Operator for e2e testing of resources
func runSolrOperator(ctx context.Context) *release.Release {
	actionConfig := new(action.Configuration)
	Expect(actionConfig.Init(settings.RESTClientGetter(), "solr-operator", helmDriver, GinkgoLogr.Info)).To(Succeed(), "Failed to create helm configuration")

	operatorRepo, operatorTag, found := strings.Cut(operatorImage, ":")
	Expect(found).To(BeTrue(), "Invalid Operator image found in envVar OPERATOR_IMAGE: "+operatorImage)
	operatorValues := map[string]interface{}{
		"image": map[string]interface{}{
			"repository": operatorRepo,
			"tag":        operatorTag,
			"pullPolicy": "Never",
		},
	}

	chart, err := loader.Load("../../helm/solr-operator")
	Expect(err).ToNot(HaveOccurred(), "Failed to load solr-operator Helm chart")

	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	var solrOperatorHelmRelease *release.Release
	if _, err = histClient.Run(solrOperatorReleaseName); errors.Is(err, driver.ErrReleaseNotFound) {
		installClient := action.NewInstall(actionConfig)

		installClient.ReleaseName = solrOperatorReleaseName
		installClient.Namespace = solrOperatorReleaseNamespace
		installClient.SkipCRDs = true
		installClient.CreateNamespace = true

		solrOperatorHelmRelease, err = installClient.RunWithContext(ctx, chart, operatorValues)
	} else {
		upgradeClient := action.NewUpgrade(actionConfig)

		upgradeClient.Namespace = solrOperatorReleaseNamespace
		upgradeClient.Install = true
		upgradeClient.SkipCRDs = true

		solrOperatorHelmRelease, err = upgradeClient.RunWithContext(ctx, solrOperatorReleaseName, chart, operatorValues)
	}
	Expect(err).ToNot(HaveOccurred(), "Failed to install solr-operator via Helm chart")
	Expect(solrOperatorHelmRelease).ToNot(BeNil(), "Failed to install solr-operator via Helm chart")

	DeferCleanup(func(ctx context.Context) {
		By("tearing down the test solr operator")
		stopSolrOperator(solrOperatorHelmRelease)
	})

	return solrOperatorHelmRelease
}

// Run Solr Operator for e2e testing of resources
func stopSolrOperator(release *release.Release) {
	actionConfig := new(action.Configuration)
	Expect(actionConfig.Init(settings.RESTClientGetter(), "solr-operator", helmDriver, GinkgoLogr.Info)).To(Succeed(), "Failed to create helm configuration")

	uninstallClient := action.NewUninstall(actionConfig)

	_, err := uninstallClient.Run(release.Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to uninstall solr-operator release: "+release.Name)
}

// Run a Zookeeper Cluster to be used for most of the SolrClouds across the e2e tests.
// This will speed up the tests considerably, as each SolrCloud does not need to wait on Zookeeper to become available.
func runSharedZookeeperCluster(ctx context.Context) *zkApi.ZookeeperCluster {
	createOrRecreateNamespace(ctx, sharedZookeeperNamespace)

	zookeeper := &zkApi.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sharedZookeeperName,
			Namespace: sharedZookeeperNamespace,
		},
		Spec: zkApi.ZookeeperClusterSpec{
			Replicas:    one,
			StorageType: "Ephemeral",
		},
	}

	Expect(k8sClient.Create(ctx, zookeeper)).To(Succeed(), "Failed to create shared Zookeeper Cluster")

	DeferCleanup(func(ctx context.Context) {
		stopSharedZookeeperCluster(ctx, zookeeper)
	})

	return zookeeper
}

// Check if the sharedZookeeper Cluster is running
func waitForSharedZookeeperCluster(ctx context.Context, sharedZk *zkApi.ZookeeperCluster) *zkApi.ZookeeperCluster {
	sharedZk = expectZookeeperClusterWithChecks(ctx, sharedZk, sharedZk.Name, func(g Gomega, found *zkApi.ZookeeperCluster) {
		g.Expect(found.Status.ReadyReplicas).To(Equal(found.Spec.Replicas), "The ZookeeperCluster should have all nodes come up healthy")
	}, 1)

	// Cleanup here, because we want to delete the zk before deleting the Zookeeper Operator.
	// DeferCleanup() is a stack, so this cleanup needs to be called after the solr operator DeferCleanup().
	DeferCleanup(func(ctx context.Context) {
		stopSharedZookeeperCluster(ctx, sharedZk)
	})

	return sharedZk
}

// Stop the sharedZookeeperCluster
func stopSharedZookeeperCluster(ctx context.Context, sharedZk *zkApi.ZookeeperCluster) {
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: sharedZk.Namespace, Name: sharedZk.Name}, sharedZk)
	if err == nil {
		By("tearing down the shared Zookeeper Cluster")
		Expect(k8sClient.Delete(ctx, sharedZk)).To(Succeed(), "Failed to delete shared Zookeeper Cluster")
	}
}

func getEnvWithDefault(envVar string, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		value = defaultValue
	}
	return value
}

// Delete the given namespace if it already exists, then create/recreate it
func createOrRecreateNamespace(ctx context.Context, namespaceName string) {
	namespace := &corev1.Namespace{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace)
	if err == nil {
		By("deleting the existing namespace " + namespaceName)
		deleteAndWait(ctx, namespace)
	}

	namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed(), "Failed to create namespace %s", namespaceName)

	DeferCleanup(func(ctx context.Context) {
		By("tearing down the testingNamespace " + namespaceName)
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			To(Or(Succeed(), MatchError(HaveSuffix("%q not found", namespaceName))), "Failed to delete testing namespace %s", namespaceName)
	})
}

func createAndQueryCollection(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, shards int, replicasPerShard int, nodes ...int) {
	createAndQueryCollectionWithGomega(ctx, solrCloud, collection, shards, replicasPerShard, Default, 1, nodes...)
}

func createAndQueryCollectionWithGomega(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, shards int, replicasPerShard int, g Gomega, additionalOffset int, nodes ...int) {
	asyncId := fmt.Sprintf("create-collection-%s-%d-%d", collection, shards, replicasPerShard)

	createParams := map[string]string{
		"action":            "CREATE",
		"name":              collection,
		"replicationFactor": strconv.Itoa(replicasPerShard),
		"numShards":         strconv.Itoa(shards),
		"maxShardsPerNode":  "10",
		"async":             asyncId,
		"wt":                "json",
	}

	if len(nodes) > 0 {
		var nodeSet []string
		for _, node := range nodes {
			nodeSet = append(nodeSet, util.SolrNodeName(solrCloud, solrCloud.GetSolrPodName(node)))
		}
		createParams["createNodeSet"] = strings.Join(nodeSet, ",")
	}

	additionalOffset += 1
	g.EventuallyWithOffset(additionalOffset, func(innerG Gomega) {
		response, err := callSolrApiInPod(
			ctx,
			solrCloud,
			"get",
			"/solr/admin/collections",
			createParams,
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while starting async command to create Solr Collection")
		innerG.Expect(response).To(ContainSubstring("\"status\":0"), "Error occurred while starting async command to create Solr Collection")
	}).Within(time.Second*10).WithContext(ctx).Should(Succeed(), "Collection creation command start was not successful")
	// Only wait 5 seconds when trying to create the asyncCommand

	g.EventuallyWithOffset(additionalOffset, func(innerG Gomega) {
		response, err := callSolrApiInPod(
			ctx,
			solrCloud,
			"get",
			"/solr/admin/collections",
			map[string]string{
				"action":    "REQUESTSTATUS",
				"requestid": asyncId,
				"wt":        "json",
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while checking if Solr Collection creation command was successful")
		if strings.Contains(response, "failed") || strings.Contains(response, "notfound") {
			StopTrying("A failure occurred while creating the Solr Collection").
				Attach("Collection", collection).
				Attach("Shards", shards).
				Attach("ReplicasPerShard", replicasPerShard).
				Attach("Response", response).
				Now()
		}
		innerG.Expect(response).To(ContainSubstring("\"status\":0"), "A failure occurred while creating the Solr Collection")
		innerG.Expect(response).To(ContainSubstring("\"state\":\"completed\""), "Did not finish creating Solr Collection in time")
	}).Within(time.Second*40).WithContext(ctx).Should(Succeed(), "Collection creation was not successful")

	g.EventuallyWithOffset(additionalOffset, func(innerG Gomega) {
		response, err := callSolrApiInPod(
			ctx,
			solrCloud,
			"get",
			"/solr/admin/collections",
			map[string]string{
				"action":    "DELETESTATUS",
				"requestid": asyncId,
				"wt":        "json",
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while deleting Solr CollectionsAPI AsyncID")
		innerG.Expect(response).To(ContainSubstring("\"status\":0"), "Error occurred while deleting Solr CollectionsAPI AsyncID")
	}).Within(time.Second*10).WithContext(ctx).Should(Succeed(), "Could not delete aysncId after collection creation")
	// Only wait 5 seconds when trying to delete the async requestId

	queryCollectionWithGomega(ctx, solrCloud, collection, 0, g, additionalOffset)
}

func commitCollectionExpectingUnauthorized(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, additionalOffset ...int) {
	commitCollectionWithGomegaExpectingUnauthorized(ctx, solrCloud, collection, Default, resolveOffset(additionalOffset))
}

func commitCollectionWithGomegaExpectingUnauthorized(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, g Gomega, additionalOffset ...int) {
	g.EventuallyWithOffset(resolveOffset(additionalOffset), func(innerG Gomega) {
		response, err := curlSolrInPod(
			ctx,
			solrCloud,
			"get",
			fmt.Sprintf("/solr/%s/update", collection),
			map[string]string{
				"wt":     "json",
				"commit": "true",
			},
		)
		innerG.Expect(err).To(HaveOccurred(), "Commit should have been blocked")
		innerG.Expect(response).To(HaveHTTPStatus(401), "Unexpected success occurred while committing Solr Collection '%s'", collection)
	}).Within(time.Second*5).WithContext(ctx).Should(Succeed(), "Unexpectedly, succesfully committed collection: %v", fetchClusterStatus(ctx, solrCloud))
}

func commitCollection(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, docCount int, additionalOffset ...int) {
	commitCollectionWithGomega(ctx, solrCloud, collection, docCount, Default, resolveOffset(additionalOffset))
}

func commitCollectionWithGomega(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, docCount int, g Gomega, additionalOffset ...int) {
	g.EventuallyWithOffset(resolveOffset(additionalOffset), func(innerG Gomega) {
		response, err := curlSolrInPod(
			ctx,
			solrCloud,
			"get",
			fmt.Sprintf("/solr/%s/update", collection),
			map[string]string{
				"wt":     "json",
				"commit": "true",
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while querying empty Solr Collection")
		innerG.Expect(response).To(ContainSubstring("\"status\":%d", 0), "Error occurred while querying Solr Collection '%s'", collection)
	}).Within(time.Second*5).WithContext(ctx).Should(Succeed(), "Could not successfully query collection: %v", fetchClusterStatus(ctx, solrCloud))
	// Only wait 5 seconds for the collection to be query-able
}

func queryCollection(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, docCount int, additionalOffset ...int) {
	queryCollectionWithGomega(ctx, solrCloud, collection, docCount, Default, resolveOffset(additionalOffset))
}

func queryCollectionWithGomega(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, docCount int, g Gomega, additionalOffset ...int) {
	g.EventuallyWithOffset(resolveOffset(additionalOffset), func(innerG Gomega) {
		response, err := callSolrApiInPod(
			ctx,
			solrCloud,
			"get",
			fmt.Sprintf("/solr/%s/select", collection),
			map[string]string{
				"rows": "0",
				"wt":   "json",
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while querying empty Solr Collection")
		innerG.Expect(response).To(ContainSubstring("\"numFound\":%d", docCount), "Error occurred while querying Solr Collection '%s'", collection)
	}).Within(time.Second*5).WithContext(ctx).Should(Succeed(), "Could not successfully query collection: %v", fetchClusterStatus(ctx, solrCloud))
	// Only wait 5 seconds for the collection to be query-able
}

func fetchClusterStatus(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) string {
	return fetchClusterStatusWithErrorHandling(ctx, solrCloud, true)
}

func fetchClusterStatusWithErrorHandling(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, expectNoError bool) string {
	response, err := callSolrApiInPod(
		ctx,
		solrCloud,
		"get",
		"/solr/admin/collections",
		map[string]string{
			"action": "CLUSTERSTATUS",
			"wt":     "json",
		},
	)
	if expectNoError {
		Expect(err).ToNot(HaveOccurred(), "Could not fetch clusterStatus for cloud")
	}

	return response
}

func queryCollectionWithNoReplicaAvailable(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, collection string, additionalOffset ...int) {
	EventuallyWithOffset(resolveOffset(additionalOffset), func(innerG Gomega) {
		response, _ := callSolrApiInPod(
			ctx,
			solrCloud,
			"get",
			fmt.Sprintf("/solr/%s/select", collection),
			map[string]string{
				"rows": "0",
				"wt":   "json",
			},
		)
		innerG.Expect(response).To(
			// "Exception in thread "main" is for 8.11, which does not handle the exception correctly
			Or(ContainSubstring("Error trying to proxy request for url"), ContainSubstring("Exception in thread \"main\" java.lang.NullPointerException")),
			"Wrong occurred while querying Solr Collection '%s', expected a proxy forwarding error", collection)
	}).Within(time.Second*5).WithContext(ctx).Should(Succeed(), "Collection query did not fail in the correct way")
}

func getPrometheusExporterPod(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter) (podName string) {
	selectorLabels := solrPrometheusExporter.SharedLabels()
	selectorLabels["technology"] = solrv1beta1.SolrPrometheusExporterTechnologyLabel

	labelSelector := labels.SelectorFromSet(selectorLabels)
	listOps := &client.ListOptions{
		Namespace:     solrPrometheusExporter.Namespace,
		LabelSelector: labelSelector,
	}

	foundPods := &corev1.PodList{}
	Expect(k8sClient.List(ctx, foundPods, listOps)).To(Succeed(), "Could not fetch PrometheusExporter pod list")

	for _, pod := range foundPods.Items {
		if pod.Status.ContainerStatuses[0].Ready {
			podName = pod.Name
			break
		}
	}
	Expect(podName).ToNot(BeEmpty(), "Could not find a ready pod to query the PrometheusExporter")
	return podName
}

func checkMetrics(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, solrCloud *solrv1beta1.SolrCloud, collection string, additionalOffset ...int) string {
	return checkMetricsWithGomega(ctx, solrPrometheusExporter, solrCloud, collection, Default, resolveOffset(additionalOffset))
}

func checkMetricsWithGomega(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, solrCloud *solrv1beta1.SolrCloud, collection string, g Gomega, additionalOffset ...int) (response string) {
	g.Eventually(func(innerG Gomega) {
		var err error
		response, err = runExecForContainer(
			ctx,
			util.SolrPrometheusExporterContainer,
			getPrometheusExporterPod(ctx, solrPrometheusExporter),
			solrCloud.Namespace,
			[]string{
				"curl",
				fmt.Sprintf("http://localhost:%d/metrics", util.SolrMetricsPort),
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while querying SolrPrometheusExporter metrics")
		// Add in "cluster_id" to the test when all supported solr versions support the feature. (Solr 9.1)
		//innerG.Expect(response).To(
		//	ContainSubstring("solr_collections_live_nodes", *solrCloud.Spec.Replicas),
		//	"Could not find live_nodes metrics in the PrometheusExporter response",
		//)
		innerG.Expect(response).To(
			MatchRegexp("solr_metrics_core_query_[^{]+\\{category=\"QUERY\",searchHandler=\"/select\",[^}]*collection=\"%s\",[^}]*shard=\"shard1\",[^}]*\\} [0-9]+.0", collection),
			"Could not find query metrics in the PrometheusExporter response",
		)
	}).WithContext(ctx).WithOffset(resolveOffset(additionalOffset)).Within(time.Second * 5).ProbeEvery(time.Millisecond * 200).Should(Succeed())

	return response
}

func checkBackup(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, solrBackup *solrv1beta1.SolrBackup, checks func(g Gomega, collection string, backupListResponse *solr_api.SolrBackupListResponse)) {
	checkBackupWithGomega(ctx, solrCloud, solrBackup, checks, Default)
}

func checkBackupWithGomega(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, solrBackup *solrv1beta1.SolrBackup, checks func(g Gomega, collection string, backupListResponse *solr_api.SolrBackupListResponse), g Gomega) {
	repository := util.GetBackupRepositoryByName(solrCloud.Spec.BackupRepositories, solrBackup.Spec.RepositoryName)
	repositoryName := repository.Name
	if repositoryName == "" {
		g.Expect(solrCloud.Spec.BackupRepositories).To(Not(BeEmpty()), "Solr BackupRepository list cannot be empty in backup test")
	}
	for _, collection := range solrBackup.Spec.Collections {
		backupParams := map[string]string{
			"action":     "LISTBACKUP",
			"name":       util.FullCollectionBackupName(collection, solrBackup.Name),
			"repository": repositoryName,
			"collection": collection,
			"location":   util.BackupLocationPath(repository, solrBackup.Spec.Location),
			"wt":         "json",
		}

		g.Eventually(func(innerG Gomega) {
			response, err := callSolrApiInPod(
				ctx,
				solrCloud,
				"get",
				"/solr/admin/collections",
				backupParams,
			)
			innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while fetching backup '%s' for collection '%s': %s", solrBackup.Name, collection, backupParams)
			backupListResponse := &solr_api.SolrBackupListResponse{}

			innerG.Expect(json.Unmarshal([]byte(response), &backupListResponse)).To(Succeed(), "Could not parse json from Solr BackupList API: %s", response)

			innerG.Expect(backupListResponse.ResponseHeader.Status).To(BeZero(), "SolrBackupList API returned exception code: %d", backupListResponse.ResponseHeader.Status)
			checks(innerG, collection, backupListResponse)
		}).WithContext(ctx).Within(time.Second * 5).ProbeEvery(time.Millisecond * 200).Should(Succeed())
	}
}

type ExecError struct {
	Command string

	Err error

	ErrorOutput string

	ResponseOutput string
}

func (r *ExecError) Error() string {
	return fmt.Sprintf("Error from Pod Exec: %v\n\nError output from Pod Exec: %sResponse output from Pod Exec: %s", r.Err, r.ErrorOutput, r.ResponseOutput)
}

func callSolrApiInPod(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, httpMethod string, apiPath string, queryParams map[string]string, hostnameOptional ...string) (response string, err error) {
	hostname := "${SOLR_HOST}"
	if len(hostnameOptional) > 0 {
		hostname = hostnameOptional[0]
	}
	var queryParamsSlice []string
	for param, val := range queryParams {
		queryParamsSlice = append(queryParamsSlice, param+"="+val)
	}
	queryParamsString := strings.Join(queryParamsSlice, "&")
	if len(queryParamsString) > 0 {
		queryParamsString = "?" + queryParamsString
	}

	command := []string{
		"solr",
		"api",
		"-verbose",
		"-" + strings.ToLower(httpMethod),
		fmt.Sprintf(
			"\"%s://%s%s%s%s\"",
			solrCloud.UrlScheme(false),
			hostname,
			solrCloud.NodePortSuffix(false),
			apiPath,
			queryParamsString),
	}
	return runExecForContainer(ctx, util.SolrNodeContainer, solrCloud.GetRandomSolrPodName(), solrCloud.Namespace, command)
}

func curlSolrInPod(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, httpMethod string, apiPath string, queryParams map[string]string) (response string, err error) {
	var queryParamsSlice []string
	for param, val := range queryParams {
		queryParamsSlice = append(queryParamsSlice, param+"="+val)
	}
	queryParamsString := strings.Join(queryParamsSlice, "&")
	if len(queryParamsString) > 0 {
		queryParamsString = "?" + queryParamsString
	}

	pod, err := createCurlPod(ctx, solrCloud)

	command := []string{
		"curl",
		fmt.Sprintf(
			"\"%s%s%s%s\"",
			solrCloud.Status.InternalCommonAddress,
			solrCloud.NodePortSuffix(false),
			apiPath,
			queryParamsString),
	}
	return runExecForContainer(ctx, util.SolrNodeContainer, pod.Name, pod.Namespace, command)
}

func createCurlPod(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) (pod *v1.Pod, err error) {
	// Define the curl pod
	curlPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl-pod",
			Namespace: solrCloud.Namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "curl",
					Image: "curlimages/curl:latest", // Lightweight image with curl installed
					Args: []string{
						"tail",
						"-f",
						"/dev/null",
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	// Create the curl pod
	return rawK8sClient.CoreV1().Pods(solrCloud.Namespace).Create(context.TODO(), curlPod, metav1.CreateOptions{})
}

func runExecForContainer(ctx context.Context, container string, podName string, namespace string, command []string) (response string, err error) {
	req := rawK8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err = corev1.AddToScheme(scheme); err != nil {
		return "", fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   []string{"sh", "-c", strings.Join(command, " ")},
		Container: container,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	var exec remotecommand.Executor
	exec, err = remotecommand.NewSPDYExecutor(k8sConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("error while creating Executor: %v", err)
	}

	var combined, stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: io.MultiWriter(&stdout, &combined),
		Stderr: io.MultiWriter(&stderr, &combined),
		Tty:    false,
	})

	response = combined.String()

	if err != nil {
		err = &ExecError{
			Command:        strings.Join(command, " "),
			Err:            err,
			ResponseOutput: stdout.String(),
			ErrorOutput:    stderr.String(),
		}

		response = combined.String()
	} else {
		response = stdout.String()
	}

	return response, err
}

func generateBaseSolrCloud(replicas int) *solrv1beta1.SolrCloud {
	return &solrv1beta1.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: testNamespace(),
		},
		Spec: solrv1beta1.SolrCloudSpec{
			Replicas: pointer.Int32(int32(replicas)),
			// Set the image to reflect the inputs given via EnvVars.
			SolrImage: &solrv1beta1.ContainerImage{
				Repository: strings.Split(solrImage, ":")[0],
				Tag:        strings.Split(solrImage+":", ":")[1],
				PullPolicy: corev1.PullIfNotPresent,
			},
			// Use the shared Zookeeper by default, with a unique chRoot for this test
			ZookeeperRef: &solrv1beta1.ZookeeperRef{
				ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{
					InternalConnectionString: sharedZookeeperConnectionString,
					ChRoot:                   "/" + testNamespace() + "/" + rand.String(5),
				},
			},
			// This seems to be the lowest memory & CPU that allow the tests to pass
			SolrJavaMem: "-Xms700m -Xmx700m",
			CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
				PodOptions: &solrv1beta1.PodOptions{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
							corev1.ResourceCPU:    resource.MustParse("1"),
						},
					},
				},
			},
		},
	}
}

// Uses default password from docs : SolrRocks
// The hash is generated as: base64(sha256(sha256(salt+password))) base64(salt))
// See https://solr.apache.org/guide/solr/latest/deployment-guide/basic-authentication-plugin.html
func generateBasicSolrSecuritySecret(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) {
	generateSolrSecuritySecret(ctx, solrCloud, `{
				"authentication": {
				"blockUnknown": false,
				"class": "solr.BasicAuthPlugin",
				"credentials": {
					"test-oper": "IV0EHq1OnNrj6gvRCwvFwTrZ1+z1oBbnQdiVC3otuq0= Ndd7LKvVBAaZIF0QAVi1ekCfAJXr1GGfLtRUXhgrF8c="
				},
				"realm": "Solr Basic Auth",
				"forwardCredentials": false
				},
				"authorization": {
				"class": "solr.RuleBasedAuthorizationPlugin",
				"user-role": {
					"test-oper": "test-oper"
				},
				"permissions": [
					{
					"name": "cluster",
					"role": null
					},
					{
					"name": "collections",
					"role": null,
					"collection": "*"
					}
				]
				}
			}`)

}

// Uses default password from docs : SolrRocks
// The hash is generated as: base64(sha256(sha256(salt+password))) base64(salt))
// See https://solr.apache.org/guide/solr/latest/deployment-guide/basic-authentication-plugin.html
func generateStricterSolrSecuritySecret(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) {
	generateSolrSecuritySecret(ctx, solrCloud, `{
				"authentication": {
				"blockUnknown": false,
				"class": "solr.BasicAuthPlugin",
				"credentials": {
					"test-oper": "IV0EHq1OnNrj6gvRCwvFwTrZ1+z1oBbnQdiVC3otuq0= Ndd7LKvVBAaZIF0QAVi1ekCfAJXr1GGfLtRUXhgrF8c="
				},
				"realm": "Solr Basic Auth",
				"forwardCredentials": false
				},
				"authorization": {
				"class": "solr.RuleBasedAuthorizationPlugin",
				"user-role": {
					"test-oper": "test-oper"
				},
				"permissions": [
					{
					"name": "cluster",
					"role": null
					},
					{
					"name": "collections",
					"role": null,
					"collection": "*"
					},
					{
					"name": "update",
					"role": "test-oper"
					}
				]
				}
			}`)

}

func generateSolrSecuritySecret(ctx context.Context, solrCloud *solrv1beta1.SolrCloud, securityJson string) {

	existingSecret := &corev1.Secret{}
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: solrCloud.Namespace, Name: solrCloud.Name + "-security-secret"}, existingSecret)
	if err == nil {
		By("deleting the existing secret " + existingSecret.Name)
		deleteAndWait(ctx, existingSecret)
	}

	securityJsonSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.Name + "-security-secret",
			Namespace: solrCloud.Namespace,
		},
		StringData: map[string]string{
			"security.json": securityJson,
		},
		Type: corev1.SecretTypeOpaque,
	}
	Expect(k8sClient.Create(ctx, securityJsonSecret)).To(Succeed(), "Failed to create secret for security json in namespace "+solrCloud.Namespace)

	expectSecret(ctx, solrCloud, securityJsonSecret.Name)
	return
}

func generateSolrBasicAuthSecret(ctx context.Context, solrCloud *solrv1beta1.SolrCloud) {
	basicAuthSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.Name + "-basic-auth-secret",
			Namespace: solrCloud.Namespace,
		},
		// Using default creds
		StringData: map[string]string{
			"username": "test-oper",
			"password": "SolrRocks",
		},
		Type: corev1.SecretTypeBasicAuth,
	}
	Expect(k8sClient.Create(ctx, basicAuthSecret)).To(Succeed(), "Failed to create secret for basic auth in namespace "+solrCloud.Namespace)

	expectSecret(ctx, solrCloud, basicAuthSecret.Name)
	return
}

func generateBaseSolrCloudWithSecurityJSON(replicas int) *solrv1beta1.SolrCloud {
	solrCloud := generateBaseSolrCloud(replicas)
	one := intstr.FromInt(1)
	hundredPerc := intstr.FromString("100%")
	solrCloud.Spec.UpdateStrategy = solrv1beta1.SolrUpdateStrategy{
		Method: "Managed",
		ManagedUpdateOptions: solrv1beta1.ManagedUpdateOptions{
			MaxPodsUnavailable:          &one,
			MaxShardReplicasUnavailable: &hundredPerc,
		},
	}

	// Ensure SolrSecurity is initialized
	if solrCloud.Spec.SolrSecurity == nil {
		solrCloud.Spec.SolrSecurity = &solrv1beta1.SolrSecurityOptions{}
	}

	if solrCloud.Spec.SolrSecurity.BootstrapSecurityJson == nil {
		solrCloud.Spec.SolrSecurity.BootstrapSecurityJson = &solrv1beta1.BootstrapSecurityJson{}
	}

	solrCloud.Spec.SolrSecurity.BootstrapSecurityJson.SecurityJsonSecret = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: solrCloud.Name + "-security-secret",
		},
		Key: "security.json",
	}

	solrCloud.Spec.SolrSecurity.AuthenticationType = "Basic"

	solrCloud.Spec.SolrSecurity.BasicAuthSecret = solrCloud.Name + "-basic-auth-secret"

	return solrCloud
}

func generateBaseSolrCloudWithPlacementPolicy(replicas int, placementPlugin string) *solrv1beta1.SolrCloud {
	solrCloud := generateBaseSolrCloud(replicas)
	solrCloud.Spec.CustomSolrKubeOptions.PodOptions.EnvVariables = []corev1.EnvVar{
		{
			Name:  "SOLR_PLACEMENTPLUGIN_DEFAULT",
			Value: placementPlugin,
		},
	}

	return solrCloud
}
