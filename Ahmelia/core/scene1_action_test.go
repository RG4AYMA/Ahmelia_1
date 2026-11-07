package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestScene1AuthorizedActionStillExecutesThroughProcess checks the other half
// of the rest-day contract: the restriction subtracts Ahmelia's own work
// initiative, not the owner's explicit orders. A deterministic production
// fixture materializes a tiny Go project while the restriction is active.
func TestScene1AuthorizedActionStillExecutesThroughProcess(t *testing.T) {
	e := buildTestEngine(t)
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, loc)
	e.sceneNow = func() time.Time { return now }
	e.enableOwnerRestrictionsAt(filepath.Join(t.TempDir(), "owner_restrictions.json"))
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "test", now); err != nil {
		t.Fatalf("restricción: %v", err)
	}
	e.SetResponseCallback(func(string) {})

	e.Process("Crea un programa Go que sume dos números y guárdalo en el proyecto")

	if !e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("la restricción debía seguir activa")
	}
	work, ok := e.workStore.Last()
	if !ok || work.LastActionResult == nil {
		t.Fatalf("la orden autorizada no produjo resultado: work=%+v exists=%t", work, ok)
	}
	root := os.Getenv("AHMELIA_WORKSPACE_ROOT")
	if root == "" {
		t.Fatal("el test no tiene workspace aislado")
	}
	mainPath := ""
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == "main.go" {
			mainPath = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if mainPath == "" {
		t.Fatalf("no se materializó main.go bajo %s", root)
	}
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(content), "func Add") || !strings.Contains(string(content), "func main") {
		t.Fatalf("main.go no corresponde a la orden autorizada:\n%s", content)
	}
	if !strings.Contains(strings.ToLower(work.Objective.Text), "sume") {
		t.Fatalf("el trabajo no corresponde a la orden: %+v", work)
	}
}
