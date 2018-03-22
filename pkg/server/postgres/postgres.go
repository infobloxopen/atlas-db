package postgres

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	plugin "github.com/infobloxopen/atlas-db/pkg/server/plugin"
)

type PostgresPlugin atlas.PostgresPlugin

func Convert(a *atlas.PostgresPlugin) *PostgresPlugin {
	p := PostgresPlugin(*a)
	return &p
}

func (p *PostgresPlugin) Name() string {
	return "Postgres"
}

func (p *PostgresPlugin) Dsn(su, pw string, s *atlas.DatabaseServer) string {
	return ""
}

func (p *PostgresPlugin) SyncDatabase(db *atlas.Database, dsn string) error {
	return nil
}

func (p *PostgresPlugin) DatabasePlugin() plugin.DatabasePlugin {
	return p
}

