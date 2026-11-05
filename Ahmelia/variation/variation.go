package variation

import (
	"math/rand"
	"sort"
	"strings"
)

// Dimension representa una dimensión de variación para generar situaciones.
type Dimension struct {
	Name   string
	Values []string
}

// SituationTemplate es una plantilla de situación con placeholders.
type SituationTemplate struct {
	Template string
	Slots    map[string]Dimension
}

// GeneratedSituation es una situación concreta generada.
// Seed es la semilla base del generador que la produjo, de modo que toda
// situación es reproducible con esa semilla (aceptación T3.9: misma semilla,
// mismas situaciones).
type GeneratedSituation struct {
	Seed       int64
	Template   string
	Values     map[string]string
	Text       string
	Properties map[string]string
}

// defaultForAllSeed es la semilla fija que usa ForAll. El spec T3.9 exige
// determinismo: la misma llamada produce las mismas situaciones en cada
// corrida. Para reproducir un fallo con otra semilla, usar ForAllSeed.
const defaultForAllSeed int64 = 1

// Generator genera situaciones estructuradas a partir de dimensiones con semilla fija.
type Generator struct {
	seed       int64
	rand       *rand.Rand
	dimensions map[string]Dimension
	templates  []SituationTemplate
}

// NewGenerator crea un generador con semilla fija para reproducibilidad.
func NewGenerator(seed int64) *Generator {
	return &Generator{
		seed:       seed,
		rand:       rand.New(rand.NewSource(seed)),
		dimensions: make(map[string]Dimension),
		templates:  nil,
	}
}

// AddDimension registra una dimensión de variación.
func (g *Generator) AddDimension(name string, values []string) {
	if len(values) == 0 {
		return
	}
	g.dimensions[name] = Dimension{Name: name, Values: values}
}

// AddTemplate registra una plantilla de situación.
func (g *Generator) AddTemplate(template string, slots map[string]Dimension) {
	g.templates = append(g.templates, SituationTemplate{
		Template: template,
		Slots:    slots,
	})
}

// Generate genera una situación concreta usando la semilla del generador.
func (g *Generator) Generate() GeneratedSituation {
	if len(g.templates) == 0 {
		return GeneratedSituation{Seed: g.seed}
	}

	// Seleccionar plantilla aleatoria
	tpl := g.templates[g.rand.Intn(len(g.templates))]

	// Iteración determinística: ordenar claves para consumo de RNG predecible
	slotNames := make([]string, 0, len(tpl.Slots))
	for k := range tpl.Slots {
		slotNames = append(slotNames, k)
	}
	sort.Strings(slotNames)

	values := make(map[string]string)
	for _, slotName := range slotNames {
		dim := tpl.Slots[slotName]
		if len(dim.Values) > 0 {
			values[slotName] = dim.Values[g.rand.Intn(len(dim.Values))]
		}
	}

	// Rellenar template
	text := tpl.Template
	// Orden determinístico para reemplazo (aunque el orden no afecta el resultado)
	for _, k := range slotNames {
		if v, ok := values[k]; ok {
			text = strings.ReplaceAll(text, "{"+k+"}", v)
		}
	}

	return GeneratedSituation{
		Seed:       g.seed,
		Template:   tpl.Template,
		Values:     values,
		Text:       text,
		Properties: map[string]string{},
	}
}

// GenerateN genera hasta N situaciones únicas. Si las combinaciones posibles
// son menos de N, devuelve todas las únicas que pueda producir.
func (g *Generator) GenerateN(n int) []GeneratedSituation {
	seen := make(map[string]bool)
	var results []GeneratedSituation

	attempts := 0
	maxAttempts := n * 10
	for len(results) < n && attempts < maxAttempts {
		s := g.Generate()
		key := s.Text
		if !seen[key] {
			seen[key] = true
			results = append(results, s)
		}
		attempts++
	}
	return results
}

// ForAll ejecuta una propiedad sobre N situaciones generadas con semilla fija
// (defaultForAllSeed). Si la propiedad falla, reporta la semilla y la situación
// mínima que falla. Es la firma del spec T3.9; para reproducir con otra
// semilla usar ForAllSeed.
func ForAll(t TestingT, dims map[string]Dimension, n int, property func(GeneratedSituation) bool) {
	ForAllSeed(t, defaultForAllSeed, dims, n, property)
}

// ForAllSeed es ForAll con semilla explícita: permite reproducir exactamente
// las situaciones de una corrida anterior a partir de la semilla reportada.
func ForAllSeed(t TestingT, seed int64, dims map[string]Dimension, n int, property func(GeneratedSituation) bool) {
	g := NewGenerator(seed)

	// Registrar dimensiones
	for name, dim := range dims {
		g.AddDimension(name, dim.Values)
	}

	// Plantilla derivada de las dimensiones provistas (nunca hardcodeada):
	// garantiza que la propiedad se ejercite sobre valores reales aunque el
	// llamador use dimensiones con otros nombres.
	template, slots := defaultTemplate(dims)
	g.AddTemplate(template, slots)

	situations := g.GenerateN(n)

	for i, s := range situations {
		if !property(s) {
			t.Errorf(
				"propiedad falsificada en la situación %d de %d\n  semilla: %d (reproducir con ForAllSeed(t, %d, dims, %d, propiedad))\n  situación mínima que falla:\n    template: %q\n    valores:  %v\n    texto:    %q",
				i+1, len(situations), seed, seed, n, s.Template, s.Values, s.Text,
			)
			return
		}
	}
}

// defaultTemplate construye una plantilla con un slot por cada dimensión
// provista, en orden fijo.
func defaultTemplate(dims map[string]Dimension) (string, map[string]Dimension) {
	names := make([]string, 0, len(dims))
	for name := range dims {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	slots := make(map[string]Dimension, len(names))
	for _, name := range names {
		parts = append(parts, "{"+name+"}")
		slots[name] = dims[name]
	}
	return strings.Join(parts, " "), slots
}

// TestingT es la interfaz mínima que necesita ForAll (compatibilidad con *testing.T).
type TestingT interface {
	Errorf(format string, args ...interface{})
	Helper()
}

// surfaceReplacements son las sustituciones de superficie, en orden fijo.
var surfaceReplacements = []struct{ from, to string }{
	{"quiero", "deseo"},
	{"necesito", "requiero"},
	{"puedes", "podrías"},
	{"haz", "realiza"},
	{"dime", "cuéntame"},
	{"explícame", "dime cómo"},
	{"por favor", ""},
	{"ahora mismo", "ya"},
	{"en este momento", "ahora"},
}

// SurfaceVariants genera hasta n variantes de superficie de una misma
// situación semántica, de forma determinista: aplica de a una las
// sustituciones aplicables, en orden fijo, sin aleatoriedad.
// Útil para probar que la misma decisión se toma con distinta redacción (G3).
func SurfaceVariants(baseText string, n int) []string {
	variants := []string{baseText}
	if n <= 1 {
		return variants
	}

	for _, r := range surfaceReplacements {
		if len(variants) >= n {
			break
		}
		if !strings.Contains(baseText, r.from) {
			continue
		}
		variant := strings.TrimSpace(strings.Replace(baseText, r.from, r.to, 1))
		if variant != "" && variant != baseText && !containsVariant(variants, variant) {
			variants = append(variants, variant)
		}
	}

	// Variante de puntuación determinista.
	if len(variants) < n && !strings.HasSuffix(baseText, ".") {
		variant := strings.TrimSpace(baseText) + "."
		if !containsVariant(variants, variant) {
			variants = append(variants, variant)
		}
	}

	return variants
}

func containsVariant(variants []string, v string) bool {
	for _, existing := range variants {
		if existing == v {
			return true
		}
	}
	return false
}

// DefaultDimensions retorna dimensiones estándar para situaciones de la visión.
func DefaultDimensions() map[string]Dimension {
	return map[string]Dimension{
		"user_state": {
			Name:   "user_state",
			Values: []string{"concentrado", "descansando", "en_reunion", "ausente", "posiblemente_frustado"},
		},
		"time_of_day": {
			Name:   "time_of_day",
			Values: []string{"mañana", "mediodía", "tarde", "noche", "madrugada"},
		},
		"activity": {
			Name:   "activity",
			Values: []string{"trabajando", "desayunando", "en_reunion", "descansando", "ociando", "programando"},
		},
		"urgency": {
			Name:   "urgency",
			Values: []string{"baja", "media", "alta", "crítica"},
		},
		"reversibility": {
			Name:   "reversibility",
			Values: []string{"reversible", "compensable", "irreversible"},
		},
		"privacy": {
			Name:   "privacy",
			Values: []string{"privado", "compartido", "público"},
		},
		"device_available": {
			Name:   "device_available",
			Values: []string{"pc", "celular", "fire_tv", "ninguno"},
		},
	}
}
