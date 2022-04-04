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

func TestDeprecatedAdditionalDomains(t *testing.T) {
	ext := &ExternalAddressability{}

	assert.False(t, ext.withDefaults(false), "withDefaults() returned true when nothing should have been changed (no additional domains in either field)")

	ext.AdditionalDomainNames = []string{"t1", "t2"}

	assert.False(t, ext.withDefaults(false), "withDefaults() returned true when nothing should have been changed (no additional domains in the deprecated field)")

	ext.AdditionalDomains = nil

	assert.False(t, ext.withDefaults(false), "withDefaults() returned true when nothing should have been changed (no additional domains in the deprecated field)")

	ext.AdditionalDomains = []string{}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t1", "t2"}, ext.AdditionalDomainNames, "There are no values from additionalDomains to append to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomains = []string{"t1", "t2"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t1", "t2"}, ext.AdditionalDomainNames, "The values are the same between additionalDomains and additionalDomainNames, so nothing should be changed in additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomains = []string{"t1", "t2", "t3"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t1", "t2", "t3"}, ext.AdditionalDomainNames, "The unique values from additionalDomains were not appended to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomains = []string{"t1", "t3"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t1", "t2", "t3"}, ext.AdditionalDomainNames, "The unique values from additionalDomains were not appended to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomains = []string{"t3"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t1", "t2", "t3"}, ext.AdditionalDomainNames, "The unique values from additionalDomains were not appended to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomains = []string{"t4", "t1", "t2", "t3"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t1", "t2", "t4", "t3"}, ext.AdditionalDomainNames, "The unique values from additionalDomains were not appended to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomainNames = nil
	ext.AdditionalDomains = []string{"t4", "t1", "t2", "t3"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t4", "t1", "t2", "t3"}, ext.AdditionalDomainNames, "The unique values from additionalDomains were not appended to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")

	ext.AdditionalDomainNames = []string{}
	ext.AdditionalDomains = []string{"t4", "t1", "t2", "t3"}

	assert.True(t, ext.withDefaults(false), "withDefaults() returned false when the additionalDomains field needs to be removed")
	assert.ElementsMatch(t, []string{"t4", "t1", "t2", "t3"}, ext.AdditionalDomainNames, "The unique values from additionalDomains were not appended to additionalDomainNames")
	assert.Nil(t, ext.AdditionalDomains, "The additionalDomains field was not set to nil")
}
