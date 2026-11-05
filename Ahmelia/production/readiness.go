package production

import (
	"fmt"
	"sort"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/tools"
)

// Severity identifies the impact of a production-readiness finding.
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warning"
	SeverityError Severity = "error"
)

// Finding is one auditable readiness result.
type Finding struct {
	ID       string
	Severity Severity
	Area     string
	Message  string
}

// Report is the deterministic output of the B94.9 architecture audit.
type Report struct {
	Ready    bool
	Findings []Finding
}

// Audit inspects the B94 capability/tool contracts without executing tools.
// Missing concrete tools are warnings, not errors: B94.9 validates that the
// architecture is ready to accept them at scale rather than pretending that
// every future environment already exists.
func Audit(capabilities *capability.Registry, toolRegistry *tools.Registry) Report {
	report := Report{Ready: true}

	if capabilities == nil {
		report.add("CAP-REGISTRY", SeverityError, "architecture", "CapabilityRegistry no está configurado")
		return report.finish()
	}
	if toolRegistry == nil {
		report.add("TOOL-REGISTRY", SeverityError, "architecture", "Tool Registry no está configurado")
		return report.finish()
	}

	caps := capabilities.List()
	registeredTools := toolRegistry.List()

	if len(caps) == 0 {
		report.add("CAP-EMPTY", SeverityWarn, "extensibility", "no hay capabilities registradas; el contrato está disponible pero aún no hay catálogo operativo")
	}
	if len(registeredTools) == 0 {
		report.add("TOOL-EMPTY", SeverityWarn, "extensibility", "no hay tools concretas registradas; la infraestructura está lista para conectarlas")
	}

	capByID := make(map[capability.ID]capability.Capability, len(caps))
	for _, c := range caps {
		capByID[c.ID] = c
		if err := c.Validate(); err != nil {
			report.add("CAP-INVALID-"+string(c.ID), SeverityError, "architecture", err.Error())
		}
		if !c.Permission.IsValid() {
			report.add("CAP-PERM-"+string(c.ID), SeverityError, "security", fmt.Sprintf("capability %q tiene Permission inválido", c.ID))
		}
		auth, err := autonomy.AuthorizeCapability(c, autonomy.ApprovalNone)
		if err != nil {
			report.add("AUTH-CONTRACT-"+string(c.ID), SeverityError, "security", fmt.Sprintf("no se puede evaluar autorización: %v", err))
		} else if auth.Allowed && !autonomy.CanExecute(auth.Authority) {
			report.add("AUTH-BYPASS-"+string(c.ID), SeverityError, "security", "la autorización reporta Allowed=true sin AuthorityExecute")
		}
	}

	covered := make(map[capability.ID]int)
	for _, t := range registeredTools {
		if t == nil {
			report.add("TOOL-NIL", SeverityError, "architecture", "el registry contiene una Tool nil")
			continue
		}
		if t.ID() == "" {
			report.add("TOOL-ID", SeverityError, "architecture", "una Tool registrada no tiene ID")
		}
		if t.Executor() == nil {
			report.add("TOOL-EXEC-"+string(t.ID()), SeverityError, "architecture", fmt.Sprintf("tool %q no tiene Executor", t.ID()))
		}
		for _, id := range t.Capabilities() {
			covered[id]++
			if _, ok := capByID[id]; !ok {
				report.add("TOOL-ORPHAN-"+string(t.ID())+"-"+string(id), SeverityWarn, "extensibility", fmt.Sprintf("tool %q referencia capability %q que no está en el Registry", t.ID(), id))
			}
		}
	}

	for _, c := range caps {
		if covered[c.ID] == 0 {
			report.add("CAP-UNCOVERED-"+string(c.ID), SeverityWarn, "extensibility", fmt.Sprintf("capability %q aún no tiene Tool concreta; esto es aceptable para capacidades declarativas futuras", c.ID))
		}
	}

	return report.finish()
}

func (r *Report) add(id string, severity Severity, area, message string) {
	r.Findings = append(r.Findings, Finding{ID: id, Severity: severity, Area: area, Message: message})
	if severity == SeverityError {
		r.Ready = false
	}
}

func (r Report) finish() Report {
	sort.Slice(r.Findings, func(i, j int) bool {
		if r.Findings[i].Severity != r.Findings[j].Severity {
			return severityRank(r.Findings[i].Severity) < severityRank(r.Findings[j].Severity)
		}
		return r.Findings[i].ID < r.Findings[j].ID
	})
	return r
}

func severityRank(s Severity) int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarn:
		return 1
	default:
		return 2
	}
}
