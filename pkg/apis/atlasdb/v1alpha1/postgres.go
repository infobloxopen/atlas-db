package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

func (i *PostgresInstance) needsPod() bool {
	return true
}

func (i *PostgresInstance) podSpec(spec *DatabaseServerSpec) corev1.PodSpec {
	return corev1.PodSpec{
		Volumes: spec.Volumes,
		Containers: []corev1.Container{
			{
				Name:  "server",
				Image: i.Image,
				Env: []corev1.EnvVar{
					{
						Name:      "MYSQL_ROOT_PASSWORD",
						Value:     spec.RootPassword,
						ValueFrom: spec.RootPasswordFrom,
					},
				},
			},
		},
	}
}
