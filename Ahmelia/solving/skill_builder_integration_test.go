package solving

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ahmelia/gateway"
)

// TestCadenaCompleta_DiscoverFetchValidateBuildInstall extiende el
// test de integración de Bloque 53 (TestCadenaCompleta_DiscoverFetchValidate)
// un paso más: toma el ValidationSet real y confirma que Skill Builder
// arma e instala una Skill persistible — contra un servidor
// httptest local, mismo criterio de no salir a Internet de verdad.
func TestCadenaCompleta_DiscoverFetchValidateBuildInstall(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Doc de prueba</title></head><body><p>`+strings.Repeat("contenido real de sobra sobre unity. ", 20)+`</p></body></html>`)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("no se pudo parsear la URL del server de test: %v", err)
	}

	testSources := []gateway.AuthorizedSource{{Name: "Unity Docs (test)", Host: u.Host}}
	discoverer := NewDiscoverer(testSources)
	if len(discoverer.AllSources) != 1 || discoverer.AllSources[0].Host != u.Host {
		t.Fatalf("esperaba que el Discoverer tuviera la fuente de prueba cargada, obtuve %+v", discoverer.AllSources)
	}
	discovery := &DiscoveryResult{Goal: "quiero aprender unity", Domain: "Unity (test)", Sources: testSources}

	gw := gateway.NewGatewayWithSources(testSources)
	gw.Client = srv.Client()

	var urls []string
	for _, src := range discovery.Sources {
		urls = append(urls, "https://"+src.Host+"/")
	}
	docs, errs := gw.FetchAll(urls)
	if len(errs) != 0 {
		t.Fatalf("no esperaba errores al fetchear el servidor de prueba, obtuve %v", errs)
	}

	sv := NewSourceValidator()
	validation := sv.ValidateAll(discovery.Goal, docs)
	if !validation.Validations[0].IsReliable() {
		t.Fatalf("esperaba que el documento de prueba fuera confiable, Issues=%v", validation.Validations[0].Issues)
	}

	// Acá empieza lo nuevo de Bloque 54: Build + Install sobre el
	// ValidationSet real que acaba de producir Bloque 53.
	sb := NewSkillBuilder()
	item := LearningItem{TaskID: "unity", TaskDesc: discovery.Goal}
	note := sb.Build(item, validation)
	if note == nil {
		t.Fatal("esperaba una Skill real, obtuve nil")
	}
	if !strings.Contains(note.Content, "contenido real de sobra sobre unity") {
		t.Errorf("Content de la nota no incluye el texto real fetcheado: %q", note.Content)
	}
	if len(note.Sources) != 1 || note.Sources[0] != "Unity Docs (test)" {
		t.Errorf("Sources = %v, esperaba [Unity Docs (test)]", note.Sources)
	}

	lib := NewSkillLibrary()
	lib.Install(note)
	if !lib.Has("unity") {
		t.Fatal("esperaba que la biblioteca tuviera la nota instalada")
	}

	tmpFile := t.TempDir() + "/skill_library.json"
	if err := lib.Save(tmpFile); err != nil {
		t.Fatalf("Save falló: %v", err)
	}
	reloaded := LoadSkillLibrary(tmpFile)
	if !reloaded.Has("unity") {
		t.Fatal("esperaba recuperar la nota 'unity' tras Save+Load — cadena completa persistida")
	}
}
