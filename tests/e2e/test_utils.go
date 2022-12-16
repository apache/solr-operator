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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"os"
	"strings"
)

const (
	helmDriver = "configmap"
)

var (
	settings = cli.New()
)

// Run Solr Operator for e2e testing of resources
func RunSolrOperator() *release.Release {
	actionConfig := new(action.Configuration)
	Expect(actionConfig.Init(settings.RESTClientGetter(), "solr-operator", helmDriver, GinkgoLogr.Info)).To(Succeed(), "Failed to create helm configuration")

	installClient := action.NewInstall(actionConfig)

	chartPath, err := installClient.LocateChart("../../helm/solr-operator", settings)
	Expect(err).ToNot(HaveOccurred(), "Failed to locate solr-operator Helm chart")

	chart, err := loader.Load(chartPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to load solr-operator Helm chart")

	installClient.Namespace = "solr-operator"
	installClient.ReleaseName = "solr-operator"
	installClient.SkipCRDs = true
	installClient.CreateNamespace = true
	operatorImage := os.Getenv("OPERATOR_IMAGE")
	operatorRepo, operatorTag, found := strings.Cut(operatorImage, ":")
	Expect(found).To(BeTrue(), "Invalid Operator image found in envVar OPERATOR_IMAGE: "+operatorImage)
	solrOperatorRelease, err := installClient.Run(chart, map[string]interface{}{
		"image": map[string]interface{}{
			"repostitory": operatorRepo,
			"tag":         operatorTag,
			"pullPolicy":  "Never",
		},
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to install solr-operator via Helm chart")
	Expect(solrOperatorRelease).ToNot(BeNil(), "Failed to install solr-operator via Helm chart")

	return solrOperatorRelease
}

// Run Solr Operator for e2e testing of resources
func StopSolrOperator(release *release.Release) {
	actionConfig := new(action.Configuration)
	Expect(actionConfig.Init(settings.RESTClientGetter(), "solr-operator", helmDriver, GinkgoLogr.Info)).To(Succeed(), "Failed to create helm configuration")

	uninstallClient := action.NewUninstall(actionConfig)

	_, err := uninstallClient.Run(release.Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to uninstall solr-operator release: "+release.Name)
}
