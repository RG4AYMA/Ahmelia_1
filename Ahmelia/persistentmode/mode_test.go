package persistentmode

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDefaultsToCompanion(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "persistent_mode.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	state := store.Current()
	if state.Mode != ModeCompanion {
		t.Fatalf("default mode = %q, quería %q", state.Mode, ModeCompanion)
	}
	if state.ChangedBy != "default" {
		t.Fatalf("changed_by = %q, quería default", state.ChangedBy)
	}
	if !state.ChangedAt.IsZero() {
		t.Fatalf("el default no debe fingir una fecha de cambio: %v", state.ChangedAt)
	}
}

func TestStorePersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "persistent_mode.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	state, err := store.Set(ModeFocus, "owner")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if state.Mode != ModeFocus || state.ChangedBy != "owner" || state.ChangedAt.IsZero() {
		t.Fatalf("estado guardado inesperado: %+v", state)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("el archivo no existe: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Current(); got.Mode != ModeFocus || got.ChangedBy != "owner" {
		t.Fatalf("reload = %+v, quería focus/owner", got)
	}
}

func TestStoreRejectsUnknownModeAndKeepsState(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "persistent_mode.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := store.Set(Mode("turbo"), "owner"); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("set inválido = %v, quería ErrInvalidMode", err)
	}
	if got := store.Current(); got.Mode != ModeCompanion {
		t.Fatalf("el estado cambió tras un modo inválido: %+v", got)
	}
}

func TestStoreFailsClosedOnMalformedFile(t *testing.T) {
	cases := []string{
		"{no es json",
		`{"mode":"turbo","changed_by":"owner"}`,
		`{"mode":"","changed_by":"owner"}`,
	}
	for _, raw := range cases {
		path := filepath.Join(t.TempDir(), "persistent_mode.json")
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		store := NewStore(path)
		if err := store.Load(); err == nil {
			t.Fatalf("%q debía fallar", raw)
		}
		if got := store.Current(); got.Mode != ModeCompanion {
			t.Fatalf("%q dejó estado %+v; debía conservar el default", raw, got)
		}
	}
}

func TestProfileForEachMode(t *testing.T) {
	cases := []struct {
		mode     Mode
		impulses bool
		quota    int
	}{
		{ModeFocus, true, 1},
		{ModeExplore, true, 1},
		{ModeCompanion, true, 1},
		{ModeRest, false, 0},
	}
	for _, tc := range cases {
		p := ProfileFor(tc.mode)
		if p.Label == "" || p.Description == "" {
			t.Fatalf("%s sin etiqueta o descripción: %+v", tc.mode, p)
		}
		if p.Impulses != tc.impulses || p.PlannerQuota != tc.quota {
			t.Fatalf("%s = impulses %t quota %d, quería %t/%d", tc.mode, p.Impulses, p.PlannerQuota, tc.impulses, tc.quota)
		}
	}
	if got := ProfileFor(ModeUnknown); got.PlannerQuota != ProfileFor(DefaultMode).PlannerQuota {
		t.Fatalf("un modo desconocido no debe ampliar el cupo: %+v", got)
	}
	if !ProfileFor(DefaultMode).Impulses {
		t.Fatal("el modo por defecto no debe apagar los impulsos")
	}
}

func TestModesInTextAliases(t *testing.T) {
	cases := []struct {
		text string
		want []Mode
	}{
		{"cambia el modo a concentracion", []Mode{ModeFocus}},
		{"pon el modo en compania", []Mode{ModeCompanion}},
		{"modo trabajo", []Mode{ModeFocus}},
		{"quiero descanso", []Mode{ModeRest}},
		{"exploracion", []Mode{ModeExplore}},
		{"cambia el modo en que trabajo", nil},
		{"no hables de trabajo", nil},
	}
	for _, tc := range cases {
		got := ModesInText(tc.text)
		if len(got) != len(tc.want) {
			t.Fatalf("%q -> %v, quería %v", tc.text, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%q -> %v, quería %v", tc.text, got, tc.want)
			}
		}
	}
}

func TestParseModeWord(t *testing.T) {
	cases := map[string]Mode{
		"focus":       ModeFocus,
		"compañía":    ModeCompanion,
		"descanso":    ModeRest,
		"exploración": ModeExplore,
		"turbo":       ModeUnknown,
	}
	for raw, want := range cases {
		if got := ParseModeWord(raw); got != want {
			t.Fatalf("%q -> %q, quería %q", raw, got, want)
		}
	}
}
