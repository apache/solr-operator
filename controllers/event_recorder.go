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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// noOpEventRecorder is a record.EventRecorder that discards all events.
// It is used as a default so that a reconciler's Recorder is never nil,
// allowing event call-sites to invoke it unconditionally.
type noOpEventRecorder struct{}

var _ record.EventRecorder = noOpEventRecorder{}

func (noOpEventRecorder) Event(_ runtime.Object, _, _, _ string) {}

func (noOpEventRecorder) Eventf(_ runtime.Object, _, _, _ string, _ ...interface{}) {}

func (noOpEventRecorder) AnnotatedEventf(_ runtime.Object, _ map[string]string, _, _, _ string, _ ...interface{}) {
}
