package postgres

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
)

type PostgresPlugin atlas.PostgresPlugin

func (p *PostgresPlugin) Name() string {
	return "Postgres"
}

func Convert(a *atlas.PostgresPlugin) *PostgresPlugin {
	p := PostgresPlugin(*a)
	return &p
}
