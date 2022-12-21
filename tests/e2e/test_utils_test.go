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
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/remotecommand"
	"math/rand"
	"os"
	"strings"
)

const (
	helmDriver = "configmap"
)

var (
	settings = cli.New()
)

func testNamespace() string {
	return fmt.Sprintf("solr-e2e-%d", GinkgoParallelProcess())
}

// Run Solr Operator for e2e testing of resources
func runSolrOperator() *release.Release {
	actionConfig := new(action.Configuration)
	Expect(actionConfig.Init(settings.RESTClientGetter(), "solr-operator", helmDriver, GinkgoLogr.Info)).To(Succeed(), "Failed to create helm configuration")

	installClient := action.NewInstall(actionConfig)

	chart, err := loader.Load("../../helm/solr-operator")
	Expect(err).ToNot(HaveOccurred(), "Failed to load solr-operator Helm chart")

	installClient.Namespace = "solr-operator"
	installClient.ReleaseName = "solr-operator"
	installClient.SkipCRDs = true
	installClient.CreateNamespace = true
	operatorImage := getEnvWithDefault(operatorImageEnv, defaultOperatorImage)
	operatorRepo, operatorTag, found := strings.Cut(operatorImage, ":")
	Expect(found).To(BeTrue(), "Invalid Operator image found in envVar OPERATOR_IMAGE: "+operatorImage)
	solrOperatorHelmRelease, err := installClient.Run(chart, map[string]interface{}{
		"image": map[string]interface{}{
			"repostitory": operatorRepo,
			"tag":         operatorTag,
			"pullPolicy":  "Never",
		},
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to install solr-operator via Helm chart")
	Expect(solrOperatorHelmRelease).ToNot(BeNil(), "Failed to install solr-operator via Helm chart")

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

// Run Solr Operator for e2e testing of resources
func getEnvWithDefault(envVar string, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		value = defaultValue
	}
	return value
}

func createAndQueryCollection(solrCloud *solrv1beta1.SolrCloud, collection string) {
	createAndQueryCollectionWithGomega(solrCloud, collection, Default)
}

func createAndQueryCollectionWithGomega(solrCloud *solrv1beta1.SolrCloud, collection string, g Gomega) {
	rand.Seed(GinkgoRandomSeed() + int64(GinkgoParallelProcess()))
	pod := solrCloud.GetAllSolrPodNames()[0]
	response, err := runExecForPod(
		pod,
		solrCloud.Namespace,
		[]string{
			"curl",
			"http://localhost:8983/solr/admin/collections?action=CREATE&name=" + collection + "&replicationFactor=2&numShards=1",
		},
	)
	g.Expect(err).To(Not(HaveOccurred()), "Error occured while creating Solr Collection")
	g.Expect(response).To(ContainSubstring("\"status\":0"), "Error occured while creating Solr Collection")

	response, err = runExecForPod(
		pod,
		solrCloud.Namespace,
		[]string{
			"curl",
			"http://localhost:8983/solr/" + collection + "/select",
		},
	)
	g.Expect(err).To(Not(HaveOccurred()), "Error occured while querying empty Solr Collection")
	g.Expect(response).To(ContainSubstring("\"numFound\":0"), "Error occured while querying empty Solr Collection")
}

func runExecForPod(podName string, namespace string, command []string) (response string, err error) {
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
		Container: "solrcloud-node",
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
