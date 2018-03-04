package server

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewPod creates a new Pod for a DatabaseServer resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DatabaseServer resource that 'owns' it.
func NewPod(s *atlas.DatabaseServer) *corev1.Pod {

	plugin, ok := ActivePlugin(s).(PodPlugin)
	if !ok {
		return nil
	}

	labels := map[string]string{
		"controller": s.Name,
	}
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(s, schema.GroupVersionKind{
					Group:   atlas.SchemeGroupVersion.Group,
					Version: atlas.SchemeGroupVersion.Version,
					Kind:    "DatabaseServer",
				}),
			},
		},
		Spec: plugin.podSpec(&s.Spec),
	}
	return p
}
