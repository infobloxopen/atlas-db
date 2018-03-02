package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewPod creates a new Pod for a DatabaseServer resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DatabaseServer resource that 'owns' it.
func (server *DatabaseServer) NewPod() *corev1.Pod {
	if !server.Spec.instance().needsPod() {
		return nil
	}

	labels := map[string]string{
		"controller": server.Name,
	}
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(server, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "DatabaseServer",
				}),
			},
		},
		Spec: server.Spec.instance().podSpec(&server.Spec),
	}
	return p
}
