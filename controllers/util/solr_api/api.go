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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/motemen/go-loghttp"
	"golang.org/x/oauth2"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"net/url"
)

const (
	TOKEN_SOURCE_CONTEXT_KEY = "TOKEN_SOURCE"
	HTTP_HEADERS_CONTEXT_KEY = "HTTP_HEADERS"
)

// Used to call a Solr pod over https when using a self-signed cert
// It's "insecure" but is only used for internal communication, such as getting cluster status
// so if you're worried about this, don't use a self-signed cert
var noVerifyTLSHttpClient *http.Client
var mTLSHttpClient *http.Client

func SetNoVerifyTLSHttpClient(client *http.Client) {
	noVerifyTLSHttpClient = client
}

func SetMTLSHttpClient(client *http.Client) {
	mTLSHttpClient = client
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
	// Possible states can be found here: https://github.com/apache/solr/blob/releases/lucene-solr%2F8.8.1/solr/solrj/src/java/org/apache/solr/client/solrj/response/RequestStatusState.java
	AsyncState string `json:"state"`

	Message string `json:"msg"`
}

func CallCollectionsApi(ctx context.Context, cloud *solr.SolrCloud, urlParams url.Values, response interface{}) (err error) {
	cloudUrl := solr.InternalURLForCloud(cloud)

	client := noVerifyTLSHttpClient
	if mTLSHttpClient != nil {
		client = mTLSHttpClient
	}

	// if the supplied Context has a token source, then use that to make our call to Solr
	if tokenSource, hasTokenSource := ctx.Value(TOKEN_SOURCE_CONTEXT_KEY).(*oauth2.TokenSource); hasTokenSource {
		// Get an Oidc client but using our existing client as the base
		client = oauth2.NewClient(context.WithValue(ctx, oauth2.HTTPClient, client), *tokenSource)
	}

	urlParams.Set("wt", "json")

	cloudUrl = cloudUrl + "/solr/admin/collections?" + urlParams.Encode()

	resp := &http.Response{}

	req, err := http.NewRequest("GET", cloudUrl, nil)

	// Any custom HTTP headers passed through the Context
	if httpHeaders, hasHeaders := ctx.Value(HTTP_HEADERS_CONTEXT_KEY).(map[string]string); hasHeaders {
		for key, header := range httpHeaders {
			req.Header.Add(key, header)
		}
	}

	if resp, err = client.Do(req); err != nil {
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

func init() {
	// setup an http client that can talk to Solr pods using untrusted, self-signed certs
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	// TODO: nocommit ~ useful for debugging Oidc activity
	transportWithLogging := &loghttp.Transport{
		Transport: customTransport,
		LogRequest: func(req *http.Request) {
			fmt.Printf("\n\nREQUEST: %s %s %+v\n", req.Method, req.URL, req.Header)
		},
		LogResponse: func(resp *http.Response) {
			fmt.Printf("RESPONSE: %+v\n", resp.Status)
		},
	}

	SetNoVerifyTLSHttpClient(&http.Client{Transport: transportWithLogging})
}
