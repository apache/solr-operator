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

package v1beta1

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSetAppliedSetting(t *testing.T) {
	status := SolrCloudStatus{}

	status.SetAppliedSetting(ReplicaPlacementFactorySetting, "some-hash")
	assert.Equal(t, map[string]string{ReplicaPlacementFactorySetting: "some-hash"}, status.AppliedSettings)

	status.SetAppliedSetting("otherSetting", "other-hash")
	assert.Equal(t, "some-hash", status.AppliedSettings[ReplicaPlacementFactorySetting], "Setting one setting must not disturb another")

	status.SetAppliedSetting(ReplicaPlacementFactorySetting, "")
	assert.Equal(t, map[string]string{"otherSetting": "other-hash"}, status.AppliedSettings, "An empty hash must remove the setting")

	status.SetAppliedSetting("otherSetting", "")
	assert.Nil(t, status.AppliedSettings, "The last setting removed must leave no map, so that the status matches what Kubernetes stores")
}

func TestSetAppliedSettingDoesNotMutateASharedMap(t *testing.T) {
	status := SolrCloudStatus{AppliedSettings: map[string]string{ReplicaPlacementFactorySetting: "some-hash"}}
	newStatus := SolrCloudStatus{AppliedSettings: status.AppliedSettings}

	newStatus.SetAppliedSetting(ReplicaPlacementFactorySetting, "new-hash")
	assert.Equal(t, "some-hash", status.AppliedSettings[ReplicaPlacementFactorySetting], "The status that the map came from must be left alone")
}
