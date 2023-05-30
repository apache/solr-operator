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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	solr "github.com/apache/solr-operator/api/v1beta1"
	"io"
	"k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"net/url"
	"strings"
)

type SolrRebalanceRequest struct {
	Nodes []string `json:"nodes,omitempty"`

	WaitForFinalState bool `json:"waitForFinalState,omitempty"`

	Async string `json:"async,omitempty"`
}

func CallCollectionsApiV2(ctx context.Context, cloud *solr.SolrCloud, urlMethod string, urlPath string, urlParams url.Values, body interface{}, response interface{}) (err error) {
	client := noVerifyTLSHttpClient
	if mTLSHttpClient != nil {
		client = mTLSHttpClient
	}

	cloudUrl := solr.InternalURLForCloud(cloud)
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}

	cloudUrl += urlPath
	if len(urlParams) > 0 {
		cloudUrl += "?" + urlParams.Encode()
	}

	resp := &http.Response{}

	var b *bytes.Buffer
	if body != nil {
		b = new(bytes.Buffer)
		if err = json.NewEncoder(b).Encode(body); err != nil {
			return
		}
	}
	var req *http.Request
	if req, err = http.NewRequest(urlMethod, cloudUrl, b); err != nil {
		return err
	}

	// Any custom HTTP headers passed through the Context
	if httpHeaders, hasHeaders := ctx.Value(HTTP_HEADERS_CONTEXT_KEY).(map[string]string); hasHeaders {
		for key, header := range httpHeaders {
			req.Header.Add(key, header)
		}
	}
	req.Header.Set("Accept", "application/json")

	if resp, err = client.Do(req); err != nil {
		return err
	}

	defer resp.Body.Close()

	if err == nil && resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		err = errors.NewServiceUnavailable(fmt.Sprintf("Recieved bad response code of %d from solr with response: %s", resp.StatusCode, string(b)))
		// try to read the response, just in case Solr returned an error that we can read
		json.NewDecoder(bytes.NewReader(b)).Decode(&response)
	}

	if err == nil {
		err = json.NewDecoder(resp.Body).Decode(&response)
	}

	return err
}

func IsNotSupportedApiError(errorBody *SolrErrorResponse) bool {
	return errorBody.Code == 404 &&
		(strings.Contains(errorBody.Message, "Cannot find API for the path") || strings.Contains(errorBody.Message, "no core retrieved for null"))
}
