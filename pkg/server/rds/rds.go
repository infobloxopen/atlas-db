package rds

import (
	"strings"

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	"github.com/infobloxopen/atlas-db/pkg/server/mysql"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
	"github.com/infobloxopen/atlas-db/pkg/server/postgres"
)

type RDSPlugin atlas.RDSPlugin

func Convert(a *atlas.RDSPlugin) *RDSPlugin {
	p := RDSPlugin(*a)
	return &p
}

func (p *RDSPlugin) Name() string {
	return "RDS"
}

func (p *RDSPlugin) DatabasePlugin() plugin.DatabasePlugin {
	switch strings.ToLower(p.Engine) {
	case "mysql":
		return &mysql.MySQLPlugin{}
	case "postgres":
		return &postgres.PostgresPlugin{}
	}
	return nil
}

func (p *RDSPlugin) Dsn(userName string, password string, db *atlas.Database, s *atlas.DatabaseServer) string {
	return p.DatabasePlugin().Dsn(userName, password, nil, s)
}

func (p *RDSPlugin) SyncCloud(key string, s *atlas.DatabaseServer) error {
	return nil
}
