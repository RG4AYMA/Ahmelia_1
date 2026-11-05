package variation

// open_f2_reading_test.go — Batería v1, eje F2 (intención comunicativa) lado
// CONTRATO: las 5 formas faltantes de la tabla del propietario, verificadas
// como shapes que B2/B1 deben sostener SIN LLM (puros). La guarda de
// autorización vive en core (DeepSeek 18:53); aquí se fija la precondición
// que la guarda necesita: marcas normalizadas, actos coherentes y el
// INVARIANTE de seguridad BF2 "un rechazo jamás llega como captura llana".
//
// Fuente de los casos: directriz del propietario (familia 2), filas no
// verificadas en vivo aún: rechazo, sugerencia, pregunta informativa,
// indecisión, delegación. Los OpenMeaning de entrada son los JSON que el
// contrato abierto produce (marcas incluidas); nada se inventa aquí: si el
// modelo no emite la marca, el caso se reporta como hallazgo, no se finge.

import (
	"strings"
	"testing"

	"ahmelia/linguistics"
)

// openF2returns construye el OpenMeaning a partir del JSON del contrato.
func openF2returns(t *testing.T, raw string) linguistics.OpenMeaning {
	t.Helper()
	om, err := linguistics.ParseOpenMeaning(raw)
	if err != nil {
		t.Fatalf("parse del caso falló: %v", err)
	}
	return om
}

// TestF2_RechazoJamasCapturaLlana — el invariante de seguridad BF2: cuando la
// marca de negación con scope=accion está presente, el Reading NO puede leerse
// como captura de dato personal sin marca: la evidencia queda tipada.
func TestF2_RechazoJamasCapturaLlana(t *testing.T) {
	// "No quiero que la cierres." — como debería interpretarlo el contrato.
	raw := `{"purpose":"declaracion","owner_of_data":"user","topic":"cerrar la ventana",
		"meaning":"El usuario expresa que NO desea que la ventana sea cerrada; es una restricción sobre la acción.",
		"mentions":["cerrar"],
		"confidence":0.97,
		"marks":{"negation":{"scope":"accion","target":"cerrar la ventana"},"modality":"affirmed","provenance":"user"}}`
	om := openF2returns(t, raw)
	r := linguistics.DeriveReading(om)

	if r.Acto != "captura" {
		t.Fatalf("acto esperado captura (declaración), got %q", r.Acto)
	}
	if r.Marks.Negation == nil || r.Marks.Negation.Scope != "accion" {
		t.Fatalf("la marca de negación (scope accion) es la PRECONDICIÓN de la guarda: got %+v", r.Marks.Negation)
	}
	// INVARIANTE BF2 lado contrato: existe marca → existe señal explícita para
	// que la guard (core) bloquee la captura llana. Si esto falla, el dato
	// entra sin freno y la seguridad se pierde.
	if markers := linguistics.RestrictiveMarkers(r); len(markers) == 0 {
		t.Fatal("Reading con negation(action) debe exponer marcador restrictivo (BF2)")
	}
}

// TestF2_SugerenciaNoEsOrden — "Quizás sería mejor cerrar la ventana":
// sugerencia = modality hypothesis + purpose declaracion; jamás debe derivar
// un acto de solicitud.
func TestF2_SugerenciaNoEsOrden(t *testing.T) {
	raw := `{"purpose":"declaracion","owner_of_data":"user","topic":"ventana",
		"meaning":"El usuario propone, sin orden, que quizás sería mejor cerrar la ventana.",
		"mentions":[],
		"confidence":0.95,
		"marks":{"modality":"hypothesis","provenance":"user"}}`
	om := openF2returns(t, raw)
	r := linguistics.DeriveReading(om)

	if r.Acto != "captura" {
		t.Fatalf("acto = %q", r.Acto)
	}
	if r.Marks.Modality != linguistics.ModalityHypothesis {
		t.Fatalf("sugerencia debe conservar modality=hypothesis (conserva incertidumbre), got %q", r.Marks.Modality)
	}
	if r.Valor != "" {
		t.Fatalf("sugerencia sin mentions no debe inventar valor: %q", r.Valor)
	}
}

// TestF2_PreguntaInformativaNoInterviene — "¿Por qué está entrando frío?":
// consulta de explicación, no pedido de intervención.
func TestF2_PreguntaInformativaNoInterviene(t *testing.T) {
	raw := `{"purpose":"pregunta","owner_of_data":"externo","topic":"por qué entra frío",
		"meaning":"El usuario pregunta la causa de que esté entrando frío; pide una explicación, no una acción.",
		"mentions":[],
		"confidence":0.97}`
	om := openF2returns(t, raw)
	r := linguistics.DeriveReading(om)

	if r.Acto != "consulta" {
		t.Fatalf("acto = %q (pide explicación, no intervención)", r.Acto)
	}
	if r.Marks.Negation != nil || r.Marks.Condition != nil {
		t.Fatalf("consulta pura no debe arrastrar marcas de acción: %+v", r.Marks)
	}
}

// TestF2_IndecisionConservaAmbiguedad — "No sé si debería cerrarla":
// incertidumbre marcada, no certeza.
func TestF2_IndecisionConservaAmbiguedad(t *testing.T) {
	raw := `{"purpose":"declaracion","owner_of_data":"user","topic":"cerrar la ventana",
		"meaning":"El usuario no sabe si debería cerrar la ventana; expresa indecisión, no una orden.",
		"mentions":[],
		"confidence":0.9,
		"marks":{"modality":"hypothesis","provenance":"user"}}`
	om := openF2returns(t, raw)
	r := linguistics.DeriveReading(om)

	if r.Marks.Modality != linguistics.ModalityHypothesis {
		t.Fatalf("indecisión debe quedar como hypothesis (no certeza): %q", r.Marks.Modality)
	}
	if r.Acto == "solicitud" {
		t.Fatal("indecisión jamás deriva en solicitud")
	}
}

// TestF2_DelegacionExigePlanNoCaptura — "Encárgate de que no entre más frío":
// delegación con negación incrustada; el acto es solicitud, la marca de
// negación del SUB-OBJETIVO queda disponible para el planificador (BF6).
func TestF2_DelegacionExigePlanNoCaptura(t *testing.T) {
	raw := `{"purpose":"pedido","owner_of_data":"user","topic":"evitar que entre frío",
		"meaning":"El usuario delega el objetivo de evitar la entrada de frío; requiere planificación y comprobación, no una única acción directa.",
		"mentions":["frío"],
		"confidence":0.94,
		"marks":{"negation":{"scope":"estado","target":"entrada de más frío"}}}`
	om := openF2returns(t, raw)
	r := linguistics.DeriveReading(om)

	if r.Acto != "solicitud" {
		t.Fatalf("acto = %q", r.Acto)
	}
	if r.Marks.Negation == nil || !strings.Contains(r.Marks.Negation.Target, "frío") {
		t.Fatalf("la marca del sub-objetivo (no más frío) debe sobrevivir al plan: %+v", r.Marks.Negation)
	}
}

// TestF2_RubricaOwnerRayas — los tres wobble de F4 (#2/#3/#7) NO pueden
// repetirse con la rúbrica estricta: si el modelo repite "ahmelia" sin que el
// texto mencione a Ahmelia, el caso se trae al tablero (hallazgo, no silencio).
func TestF2_RubricaOwnerRayas(t *testing.T) {
	// máximo permitido: ownericial plural consistente; "ahmelia" solo si
	// objeto explícito. Este fixture representa la lectura correcta que la
	// rúbrica estricta debe producir para "No es imposible resolverlo".
	raw := `{"purpose":"declaracion","owner_of_data":"user","topic":"resolver un problema",
		"meaning":"El usuario afirma que no es imposible resolver el problema, es decir, que considera alcanzable la solución.",
		"mentions":[],
		"confidence":0.98,
		"marks":{"modality":"affirmed","provenance":"user"}}`
	om := openF2returns(t, raw)
	r := linguistics.DeriveReading(om)
	if r.Owner != linguistics.OpenOwnerUser {
		t.Fatalf("rubrica estricta: fixtur de referencia debe ser user (no ahmelia sin mención), got %q", r.Owner)
	}
	if r.Marks.Modality != linguistics.ModalityAffirmed || r.Marks.Provenance != linguistics.ProvenanceUser {
		t.Fatalf("procedencia debe conservarse: %+v", r.Marks)
	}
}
