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

package util

import (
	"testing"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	solr "github.com/apache/solr-operator/api/v1beta1"
)

// podMonitorTestCloud returns a defaulted SolrCloud that wants a PodMonitor
func podMonitorTestCloud(t *testing.T) *solr.SolrCloud {
	t.Helper()
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{InternalConnectionString: "zoo-headless:2181"},
			},
			Prometheus: solr.SolrPrometheusOptions{
				PodMonitor:        &solr.SolrPodMonitorOptions{Enable: ptr.To(true)},
				ScrapeAnnotations: &solr.SolrPrometheusScrapeAnnotationsOptions{Enable: ptr.To(true)},
			},
		},
	}
	solrCloud.WithDefaults(logr.Discard())
	return solrCloud
}

func TestPodMonitorForOneCloudSelectsAllOfItsPods(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	solrCloud.Spec.Prometheus.PodMonitor.Interval = "30s"
	solrCloud.Spec.Prometheus.PodMonitor.ScrapeTimeout = "10s"
	solrCloud.Spec.Prometheus.PodMonitor.Labels = map[string]string{"release": "mon"}

	podMonitor := GeneratePodMonitor(solrCloud)

	assert.Equal(t, "example-solrcloud-metrics", podMonitor.Name, "Wrong name for the PodMonitor")
	assert.Equal(t, "default", podMonitor.Namespace, "The PodMonitor must live in the namespace of the SolrCloud")
	assert.Equal(t, "mon", podMonitor.Labels["release"], "The PodMonitor must carry the user-provided labels, as the Prometheus instance selects it by those")
	assert.Equal(t, "example", podMonitor.Labels["solr-cloud"], "The PodMonitor must carry the shared labels of the SolrCloud")

	// One PodMonitor selects every pod of the cloud, instead of one PodMonitor per pod
	assert.Equal(t, map[string]string{"solr-cloud": "example", "technology": solr.SolrTechnologyLabel}, podMonitor.Spec.Selector.MatchLabels,
		"The PodMonitor must select all pods of the SolrCloud")

	assert.Len(t, podMonitor.Spec.PodMetricsEndpoints, 1, "Wrong number of endpoints for the PodMonitor")
	endpoint := podMonitor.Spec.PodMetricsEndpoints[0]
	assert.Equal(t, ptr.To(SolrClientPortName), endpoint.Port, "The PodMonitor must scrape the Solr client port")
	assert.Equal(t, "/solr/admin/metrics", endpoint.Path, "Wrong metrics path for the PodMonitor")
	// Solr 10 defaults to this format, but the PodMonitor pins it instead of relying on the default
	assert.Equal(t, map[string][]string{"wt": {"prometheus"}}, endpoint.Params, "The PodMonitor must ask the metrics API for the Prometheus format")
	assert.Equal(t, ptr.To(monitoringv1.Scheme("http")), endpoint.Scheme, "Wrong scheme for the PodMonitor")
	assert.Equal(t, monitoringv1.Duration("30s"), endpoint.Interval, "Wrong scrape interval for the PodMonitor")
	assert.Equal(t, monitoringv1.Duration("10s"), endpoint.ScrapeTimeout, "Wrong scrape timeout for the PodMonitor")
	assert.Nil(t, endpoint.BasicAuth, "The PodMonitor should not use basic auth, as the SolrCloud does not")
	assert.Nil(t, endpoint.TLSConfig, "The PodMonitor should not have a TLS config, as the SolrCloud does not use TLS")
}

func TestPodMonitorUsesHttpsAndTheGivenCAForATlsEnabledCloud(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	solrCloud.Spec.SolrTLS = &solr.SolrTLSOptions{
		PKCS12Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "solr-tls"},
			Key:                  "keystore.p12",
		},
	}
	solrCloud.Spec.Prometheus.PodMonitor.TLSConfig = &solr.SolrPodMonitorTLSConfig{
		CASecret:   &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "solr-ca"}, Key: "ca.crt"},
		ServerName: "solr.example.com",
	}

	endpoint := GeneratePodMonitor(solrCloud).Spec.PodMetricsEndpoints[0]

	assert.Equal(t, ptr.To(monitoringv1.Scheme("https")), endpoint.Scheme, "A TLS-enabled SolrCloud must be scraped over https")
	assert.NotNil(t, endpoint.TLSConfig, "The PodMonitor must tell Prometheus how to trust the Solr certificates")
	assert.Equal(t, "solr-ca", endpoint.TLSConfig.CA.Secret.Name, "Wrong CA secret for the PodMonitor")
	assert.Equal(t, "ca.crt", endpoint.TLSConfig.CA.Secret.Key, "Wrong CA secret key for the PodMonitor")
	assert.Equal(t, ptr.To("solr.example.com"), endpoint.TLSConfig.ServerName, "Wrong serverName for the PodMonitor")
	assert.Equal(t, ptr.To(false), endpoint.TLSConfig.InsecureSkipVerify, "The Solr certificates should be verified unless asked otherwise")
	assert.Nil(t, endpoint.TLSConfig.Cert.Secret, "No client certificate should be presented unless given")
	assert.Nil(t, endpoint.TLSConfig.KeySecret, "No client key should be presented unless given")
}

func TestPodMonitorPresentsAClientCertificateWhenGiven(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	// Solr rejects the TLS handshake of a scrape without a client certificate
	solrCloud.Spec.SolrTLS = &solr.SolrTLSOptions{
		PKCS12Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "solr-tls"},
			Key:                  "keystore.p12",
		},
		ClientAuth: solr.Need,
	}
	solrCloud.Spec.Prometheus.PodMonitor.TLSConfig = &solr.SolrPodMonitorTLSConfig{
		CASecret:   &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "prometheus-tls"}, Key: "ca.crt"},
		CertSecret: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "prometheus-tls"}, Key: "tls.crt"},
		KeySecret:  &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "prometheus-tls"}, Key: "tls.key"},
	}

	endpoint := GeneratePodMonitor(solrCloud).Spec.PodMetricsEndpoints[0]

	require.NotNil(t, endpoint.TLSConfig, "The PodMonitor must tell Prometheus how to trust the Solr certificates")
	require.NotNil(t, endpoint.TLSConfig.Cert.Secret, "The PodMonitor must present the given client certificate")
	assert.Equal(t, "prometheus-tls", endpoint.TLSConfig.Cert.Secret.Name, "Wrong client certificate secret for the PodMonitor")
	assert.Equal(t, "tls.crt", endpoint.TLSConfig.Cert.Secret.Key, "Wrong client certificate key for the PodMonitor")
	require.NotNil(t, endpoint.TLSConfig.KeySecret, "The PodMonitor must present the given client key")
	assert.Equal(t, "prometheus-tls", endpoint.TLSConfig.KeySecret.Name, "Wrong client key secret for the PodMonitor")
	assert.Equal(t, "tls.key", endpoint.TLSConfig.KeySecret.Key, "Wrong client key for the PodMonitor")
}

func TestPodMonitorAuthenticatesWhenTheCloudUsesBasicAuth(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	solrCloud.Spec.SolrSecurity = &solr.SolrSecurityOptions{AuthenticationType: solr.Basic}

	endpoint := GeneratePodMonitor(solrCloud).Spec.PodMetricsEndpoints[0]

	assert.NotNil(t, endpoint.BasicAuth, "The PodMonitor must authenticate when the SolrCloud uses basic auth")
	assert.Equal(t, solrCloud.BasicAuthSecretName(), endpoint.BasicAuth.Username.Name,
		"The PodMonitor should default to the basic auth secret of the SolrCloud, whose user may read /admin/metrics")
	assert.Equal(t, corev1.BasicAuthUsernameKey, endpoint.BasicAuth.Username.Key, "Wrong username key for the PodMonitor")
	assert.Equal(t, solrCloud.BasicAuthSecretName(), endpoint.BasicAuth.Password.Name, "Wrong password secret for the PodMonitor")
	assert.Equal(t, corev1.BasicAuthPasswordKey, endpoint.BasicAuth.Password.Key, "Wrong password key for the PodMonitor")

	// A dedicated, less privileged Solr user can be given to Prometheus instead
	solrCloud.Spec.Prometheus.PodMonitor.BasicAuthSecret = "metrics-only-user"
	endpoint = GeneratePodMonitor(solrCloud).Spec.PodMetricsEndpoints[0]
	assert.Equal(t, "metrics-only-user", endpoint.BasicAuth.Username.Name, "The provided basic auth secret must be used")
	assert.Equal(t, "metrics-only-user", endpoint.BasicAuth.Password.Name, "The provided basic auth secret must be used")
}

func TestPrometheusScrapeAnnotationsPointAtTheMetricsEndpoint(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)

	annotations := PrometheusScrapeAnnotations(solrCloud)
	assert.Equal(t, map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/scheme": "http",
		"prometheus.io/path":   "/solr/admin/metrics",
		"prometheus.io/port":   "8983",
		// The wt parameter cannot go in the path annotation, as Prometheus escapes the "?" there
		"prometheus.io/param-wt": "prometheus",
	}, annotations, "Wrong prometheus.io/* annotations for the Solr pods")

	solrCloud.Spec.SolrAddressability.PodPort = 9983
	solrCloud.Spec.SolrTLS = &solr.SolrTLSOptions{}
	annotations = PrometheusScrapeAnnotations(solrCloud)
	assert.Equal(t, "9983", annotations["prometheus.io/port"], "The annotations must point at the port that Solr listens on")
	assert.Equal(t, "https", annotations["prometheus.io/scheme"], "A TLS-enabled SolrCloud must be scraped over https")
}

func TestCopyPodMonitorFieldsDetectsSpecChanges(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	wanted := GeneratePodMonitor(solrCloud)

	unchanged := GeneratePodMonitor(solrCloud)
	assert.False(t, CopyPodMonitorFields(wanted, unchanged, logr.Discard()), "No update should be required for an unchanged PodMonitor")

	// Someone changed the scrape interval of the PodMonitor behind the operator's back
	drifted := GeneratePodMonitor(solrCloud)
	drifted.Spec.PodMetricsEndpoints[0].Interval = monitoringv1.Duration("5m")
	assert.True(t, CopyPodMonitorFields(wanted, drifted, logr.Discard()), "An update should be required after the PodMonitor spec was changed")
	assert.Equal(t, wanted.Spec, drifted.Spec, "The PodMonitor spec should be reset to what the operator generated")
}

func TestCopyPodMonitorFieldsReconcilesTheLabels(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	solrCloud.Spec.Prometheus.PodMonitor.Labels = map[string]string{"release": "mon", "team": "search"}
	existing := GeneratePodMonitor(solrCloud)
	// A label that someone else put on the PodMonitor, which the operator does not manage
	existing.Labels["added-by-policy"] = "yes"

	// The user takes the cloud out of that Prometheus instance
	delete(solrCloud.Spec.Prometheus.PodMonitor.Labels, "release")
	wanted := GeneratePodMonitor(solrCloud)

	assert.True(t, CopyPodMonitorFields(wanted, existing, logr.Discard()), "An update should be required after a label was dropped from the spec")
	assert.NotContains(t, existing.Labels, "release", "A label dropped from the spec must come off the PodMonitor, or that Prometheus keeps scraping")
	assert.Equal(t, "search", existing.Labels["team"], "The remaining labels of the spec must stay on the PodMonitor")
	assert.Equal(t, "yes", existing.Labels["added-by-policy"], "A label that the operator never set is not the operator's to remove")
	assert.Equal(t, "solr-cloud,team", existing.Annotations[ManagedLabelsAnnotation], "The annotation must list the labels that the operator now manages")

	assert.False(t, CopyPodMonitorFields(wanted, existing, logr.Discard()), "No further update should be required once the labels are reconciled")
}

func TestCopyPodMonitorFieldsAddsAnEmptyValuedLabel(t *testing.T) {
	solrCloud := podMonitorTestCloud(t)
	existing := GeneratePodMonitor(solrCloud)

	// An empty value is a valid label value, and a Prometheus can select on it
	solrCloud.Spec.Prometheus.PodMonitor.Labels = map[string]string{"scrape": ""}
	wanted := GeneratePodMonitor(solrCloud)

	assert.True(t, CopyPodMonitorFields(wanted, existing, logr.Discard()), "An update should be required after an empty-valued label was added to the spec")
	assert.Contains(t, existing.Labels, "scrape", "An empty-valued label must be put on the PodMonitor, so that a Prometheus selecting it finds the PodMonitor")
}
