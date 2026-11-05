package claims

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// D1 (2026-10-19, decisión del propietario) — Política de confianza de fuentes.
// Criterio del propietario: "solo quiero que entre a sitios verificados y de
// fuentes oficiales, quiero evitar problemas de hacking al sistema", y doble
// argumento: Ahmelia aún NO tiene capacidad autónoma de ciberseguridad, de modo
// que la postura correcta es conservadora y con falla cerrada.
//
// Mecanismo (R11: por IDENTIDAD de fuente, jamás por contenido o frase; R12:
// extiende claims/T5 con la puerta de admisión, no nuevo dominio):
//
//   Tipo local    → fuentes del propio sistema (notas locales del
//                   propietario, documentos internos): permitidas por diseño.
//   Tipo host     → un host EXACTO https aprobado por el propietario, uno
//                   por uno (sin comodines); cualquier otro dominio con
//                   falla cerrada se RECHAZA.
//   HTTP plano    → rechazado siempre (sin criptografía hay MITM; el propietario
//                   no pidió excepciones).
//   JSON corrupto → rechazado todo (nunca aceptar por falla de archivo).
//
// Defensa en profundidad: aun admitiendo la fuente, Compile solo arma pasos con
// corroboration mult-host (MinHosts recomendado 2) — una fuente sola NUNCA
// produce un paso.
//
// PUERTA DE USO: toda adquisición futura (lector de notas, investigación con
// fuente, T7+) debe pasar por Gate ANTES de NewClaim. Con falla cerrada no hay
// lector vivo todavía en producción: no hay claims externos hoy — este bloque
// existe para que el día que exista, el candado ya esté puesto.

type SourceKind string

const (
	SourceKindLocal SourceKind = "local" // notas/archivos del propietario en disco
	SourceKindHost  SourceKind = "host"  // host https exacto aprobado
)

const RecommendedMinCorroboratingHosts = 2

type SourceEntry struct {
	Kind    SourceKind `json:"kind"`
	Host    string     `json:"host,omitempty"`     // exacto, sin comodines (solo host)
	Path    string     `json:"path,omitempty"`     // prefijo de ruta local, p.ej. file:///
	AddedBy string     `json:"added_by,omitempty"` // "propietario"
	AddedAt time.Time  `json:"added_at,omitempty"`
	Note    string     `json:"note,omitempty"`
}

// SourcePolicy es el candado D1. La nula/corrupta NO es permisiva:
// el llamador con falla cerrada debe rechazar y declarar.
type SourcePolicy struct {
	Entries []SourceEntry `json:"entries"`
}

const DefaultSourceAllowlistPath = "memory/source_allowlist.json"

// DefaultSourcePolicy arranca con SOLO las fuentes locales del propio
// sistema: cero web hasta que el propietario agregue hosts a su lista.
func DefaultSourcePolicy() *SourcePolicy {
	return &SourcePolicy{Entries: []SourceEntry{{
		Kind:    SourceKindLocal,
		Path:    "file://",
		AddedBy: "sistema",
		Note:    "notas y archivos locales del propietario (D1: arranque)",
	}}}
}

// LoadSourcePolicy lee el archivo del propietario. Archivo ausente → política
// default (solo local, falla cerrada). Archivo ILEGIBLE o inválido → ERROR
// (el llamador NO debe permitir nada: never unsafe by default).
func LoadSourcePolicy(path string) (*SourcePolicy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSourcePolicy(), nil
		}
		return nil, fmt.Errorf("claims: no se pudo leer %s: %w", path, err)
	}
	p := &SourcePolicy{}
	if jsonErr := json.Unmarshal(raw, p); jsonErr != nil {
		return nil, fmt.Errorf("claims: allowlist corrupta: %w", jsonErr)
	}
	for i := range p.Entries {
		e := &p.Entries[i]
		if e.AddedAt.IsZero() {
			e.AddedAt = time.Now() // marca de carga, no de decisión (honestidad de trazas)
		}
	}
	return p, nil
}

// Gate decide si una Provenance puede convertirse en Claim por D1.
// Devuelve admitido=true solo para fuentes claramente enlistadas; el motivo
// de rechazo es trazable y declarativo.
func (p *SourcePolicy) Gate(src Provenance) (bool, string) {
	if p == nil {
		return false, "política ausente: rechazo (falla cerrada)"
	}
	if src.ContentSHA256 == "" {
		return false, "sin huella de contenido: rechazo (falla cerrada)"
	}
	host := strings.ToLower(strings.TrimSpace(src.Host))
	u, uerr := url.Parse(src.URL)
	if uerr == nil {
		switch u.Scheme {
		case "http":
			return false, "http plano rechazado (sin cifrado; D1 sitios verificados)"
		case "https", "file", "":
		default:
			return false, fmt.Sprintf("esquema %q no admitido", u.Scheme)
		}
		// Defensa anti-spoofing: la hosts de provenance y de la URL deben
		// COINCIDIR. Un host distinto dentro de la URL (userinfo @ o error del
		// lector) no se puede colar jamás.
		if uh := strings.ToLower(u.Hostname()); uh != "" {
			if host != "" && host != uh {
				return false, fmt.Sprintf("mismatch url/hoteles: %q vs %q → rechazo (falla cerrada)", host, uh)
			}
			host = uh
		}
	}
	localOnly := len(p.Entries) == 0 || countKind(p.Entries, SourceKindHost) == 0
	for _, e := range p.Entries {
		switch e.Kind {
		case SourceKindLocal:
			if host == "" || host == "local" {
				return true, "fuente local del sistema (D1)"
			}
			if strings.HasPrefix(src.URL, "file://") {
				return true, "fuente local del sistema (D1, file://)"
			}
		case SourceKindHost:
			allowed := strings.ToLower(strings.TrimSpace(e.Host))
			if allowed == "" || host != allowed {
				continue
			}
			if u != nil && u.Scheme == "https" {
				return true, fmt.Sprintf("host en la lista del propietario: %s", host)
			}
			if u == nil {
				return false, "fuente sin URL verificada: rechazo"
			}
		}
	}
	if localOnly {
		return false, fmt.Sprintf("host %q NO está en la lista del propietario: rechazo (falla cerrada;arranque solo-local)", displayHost(host))
	}
	return false, fmt.Sprintf("host %q NO está en la lista del propietario: rechazo (falla cerrada)", displayHost(host))
}

func countKind(entries []SourceEntry, k SourceKind) int {
	n := 0
	for _, e := range entries {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func displayHost(h string) string {
	if h == "" {
		h = "@local"
	}
	return h
}

// CompileOptions garantiza la segunda barrera D1: corroboration mult-host.
// El llamador usará estos mínimos salvo explicita instrucción stronger.
func (p *SourcePolicy) RecommendedOptions() Options {
	return Options{MinHosts: RecommendedMinCorroboratingHosts, MinSim: 0.60}
}

var errSourcePolicyRejects = errors.New("claims: fuente rechazada por la política D1")
