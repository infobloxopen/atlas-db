package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type activeInstance interface {
	needsPod() bool
	podSpec(*DatabaseServerSpec) corev1.PodSpec
}

func (i *DatabaseServerInstance) instance() activeInstance {
	members := []activeInstance{
		i.RDS,
		i.MySQL,
		i.Postgres,
	}

	for _, m := range members {
		if m != nil {
			return m
		}
	}
	return nil
}
