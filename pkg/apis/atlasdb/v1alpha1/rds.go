package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

func (i *RDSInstance) needsPod() bool {
	return false
}

// podSpec satisfies activeInstance interface but doesn't return anything meaningful
func (i *RDSInstance) podSpec(spec *DatabaseServerSpec) corev1.PodSpec {
	return corev1.PodSpec{}
}
