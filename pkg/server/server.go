package server

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"

	"github.com/infobloxopen/atlas-db/pkg/server/mysql"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
	"github.com/infobloxopen/atlas-db/pkg/server/postgres"
	"github.com/infobloxopen/atlas-db/pkg/server/rds"
)

func ActivePlugin(s *atlas.DatabaseServer) plugin.Plugin {
	if s.Spec.RDS != nil {
		return rds.Convert(s.Spec.RDS)
	}

	if s.Spec.MySQL != nil {
		return mysql.Convert(s.Spec.MySQL)
	}

	if s.Spec.Postgres != nil {
		return postgres.Convert(s.Spec.Postgres)
	}

	return nil
}

