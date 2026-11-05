package language

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/situation"
	"ahmelia/socialinteraction"
)

// ConversationalClient is the narrow local-language contract used only to
// render already-decided conversational meaning. It cannot execute actions,
// select capabilities, or mutate memory.
type ConversationalClient interface {
	GenerateContext(ctx context.Context, prompt string) (string, error)
	Available() bool
}

var ErrConversationalGenerationTimeout = errors.New("conversational generation deadline exceeded")

const defaultConversationalGenerationBudget = 15 * time.Second
const conversationalGenerationBudgetEnv = "AHMELIA_CONVERSATIONAL_GENERATION_BUDGET_MS"

type ConversationalGenerator struct {
	client ConversationalClient
}

func NewConversationalGenerator(client ConversationalClient) *ConversationalGenerator {
	return &ConversationalGenerator{client: client}
}

// RendererID exposes adapter provenance for runtime diagnostics without giving
// the language client authority over response meaning or execution.
func (g *ConversationalGenerator) RendererID() string {
	if g == nil || g.client == nil {
		return "unavailable"
	}
	if identified, ok := g.client.(interface{ ModelName() string }); ok {
		if name := strings.TrimSpace(identified.ModelName()); name != "" {
			return name
		}
	}
	return "configured_language_client"
}

type conversationalEnvelope struct {
	Response string `json:"response"`
}

// RuntimeFacts are grounded observations supplied by the Engine. They let the
// language layer describe Ahmelia's current state without turning the LLM into
// an authority over goals, work, memory, or capabilities.
type RuntimeFacts struct {
	ActiveTopic     string
	ActiveObjective string
	ActiveWorkID    string
	ActiveTask      string
	LastAction      string
	SystemReady     bool
	// PersistentMode is the owner's durable posture. It is context for the
	// renderer, never a permission to change it.
	PersistentMode string
	// LastExpectation is the harness verdict of the most recent declared
	// expectation. It is readable context; the renderer cannot change it.
	LastExpectation  string
	DecisionEvidence situation.DecisionEvidence
}

// Generate renders a conversational turn from semantic/context contracts.
// The model is explicitly a language renderer here: it must not invent facts,
// decide actions, or ask a question merely to keep the conversation alive.
func (g *ConversationalGenerator) Generate(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, intent interaction.ResponseIntent) (string, error) {
	return g.GenerateWithContext(context.Background(), req, snap, intent, cognition.ConversationalResponseStrategy{}, socialinteraction.ResponsePlan{})
}

// GenerateWithContext is the live B96.33+B96.38 rendering boundary. The
// generator consumes the response strategy and social plan already selected by
// upstream layers; it does not create those decisions itself. The generator's
// own time budget is always applied on top of the caller context.
func (g *ConversationalGenerator) GenerateWithContext(ctx context.Context, req interaction.SemanticRequest, snap interaction.ConversationSnapshot, intent interaction.ResponseIntent, strategy cognition.ConversationalResponseStrategy, socialPlan socialinteraction.ResponsePlan) (string, error) {
	return g.GenerateGroundedContext(ctx, req, snap, intent, strategy, socialPlan, RuntimeFacts{}, nil)
}

// GenerateGrounded preserves the historical API while routing through the
// budgeted context-aware rendering boundary.
func (g *ConversationalGenerator) GenerateGrounded(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, intent interaction.ResponseIntent, strategy cognition.ConversationalResponseStrategy, socialPlan socialinteraction.ResponsePlan, facts RuntimeFacts, evidence []string) (string, error) {
	return g.GenerateGroundedContext(context.Background(), req, snap, intent, strategy, socialPlan, facts, evidence)
}

// GenerateGroundedContext is the single language-rendering boundary for
// conversational responses and autonomous turns. Evidence/facts are inputs
// only; the model cannot authorize actions or manufacture state. Every call is
// bounded by the same generator-level budget, including guardrail retries.
func (g *ConversationalGenerator) GenerateGroundedContext(ctx context.Context, req interaction.SemanticRequest, snap interaction.ConversationSnapshot, intent interaction.ResponseIntent, strategy cognition.ConversationalResponseStrategy, socialPlan socialinteraction.ResponsePlan, facts RuntimeFacts, evidence []string) (string, error) {
	if g == nil || g.client == nil || !g.client.Available() {
		return "", fmt.Errorf("conversational generator unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	budget := ConversationalGenerationBudget()
	budgetCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	prompt := buildConversationalPrompt(req, snap, intent, strategy, socialPlan, facts, evidence)
	raw, err := func() (string, error) {
		if client, ok := g.client.(interface {
			GenerateConversationalContext(context.Context, string) (string, error)
		}); ok {
			return client.GenerateConversationalContext(budgetCtx, prompt)
		}
		return g.client.GenerateContext(budgetCtx, prompt)
	}()
	if err != nil {
		return "", normalizeConversationalGenerationError(err, budgetCtx)
	}
	var env conversationalEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return "", fmt.Errorf("invalid conversational JSON: %w", err)
	}
	response := strings.TrimSpace(env.Response)
	if response == "" {
		return "", fmt.Errorf("empty conversational response")
	}

	if violatesFeedbackInventionGuardrail(req, response) {
		retryPrompt := prompt + "\n\nADVERTENCIA DE CUMPLIMIENTO: tu respuesta anterior violó la regla de no prometer cambios futuros ni atribuir trabajo interno al responder feedback del usuario. Genera una respuesta nueva que reconozca la observación concreta del usuario SIN usar ningún verbo de compromiso futuro (voy a, haré, vamos a, prometo, me comprometo, lograré, seguiré trabajando en eso, etc.). Devuelve EXCLUSIVAMENTE JSON válido con esta forma: {\"response\":\"...\"}."
		if retryRaw, retryErr := g.client.GenerateContext(budgetCtx, retryPrompt); retryErr == nil {
			var retryEnv conversationalEnvelope
			if json.Unmarshal([]byte(retryRaw), &retryEnv) == nil {
				retryResponse := strings.TrimSpace(retryEnv.Response)
				if retryResponse != "" && !violatesFeedbackInventionGuardrail(req, retryResponse) {
					response = retryResponse
				} else {
					response = deterministicFeedbackAcknowledge(req)
				}
			} else {
				response = deterministicFeedbackAcknowledge(req)
			}
		} else {
			if errors.Is(normalizeConversationalGenerationError(retryErr, budgetCtx), ErrConversationalGenerationTimeout) {
				return "", normalizeConversationalGenerationError(retryErr, budgetCtx)
			}
			response = deterministicFeedbackAcknowledge(req)
		}
	}

	if len([]rune(response)) > 1200 {
		response = string([]rune(response)[:1200])
	}
	return response, nil
}

func ConversationalGenerationBudget() time.Duration {
	ms, err := strconv.Atoi(strings.TrimSpace(os.Getenv(conversationalGenerationBudgetEnv)))
	if err != nil || ms <= 0 {
		return defaultConversationalGenerationBudget
	}
	return time.Duration(ms) * time.Millisecond
}

func normalizeConversationalGenerationError(err error, ctx context.Context) error {
	if err == nil {
		return nil
	}
	if (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrConversationalGenerationTimeout, err)
	}
	return err
}

func buildConversationalPrompt(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, intent interaction.ResponseIntent, strategy cognition.ConversationalResponseStrategy, socialPlan socialinteraction.ResponsePlan, facts RuntimeFacts, evidence []string) string {
	var b strings.Builder
	b.WriteString(`Eres el generador de lenguaje conversacional interno de Ahmelia.
Tu trabajo es expresar en español natural una respuesta cuyo propósito ya fue decidido por el sistema.
NO ejecutes acciones.
NO inventes hechos, recuerdos, proyectos, horarios, preferencias ni capacidades.
NO cambies el objetivo de la conversación.
Cuando recibas evidencia que describe una decisión operativa ya tomada por Ahmelia, conserva exactamente su estado, sus negaciones y sus condiciones (por ejemplo, pendiente, autorizado, rechazado o no ejecutado). Puedes expresarlo con lenguaje natural, pero nunca cambies la decisión ni afirmes que una acción ocurrió si la evidencia no lo dice.
NO conviertas una conversación en una orden.
NO hagas una pregunta por costumbre. Solo pregunta si el propósito de esta respuesta realmente necesita una respuesta del usuario.
En feedback o crítica del usuario, responde al contenido concreto de su observación. No prometas cambios futuros ni atribuyas trabajo interno que no esté respaldado por los hechos disponibles.
NO uses frases de asistente genéricas como "Te escucho. Dime qué tienes en mente y lo vemos juntos" salvo que sean realmente apropiadas para el contexto.
Habla como una compañera conversacional natural: directa, humana, cálida cuando corresponda y sin sonar prefabricada.
Puedes reconocer lo que el usuario acaba de decir, relacionarlo con el contexto disponible y ofrecer una observación útil.
Si el usuario expresa una dificultad o estado sin pedir una solución concreta, puedes responder a ese estado sin convertirlo automáticamente en un interrogatorio.
Devuelve EXCLUSIVAMENTE JSON válido con esta forma: {"response":"..."}.
`)
	fmt.Fprintf(&b, "\nPropósito comunicativo: %s\n", intent.Kind)
	fmt.Fprintf(&b, "Propietario semántico del turno: %q confianza=%.2f\n", req.Subject, req.SubjectConfidence)
	for _, ownerEvidence := range req.SubjectEvidence {
		if strings.TrimSpace(ownerEvidence.Token) != "" {
			fmt.Fprintf(&b, "Evidencia de propiedad: señal=%s token=%q confianza=%.2f proveniencia=%s\n", ownerEvidence.Signal, ownerEvidence.Token, ownerEvidence.Confidence, ownerEvidence.Provenance)
		}
	}
	b.WriteString("Regla de propiedad: si el propietario es \"self\", no uses datos del User Model; si es \"user\", no atribuyas datos del usuario a Ahmelia.\n")
	b.WriteString("Regla de relevancia: no menciones recuerdos, preferencias o hechos disponibles solo porque existan; cada dato debe responder directamente al propósito de este turno.\n")
	fmt.Fprintf(&b, "Estrategia B96.33: modo=%s directness=%s structure=%s initiative=%s confidence=%.2f needs_clarification=%t preserve_user_agency=%t\n", strategy.Mode, strategy.Directness, strategy.Structure, strategy.Initiative, strategy.Confidence, strategy.NeedsClarification, strategy.PreserveUserAgency)
	if strategy.Adaptation.Version != "" {
		a := strategy.Adaptation
		fmt.Fprintf(&b, "Adaptación contextual B96.33.11: modo=%s detalle=%s ritmo=%s directness=%s initiative=%s continuidad=%t cautela=%t preservar_agencia=%t evidencia=%s\n", a.Mode, a.Detail, a.Pacing, a.Directness, a.Initiative, a.Continuity, a.Caution, a.PreserveUserAgency, strings.Join(a.Evidence, ","))
		if a.Mode == cognition.AdaptationModeSupportive {
			b.WriteString("Ajuste de participación: toma en cuenta la frustración expresada sin diagnosticarla ni minimizarla; reduce la presión y no conviertas el relato en una orden de trabajo. Si pidió ayuda concreta, responde a esa petición con un paso pequeño y manejable; evita preguntas innecesarias y acepta que prefiera esperar.\n")
		}
	}
	fmt.Fprintf(&b, "Conducta social B96.38: intent=%s state=%s participation=%s formality=%s repair=%t\n", socialPlan.Intent, socialPlan.State, socialPlan.Participation, socialPlan.Formality, socialPlan.RepairRequired)
	fmt.Fprintf(&b, "Estado operativo observado: tema=%q objetivo_interaccion=%q trabajo=%q tarea=%q ultima_accion=%q sistema_listo=%t\n", facts.ActiveTopic, facts.ActiveObjective, facts.ActiveWorkID, facts.ActiveTask, facts.LastAction, facts.SystemReady)
	if mode := strings.TrimSpace(facts.PersistentMode); mode != "" {
		fmt.Fprintf(&b, "Modo persistente elegido por el propietario: %s. Es contexto, no una orden: no lo cambies ni actúes como si ya hubiera cambiado.\n", mode)
	}
	if verdict := strings.TrimSpace(facts.LastExpectation); verdict != "" {
		fmt.Fprintf(&b, "Último veredicto de expectativa del arnés: %s Es evidencia de autoridad: puedes leerlo, no cambiarlo, no declararlo de nuevo ni contradecirlo sin observación nueva.\n", verdict)
	}
	if facts.DecisionEvidence.SituationID != "" {
		fmt.Fprintf(&b, "\nEvidencia situacional evaluada (Situation %s v%d):\n", facts.DecisionEvidence.SituationID, facts.DecisionEvidence.SituationVersion)
		for _, f := range facts.DecisionEvidence.Findings {
			fmt.Fprintf(&b, "- finding=%s perspective=%s confidence=%.2f subject=%q target=%q\n", f.Kind, f.Perspective, f.Confidence, f.Subject, f.Target)
		}
		for _, c := range facts.DecisionEvidence.Conflicts {
			fmt.Fprintf(&b, "- conflict=%s\n", c)
		}
		for _, u := range facts.DecisionEvidence.Uncertainty {
			fmt.Fprintf(&b, "- uncertainty=%s\n", u)
		}
		b.WriteString("Prioridad: la evidencia situacional evaluada de este turno prevalece sobre continuidad histórica cuando existe contradicción explícita.\n")
	}
	if len(evidence) > 0 {
		b.WriteString("\nEvidencia factual disponible (usar solo si responde al turno):\n")
		for _, item := range evidence {
			if strings.TrimSpace(item) != "" {
				fmt.Fprintf(&b, "- %s\n", item)
			}
		}
	}
	fmt.Fprintf(&b, "Entrada del usuario: %q\n", req.Raw)
	fmt.Fprintf(&b, "Tema actual: %q\n", snap.ActiveTopic)
	fmt.Fprintf(&b, "Objetivo actual: %q\n", snap.ActiveObjective)
	fmt.Fprintf(&b, "Señales del usuario: frustración=%.2f, fatiga=%.2f, positivo=%.2f, urgencia=%.2f\n", req.UserSignals.Frustration, req.UserSignals.Fatigue, req.UserSignals.Positive, req.UserSignals.Urgency)
	if len(snap.Turns) > 0 {
		start := len(snap.Turns) - 6
		if start < 0 {
			start = 0
		}
		b.WriteString("\nÚltimos turnos relevantes:\n")
		for _, t := range snap.Turns[start:] {
			fmt.Fprintf(&b, "- Usuario: %q\n", t.UserText)
			if strings.TrimSpace(t.Assistant) != "" {
				fmt.Fprintf(&b, "  Ahmelia: %q\n", t.Assistant)
			}
		}
	}
	b.WriteString(`
Regla final: responde a ESTA intervención primero. Si la entrada del usuario está vacía, estás expresando una iniciativa conversacional autorizada por B96.37; usa el estado observado para decidir qué decir, sin inventar actividad. La naturalidad viene de entender el contexto, no de añadir preguntas o frases de relleno.`)
	return b.String()
}
