package solving

import (
	"strings"
	"testing"

	"ahmelia/gateway"
)

// TestRetrieve_DiscovererNilNoRompeYNecesitaFetch confirma el
// criterio defensivo: sin Discoverer, no hay forma de detectar
// dominio, así que sigue haciendo falta salir a buscar.
func TestRetrieve_DiscovererNilNoRompeYNecesitaFetch(t *testing.T) {
	kr := NewKnowledgeRetriever()
	rr := kr.Retrieve("aprender unity", nil, NewSkillLibrary())
	if rr == nil {
		t.Fatal("Retrieve no debe devolver nil")
	}
	if !rr.NeedsFetch() {
		t.Error("sin Discoverer, NeedsFetch debe ser true")
	}
	if rr.Domain != "" {
		t.Errorf("sin Discoverer no debe detectarse ningún Domain, obtuve %q", rr.Domain)
	}
}

// TestRetrieve_GoalVacioNoRompe confirma el criterio defensivo con
// texto vacío.
func TestRetrieve_GoalVacioNoRompe(t *testing.T) {
	kr := NewKnowledgeRetriever()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	rr := kr.Retrieve("", discoverer, NewSkillLibrary())
	if !rr.NeedsFetch() {
		t.Error("con Goal vacío, NeedsFetch debe ser true")
	}
}

// TestRetrieve_SkillLibraryNilNecesitaFetchAunqueElDominioSeReconozca
// confirma que sin biblioteca no hay nada que devolver, aunque el
// dominio sí se detecte correctamente.
func TestRetrieve_SkillLibraryNilNecesitaFetchAunqueElDominioSeReconozca(t *testing.T) {
	kr := NewKnowledgeRetriever()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	rr := kr.Retrieve("quiero aprender python", discoverer, nil)
	if rr.Domain != "Python" {
		t.Fatalf("Domain = %q, esperaba Python", rr.Domain)
	}
	if !rr.NeedsFetch() {
		t.Error("sin SkillLibrary, NeedsFetch debe ser true aunque el dominio se reconozca")
	}
}

// TestRetrieve_DominioNoReconocidoNecesitaFetch confirma que sin
// dominio reconocido, ni siquiera se intenta consultar la biblioteca.
func TestRetrieve_DominioNoReconocidoNecesitaFetch(t *testing.T) {
	kr := NewKnowledgeRetriever()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "", Label: "no debería matchear nada"})

	rr := kr.Retrieve("quiero aprender a cocinar", discoverer, skills)
	if rr.Domain != "" {
		t.Errorf("Domain debe quedar vacío para un objetivo sin dominio técnico, obtuve %q", rr.Domain)
	}
	if !rr.NeedsFetch() {
		t.Error("sin Domain reconocido, NeedsFetch debe ser true")
	}
}

// TestRetrieve_DominioReconocidoSinNotaNecesitaFetch confirma el caso
// intermedio: dominio detectado, pero todavía no hay nada guardado.
//
// Bug preexistente encontrado y corregido acá (no en este bloque, en
// el zip que llegó como base de Bloque 58): el input usaba "quiero
// aprender go", pero el dominio "Go" solo reconoce "golang" y
// "lenguaje go" como keywords (Bloque 52) — "go" a secas quedó
// deliberadamente afuera porque es una palabra común en español
// ("voy a") e inglés que generaría falsos positivos masivos. El test
// original nunca podía pasar; el bug estaba en la prueba, no en el
// reconocimiento de dominio.
func TestRetrieve_DominioReconocidoSinNotaNecesitaFetch(t *testing.T) {
	kr := NewKnowledgeRetriever()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	rr := kr.Retrieve("quiero aprender golang", discoverer, NewSkillLibrary())
	if rr.Domain != "Go" {
		t.Fatalf("Domain = %q, esperaba Go", rr.Domain)
	}
	if !rr.NeedsFetch() {
		t.Error("sin nota guardada, NeedsFetch debe ser true aunque el dominio se reconozca")
	}
	if rr.Skill != nil {
		t.Error("Skill debe quedar nil sin nada guardado")
	}
}

// TestRetrieve_DominioReconocidoConNotaNoNecesitaFetch es el camino
// feliz: dominio reconocido Y nota ya guardada — NO hace falta salir
// a Internet. El comportamiento central que Bloque 56 existe para
// dar. Importante: esta prueba NUNCA instancia un gateway.Gateway ni
// un servidor de prueba — demuestra que Retrieve resuelve todo desde
// memoria, sin ninguna posibilidad de tocar la red.
func TestRetrieve_DominioReconocidoConNotaNoNecesitaFetch(t *testing.T) {
	kr := NewKnowledgeRetriever()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID:      "Unreal Engine",
		Label:        "Unreal Engine",
		Content:      "contenido real guardado antes",
		Sources:      []string{"Epic Developer Docs"},
		Corroborated: true,
	})

	rr := kr.Retrieve("quiero hacer un juego en unreal engine", discoverer, skills)
	if rr.Domain != "Unreal Engine" {
		t.Fatalf("Domain = %q, esperaba Unreal Engine", rr.Domain)
	}
	if rr.NeedsFetch() {
		t.Fatal("con una nota ya guardada para el dominio, NeedsFetch debe ser false")
	}
	if rr.Skill == nil || rr.Skill.Label != "Unreal Engine" {
		t.Errorf("Skill devuelto incorrecto: %+v", rr.Skill)
	}
}

// TestRetrieve_ClaveDeTextoLibreNoMatchea confirma que Retrieve busca
// por la clave EXACTA (Domain), no por el texto completo del
// objetivo — mismo tipo de verificación que Bloque 55 hizo para
// Skill Integration, ahora del lado de Retrieval.
func TestRetrieve_ClaveDeTextoLibreNoMatchea(t *testing.T) {
	kr := NewKnowledgeRetriever()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "quiero aprender unity para hacer un juego", Label: "unity"})

	rr := kr.Retrieve("quiero aprender unity para hacer un juego", discoverer, skills)
	if rr.Domain != "Unity" {
		t.Fatalf("Domain = %q, esperaba Unity", rr.Domain)
	}
	if !rr.NeedsFetch() {
		t.Error("no debe encontrar la nota — está guardada bajo una clave de texto libre, no bajo el Domain real 'Unity'")
	}
}

// TestDescribe_SinDominioReconocido confirma la narrativa cuando no
// se reconoce ningún dominio.
func TestDescribe_RetrievalResult_SinDominio(t *testing.T) {
	rr := &RetrievalResult{Goal: "aprender a cocinar"}
	desc := rr.Describe()
	if !strings.Contains(desc, "No reconozco un dominio técnico conocido") {
		t.Errorf("Describe() = %q", desc)
	}
}

// TestDescribe_DominioSinNota confirma la narrativa del caso
// intermedio: se sabe el dominio, pero no hay nada guardado.
func TestDescribe_RetrievalResult_DominioSinNota(t *testing.T) {
	rr := &RetrievalResult{Goal: "aprender go", Domain: "Go"}
	desc := rr.Describe()
	if !strings.Contains(desc, "Detecté el dominio Go") || !strings.Contains(desc, "hace falta salir a buscar") {
		t.Errorf("Describe() = %q", desc)
	}
}

// TestDescribe_ConNotaMencionaQueNoHizoFaltaInternet confirma la
// narrativa central del bloque: cuando resuelve desde caché, lo dice
// explícitamente.
func TestDescribe_RetrievalResult_ConNota(t *testing.T) {
	rr := &RetrievalResult{
		Goal:   "aprender unreal engine",
		Domain: "Unreal Engine",
		Skill:  &Skill{Sources: []string{"Epic Developer Docs"}, Corroborated: true},
	}
	desc := rr.Describe()
	if !strings.Contains(desc, "no hizo falta volver a Internet") {
		t.Errorf("Describe() debe decir explícitamente que no hizo falta volver a Internet: %q", desc)
	}
	if !strings.Contains(desc, "corroborada") {
		t.Errorf("Describe() debe reflejar que la nota está corroborada: %q", desc)
	}
}

// TestDescribe_NotaSinCorroborarSeDistingue confirma que la
// narrativa distingue una nota corroborada de una que no.
func TestDescribe_RetrievalResult_NotaSinCorroborarSeDistingue(t *testing.T) {
	rr := &RetrievalResult{
		Goal:   "aprender go",
		Domain: "Go",
		Skill:  &Skill{Sources: []string{"Go Docs"}, Corroborated: false},
	}
	desc := rr.Describe()
	if !strings.Contains(desc, "sin corroborar") {
		t.Errorf("Describe() debe decir 'sin corroborar' para una nota no corroborada: %q", desc)
	}
}

// TestNeedsFetch_ReflejaSkillDirectamente confirma que NeedsFetch es
// exactamente el inverso de tener un Skill no-nil, sin lógica extra
// escondida.
func TestNeedsFetch_ReflejaSkillDirectamente(t *testing.T) {
	sinSkill := &RetrievalResult{}
	if !sinSkill.NeedsFetch() {
		t.Error("sin Skill, NeedsFetch debe ser true")
	}
	conSkill := &RetrievalResult{Skill: &Skill{}}
	if conSkill.NeedsFetch() {
		t.Error("con Skill, NeedsFetch debe ser false")
	}
}
