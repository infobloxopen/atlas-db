package server

import (
	atlas "github.com/infobloxopen/atlas/pkg/apis/atlasdb/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type Plugin interface {
	Name() string
}

// PodPlugin is for instances backed by a Pod
type PodPlugin interface {
	podSpec(*atlas.DatabaseServerSpec) corev1.PodSpec
}

func activePlugin(s *atlas.DatabaseServer) Plugin {
	if s.Spec.RDS != nil {
		return convertRDS(s.Spec.RDS)
	}

	if s.Spec.MySQL != nil {
		return convertMySQL(s.Spec.MySQL)
	}

	if s.Spec.Postgres != nil {
		return convertPostgres(s.Spec.Postgres)
	}

	return nil
}
