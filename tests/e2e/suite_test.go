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
	"helm.sh/helm/v3/pkg/release"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var solrOperatorRelease *release.Release

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting Solr Operator E2E suite\n")
	RunSpecs(t, "Solr Operator e2e suite")
}

var _ = BeforeSuite(func() {
	By("starting the test solr operator")
	solrOperatorRelease = RunSolrOperator()
	Expect(solrOperatorRelease).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	if solrOperatorRelease != nil {
		By("tearing down the test solr operator")
		StopSolrOperator(solrOperatorRelease)
	}
})
