package server

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type PostgresPlugin atlas.PostgresPlugin

func (p *PostgresPlugin) Name() string {
	return "Postgres"
}

func convertPostgres(a *atlas.PostgresPlugin) *PostgresPlugin {
	p := PostgresPlugin(*a)
	return &p
}

func (p *PostgresPlugin) podSpec(spec *atlas.DatabaseServerSpec) corev1.PodSpec {
	return corev1.PodSpec{
		Volumes: spec.Volumes,
		Containers: []corev1.Container{
			{
				Name:  "server",
				Image: p.Image,
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
