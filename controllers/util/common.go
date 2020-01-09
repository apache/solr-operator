/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CopyLabelsAndAnnotations copies the labels and annotations from one object to another.
// Additional Labels and Annotations in the 'to' object will not be removed.
// Returns true if there are updates required to the object.
func CopyLabelsAndAnnotations(from, to *metav1.ObjectMeta) (requireUpdate bool) {
	if len(to.Labels) == 0 && len(from.Labels) > 0 {
		to.Labels = map[string]string{}
	}
	for k, v := range from.Labels {
		if to.Labels[k] != v {
			requireUpdate = true
			log.Info("Update Label", "label", k, "newValue", v, "oldValue", to.Labels[k])
			to.Labels[k] = v
		}
	}

	if len(to.Annotations) == 0 && len(from.Annotations) > 0 {
		to.Annotations = map[string]string{}
	}
	for k, v := range from.Annotations {
		if to.Annotations[k] != v {
			requireUpdate = true
			log.Info("Update Annotation", "annotation", k, "newValue", v, "oldValue", to.Annotations[k])
			to.Annotations[k] = v
		}
	}

	return requireUpdate
}
