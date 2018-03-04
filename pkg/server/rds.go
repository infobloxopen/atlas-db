package server

import (
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
)

type RDSPlugin atlas.RDSPlugin

func (p *RDSPlugin) Name() string {
	return "RDS"
}

func convertRDS(a *atlas.RDSPlugin) *RDSPlugin {
	p := RDSPlugin(*a)
	return &p
}
