package solving

import (
	"ahmelia/interaction"
	"os"
	"strings"
	"testing"
)

// TestRemember_PrimeraVezValidadaAnotaFirstValidatedAt confirma el dato
// central de este bloque: la primera vez que una Action llega a
// StatusValidated, FirstValidatedAt se completa.
func TestRemember_PrimeraVezValidadaAnotaFirstValidatedAt(t *testing.T) {
	km := NewKnowledgeMemory()
	v := &Validation{Action: "buscar_archivo", Label: "Buscar archivos", Status: StatusValidated, Samples: 5, Confidence: 90}

	km.Remember(v)

	entry, ok := km.Recall("buscar_archivo")
	if !ok {
		t.Fatal("esperaba encontrar la entrada recién registrada")
	}
	if entry.FirstValidatedAt == "" {
		t.Fatal("esperaba que FirstValidatedAt se complete en la primera validación")
	}
}

// TestRemember_NoPisaFirstValidatedAtEnConsultasPosteriores confirma
// que, una vez anotado, FirstValidatedAt NO cambia aunque la Action se
// vuelva a validar más tarde — es "desde cuándo", no "última vez".
func TestRemember_NoPisaFirstValidatedAtEnConsultasPosteriores(t *testing.T) {
	km := NewKnowledgeMemory()
	km.Remember(&Validation{Action: "buscar_archivo", Label: "Buscar archivos", Status: StatusValidated, Samples: 5})

	entry, _ := km.Recall("buscar_archivo")
	first := entry.FirstValidatedAt

	km.Remember(&Validation{Action: "buscar_archivo", Label: "Buscar archivos", Status: StatusValidated, Samples: 20})

	entry, _ = km.Recall("buscar_archivo")
	if entry.FirstValidatedAt != first {
		t.Fatalf("FirstValidatedAt no debería cambiar en una revalidación posterior: antes %q, ahora %q", first, entry.FirstValidatedAt)
	}
	if entry.Samples != 20 {
		t.Fatalf("el resto de la entrada SÍ debería actualizarse (Samples), obtuve %d", entry.Samples)
	}
}

// TestRemember_TentativaNoAnotaFirstValidatedAt confirma que una
// validación StatusTentative NO cuenta como "cruzó el umbral" — solo
// StatusValidated lo hace.
func TestRemember_TentativaNoAnotaFirstValidatedAt(t *testing.T) {
	km := NewKnowledgeMemory()
	km.Remember(&Validation{Action: "crear_carpeta", Label: "Crear carpetas", Status: StatusTentative, Samples: 1})

	entry, ok := km.Recall("crear_carpeta")
	if !ok {
		t.Fatal("esperaba que la entrada se registre igual, aunque sea tentativa")
	}
	if entry.FirstValidatedAt != "" {
		t.Fatalf("una validación tentativa no debería anotar FirstValidatedAt, obtuve %q", entry.FirstValidatedAt)
	}
}

// TestSaveAndLoad_Persiste confirma el ciclo completo de persistencia,
// mismo patrón que LearningState (Bloque 7/8): Save y después Load
// devuelve la misma información.
func TestSaveAndLoad_Persiste(t *testing.T) {
	path := t.TempDir() + "/knowledge_memory_test.json"
	defer os.Remove(path)

	km := NewKnowledgeMemory()
	km.Remember(&Validation{Action: "abrir_programa", Label: "Abrir programas", Status: StatusValidated, Samples: 8, Confidence: 95})

	if err := km.Save(path); err != nil {
		t.Fatalf("Save() no debería fallar: %v", err)
	}

	loaded := LoadKnowledgeMemory(path)
	entry, ok := loaded.Recall("abrir_programa")
	if !ok {
		t.Fatal("esperaba recuperar la entrada persistida después de Load")
	}
	if entry.Confidence != 95 || entry.FirstValidatedAt == "" {
		t.Fatalf("la entrada cargada debería conservar los datos guardados, obtuve: %+v", entry)
	}
}

// TestLoadKnowledgeMemory_SinArchivoArrancaVacia confirma que, sin
// archivo previo, arranca vacía en vez de fallar — mismo criterio que
// LoadLearningState.
func TestLoadKnowledgeMemory_SinArchivoArrancaVacia(t *testing.T) {
	km := LoadKnowledgeMemory("/ruta/que/no/existe/knowledge_memory.json")
	if km == nil || km.Entries == nil {
		t.Fatal("sin archivo, debería devolver una KnowledgeMemory vacía y usable, no nil")
	}
	if len(km.Entries) != 0 {
		t.Fatalf("esperaba 0 entradas sin archivo previo, obtuve %d", len(km.Entries))
	}
}

// TestDescribe_SoloListaValidadasConFecha confirma que Describe() solo
// menciona lo que de verdad cruzó el umbral (StatusValidated +
// FirstValidatedAt), no las tentativas ni sin-datos.
func TestDescribe_SoloListaValidadasConFecha(t *testing.T) {
	km := NewKnowledgeMemory()
	km.Remember(&Validation{Action: "buscar_archivo", Label: "Buscar archivos", Status: StatusValidated, Samples: 5})
	km.Remember(&Validation{Action: "crear_carpeta", Label: "Crear carpetas", Status: StatusTentative, Samples: 1})

	desc := km.Describe()
	if !strings.Contains(desc, "Buscar archivos") {
		t.Errorf("esperaba mencionar la Action validada, obtuve: %q", desc)
	}
	if strings.Contains(desc, "Crear carpetas") {
		t.Errorf("NO esperaba mencionar la tentativa en Describe(), obtuve: %q", desc)
	}
}

// TestParseKnowledgeMemoryQuery_DistintoDeValidator confirma que los
// triggers de Knowledge Memory no se pisan con Learning Validator (48).
func TestParseKnowledgeMemoryQuery_DistintoDeValidator(t *testing.T) {
	if !ParseKnowledgeMemoryQuery(interaction.Analyze("desde cuando confias en buscar archivos", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Knowledge Memory")
	}
	if ParseKnowledgeMemoryQuery(interaction.Analyze("puedo confiar en esa confianza", interaction.Context{})) {
		t.Error("un pedido de Learning Validator no debería disparar Knowledge Memory")
	}
}
