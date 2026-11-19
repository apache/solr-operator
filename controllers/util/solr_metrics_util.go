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
	// SolrMetricsPath is the path of Solr's metrics API
	SolrMetricsPath = "/solr/admin/metrics"

	// Solr 10 answers in the Prometheus exposition format even without this parameter, but asking for
	// it explicitly means a change of that default cannot silently break scraping.
	SolrMetricsWtParam      = "wt"
	SolrMetricsWtPrometheus = "prometheus"

	PrometheusScrapeAnnotation = "prometheus.io/scrape"
	PrometheusSchemeAnnotation = "prometheus.io/scheme"
	PrometheusPathAnnotation   = "prometheus.io/path"
	PrometheusPortAnnotation   = "prometheus.io/port"
	// The wt parameter needs an annotation of its own: Prometheus puts the path annotation in
	// __metrics_path__, where a "?" is escaped. A scrape config maps this one to the __param_wt label.
	PrometheusWtParamAnnotation = "prometheus.io/param-" + SolrMetricsWtParam
)

// PrometheusScrapeAnnotations returns the prometheus.io/* annotations describing how to scrape the
// metrics API of a Solr pod. They are a convention, so only a scrape config acting on them has effect.
func PrometheusScrapeAnnotations(solrCloud *solr.SolrCloud) map[string]string {
	return map[string]string{
		PrometheusScrapeAnnotation:  "true",
		PrometheusSchemeAnnotation:  solrCloud.UrlScheme(false),
		PrometheusPathAnnotation:    SolrMetricsPath,
		PrometheusPortAnnotation:    strconv.Itoa(solrCloud.Spec.SolrAddressability.PodPort),
		PrometheusWtParamAnnotation: SolrMetricsWtPrometheus,
	}
}

// GeneratePodMonitor returns a new monitoringv1.PodMonitor for the metrics API of the SolrCloud's
// pods. One PodMonitor covers the whole cloud: the Prometheus Operator turns it into a single scrape
// job that discovers every Solr pod as its own target, following the cloud as it scales.
func GeneratePodMonitor(solrCloud *solr.SolrCloud) *monitoringv1.PodMonitor {
	options := solrCloud.Spec.Prometheus.PodMonitor

	endpoint := monitoringv1.PodMetricsEndpoint{
		Port:          ptr.To(SolrClientPortName),
		Path:          SolrMetricsPath,
		Params:        map[string][]string{SolrMetricsWtParam: {SolrMetricsWtPrometheus}},
		Scheme:        ptr.To(monitoringv1.Scheme(solrCloud.UrlScheme(false))),
		Interval:      monitoringv1.Duration(options.Interval),
		ScrapeTimeout: monitoringv1.Duration(options.ScrapeTimeout),
	}

	// The secret of the SolrCloud is used unless another one is given, as the bootstrapped
	// security.json grants its user access to /admin/metrics.
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

	if tlsConfig := options.TLSConfig; tlsConfig != nil {
		endpoint.TLSConfig = &monitoringv1.SafeTLSConfig{
			InsecureSkipVerify: ptr.To(tlsConfig.InsecureSkipVerify),
		}
		if tlsConfig.CASecret != nil {
			endpoint.TLSConfig.CA = monitoringv1.SecretOrConfigMap{Secret: tlsConfig.CASecret}
		}
		// A client certificate, for a SolrCloud that requires client authentication
		if tlsConfig.CertSecret != nil {
			endpoint.TLSConfig.Cert = monitoringv1.SecretOrConfigMap{Secret: tlsConfig.CertSecret}
		}
		endpoint.TLSConfig.KeySecret = tlsConfig.KeySecret
		if tlsConfig.ServerName != "" {
			endpoint.TLSConfig.ServerName = ptr.To(tlsConfig.ServerName)
		}
	}

	// The Prometheus instance selects the PodMonitor by its labels, so the user's labels win
	labels := MergeLabelsOrAnnotations(options.Labels, solrCloud.SharedLabels())

	return &monitoringv1.PodMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.PodMonitorName(),
			Namespace:   solrCloud.Namespace,
			Labels:      labels,
			Annotations: map[string]string{ManagedLabelsAnnotation: ManagedLabelKeys(labels)},
		},
		Spec: monitoringv1.PodMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: solrCloud.SharedLabelsWith(map[string]string{"technology": solr.SolrTechnologyLabel}),
			},
			PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{endpoint},
		},
	}
}
