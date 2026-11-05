package programming

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/tools"
)

// B96.44.x — Objective-driven synthesis contract.
// The synthesizer is a bounded capability: it transforms a programming
// objective and observed project state into a MultiFilePlan. It never writes,
// builds, tests, authorizes, recovers, or decides global workflow policy.
type CodeObjective struct {
	Raw              string
	Contract         ObjectiveContract
	RecoveryEvidence string
	RecoveryContext  *tools.RecoveryContext
	Language         string
	ProjectID        string
	ProjectRoot      string
	Existing         bool
	Files            []CodeObjectiveFile
}

// ObjectiveContract preserves structured meaning across the programming
// boundary. These values are input semantics, not executable acceptance tests.
type ObjectiveContract struct {
	PrimaryGoal      string
	PrimaryOperation string
	Subject          string
	Workspace        string
	Constraints      []string
	RequiredActions  []string
	Verification     []string
	Recovery         []string
	Confidence       float64
	Provenance       string
}

func (c ObjectiveContract) Clone() ObjectiveContract {
	c.Constraints = append([]string(nil), c.Constraints...)
	c.RequiredActions = append([]string(nil), c.RequiredActions...)
	c.Verification = append([]string(nil), c.Verification...)
	c.Recovery = append([]string(nil), c.Recovery...)
	return c
}

type CodeObjectiveFile struct {
	Path    string
	Content string
}

func (o CodeObjective) Validate() error {
	if strings.TrimSpace(o.Raw) == "" {
		return fmt.Errorf("programming objective: raw vacío")
	}
	if strings.TrimSpace(o.Language) == "" {
		return fmt.Errorf("programming objective: language vacío")
	}
	if strings.TrimSpace(o.ProjectID) == "" {
		return fmt.Errorf("programming objective: project id vacío")
	}
	if strings.TrimSpace(o.ProjectRoot) == "" {
		return fmt.Errorf("programming objective: project root vacío")
	}
	return nil
}

type CodeObjectiveSynthesizer interface {
	Synthesize(context.Context, CodeObjective) (*MultiFilePlan, error)
}
