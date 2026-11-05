package dialogue

import (
	"os"
	"strings"
	"testing"
)

// TestRemember_CreaYCuentaMenciones confirma get-or-create y el
// conteo de menciones.
func TestRemember_CreaYCuentaMenciones(t *testing.T) {
	sm := NewSocialMemory()
	sm.Remember("Sebas")
	sm.Remember("Sebas")

	p, ok := sm.Recall("Sebas")
	if !ok {
		t.Fatal("esperaba encontrar a Sebas")
	}
	if p.MentionCount != 2 {
		t.Fatalf("esperaba MentionCount=2, obtuve %d", p.MentionCount)
	}
}

// TestAddFact_AgregaYNoDuplica confirma que agregar el mismo hecho dos
// veces no genera dos entradas — mismo criterio de Remember() en
// ConversationalMemory con TurnCount, pero acá con deduplicación de
// contenido en vez de un contador.
func TestAddFact_AgregaYNoDuplica(t *testing.T) {
	sm := NewSocialMemory()
	sm.AddFact("Sebas", "gusto", "le gusta Unreal")
	sm.AddFact("Sebas", "gusto", "le gusta Unreal")
	sm.AddFact("Sebas", "trabajo", "trabaja en AK Game Studios")

	p, _ := sm.Recall("Sebas")
	if len(p.Facts) != 2 {
		t.Fatalf("esperaba 2 Facts (el repetido no debería duplicar), obtuve %d: %+v", len(p.Facts), p.Facts)
	}
}

// TestFactsByCategory_Filtra confirma el filtro por categoría.
func TestFactsByCategory_Filtra(t *testing.T) {
	sm := NewSocialMemory()
	sm.AddFact("Sebas", "gusto", "le gusta Unreal")
	sm.AddFact("Sebas", "trabajo", "trabaja en AK Game Studios")
	p, _ := sm.Recall("Sebas")

	gustos := p.FactsByCategory("gusto")
	if len(gustos) != 1 || gustos[0].Value != "le gusta Unreal" {
		t.Fatalf("esperaba 1 fact de categoría 'gusto', obtuve: %+v", gustos)
	}
}

// TestRecall_ComparacionLaxa confirma que Recall() encuentra a la
// persona aunque se la nombre parcialmente.
func TestSocialRecall_ComparacionLaxa(t *testing.T) {
	sm := NewSocialMemory()
	sm.Remember("Sebastian Marin")

	if _, ok := sm.Recall("sebastian"); !ok {
		t.Fatal("esperaba encontrar 'Sebastian Marin' buscando solo 'sebastian'")
	}
}

// TestDescribe_EncadenaLosFacts confirma el formato del ejemplo del
// roadmap: nombre + hechos encadenados.
func TestDescribe_EncadenaLosFacts(t *testing.T) {
	sm := NewSocialMemory()
	sm.AddFact("Sebas", "trabajo", "trabaja en AK Game Studios")
	sm.AddFact("Sebas", "gusto", "le gusta Unreal")
	p, _ := sm.Recall("Sebas")

	desc := p.Describe()
	if !strings.Contains(desc, "AK Game Studios") || !strings.Contains(desc, "Unreal") {
		t.Fatalf("esperaba que Describe() incluya ambos hechos, obtuve: %q", desc)
	}
}

// TestDescribe_SoloNombreSinHechos confirma que una persona sin
// ningún Fact todavía da un mensaje honesto, no una lista vacía.
func TestDescribe_SoloNombreSinHechos(t *testing.T) {
	sm := NewSocialMemory()
	sm.Remember("Alguien")
	p, _ := sm.Recall("Alguien")

	desc := p.Describe()
	if !strings.Contains(desc, "todavía no tengo ningún dato") {
		t.Fatalf("esperaba mensaje explícito sin hechos, obtuve: %q", desc)
	}
}

// TestSaveAndLoad_Persiste confirma persistencia real a disco.
func TestSocialSaveAndLoad_Persiste(t *testing.T) {
	path := t.TempDir() + "/social_memory_test.json"
	defer os.Remove(path)

	sm := NewSocialMemory()
	sm.AddFact("Sebas", "trabajo", "trabaja en AK Game Studios")

	if err := sm.Save(path); err != nil {
		t.Fatalf("Save() no debería fallar: %v", err)
	}

	loaded := LoadSocialMemory(path)
	p, ok := loaded.Recall("Sebas")
	if !ok || len(p.Facts) != 1 {
		t.Fatalf("esperaba recuperar a Sebas con su hecho persistido, obtuve: %+v", p)
	}
}

// TestInferFactCategory_DetectaCategorias confirma la heurística de
// categorización por keywords.
func TestInferFactCategory_DetectaCategorias(t *testing.T) {
	cases := map[string]string{
		"trabaja en AK Game Studios": "trabajo",
		"le gusta Unreal":            "gusto",
		"participó en el proyecto X": "proyecto",
		"es mi hermano":              "relacion",
		"tiene 30 años":              "general",
	}
	for input, expected := range cases {
		if got := InferFactCategory(input); got != expected {
			t.Errorf("InferFactCategory(%q) = %q, esperaba %q", input, got, expected)
		}
	}
}

// TestParseAddFactCommand_RequiereSeparadorExplicito confirma la
// decisión central de diseño: sin ":" no hay match, aunque el texto
// mencione a alguien y un hecho.
func TestParseAddFactCommand_RequiereSeparadorExplicito(t *testing.T) {
	name, fact, ok := ParseAddFactCommand("recorda que Sebas: trabaja en AK Game Studios")
	if !ok {
		t.Fatal("esperaba matchear el comando con separador explícito")
	}
	if name != "Sebas" || fact != "trabaja en AK Game Studios" {
		t.Fatalf("esperaba name='Sebas' fact='trabaja en AK Game Studios', obtuve name=%q fact=%q", name, fact)
	}

	if _, _, ok := ParseAddFactCommand("recorda que Sebas trabaja en AK Game Studios"); ok {
		t.Fatal("sin ':' no debería matchear — no hay forma confiable de separar nombre de hecho")
	}
}

// TestParseSocialMemoryQuery_DistintoDelComando confirma que consulta
// y comando de agregar son triggers distintos.
func TestParseSocialMemoryQuery_DistintoDelComando(t *testing.T) {
	if !ParseSocialMemoryQuery("que sabes de sebas") {
		t.Error("esperaba detectar la consulta")
	}
	if ParseSocialMemoryQuery("recorda que sebas: trabaja en AK") {
		t.Error("el comando de agregar no debería disparar la consulta")
	}
}
