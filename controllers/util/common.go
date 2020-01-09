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