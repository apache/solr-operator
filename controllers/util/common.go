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
	"reflect"
)

// CopyLabelsAndAnnotations copies the labels and annotations from one object to another.
// Additional Labels and Annotations in the 'to' object will not be removed.
// Returns true if there are updates required to the object.
func CopyLabelsAndAnnotations(from, to *metav1.ObjectMeta) (requireUpdate bool) {
	if len(to.Labels) == 0 && len(from.Labels) > 0 {
		to.Labels = make(map[string]string, len(from.Labels))
	}
	for k, v := range from.Labels {
		if to.Labels[k] != v {
			requireUpdate = true
			log.Info("Update Label", "label", k, "newValue", v, "oldValue", to.Labels[k])
			to.Labels[k] = v
		}
	}

	if len(to.Annotations) == 0 && len(from.Annotations) > 0 {
		to.Annotations = make(map[string]string, len(from.Annotations))
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

func DuplicateLabelsOrAnnotations(from map[string]string) map[string]string {
	to := make(map[string]string, len(from))
	for k, v := range from {
		to[k] = v
	}
	return to
}

func MergeLabelsOrAnnotations(base, additional map[string]string) map[string]string {
	merged := DuplicateLabelsOrAnnotations(base)
	for k, v := range additional {
		if _, alreadyExists := merged[k]; !alreadyExists {
			merged[k] = v
		}
	}
	return merged
}

// DeepEqualWithNils returns a deepEquals call that treats nil and zero-length maps, arrays and slices as the same.
func DeepEqualWithNils(x, y interface{}) bool {
	if (x == nil) != (y == nil) {
		// Make sure that x is not the nil value
		if x == nil {
			x = y
		}
		v := reflect.ValueOf(x)
		switch v.Kind() {
		case reflect.Array:
		case reflect.Map:
		case reflect.Slice:
			return v.Len() == 0
		}
	}
	return reflect.DeepEqual(x, y)
}
