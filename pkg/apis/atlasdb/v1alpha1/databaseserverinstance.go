package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type activeInstance interface {
	needsPod() bool
	podSpec(*DatabaseServerSpec) corev1.PodSpec
}

func (i *DatabaseServerInstance) instance() activeInstance {
	//TODO: there must be a better way??

	if i.RDS != nil {
		return i.RDS
	}

	if i.MySQL != nil {
		return i.MySQL
	}

	if i.Postgres != nil {
		return i.Postgres
	}

	return nil
}
