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
	"fmt"
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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
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
	if _, err = histClient.Run(solrOperatorReleaseName); err == driver.ErrReleaseNotFound {
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

	By("creating the shared ZK cluster")
	Expect(k8sClient.Create(ctx, zookeeper)).To(Succeed(), "Failed to create shared Zookeeper Cluster")

	By("Waiting for the Zookeeper to come up healthy")
	zookeeper = expectZookeeperClusterWithChecks(ctx, zookeeper, zookeeper.Name, func(g Gomega, found *zkApi.ZookeeperCluster) {
		g.Expect(found.Status.ReadyReplicas).To(Equal(found.Spec.Replicas), "The ZookeeperCluster should have all nodes come up healthy")
	})

	DeferCleanup(func(ctx context.Context) {
		By("tearing down the shared Zookeeper Cluster")
		stopSharedZookeeperCluster(ctx, zookeeper)
	})

	return zookeeper
}

// Run Solr Operator for e2e testing of resources
func stopSharedZookeeperCluster(ctx context.Context, sharedZk *zkApi.ZookeeperCluster) {
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: sharedZk.Namespace, Name: sharedZk.Name}, sharedZk)
	if err == nil {
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

func createAndQueryCollection(solrCloud *solrv1beta1.SolrCloud, collection string, shards int, replicasPerShard int, nodes ...int) {
	createAndQueryCollectionWithGomega(solrCloud, collection, shards, replicasPerShard, Default, nodes...)
}

func createAndQueryCollectionWithGomega(solrCloud *solrv1beta1.SolrCloud, collection string, shards int, replicasPerShard int, g Gomega, nodes ...int) {
	pod := solrCloud.GetAllSolrPodNames()[0]
	asyncId := fmt.Sprintf("create-collection-%s-%d-%d", collection, shards, replicasPerShard)

	var nodeSet []string
	for _, node := range nodes {
		nodeSet = append(nodeSet, util.SolrNodeName(solrCloud, solrCloud.GetSolrPodName(node)))
	}
	createNodeSet := ""
	if len(nodeSet) > 0 {
		createNodeSet = "&createNodeSet=" + strings.Join(nodeSet, ",")
	}

	g.Eventually(func(innerG Gomega) {
		response, err := runExecForContainer(
			util.SolrNodeContainer,
			pod,
			solrCloud.Namespace,
			[]string{
				"curl",
				fmt.Sprintf(
					"http://localhost:%d/solr/admin/collections?action=CREATE&name=%s&replicationFactor=%d&numShards=%d%s&async=%s",
					solrCloud.Spec.SolrAddressability.PodPort,
					collection,
					replicasPerShard,
					shards,
					createNodeSet,
					asyncId),
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while creating Solr Collection")
		innerG.Expect(response).To(ContainSubstring("\"status\":0"), "Error occurred while creating Solr Collection")
	}).Should(Succeed(), "Collection creation was not successful")

	g.Eventually(func(innerG Gomega) {
		response, err := runExecForContainer(
			util.SolrNodeContainer,
			pod,
			solrCloud.Namespace,
			[]string{
				"curl",
				fmt.Sprintf(
					"http://localhost:%d/solr/admin/collections?action=REQUESTSTATUS&requestid=%s",
					solrCloud.Spec.SolrAddressability.PodPort,
					asyncId),
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while checking if Solr Collection has been created")
		innerG.Expect(response).To(ContainSubstring("\"status\":0"), "Error occurred while creating Solr Collection")
		innerG.Expect(response).To(ContainSubstring("\"state\":\"completed\""), "Did not finish creating Solr Collection in time")
		if strings.Contains(response, "\"state\":\"failed\"") || strings.Contains(response, "\"state\":\"notfound\"") {
			StopTrying("A failure occurred while creating the Solr Collection").
				Attach("Collection", collection).
				Attach("Shards", shards).
				Attach("ReplicasPerShard", replicasPerShard).
				Attach("Response", response).
				Now()
		}
	}).Should(Succeed(), "Collection creation was not successful")

	g.Eventually(func(innerG Gomega) {
		response, err := runExecForContainer(
			util.SolrNodeContainer,
			pod,
			solrCloud.Namespace,
			[]string{
				"curl",
				fmt.Sprintf(
					"http://localhost:%d/solr/admin/collections?action=DELETESTATUS&requestid=%s",
					solrCloud.Spec.SolrAddressability.PodPort,
					asyncId),
			},
		)
		innerG.Expect(err).ToNot(HaveOccurred(), "Error occurred while deleting Solr CollectionsAPI AsyncID")
		innerG.Expect(response).To(ContainSubstring("\"status\":0"), "Error occurred while deleting Solr CollectionsAPI AsyncID")
	}).Should(Succeed(), "Could not delete aysncId after collection creation")

	queryCollectionWithGomega(solrCloud, collection, 0, g)
}

func queryCollection(solrCloud *solrv1beta1.SolrCloud, collection string, docCount int) {
	queryCollectionWithGomega(solrCloud, collection, docCount, Default)
}

func queryCollectionWithGomega(solrCloud *solrv1beta1.SolrCloud, collection string, docCount int, g Gomega) {
	pod := solrCloud.GetAllSolrPodNames()[0]
	response, err := runExecForContainer(
		util.SolrNodeContainer,
		pod,
		solrCloud.Namespace,
		[]string{
			"curl",
			fmt.Sprintf("http://localhost:%d/solr/%s/select", solrCloud.Spec.SolrAddressability.PodPort, collection),
		},
	)
	g.Expect(err).ToNot(HaveOccurred(), "Error occurred while querying empty Solr Collection")
	g.Expect(response).To(ContainSubstring("\"numFound\":%d", docCount), "Error occurred while querying Solr Collection '%s'", collection)
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

func checkMetrics(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, solrCloud *solrv1beta1.SolrCloud, collection string) string {
	return checkMetricsWithGomega(ctx, solrPrometheusExporter, solrCloud, collection, Default)
}

func checkMetricsWithGomega(ctx context.Context, solrPrometheusExporter *solrv1beta1.SolrPrometheusExporter, solrCloud *solrv1beta1.SolrCloud, collection string, g Gomega) (response string) {
	g.Eventually(func(innerG Gomega) {
		var err error
		response, err = runExecForContainer(
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
	}).WithContext(ctx).Within(time.Second * 5).ProbeEvery(time.Millisecond * 200).Should(Succeed())

	return response
}

func checkBackup(solrCloud *solrv1beta1.SolrCloud, solrBackup *solrv1beta1.SolrBackup, checks func(collection string, backupListResponse *solr_api.SolrBackupListResponse)) {
	checkBackupWithGomega(solrCloud, solrBackup, checks, Default)
}

func checkBackupWithGomega(solrCloud *solrv1beta1.SolrCloud, solrBackup *solrv1beta1.SolrBackup, checks func(collection string, backupListResponse *solr_api.SolrBackupListResponse), g Gomega) {
	solrCloudPod := solrCloud.GetAllSolrPodNames()[0]

	repository := util.GetBackupRepositoryByName(solrCloud.Spec.BackupRepositories, solrBackup.Spec.RepositoryName)
	repositoryName := repository.Name
	if repositoryName == "" {
		g.Expect(solrCloud.Spec.BackupRepositories).To(Not(BeEmpty()), "Solr BackupRepository list cannot be empty in backup test")
	}
	for _, collection := range solrBackup.Spec.Collections {
		curlCommand := fmt.Sprintf(
			"http://localhost:%d/solr/admin/collections?action=LISTBACKUP&name=%s&repository=%s&collection=%s&location=%s",
			solrCloud.Spec.SolrAddressability.PodPort,
			util.FullCollectionBackupName(collection, solrBackup.Name),
			repositoryName,
			collection,
			util.BackupLocationPath(repository, solrBackup.Spec.Location))
		response, err := runExecForContainer(
			util.SolrNodeContainer,
			solrCloudPod,
			solrCloud.Namespace,
			[]string{
				"curl",
				curlCommand,
			},
		)
		g.Expect(err).ToNot(HaveOccurred(), "Error occurred while fetching backup '%s' for collection '%s': %s", solrBackup.Name, collection, curlCommand)
		backupListResponse := &solr_api.SolrBackupListResponse{}

		g.Expect(json.Unmarshal([]byte(response), &backupListResponse)).To(Succeed(), "Could not parse json from Solr BackupList API")

		g.Expect(backupListResponse.ResponseHeader.Status).To(BeZero(), "SolrBackupList API returned exception code: %d", backupListResponse.ResponseHeader.Status)
		checks(collection, backupListResponse)
	}
}

func runExecForContainer(container string, podName string, namespace string, command []string) (response string, err error) {
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
		Command:   command,
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

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return "", fmt.Errorf("error in Stream: %v", err)
	}

	return stdout.String(), err
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
					ChRoot:                   "/" + rand.String(5),
				},
			},
			// This seems to be the lowest memory & CPU that allow the tests to pass
			SolrJavaMem: "-Xms512m -Xmx512m",
			CustomSolrKubeOptions: solrv1beta1.CustomSolrKubeOptions{
				PodOptions: &solrv1beta1.PodOptions{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("600Mi"),
							corev1.ResourceCPU:    resource.MustParse("1"),
						},
					},
				},
			},
		},
	}
}
