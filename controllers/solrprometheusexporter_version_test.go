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

package controllers

import (
	"testing"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
)

func TestUnsupportedExporterSolrVersion(t *testing.T) {
	img := func(tag string) *solrv1beta1.ContainerImage {
		return &solrv1beta1.ContainerImage{Repository: "solr", Tag: tag}
	}
	tests := []struct {
		name          string
		exporterImage *solrv1beta1.ContainerImage
		cloudImage    *solrv1beta1.ContainerImage
		wantBlocked   bool
	}{
		{name: "no images", wantBlocked: false},
		{name: "Solr 9 image", exporterImage: img("9.10.0"), wantBlocked: false},
		{name: "Solr 10 image", exporterImage: img("10.0.0"), wantBlocked: true},
		{name: "Solr 10 image with digest", exporterImage: img("10.0.0@sha256:abc"), wantBlocked: true},
		{name: "Solr 11 image", exporterImage: img("11.0.0"), wantBlocked: true},
		{name: "custom exporter image tag", exporterImage: img("my-exporter-3"), wantBlocked: false},
		{name: "inherited Solr 9 cloud image", cloudImage: img("9.10.0"), wantBlocked: false},
		{name: "inherited Solr 10 cloud image", cloudImage: img("10.0.0"), wantBlocked: true},
		{name: "Solr 9 exporter for a Solr 10 cloud", exporterImage: img("9.10.0"), cloudImage: img("10.0.0"), wantBlocked: true},
		{name: "custom exporter image for a Solr 10 cloud", exporterImage: img("my-exporter-3"), cloudImage: img("10.0.0"), wantBlocked: true},
		{name: "Solr 9 exporter for a cloud with a custom tag", exporterImage: img("9.10.0"), cloudImage: img("latest"), wantBlocked: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := unsupportedExporterSolrVersion(tt.exporterImage, tt.cloudImage)
			if got := msg != ""; got != tt.wantBlocked {
				t.Errorf("blocked = %v (message %q), want %v", got, msg, tt.wantBlocked)
			}
		})
	}
}
