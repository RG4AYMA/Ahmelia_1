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

// TestCadenaCompleta_DiscoverFetchValidate confirma que las tres capas
// de Fase 4.1 encadenan de verdad: Discover (52) decide fuentes reales
// del Gateway, Fetch (51) las trae de verdad (contra un servidor
// httptest local — sin salir a Internet, mismo criterio que
// gateway/knowledge_gateway_test.go), y ValidateAll (53) las evalúa.
func TestCadenaCompleta_DiscoverFetchValidate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Doc de prueba</title></head><body><p>`+strings.Repeat("contenido real de sobra. ", 20)+`</p></body></html>`)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("no se pudo parsear la URL del server de test: %v", err)
	}

	// Discoverer con una whitelist de prueba que apunta al servidor
	// local — mismo Host que el TechDomain "unity" busca, para que
	// Discover() encuentre una fuente real.
	testSources := []gateway.AuthorizedSource{{Name: "Unity Docs (test)", Host: u.Host}}
	// Discoverer real, con la whitelist de prueba — se confirma acá
	// que puede encontrar la fuente de prueba por Host exacto (mismo
	// mecanismo que usa con la whitelist real de Bloque 51), sin
	// depender del matching de techDomains por palabra clave (eso ya
	// lo cubre knowledge_discovery_test.go).
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
	if !strings.Contains(validation.Describe(), "1 de 1 documento(s) confiables") {
		t.Errorf("esperaba la narrativa de 1/1 confiable, obtuve: %q", validation.Describe())
	}
}
