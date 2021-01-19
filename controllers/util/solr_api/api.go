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

package solr_api

import (
	"encoding/json"
	"fmt"
	solr "github.com/apache/lucene-solr-operator/api/v1beta1"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"net/url"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Used to call a Solr pod over https when using a self-signed cert
// It's "insecure" but is only used for internal communication, such as getting cluster status
// so if you're worried about this, don't use a self-signed cert
var noVerifyTLSHttpClient *http.Client

func SetNoVerifyTLSHttpClient(client *http.Client) {
	noVerifyTLSHttpClient = client
}

type SolrAsyncResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	RequestId string `json:"requestId"`

	// +optional
	Status SolrAsyncStatus `json:"status"`
}

type SolrResponseHeader struct {
	Status int `json:"status"`

	QTime int `json:"QTime"`
}

type SolrAsyncStatus struct {
	// Possible states can be found here: https://github.com/apache/lucene-solr/blob/1d85cd783863f75cea133fb9c452302214165a4d/solr/solrj/src/java/org/apache/solr/client/solrj/response/RequestStatusState.java
	AsyncState string `json:"state"`

	Message string `json:"msg"`
}

// Used to call a Solr pod over https when using a self-signed cert
// It's "insecure" but is only used for internal communication, such as getting cluster status
// so if you're worried about this, don't use a self-signed cert
//var noVerifyTLSHttpClient *http.Client = nil

var logger = ctrl.Log.WithName("controllers").WithName("SolrCloud")

func CallCollectionsApi(cloud *solr.SolrCloud, urlParams url.Values, response interface{}) (err error) {
	cloudUrl := solr.InternalURLForCloud(cloud)

	client := http.DefaultClient
	if cloud.Spec.SolrTLS != nil && cloud.Spec.SolrTLS.AutoCreate != nil {
		client = noVerifyTLSHttpClient
	}

	urlParams.Set("wt", "json")

	cloudUrl = cloudUrl + "/solr/admin/collections?" + urlParams.Encode()

	resp := &http.Response{}
	if resp, err = client.Get(cloudUrl); err != nil {
		return err
	}

	defer resp.Body.Close()

	if err == nil && resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		err = errors.NewServiceUnavailable(fmt.Sprintf("Recieved bad response code of %d from solr with response: %s", resp.StatusCode, string(b)))
	}

	if err == nil {
		json.NewDecoder(resp.Body).Decode(&response)
	}

	return err
}
