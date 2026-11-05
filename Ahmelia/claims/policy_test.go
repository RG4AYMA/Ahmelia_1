package claims

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSourcePolicyMissingStartsLocalOnly(t *testing.T) {
	p, err := LoadSourcePolicy(filepath.Join(t.TempDir(), "ausente.json"))
	if err != nil {
		t.Fatalf("archivo ausente debe cargar el default solo-local: %v", err)
	}
	if countKind(p.Entries, SourceKindHost) != 0 {
		t.Fatalf("el arranque no debe tener web pre-aprobada (D1 de sitios verificados): %+v", p.Entries)
	}
	if got := p.RecommendedOptions(); got.MinHosts != 2 {
		t.Fatalf("corroboration multi-host por defecto: got %+v", got)
	}
}

func TestLoadSourcePolicyCorruptRejectsEverythingNothingAllowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allowlist.json")
	if err := os.WriteFile(path, []byte("{no es json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSourcePolicy(path); err == nil {
		t.Fatal("la allowlist corrupta debe producir un ERROR y el llamador rechaza TODO (never unsafe by default)")
	}
}

func TestGateLocalSourcesAdmitted(t *testing.T) {
	p := DefaultSourcePolicy()
	// Nota local del propietario con huella de contenido.
	ok, why := p.Gate(Provenance{URL: "file:///C:/notas/blender.md", ContentSHA256: "h1", FetchedAt: time.Now()})
	if !ok || !strings.Contains(why, "local") {
		t.Fatalf("fuente local file:// debe admitirse: ok=%v why=%s", ok, why)
	}
	// Sin URL absoluta pero sin host web (fuente interna del sistema).
	ok, _ = p.Gate(Provenance{ContentSHA256: "h2"})
	if !ok {
		t.Fatal("fuente interna sin host web debe admitirse como local")
	}
}

func TestGateUnknownHostsAreRejected(t *testing.T) {
	p := DefaultSourcePolicy()
	casos := []Provenance{
		{URL: "https://ejemplo-desconocido.com/manual", Host: "ejemplo-desconocido.com", ContentSHA256: "h"},
		{URL: "https://foro-cualquiera.net/post", ContentSHA256: "h"},
	}
	for _, src := range casos {
		if ok, why := p.Gate(src); ok {
			t.Fatalf("host desconocido debe RECHAZARSE (falla cerrada): %s → %s", src.URL, why)
		}
	}
}

func TestGateHostAllowlistExactlyMatching(t *testing.T) {
	p := DefaultSourcePolicy()
	p.Entries = append(p.Entries, SourceEntry{Kind: SourceKindHost, Host: "docs.ollama.com", AddedBy: "propietario", AddedAt: time.Now()})
	ok, why := p.Gate(Provenance{URL: "https://docs.ollama.com/library/x", Host: "docs.ollama.com", ContentSHA256: "h"})
	if !ok || !strings.Contains(why, "docs.ollama.com") {
		t.Fatalf("host EXACTO enlistado en https debe admitirse: ok=%v why=%s", ok, why)
	}
	// http plano: rechazado incluso con host enlistado.
	if ok, why := p.Gate(Provenance{URL: "http://docs.ollama.com/x", Host: "docs.ollama.com", ContentSHA256: "h"}); ok || !strings.Contains(why, "http") {
		t.Fatalf("http plano debe rechazarse siempre: ok=%v why=%s", ok, why)
	}
	// Subdominio cercano NO enlistado: rechazo (sin comodines).
	if ok, _ := p.Gate(Provenance{URL: "https://es.docs.ollama.com/x", Host: "es.docs.ollama.com", ContentSHA256: "h"}); ok {
		t.Fatal("subdominio no enlistado debe rechazarse (sin comodines)")
	}
	if ok, _ := p.Gate(Provenance{URL: "https://docs.evil.tk@docs.ollama.com/x", Host: "docs.evil.tk", ContentSHA256: "h"}); ok {
		t.Fatal("host real distinto al enlistado debe rechazarse")
	}
}

func TestGateRequiresContentFingerprint(t *testing.T) {
	p := DefaultSourcePolicy()
	p.Entries = append(p.Entries, SourceEntry{Kind: SourceKindHost, Host: "docs.example.org"})
	if ok, _ := p.Gate(Provenance{URL: "https://docs.example.org/a"}); ok {
		t.Fatal("sin huella de contenido no entra ni un host enlistado")
	}
}
