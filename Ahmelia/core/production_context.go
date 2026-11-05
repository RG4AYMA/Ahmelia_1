package core

import "ahmelia/production"

// Compatibility aliases: the Engine historically exposed these names from
// core. The authoritative implementation now lives in the domain-neutral
// Production layer so B96/Programming is only one possible producer.
type ProductionContext = production.ProductionContext
type ProductionArtifact = production.ProductionArtifact
type ProductionProject = production.ProductionProject
