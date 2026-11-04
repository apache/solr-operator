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
	"strconv"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	solr "github.com/apache/solr-operator/api/v1beta1"
)

const (
	// SolrMetricsPath is the path that Solr 10 and later serve prometheus-format metrics on
	SolrMetricsPath = "/solr/admin/metrics"

	PrometheusScrapeAnnotation = "prometheus.io/scrape"
	PrometheusSchemeAnnotation = "prometheus.io/scheme"
	PrometheusPathAnnotation   = "prometheus.io/path"
	PrometheusPortAnnotation   = "prometheus.io/port"
)

// PrometheusScrapeAnnotations returns the prometheus.io/* annotations describing how to scrape the
// metrics endpoint of a Solr pod. These are a widely used convention rather than a part of Prometheus
// itself, so they only have an effect if the Prometheus scrape config is set up to act on them.
func PrometheusScrapeAnnotations(solrCloud *solr.SolrCloud) map[string]string {
	return map[string]string{
		PrometheusScrapeAnnotation: "true",
		PrometheusSchemeAnnotation: solrCloud.UrlScheme(false),
		PrometheusPathAnnotation:   SolrMetricsPath,
		PrometheusPortAnnotation:   strconv.Itoa(solrCloud.Spec.SolrAddressability.PodPort),
	}
}

// GeneratePodMonitor returns a new monitoringv1.PodMonitor pointing at the metrics endpoint of the
// SolrCloud's pods. One PodMonitor covers the whole cloud: the Prometheus Operator turns it into a
// single scrape job that discovers every Solr pod as its own target, following the cloud as it scales.
func GeneratePodMonitor(solrCloud *solr.SolrCloud) *monitoringv1.PodMonitor {
	options := solrCloud.Spec.Prometheus.PodMonitor

	endpoint := monitoringv1.PodMetricsEndpoint{
		Port:          ptr.To(SolrClientPortName),
		Path:          SolrMetricsPath,
		Scheme:        ptr.To(monitoringv1.Scheme(solrCloud.UrlScheme(false))),
		Interval:      monitoringv1.Duration(options.Interval),
		ScrapeTimeout: monitoringv1.Duration(options.ScrapeTimeout),
	}

	// Solr requires authentication for /admin/metrics when basic auth is enabled, so Prometheus needs
	// credentials. The secret of the SolrCloud is used unless another one is given, as its user is
	// granted access to /admin/metrics by the bootstrapped security.json.
	if solrCloud.Spec.SolrSecurity != nil && solrCloud.Spec.SolrSecurity.AuthenticationType == solr.Basic {
		basicAuthSecret := options.BasicAuthSecret
		if basicAuthSecret == "" {
			basicAuthSecret = solrCloud.BasicAuthSecretName()
		}
		endpoint.BasicAuth = &monitoringv1.BasicAuth{
			Username: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: basicAuthSecret},
				Key:                  corev1.BasicAuthUsernameKey,
			},
			Password: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: basicAuthSecret},
				Key:                  corev1.BasicAuthPasswordKey,
			},
		}
	}

	// The scheme already says https when the SolrCloud uses TLS, but Prometheus also needs to be told
	// how to trust the Solr certificates.
	if tlsConfig := options.TLSConfig; tlsConfig != nil {
		endpoint.TLSConfig = &monitoringv1.SafeTLSConfig{
			InsecureSkipVerify: ptr.To(tlsConfig.InsecureSkipVerify),
		}
		if tlsConfig.CASecret != nil {
			endpoint.TLSConfig.CA = monitoringv1.SecretOrConfigMap{Secret: tlsConfig.CASecret}
		}
		if tlsConfig.ServerName != "" {
			endpoint.TLSConfig.ServerName = ptr.To(tlsConfig.ServerName)
		}
	}

	return &monitoringv1.PodMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrCloud.PodMonitorName(),
			Namespace: solrCloud.Namespace,
			// The labels of the PodMonitor are what the Prometheus instance selects it by, so the
			// user-provided labels are the important ones here.
			Labels: MergeLabelsOrAnnotations(options.Labels, solrCloud.SharedLabels()),
		},
		Spec: monitoringv1.PodMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: solrCloud.SharedLabelsWith(map[string]string{"technology": solr.SolrTechnologyLabel}),
			},
			PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{endpoint},
		},
	}
}
