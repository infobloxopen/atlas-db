package server

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type MySQLPlugin atlas.MySQLPlugin

func (p *MySQLPlugin) Name() string {
	return "MySQL"
}

func convertMySQL(a *atlas.MySQLPlugin) *MySQLPlugin {
	p := MySQLPlugin(*a)
	return &p
}

func (p *MySQLPlugin) podSpec(spec *atlas.DatabaseServerSpec) corev1.PodSpec {
	img := p.Image
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
