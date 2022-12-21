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
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/zk_api"
	"github.com/apache/solr-operator/version"
	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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

var _ = BeforeSuite(func() {
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

	By("starting the test solr operator")
	solrOperatorRelease = runSolrOperator()
	Expect(solrOperatorRelease).ToNot(BeNil())

	By("setting up the k8s client")
	Expect(solrv1beta1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(zk_api.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	var err error
	k8sConfig, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Could not load in default kubernetes config")
	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
})

var _ = AfterSuite(func() {
	if solrOperatorRelease != nil {
		By("tearing down the test solr operator")
		stopSolrOperator(solrOperatorRelease)
	}
})
