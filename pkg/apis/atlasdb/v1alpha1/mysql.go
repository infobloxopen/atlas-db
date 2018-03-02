package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

func (i *MySQLInstance) needsPod() bool {
	return true
}

func (i *MySQLInstance) podSpec(spec *DatabaseServerSpec) corev1.PodSpec {
	img := i.Image
	if img == "" {
		img = "mysql:5.6"
	}
	return corev1.PodSpec{
		Volumes: spec.Volumes,
		Containers: []corev1.Container{
			{
				Name:  "server",
				Image: img,
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
