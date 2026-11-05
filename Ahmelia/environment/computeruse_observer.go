package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/computeruse"
)

// ComputerUseSnapshot exposes the current digital world observation through
// the provider-neutral Computer Use boundary. It intentionally contains only
// facts the environment manager can currently establish; screenshots and
// application-specific semantics remain the responsibility of concrete
// adapters.
func (m *Manager) ComputerUseSnapshot() (computeruse.Snapshot, bool) {
	world, ok := m.WorldSnapshot()
	if !ok || world == nil {
		return computeruse.Snapshot{}, false
	}

	ui := map[string]string{
		"app":               world.AppName,
		"activity":          string(world.Activity),
		"category":          string(world.Category),
		"project":           world.Project,
		"file":              world.File,
		"workspace":         world.Workspace,
		"workspace_current": fmt.Sprintf("%t", world.WorkspaceIsCurrent),
		"unsaved":           fmt.Sprintf("%t", world.HasUnsavedChanges),
	}
	return computeruse.Snapshot{
		ID:                worldFingerprint(world),
		CapturedAt:        time.Now(),
		ActiveApplication: world.AppName,
		ActiveWindow:      world.File,
		UI:                ui,
		Fingerprint:       worldFingerprint(world),
		Source:            "environment.world_model",
	}, true
}

func worldFingerprint(s *WorldSnapshot) string {
	keys := make([]string, 0, 8)
	values := map[string]string{
		"app":               s.AppName,
		"activity":          string(s.Activity),
		"category":          string(s.Category),
		"project":           s.Project,
		"file":              s.File,
		"workspace":         s.Workspace,
		"workspace_current": fmt.Sprintf("%t", s.WorkspaceIsCurrent),
		"unsaved":           fmt.Sprintf("%t", s.HasUnsavedChanges),
	}
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, values[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
