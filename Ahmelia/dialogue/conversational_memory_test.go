package dialogue

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestTouch_PrimeraVezQuedaActiva confirma que tocar un tema nuevo lo
// crea como Active, con TurnCount=1.
func TestTouch_PrimeraVezQuedaActiva(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("Unity")

	rec, ok := cm.Recall("Unity")
	if !ok {
		t.Fatal("esperaba encontrar el tema recién tocado")
	}
	if rec.Status != TopicActive || rec.TurnCount != 1 {
		t.Fatalf("esperaba Status=activo, TurnCount=1, obtuve Status=%q TurnCount=%d", rec.Status, rec.TurnCount)
	}
}

// TestTouch_DegradaElActivoAnteriorAPendiente confirma el
// comportamiento central: solo puede haber un tema Active a la vez —
// tocar uno nuevo degrada al anterior a Pending, no lo cierra.
func TestTouch_DegradaElActivoAnteriorAPendiente(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("Unity")
	cm.Touch("musica")

	unity, _ := cm.Recall("Unity")
	musica, _ := cm.Recall("musica")

	if unity.Status != TopicPending {
		t.Fatalf("esperaba que Unity pase a pendiente al tocar otro tema, obtuve %q", unity.Status)
	}
	if musica.Status != TopicActive {
		t.Fatalf("esperaba que musica quede activa, obtuve %q", musica.Status)
	}
}

// TestTouch_TocarDeNuevoSumaTurnCountYReactiva confirma que retomar un
// tema pendiente lo vuelve a poner Active y suma al TurnCount
// acumulado — no lo resetea.
func TestTouch_TocarDeNuevoSumaTurnCountYReactiva(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("Unity")
	cm.Touch("musica")
	cm.Touch("Unity")

	unity, _ := cm.Recall("Unity")
	if unity.Status != TopicActive {
		t.Fatalf("esperaba que Unity vuelva a estar activa, obtuve %q", unity.Status)
	}
	if unity.TurnCount != 2 {
		t.Fatalf("esperaba TurnCount=2 (acumulado, no reseteado), obtuve %d", unity.TurnCount)
	}
}

// TestClose_MarcaCerradoYNoAfectaOtros confirma que Close() es
// explícito — no pasa solo por tocar otro tema (eso es Pending, no
// Closed).
func TestClose_MarcaCerradoYNoAfectaOtros(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("Unity")

	if !cm.Close("Unity") {
		t.Fatal("Close() debería confirmar que encontró el tema")
	}
	rec, _ := cm.Recall("Unity")
	if rec.Status != TopicClosed {
		t.Fatalf("esperaba Status=cerrado, obtuve %q", rec.Status)
	}
	if cm.Close("tema que no existe") {
		t.Fatal("Close() sobre un tema inexistente debería devolver false")
	}
}

// TestPendingTopics_SoloListaPendientes confirma que PendingTopics()
// no mezcla Active ni Closed.
func TestPendingTopics_SoloListaPendientes(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("Unity")
	cm.Touch("musica") // Unity -> pendiente
	cm.Close("musica") // musica -> cerrado (no pendiente)

	pending := cm.PendingTopics()
	if len(pending) != 1 || pending[0].Name != "Unity" {
		t.Fatalf("esperaba solo 'Unity' como pendiente, obtuve: %+v", pending)
	}
}

// TestRecall_ComparacionLaxa confirma que Recall() encuentra el tema
// aunque el usuario lo nombre distinto (substring en cualquier
// dirección) — mismo criterio que topicWasDiscussed en dialogue_manager.go.
func TestRecall_ComparacionLaxa(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("Unreal Engine")

	if _, ok := cm.Recall("unreal"); !ok {
		t.Fatal("esperaba encontrar 'Unreal Engine' buscando solo 'unreal'")
	}
}

// TestSaveAndLoad_Persiste confirma el ciclo completo, mismo patrón
// que KnowledgeMemory (Bloque 49).
func TestSaveAndLoad_Persiste(t *testing.T) {
	path := t.TempDir() + "/conversational_memory_test.json"
	defer os.Remove(path)

	cm := NewConversationalMemory()
	cm.Touch("Unity")
	cm.SetNote("Unity", "vimos Blueprints, falta Materials")

	if err := cm.Save(path); err != nil {
		t.Fatalf("Save() no debería fallar: %v", err)
	}

	loaded := LoadConversationalMemory(path)
	rec, ok := loaded.Recall("Unity")
	if !ok {
		t.Fatal("esperaba recuperar el tema persistido")
	}
	if rec.LastNote != "vimos Blueprints, falta Materials" {
		t.Fatalf("esperaba que LastNote se conserve, obtuve %q", rec.LastNote)
	}
}

// TestLoadConversationalMemory_SinArchivoArrancaVacia confirma que,
// sin archivo previo, no rompe.
func TestLoadConversationalMemory_SinArchivoArrancaVacia(t *testing.T) {
	cm := LoadConversationalMemory("/ruta/que/no/existe.json")
	if cm == nil || cm.Topics == nil || len(cm.Topics) != 0 {
		t.Fatal("sin archivo previo, debería devolver una memoria vacía y usable")
	}
}

// TestRecentAndRemoteTopics_SeparaPorAntiguedad confirma la
// distinción reciente/remoto que pide el roadmap, manipulando
// LastTouched directamente para simular un tema de "hace más de un día".
func TestRecentAndRemoteTopics_SeparaPorAntiguedad(t *testing.T) {
	cm := NewConversationalMemory()
	cm.Touch("hoy")
	cm.Topics["viejo"] = &TopicRecord{
		Name:        "viejo",
		Status:      TopicPending,
		LastTouched: time.Now().Add(-72 * time.Hour).Format(time.RFC3339),
	}

	recent := cm.RecentTopics()
	remote := cm.RemoteTopics()

	if len(recent) != 1 || recent[0].Name != "hoy" {
		t.Fatalf("esperaba solo 'hoy' como reciente, obtuve: %+v", recent)
	}
	if len(remote) != 1 || remote[0].Name != "viejo" {
		t.Fatalf("esperaba solo 'viejo' como remoto, obtuve: %+v", remote)
	}
}

// TestDescribe_SinRegistroNoRompe confirma que Describe() sobre un
// *TopicRecord nil da un mensaje explícito, no un panic.
func TestDescribe_SinRegistroNoRompe(t *testing.T) {
	var rec *TopicRecord
	desc := rec.Describe()
	if !strings.Contains(desc, "No tengo ningún registro") {
		t.Fatalf("esperaba mensaje explícito sobre nil, obtuve %q", desc)
	}
}

// TestParseConversationalMemoryQuery_DetectaTriggers confirma los
// triggers principales.
func TestParseConversationalMemoryQuery_DetectaTriggers(t *testing.T) {
	if !ParseConversationalMemoryQuery("donde lo dejamos con unity") {
		t.Error("esperaba detectar 'donde lo dejamos con'")
	}
	if !ParseConversationalMemoryQuery("que temas tenemos pendientes") {
		t.Error("esperaba detectar 'que temas tenemos pendientes'")
	}
}

// TestExtractMemoryTarget_ConYSinTema confirma que triggers con tema
// devuelven el tema, y triggers sin tema devuelven vacío.
func TestExtractMemoryTarget_ConYSinTema(t *testing.T) {
	if got := ExtractMemoryTarget("donde lo dejamos con unity"); got != "unity" {
		t.Errorf("esperaba 'unity', obtuve %q", got)
	}
	if got := ExtractMemoryTarget("que temas tenemos pendientes"); got != "" {
		t.Errorf("esperaba vacío para un trigger sin tema, obtuve %q", got)
	}
}
