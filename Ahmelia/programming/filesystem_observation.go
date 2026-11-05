package programming

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ObservedFile is a direct observation of the current filesystem state for one
// programming source path. It contains no execution result and makes no claim
// about which attempt produced the state.
type ObservedFile struct {
	Source     string
	Path       string
	Exists     bool
	SHA256     string
	ObservedAt time.Time
}

// ObserveCreate inspects the current filesystem state for a CREATE change.
// The expected effect remains the existing MultiFileChange; this contract only
// represents what is observed now.
func ObserveCreate(workspace *ProgrammingWorkspace, change MultiFileChange) (ObservedFile, error) {
	if workspace == nil {
		return ObservedFile{}, fmt.Errorf("programming observation: workspace nil")
	}
	if change.Operation != SourceCreate {
		return ObservedFile{}, fmt.Errorf("programming observation: expected create change")
	}
	path, err := workspace.SourcePath(change.File.Path)
	if err != nil {
		return ObservedFile{}, err
	}
	observed := ObservedFile{
		Source:     "filesystem",
		Path:       change.File.Path,
		ObservedAt: time.Now().UTC(),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return observed, nil
		}
		return ObservedFile{}, fmt.Errorf("programming observation: read %q: %w", change.File.Path, err)
	}
	observed.Exists = true
	sum := sha256.Sum256(data)
	observed.SHA256 = hex.EncodeToString(sum[:])
	return observed, nil
}

// CreateCompatible compares only the expected CREATE state with the current
// filesystem observation. It deliberately does not attribute the state to an
// execution attempt and does not make a recovery decision.
func CreateCompatible(change MultiFileChange, observed ObservedFile) bool {
	if change.Operation != SourceCreate || observed.Source != "filesystem" {
		return false
	}
	if observed.Path != change.File.Path || !observed.Exists {
		return false
	}
	expected := sha256.Sum256([]byte(change.Content))
	return observed.SHA256 == hex.EncodeToString(expected[:])
}

// ObserveFile is a provider-domain filesystem observation primitive used after
// any materialized source mutation whose final path remains stable.
func ObserveFile(root, relativePath string) (ObservedFile, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(relativePath) == "" {
		return ObservedFile{}, fmt.Errorf("programming observation: root/path vacío")
	}
	path := filepath.Join(root, filepath.Clean(relativePath))
	observed := ObservedFile{Source: "filesystem", Path: relativePath, ObservedAt: time.Now().UTC()}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return observed, nil
		}
		return ObservedFile{}, fmt.Errorf("programming observation: read %q: %w", relativePath, err)
	}
	observed.Exists = true
	sum := sha256.Sum256(data)
	observed.SHA256 = hex.EncodeToString(sum[:])
	return observed, nil
}

func EvaluateContentCompatibility(change MultiFileChange, observed ObservedFile) CompatibilityJudgment {
	if observed.Source != "filesystem" || observed.Path != change.File.Path || !observed.Exists {
		return CompatibilityJudgment{}
	}
	expected := change.Content
	if change.Operation == SourceEdit {
		expected = change.NewText
	}
	if change.Operation == SourceWrite || change.Operation == SourceCreate || change.Operation == SourceEdit {
		sum := sha256.Sum256([]byte(expected))
		return CompatibilityJudgment{Compatible: observed.SHA256 == hex.EncodeToString(sum[:])}
	}
	return CompatibilityJudgment{}
}
