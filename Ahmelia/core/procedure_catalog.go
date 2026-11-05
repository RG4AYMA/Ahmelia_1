package core

import (
	"ahmelia/capability"
)

// registryCatalog adapta capability.Registry a procedure.Catalog.
type registryCatalog struct {
	r *capability.Registry
}

func (c registryCatalog) Known(id, op string) bool {
	cp, ok := c.r.Get(capability.ID(id))
	return ok && cp.Action == op
}

// KnownProcedureCapability verifica si una capability/operation está registrada
// en el catálogo de capabilities del Engine.
func (e *Engine) KnownProcedureCapability(capabilityID, operation string) bool {
	if e.capabilityRegistry == nil {
		return false
	}
	cat := registryCatalog{r: e.capabilityRegistry}
	return cat.Known(capabilityID, operation)
}
