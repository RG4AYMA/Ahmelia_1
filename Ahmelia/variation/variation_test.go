package variation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// mockT captura los errores reportados por ForAll sin depender de *testing.T,
// para poder verificar el reporte de fallo como contrato (semilla + situación
// mínima reproducible).
type mockT struct {
	errs []string
}

func (m *mockT) Errorf(format string, args ...interface{}) {
	m.errs = append(m.errs, fmt.Sprintf(format, args...))
}
func (m *mockT) Helper() {}

func TestGenerator_DeterministicWithSeed(t *testing.T) {
	g1 := NewGenerator(42)
	g1.AddDimension("user_state", []string{"a", "b", "c"})
	g1.AddDimension("action", []string{"x", "y"})
	g1.AddTemplate("{user_state} hace {action}", map[string]Dimension{
		"user_state": g1.dimensions["user_state"],
		"action":     g1.dimensions["action"],
	})

	s1 := g1.GenerateN(5)

	g2 := NewGenerator(42)
	g2.AddDimension("user_state", []string{"a", "b", "c"})
	g2.AddDimension("action", []string{"x", "y"})
	g2.AddTemplate("{user_state} hace {action}", map[string]Dimension{
		"user_state": g2.dimensions["user_state"],
		"action":     g2.dimensions["action"],
	})

	s2 := g2.GenerateN(5)

	if len(s1) != len(s2) {
		t.Fatalf("length mismatch: %d vs %d", len(s1), len(s2))
	}
	for i := range s1 {
		if s1[i].Text != s2[i].Text {
			t.Fatalf("mismatch at %d: %q vs %q", i, s1[i].Text, s2[i].Text)
		}
		if s1[i].Seed != s2[i].Seed {
			t.Fatalf("seed mismatch at %d: %d vs %d", i, s1[i].Seed, s2[i].Seed)
		}
	}
}

func TestGenerator_DifferentSeedsDifferentResults(t *testing.T) {
	g1 := NewGenerator(1)
	g1.AddDimension("test", []string{"a", "b", "c"})
	g1.AddTemplate("{test}", map[string]Dimension{
		"test": g1.dimensions["test"],
	})

	// Semillas bien separadas: seeds consecutivos pueden producir secuencias
	// iniciales idénticas con Intn pequeño (propiedad conocida de math/rand).
	g2 := NewGenerator(1000003)
	g2.AddDimension("test", []string{"a", "b", "c"})
	g2.AddTemplate("{test}", map[string]Dimension{
		"test": g2.dimensions["test"],
	})

	s1 := g1.GenerateN(10)
	s2 := g2.GenerateN(10)

	different := false
	for i := range s1 {
		if s1[i].Text != s2[i].Text {
			different = true
			break
		}
	}
	if !different {
		t.Fatal("different seeds should produce different sequences")
	}
}

func TestGenerator_UniqueResults(t *testing.T) {
	g := NewGenerator(123)
	g.AddDimension("color", []string{"rojo", "azul", "verde"})
	g.AddTemplate("color {color}", map[string]Dimension{
		"color": g.dimensions["color"],
	})

	results := g.GenerateN(10)

	seen := make(map[string]bool)
	for _, s := range results {
		if seen[s.Text] {
			t.Fatalf("duplicate: %s", s.Text)
		}
		seen[s.Text] = true
	}
}

func TestGenerator_SeedIsBaseSeed(t *testing.T) {
	// La situación generada reporta la semilla base del generador, no un
	// sorteo adicional: así toda situación es reproducible.
	g := NewGenerator(99)
	g.AddDimension("test", []string{"a", "b", "c"})
	g.AddTemplate("{test}", map[string]Dimension{
		"test": g.dimensions["test"],
	})

	for _, s := range g.GenerateN(3) {
		if s.Seed != 99 {
			t.Fatalf("situation Seed = %d, want 99 (base seed)", s.Seed)
		}
	}
}

func TestForAll_Passes(t *testing.T) {
	dims := map[string]Dimension{
		"user_state": {Name: "user_state", Values: []string{"concentrado", "descansando"}},
		"urgency":    {Name: "urgency", Values: []string{"baja", "media"}},
	}

	ForAll(t, dims, 10, func(s GeneratedSituation) bool {
		// Propiedad: toda situación debe tener texto no vacío
		return s.Text != ""
	})
}

func TestForAll_FailsAndReports(t *testing.T) {
	dims := map[string]Dimension{
		"test": {Name: "test", Values: []string{"a", "b", "c"}},
	}

	// Esta propiedad fallará para la situación cuyo texto sea "c".
	mt := &mockT{}
	ForAllSeed(mt, 7, dims, 5, func(s GeneratedSituation) bool {
		return s.Text != "c"
	})

	if len(mt.errs) != 1 {
		t.Fatalf("expected exactly 1 error report, got %d: %v", len(mt.errs), mt.errs)
	}
	report := mt.errs[0]
	for _, want := range []string{"semilla: 7", "ForAllSeed(t, 7", "template:", "valores:", `texto:    "c"`} {
		if !strings.Contains(report, want) {
			t.Fatalf("failure report missing %q:\n%s", want, report)
		}
	}
}

func TestForAll_SameSeedSameSituations(t *testing.T) {
	// Aceptación T3.9: misma semilla, mismas situaciones.
	dims := map[string]Dimension{
		"estado":   {Name: "estado", Values: []string{"a", "b", "c"}},
		"urgencia": {Name: "urgencia", Values: []string{"x", "y"}},
	}

	collect := func(seed int64) []string {
		var texts []string
		ForAllSeed(&mockT{}, seed, dims, 6, func(s GeneratedSituation) bool {
			texts = append(texts, s.Text)
			return true
		})
		return texts
	}

	run1 := collect(7)
	run2 := collect(7)
	if !reflect.DeepEqual(run1, run2) {
		t.Fatalf("same seed must produce same situations:\n%v\nvs\n%v", run1, run2)
	}

	run3 := collect(8)
	if reflect.DeepEqual(run1, run3) {
		t.Fatal("different seeds should produce different situations for this dims set")
	}
}

func TestSurfaceVariants(t *testing.T) {
	base := "quiero que abras chrome por favor"
	variants := SurfaceVariants(base, 5)

	if len(variants) < 2 {
		t.Fatal("should generate at least 2 variants")
	}
	if variants[0] != base {
		t.Fatal("first variant should be original")
	}
	for _, v := range variants[1:] {
		if v == base {
			t.Fatal("variant should differ from base")
		}
	}
}

func TestSurfaceVariants_Deterministic(t *testing.T) {
	base := "quiero que abras chrome por favor"
	v1 := SurfaceVariants(base, 5)
	v2 := SurfaceVariants(base, 5)
	if !reflect.DeepEqual(v1, v2) {
		t.Fatalf("SurfaceVariants must be deterministic:\n%v\nvs\n%v", v1, v2)
	}
}

func TestDefaultDimensions(t *testing.T) {
	dims := DefaultDimensions()

	expected := []string{"user_state", "time_of_day", "activity", "urgency", "reversibility", "privacy", "device_available"}
	for _, name := range expected {
		dim, ok := dims[name]
		if !ok {
			t.Fatalf("missing dimension: %s", name)
		}
		if len(dim.Values) == 0 {
			t.Fatalf("dimension %s has no values", name)
		}
	}
}

func TestGenerator_EmptyTemplate(t *testing.T) {
	g := NewGenerator(1)
	s := g.Generate()
	if s.Text != "" {
		t.Fatal("empty generator should produce empty situation")
	}
}

func TestGenerator_GenerateNRespectsLimit(t *testing.T) {
	g := NewGenerator(1)
	g.AddDimension("test", []string{"a", "b", "c"})
	g.AddTemplate("{test}", map[string]Dimension{
		"test": g.dimensions["test"],
	})

	results := g.GenerateN(5)
	if len(results) != 3 { // 3 unique values possible
		t.Fatalf("GenerateN(5) returned %d results, want 3", len(results))
	}

	// Pedir más de lo posible con 3 valores únicos
	results = g.GenerateN(10)
	if len(results) > 3 {
		t.Fatalf("GenerateN(10) with 3 unique values returned %d", len(results))
	}
}

func TestSurfaceVariants_RespectsCount(t *testing.T) {
	base := "test"
	variants := SurfaceVariants(base, 1)
	if len(variants) != 1 {
		t.Fatalf("SurfaceVariants(base, 1) = %d, want 1", len(variants))
	}
	if variants[0] != base {
		t.Fatal("first should be base")
	}
}
