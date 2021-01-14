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

func CheckForCollectionsApiError(action string, header SolrResponseHeader) (hasError bool, err error) {
	if header.Status > 0 {
		hasError = true
		err = APIError{
			Detail: fmt.Sprintf("Error occured while calling the Collections api for action=%s", action),
			Status: header.Status,
		}
	}
	return hasError, err
}

func CollectionsAPIError(action string, responseStatus int) error {
	return APIError{
		Detail: fmt.Sprintf("Error occured while calling the Collections api for action=%s", action),
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
