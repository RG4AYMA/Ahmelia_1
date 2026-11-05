// Overlay agregación de evidencia para T4 (plan unificado, M5/E2): convierte
// intentos verificados del Ledger + procedimientos conocidos + oráculo
// declarado en el Standing de cada capacidad, en cada consulta.
//
// Contrato de aceptación T4 ("¿Qué sabes hacer?" con Standing derivado):
//   - los estados se DERIVAN de evidencia en el momento de la consulta;
//     nunca se afirma lo que la evidencia no demuestra;
//   - sin oráculo declarado, el máximo afirmable es `unverifiable`;
//   - con un solo contexto (o ninguno), nunca `demonstrated`.
//
// Los seams (Catalog, OracleDeclarer) existen para que la integración en
// core reutilice los adaptadores ya cableados por E1 sin acoplarse a sus
// paquetes. Overlay no guarda estados derivados: Derive es puro.
package competence

import "sort"

// Catalog es el seam hacia procedure.Catalog / capability.Registry.
type Catalog interface {
	Known(capability, operation string) bool
}

// OracleDeclarer es el seam del oráculo (T6: VerificationProfile). Sin
// declaraciones, ninguna capacidad puede afirmar competencia verificada.
type OracleDeclarer interface {
	DeclaresOracle(capability, operation string) bool
}

// Assessment es el resultado derivado, con la razón textual de Derive.
type Assessment struct {
	Capability string
	Standing   Standing
	Reason     string
}

type Overlay struct {
	ledger    *Ledger
	catalog   Catalog
	oracle    OracleDeclarer
	regressed bool // señal externa (suite del T10/T12): regresa si falla algo que antes pasaba
	policy    Policy
}

// NewOverlay construye el agregador. Todos los parámetros son nil-safe:
// sin ledger no hay nada que afirmar; sin catálogo, los intentos
// existentes bastan para considerar el procedimiento compilado (el loop de
// trabajo ya existió); sin oráculo, la escalera queda tapada en unverifiable.
func NewOverlay(ledger *Ledger, catalog Catalog, oracle OracleDeclarer, policy Policy) *Overlay {
	if policy.MinTrials <= 0 {
		policy = DefaultPolicy()
	}
	return &Overlay{ledger: ledger, catalog: catalog, oracle: oracle, policy: policy}
}

// SetRegressed instala la señal de regresión que llega de la compuerta de
// regresión (trial.Release) cuando se integre T10/T12. Nunca se calcula aquí.
func (o *Overlay) SetRegressed(v bool) {
	if o == nil {
		return
	}
	o.regressed = v
}

// StandingOf evalúa una capacidad contra toda su evidencia. Devuelve
// ok=false cuando no hay NINGÚN intento (no hay qué afirmar de ella).
func (o *Overlay) StandingOf(capability string) (Assessment, bool) {
	if o == nil || capability == "" {
		return Assessment{}, false
	}
	attempts := o.ledger.byCapability(capability)
	if len(attempts) == 0 {
		return Assessment{}, false
	}
	ops := distinctOperations(attempts)
	known := o.knownOnAny(capability, ops)
	if !known {
		return Assessment{Capability: capability, Standing: StandingHeard,
			Reason: "intentos registrados sin pasos válidos contra capacidades registradas"}, true
	}
	verified := 0
	for _, a := range attempts {
		if a.Verified {
			verified++
		}
	}
	// Cada intento real es también práctica ejecutada: Trials los cuenta
	// para cruzar el peldaño compiled; Real los cuenta para contextos.
	evidence := Evidence{
		Compiled:       true,
		HasOracle:      o.oracleDeclared(capability, ops),
		Trials:         Tally{Runs: len(attempts), Pass: verified},
		Real:           attempts,
		SuiteRegressed: o.regressed,
	}
	standing, reason := Derive(evidence, o.policy)
	return Assessment{Capability: capability, Standing: standing, Reason: reason}, true
}

// Capabilities lista las capacidades con intentos, en orden determinista,
// con su estado derivado. Sin registros devuelve vacío (nunca inventa).
func (o *Overlay) Capabilities() []Assessment {
	if o == nil || o.ledger == nil {
		return nil
	}
	out := []Assessment{}
	for _, capability := range o.ledger.capabilities() {
		if a, ok := o.StandingOf(capability); ok {
			out = append(out, a)
		}
	}
	return out
}

func (o *Overlay) knownOnAny(capability string, ops []string) bool {
	if o == nil || o.catalog == nil {
		return true // sin catálogo disponible: los intentos atestiguan el loop
	}
	for _, op := range ops {
		if o.catalog.Known(capability, op) {
			return true
		}
	}
	return false
}

func (o *Overlay) oracleDeclared(capability string, ops []string) bool {
	if o == nil || o.oracle == nil {
		return false
	}
	for _, op := range ops {
		if o.oracle.DeclaresOracle(capability, op) {
			return true
		}
	}
	return false
}

// byCapability consulta el Ledger con su mutex (paquete interno) en orden
// cronológico: mismos datos ⇒ mismo Standing, siempre.
func (l *Ledger) byCapability(capability string) []Attempt {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Attempt
	for _, a := range l.data {
		if a.Capability == capability {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

func (l *Ledger) capabilities() []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, a := range l.data {
		if !seen[a.Capability] {
			seen[a.Capability] = true
			out = append(out, a.Capability)
		}
	}
	sort.Strings(out)
	return out
}

func distinctOperations(attempts []Attempt) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range attempts {
		if !seen[a.Operation] {
			seen[a.Operation] = true
			out = append(out, a.Operation)
		}
	}
	sort.Strings(out)
	return out
}
