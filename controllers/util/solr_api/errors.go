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

import "fmt"

func CheckForCollectionsApiError(action string, header SolrResponseHeader, errorBody *SolrErrorResponse) (apiUnsupported bool, err error) {
	if errorBody != nil {
		err = *errorBody
		apiUnsupported = IsNotSupportedApiError(errorBody)
	} else if header.Status > 0 {
		err = APIError{
			Detail: fmt.Sprintf("Error occurred while calling the Collections api for action=%s", action),
			Status: header.Status,
		}
	}
	return
}

func CollectionsAPIError(action string, responseStatus int) error {
	return APIError{
		Detail: fmt.Sprintf("Error occurred while calling the Collections api for action=%s", action),
		Status: responseStatus,
	}
}

type APIError struct {
	Detail string
	Status int // API-specific error code
}

func (e APIError) Error() string {
	if e.Status == 0 {
		return e.Detail
	}
	return fmt.Sprintf("Solr response status: %d. %s", e.Status, e.Detail)
}

type SolrErrorResponse struct {
	Metadata SolrErrorMetadata `json:"metadata,omitempty"`

	Message string `json:"msg,omitempty"`

	Code int `json:"code,omitempty"`
}

type SolrErrorMetadata struct {
	ErrorClass string `json:"error-class,omitempty"`

	RootErrorClass string `json:"root-error-class,omitempty"`
}

func (e SolrErrorResponse) Error() string {
	return fmt.Sprintf("Error returned from Solr API: %d. %s", e.Code, e.Message)
}
