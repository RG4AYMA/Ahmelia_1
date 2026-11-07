package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ahmelia/agency"
	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/cognition"
	"ahmelia/competence"
	"ahmelia/comprehension"
	"ahmelia/consult"
	contextintelligence "ahmelia/contextintelligence"
	"ahmelia/deliberation"
	"ahmelia/dialogue"
	"ahmelia/environment"
	"ahmelia/execution"
	"ahmelia/expectation"
	"ahmelia/experience"
	"ahmelia/files"
	"ahmelia/gateway"
	conversationalinitiative "ahmelia/initiative"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/interrupt"
	"ahmelia/knowledge"
	"ahmelia/language"
	"ahmelia/linguistics"
	"ahmelia/memory"
	memoryintelligence "ahmelia/memory_intelligence"
	naturaldialogue "ahmelia/naturaldialogue"
	"ahmelia/ownerstate"
	"ahmelia/persistentmode"
	"ahmelia/personality"
	"ahmelia/planning"
	"ahmelia/procedure"
	"ahmelia/production"
	"ahmelia/programming"
	"ahmelia/reasoning"
	"ahmelia/reasoning_deep"
	"ahmelia/reference"
	"ahmelia/resource"
	"ahmelia/restriction"
	"ahmelia/selfknowledge"
	"ahmelia/shared"
	"ahmelia/situation"
	socialinteraction "ahmelia/socialinteraction"
	"ahmelia/solving"
	"ahmelia/system"
	"ahmelia/taste"
	"ahmelia/tts"
)

/* =======================
   TIPOS PROPIOS DEL ENGINE
======================= */

func Process(text string) {
	fmt.Println("🧠 Engine recibió:", text)

	tts.Speak(text)

	// aquí va tu lógica de Ahmelia
}

const (
	indexPath    = "memory/program_index.json"
	shortMemPath = "memory/short.json"
	maxShortMem  = 100
)

type ShortMemory struct {
	Commands []string `json:"commands"`
}

type ProgramIndex struct {
	Programs map[string]string `json:"programs"`
	LastScan time.Time         `json:"last_scan"`
}

func LoadShortMemory() ShortMemory {
	mem := ShortMemory{Commands: []string{}}
	data, err := os.ReadFile(shortMemPath)
	if err == nil {
		_ = json.Unmarshal(data, &mem)
	}
	return mem
}

func SaveShortMemory(mem ShortMemory) {
	if len(mem.Commands) > maxShortMem {
		mem.Commands = mem.Commands[len(mem.Commands)-maxShortMem:]
	}
	data, _ := json.MarshalIndent(mem, "", "  ")
	_ = os.WriteFile(shortMemPath, data, 0644)
}

func loadOrCreateIndex() ProgramIndex {
	// Los tests del core no necesitan recorrer el sistema operativo para
	// construir el índice de programas. El caller de test puede pedir un
	// índice vacío y evitar que cada NewEngine() haga un filepath.Walk real.
	if os.Getenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN") == "1" {
		return ProgramIndex{Programs: make(map[string]string)}
	}

	if _, err := os.Stat(indexPath); err == nil {
		idx := ProgramIndex{Programs: make(map[string]string)}
		data, err := os.ReadFile(indexPath)
		if err != nil {
			return idx
		}
		_ = json.Unmarshal(data, &idx)
		fmt.Printf("🧠 Índice: %d programas\n", len(idx.Programs))
		return idx
	}
	return createIndex()
}

func createIndex() ProgramIndex {
	fmt.Println("🔍 Creando índice...")
	start := time.Now()
	idx := ProgramIndex{Programs: make(map[string]string), LastScan: time.Now()}

	folders := []string{
		`C:\Program Files`,
		`C:\Program Files (x86)`,
		os.Getenv("APPDATA") + `\Microsoft\Windows\Start Menu\Programs`,
		os.Getenv("ProgramData") + `\Microsoft\Windows\Start Menu\Programs`,
		os.Getenv("LOCALAPPDATA") + `\Programs`,
		os.Getenv("LOCALAPPDATA") + `\Microsoft\WindowsApps`,
	}

	for _, base := range folders {
		_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(info.Name()), ".exe") {
				name := strings.ToLower(strings.TrimSuffix(info.Name(), ".exe"))
				if _, exists := idx.Programs[name]; !exists {
					idx.Programs[name] = path
				}
			}
			return nil
		})
	}

	data, _ := json.MarshalIndent(idx, "", "  ")
	_ = os.WriteFile(indexPath, data, 0644)
	fmt.Printf("✅ Índice: %d programas (%.1fs)\n", len(idx.Programs), time.Since(start).Seconds())
	return idx
}

/* =======================
   ENGINE
======================= */

type Engine struct {
	sess      *memory.Session
	longMem   *memory.LongMemory
	shortMem  *ShortMemory
	learner   *system.LearningSystem
	idx       ProgramIndex
	folderDst map[string]string
	context   *comprehension.ConversationContext // Historial de interacciones
	history   *selfknowledge.ActionHistory       // Historial de acciones (rápido)
	selfQuery *selfknowledge.SelfQuery           // Auto-consultas
	reasoner  *reasoning.Reasoner                // Capa 4 — Razonamiento

	// FASE 2 - Consolidación Cognitiva
	knowledgeEngine     *knowledge.Engine                 // Knowledge Core: un único punto de consulta
	uncertaintyHandler  *language.UncertaintyHandler      // Expresa dudas/incertidumbre de forma honesta
	world               *environment.Manager              // Modelo del entorno: qué está abierto AHORA
	refResolver         *reference.Resolver               // Nivel 1: referencias operativas a la app activa
	contextIntelligence *contextintelligence.Intelligence // B96.36 — autoridad de resolución de referencias/contexto conversacional               // "¿a qué se refiere eso?" — ver reference/resolver.go
	episode             *memoryintelligence.Episode       // memoria episódica de la sesión actual — ver memory_intelligence/episode.go
	eventStore          *memoryintelligence.EventStore    // Fase 1 (Observación) de Pattern Learning — dueño único del log crudo compartido con el ciclo autónomo (Bloque 93.3), ver memory_intelligence/eventstore.go
	learningState       *memoryintelligence.LearningStore // Bloque 7/8 — estrategias por Action + confianza; AdaptStrategy() decide ANTES de ejecutar, NoteOutcome() cierra el ciclo después. Dueño único compartido con el ciclo autónomo desde Bloque 93.3, ver memory_intelligence/learningstore.go

	// FASE 2.6 — Integración Cognitiva
	agenda                *planning.AgendaManager            // Goals — existía huérfano en planning/, nadie lo instanciaba. Bloque 9 lo conecta como fuente del Context Builder.
	planner               *planning.Planner                  // Bloque 46 — Executive Project Manager (núcleo Planner+Progress): genera planes reales para objetivos de varios días/semanas, algo que Fase 3 (Cognitive Mind) no hace — esa planifica UN pedido de UN turno.
	executiveGoals        map[string]*planning.Goal          // nombre normalizado -> Goal, para poder buscarlo por nombre en consultas futuras ("¿cómo vamos con X?")
	executivePlans        map[string]*planning.Plan          // Goal.ID -> Plan generado por planner.GeneratePlan()
	executiveProgress     map[string]*planning.Progress      // Goal.ID -> Progress, el seguimiento real de avance
	contextBuilder        *cognition.Builder                 // Bloque 9 — junta World/Timeline/Habits/Insights/Reflection/Learning/Goals/MentalState/Conversation en un solo UnifiedContext
	contextCache          *cognition.Cache                   // Bloque 10 — evita reconstruir el UnifiedContext si nada cambió desde la última vez
	pipeline              *cognition.Pipeline                // Bloque 11 — trace de las 8 etapas del roadmap (Escuchar→...→Guardar experiencia); Process()/execute() marcan por dónde van
	internalState         *cognition.StateManager            // Bloque 15 — Ocupada/Escuchando/Pensando/Ejecutando/Esperando respuesta/Aprendiendo; transiciones en los mismos puntos que marca pipeline.Enter()
	conversationMode      *cognition.ConversationModeManager // Bloque 16 (Fase 2.8) — Task/Discussion/Reflection/Learning/Brainstorming/Emotional; nace antes de generar lenguaje, no reemplaza ningún módulo todavía
	relationship          *personality.Relationship          // Bloque 17 (Fase 2.8) — reconecta personality.Relationship (huérfano); TrustLevel/SharedHistory viven en memory/relationship.json, independientes de ActionConfidence
	conversationFlow      *language.ConversationFlow         // ConversationFlow is retained only as a compatibility/session index for topic summaries and legacy adapters; canonical conversational movement belongs to B96.39.
	conversationalMemory  *dialogue.ConversationalMemory     // Bloque 62 (Fase 5, Capa 5.2) — compatibilidad/índice de temas
	conversationHistory   *dialogue.ConversationHistory      // B96.35 — memoria histórica conversacional canónica
	conversationEpisodeID string                             // B96.35 — episodio de esta ejecución/proceso
	socialMemory          *dialogue.SocialMemory             // Bloque 63 (Fase 5, Capa 5.3) — persiste personas: hechos, relaciones, historia compartida
	skillLibrary          *solving.SkillLibrary              // Integration Fix 2 — mismo *solving.SkillLibrary que ya usan los closures de knowledgeEngine (Bloques 54/58/59); expuesto acá para que Process() pueda consultarlo directo en la cascada de "qué sabes de X"
	knowledgeMemory       *solving.KnowledgeMemory           // Integration Fix 2 — idem, tercer escalón de esa cascada (conocimiento validado por Learning Validator, Bloque 48/49)
	knowledgeDiscoverer   *solving.Discoverer                // Integration Fix 2 — normaliza cualquier target consultado a la misma clave de dominio (discovery.Domain, ej. "Blender") que usa SkillLibrary como índice; sin red, solo matching contra techDomains
	contextAdapter        *language.ContextAdapter           // Bloque 18 — reconecta language.ContextAdapter; PreferredLength ahora deriva de conversationMode.DetailLevel (Bloque 16), primer consumidor real de ese cálculo
	humorStyle            *personality.HumorStyle            // Bloque 19 (Fase 2.8) — reconecta personality.HumorStyle; decide SI tiene sentido bromear (según el modo, Bloque 16) y QUÉ variante (según Relationship, Bloque 17), no inyecta chistes en respuestas todavía
	attention             *cognition.Manager                 // Bloque 12 — filtra el UnifiedContext a lo que realmente importa AHORA, antes de que vaya a memoria de trabajo (MentalAgendaV2)
	workingMemory         *cognition.WorkingMemory           // Bloque 13 — retiene lo que Attention atendió, con TTL propio (5 min), entre Attention y MentalAgenda: "hace un rato hablamos de X" sigue siendo consultable

	// FASE 3 — Cognitive Mind (razonamiento PROSPECTIVO, distinto de reasoning/ que es retrospectivo)
	goalAnalyzer        *solving.Analyzer           // Bloque 21, Capa 3.1 — primer nodo del grafo: transforma una petición en objetivo + subobjetivos + requisitos + supuestos. Sin estado propio.
	problemDecomposer   *solving.Decomposer         // Bloque 22, Capa 3.2 — expande los Subgoals de goalAnalyzer en una jerarquía de tareas. Sin estado propio.
	knowledgeMapper     *solving.Mapper             // Bloque 23, Capa 3.3 — cruza cada Task contra shared.ActionPatterns + learningState.ActionConfidence: qué ya es capacidad real vs. territorio nuevo.
	dependencySolver    *solving.Solver             // Bloque 24, Capa 3.4 — arma el orden secuencial de Tasks y propaga bloqueos a partir de los Gaps() que deja knowledgeMapper. Sin estado propio.
	hypothesisGenerator *solving.Generator          // Bloque 25, Capa 3.5 — combina Gaps() (Bloque 23) y Blocked() (Bloque 24) para proponer la Action más parecida a cada gap y priorizar cuál resolver primero. Sin estado propio.
	constraintSolver    *solving.ConstraintSolver   // Bloque 26, Capa 3.6 — cruza los Requirements de goalAnalyzer contra Gaps()/Blocked() para decir si el plan puede cumplirlos. Sin estado propio.
	decisionEngine      *solving.DecisionEngine     // Bloque 27, Capa 3.7 — junta el HypothesisSet (Bloque 25) y el ConstraintReport (Bloque 26) para decidir si avanzar, ajustar o pedir más información. Sin estado propio.
	analogicalReasoner  *solving.AnalogicalReasoner // Bloque 28, Capa 3.8 — busca en el EventLog (Fase 1-2) un Target ya ejecutado que se parezca a un gap, en vez de comparar solo contra shared.ActionPatterns como Hypothesis Generator. Sin estado propio.
	selfEvaluator       *solving.SelfEvaluator      // Bloque 29, Capa 3.9 (última capa) — no evalúa el plan, evalúa el PROCESO: qué tan bien le fue a cada una de las 8 capas anteriores para este pedido puntual. Sin estado propio.
	reasoningCache      *cognition.ReasoningCache   // Bloque 32, Fase 3.0 (cierre) — evita recalcular la cadena Decompose→Map→Solve→Generate→Check→Decide si el mismo input ya se razonó hace poco y el EventLog no creció desde entonces. Mismo criterio que contextCache (Bloque 10), pero keyed por input en vez de un único valor.

	// CAPA 5.5 - Pensamiento Deliberativo
	perspectiveEngine    *reasoning_deep.PerspectiveEngine
	confidenceCalculator *reasoning_deep.ConfidenceCalculator
	evaluator            *reasoning_deep.Evaluator
	conclusionBuilder    *reasoning_deep.ConclusionBuilder
	conversationPlanner  *reasoning_deep.ConversationPlanner
	mentalAgenda         *reasoning_deep.MentalAgendaV2

	// CAPA 6 - Generación de Lenguaje Natural
	languageGenerator     *language.LanguageGenerator
	conversationGenerator *language.ConversationalGenerator
	responseComposer      *language.ResponseComposer     // Bloque 20 — middleware decorador de respuestas
	interactionManager    *interaction.Manager           // B96.22 — frontera semántica general entre entradas humanas y el runtime
	naturalDialogue       *naturaldialogue.Integration   // B96.39 — estructura/continuidad del diálogo; no autoriza iniciativa
	socialInteraction     *socialinteraction.Integration // B96.38 — conducta social; no autoriza iniciativa
	lastSocialAssessment  socialinteraction.Assessment
	lastSocialPlan        socialinteraction.ResponsePlan

	// B96.40 — Transversal Situation. This is a derived snapshot built from
	// authoritative domain projections; it never becomes a new source of truth.
	lastSituation                situation.Situation
	lastSituationContributions   []situation.Contribution
	lastDecisionEvidence         situation.DecisionEvidence
	pendingSituationObservations []situation.Observation

	// clarificationQueue — B96.22.6. Cola general de preguntas pendientes en
	// la frontera de interacción (distinta de memory.PendingClarification,
	// Bloque 61, que sigue atada solo al flujo de aprendizaje/MoveAsk). Un
	// futuro consumidor (ProductionRuntime, un handler de acciones, una
	// capacidad) empuja acá cuando necesita más información; el turno
	// siguiente se revisa primero contra esta cola antes de clasificarse
	// como algo nuevo, y una pregunta que quedó pendiente porque el usuario
	// se desvió se retoma sola al cerrar el turno que lo desvió.
	clarificationQueue *interaction.ClarificationQueue

	// B96.22 interaction state. These are communication primitives, not
	// capability state: conversation must work even when no work is active.
	conversationState *interaction.ConversationState
	interactionState  *interaction.InteractionState
	threadRouter      *interaction.ThreadRouter
	workBridge        *interaction.WorkBridge
	// B96.22.23 — explicit bridge between conversational continuity and operational work.
	// It never executes or owns work; it only establishes the semantic relationship.
	userProfile                 *interaction.UserProfile
	personalMemory              *interaction.PersonalMemory              // B96.22.18 — durable evidence/history behind the evolving User Model
	userModelStore              *interaction.UserModelStore              // B96.34 — durable formal User Model ownership boundary
	prefModel                   *personality.PreferenceModel             // P4 — posterior de preferencias (olvido), alimentado SOLO por aceptaciones explícitas de B96.34
	competenceOverlay           *competence.Overlay                      // P2.3/T4-M5 (19/10) — lectura de competencia en la situación compartida; nil = sin perspectiva, piso previo intacto
	lastConversationalCognition cognition.ConversationalCognitionE2E     // B96.33 — last real cognition result consumed by Engine
	lastBehavioralDecision      cognition.ConversationalBehaviorDecision // B96.40 — final current-turn conduct decision
	lastNaturalDialogue         naturaldialogue.Decision                 // B96.39 — current dialogue structure consumed by initiative/response flow
	authorityService            *interaction.AuthorityService
	linguisticClient            *linguistics.OllamaClient
	resourceGovernor            *resource.Governor // T3 (2026-10-17) — única ranura de LLM con prioridades reales

	// B96.40.2 — ESPACIO DE TRABAJO + DELIBERACIÓN EN SOMBRA.
	// El modelo recibe una proyección acotada de lo que el arnés ya posee y
	// propone lectura, estrategia, crítica y expectativa. Su propuesta se
	// compara contra la decisión ya tomada y se registra; no la cambia.
	// Habilitado explícitamente por el composition root para que las suites
	// no despierten trabajo de fondo al construir un Engine.
	deliberationClient  deliberation.Client
	deliberationShadow  *deliberation.ShadowLog
	deliberationEnabled bool
	deliberationMu      sync.Mutex
	deliberationRunning bool
	lastDeliberation    deliberation.Record
	// Paso 3 (2026-11-01) - metas propias. El libro guarda propuestas y la
	// decisión del propietario; no registra nada en el planificador ni ejecuta.
	agentGoalLedger  *agency.Ledger
	agentGoalOn      bool
	lastAgentGoalErr string
	// Paso 3.3 (2026-11-01) - gustos propios con evidencia. El registro
	// guarda experiencias verificadas; solo la confirmación del propietario
	// las convierte en evidencia de gusto.
	selfTaste        *taste.Model
	selfTasteOn      bool
	lastSelfTasteErr string
	// Paso 3.4 (2026-11-02) - modo persistente elegido por el propietario.
	// El modelo puede verlo y sugerirlo; solo la puerta del propietario lo
	// cambia. Un archivo inválido apaga el mecanismo (fail closed).
	persistentMode        *persistentmode.Store
	persistentModeOn      bool
	lastPersistentModeErr string
	// Paso 4.1 (2026-11-04) - acta durable de expectativa. El arnés declara
	// antes de ejecutar y emite un veredicto después de observar. El modelo
	// solo puede leerlo y sugerir una hipótesis; nunca escribirlo ni cambiarlo.
	expectationLedger     *expectation.Ledger
	expectationOn         bool
	expectationConfigured bool
	expectationMu         sync.Mutex
	lastExpectation       expectation.Verdict
	lastExpectationSeen   bool
	lastExpectationErr    string
	// Escena 1 (2026-11-04) - prohibición temporal del propietario. Restringe
	// solo su alcance declarado (iniciativa de trabajo u orden concreta); no es
	// un modo durable ni un hecho del propietario. Un archivo ilegible apaga el
	// mecanismo: el arnés no inventa una restricción.
	ownerRestrictions        *restriction.Store
	ownerRestrictionsOn      bool
	lastOwnerRestrictionNote string
	// Escena 1 (2026-11-05) - estado efímero y sensible del propietario. Dolor,
	// cansancio y receptividad con TTL; nunca se guarda como hecho médico del
	// UserModel y nunca se usa como autoridad sobre una orden explícita.
	ownerState         *ownerstate.Store
	ownerStateOn       bool
	lastOwnerStateNote string
	// Escena 2 (2026-11-05) - runtime de interrupciones. El bus de canales es
	// inyectable; el camino crítico es determinista y no consulta al modelo.
	interruptionStore         *interrupt.Store
	interruptionBus           NoticeChannel
	interruptionOn            bool
	interruptionCooldown      time.Duration
	interruptionReminderAfter time.Duration
	interruptionSilence       bool
	lastInterruptionNote      string
	// sceneNow is the injectable clock for scene runs. Nil means time.Now.
	// The scene-1 gates read it; the rest of the engine keeps its own clock.
	sceneNow func() time.Time
	// Consulta acotada del modelo (2026-11-04). El modelo interpreta una frase
	// de descanso que el parser explícito no reconoció; el arnés valida y, como
	// mucho, admite una restricción provisional con fuente confirmed_inference.
	// Nunca corre en el turno: se agenda al final y no bloquea la respuesta.
	consultClient            consult.Client
	consultWorker            *consult.Worker
	modelConsultationOn      bool
	modelConsultationRunning bool
	consultTurnID            string
	consultSeq               uint64
	lastModelConsultation    consult.Decision
	lastModelConsultationErr string
	// Planificación de fondo (2026-11-05): el modelo propone un grafo de tareas
	// mientras el propietario decide. El planner lo valida y lo usa solo si
	// llega antes de generar el primer plan del objetivo; un plan tardío nunca
	// reemplaza uno existente.
	planClient    integration.StructuredGoalPlanner
	planMu        sync.Mutex
	planProposals map[string]goalPlanProposal
	planGoalIDs   map[string]pendingPlanGoal
	planRunning   bool
	// Latido N0+N1 v0 (2026-10-17) — continuidad temporal: percepción de
	// foco (N0), digestión del mundo (N1) y driver del gobernador. Estado
	// bajo heartbeatMu; las fuentes quedan inyectables para tests (defaults
	// reales cableados en StartHeartbeat). Cero LLM.
	heartbeatMu       sync.Mutex
	heartbeat         heartbeatState
	heartbeatPulse    func()                                               // N0: percepción barata del foco (mundo real)
	heartbeatFocusKey func() (string, bool)                                // N0: lectura sin escaneo de la referencia activa
	heartbeatDigest   func() (fingerprint string, summary string, ok bool) // N1: digestión completa del mundo
	heartbeatGovernor func()                                               // N0: driver de PreemptIfNeeded
	// H7 (2026-10-17) — cierre de sesión pedido por el usuario («salir»):
	// el Engine NO termina el proceso (sigue siendo biblioteca para las
	// suites); marca la petición y la raíz viva (main.go) la observa.
	exitMu                   sync.Mutex
	exitRequested            bool
	linguisticInterpreter    *linguistics.Interpreter
	linguisticPipeline       *linguistics.Pipeline
	deferredLinguistic       *linguistics.DeferredResolver // B96.39.35 — delayed linguistic evidence, never a response authority
	lastOpenMeaning          *openMeaningEvidence          // "van juntos" 2026-10-17 — most recent open comprehension evidence for mechanisms
	activeConversationTurnID uint64
	conversationTurnActive   bool
	// D (2026-10-18): una sola habla por turno — último contenido hablado del
	// turno activo (normalizado) para suprimir duplicados exactos.
	speechMu                            sync.Mutex
	lastSpokenTurnID                    uint64
	lastSpokenNormalized                string
	conversationalInitiative            *conversationalinitiative.Policy                    // B96.37.2 — canonical authority for conversational initiative authorization
	initiativeSignals                   *conversationalinitiative.Runtime                   // B96.37.4 — runtime source lifecycle; never decides initiative
	initiativeActionSelector            *conversationalinitiative.ActionSelector            // B96.37.6 — completes authorized decisions; never decides initiative
	initiativeActionContract            *conversationalinitiative.ActionContractBuilder     // B96.37.7 — validates selected action contracts; never authorizes turns
	initiativeTimingEvaluator           *conversationalinitiative.TimingEvaluator           // B96.37.8 — temporal assessment; never authorizes turns
	initiativeAvailabilityPolicy        *conversationalinitiative.AvailabilityPolicy        // B96.37.9 — availability interpretation; never authorizes turns
	initiativePriorityUrgencyPolicy     *conversationalinitiative.PriorityUrgencyPolicy     // B96.37.10 — priority/urgency enrichment; never authorizes turns
	initiativePrivacyPolicy             *conversationalinitiative.DisclosurePolicy          // B96.37.11 — privacy/disclosure enrichment; never authorizes turns
	initiativeInterruptionPolicy        *conversationalinitiative.InterruptionPolicy        // B96.37.12 — interruption admissibility; never authorizes turns
	initiativeConversationLoadEvaluator *conversationalinitiative.ConversationLoadEvaluator // B96.37.13 — load/cooldown assessment; never authorizes turns
	initiativeDeferredIntentManager     *conversationalinitiative.DeferredIntentManager     // B96.37.14 — deferred-intent lifecycle; never authorizes turns
	initiativeScheduler                 *conversationalinitiative.InitiativeScheduler       // B96.37.18 — temporal scheduling only; never authorizes turns
	initiativeOutcomeEvaluator          *conversationalinitiative.OutcomeEvaluator          // B96.37.15 — observes initiative outcomes; never authorizes or learns
	initiativeArbiter                   *conversationalinitiative.InitiativeArbiter         // B96.37.17 — candidate arbitration only; never authorizes turns
	focusArbiter                        *FocusArbiter                                       // B96.37.20+ — unified focus arbitration across interactive/initiative/autonomous
	lastInitiativeProposal              *ProposalContract                                   // pending initiative proposal for FocusArbiter
	lastAutonomousProposal              *ProposalContract                                   // pending autonomous proposal for FocusArbiter
	pendingInteractiveDecision          *interactiveDecisionContext                         // pending interactive decision for FocusArbiter
	autonomyExecutor                    *AutonomyExecutor                                   // B96.37.20+ — autonomous executor for FocusArbiter
	initiativeSimilarityGuard           *interaction.InitiativeSimilarityGuard
	initiativeLifecycleManager          *conversationalinitiative.LifecycleManager        // B96.37.19 — lifecycle/state ownership only; never authorizes turns
	initiativeObservability             *conversationalinitiative.InitiativeObservability // B96.37.20 — observability/decision trace only; never authorizes turns
	// B96.22.12 — last domain-neutral communicative intent selected for the turn.
	// This is the bridge from semantic understanding to response behavior; it never selects a capability.
	lastResponseIntent interaction.ResponseIntent
	flowTrace          *TurnFlowTrace
	lastFlowTrace      *TurnFlowTrace // retained diagnostic trace for adversarial/behavioral validation

	// B94-B96 runtime integration — estas referencias hacen que las capacidades
	// y el dominio Programming formen parte del Engine vivo, no solo de tests
	// aislados. Capability/Production conservan la autoridad; Programming
	// aporta la implementación especializada.
	capabilityRegistry         *capability.Registry
	procedures                 *procedure.Registry
	capabilityDiscovery        *capability.Discovery
	capabilityResolver         *capability.Resolver
	programmingIntegration     *programming.ProgrammingIntegration
	productionProviders        *production.ProviderRegistry
	technicalRecovery          *production.TechnicalRecoveryService
	productionRuntime          *integration.ProductionRuntime
	workStore                  *PersistentWorkStore
	goalStore                  *GoalStore
	capabilityApprovalResolver func(capability.ID) autonomy.Approval // identity/permission owner input; nil means no approval
	programmingWorkspace       *programming.ProgrammingWorkspace
	productionContext          *ProductionContext // B96 integration: latest produced project/artifacts for deterministic follow-up resolution
	goalPlanCompiler           *integration.GoalPlanCompiler
	goalExecutionRunner        *integration.GoalExecutionRunner

	// FASE 6 — Autonomía Cognitiva e Iniciativa (B72-77). Ver el
	// bloque de instanciación en NewEngine para el detalle de cada
	// pieza; acá solo los punteros que Process()/execute()/Shutdown()
	// necesitan tocar en vivo.
	autonomousState              *autonomy.AutonomousState     // Bloque 72 — "¿en qué estado cognitivo está Ahmelia ahora?"
	goalMonitor                  *autonomy.GoalMonitor         // Bloque 73 — vigila inactividad/bloqueo de autonomousState, sin decidir nada
	goalPlanningAdapter          *autonomy.GoalPlanningAdapter // Bloque 90.3 — Goal->Plan->Task->ActionRequest (B81/B82) + Replanner (B87); dueño único, construido en NewEngine, reutilizado por StartAutonomousCycle (antes de B90.3 se recreaba en cada llamada, ver auditoría de B90)
	contextObserver              *autonomy.ContextObserver     // Bloque 74 — buffer de señales; hoy solo se alimenta con GOAL_STATE_CHANGED real (ver Bloque 46)
	initiativeEngine             *autonomy.InitiativeEngine    // Bloque 75 — decide NoAction/Wait/AskUser/... a partir de 72-74
	conversationManager          *autonomy.ConversationManager // Bloque 76 — decide CÓMO entra esa decisión a la conversación
	autonomousLoop               *autonomy.AutonomousLoop      // Bloque 77 — orquesta el tick 74→72/73→75→76 en background
	autonomyCancel               context.CancelFunc            // detiene el ciclo autónomo operativo
	conversationInitiativeCancel context.CancelFunc            // B96.37 — ciclo autónomo conversacional canónico
	lastInitiativeUserTurnAt     time.Time                     // evita repetir una intervención sin nueva respuesta humana
	lastAutonomousTurnAt         time.Time                     // B96.37.13 — autonomous turn requires a new user turn before another intervention
	lastInputChannel             interaction.InputChannel      // canal real de la última interacción; el árbitro lo exige para hablar sin turno del usuario
	autonomyRuntimeMu            sync.Mutex
	autonomySupervisor           *GoroutineSupervisor
	backgroundJobs               backgroundJobRegistry
	backgroundResults            backgroundResultQueue
	backgroundNotifier           func(string)
	asyncProduction              bool
	autonomyDecisionConsumerHook func(autonomy.ConversationDecision) // test/diagnostic seam; never an authority                    // B93.4 — ownership común entre Process() y un tick autónomo completo

	// HUD — callback hacia la interfaz Wails
	onResponse func(string)
}

// SetResponseCallback registra un callback que recibe cada respuesta
// que Ahmelia genera. Usado por la interfaz Wails para mostrar las
// respuestas en el HUD sin modificar la lógica del engine.
func (e *Engine) SetResponseCallback(fn func(string)) {
	e.onResponse = fn
}

// RestoreContext alimenta el ConversationContext con interacciones
// recuperadas de memoria larga (llamar justo después de NewEngine, con
// lo que devuelva learner.LoadContextFromMemory()). Sin esto, el tema
// activo y el historial de "¿en qué estábamos?" arrancan vacíos cada
// vez que Ahmelia reinicia, aunque la memoria larga sí tenga el dato.
func (e *Engine) RestoreContext(interactions []comprehension.Interaction) {
	for _, i := range interactions {
		e.context.Add(i.Input, i.Action, i.Target)
	}
}

// Shutdown vuelca el contexto de conversación a memoria larga y guarda
// todo antes de cerrar. Antes esta lógica vivía solo dentro del caso
// "salir" hablado — si el proceso se cerraba de cualquier otra forma
// (Ctrl+C, cerrar la ventana), no se guardaba nada, ni siquiera la
// memoria base. Ahora main.go también la llama desde un manejador de
// señales para esos casos.
// stopAutonomyAndWait detiene el bucle autónomo de B77 y ESPERA a que su
// goroutine efectivamente termine antes de devolver el control.
//
// B93.2 — HALLAZGO REAL (auditoría de estado compartido, no hipotético):
// antes de este bloque, Shutdown() solo llamaba a e.autonomyCancel() y
// seguía de largo hacia PersistAutonomyState(). Cancelar el contexto solo
// *pide* que la goroutine de AutonomousLoop.Start (autonomy/
// autonomous_loop.go) termine — no espera a que efectivamente lo haga. Esa
// goroutine puede seguir varios milisegundos más dentro de un
// Cycle.RunStep ya en curso, mutando planner/goalPlanningAdapter/
// autonomousState (los mismos tres punteros que PersistAutonomyState()
// lee) sin que nada lo serialice contra esa lectura: un data race real
// entre "Shutdown lee para persistir" y "el tick en curso sigue
// escribiendo", que -race detectaría si el timing lo atrapa.
//
// autonomousLoop.Stop() ya existe desde B77 y ya hace exactamente
// wg.Wait() — nadie lo llamaba desde Shutdown. Con esto, ningún lector de
// estado compartido corre hasta que la única goroutine que lo escribe
// efectivamente terminó: el contrato de ownership que B93.2 exige, no una
// corrección cosmética. nil-safe en ambos campos: un Engine donde nunca se
// llamó a StartAutonomy (ej. la mayoría de los tests que instancian
// Engine directo) no tiene nada que detener.
func (e *Engine) stopAutonomyAndWait() {
	if e.conversationInitiativeCancel != nil {
		e.conversationInitiativeCancel()
	}
	if e.autonomyCancel != nil {
		e.autonomyCancel()
	}
	if e.autonomousLoop != nil {
		e.autonomousLoop.Stop()
	}
}

// RequestExit marca que la sesión fue cerrada a pedido explícito del
// usuario («salir») y que la raíz viva debe terminar el proceso después
// de responder. El Engine no mata el proceso por sí mismo — las suites lo
// instancian —; main.go observa ExitRequested() tras cada turno. H7
// (2026-10-17): antes, «salir» guardaba todo con Shutdown() pero el
// proceso quedaba vivo en el lazo de voz y había que matarlo a mano.
func (e *Engine) RequestExit() {
	e.exitMu.Lock()
	e.exitRequested = true
	e.exitMu.Unlock()
}

// ExitRequested indica si una ruta de cierre explícito («salir») pidió
// terminar el proceso. Solo la raíz viva decide cómo cerrar.
func (e *Engine) ExitRequested() bool {
	e.exitMu.Lock()
	defer e.exitMu.Unlock()
	return e.exitRequested
}

func (e *Engine) Shutdown() {
	// Latido N0+N1 (2026-10-17): el pulso de continuidad no debe
	// sobrevivir al cierre del proceso.
	e.stopHeartbeat()
	// B96.39.35 — stop the deferred linguistic worker before persisting the
	// rest of the Engine. Its results are process-local evidence and must not
	// outlive shutdown.
	if e.deferredLinguistic != nil {
		e.deferredLinguistic.Stop()
	}
	// Fase 6 (B77) — ver stopAutonomyAndWait para el detalle de por qué
	// esto ya no es un simple e.autonomyCancel() suelto (hallazgo real
	// de B93.2).
	e.stopAutonomyAndWait()

	// B90.7 — guardado final del subsistema autónomo. Los guardados tras
	// cada mutación (persistingCycleAdapter) ya cubren el caso normal;
	// esto es la red de seguridad para el caso en que el proceso se
	// cierre entre una mutación real y su guardado, o si algún guardado
	// intermedio falló (ver persistAutonomyStateBestEffort).
	if e.planner != nil && e.autonomousState != nil && e.goalPlanningAdapter != nil {
		if err := e.PersistAutonomyState(); err != nil {
			fmt.Printf("⚠️ [Shutdown] no se pudo persistir el estado autónomo (B90): %v\n", err)
		} else {
			fmt.Println("💾 [Shutdown] Estado autónomo (B90) guardado")
		}
	}

	contextInteractions := e.context.GetContext(10)
	fmt.Printf("💾 [Shutdown] Interacciones en contexto a volcar: %d\n", len(contextInteractions))
	for _, ci := range contextInteractions {
		fmt.Printf("💾 [Shutdown]   - input=%q action=%q target=%q\n", ci.Input, ci.Action, ci.Target)
	}

	if len(contextInteractions) > 0 {
		learner := system.NewLearningSystem(e.longMem)
		learner.DumpContextToMemory(contextInteractions)
		fmt.Println("💾 [Shutdown] DumpContextToMemory ejecutado")
	} else {
		fmt.Println("💾 [Shutdown] Nada que volcar: el contexto de esta sesión está vacío")
	}

	if e.personalMemory != nil {
		if err := e.personalMemory.Save(interaction.PersonalMemoryPath); err != nil {
			fmt.Printf("⚠️ [Shutdown] no se pudo persistir memoria personal: %v\n", err)
		}
	}
	memory.SaveLongMemory(e.longMem)
	SaveShortMemory(*e.shortMem)

	// Memoria episódica: un resumen de TODA la sesión (no solo las
	// últimas 10 interacciones), con key única por sesión — a
	// diferencia de DumpContextToMemory, esto no se sobreescribe entre
	// sesiones distintas.
	if summary, ok := e.episode.Summarize(e.longMem, e.world); ok {
		fmt.Println("📖 [Episodio] " + summary)
		memory.SaveLongMemory(e.longMem)
	}

	// B96.35 — cierra el episodio conversacional COMPLETO. El histórico ya
	// fue persistido turno a turno; aquí solo cambia su estado y aplica la
	// política formal de retención. Esto evita depender exclusivamente de
	// Shutdown para no perder una sesión parcialmente registrada.
	if e.conversationHistory != nil && e.conversationEpisodeID != "" {
		snap := e.conversationState.Snapshot()
		summary := fmt.Sprintf("Sesión con %d turnos; último tema: %s", len(snap.Turns), snap.ActiveTopic)
		e.conversationHistory.CompleteSession(e.conversationEpisodeID, summary, time.Now())
		if err := e.conversationHistory.Save(dialogue.ConversationHistoryPath); err != nil {
			fmt.Printf("⚠️ [B96.35] no se pudo guardar memoria histórica: %v\n", err)
		}
	}

	fmt.Println("💾 [Shutdown] Memoria guardada en disco")
}

// ============================================================================
// FASE 6 (B72-77) — arranque del bucle autónomo en background
// ============================================================================
//
// Deliberadamente separado de NewEngine(): construir el Engine nunca
// debe arrancar goroutines por sí solo (mismo criterio que ya regía
// para el resto del Engine — nada en NewEngine hace I/O de red ni
// arranca procesos). StartAutonomy() es lo que main.go llama una vez,
// después de tener el Engine ya armado, cuando decide que es momento
// de que el ciclo 74→72/73→75→76 empiece a correr de verdad.

// autonomyStateAdapter implementa autonomy.StateProvider (la interfaz
// que pide AutonomousLoop, Bloque 77) sobre AutonomousState real
// (Bloque 72) — no reimplementa nada, solo adapta la firma.
type autonomyStateAdapter struct {
	state *autonomy.AutonomousState
}

func (a autonomyStateAdapter) LastUserActivity() time.Time {
	return a.state.LastInteraction()
}

// autonomyGoalAdapter implementa autonomy.GoalProvider sobre
// GoalMonitor real (Bloque 73). "Pendiente de retomar" para B77 es el
// primer objetivo Stale que 73 encontró (Active + inactivo más allá
// del umbral) — no cualquier entrada cruda de PendingGoals(), porque
// "vale la pena recordarlo" es justamente el filtro que StaleGoals()
// ya aplica; usar la cola sin filtrar dispararía ResumeGoal sobre un
// objetivo recién creado que todavía no tuvo tiempo de quedar inactivo.
type autonomyGoalAdapter struct {
	monitor *autonomy.GoalMonitor
}

func (a autonomyGoalAdapter) PendingGoal() (hasPending bool, goalID string) {
	// CurrentGoal is already the canonical ownership decision made by
	// GoalStore.ActivateOrQueue. Waiting for it to become stale before the
	// autonomous cycle can even start turned "execute this active goal" into
	// "remember this forgotten goal after 20 minutes". Those are different
	// semantics. An active current goal is immediately eligible for the
	// initiative gate; stale pending goals remain the fallback for resume.
	if a.monitor != nil {
		if current := a.monitor.CurrentGoal(); current != nil {
			switch current.Status {
			case planning.GoalActive:
				return true, current.ID
			}
		}
	}

	stale := a.monitor.StaleGoals()
	if len(stale) == 0 {
		return false, ""
	}
	return true, stale[0].Goal.ID
}

// StartAutonomy arma el AutonomousLoop (77) con los adaptadores reales
// sobre 72/73, lo arranca en background sobre un context derivado de
// ctx, y levanta el consumidor de su OutputChan. Guarda el cancel para
// que Shutdown() pueda cortar limpio. Llamar como máximo una vez por
// Engine — llamar de nuevo simplemente reemplaza autonomyCancel sin
// cortar la corrida anterior, así que es responsabilidad de quien
// llama (main.go) no invocarlo dos veces.
// StartDeferredLinguistic starts the single background worker responsible
// for delayed linguistic interpretation. The worker produces evidence only;
// it never emits a response or authorizes initiative.
func (e *Engine) StartDeferredLinguistic(ctx context.Context) {
	if e == nil || e.deferredLinguistic == nil {
		return
	}
	// Model lifecycle starts once at the Engine boundary. The live turn keeps
	// its short interaction budget; the shared client warms the local model in
	// the background so the deferred path is not the first request that pays
	// the cold-load cost.
	if e.linguisticClient != nil {
		e.linguisticClient.Warmup(ctx)
		go func(client *linguistics.OllamaClient, parent context.Context) {
			warmCtx, cancel := context.WithTimeout(parent, 180*time.Second)
			defer cancel()
			if err := client.WaitReady(warmCtx); err != nil {
				fmt.Printf("🔎 OLLAMA READY | model=%s result=error err=%q\n", client.Model, err)
				return
			}
			fmt.Printf("🔎 OLLAMA READY | model=%s result=ok\n", client.Model)
		}(e.linguisticClient, ctx)
	}
	e.deferredLinguistic.SetObserver(func(result linguistics.DeferredResult) {
		elapsed := time.Since(result.Job.EnqueuedAt)
		if result.Error != "" {
			fmt.Printf("🔎 LINGUISTIC_DEFERRED | job=%s elapsed=%.1fs result=error err=%q\n", result.Job.ID, elapsed.Seconds(), result.Error)
			return
		}
		if result.Open != nil {
			fmt.Printf("🔎 LINGUISTIC_DEFERRED | job=%s elapsed=%.1fs result=ok comprehension=open purpose=%s topic=%q confidence=%.2f\n", result.Job.ID, elapsed.Seconds(), result.Open.Purpose, result.Open.Topic, result.Open.Confidence)
			return
		}
		fmt.Printf("🔎 LINGUISTIC_DEFERRED | job=%s elapsed=%.1fs result=ok relations=%d entities=%d\n", result.Job.ID, elapsed.Seconds(), len(result.Interpretation.Relations), len(result.Interpretation.Entities))
	})
	e.deferredLinguistic.Start(ctx)
}

func (e *Engine) StartAutonomy(ctx context.Context) {
	if err := e.StartAutonomousCycle(ctx); err != nil {
		// B77 sigue siendo el fallback de compatibilidad si la composición
		// de B88 no puede construirse; no se inventa un ciclo parcialmente
		// configurado ni se deja una goroutine inútil corriendo.
		fmt.Printf("⚠️ [Autonomía B88] no se pudo construir el ciclo integrado: %v\n", err)
		loopCtx, cancel := context.WithCancel(ctx)
		e.autonomyCancel = cancel
		e.autonomousLoop = autonomy.NewAutonomousLoop(
			autonomy.LoopConfig{},
			e.contextObserver,
			e.initiativeEngine,
			e.conversationManager,
			autonomyStateAdapter{state: e.autonomousState},
			autonomyGoalAdapter{monitor: e.goalMonitor},
		)
		e.autonomousLoop.SetRunLock(&e.autonomyRuntimeMu)
		e.autonomousLoop.Start(loopCtx)
		e.startSupervisedAutonomyDecisionConsumer(loopCtx)
	}
}

// consumeAutonomyDecisions — B77 emite ConversationDecision por su
// OutputChan; sin esto, el ciclo completo 72-77 quedaría corriendo sin
// ningún efecto observable, el mismo tipo de "cableado huérfano" que
// ya se corrigió antes para AgendaManager/Timeline/Relaciones. Alcance
// honesto, mismo criterio que el resto de esta fase: solo se
// implementa lo que hoy tiene un canal de salida real.
//   - StrategyInterruptLightly / StrategyResumeTopic: sí hay un canal
//     real — via the canonical response output boundary — así que se dice ahora mismo.
//   - StrategyAppendContext: necesitaría una cola de "agregar a la
//     próxima respuesta" que no existe en ningún lado del pipeline
//     todavía (Process() no lee de ningún buffer de este tipo); en vez
//     de inventarla acá, queda documentado como pendiente real, no
//     como un olvido — se loguea para no perder la señal.
//   - StrategyDefer / StrategyNone: por diseño de B76 no producen
//     salida — no hay nada que hacer acá.
func (e *Engine) consumeAutonomyDecisions(ctx context.Context) {
	// Legacy B72-77 conversational strategies are no longer a response authority.
	// Operational autonomy may still emit decisions for compatibility, but no
	// legacy strategy is allowed to speak into the conversation. B96.37 is the
	// sole conversational initiative authority.
	for {
		select {
		case <-ctx.Done():
			return
		case decision, ok := <-e.autonomousLoop.OutputChan():
			if !ok {
				return
			}
			if e.autonomyDecisionConsumerHook != nil {
				e.autonomyDecisionConsumerHook(decision)
			}
			if decision.Strategy == autonomy.StrategyAppendContext {
				fmt.Println("🧭 [Legacy autonomy] conversational output suppressed; B96.37 owns initiative")
			}
		}
	}
}

func (e *Engine) startSupervisedAutonomyDecisionConsumer(ctx context.Context) {
	if e == nil || e.autonomousLoop == nil {
		return
	}
	if e.autonomySupervisor == nil {
		e.autonomySupervisor = NewGoroutineSupervisor()
	}
	e.autonomySupervisor.Start(ctx, "core.consumeAutonomyDecisions", e.consumeAutonomyDecisions)
}

// StartConversationalInitiative starts the canonical B96.37 idle dialogue loop.
// It is deliberately separate from the legacy operational autonomy loop: the
// latter cannot speak, ask, resume topics, or create conversational turns.
func (e *Engine) StartConversationalInitiative(ctx context.Context) {
	if e == nil || e.initiativeSignals == nil || e.conversationalInitiative == nil || e.conversationState == nil {
		return
	}
	if e.conversationInitiativeCancel != nil {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	e.conversationInitiativeCancel = cancel
	go e.runConversationalInitiativeLoop(loopCtx)
}

func (e *Engine) runConversationalInitiativeLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.autonomyRuntimeMu.Lock()
			e.evaluateIdleConversationalInitiative()
			e.runIdleFocusArbitration(ctx)
			e.autonomyRuntimeMu.Unlock()
		}
	}
}

// buildConversationalRuntimeFacts exposes only grounded state to the language
// renderer. It is deliberately generic: no user phrase or specific question is
// hardcoded here. The renderer can therefore answer self-state questions and
// formulate autonomous turns from the same state regardless of wording.
func (e *Engine) buildConversationalRuntimeFacts(snap interaction.ConversationSnapshot) language.RuntimeFacts {
	facts := language.RuntimeFacts{
		ActiveTopic: snap.ActiveTopic, ActiveObjective: snap.ActiveObjective,
		ActiveWorkID: snap.ActiveWorkID, SystemReady: true, DecisionEvidence: e.lastDecisionEvidence,
	}
	if e != nil && e.autonomousState != nil {
		facts.ActiveTask = e.autonomousState.ActiveTask()
	}
	if state, ok := e.persistentModeState(); ok {
		facts.PersistentMode = persistentModeSummary(state)
	}
	if verdict := e.expectationRuntimeSummary(); verdict != "" {
		facts.LastExpectation = verdict
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXPECTATION", verdict)
		}
	}
	return facts
}

func initiativeCandidateSemanticLabel(candidate interaction.QuestionCandidate) string {
	purpose := strings.TrimSpace(string(candidate.Purpose))
	reason := strings.TrimSpace(candidate.Reason)
	topic := strings.TrimSpace(candidate.Topic)
	parts := make([]string, 0, 3)
	if purpose != "" {
		parts = append(parts, "purpose="+purpose)
	}
	if topic != "" {
		parts = append(parts, "topic="+topic)
	}
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}
	return strings.Join(parts, "; ")
}

func (e *Engine) initiativeResponseStrategy(proposal conversationalinitiative.InterventionProposal) cognition.ConversationalResponseStrategy {
	strategy := e.lastConversationalCognition.Strategy
	if strategy.Version == "" {
		strategy = cognition.ConversationalResponseStrategy{
			Version: "B96.33.10", Mode: cognition.ResponseModeDiscuss,
			Directness: cognition.DirectnessExploratory, Structure: cognition.StructureDiscussion,
			Initiative: cognition.InitiativeHelpful, PreserveUserAgency: true,
			ShouldGenerateLanguage: true, Reason: "authorized initiative intervention",
		}
	}
	strategy.PreserveUserAgency = true
	strategy.ShouldGenerateLanguage = true
	strategy.ExecutionBoundary = true
	strategy.Confidence = maxFloat(strategy.Confidence, proposal.Confidence)
	strategy.Reason = "B96.37 InterventionProposal rendered through Dialogue and Language"
	switch proposal.ContentMode {
	case conversationalinitiative.ContentQuestion:
		strategy.Mode = cognition.ResponseModeDiscuss
		strategy.Directness = cognition.DirectnessExploratory
		strategy.Structure = cognition.StructureDiscussion
	case conversationalinitiative.ContentSuggestion:
		strategy.Mode = cognition.ResponseModePropose
		strategy.Directness = cognition.DirectnessGuided
		strategy.Structure = cognition.StructureProposal
	case conversationalinitiative.ContentProposal:
		strategy.Mode = cognition.ResponseModePropose
		strategy.Directness = cognition.DirectnessGuided
		strategy.Structure = cognition.StructureProposal
	case conversationalinitiative.ContentWarning, conversationalinitiative.ContentDisagreement:
		strategy.Mode = cognition.ResponseModeDiscuss
		strategy.Directness = cognition.DirectnessDirect
		strategy.Structure = cognition.StructureExplanation
	case conversationalinitiative.ContentReminder, conversationalinitiative.ContentContinuation, conversationalinitiative.ContentRedirection:
		strategy.Mode = cognition.ResponseModeDiscuss
		strategy.Structure = cognition.StructureDiscussion
	}
	return strategy
}

func isDuplicateInitiativeResponse(candidate, previous string) bool {
	candidate = strings.TrimSpace(candidate)
	previous = strings.TrimSpace(previous)
	return candidate != "" && previous != "" && candidate == previous
}

func (e *Engine) evaluateIdleConversationalInitiative() {
	if e.conversationState == nil || e.conversationalInitiative == nil || e.initiativeSignals == nil || e.conversationTurnActive {
		return
	}
	snap := e.conversationState.Snapshot()
	now := time.Now()

	// B96.37 evaluates an opportunity from state, not from a question template.
	// A session with no user turn is still a valid conversational state: Ahmelia
	// may initiate once the runtime has settled. After a user turn, the same
	// pipeline reevaluates continuity after a genuine silence window.
	hasUserTurn := strings.TrimSpace(snap.LastUserInput) != ""
	if hasUserTurn && now.Sub(snap.LastTurnAt) < 8*time.Second {
		return
	}
	// Deuda personal propia: los jobs diferidos ya resueltos que son consultas
	// sobre un dato del usuario se pagan ANTES de la guarda de rendición, de
	// las señales y del arbitraje de iniciativa. Una consulta personal no es
	// una intervención autónoma: es la respuesta que el usuario ya espera, y no
	// puede quedar a merced del arbitraje (gap de la re-demo 2026-10-18, turno
	// 3: la repetición con el dato capturado respondió el piso y una invitación
	// vacía ocupó el turno autónomo).
	if e.payReadyPersonalQueries() {
		return
	}
	// Once Ahmelia has taken an autonomous turn, silence alone cannot create
	// another turn. A new user turn must be observed first. This is the actual
	// yield boundary; it applies equally after startup and after a mid-dialogue
	// intervention.
	if !e.lastAutonomousTurnAt.IsZero() {
		if snap.LastTurnAt.Equal(e.lastAutonomousTurnAt) || snap.LastTurnAt.Before(e.lastAutonomousTurnAt) {
			return
		}
	}
	if !e.lastInitiativeUserTurnAt.IsZero() && hasUserTurn && snap.LastTurnAt.Equal(e.lastInitiativeUserTurnAt) {
		return
	}

	deferredReady := []linguistics.DeferredResult(nil)
	if e.deferredLinguistic != nil {
		deferredReady = e.deferredLinguistic.Ready()
	}

	req := interaction.SemanticRequest{Raw: snap.LastUserInput, Normalized: snap.LastUserInput, Kind: interaction.KindConversation, Language: snap.Language, Topic: snap.ActiveTopic, Channel: e.lastInputChannel}
	if !hasUserTurn {
		req.Normalized = ""
	}
	var workContext interaction.WorkContext
	hasWork := false
	if workID := strings.TrimSpace(snap.ActiveWorkID); workID != "" && e.workStore != nil {
		if work, ok := e.workStore.Get(workID); ok {
			workContext = workContextFromSession(work)
			hasWork = true
		}
	}
	idleSituation, idleContributions := e.buildSharedSituation(req, snap, workContext, hasWork, nil)
	// Add UserModel contribution so it flows into DecisionEvidence for initiative
	if umc := e.userModelContribution(now); umc != nil {
		idleContributions = append(idleContributions, *umc)
	}
	idleContributions = attachContributionsToSituation(idleSituation, idleContributions...)
	idleIteration := situation.NewIterationContext("idle_initiative", idleSituation, idleContributions)
	e.lastSituation = idleSituation
	e.lastSituationContributions = idleContributions
	idleDecisionEvidence := idleIteration.Evidence
	e.lastDecisionEvidence = idleDecisionEvidence
	dialogue := e.naturalDialogue.EvaluateRequest(req, snap, naturaldialogue.UserTurn)
	if hasUserTurn && len(deferredReady) == 0 && (dialogue.RequiresDirectTurn || !dialogue.AllowProactiveFollowUp) {
		return
	}

	// Candidate generation is evidence-based. No canned question is selected
	// here. The candidate says only that conversational participation is
	// available; B96.37.2 still decides whether Ahmelia actually speaks.
	candidateTopic := strings.TrimSpace(snap.ActiveTopic)
	if candidateTopic == "" {
		candidateTopic = strings.TrimSpace(snap.ActiveObjective)
	}
	if candidateTopic == "" {
		candidateTopic = "conversation"
	}
	candidateConfidence := 0.72
	if len(snap.Turns) == 0 {
		candidateConfidence = 0.78
	}
	deferredEvidence := make([]conversationalinitiative.DeferredLinguisticEvidence, 0, len(deferredReady))
	for _, result := range deferredReady {
		meaning, confidence := result.Interpretation.Meaning, result.Interpretation.Confidence
		if result.Open != nil {
			meaning, confidence = result.Open.Meaning, result.Open.Confidence
		}
		deferredEvidence = append(deferredEvidence, conversationalinitiative.DeferredLinguisticEvidence{
			JobID: result.Job.ID, RawInput: result.Job.Request.Raw, Meaning: meaning,
			Confidence: confidence, CompletedAt: result.CompletedAt,
		})
	}
	input := conversationalinitiative.SignalInput{
		Now: now, RequestKind: string(req.Kind), ActiveTopic: snap.ActiveTopic, ActiveObjective: snap.ActiveObjective,
		ActiveWorkID: snap.ActiveWorkID, DecisionEvidence: idleDecisionEvidence, DeferredLinguistic: deferredEvidence,
		LastUserInput: snap.LastUserInput, LastAssistant: snap.LastAssistant, TurnCount: len(snap.Turns),
		OpenThreadCount: len(snap.OpenThreads), CommitmentCount: len(snap.Commitments), CandidateExists: true,
		CandidateTopic: candidateTopic, CandidateText: "", CandidateConfidence: candidateConfidence,
		Channel: string(req.Channel), HistoricalAvailable: e.conversationHistory != nil,
		UserUrgency: snap.UserState.Urgency, UserFrustration: snap.UserState.Frustration, UserConfusion: snap.UserState.Confusion,
		UserAvailability: conversationalinitiative.AvailabilityUnknown, Privacy: conversationalinitiative.InformationAllowed,
		Interruptibility: conversationalinitiative.InterruptSafe, ConversationLoad: conversationalinitiative.LoadLow,
		RequiresDirectTurn: false,
	}

	if e.initiativeAvailabilityPolicy != nil {
		a := e.initiativeAvailabilityPolicy.Evaluate(conversationalinitiative.AvailabilityEvidence{Explicit: input.UserAvailability, LastUserActivity: snap.LastTurnAt, CurrentTurnActive: false, WaitingForUser: false, Now: now})
		if err := conversationalinitiative.ValidateAvailabilityAssessment(a); err != nil || a.Availability == conversationalinitiative.AvailabilityUnknown {
			return
		}
		input.UserAvailability = a.Availability
	}

	signals := e.initiativeSignals.Collect(input)
	ops := conversationalinitiative.BuildCandidateOpportunities(signals, input)
	prepared := make([]conversationalinitiative.InitiativeOpportunity, 0, len(ops))
	for _, op := range ops {
		prepared = append(prepared, e.prepareInitiativeOpportunity(op, input, snap, now))
	}
	var opportunity conversationalinitiative.InitiativeOpportunity
	if e.initiativeArbiter != nil {
		if result, err := e.initiativeArbiter.Arbitrate(conversationalinitiative.NewArbitrationInput(prepared, e.initiativeSignals.InitiativeAdaptationSignals(now), e.initiativeTimingEvaluator, now)); err == nil && result.Selected.Valid {
			opportunity = result.Selected.Opportunity
		}
	}
	if !opportunity.Exists {
		opportunity = conversationalinitiative.BuildOpportunity(nil, input)
	}

	decision := e.conversationalInitiative.Evaluate(opportunity, now)
	if e.initiativeActionSelector != nil {
		var err error
		decision, err = e.initiativeActionSelector.FinalizeDecision(decision, now)
		if err != nil {
			return
		}
	}
	contract, err := e.initiativeActionContract.Build(decision)
	if err != nil || decision.Decision != conversationalinitiative.Intervene {
		return
	}

	var deferredResult linguistics.DeferredResult
	isDeferredDelivery := decision.Source == conversationalinitiative.SourceDeferredLinguistic || opportunity.Source == conversationalinitiative.SourceDeferredLinguistic
	if isDeferredDelivery && e.deferredLinguistic != nil {
		jobID := strings.TrimSpace(opportunity.Target)
		if jobID == "" {
			jobID = strings.TrimSpace(decision.Target)
		}
		if jobID == "" {
			return
		}
		result, ok := e.deferredLinguistic.Get(jobID)
		if !ok {
			return
		}
		// Deferred linguistic evidence may enrich cognition, but an interpretation
		// of an executable user turn is not an autonomous conversational
		// opportunity. It must return through the normal interaction/execution
		// path rather than becoming a delayed initiative after the user has moved on.
		if interaction.CanonicalActs(result.Job.Request).Has(interaction.ActCommand) ||
			result.Job.Request.Kind == interaction.KindProduction ||
			result.Job.Request.Kind == interaction.KindAction ||
			result.Job.Request.Continues || result.Job.Request.FollowUp {
			if e.flowTrace != nil {
				e.flowTrace.Mark("INITIATIVE", fmt.Sprintf("deferred job=%s suppressed: executable/continuation evidence", jobID))
			}
			return
		}
		deferredResult = result
		req = result.Job.Request
		if result.Open != nil {
			om := *result.Open
			req = mergeOpenMeaning(req, om)
			e.adoptOpenMeaning(result.Job.Request.Raw, om)
		} else {
			if strings.TrimSpace(result.Interpretation.Topic) != "" {
				req.Topic = strings.TrimSpace(result.Interpretation.Topic)
			}
			req = mergeLinguisticInterpretation(req, result.Interpretation)
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("INITIATIVE", fmt.Sprintf("decision=%s source=deferred_linguistic job=%s", decision.Decision, jobID))
		}
		// Lectura abierta tardía de una consulta personal: el turno vivo ya no
		// podía esperarla (presupuesto sync), pero la deuda honesta — reconfirmar
		// y abrir la admisión — sigue igual. La entrega ya pasó el gobierno de
		// iniciativa; aquí se decide el CONTENIDO: ask-back o silencio — jamás
		// una intervención sin contenido por una consulta personal.
		if e.deliverLatePersonalQuery(result) {
			return
		}
		// Fase 2 (2026-10-18): la resolución de una lectura diferida no es
		// contenido por sí misma. Si la lectura no produjo un resultado
		// tipado (consulta personal o acto de feedback), la entrega se cierra
		// en silencio: la intervención genérica solo repetía "encontré algo
		// útil" sin nada útil que decir (defecto vivo de la corrida 18/10).
		if e.deliverLateTypedAct(result) {
			return
		}
		if e.deferredLinguistic != nil {
			e.deferredLinguistic.MarkDelivered(result.Job.ID)
		}
		if e.flowTrace != nil {
			purpose := "closed"
			if result.Open != nil {
				purpose = string(result.Open.Purpose)
			}
			e.flowTrace.Mark("INITIATIVE", fmt.Sprintf("deferred job=%s suppressed: no typed outcome purpose=%s", result.Job.ID, purpose))
		}
		return
	}

	var autonomousTrace *TurnFlowTrace
	if flowTraceEnabled() {
		autonomousTrace = NewTurnFlowTrace("<autonomous-initiative>")
		autonomousTrace.Mark("INITIATIVE-AUTONOMOUS", fmt.Sprintf("signals=%d opportunity=%s/%s decision=%s action=%s confidence=%.2f", len(signals), opportunity.Source, opportunity.Reason, decision.Decision, contract.Action, decision.Confidence))
		if isDeferredDelivery {
			autonomousTrace.Mark("INITIATIVE", fmt.Sprintf("decision=%s source=deferred_linguistic job=%s", decision.Decision, deferredResult.Job.ID))
		}
		defer autonomousTrace.PrintFinal()
	}

	// Store initiative proposal for FocusArbiter instead of executing directly
	initiativeProp := FromIdleInitiative(e, idleSituation, idleContributions, opportunity, decision)
	if initiativeProp != nil {
		initiativeProp.Initiative = &InitiativeDelivery{Request: req, Opportunity: opportunity, Decision: decision, Topic: candidateTopic, Idle: true}
		e.lastInitiativeProposal = initiativeProp
		if e.flowTrace != nil {
			e.flowTrace.Mark("INITIATIVE", fmt.Sprintf("proposal_stored=%s kind=%s", initiativeProp.Intention, initiativeProp.Kind))
		}
	}
	return // Let FocusArbiter handle execution

}

// runFocusArbitration runs the unified FocusArbiter across all proposal sources.
// It collects proposals from interactive, initiative, and autonomous routes,
// runs the FocusArbiter, and executes the winning proposal if any.
// Returns true if a proposal was selected and handled.
func (e *Engine) runFocusArbitration(ctx context.Context, t *processTurn, interactiveProp *ProposalContract) bool {
	if e.focusArbiter == nil {
		return false
	}

	var proposals []*ProposalContract

	// 1. Interactive proposal (from current turn's cognition/dialogue)
	if interactiveProp != nil {
		proposals = append(proposals, interactiveProp)
	} else if t != nil && e.lastConversationalCognition.Valid {
		sit := e.lastSituation
		contrib := e.lastSituationContributions
		if sit.ID != "" {
			interactiveProp := FromInteractiveDecision(t, e, sit, contrib)
			if interactiveProp != nil {
				proposals = append(proposals, interactiveProp)
			}
		}
	}

	// 2. Initiative proposal (from idle evaluation)
	// Check if there's a pending initiative decision
	if e.lastInitiativeProposal != nil {
		proposals = append(proposals, e.lastInitiativeProposal)
		e.lastInitiativeProposal = nil // consume it
	}

	// 3. Autonomous proposal (from cycle)
	if e.lastAutonomousProposal != nil {
		proposals = append(proposals, e.lastAutonomousProposal)
		e.lastAutonomousProposal = nil // consume it
	}

	if len(proposals) == 0 {
		return false
	}
	return e.arbitrateAndExecute(ctx, proposals)
}

// runIdleFocusArbitration is the idle entry into the SAME arbiter. Called from
// the 2 s loop right after evaluateIdleConversationalInitiative, so a pending
// initiative proposal is arbitrated (and delivered) without needing a user
// turn. Only the initiative source is collected here: the autonomous source
// still emits a placeholder action and must not be executed from a timer.
func (e *Engine) runIdleFocusArbitration(ctx context.Context) bool {
	if e == nil || e.focusArbiter == nil || e.lastInitiativeProposal == nil {
		return false
	}
	pending := e.lastInitiativeProposal
	e.lastInitiativeProposal = nil // consumed whether or not it wins
	return e.arbitrateAndExecute(ctx, []*ProposalContract{pending})
}

// arbitrateAndExecute runs the unified FocusArbiter over already-collected
// proposals and executes the winner. It is the single place where a proposal
// is allowed to become behavior, for turn-driven and idle-driven callers alike.
func (e *Engine) arbitrateAndExecute(ctx context.Context, proposals []*ProposalContract) bool {
	now := time.Now()
	result, err := e.focusArbiter.Arbitrate(proposals, now)
	if err != nil || result.Selected == nil {
		return false
	}

	// Execute the winning proposal
	selected := result.Selected
	trace := e.flowTrace
	if trace == nil && flowTraceEnabled() {
		trace = NewTurnFlowTrace("<idle-focus-arbitration>")
		defer trace.PrintFinal()
	}
	if trace != nil {
		score := 0.0
		if len(result.Candidates) > 0 {
			score = result.Candidates[0].Score
		}
		trace.Mark("FOCUS_ARBITER", fmt.Sprintf("selected=%s kind=%s score=%.3f reasoning=%s",
			selected.Intention, selected.Kind, score, result.Reasoning))
	}

	// Execute based on proposal kind
	switch selected.Kind {
	case ProposalKindInteractive:
		// Execute the interactive decision using the stored context
		return e.executeInteractiveDecision(selected)

	case ProposalKindInitiative:
		// Execute initiative proposal
		return e.executeInitiativeProposal(selected)

	case ProposalKindAutonomous:
		// Execute autonomous proposal
		return e.executeAutonomousProposal(ctx, selected)

	default:
		return false
	}
}

// executeInitiativeProposal delivers an arbitrated initiative proposal through
// the same Dialogue/Language path the interactive initiative phase uses.
func (e *Engine) executeInitiativeProposal(p *ProposalContract) bool {
	if e == nil || p == nil || p.Initiative == nil {
		return false
	}
	return e.deliverInitiativeIntervention(p.Initiative)
}

// executeAutonomousProposal executes an autonomous proposal through the autonomy executor
func (e *Engine) executeAutonomousProposal(ctx context.Context, p *ProposalContract) bool {
	if p.Action == nil {
		return false
	}
	// Create ActionRequest from proposal
	req := autonomy.ActionRequest{
		ID:         p.ID,
		Action:     p.Action.Name,
		Objective:  p.Intention,
		Target:     p.Action.Target,
		Parameters: p.Action.Parameters,
	}
	resp := e.autonomyExecutor.ExecuteAction(ctx, req)
	return resp.Executed && resp.Error == nil
}

// executeInteractiveDecision executes the interactive decision using the stored context
func (e *Engine) executeInteractiveDecision(p *ProposalContract) bool {
	ctx := e.pendingInteractiveDecision
	if ctx == nil || ctx.turn == nil {
		return false
	}
	t := ctx.turn

	// IntentSearch is a knowledge-request lane, not physical capability execution.
	// Consume it after Cognition/Dialogue have established the canonical response
	// intent, and before workflow/capability actions can claim the turn.
	if t.semanticReq.Kind == interaction.KindSearch && e.capabilityResolver != nil {
		capabilityDecision := interaction.ResolveCapabilityInteraction(t.semanticReq)
		capabilityRoute := e.capabilityResolver.Resolve(capabilityDecision, t.semanticReq)
		if capabilityRoute.Kind == capability.IntentSearch {
			if e.consumeSearchRoute(t.semanticReq, capabilityDecision, capabilityRoute) {
				return true
			}
			// Search was recognized structurally but could not render. Do not
			// fall through to the legacy local-file action derived from the same
			// wording.
			return true
		}
	}

	if e.sess.IsActive() {
		workflowDecision, workflowEvidence := e.evaluateWorkflow(t.semanticReq, t.input)
		if e.flowTrace != nil {
			e.flowTrace.Mark("WORKFLOW STATE", fmt.Sprintf("state=%s active_file=%q selection=%t action=%q", workflowEvidence.State, workflowEvidence.ActiveFileName, workflowEvidence.SelectionProvided, workflowEvidence.FileAction))
			e.flowTrace.Mark("WORKFLOW EVIDENCE", fmt.Sprintf("selection=%d valid=%t volume=%d valid=%t target=%q", workflowEvidence.SelectionIndex, workflowEvidence.SelectionProvided, workflowEvidence.VolumePercent, workflowEvidence.VolumeProvided, workflowEvidence.Target))
			e.flowTrace.Mark("WORKFLOW DECISION", fmt.Sprintf("handled=%t outcome=%s reason=%s", workflowDecision.Handled, workflowDecision.Outcome, workflowDecision.Reason))
		}
		if workflowDecision.Handled && e.applyWorkflowDecision(t.semanticReq, t.input, workflowDecision, workflowEvidence) {
			return true
		}
	}
	if e.evaluateMetacognition(t.semanticReq) {
		return true
	}

	// A Cognition repair assessment is a capability decision shared by every
	// route. It must pass the same execution authority as other physical work.
	if assessment := e.lastConversationalCognition.Repair; assessment.Required {
		repair := interaction.ResolvePostExecutionRepair(interaction.PostExecutionRepairRequest{
			WorkID: assessment.WorkID, Target: assessment.Target,
			Replacement: assessment.Replacement, Confidence: assessment.Confidence,
			Reason: assessment.Reason,
		})
		if repair.Executable {
			result := e.authorityExecute(repair, t.semanticReq)
			if result.Handled {
				return true
			}
		}
		e.renderCanonicalSystemResponse(t.semanticReq, "repair_not_executable",
			"Entendi que hay que corregir el resultado, pero no pude verificar una reparacion segura.",
			interaction.MoveClarify, interaction.ResponseIntentClarify, true)
		return true
	}

	observationReport := e.persistUserObservations(t.semanticReq)
	if e.flowTrace != nil {
		e.flowTrace.Mark("USER MODEL", fmt.Sprintf("observations attempted=%d handled=%d applied=%d rejected=%d errors=%d", observationReport.Attempted, observationReport.Handled, observationReport.Applied, observationReport.Rejected, len(observationReport.Errors)))
	}
	if observationReport.Attempted > 0 && !observationReport.Successful() && t.semanticReq.CommunicativeActs.Primary != interaction.ActQuestion {
		msg := "Entendí el dato, pero no logré registrarlo de forma confiable."
		if len(observationReport.Errors) > 0 {
			msg = "Entendí el dato, pero ocurrió un error al registrarlo; no voy a afirmar que quedó guardado."
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("USER MODEL", "grounded acknowledgement blocked: persistence not confirmed")
		}
		if e.renderCanonicalTextResult(t.semanticReq, "user_model_observation_result", msg, 0.99, interaction.MoveAcknowledge) {
			return true
		}
		return true
	}
	if observationReport.Successful() && t.semanticReq.CommunicativeActs.Primary != interaction.ActQuestion {
		msg := "Entendido. Lo he registrado."
		if e.flowTrace != nil {
			e.flowTrace.Mark("RESPONSE GATE", "grounded acknowledgement: persistence confirmed")
		}
		if e.renderCanonicalTextResult(t.semanticReq, "user_model_observation_result", msg, 1, interaction.MoveAcknowledge) {
			return true
		}
		return true
	}

	if e.socialInteraction != nil {
		previousSocial := socialinteraction.StateNeutral
		if e.lastSocialAssessment.State != "" {
			previousSocial = e.lastSocialAssessment.State
		}
		e.lastSocialAssessment = e.socialInteraction.Assess(t.semanticReq.Raw, string(t.semanticReq.Channel), previousSocial)
		e.lastSocialPlan = e.socialInteraction.Plan(e.lastSocialAssessment)
	}
	if e.lastConversationalCognition.Action.Blocked && e.lastConversationalCognition.Interpretation.ActionOriented && e.lastConversationalCognition.Interpretation.HasUnresolvedRefs {
		question := "¿Qué quieres que haga exactamente?"
		if len(t.semanticReq.References) > 0 {
			question = "¿A qué te refieres exactamente?"
		}
		e.AskClarification(t.semanticReq.Objective.Text, question)
		e.renderCanonicalSystemResponse(t.semanticReq, "unresolved_reference", question, interaction.MoveClarify, interaction.ResponseIntentClarify, true)
		return true
	}
	if t.semanticReq.Action.Name == "salir" || t.semanticReq.CommunicativeActs.Has(interaction.ActControl) {
		e.renderCanonicalSystemResponse(t.semanticReq, "goodbye", personality.Goodbye(), interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
		e.Shutdown()
		// H7 (2026-10-17): pedir el cierre; la raíz viva termina el proceso.
		e.RequestExit()
		return true
	}
	// Admisión única de respuestas personales (Eje Compañera, Paso 1): una
	// respuesta entra al modelo solo si responde a una pregunta abierta de
	// Ahmelia y pasa la verificación de señales del contrato semántico. No
	// hay escritura por relación conversacional ni decisión ciega: un
	// turno-pregunta del usuario jamás ingresa datos.
	if e.conversationState != nil {
		now := time.Now()
		if open, ok := e.conversationState.ActiveOpenQuestion(now, e.activeConversationTurnID); ok {
			if answer, accepted := interaction.VerifyAnswerToOpenQuestion(open, t.semanticReq, now, answerSocialDetector); accepted {
				return e.captureVerifiedPersonalAnswer(t, answer, open)
			}
		}
	}
	e.applyUserModelCommunicationProfile()
	if e.renderSpecialistResponseResults(t.semanticReq, t.responseSpecialistResults) {
		return true
	}
	// Self/user model decisions (self-knowledge, mental state, etc.)
	if e.runSelfAndUserModelDecisions(t) {
		return true
	}
	e.applyUserModelCommunicationProfile()
	if conversationalResponseEligible(t.semanticReq, e.lastResponseIntent) && e.lastBehavioralDecision.SpeakToCurrentTurn {
		if e.flowTrace != nil {
			e.flowTrace.Mark("CONTRACT", fmt.Sprintf("canonical_act=%s subject=%s", t.semanticReq.CommunicativeActs.Primary, t.semanticReq.Subject))
			e.flowTrace.Mark("RESPONSE GATE", fmt.Sprintf("intent=%s confidence=%.2f", e.lastResponseIntent.Kind, e.lastResponseIntent.Confidence))
		}
		contract := interaction.NewResponseContract(t.semanticReq, e.lastResponseIntent, e.lastNaturalDialogue.Move)
		contract.Source = "b96.39_response_contract"
		if e.renderResponseContractWithStrategy(t.semanticReq, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()), e.lastBehavioralDecision.Strategy) {
			return true
		}
	}
	return false
}

func NewEngine(sess *memory.Session, longMem *memory.LongMemory, shortMem *ShortMemory, history *selfknowledge.ActionHistory, selfQuery *selfknowledge.SelfQuery) *Engine {
	home, _ := os.UserHomeDir()

	reasoner := reasoning.NewReasoner(history)
	learner := system.NewLearningSystem(longMem)

	// FASE 2 — Knowledge Core: reemplaza el "primero pregunto al
	// razonador, si no responde pregunto a selfknowledge" que antes
	// vivía repetido a mano dentro de Process() (con su propio bloque
	// de confidenceCalculator.AddSource copiado dos veces). Ahora es
	// una sola fuente ordenada por prioridad; sources.go ya definía
	// estos adaptadores, solo faltaba registrarlos.
	knowledgeEngine := knowledge.NewEngine()
	world := environment.NewManager()
	convoContext := comprehension.NewConversationContext()
	memIntel := memoryintelligence.New(convoContext, longMem)
	// Bloque 93.3 — EventStore/LearningStore son el dueño único de
	// EventLog/LearningState: antes de este bloque, la conversación (acá)
	// y el ciclo autónomo (autonomy.LearningMemoryAdapter) tocaban los
	// mismos punteros sin ningún lock compartido. Ver
	// memory_intelligence/eventstore.go y learningstore.go.
	eventStore := memoryintelligence.LoadEventStore()
	// Bloque 8 — se carga UNA sola vez y vive todo el proceso (a diferencia
	// de antes, donde el closure de "que aprendiste" creaba un
	// LearningState nuevo cada vez y lo tiraba: nunca persistía a disco,
	// nunca acumulaba SuccessfulAdaptations). Se refresca contra el
	// EventStore ya cargado para que ActionConfidence arranque al día con
	// el historial de sesiones anteriores, no en cero.
	learningState := memoryintelligence.LoadLearningStore(memoryintelligence.LearningStatePath)
	learningState.UpdateFromReflection(eventStore.Snapshot())
	// Bloque 9 (Fase 2.6) — AgendaManager (planning/agenda.go) existía
	// desde antes pero nunca se instanciaba en el Engine: Goals era un
	// módulo huérfano, igual que Timeline/Relaciones antes de que alguien
	// los cableara. Arranca sin objetivos registrados — eso es normal
	// (GetQuickStatus() lo dice explícito), y queda listo para que
	// futuros bloques de Fase 2.6 (Goal Context) lo empiecen a llenar.
	agenda := planning.NewAgendaManager()
	// Bloque 46 — Executive Project Manager: Planner + Progress como
	// núcleo, tal como se acordó al descubrir que planning/ tenía casi
	// 2000 líneas huérfanas. Scheduler, Reviewer y el DependencyGraph
	// de Goals (distinto del de Fase 3, que es de Tasks dentro de un
	// solo pedido) quedan deliberadamente sin conectar — son bloques
	// futuros, no parte de este recorte.
	planner := planning.NewPlanner()
	executiveGoals := make(map[string]*planning.Goal)
	executivePlans := make(map[string]*planning.Plan)
	executiveProgress := make(map[string]*planning.Progress)
	mentalAgenda := reasoning_deep.NewMentalAgendaV2()
	contextBuilder := cognition.NewBuilder(world, eventStore, longMem, agenda, mentalAgenda, convoContext, learningState)
	contextCache := cognition.NewCache(contextBuilder, cognition.DefaultCacheTTL)
	// Bloque 11 (Fase 2.6) — Cognitive Pipeline: no consulta ninguna
	// fuente nueva, solo registra por dónde va pasando cada input dentro
	// de Process()/execute(). Se instancia acá, mismo criterio que
	// contextCache: vive todo el proceso, no se recrea por request.
	pipeline := cognition.NewPipeline()
	// Bloque 15 (Fase 2.6) — Internal State: mismo criterio de
	// instanciación que Pipeline (vive todo el proceso, un solo lugar).
	// Se declara junto a pipeline a propósito — sus transiciones van a
	// vivir en los mismos puntos de Process()/execute() donde ya está
	// pipeline.Enter().
	internalState := cognition.NewStateManager()
	// Bloque 16 (Fase 2.8) — arranca la fase de Conversación. Vive acá
	// (no adentro de Process()) porque calcula el modo UNA vez por
	// turno, antes de cualquier generación de lenguaje — mismo
	// criterio de instanciación que el resto de Fase 2.6.
	conversationMode := cognition.NewConversationModeManager()
	// Bloque 17 (Fase 2.8) — reconecta personality.Relationship. Carga
	// desde disco (mismo criterio que LoadLearningState): si
	// relationship.json no existe todavía (primera corrida de este
	// bloque), arranca una relación nueva con Type="Cognitive Companion".
	relationship := personality.LoadRelationship(personality.RelationshipPath)
	// Bloque 18 (Fase 2.8) — reconecta ConversationFlow/ContextAdapter.
	// Ninguno de los dos persiste a disco (a diferencia de relationship,
	// arriba): son estado de LA SESIÓN actual — turno, tema, largo
	// preferido — no memoria de largo plazo entre reinicios.
	conversationFlow := language.NewConversationFlow()
	contextAdapter := language.NewContextAdapter()
	// Bloque 19 (Fase 2.8) — no persiste a disco: la POLÍTICA de humor
	// (NeverJokesAbout, HumorTypes) es de sesión/configuración, no
	// memoria de largo plazo — lo que sí se persiste (TrustLevel,
	// SharedHistory) ya vive en relationship, arriba.
	humorStyle := personality.NewHumorStyle()
	// Bloque 12 (Fase 2.6) — Attention Manager: mismo criterio de "vive
	// todo el proceso, se instancia una vez" que el resto de Fase 2.6.
	// No guarda referencias a nada — solo puntúa lo que Context
	// Builder/Cache ya produjeron, así que no necesita más que los
	// defaults del roadmap.
	attention := cognition.NewManager()
	// Bloque 13 (Fase 2.6) — Working Memory: se para ENTRE Attention y
	// MentalAgenda (ver el branch de IntentQuestion más abajo), no al
	// lado. Mismo criterio de instanciación que el resto de Fase 2.6.
	workingMemory := cognition.NewWorkingMemory()
	// Bloque 21 (Fase 3, Capa 3.1) — Goal Analyzer: primer nodo del
	// razonamiento PROSPECTIVO (a diferencia de TODO lo de arriba, que es
	// Fase 2.x y mira hacia atrás — memoria, contexto, estado interno).
	// Paquete nuevo solving/, no reasoning/: reasoning/ ya usa esos
	// nombres (hypothesis.go, decision.go, evaluator.go, critic.go) para
	// analizar acciones YA ejecutadas. Sin estado propio — se instancia
	// una vez por prolijidad, no porque haga falta.
	goalAnalyzer := solving.NewAnalyzer()
	// Bloque 62 (Fase 5, Capa 5.2) — Conversational Memory: a diferencia
	// de conversationFlow (18, vive y muere con el proceso), esto
	// persiste a disco — mismo criterio que learningState/knowledgeMemory.
	// Se carga UNA vez acá y vive todo el proceso.
	conversationalMemory := dialogue.LoadConversationalMemory(dialogue.ConversationalMemoryPath)
	conversationHistory := dialogue.LoadConversationHistory(dialogue.ConversationHistoryPath, conversationalMemory)
	// B96.35: ConversationState puede sobrevivir al proceso por compatibilidad,
	// pero un episodio histórico representa una EJECUCIÓN real. Nunca reutilizamos
	// el SessionID persistido como identidad del episodio; así dos arranques distintos
	// no se fusionan accidentalmente.
	conversationEpisodeID := conversationHistory.BeginSession(fmt.Sprintf("process_%d", time.Now().UnixNano()), time.Now())
	// Bloque 63 (Fase 5, Capa 5.3) — Social Memory: mismo criterio de
	// persistencia que conversationalMemory (62), pero para PERSONAS en
	// vez de temas.
	socialMemory := dialogue.LoadSocialMemory(dialogue.SocialMemoryPath)
	// Bloque 22 (Fase 3, Capa 3.2) — Problem Decomposer: toma los
	// Subgoals (planos) que ya produce goalAnalyzer y los expande en una
	// jerarquía de tareas. Sin estado propio, mismo criterio que
	// goalAnalyzer.
	problemDecomposer := solving.NewDecomposer()
	// Bloque 23 (Fase 3, Capa 3.3) — Knowledge Mapper: a diferencia de
	// goalAnalyzer/problemDecomposer, SÍ tiene una dependencia real —
	// necesita learningState (Bloque 7/8) para saber la confianza
	// histórica de cada Action. Misma referencia que ya vive en el
	// Engine, no una copia.
	knowledgeMapper := solving.NewMapper(learningState)
	// Bloque 24 (Fase 3, Capa 3.4) — Dependency Solver: consume la
	// SALIDA de knowledgeMapper (el KnowledgeMap), no un módulo aparte
	// — primer nodo de Fase 3 que depende de otro nodo de Fase 3. Sin
	// estado propio, mismo criterio que goalAnalyzer/problemDecomposer.
	dependencySolver := solving.NewSolver()
	// Bloque 25 (Fase 3, Capa 3.5) — Hypothesis Generator: combina las
	// salidas de knowledgeMapper (Gaps) y dependencySolver (Blocked) —
	// segundo nodo de Fase 3 que depende de MÁS de un nodo anterior, no
	// solo del que le sigue en la cadena. Sin estado propio, mismo
	// criterio que el resto de Fase 3.
	hypothesisGenerator := solving.NewGenerator()
	// Bloque 38 (Fase 3 Profundizada, Nivel II) — Hypothesis Comparator:
	// mira el 2do/3er candidato que Multi-Hypothesis Engine (Bloque 36)
	// dejó de descartar. No decide (eso es Strategy Selector, bloque
	// siguiente) — solo explica qué tan cerca estuvo lo que no se
	// eligió. Sin estado propio, mismo criterio que el resto de Fase 3.
	comparator := solving.NewComparator()
	// Bloque 39 (Fase 3 Profundizada, Nivel II) — Strategy Selector: la
	// misma referencia de LearningState que ya usa knowledgeMapper
	// (Bloque 23), no una copia — es la segunda señal real (junto con
	// Similarity) para decidir entre candidatos, en vez de siempre
	// tomar el de mayor solapamiento de palabras.
	strategySelector := solving.NewSelector(learningState)
	// Bloque 40 (Fase 3 Profundizada, Nivel II) — Adaptive Confidence:
	// la primera capa que usa Decision.Confidence (Bloque 37) para
	// decidir CÓMO comportarse, no solo qué describir. Cruza Decision
	// (37) con []*Selection (39) — ninguna señal nueva, mismo criterio
	// "integrar, no crear". Sin estado propio.
	adaptiveConfidence := solving.NewAdaptiveConfidence()
	// Bloque 41 (Fase 3 Profundizada, Nivel II) — Reflection Loop:
	// cruza Comparator (38) y Strategy Selector (39), emparejados por
	// TaskID, más la nota de Adaptive Confidence (40) — el primero que
	// se pregunta si una elección fue sólida, con material real en vez
	// de una tautología ("elegí la única opción que había"). Sin
	// estado propio.
	reflector := solving.NewReflector()
	// Bloque 42 (Fase 3 Profundizada, Nivel II) — Counterfactual
	// Evaluation: reusa exactamente los mismos números que ya trae
	// cada CandidateScore (Similarity, TrackRecord, Bloque 39),
	// llevados a sus extremos, para responder "¿qué habría pasado si
	// solo hubiera importado uno de los dos?". Sin estado propio.
	counterfactualEvaluator := solving.NewCounterfactualEvaluator()
	// Bloque 43 (Fase 3 Profundizada, Nivel III — Executive Reasoning)
	// — Causal Reasoning: opera directo sobre DependencyGraph (Bloque
	// 24), que ya tenía la relación causa-efecto (BlockedByID) sin
	// explotar del todo. No rastrea cadenas multi-hop (Order es
	// lineal, no hace falta) — agrega explicar UNA Task puntual y
	// proyectar el efecto de resolver UN gap, ninguno de los dos
	// existía antes. Sin estado propio.
	causalReasoner := solving.NewCausalReasoner()
	// Bloque 44 (Fase 3 Profundizada, Nivel III) — Failure Analysis:
	// misma referencia de LearningState que ya usan knowledgeMapper
	// (23) y strategySelector (39) — consume
	// FailurePatternsByAction (Bloque 7/8), nunca leído fuera de
	// LearningState hasta ahora. Responde POR QUÉ falló antes, no solo
	// CUÁNTO le fue bien (eso ya es ActionConfidence).
	failureAnalyzer := solving.NewFailureAnalyzer(learningState)
	// Bloque 45 (Fase 3 Profundizada, Nivel III) — Learning
	// Integration: misma referencia de LearningState que ya usan
	// knowledgeMapper (23), strategySelector (39) y failureAnalyzer
	// (44) — reusa SuggestStrategyChange() (Bloque 7/8) tal cual, no
	// reimplementa el criterio de umbrales.
	learningIntegrator := solving.NewLearningIntegrator(learningState)
	// Bloque 47 (Fase 4 — Knowledge Acquisition Mind, primer módulo) —
	// Curriculum Builder: cruza HypothesisSet (25) y AnalogySet (28)
	// para priorizar qué gaps necesitan aprendizaje genuino, sin
	// depender de Internet (Ahmelia hoy solo puede abrir una URL, no
	// leer su contenido). Sin estado propio.
	curriculumBuilder := solving.NewCurriculumBuilder()
	// Bloque 48 (Fase 4 — Knowledge Acquisition Mind) — Learning
	// Validator: misma referencia de eventLog y learningState que ya
	// usan knowledgeMapper (23) y el resto de Fase 3/4. No recalcula
	// ActionConfidence — vuelve a contar las muestras reales detrás de
	// cada una para saber si el número es estadísticamente confiable o
	// todavía tentativo.
	learningValidator := solving.NewValidator(eventStore, learningState)
	// Bloque 49 (Fase 4) — Knowledge Memory: persiste el último
	// Validation conocido por Action (knowledge_memory.json, mismo
	// criterio que learning.json/events.json) para no recalcular la
	// fiabilidad desde cero en cada consulta. Se carga UNA vez acá — el
	// resto del proceso lo actualiza in-memory y persiste en cada
	// validación real (ver el trigger de Learning Validator más abajo).
	knowledgeMemory := solving.LoadKnowledgeMemory(solving.KnowledgeMemoryPath)
	// Bloque 50 (Fase 4, último de la Opción A.5) — Learning Planner:
	// combina knowledgeMemory (49, qué ya es confiable) con lo que
	// curriculumBuilder (47) arme al vuelo (qué es territorio genuino) en
	// un solo plan de reforzar vs. aprender. Sin estado propio.
	learningPlanner := solving.NewPlanner()
	// Bloque 52 (Fase 4.1) — Knowledge Discovery: primer módulo de
	// solving/ que depende de gateway/ (I/O real, Bloque 51). Recibe la
	// whitelist REAL del Gateway, no una copia — así nunca puede quedar
	// desincronizado de qué fuentes están autorizadas de verdad.
	knowledgeDiscoverer := solving.NewDiscoverer(gateway.AuthorizedSources)
	// Bloque 53 (Fase 4.1) — Knowledge Validation: primer trigger que
	// de verdad encadena Discover → Fetch → Validate. knowledgeGateway
	// es la primera instancia REAL de gateway.Gateway en todo
	// core/engine.go — hasta acá, Discovery (52) solo decidía fuentes
	// sin tocar la red.
	knowledgeGateway := gateway.NewGateway()
	sourceValidator := solving.NewSourceValidator()
	// Bloque 54 (Fase 4.1) — Skill Builder: cierra la cadena
	// Discover → Fetch → Validate → Build. skillLibrary se carga de
	// disco al arrancar (mismo criterio que knowledgeMemory, 49) para
	// que las notas ya construidas sobrevivan entre sesiones — no
	// hace falta volver a salir a Internet por algo que ya se guardó.
	skillBuilder := solving.NewSkillBuilder()
	skillLibrary := solving.LoadSkillLibrary(solving.SkillLibraryPath)

	// ── Fase 6 (B72-77) — Autonomía Cognitiva e Iniciativa ──
	//
	// Se instancia acá (antes de los closures de knowledgeEngine, más
	// abajo, que ya llaman a activateTopic()) porque autonomousState
	// tiene que estar disponible para esos mismos closures — mismo
	// motivo por el que conversationFlow/conversationalMemory ya
	// vivían arriba de este punto. Ningún bloque de esta familia
	// arranca ninguna goroutine acá: eso es responsabilidad explícita
	// de StartAutonomy(), llamada aparte desde main.go, para que
	// NewEngine() siga siendo puro (construir, no arrancar procesos
	// en background — mismo criterio que ya regía para el resto del
	// Engine).
	autonomousState := autonomy.New()                                             // Bloque 72
	goalMonitor := autonomy.NewGoalMonitor(autonomousState, 0)                    // Bloque 73 — 0 usa DefaultStaleAfter
	contextObserver := autonomy.NewContextObserver(autonomy.ObserverConfig{})     // Bloque 74 — defaults del propio bloque
	initiativeEngine := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{}) // Bloque 75
	conversationManager := autonomy.NewConversationManager()                      // Bloque 76

	// Bloque 90.3 — GoalPlanningAdapter (con su Replanner, B87) pasa a ser
	// una dependencia estable de Engine, construida UNA sola vez acá, en
	// vez de recrearse cada vez que StartAutonomousCycle() corre (ver
	// auditoría de B90: antes de este bloque, buildAutonomousExecutionCycle
	// construía un GoalPlanningAdapter nuevo — con plans/activeTasks
	// vacíos — en cada llamada, y Engine no tenía forma de alcanzar el que
	// ya existía. Eso hacía imposible tanto persistir su estado como
	// evitar perder progreso si el ciclo se reiniciaba). selector=nil y
	// replanner=nil mantienen exactamente el comportamiento por defecto
	// que ya tenía buildAutonomousExecutionCycle antes de este bloque
	// (GoalSelector con DefaultGoalSelectionWeights, Replanner con
	// DefaultReplanLimits) — B90.3 no cambia ninguna política, solo dónde
	// vive la instancia. El error solo puede darse si state/monitor/
	// planner/mapper llegaran nil, lo cual no ocurre en este punto de
	// NewEngine (los tres primeros ya están construidos arriba, el mapper
	// nunca es nil) — un panic acá es preferible a dejar Engine en un
	// estado a medias con un adapter inexistente que el resto de B90 no
	// puede auditar.
	//
	// B89: CapabilityResolver reemplaza a ExplicitGoalActionMapper (ver
	// core/autonomy_b88.go) como mapper real del ciclo autónomo — resuelve
	// el texto de la Task contra el mismo vocabulario de capacidad que ya
	// usa el camino conversacional (solving.Decompose, B62/B69), en vez de
	// exigir la sintaxis literal "action=X target=Y". ExplicitGoalActionMapper
	// se mantiene en autonomy_b88.go (no se borra) porque sus tests de B88
	// la siguen instanciando directo — sigue siendo un GoalActionMapper
	// válido, solo que ya no es el que compone el ciclo real.
	goalPlanningAdapter, err := autonomy.NewGoalPlanningAdapter(
		autonomousState,
		goalMonitor,
		planner,
		autonomy.NewCapabilityResolver(skillLibrary),
		nil, // selector: nil usa GoalSelector con DefaultGoalSelectionWeights (B86)
		nil, // replanner: nil usa Replanner con DefaultReplanLimits (B87)
	)
	if err != nil {
		panic(fmt.Sprintf("B90.3: no se pudo construir GoalPlanningAdapter con dependencias ya válidas — bug de NewEngine: %v", err))
	}

	// Bloque 90.7 — carga al iniciar. Este es el único punto de todo
	// NewEngine donde planner/autonomousState/goalPlanningAdapter siguen
	// exactamente como se acaban de construir (sin Goals/Tasks/Plans
	// propios) — la ventana exacta que ApplyAutonomyPersistenceSnapshot
	// (B90.6) exige. Si memory/autonomy_state.json no existe todavía
	// (primera corrida), LoadAutonomyPersistenceSnapshot devuelve un
	// snapshot vacío sin error y Apply no tiene nada que reconciliar. Un
	// snapshot ilegible o inválido no aborta el arranque: se descarta y
	// Ahmelia sigue con estado autónomo limpio, mismo criterio de "falla
	// honesta sin dejar nada a medias" que ya rige en B90.6.
	if snap, loadErr := LoadAutonomyPersistenceSnapshot(AutonomyStatePath); loadErr != nil {
		fmt.Printf("⚠️ [B90.7] no se pudo leer %s: %v — arranca con estado autónomo vacío\n", AutonomyStatePath, loadErr)
	} else if applyErr := ApplyAutonomyPersistenceSnapshot(snap, planner, autonomousState, goalPlanningAdapter); applyErr != nil {
		fmt.Printf("⚠️ [B90.7] snapshot de autonomía en %s inválido, se descarta: %v\n", AutonomyStatePath, applyErr)
	} else {
		// Bloque 90.8.2 — las vistas ejecutivas de B46 (executiveGoals/
		// executivePlans/executiveProgress) no son parte de lo que B90.6
		// reconcilia; sin este paso quedarían vacías tras cada reinicio
		// aunque el Goal siga vivo para el ciclo autónomo (ver auditoría
		// B90.8). Solo indexa lo que la reconciliación de arriba ya
		// dejó cargado — no crea ningún Goal/Plan nuevo.
		rebuildExecutiveViews(planner, goalPlanningAdapter, executiveGoals, executivePlans, executiveProgress)
	}

	// B96.42.2 — GoalStore becomes the Core-owned live owner of the existing
	// Planner/AutonomousState/GoalPlanningAdapter graph. It does not create a
	// second Goal model or persistence file.
	goalStore, err := NewGoalStore(planner, autonomousState, goalPlanningAdapter)
	if err != nil {
		panic(fmt.Sprintf("B96.42.2: no se pudo construir GoalStore: %v", err))
	}

	// Bloque 55 (Fase 4.1) — Skill Integration: cruza Curriculum (47)
	// contra skillLibrary (54) usando knowledgeDiscoverer (52) para
	// identificar el dominio — primer bloque que hace que Curriculum
	// Builder y Skill Library de verdad se hablen entre sí. Sin
	// estado propio.
	skillIntegrator := solving.NewSkillIntegrator()
	// Bloque 56 (Fase 4.1) — Knowledge Retrieval: gatilla ANTES de la
	// cadena real Discover→Fetch→Validate (54) para evitar salir a
	// Internet cuando ya hay algo guardado. Sin estado propio.
	knowledgeRetriever := solving.NewKnowledgeRetriever()
	// Bloque 57 (Fase 4.1) — Semantic Skill Search: busca por término
	// (literal, no por embeddings) sobre el Content ya guardado en
	// skillLibrary, sin pasar por Discoverer — no hace falta que el
	// término buscado sea un dominio técnico reconocido. Sin estado
	// propio.
	semanticSkillSearcher := solving.NewSemanticSkillSearcher()
	// Bloque 58 (Fase 4.1) — Skill Update Detector: reusa
	// SkillBuilder.Build (54) tal cual para construir lo que una nota
	// armaría HOY con documentos frescos, y lo compara contra lo que
	// ya está guardado. No actualiza nada — solo detecta. Sin estado
	// propio.
	updateDetector := solving.NewUpdateDetector()
	// Bloque 59 (Fase 4.1) — Skill Versioning: decide qué hacer con un
	// cambio ya detectado por UpdateDetector (58) — en vez de pisar la
	// nota en silencio, archiva la versión anterior completa antes de
	// instalar la nueva. skillVersionStore se carga de disco al
	// arrancar, mismo criterio que skillLibrary (54). Sin estado propio.
	versioner := solving.NewVersioner()
	skillVersionStore := solving.LoadSkillVersionStore(solving.SkillVersionStorePath)
	// Bloque 60 (Fase 4.1, último bloque de la Fase 4) — Knowledge
	// Conflict Resolver: compara el Content de las fuentes confiables
	// entre sí (superposición de keywords, Jaccard) para detectar
	// divergencia — nunca decide cuál fuente es correcta, solo marca
	// para revisión manual. Sin estado propio.
	conflictResolver := solving.NewConflictResolver()
	// Bloque 26 (Fase 3, Capa 3.6) — Constraint Solver: cruza los
	// Requirements que goalAnalyzer detecta (Bloque 21) contra Gaps()
	// (Bloque 23) y Blocked() (Bloque 24) — tercer nodo de Fase 3 que
	// depende de más de un nodo anterior. Sin estado propio, mismo
	// criterio que el resto de Fase 3.
	constraintSolver := solving.NewConstraintSolver()
	// Bloque 27 (Fase 3, Capa 3.7) — Decision Engine: el nodo con MÁS
	// dependencias de Fase 3 hasta ahora — junta el HypothesisSet
	// completo (Bloque 25) y el ConstraintReport completo (Bloque 26),
	// no un dato suelto de cada uno. Sin estado propio, mismo criterio
	// que el resto de Fase 3.
	decisionEngine := solving.NewDecisionEngine()
	// Bloque 28 (Fase 3, Capa 3.8) — Analogical Reasoning: a diferencia
	// de hypothesisGenerator (compara contra el catálogo ESTÁTICO de
	// shared.ActionPatterns), este busca en eventLog (Fase 1-2,
	// memory_intelligence/eventlog.go) — lo que Ahmelia REALMENTE
	// ejecutó. Sin estado propio, mismo criterio que el resto de Fase 3.
	analogicalReasoner := solving.NewAnalogicalReasoner()
	// Bloque 29 (Fase 3, Capa 3.9) — Self Evaluation: la última capa,
	// cierra Fase 3. No depende de ningún nodo en particular como
	// entrada fija — Evaluate() recibe las salidas de los ocho
	// anteriores, ya calculadas por quien la llama (ver el branch de
	// abajo). Sin estado propio, mismo criterio que el resto de Fase 3.
	selfEvaluator := solving.NewSelfEvaluator()
	// Bloque 32 (Fase 3.0, cierre) — Reasoning Cache: comparte eventLog
	// con contextCache (Bloque 10) para la misma señal barata de
	// invalidación (creció el EventLog → lo cacheado puede estar
	// desactualizado, porque ActionConfidence/ActionPatterns-derived
	// pudo cambiar). Vive todo el proceso, no se recrea por request —
	// mismo criterio que contextCache.
	reasoningCache := cognition.NewReasoningCache(eventStore, cognition.ReasoningCacheTTL)
	responseComposer := language.NewResponseComposer(nil, contextAdapter, humorStyle, workingMemory, conversationFlow, relationship)
	knowledgeEngine.Register(knowledge.NewMemorySource(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		input := req.Raw
		// MemorySource ya existía en knowledge/sources.go con su
		// Priority() y todo, pero nunca se registraba acá — quedó
		// armado y sin conectar. memory_intelligence/ es lo que le da
		// contenido real: cruza el tema activo de la sesión
		// (comprehension) con lo persistido en memory/long.json.
		norm := shared.Normalize(input)

		// Timeline (Bloque 2, Nivel 1) va PRIMERO por ser más específico:
		// "qué hicimos hoy" también contiene "que hicimos", así que si el
		// genérico se revisara primero se comería la referencia temporal
		// y respondería con el episodio más reciente sin importar si de
		// verdad fue hoy o hace tres días.
		if from, to, label, ok := memoryintelligence.ParseTimelineQuery(req); ok {
			episodes := memoryintelligence.EpisodesInRange(longMem, from, to)
			return memoryintelligence.DescribeTimeline(episodes, label), 0.85, true
		}

		// Relaciones (Bloque 2, Nivel 2) también va ANTES del genérico:
		// el trigger ("que esta relacionado con...") es específico y no
		// debería competir con "que hicimos".
		if subject, ok := memoryintelligence.ParseRelationsQuery(req); ok {
			desc, found := memoryintelligence.DescribeRelations(longMem, subject)
			if !found {
				// Entendimos la pregunta, pero no hay conexión guardada
				// todavía — distinto de no entender la pregunta.
				return "No tengo ningún comando conectado con eso todavía", 0.6, true
			}
			return desc, 0.8, true
		}

		// Patrones (Fase 3 de Pattern Learning) también va ANTES del
		// genérico, mismo motivo que Timeline/Relaciones: trigger
		// específico, no debería competir con "que hicimos".
		if memoryintelligence.ParsePatternsQuery(req) {
			chains := eventStore.BuildChains(memoryintelligence.MaxChainLength)
			patterns := memoryintelligence.DetectPatterns(chains, memoryintelligence.DefaultPatternThreshold)
			return memoryintelligence.DescribePatterns(patterns), 0.8, true
		}

		// Hábitos (Fase 4) también va ANTES del genérico, mismo motivo.
		if memoryintelligence.ParseHabitsQuery(req) {
			habits := eventStore.DetectHabits(memoryintelligence.DefaultPatternThreshold, memoryintelligence.DefaultMinHabitDays)
			return memoryintelligence.DescribeHabits(habits), 0.8, true
		}

		// Experiencia (Bloque 3) también va ANTES del genérico, mismo
		// motivo: "por qué falla blender" no debería competir con
		// "que hicimos".
		if subject, ok := memoryintelligence.ParseExperienceQuery(req); ok {
			stats, found := eventStore.ExperienceForSubject(subject)
			return memoryintelligence.DescribeExperience(stats, found), 0.8, true
		}

		// Insights (Bloque 4) también va ANTES del genérico, mismo
		// motivo.
		if memoryintelligence.ParseInsightsQuery(req) {
			insights := eventStore.GenerateInsights()
			return memoryintelligence.DescribeInsights(insights), 0.8, true
		}

		// Reflection (Bloque 6) — análisis de cómo está funcionando
		// Ahmelia misma (tasa de éxito, fortalezas/debilidades),
		// no del usuario.
		if memoryintelligence.ParseReflectionQuery(req) {
			reflections := eventStore.GenerateReflections()
			return memoryintelligence.DescribeReflections(reflections), 0.8, true
		}

		// Learning (Bloque 7) — análisis de qué estrategias aprendió
		// Ahmelia y cómo adaptó su enfoque. Distinto de Reflection:
		// Reflection mira "cómo está funcionando", Learning mira
		// "qué cambió basado en lo que funcionó".
		if memoryintelligence.ParseLearningQuery(req) {
			learningState.UpdateFromReflection(eventStore.Snapshot())
			return learningState.DescribeLearning(), 0.8, true
		}

		// Goal Analyzer (Bloque 21, Fase 3 Capa 3.1) — primera capa
		// PROSPECTIVA: a diferencia de todo lo de arriba (que mira hacia
		// atrás — memoria, reflexión, aprendizaje), esto analiza una
		// petición que el usuario todavía no pidió ejecutar. Usa
		// goalAnalyzer (variable local, no e.goalAnalyzer — dentro de
		// este closure "e" todavía no existe, mismo motivo por el que
		// todo lo de arriba usa eventLog/learningState directo).
		if solving.ParseGoalAnalysisQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			return analysis.Describe(), 0.8, true
		}

		// Problem Decomposer (Bloque 22, Fase 3 Capa 3.2) — pregunta
		// distinta de Goal Analyzer: no pide EL objetivo, pide en qué
		// TAREAS se divide. Encadena los dos: primero analiza (Bloque
		// 21), después descompone los Subgoals que ese análisis dejó.
		if solving.ParseDecomposeQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			return plan.Describe(), 0.8, true
		}

		// Knowledge Mapper (Bloque 23, Fase 3 Capa 3.3) — pregunta
		// distinta de las dos anteriores: no pide el objetivo ni cómo se
		// divide en tareas, pide cuánto de eso YA es capacidad real.
		// Encadena las tres capas: Analyze (21) → Decompose (22) → Map (23).
		if solving.ParseKnowledgeMapQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			return km.Describe(), 0.8, true
		}

		// Dependency Solver (Bloque 24, Fase 3 Capa 3.4) — pregunta
		// distinta de las tres anteriores: no pide el objetivo, cómo se
		// divide, ni cuánto ya se sabe hacer, pide el ORDEN y qué queda
		// bloqueado por territorio nuevo. Encadena las cuatro capas:
		// Analyze (21) → Decompose (22) → Map (23) → Solve (24).
		if solving.ParseDependencySolveQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			return graph.Describe(), 0.8, true
		}

		// Hypothesis Generator (Bloque 25, Fase 3 Capa 3.5) — pregunta
		// distinta de las cuatro anteriores: ninguna de ellas proponía
		// nada, solo describían (qué es, cómo se divide, cuánto ya se
		// sabe, en qué orden). Esta es la primera que sugiere por dónde
		// probar. Encadena las cinco capas: Analyze (21) → Decompose
		// (22) → Map (23) → Solve (24) → Generate (25).
		if solving.ParseHypothesisQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			return hs.Describe(), 0.8, true
		}

		// Hypothesis Comparator (Bloque 38, Nivel II) — pregunta
		// distinta de Hypothesis Generator: esa pide CUÁL es la mejor
		// pista, esta pide qué tan cerca estuvieron las que no se
		// eligieron. Encadena las mismas cinco capas que Hypothesis
		// Generator, agregando el paso de comparación al final.
		if solving.ParseComparisonQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			comparisons := comparator.CompareAll(hs)
			return solving.DescribeAll(comparisons), 0.8, true
		}

		// Strategy Selector (Bloque 39, Nivel II) — a diferencia de
		// Comparator (que solo describe), esta SÍ puede elegir un
		// candidato distinto del de mayor Similarity, si el historial
		// real (LearningState.ActionConfidence) lo respalda.
		if solving.ParseStrategyQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			selections := strategySelector.SelectAll(hs)
			return solving.DescribeSelections(selections), 0.8, true
		}

		// Constraint Solver (Bloque 26, Fase 3 Capa 3.6) — pregunta
		// distinta de las cinco anteriores: vuelve a mirar los
		// Requirements que Goal Analyzer (Bloque 21) ya había detectado
		// y los cruza contra Gaps()/Blocked() para decir si el plan
		// puede cumplirlos. Encadena las seis capas: Analyze (21) →
		// Decompose (22) → Map (23) → Solve (24) → [Generate (25) no
		// hace falta acá, Check solo necesita ga/km/dg] → Check (26).
		if solving.ParseConstraintQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			report := constraintSolver.Check(analysis, km, graph)
			return report.Describe(), 0.8, true
		}

		// Decision Engine (Bloque 27, Fase 3 Capa 3.7) — pregunta
		// distinta de las seis anteriores: ninguna daba un veredicto
		// único de avanzar/ajustar/pedir más información, todas
		// describían un aspecto parcial. Esta junta HypothesisSet
		// (Bloque 25) y ConstraintReport (Bloque 26) y decide. Encadena
		// las siete capas completas: Analyze (21) → Decompose (22) →
		// Map (23) → Solve (24) → Generate (25) → Check (26) → Decide
		// (27).
		if solving.ParseDecisionQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			report := constraintSolver.Check(analysis, km, graph)
			decision := decisionEngine.Decide(analysis.MainGoal, hs, report)
			return decision.Describe(), 0.8, true
		}

		// Adaptive Confidence (Bloque 40, Nivel II) — pregunta distinta
		// de Decision Engine: esa pregunta si conviene avanzar con el
		// plan; esta pregunta qué tan sólida es esa recomendación y, si
		// no es tan sólida, qué otra cosa valdría la pena tener en
		// cuenta. Encadena las mismas siete capas de Decision Engine,
		// agregando Strategy Selector (39) para las alternativas y
		// Adaptive Confidence (40) al final.
		if solving.ParseAdaptiveConfidenceQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			report := constraintSolver.Check(analysis, km, graph)
			decision := decisionEngine.Decide(analysis.MainGoal, hs, report)
			selections := strategySelector.SelectAll(hs)
			action := adaptiveConfidence.Decide(decision, selections)
			return action.Describe(), 0.8, true
		}

		// Reflection Loop (Bloque 41, Nivel II) — la primera pregunta
		// que mira hacia atrás sobre las elecciones ya hechas, no hacia
		// adelante sobre el plan. Encadena las mismas capas que
		// Adaptive Confidence, agregando Comparator (38) — que
		// Adaptive Confidence no necesitaba — para tener HasCloseCall()
		// disponible.
		if solving.ParseReflectionQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			report := constraintSolver.Check(analysis, km, graph)
			decision := decisionEngine.Decide(analysis.MainGoal, hs, report)
			comparisons := comparator.CompareAll(hs)
			selections := strategySelector.SelectAll(hs)
			action := adaptiveConfidence.Decide(decision, selections)
			reflections := reflector.Reflect(comparisons, selections, action)
			return reflections.Describe(), 0.8, true
		}

		// Counterfactual Evaluation (Bloque 42, Nivel II) — pregunta
		// distinta de Reflection Loop: esa se pregunta si la elección
		// fue sólida; esta compara escenarios explícitos ("si solo
		// hubiera importado X"). Reusa el filtro de Reflect() (41) en
		// vez de recorrer todos los gaps de nuevo.
		if solving.ParseCounterfactualQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			report := constraintSolver.Check(analysis, km, graph)
			decision := decisionEngine.Decide(analysis.MainGoal, hs, report)
			comparisons := comparator.CompareAll(hs)
			selections := strategySelector.SelectAll(hs)
			action := adaptiveConfidence.Decide(decision, selections)
			reflections := reflector.Reflect(comparisons, selections, action)
			counterfactuals := counterfactualEvaluator.EvaluateReflections(reflections.Reflections, selections)
			return solving.DescribeCounterfactuals(counterfactuals), 0.8, true
		}

		// Causal Reasoning (Bloque 43, Nivel III) — "por qué está
		// bloqueada esto": explica la causa de cada Task bloqueada del
		// plan. Solo necesita Decompose→Map→Solve, no las capas de
		// decisión/hipótesis — es sobre la ESTRUCTURA del plan, no
		// sobre qué Action ejecutar.
		if solving.ParseCausalExplainQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			links := causalReasoner.ExplainAllBlocks(graph)
			return solving.DescribeLinks(links), 0.8, true
		}

		// Causal Reasoning (Bloque 43) — "qué se desbloquearía si
		// resuelvo X": el reverso de la pregunta anterior, proyecta
		// hacia adelante en vez de explicar hacia atrás.
		if solving.ParseCausalProjectQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			projections := causalReasoner.ProjectAllGaps(graph)
			return solving.DescribeProjections(projections), 0.8, true
		}

		// Failure Analysis (Bloque 44, Nivel III) — "qué riesgo de
		// fallo tiene esto": distinto de Causal Reasoning (bloqueos
		// DENTRO del plan) y de Strategy Selector (cuál Action
		// conviene) — este mira el historial de fallos de la Action
		// que Strategy Selector efectivamente eligió para cada gap.
		if solving.ParseFailureQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			selections := strategySelector.SelectAll(hs)
			risks := failureAnalyzer.AnalyzeAllSelections(selections)
			return solving.DescribeRisks(risks), 0.8, true
		}

		// Learning Integration (Bloque 45, Nivel III) — "con qué
		// estrategia lo intentarías": distinta de Strategy Selector
		// (Bloque 39, que elige CUÁL Action) — esta pregunta CÓMO
		// intentar la Action ya elegida, según lo que Ahmelia aprendió
		// ejecutándola en el pasado (SuggestStrategyChange, Bloque 7/8).
		if solving.ParseLearningIntegrationQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			selections := strategySelector.SelectAll(hs)
			recs := learningIntegrator.RecommendAll(selections)
			return solving.DescribeRecommendations(recs), 0.8, true
		}

		// Learning Validator (Bloque 48, Fase 4) — pregunta distinta de
		// Learning Integration (45): esa pregunta CON QUÉ estrategia
		// intentarlo, esta pregunta si el número de confianza detrás de
		// esa recomendación tiene suficiente respaldo real (EventLog)
		// como para tomarlo en serio, o si todavía es una muestra chica.
		if solving.ParseValidatorQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			selections := strategySelector.SelectAll(hs)
			recs := learningIntegrator.RecommendAll(selections)
			validations := learningValidator.ValidateAll(recs)

			// Bloque 49 — cada validación real que se calcula acá queda
			// registrada en Knowledge Memory, con fecha de primera vez
			// que cruzó el umbral de confiabilidad. Se persiste de
			// inmediato, mismo criterio que LearningState.Save (8).
			knowledgeMemory.RememberAll(validations)
			_ = knowledgeMemory.Save(solving.KnowledgeMemoryPath)

			return solving.DescribeValidations(validations), 0.8, true
		}

		// Knowledge Memory (Bloque 49, Fase 4) — pregunta distinta de
		// Learning Validator (48): esa SIEMPRE recalcula sobre el plan
		// actual (Analyze → ... → ValidateAll, 7 capas); esta responde
		// directo desde lo que YA quedó registrado como confiable en
		// consultas anteriores, sin volver a analizar nada — el punto
		// entero de este bloque es evitar ese trabajo repetido.
		if solving.ParseKnowledgeMemoryQuery(req) {
			return knowledgeMemory.Describe(), 0.8, true
		}

		// Learning Planner (Bloque 50, último de la Opción A.5) —
		// pregunta distinta de Curriculum (47, solo gaps genuinos) y de
		// Knowledge Memory (49, solo lo ya consolidado): combina los
		// dos en un plan único de reforzar (StatusTentative, ya
		// conocido) vs. aprender de cero (gap genuino).
		if solving.ParsePlannerQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			as := analogicalReasoner.FindAnalogies(km, eventStore.SnapshotLog())
			curriculum := curriculumBuilder.Build(hs, as)
			learningPlan := learningPlanner.Plan(knowledgeMemory, curriculum)
			return learningPlan.Describe(), 0.8, true
		}

		// Knowledge Discovery (Bloque 52, Fase 4.1) — primer trigger que
		// toca gateway/ (I/O real, Bloque 51), aunque todavía no
		// descarga nada: solo decide QUÉ fuentes autorizadas son
		// relevantes para el objetivo. No encadena con Analyze() (21) —
		// el objetivo acá es un dominio técnico ("Unreal Engine"), no
		// una tarea a descomponer; usa goalAnalyzer solo para limpiar
		// muletillas y el trigger mismo del texto (MainGoal).
		if solving.ParseDiscoveryQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			return discovery.Describe(), 0.8, true
		}

		// Knowledge Validation (Bloque 53, Fase 4.1) — pregunta
		// distinta de Discovery: esa decide DE DÓNDE traer información;
		// esta de verdad la trae (gateway.Gateway.FetchAll, I/O real) y
		// evalúa si lo que llegó es confiable. Fetchea la raíz de cada
		// fuente autorizada — no navega a una página específica del
		// tema (eso necesitaría un buscador dentro del sitio, que no
		// existe todavía).
		if solving.ParseSourceValidationQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			if !discovery.HasSources() {
				return discovery.Describe(), 0.7, true
			}
			// Integration Fix — antes esto armaba siempre
			// "https://"+Host+"/" (la raíz del sitio) sin importar el
			// objetivo. discovery.ResourceURLs ya trae, cuando
			// Discover() encontró un Resource curado que matchea el
			// objetivo, la página específica en vez de la portada —
			// mismo fallback a la raíz si no hay ningún match, así que
			// esto nunca se comporta peor que antes.
			docs, _ := knowledgeGateway.FetchAll(discovery.ResourceURLs)
			validation := sourceValidator.ValidateAll(discovery.Goal, docs)
			return validation.Describe(), 0.8, true
		}

		// Skill Builder — consulta (Bloque 54, Fase 4.1) — a
		// diferencia de las dos ramas de arriba, esto NO toca la red:
		// lee skill_library.json (ya cargado en skillLibrary al
		// arrancar) y devuelve lo que ya se guardó en sesiones
		// anteriores. Mismo espíritu que Recall() en Knowledge
		// Memory (49): responder sin recalcular ni volver a salir a
		// buscar.
		if solving.ParseSkillBuilderQuery(req) {
			return skillLibrary.Describe(), 0.8, true
		}

		// Skill Builder — construir (Bloque 54, Fase 4.1) — cierra
		// la cadena completa: Discover (52) → Fetch (Gateway, 51) →
		// Validate (53) → Build → Install. Solo persiste si
		// Validate encontró al menos un documento confiable —
		// Build() devuelve nil si no, y no hay nada que instalar. El
		// TaskID de la nota es discovery.Domain (corregido en
		// Bloque 55 — antes era MainGoal en texto libre, lo que
		// rompía cualquier búsqueda posterior por clave). Sin
		// Curriculum de por medio todavía (ese cruce, guardar una
		// nota para un gap PUNTUAL del plan, queda para cuando haga
		// falta encadenar con Bloque 47 de verdad). Desde Bloque 56,
		// esta rama primero consulta Knowledge Retrieval — si ya hay
		// algo guardado para el dominio, ni siquiera llega a esta
		// cadena.
		if solving.ParseSkillBuildQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			// Bloque 56 — antes de tocar la red, consulta si ya hay
			// algo guardado para el dominio detectado. Si lo hay,
			// responde directo y NO ejecuta Discover→Fetch→Validate
			// de acá para abajo — esto es lo que hace que "guarda
			// una nota sobre X" la segunda vez sea gratis.
			retrieval := knowledgeRetriever.Retrieve(analysis.MainGoal, knowledgeDiscoverer, skillLibrary)
			if !retrieval.NeedsFetch() {
				return retrieval.Describe(), 0.85, true
			}
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			if !discovery.HasSources() {
				return discovery.Describe(), 0.7, true
			}
			// Integration Fix — antes esto armaba siempre
			// "https://"+Host+"/" (la raíz del sitio) sin importar el
			// objetivo. discovery.ResourceURLs ya trae, cuando
			// Discover() encontró un Resource curado que matchea el
			// objetivo, la página específica en vez de la portada —
			// mismo fallback a la raíz si no hay ningún match, así que
			// esto nunca se comporta peor que antes.
			docs, _ := knowledgeGateway.FetchAll(discovery.ResourceURLs)
			validation := sourceValidator.ValidateAll(discovery.Goal, docs)
			// Bloque 55 — corrección: el SkillID tiene que ser el
			// Domain detectado (clave estable, ej. "Unreal Engine"),
			// no analysis.MainGoal (texto libre, distinto cada vez
			// que alguien pregunta con otras palabras). Con
			// MainGoal como clave, Skill Integration nunca hubiera
			// podido encontrar esta nota después — HasSources() ya
			// garantiza acá que discovery.Domain no está vacío.
			item := solving.LearningItem{
				TaskID:   discovery.Domain,
				TaskDesc: discovery.Goal,
			}
			note := skillBuilder.Build(item, validation)
			if note == nil {
				return "No encontré ninguna fuente confiable todavía sobre \"" + discovery.Goal + "\" — no guardo una nota sin contenido real detrás.", 0.6, true
			}
			skillLibrary.Install(note)
			if err := skillLibrary.Save(solving.SkillLibraryPath); err != nil {
				return "Armé la nota pero no pude guardarla en disco: " + err.Error(), 0.5, true
			}
			// Integration Fix 2 — acá es donde el conocimiento se
			// termina de instalar de verdad; discovery.Domain (ej.
			// "Blender") es el tema que arranca ahora, no "general".
			// Este closure no tiene un *Engine (se registra antes de
			// que exista uno, ver NewEngine), por eso la versión libre
			// de activateTopic() en vez del método.
			activateTopic(conversationFlow, conversationalMemory, autonomousState, conversationManager, discovery.Domain)
			return "Guardé una nota de estudio: " + note.Describe(), 0.85, true
		}

		// Skill Integration (Bloque 55, Fase 4.1) — la misma cadena
		// de Curriculum Builder (47), pero cruzada contra
		// skillLibrary (54) antes de responder: si ya hay una nota
		// guardada sobre el dominio técnico completo del objetivo,
		// lo informa junto al curriculum — sin quitar ni un item de
		// la lista de gaps, esa granularidad por subtema no existe
		// todavía (ver el límite documentado en skill_integration.go).
		if solving.ParseSkillContextQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			as := analogicalReasoner.FindAnalogies(km, eventStore.SnapshotLog())
			curriculum := curriculumBuilder.Build(hs, as)
			context := skillIntegrator.Integrate(curriculum, knowledgeDiscoverer, skillLibrary)
			return context.Describe(), 0.8, true
		}

		// Semantic Skill Search (Bloque 57, Fase 4.1) — a diferencia
		// de todas las ramas de arriba, NO pasa por Discoverer: el
		// término buscado no tiene por qué ser un dominio técnico
		// reconocido ("renderizado" no es un TechDomain, pero puede
		// aparecer dentro del Content de la nota de Unity). Busca
		// literal sobre lo ya guardado — ver el límite honesto sobre
		// idioma documentado en semantic_skill_search.go.
		if solving.ParseSemanticSkillSearchQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			search := semanticSkillSearcher.Search(analysis.MainGoal, skillLibrary)
			// Integration Fix — mismo criterio que el fallback agregado
			// en ParseSocialMemoryQuery (core/engine.go, rama de
			// Social Memory): con un único match inequívoco, la
			// narrativa rica de esa Skill (Skill.Describe(), incluye
			// State/Steps/Operations) es estrictamente más informativa
			// que "Encontré 1 nota(s) que mencionan X" — antes esta
			// rama SIEMPRE devolvía la genérica, incluso con un solo
			// resultado sin ambigüedad. Con más de un match, sigue
			// siendo la genérica: no hay forma honesta de elegir "la"
			// respuesta entre varias sin arbitrariedad.
			if len(search.Matches) == 1 {
				return search.Matches[0].Note.Describe(), 0.75, true
			}
			return search.Describe(), 0.75, true
		}

		// Skill Procedure Explanation (Bloque 61 — primer bloque
		// después del cierre de Fase 4.1 en el 60) — distinta de
		// Semantic Skill Search (57) arriba: 57 responde "qué sabés"
		// (resumen/metadata), esto responde "cómo se hace" (el
		// procedimiento real). Auditoría que motivó este bloque:
		// Skill.Steps nunca se leía en ningún lugar fuera de
		// skill_builder.go y los bloques que solo lo persisten
		// (58/59) — "cómo instalo Go" no lo manejaba ningún Source,
		// cayendo al fallback genérico. Reusa SemanticSkillSearcher
		// (57) para encontrar la Skill — no reimplementa matching.
		// Deliberadamente NO toca Operations para ejecutar nada —
		// "decir lo que sabe", no "hacer lo que sabe"; eso queda para
		// un futuro Bloque 62 (Skill Execution).
		if solving.ParseSkillProcedureQuery(req) {
			return solving.ExplainProcedure(solving.ExtractProcedureTarget(norm), skillLibrary), 0.8, true
		}

		// Goal & Skill Resolution (Bloque 62, primer bloque de la Fase
		// 6 — Conocimiento → Capacidad) — a diferencia de TODO lo de
		// arriba, no depende de una frase-trigger fija. Se dispara con
		// cualquier pedido que TENGA FORMA de acción (arranca con un
		// verbo imperativo reconocido, ver startsWithActionVerb de 54)
		// y que ninguna rama más específica de arriba ya haya
		// reclamado — por eso va al final de la cadena de Fase 4.1/61,
		// nunca antes. Responde "¿qué necesito y qué tan preparada
		// estoy?" (acción local ya disponible / Skill con capacidad
		// completa, parcial, sin respaldo, o solo conocimiento / nada
		// guardado) — DELIBERADAMENTE nunca ejecuta nada. Ejecutar es
		// el futuro Bloque 63 (Skill Execution), que todavía no existe.
		if solving.ParseGoalCapabilityQuery(norm) {
			return solving.ResolveGoal(input, skillLibrary).Describe(), 0.75, true
		}

		// Skill Update Detector (Bloque 58, Fase 4.1) — la primera
		// pregunta que vuelve a tocar la red DESPUÉS de que ya existe
		// una nota guardada, específicamente para comparar, no para
		// evitar la comparación (eso es lo que hace Knowledge
		// Retrieval, 56, en el resto de las ramas). Sin nota guardada,
		// no fetchea nada — no tiene sentido "comparar" contra algo
		// que no existe.
		if solving.ParseUpdateCheckQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			if !discovery.HasSources() {
				return discovery.Describe(), 0.7, true
			}
			existing, hasExisting := skillLibrary.Get(discovery.Domain)
			if !hasExisting {
				return fmt.Sprintf("No tengo ninguna nota guardada sobre \"%s\" todavía — no hay nada que comparar. Pedime que la guarde primero.", discovery.Domain), 0.7, true
			}
			// Integration Fix — antes esto armaba siempre
			// "https://"+Host+"/" (la raíz del sitio) sin importar el
			// objetivo. discovery.ResourceURLs ya trae, cuando
			// Discover() encontró un Resource curado que matchea el
			// objetivo, la página específica en vez de la portada —
			// mismo fallback a la raíz si no hay ningún match, así que
			// esto nunca se comporta peor que antes.
			docs, _ := knowledgeGateway.FetchAll(discovery.ResourceURLs)
			validation := sourceValidator.ValidateAll(discovery.Goal, docs)
			item := solving.LearningItem{TaskID: discovery.Domain, TaskDesc: discovery.Goal}
			check := updateDetector.Check(item, existing, validation)
			return check.Describe(), 0.8, true
		}

		// Skill Versioning — actualizar (Bloque 59, Fase 4.1) —
		// distinta de Skill Update Detector (58): esa solo compara y
		// narra; esta, si Versioner confirma que cambió de verdad,
		// archiva la nota vieja completa en skillVersionStore ANTES
		// de instalar la nueva en skillLibrary. Sin nota existente, no
		// hay nada que versionar — mismo criterio defensivo que el
		// resto de Fase 4.1.
		if solving.ParseSkillVersionUpdateQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			if !discovery.HasSources() {
				return discovery.Describe(), 0.7, true
			}
			existing, hasExisting := skillLibrary.Get(discovery.Domain)
			if !hasExisting {
				return fmt.Sprintf("No tengo ninguna nota guardada sobre \"%s\" todavía — no hay nada que versionar. Pedime que la guarde primero.", discovery.Domain), 0.7, true
			}
			// Integration Fix — antes esto armaba siempre
			// "https://"+Host+"/" (la raíz del sitio) sin importar el
			// objetivo. discovery.ResourceURLs ya trae, cuando
			// Discover() encontró un Resource curado que matchea el
			// objetivo, la página específica en vez de la portada —
			// mismo fallback a la raíz si no hay ningún match, así que
			// esto nunca se comporta peor que antes.
			docs, _ := knowledgeGateway.FetchAll(discovery.ResourceURLs)
			validation := sourceValidator.ValidateAll(discovery.Goal, docs)
			item := solving.LearningItem{TaskID: discovery.Domain, TaskDesc: discovery.Goal}
			check, oldVersion := versioner.Update(item, existing, validation)
			if !check.NeedsUpdate() {
				return check.Describe(), 0.8, true
			}
			skillVersionStore.ArchiveVersion(discovery.Domain, oldVersion)
			if oldVersion != nil {
				if err := skillVersionStore.Save(solving.SkillVersionStorePath); err != nil {
					return "Detecté el cambio pero no pude archivar la versión anterior: " + err.Error(), 0.5, true
				}
			}
			skillLibrary.Install(check.Candidate)
			if err := skillLibrary.Save(solving.SkillLibraryPath); err != nil {
				return "Archivé la versión anterior pero no pude guardar la nueva: " + err.Error(), 0.5, true
			}
			// Integration Fix 2 — mismo motivo que en Skill Build:
			// renovar la nota también reactiva el dominio como tema.
			activateTopic(conversationFlow, conversationalMemory, autonomousState, conversationManager, discovery.Domain)
			// Integration Fix — Bloque 59: oldVersion puede ser nil aunque
			// NeedsUpdate() sea true (cambio detectado pero puramente
			// cosmético — ver StructuralChange, 58) — decir "Archivé la
			// versión anterior" en ese caso sería mentir sobre lo que
			// realmente pasó. El contenido se actualiza igual.
			if oldVersion == nil {
				return check.Describe() + " Actualicé el contenido guardado — el cambio fue de redacción, no de sustancia, así que no archivé una versión aparte.", 0.85, true
			}
			return check.Describe() + " Archivé la versión anterior y guardé la nueva.", 0.85, true
		}

		// Skill Versioning — historial (Bloque 59, Fase 4.1) — a
		// diferencia de la rama de arriba, esto NO toca la red: lee
		// skill_versions.json (ya cargado en skillVersionStore al
		// arrancar), mismo espíritu que consultar skillLibrary.Describe()
		// (54) sin volver a salir a buscar.
		if solving.ParseSkillVersionHistoryQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			if discovery.Domain == "" {
				return fmt.Sprintf("No identifiqué un dominio conocido para \"%s\".", discovery.Goal), 0.6, true
			}
			history, ok := skillVersionStore.History(discovery.Domain)
			if !ok {
				return fmt.Sprintf("%s: sin versiones archivadas todavía — nunca se detectó un cambio real, o nunca se lo versionó.", discovery.Domain), 0.7, true
			}
			return history.Describe(), 0.8, true
		}

		// Knowledge Conflict Resolver (Bloque 60, Fase 4.1 — cierra la
		// Fase 4) — pregunta distinta de Source Validation (53, evalúa
		// CONFIABILIDAD individual) y de Update Detector (58, compara
		// una fuente contra SU PROPIA nota guardada en el tiempo):
		// esto compara el contenido de las fuentes confiables ENTRE
		// SÍ, ahora mismo, y marca para revisión si divergen — nunca
		// decide cuál es la correcta.
		if solving.ParseConflictQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			discovery := knowledgeDiscoverer.Discover(analysis.MainGoal)
			if !discovery.HasSources() {
				return discovery.Describe(), 0.7, true
			}
			// Integration Fix — antes esto armaba siempre
			// "https://"+Host+"/" (la raíz del sitio) sin importar el
			// objetivo. discovery.ResourceURLs ya trae, cuando
			// Discover() encontró un Resource curado que matchea el
			// objetivo, la página específica en vez de la portada —
			// mismo fallback a la raíz si no hay ningún match, así que
			// esto nunca se comporta peor que antes.
			docs, _ := knowledgeGateway.FetchAll(discovery.ResourceURLs)
			validation := sourceValidator.ValidateAll(discovery.Goal, docs)
			report := conflictResolver.Resolve(validation)
			return report.Describe(), 0.8, true
		}

		// Curriculum Builder (Bloque 47, Fase 4 — Knowledge Acquisition
		// Mind) — pregunta distinta de todo lo anterior: no busca cómo
		// resolver el plan actual, pregunta qué haría falta APRENDER de
		// cero para dejar de tener este tipo de gaps. Encadena hasta
		// Analogical Reasoning (28) — necesita tanto HasLead() (25)
		// como HasAnalogy() (28) para saber qué es territorio
		// GENUINAMENTE nuevo.
		if solving.ParseCurriculumQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			as := analogicalReasoner.FindAnalogies(km, eventStore.SnapshotLog())
			curriculum := curriculumBuilder.Build(hs, as)
			return curriculum.Describe(), 0.8, true
		}

		// Analogical Reasoning (Bloque 28, Fase 3 Capa 3.8) — pregunta
		// distinta de Hypothesis Generator (Bloque 25): no busca en el
		// catálogo declarado de Actions, busca en lo que Ahmelia
		// REALMENTE ejecutó (eventLog, Fase 1-2). Encadena: Analyze (21)
		// → Decompose (22) → Map (23) → FindAnalogies (28) — no necesita
		// Solve/Generate/Check/Decide, solo los Gaps() de Knowledge
		// Mapper y el EventLog completo.
		if solving.ParseAnalogyQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			analogies := analogicalReasoner.FindAnalogies(km, eventStore.SnapshotLog())
			return analogies.Describe(), 0.8, true
		}

		// Self Evaluation (Bloque 29, Fase 3 Capa 3.9) — última capa,
		// cierra Fase 3. Pregunta distinta de las ocho anteriores: no
		// pide un aspecto del PLAN, pide cómo le fue al PROCESO de
		// razonamiento en sí. Por eso es la única que corre las ocho
		// capas completas, incluyendo Analogical Reasoning (que
		// Decision Engine, arriba, no necesita).
		if solving.ParseSelfEvalQuery(req) {
			analysis := goalAnalyzer.Analyze(input)
			plan := problemDecomposer.Decompose(analysis)
			km := knowledgeMapper.Map(plan)
			graph := dependencySolver.Solve(plan, km)
			hs := hypothesisGenerator.Generate(km, graph)
			report := constraintSolver.Check(analysis, km, graph)
			decision := decisionEngine.Decide(analysis.MainGoal, hs, report)
			analogies := analogicalReasoner.FindAnalogies(km, eventStore.SnapshotLog())
			evalReport := selfEvaluator.Evaluate(analysis, plan, km, graph, hs, report, decision, analogies)
			return evalReport.Describe(), 0.8, true
		}

		// ── Bloque 46 — Executive Project Manager (Planner + Progress) ──
		//
		// Distinto en naturaleza de todo lo de Fase 3 arriba: esas nueve
		// capas + Nivel II/III razonan sobre UN pedido de UN turno y no
		// guardan nada entre turnos. Esto administra objetivos que duran
		// días o semanas — necesita persistir Goal/Plan/Progress en el
		// Engine (executiveGoals/executivePlans/executiveProgress), no
		// recalcular todo de cero en cada consulta.
		if name, ok := planning.ParseCreateProjectQuery(norm); ok {
			goal := planning.NewGoal(name, "Creado desde conversación: "+input, planning.PriorityMedium, "user")
			if err := goalStore.Register(goal); err != nil {
				return fmt.Sprintf("No pude registrar el objetivo \"%s\": %v", name, err), 0.5, true
			}
			// Bloque 90.8.1 — EnsurePlan (autonomy/goal_planner.go) es
			// ahora el único punto que genera y cachea el Plan de un
			// Goal: antes de este bloque, esta línea llamaba a
			// planner.GeneratePlan(goal) por su cuenta, generando un
			// Plan y unas Tasks completamente distintas de las que el
			// ciclo autónomo iba a generar por separado la primera vez
			// que tocara este mismo Goal — dos fuentes de verdad para
			// el mismo trabajo (ver auditoría B90.8). RegisterTask ya no
			// se llama acá: EnsurePlan lo hace por dentro, para el
			// mismo Plan que ahora comparten B46 y el ciclo autónomo.
			plan, err := goalPlanningAdapter.EnsurePlan(goal)
			if err != nil {
				return fmt.Sprintf("No pude generar un plan viable para \"%s\": %v", name, err), 0.5, true
			}
			progress := planning.NewProgress(goal.ID, len(plan.Tasks), plan.TotalEstimate)

			key := shared.Normalize(name)
			executiveGoals[key] = goal
			executivePlans[goal.ID] = plan
			executiveProgress[goal.ID] = progress

			// Fase 6 (B72) — el objetivo recién creado entra al ciclo
			// de vida autónomo real: si Ahmelia no tenía nada activo,
			// este pasa a ser el foco (CurrentGoal); si ya había uno,
			// se encola (PendingGoals) — Goal Monitor (73) lo va a
			// vigilar en cualquiera de los dos lugares por igual (ver
			// Snapshot()), la diferencia es solo cuál es "lo que se
			// está haciendo ahora" vs. "lo que espera turno".
			if err := goalStore.ActivateOrQueue(goal); err != nil {
				return fmt.Sprintf("El proyecto \"%s\" quedó registrado pero no pude fijar su estado de trabajo: %v", name, err), 0.5, true
			}

			// ACTIVE es la señal causal de inicio de trabajo. Sin ella, el
			// objetivo recién creado solo podía alcanzar RESUME_GOAL después
			// del umbral de inactividad, que pertenece a recuperar trabajo
			// olvidado, no a iniciar trabajo nuevo.
			contextObserver.Observe(autonomy.TypeGoalStateChanged, "planning.GoalStore", map[string]interface{}{
				"goal_id": goal.ID,
				"name":    name,
				"status":  string(planning.GoalActive),
			})

			return fmt.Sprintf("Proyecto \"%s\" creado. %s", name, planner.AnalyzePlan(plan)), 0.8, true
		}

		if name, ok := planning.ParseProjectStatusQuery(norm); ok {
			goal, found := executiveGoals[shared.Normalize(name)]
			if !found {
				return fmt.Sprintf("No tengo ningún proyecto registrado como \"%s\" — para crear uno, pedime algo como \"quiero crear un proyecto para %s\".", name, name), 0.6, true
			}
			progress := executiveProgress[goal.ID]
			if progress == nil {
				return fmt.Sprintf("Tengo el proyecto \"%s\" registrado, pero sin seguimiento de progreso todavía.", name), 0.6, true
			}
			recs := strings.Join(progress.GetRecommendations(), " ")
			return fmt.Sprintf("%s\n\n%s", progress.GetStatus(), recs), 0.8, true
		}

		if name, ok := planning.ParseCompleteTaskQuery(norm); ok {
			goal, found := executiveGoals[shared.Normalize(name)]
			if !found {
				return fmt.Sprintf("No tengo ningún proyecto registrado como \"%s\".", name), 0.6, true
			}
			plan := executivePlans[goal.ID]
			progress := executiveProgress[goal.ID]
			if plan == nil || progress == nil {
				return fmt.Sprintf("Tengo el proyecto \"%s\", pero falta el plan o el seguimiento — no puedo registrar avance.", name), 0.6, true
			}
			next := planner.GetNextTask(plan)
			if next == nil {
				return fmt.Sprintf("El proyecto \"%s\" ya no tiene tareas pendientes — está completo.", name), 0.8, true
			}
			if next.StartedAt.IsZero() {
				next.Start()
			}
			next.Complete()
			progress.UpdateTaskCompleted()
			goal.UpdateProgress(progress.TasksCompleted, progress.TasksTotal)

			// Fase 6 (B72/B74) — si esta era la última tarea del plan,
			// el objetivo en sí se completa: es la ÚNICA señal
			// genuina que existe hoy en este código para alimentar
			// ContextObserver (74) con GOAL_STATE_CHANGED — no hay
			// watcher de archivos, foco de ventana ni señal de
			// sistema real conectado todavía (esos quedan sin
			// productor, documentado a propósito en vez de
			// fabricarlos). goal.Complete() es la misma llamada que
			// ya expone planning.Goal (usada por IsComplete()/agenda),
			// nunca invocada desde acá hasta ahora. Se saca también de
			// AutonomousState (72): un objetivo completo no es ni
			// CurrentGoal ni PendingGoal — Goal Monitor (73) dejaría
			// de reportarlo Stale, que sería engañoso.
			if progress.TasksTotal > 0 && progress.TasksCompleted >= progress.TasksTotal {
				goal.Complete()
				contextObserver.Observe(autonomy.TypeGoalStateChanged, "planning.ExecutiveProjectManager", map[string]interface{}{
					"goal_id": goal.ID,
					"name":    name,
					"status":  string(planning.GoalCompleted),
				})
				if current := goalStore.Current(); current != nil && current.ID == goal.ID {
					if err := goalStore.SetCurrent(nil); err != nil {
						return fmt.Sprintf("La tarea quedó completa, pero no pude persistir el cierre del objetivo \"%s\": %v", name, err), 0.5, true
					}
				} else {
					if err := goalStore.RemovePending(goal.ID); err != nil {
						return fmt.Sprintf("La tarea quedó completa, pero no pude persistir la cola del objetivo \"%s\": %v", name, err), 0.5, true
					}
				}
			}

			return fmt.Sprintf("Registrado: \"%s\" completada en \"%s\". %s", next.Title, name, progress.GetStatus()), 0.8, true
		}

		if !interaction.HasSemanticQuery(req, interaction.QueryHistoryActivity) {
			return "", 0, false
		}
		content, ok := memIntel.Query(req.Topic)
		if !ok {
			// La consulta histórica ya fue entendida semánticamente; lo que
			// falta es dato: ni tema activo con algo guardado ni
			// episodio previo en memory/long.json (normal en la
			// primera sesión con esta función, o si nunca cerraste
			// con "amelia salir" antes). Decir eso, en vez de dejar
			// que caiga a las demás fuentes y termine en el mensaje
			// genérico de incertidumbre (30%), que suena a que NO
			// entendió la pregunta cuando sí la entendió.
			return "Todavía no tengo ningún episodio anterior guardado — esto se llena la primera vez que cierre sesión", 0.6, true
		}
		return content, 0.75, ok
	}))
	knowledgeEngine.Register(knowledge.NewMentalStateSource(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		switch req.Meaning.Query {
		case interaction.QueryMentalEvolution:
			return mentalAgenda.GetMentalEvolution(), 0.7, true
		case interaction.QueryMentalDetailed:
			return mentalAgenda.GetMentalReport(), 0.8, true
		case interaction.QueryMentalState:
			return mentalAgenda.GetQuickMentalState(), 0.75, true
		default:
			return "", 0, false
		}
	}))
	// Phase 2 — Reasoner is deliberately NOT registered as a KnowledgeEngine
	// response source. Its unique reasoning capabilities are consumed by
	// collectSpecialistEvidence(), which exposes them as evidence to Situation
	// and Cognition before the canonical ResponseContract/Language boundary.
	knowledgeEngine.Register(knowledge.NewEnvironmentSource(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		if req.Meaning.Query == interaction.QueryEnvironmentResources {
			res, err := environment.GetSystemResources()
			if err != nil {
				return "No pude leer el estado del sistema ahora mismo", 0.6, true
			}
			return res.Describe(), 0.85, true
		}

		// Capa C legítima: las categorías describen texto máquina/estado
		// del inventario de aplicaciones. G3 no reemplaza ese clasificador.
		if req.Meaning.Query == interaction.QueryEnvironmentOpenApps {
			if desc, ok := world.Describe(); ok {
				return desc, 0.9, true
			}
			return "No tengo ninguna aplicación abierta que yo sepa", 0.6, true
		}
		if req.Meaning.Query == interaction.QueryEnvironmentTabs {
			if desc, ok := world.BrowserTabs(); ok {
				return desc, 0.85, true
			}
			if app, ok := world.ActiveApp(); ok && app.Name == "chrome" {
				return "Chrome está abierto, pero no puedo ver las pestañas — necesita el puerto de depuración habilitado (--remote-debugging-port) para eso", 0.75, true
			}
			return "No tengo ninguna pestaña de navegador para reportar ahora mismo", 0.7, true
		}
		if req.Meaning.Query == interaction.QueryEnvironmentContext {
			if snap, ok := world.WorldSnapshot(); ok {
				return snap.Describe(), 0.85, true
			}
			return "No tengo suficiente información sobre lo que estás haciendo ahora mismo", 0.7, true
		}
		if req.Meaning.Query == interaction.QueryEnvironmentActiveWork {
			if desc, ok := world.ActiveWork(); ok {
				return desc, 0.85, true
			}
			return "No tengo ningún archivo o proyecto reconocible abierto ahora mismo", 0.75, true
		}

		// CategoryQuery permanece fuera de G3 como clasificador C. Su texto
		// llega aquí solo después de que el contrato semántico no haya
		// reclamado una de las cinco consultas de entorno.
		norm := shared.Normalize(req.Raw)
		if cat, ok := environment.ParseCategoryQuery(norm); ok {
			if desc, ok := world.DescribeByCategory(cat); ok {
				return desc, 0.85, true
			}
		}
		return "", 0, false
	}))
	knowledgeEngine.Register(knowledge.NewSelfKnowledgeSource(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		ans, ok := selfQuery.QuerySemantic(req)
		return ans, 0.99, ok
	}))

	// D2 — external search is a controlled Knowledge source. The capability
	// router decides the search lane; Knowledge owns external-source evidence;
	// the existing Discovery → Gateway → Validation chain decides whether a
	// fetched document is admissible. No search path registers or executes a
	// capability. The source is disabled unless the host explicitly enables
	// AHMELIA_KNOWLEDGE_INTERNET.
	internetSource := knowledge.NewInternetSourceWithConfig(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		goal := strings.TrimSpace(req.Objective.Text)
		if goal == "" {
			goal = strings.TrimSpace(req.Topic)
		}
		if goal == "" || knowledgeDiscoverer == nil || knowledgeGateway == nil || sourceValidator == nil {
			return "", 0, false
		}
		discovery := knowledgeDiscoverer.Discover(goal)
		if !discovery.HasSources() || len(discovery.ResourceURLs) == 0 {
			return "", 0, false
		}
		docs, _ := knowledgeGateway.FetchAll(discovery.ResourceURLs)
		validation := sourceValidator.ValidateAll(discovery.Goal, docs)
		var chosen *gateway.Document
		for _, checked := range validation.Validations {
			if checked != nil && checked.IsReliable() && checked.Document != nil {
				chosen = checked.Document
				break
			}
		}
		if chosen == nil {
			return "", 0, false
		}
		text := strings.TrimSpace(chosen.Text)
		if len(text) > 360 {
			text = text[:360] + "…"
		}
		confidence := 0.82
		if validation.Corroborated {
			confidence = 0.92
		}
		content := fmt.Sprintf("Fuente autorizada: %s (%s). Contenido recuperado y validado: %s", chosen.Source, chosen.URL, text)
		return content, confidence, true
	}, knowledge.InternetSourceConfig{Enabled: knowledge.InternetSourceEnabled(os.Getenv)})
	knowledgeEngine.Register(internetSource)

	// ========================================================================
	// B94-B96 — INTEGRACIÓN REAL AL ENGINE
	// ========================================================================
	// Antes de esta integración, CapabilityRegistry/Discovery y Programming
	// podían compilar y pasar sus tests pero ninguna instancia pertenecía al
	// runtime principal. El Engine ahora conserva una única instancia viva.
	capRegistry := capability.NewRegistry()
	_ = integration.RegisterCapabilities(capRegistry)
	capDiscovery := capability.NewDiscovery(capRegistry)
	capResolver := capability.NewResolver(capRegistry)
	// E1 — Procedure Registry (cableado, no ejecutado).
	procedures, procErr := procedure.Open("memory/procedures.json")
	if procErr != nil {
		fmt.Printf("⚠️ [E1] no se pudo abrir registry de procedimientos: %v — módulo deshabilitado\n", procErr)
		procedures = nil
	}

	productionProviders := production.NewProviderRegistry()

	// One local language-provider lifecycle belongs to the Engine. The same
	// Ollama client proposes code, task graphs and language; OpenCode is not an
	// Ahmelia runtime dependency.
	linguisticClient := linguistics.NewOllamaClient()
	// The linguistic model proposes a task graph; the canonical planning.Planner
	// still validates and owns Goal/Plan/Task identity.
	planner.SetGoalTaskDecomposer(integration.NewOllamaGoalTaskDecomposer(linguisticClient))
	linguisticInterpreter := linguistics.NewInterpreter(linguisticClient)
	linguisticPipeline := linguistics.NewPipeline(linguistics.NewRouter(), linguisticInterpreter, linguistics.NewValidator())
	deferredLinguistic := linguistics.NewDeferredResolver(linguisticInterpreter, 16)
	// T3 — Gobernador de recursos (cableado 2026-10-17): una única ranura de
	// modelo para todo el sistema, con prioridades reales (el turno del
	// usuario nunca es desplazado) y cola con anti-inanición. El motor ya
	// existía completo en resource/ pero nadie lo pedaleaba (auditoría T0).
	// El preempt se dispara al encolar; el envejecimiento se recalcula cuando
	// el driver periódico (latido N0) llame PreemptIfNeeded.
	resourceGovernor := resource.NewGovernor(resource.GovernorConfig{})
	linguisticClient.Gate = resource.NewLinguisticsGate(resourceGovernor)
	programmingSynthesizer := integration.NewStructuredCodeObjectiveSynthesizerWithClient(linguisticClient)
	if store, err := experience.Open(experience.StorePath); err == nil {
		programmingSynthesizer.Experience = store
	}
	_ = integration.RegisterRuntimeProvidersWithSynthesizer(capRegistry, productionProviders, world, programmingSynthesizer)
	programmingIntegration, _ := integration.NewProgrammingIntegration(world)
	technicalRecovery := production.NewTechnicalRecoveryService()
	productionContext := &production.ProductionContext{}
	workStoreStorage := production.NewWorkStore("memory/production_work.json")
	workStore := NewPersistentWorkStore(workStoreStorage)
	productionRuntime, _ := integration.NewProductionRuntime(capResolver, capRegistry, productionProviders, productionContext, workStore, technicalRecovery)

	// Programming usa el mismo Workspace de Production y los adaptadores OS
	// especializados de B96. No se crea un segundo mundo ni un segundo
	// executor genérico. El workspace raíz de trabajo se resuelve por petición.
	userProfile := interaction.LoadUserProfile(interaction.UserProfilePath)
	userID := strings.TrimSpace(userProfile.PrimaryAuthority.ID)
	if userID == "" {
		userID = "default-user"
	}
	userModelStore, _ := interaction.LoadUserModelStore(interaction.UserModelPath, userID)
	// P4 — posterior de preferencias: carga honesta (fail-open) del snapshot.
	prefModel := loadUserPreferenceModel()
	engine := &Engine{
		sess:      sess,
		longMem:   longMem,
		shortMem:  shortMem,
		learner:   learner,
		idx:       loadOrCreateIndex(),
		context:   convoContext,
		history:   history,
		selfQuery: selfQuery,
		reasoner:  reasoner,

		// FASE 2 - Consolidación Cognitiva
		knowledgeEngine:     knowledgeEngine,
		uncertaintyHandler:  language.NewUncertaintyHandler(),
		world:               world,
		refResolver:         reference.New(world),
		contextIntelligence: contextintelligence.New(),
		naturalDialogue:     naturaldialogue.NewIntegration(),
		socialInteraction:   socialinteraction.NewIntegration(),
		conversationalInitiative: func() *conversationalinitiative.Policy {
			p := conversationalinitiative.DefaultPolicy()
			p.Timing = conversationalinitiative.NewTimingEvaluator()
			return &p
		}(),
		initiativeSignals:               conversationalinitiative.NewRuntime(),
		initiativeActionSelector:        conversationalinitiative.NewActionSelector(),
		initiativeActionContract:        conversationalinitiative.NewActionContractBuilder(),
		initiativeTimingEvaluator:       conversationalinitiative.NewTimingEvaluator(),
		initiativeAvailabilityPolicy:    conversationalinitiative.NewAvailabilityPolicy(),
		initiativePriorityUrgencyPolicy: conversationalinitiative.NewPriorityUrgencyPolicy(),
		initiativePrivacyPolicy: func() *conversationalinitiative.DisclosurePolicy {
			p := conversationalinitiative.DefaultDisclosurePolicy()
			return &p
		}(),
		initiativeInterruptionPolicy:        conversationalinitiative.NewInterruptionPolicy(),
		initiativeConversationLoadEvaluator: conversationalinitiative.NewConversationLoadEvaluator(),
		initiativeDeferredIntentManager:     conversationalinitiative.NewDeferredIntentManager(nil),
		initiativeScheduler:                 conversationalinitiative.NewInitiativeScheduler(nil),
		initiativeOutcomeEvaluator:          conversationalinitiative.NewOutcomeEvaluator(nil),
		initiativeArbiter:                   conversationalinitiative.NewInitiativeArbiter(),
		focusArbiter:                        NewFocusArbiter(conversationalinitiative.NewInterruptionPolicy()),
		initiativeSimilarityGuard: func() *interaction.InitiativeSimilarityGuard {
			ollamaURL := os.Getenv("AHMELIA_OLLAMA_URL")
			embeddingModel := os.Getenv("AHMELIA_EMBEDDING_MODEL")
			if strings.TrimSpace(ollamaURL) != "" || strings.TrimSpace(embeddingModel) != "" {
				provider := interaction.NewOllamaEmbeddingProvider(ollamaURL, embeddingModel)
				return interaction.NewInitiativeSimilarityGuard(provider)
			}
			return nil
		}(),
		initiativeLifecycleManager: conversationalinitiative.NewLifecycleManager(nil),
		initiativeObservability:    conversationalinitiative.NewInitiativeObservability(nil),
		episode:                    memoryintelligence.NewEpisode(),
		eventStore:                 eventStore,
		learningState:              learningState,

		// FASE 2.6 - Integración Cognitiva
		agenda:                agenda,
		planner:               planner,
		executiveGoals:        executiveGoals,
		executivePlans:        executivePlans,
		executiveProgress:     executiveProgress,
		contextBuilder:        contextBuilder,
		contextCache:          contextCache,
		pipeline:              pipeline,
		internalState:         internalState,
		conversationMode:      conversationMode,
		relationship:          relationship,
		conversationFlow:      conversationFlow,
		conversationalMemory:  conversationalMemory,
		conversationHistory:   conversationHistory,
		conversationEpisodeID: conversationEpisodeID,
		socialMemory:          socialMemory,
		skillLibrary:          skillLibrary,
		knowledgeMemory:       knowledgeMemory,
		knowledgeDiscoverer:   knowledgeDiscoverer,
		contextAdapter:        contextAdapter,
		humorStyle:            humorStyle,
		attention:             attention,
		workingMemory:         workingMemory,

		// FASE 3 - Cognitive Mind
		goalAnalyzer:        goalAnalyzer,
		problemDecomposer:   problemDecomposer,
		knowledgeMapper:     knowledgeMapper,
		dependencySolver:    dependencySolver,
		hypothesisGenerator: hypothesisGenerator,
		constraintSolver:    constraintSolver,
		decisionEngine:      decisionEngine,
		analogicalReasoner:  analogicalReasoner,
		selfEvaluator:       selfEvaluator,
		reasoningCache:      reasoningCache,

		// CAPA 5.5 - Pensamiento Deliberativo
		perspectiveEngine:    reasoning_deep.NewPerspectiveEngine(),
		confidenceCalculator: reasoning_deep.NewConfidenceCalculator(),
		evaluator:            reasoning_deep.NewEvaluator(),
		conclusionBuilder:    reasoning_deep.NewConclusionBuilder(),
		conversationPlanner:  reasoning_deep.NewConversationPlanner(),
		mentalAgenda:         mentalAgenda,

		// CAPA 6 - Generación de Lenguaje Natural
		languageGenerator:     language.NewLanguageGenerator(),
		conversationGenerator: language.NewConversationalGenerator(linguisticClient),
		responseComposer:      responseComposer,
		interactionManager:    interaction.NewManager(),
		clarificationQueue:    interaction.NewClarificationQueue(),

		conversationState:     interaction.LoadConversationState(interaction.ConversationStatePath),
		threadRouter:          interaction.NewThreadRouter(),
		workBridge:            interaction.NewWorkBridge(),
		userProfile:           userProfile,
		personalMemory:        interaction.LoadPersonalMemory(interaction.PersonalMemoryPath),
		userModelStore:        userModelStore,
		prefModel:             prefModel,
		authorityService:      interaction.NewAuthorityService(interaction.NewUserProfile().PrimaryAuthority),
		linguisticClient:      linguisticClient,
		resourceGovernor:      resourceGovernor,
		linguisticInterpreter: linguisticInterpreter,
		linguisticPipeline:    linguisticPipeline,
		deferredLinguistic:    deferredLinguistic,
		// La deliberación en sombra comparte el cliente lingüístico (misma
		// ranura y mismo modelo) pero entra con prioridad propia. El registro
		// en disco queda deshabilitado hasta que el composition root llame a
		// EnableShadowDeliberation; las suites no escriben estado vivo.
		deliberationClient: linguisticClient,
		deliberationShadow: deliberation.NewShadowLog(""),
		agentGoalLedger:    agency.NewLedger(""),
		selfTaste:          taste.NewModel(taste.NewLedger("")),

		capabilityRegistry:     capRegistry,
		procedures:             procedures,
		capabilityDiscovery:    capDiscovery,
		capabilityResolver:     capResolver,
		productionProviders:    productionProviders,
		programmingIntegration: programmingIntegration,
		technicalRecovery:      technicalRecovery,
		productionContext:      productionContext,
		workStore:              workStore,
		goalStore:              goalStore,
		productionRuntime:      productionRuntime,
		// FASE 6 — Autonomía Cognitiva e Iniciativa (B72-77).
		// autonomousLoop (77) NO se instancia acá — necesita los
		// adaptadores StateProvider/GoalProvider reales, que a su vez
		// necesitan un *Engine ya construido (GoalMonitor cuelga de
		// autonomousState, que sí existe acá, pero el Loop en sí recién
		// se arma en StartAutonomy(), ver más abajo) — mismo criterio
		// que autonomyCancel, que arranca nil hasta que StartAutonomy
		// lo llene.
		autonomousState:     autonomousState,
		goalMonitor:         goalMonitor,
		goalPlanningAdapter: goalPlanningAdapter,
		contextObserver:     contextObserver,
		initiativeEngine:    initiativeEngine,
		conversationManager: conversationManager,

		folderDst: map[string]string{
			"escritorio": filepath.Join(home, "Desktop"),
			"desktop":    filepath.Join(home, "Desktop"),
			"documentos": filepath.Join(home, "Documents"),
			"documents":  filepath.Join(home, "Documents"),
			"descargas":  filepath.Join(home, "Downloads"),
			"downloads":  filepath.Join(home, "Downloads"),
		},
	}

	// Reconcile persisted conversational ownership before the first turn. A
	// previous process may have been killed while a delegation was still only
	// a conversational draft; it must not reappear as an operational objective
	// or contaminate a new Work session after restart.
	hasOperationalWork := false
	if engine.workStore != nil {
		if w, ok := engine.workStore.Last(); ok {
			switch strings.ToLower(strings.TrimSpace(string(w.Status))) {
			case "pending", "running", "blocked", "recovering":
				hasOperationalWork = true
			}
		}
	}
	pendingAuthorizationCount := recoverPendingAuthorizationOwner(engine.conversationState, engine.workStore, engine.sess)
	if pendingAuthorizationCount > 0 {
		hasOperationalWork = true
	}
	engine.conversationState.ReconcileObjectiveOwnership(hasOperationalWork)
	if _, ok := engine.conversationState.PendingObjectiveDraft(); ok && engine.sess != nil {
		engine.sess.SetVoiceContinuation()
	}

	engine.interactionState = interaction.NewInteractionState(engine.conversationState)
	goalStore.SetPersister(engine.PersistAutonomyState)

	// B96.42.1 — every legacy local action is now a declared capability/provider.
	// The Engine exposes only the provider-side adapter; authorityExecute never
	// calls Engine.execute directly.
	if err := integration.RegisterLocalActionProvider(capRegistry, productionProviders, engine); err != nil {
		panic(fmt.Sprintf("B96.42.1: no se pudo registrar el provider de acciones locales: %v", err))
	}

	// Authority is derived from the same persisted profile instance; identity,
	// authentication and authorization remain separate contracts.
	if engine.userProfile != nil {
		engine.authorityService = interaction.NewAuthorityService(engine.userProfile.PrimaryAuthority)
	}

	// B96.44 — Core-owned Goal -> Plan -> Workflow execution bridge. The
	// dispatcher is orchestration only; physical execution still crosses the
	// existing Engine authority through AutonomyExecutor.
	goalCompiler, err := integration.NewGoalPlanCompiler(goalPlanningAdapter)
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir GoalPlanCompiler: %v", err))
	}
	dispatcher, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir Goal dispatcher: %v", err))
	}
	worker := &execution.LocalWorker{WorkerID: "core-goal-workflow", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"goal": true}, Capabilities: map[string]bool{"produce": true, "action": true}, CPU: 1, MemoryMB: 1024}}
	if err := dispatcher.RegisterWorker(worker); err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo registrar Goal worker: %v", err))
	}
	graph, err := execution.NewTaskGraphExecutor(dispatcher, 0)
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir Goal graph: %v", err))
	}
	checkpointDir := filepath.Join("memory", "goal_workflow_checkpoints")
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(checkpointDir)
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir Goal checkpoints: %v", err))
	}
	workflow, err := integration.NewWorkflowRunner(graph, workStore, checkpoints)
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir Goal workflow: %v", err))
	}
	goalWorkflow, err := integration.NewGoalWorkflowRunner(workflow, goalStore)
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir Goal workflow bridge: %v", err))
	}
	goalExecution, err := integration.NewGoalExecutionRunner(goalCompiler, goalWorkflow, NewAutonomyExecutor(engine), goalActionVerifier{technical: autonomy.NewActionVerifier(), inspector: engineEnvironmentInspector{}})
	if err != nil {
		panic(fmt.Sprintf("B96.44: no se pudo construir Goal execution runner: %v", err))
	}
	goalExecution.SetActionReconciler(engineEnvironmentInspector{})
	engine.goalPlanCompiler = goalCompiler
	engine.goalExecutionRunner = goalExecution
	engine.autonomyExecutor = NewAutonomyExecutor(engine)

	// Domain providers are composed before the Engine starts. The Engine only
	// consumes the generic ProviderRegistry and never owns domain-specific
	// registration logic.
	return engine
}

// recoverPendingAuthorizationOwner restores the exact conversational owner
// only when the persistent store contains one unambiguous pending execution.
// Multiple pending contracts clear conversational selection and remain closed
// until explicit reconciliation. The direct voice mode is restored with the
// unique pending owner so a spoken answer can continue after process restart.
func recoverPendingAuthorizationOwner(state *interaction.ConversationState, store *PersistentWorkStore, sess *memory.Session) int {
	if state == nil || store == nil {
		return 0
	}
	pending := store.PendingAuthorizations()
	switch len(pending) {
	case 0:
		return 0
	case 1:
		state.SetActiveWork(pending[0].ID)
		if sess != nil {
			sess.SetVoiceContinuation()
		}
	default:
		state.SetActiveWork("")
		if sess != nil {
			sess.RestoreVoiceMode()
		}
	}
	return len(pending)
}

func interactionNeedsLinguisticDepth(r interaction.SemanticRequest) bool {
	return interaction.NeedsLinguisticDepth(r)
}

// markLinguisticTrace makes the outcome of every attempt to reach the deep
// linguistic interpreter observable in the flow trace. Before this, a
// skipped call (interpreter absent, Ollama unreachable), a call that errored
// (network/timeout, invalid JSON from the model), and a call that succeeded
// but was silently dropped downstream were all indistinguishable from the
// outside: the turn simply looked like it had no linguistic depth. This is a
// diagnostic aid only — it does not change routing, memory, or response
// behavior.
func (e *Engine) markLinguisticTrace(detail string) {
	if e == nil || e.flowTrace == nil {
		return
	}
	e.flowTrace.Mark("LINGUISTIC", detail)
}

func mergeLinguisticInterpretation(r interaction.SemanticRequest, li linguistics.Interpretation) interaction.SemanticRequest {
	if li.Intent != "" {
		switch li.Intent {
		case comprehension.IntentQuestion:
			if r.Kind == interaction.KindUnknown || r.Kind == interaction.KindConversation {
				r.Kind = interaction.KindQuestion
			}
		case comprehension.IntentCommand:
			if r.Kind == interaction.KindUnknown {
				r.Kind = interaction.KindContinuation
			}
		case comprehension.IntentCorrection:
			r.Kind = interaction.KindCorrection
		case comprehension.IntentFeedback:
			if r.Kind == interaction.KindUnknown {
				r.Kind = interaction.KindInform
			}
		}
	}
	if li.Confidence > r.Confidence {
		r.Confidence = li.Confidence
	}

	// Preserve the complete linguistic evidence at the semantic boundary.
	// Linguistics may enrich meaning, but it does not become an authority for
	// action selection or durable memory mutation.
	r.Entities = appendUniqueStrings(r.Entities, li.Entities...)
	for _, rel := range li.Relations {
		source, predicate, target := strings.TrimSpace(rel.Source), strings.TrimSpace(rel.Predicate), strings.TrimSpace(rel.Target)
		if source == "" || predicate == "" || target == "" {
			continue
		}
		conf := rel.Confidence
		if conf <= 0 {
			conf = li.Confidence
		}
		if conf > 1 {
			conf = 1
		}
		exists := false
		for _, existing := range r.Relations {
			if strings.EqualFold(existing.Source, source) && strings.EqualFold(existing.Predicate, predicate) && strings.EqualFold(existing.Target, target) {
				exists = true
				break
			}
		}
		if !exists {
			r.Relations = append(r.Relations, interaction.SemanticRelation{Source: source, Predicate: predicate, Target: target, Value: strings.TrimSpace(rel.Value), Confidence: conf, Evidence: "linguistics:relation"})
		}
	}
	for _, expression := range li.References {
		if strings.TrimSpace(expression) == "" {
			continue
		}
		r.References = appendUniqueInteractionReference(r.References, interaction.Reference{
			Kind: interaction.ReferenceNone, Expression: strings.TrimSpace(expression), Resolved: false,
		})
	}
	if len(li.Constraints) > 0 {
		r.Objective.Constraints = appendUniqueStrings(r.Objective.Constraints, li.Constraints...)
	}
	if strings.TrimSpace(li.Topic) != "" {
		r.Topic = strings.TrimSpace(li.Topic)
	}
	if strings.TrimSpace(li.QuestionType) != "" {
		r.QuestionType = strings.TrimSpace(li.QuestionType)
	}
	if strings.TrimSpace(li.Directness) != "" {
		r.Directness = strings.TrimSpace(li.Directness)
	}
	if strings.TrimSpace(li.CorrectionTarget) != "" {
		r.CorrectionTarget = strings.TrimSpace(li.CorrectionTarget)
	}
	if strings.TrimSpace(li.SpeechAct) != "" {
		r.SpeechAct = interaction.SpeechAct(strings.TrimSpace(li.SpeechAct))
	}
	if strings.TrimSpace(li.Meaning) != "" {
		r.Meaning.Evidence = appendUniqueStrings(r.Meaning.Evidence, li.Meaning)
		if r.Meaning.SourceText == "" {
			r.Meaning.SourceText = r.Raw
		}
	}

	// Rebuild the interaction-layer communicative-act contract after linguistic
	// enrichment. Linguistics contributes evidence; it never selects behavior.
	r.CommunicativeActs = interaction.ClassifyCommunicativeActs(r)
	if liAct := strings.ToLower(strings.TrimSpace(li.SpeechAct)); liAct != "" && liAct != "unknown" {
		var act interaction.CommunicativeAct
		switch liAct {
		case "question":
			act = interaction.ActQuestion
		case "command":
			act = interaction.ActCommand
		case "inform":
			act = interaction.ActInform
		case "propose":
			act = interaction.ActProposal
		case "correct":
			act = interaction.ActCorrection
		case "confirm":
			act = interaction.ActConfirmation
		case "reject":
			act = interaction.ActRejection
		case "continue":
			act = interaction.ActContinuation
		case "social":
			act = interaction.ActSocial
		case "feedback":
			act = interaction.ActFeedback
		case "learning":
			act = interaction.ActLearning
		}
		if act != "" {
			seen := r.CommunicativeActs.Primary == act
			if !seen {
				for _, a := range r.CommunicativeActs.Secondary {
					if a == act {
						seen = true
						break
					}
				}
			}
			if !seen {
				r.CommunicativeActs.Secondary = append(r.CommunicativeActs.Secondary, act)
			}
			r.CommunicativeActs.Evidence = append(r.CommunicativeActs.Evidence, interaction.CommunicativeActEvidence{Act: act, Confidence: li.Confidence, Source: "linguistics:speech_act"})
		}
	}
	for _, signal := range li.UserStateSignals {
		if strings.TrimSpace(signal) != "" {
			r.Meaning.Evidence = appendUniqueStrings(r.Meaning.Evidence, "linguistic_user_state:"+signal)
		}
	}
	return r
}

// mergeOpenMeaning projects the open comprehension contract onto the semantic
// request as evidence-level enrichment. Fine discrimination (preference,
// correction, third-party comment...) is reasoned later by mechanisms over
// Meaning; this adapter only preserves what the boundary helpers already
// consumed: communicative kind, confidence, entities (mentions) and topic.
func mergeOpenMeaning(r interaction.SemanticRequest, om linguistics.OpenMeaning) interaction.SemanticRequest {
	switch om.Purpose {
	case linguistics.OpenPurposeQuestion:
		if r.Kind == interaction.KindUnknown || r.Kind == interaction.KindConversation {
			r.Kind = interaction.KindQuestion
		}
	case linguistics.OpenPurposeRequest:
		if r.Kind == interaction.KindUnknown {
			r.Kind = interaction.KindContinuation
		}
	case linguistics.OpenPurposeFeedback:
		if r.Kind == interaction.KindUnknown {
			r.Kind = interaction.KindInform
		}
	}
	if om.Confidence > r.Confidence {
		r.Confidence = om.Confidence
	}
	r.Entities = appendUniqueStrings(r.Entities, om.Mentions...)
	if strings.TrimSpace(om.Topic) != "" {
		r.Topic = strings.TrimSpace(om.Topic)
	}
	if strings.TrimSpace(om.Meaning) != "" {
		r.Meaning.Evidence = appendUniqueStrings(r.Meaning.Evidence, om.Meaning)
		if r.Meaning.SourceText == "" {
			r.Meaning.SourceText = r.Raw
		}
	}
	// B (2026-10-18): cuando el SEMANTIC de reglas deja el sujeto vacío, la
	// lectura abierta resuelve de quién es el dato del turno (owner_of_data):
	// user → sujeto user, ahmelia → sujeto self. Terceros ("externo") quedan
	// fuera de la fase compañera. Un sujeto ya resuelto por reglas no se pisa.
	if r.Subject == "" {
		switch om.OwnerOfData {
		case linguistics.OpenOwnerUser:
			r.Subject = interaction.UserObservationSubjectUser
		case linguistics.OpenOwnerAhmelia:
			r.Subject = interaction.UserObservationSubjectSelf
		}
	}
	r.CommunicativeActs = interaction.ClassifyCommunicativeActs(r)
	// C (2026-10-18): el acto tipado de la lectura abierta enruta la conducta
	// cuando las reglas locales no la clasificaron (elogios largos, frases
	// sobre el futuro de Ahmelia): evaluación → feedback, y el renderizador
	// tiene su escena.
	r = openReadingActDerivation(r, om)
	return r
}

// openReadingActDerivation aplica el acto tipado de una lectura abierta sobre
// los actos comunicativos del turno. Las reglas clasifican la superficie; la
// lectura clasificó el significado (evaluación → feedback). Nunca pisa actos
// ejecutables, preguntas, correcciones o aprendizaje: la lectura abierta no
// autoriza otra cosa. Es idempotente y se re-aplica después de la
// reclasificación textual final del turno, que no puede ver lo que la lectura
// sí vio.
func openReadingActDerivation(r interaction.SemanticRequest, om linguistics.OpenMeaning) interaction.SemanticRequest {
	if om.Purpose != linguistics.OpenPurposeFeedback {
		return r
	}
	if r.CommunicativeActs.Has(interaction.ActCommand) ||
		r.CommunicativeActs.Has(interaction.ActControl) ||
		r.CommunicativeActs.Has(interaction.ActCorrection) ||
		r.CommunicativeActs.Has(interaction.ActLearning) ||
		r.CommunicativeActs.Has(interaction.ActQuestion) {
		return r
	}
	confidence := om.Confidence
	if confidence < .5 {
		confidence = .5
	}
	r.CommunicativeActs = r.CommunicativeActs.WithDerivedAct(interaction.ActFeedback, confidence, "open_reading:evaluacion")
	return r
}

// openMeaningEvidence is the most recent open comprehension observed at the
// live linguistic boundary. Mechanisms may reason over it; it never mutates
// durable stores by itself and it is never a conduct decision.
type openMeaningEvidence struct {
	Raw        string
	Meaning    linguistics.OpenMeaning
	ObservedAt time.Time
}

// adoptOpenMeaning records the most recent open comprehension as engine
// evidence for mechanisms.
func (e *Engine) adoptOpenMeaning(raw string, om linguistics.OpenMeaning) {
	if e == nil {
		return
	}
	e.lastOpenMeaning = &openMeaningEvidence{Raw: raw, Meaning: om, ObservedAt: time.Now()}
}

// SetCompetenceOverlay adjunta la lectura de competencia a la situación
// compartida (P2.3/T4-M5, directiva DeepSeek 19/10 — la pieza estaba escrita
// en integration.CompetencePerspective sin caller). Lo llama el composition
// root después de crear el overlay; nil o no llamado = sin perspectiva y la
// conducta previa queda intacta. La perspectiva NO vota ni ejecuta: solo
// traduce Standing derivado a la lectura de situación.
func (e *Engine) SetCompetenceOverlay(o *competence.Overlay) {
	if e == nil {
		return
	}
	e.competenceOverlay = o
}

// WorkStore expone el almacén de trabajo real persistido. Caller de
// producción del feed idempotente de competencia (P2/Paso 1.2): main
// reconstruye la ledger desde aquí tras crear el engine. Solo lectura — el
// dueño del almacén sigue siendo el engine.
func (e *Engine) WorkStore() *PersistentWorkStore {
	if e == nil {
		return nil
	}
	return e.workStore
}

// EnableOpenComprehension switches the live engine's linguistic boundary from
// the closed interpretation contract to the open meaning contract (owner
// directive 2026-10-17 16:04). The composition root enables it for the live
// process; suites and focused fixtures keep the closed contract by default.
func (e *Engine) EnableOpenComprehension() {
	if e == nil {
		return
	}
	if e.linguisticPipeline != nil {
		e.linguisticPipeline.SetOpenComprehension(true)
	}
	if e.deferredLinguistic != nil {
		e.deferredLinguistic.SetOpenComprehension(true)
	}
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := map[string]bool{}
	for _, v := range dst {
		seen[strings.ToLower(strings.TrimSpace(v))] = true
	}
	for _, v := range values {
		k := strings.ToLower(strings.TrimSpace(v))
		if k == "" || seen[k] {
			continue
		}
		dst = append(dst, strings.TrimSpace(v))
		seen[k] = true
	}
	return dst
}

func appendUniqueInteractionReference(dst []interaction.Reference, ref interaction.Reference) []interaction.Reference {
	for _, existing := range dst {
		if strings.EqualFold(existing.Expression, ref.Expression) {
			return dst
		}
	}
	return append(dst, ref)
}

// resumeClarificationIfStillOpen — B96.22.6. Se llama al cerrar CADA turno
// (defer). id es la aclaración que ya estaba pendiente ANTES de procesar
// este turno (0 si no había ninguna). Si sigue abierta después de procesar
// el turno — es decir, el turno no la contestó, la resolvió otra cosa — se
// retoma acá, en el mismo cierre, tal como decidió el usuario explícitamente
// ("atender lo nuevo pero mantener la aclaración pendiente para retomarla
// después"). Una aclaración recién creada EN este mismo turno (id==0 al
// entrar) nunca se retoma acá: ya fue entregada como la respuesta principal
// del turno por quien la encoló (ver AskClarification).
func (e *Engine) resumeClarificationIfStillOpen(id uint64) {
	if e == nil || e.clarificationQueue == nil || id == 0 {
		return
	}
	pc := e.clarificationQueue.Oldest()
	if pc == nil || pc.ID() != id || pc.Status != interaction.ClarificationOpen {
		return
	}
	e.renderCanonicalSystemResponse(interaction.SemanticRequest{Raw: pc.Question, Objective: interaction.Objective{Text: pc.Question}}, "clarification_resume", "Volviendo a lo anterior: "+pc.Question, interaction.MoveClarify, interaction.ResponseIntentResume, true)
}

// AskClarification — B96.22.6. Punto de entrada único para que cualquier
// consumidor (ProductionRuntime, un handler de acciones, una capacidad
// futura) le pida al usuario información que falta sin perder el objetivo
// original. Encola la pregunta en clarificationQueue (sobrevive a que el
// usuario se desvíe con otra cosa en el medio) y devuelve el texto de la
// pregunta para que el llamador lo use como respuesta de este turno.
// originalGoal debe ser el texto crudo del objetivo (mismo criterio que
// memory.PendingClarification: crudo, no ya recortado), y assumptions son
// las opciones/supuestos que motivan la pregunta (ej. ["iluminación",
// "interacción"]), usados después para reconocer una respuesta corta que
// elige una de ellas.
func (e *Engine) AskClarification(originalGoal, question string, assumptions ...string) string {
	if e == nil || e.clarificationQueue == nil {
		return question
	}
	e.clarificationQueue.Push(originalGoal, question, assumptions)
	return question
}

// analyzeSemanticBoundary is the only Engine bridge from raw human input
// into the interaction semantic contract. Reanalysis replaces the in-flight
// Manager record rather than creating a second turn, and both paths apply the
// same linguistic enrichment + semantic evaluation contract.
func (e *Engine) analyzeSemanticBoundary(input string, ctx interaction.Context, reanalyze bool, pending *[]pendingDeferredReading) interaction.SemanticRequest {
	if e == nil || e.interactionManager == nil {
		return interaction.Analyze(input, ctx)
	}
	var r interaction.SemanticRequest
	if reanalyze {
		r = e.interactionManager.Reanalyze(input, ctx)
	} else {
		r = e.interactionManager.Analyze(input, ctx)
	}

	// turnReading retiene la lectura abierta adoptada por este turno. La
	// reclasificación textual final no puede ver lo que la lectura sí vio
	// (elogios largos, frases sobre el futuro de Ahmelia): su acto tipado se
	// re-aplica después de clasificar para que llegue al renderizador.
	var turnReading *linguistics.OpenMeaning

	// B96.39.35 — resolved deferred evidence is reusable by semantic signature.
	// Cache access is evidence enrichment; it never bypasses the interaction
	// authority or produces a response by itself.
	if e.deferredLinguistic != nil {
		if cached, ok := e.deferredLinguistic.Lookup(r); ok {
			if cached.Open != nil {
				om := *cached.Open
				r = mergeOpenMeaning(r, om)
				e.adoptOpenMeaning(cached.Job.Request.Raw, om)
				turnReading = &om
				e.markLinguisticTrace(fmt.Sprintf("mode=cache result=ok comprehension=open job=%s signature=%q purpose=%s topic=%q confidence=%.2f", cached.Job.ID, cached.Job.Signature, om.Purpose, om.Topic, om.Confidence))
			} else {
				r = mergeLinguisticInterpretation(r, cached.Interpretation)
				e.markLinguisticTrace(fmt.Sprintf("mode=cache result=ok job=%s signature=%q relations=%d entities=%d", cached.Job.ID, cached.Job.Signature, len(cached.Interpretation.Relations), len(cached.Interpretation.Entities)))
			}
		}
	}

	needed := interactionNeedsLinguisticDepth(r)
	switch {
	case !needed:
		e.markLinguisticTrace("mode=sync skipped=not_needed")
	case e.linguisticInterpreter == nil:
		e.markLinguisticTrace("mode=sync skipped=no_interpreter_configured")
	case !e.linguisticInterpreter.Available():
		e.markLinguisticTrace("mode=sync skipped=ollama_unavailable")
	default:
		pipeline := e.linguisticPipeline
		if pipeline == nil {
			// Compatibility for focused Engine fixtures that inject only the
			// Interpreter. The live NewEngine path always owns one canonical
			// Pipeline instance.
			pipeline = linguistics.NewPipeline(linguistics.NewRouter(), e.linguisticInterpreter, linguistics.NewValidator())
		}
		budget := linguisticSyncBudget()
		callCtx, cancel := context.WithTimeout(context.Background(), budget)
		snapshot := e.buildLinguisticSituationSnapshot(time.Now(), input)
		result, err := pipeline.ProcessRequestWithSituation(callCtx, input, r, snapshot)
		cancel()
		if err == nil && result.OpenMeaning != nil {
			om := *result.OpenMeaning
			r = mergeOpenMeaning(r, om)
			e.adoptOpenMeaning(input, om)
			turnReading = &om
			e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=ok comprehension=open purpose=%s topic=%q confidence=%.2f mentions=%d", budget, om.Purpose, om.Topic, om.Confidence, len(om.Mentions)))
		} else if err == nil && result.Interpretation != nil {
			li := *result.Interpretation
			r = mergeLinguisticInterpretation(r, li)
			e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=ok intent=%s confidence=%.2f relations=%d entities=%d", budget, li.Intent, li.Confidence, len(li.Relations), len(li.Entities)))
		} else if err != nil && errors.Is(err, context.DeadlineExceeded) {
			switch {
			case e.deferredLinguistic == nil:
				e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=false err=%q", budget, err.Error()))
			case !interaction.ShouldDeferLinguisticDepth(r):
				// The canonical semantic boundary already owns an executable
				// command. A deferred interpretation would be optional evidence
				// competing for the same local model while execution is waiting.
				// Do not create background model work that cannot alter the
				// already-established execution contract.
				e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=false reason=executable_command", budget))
			case e.openReadingAwaitEligible(r):
				// Fase 2 C (2026-10-18): las reglas no dejaron ningún acto
				// tipado. Si la lectura abierta alcanza un feedback (elogios
				// largos, frases sobre el futuro de Ahmelia), su acto debe
				// llegar al renderizador EN VIVO: espera acotada con
				// presupuesto propio (AHMELIA_OPEN_READING_AWAIT_MS, 30 s por
				// defecto). Sin esto la lectura llegaba tarde y la respuesta
				// viva era el no-entendido (defecto de la corrida 18/10).
				if jobID, queueErr := e.deferredLinguistic.Submit(r, ctx); queueErr != nil {
					e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=false await=true err=%q", budget, queueErr.Error()))
				} else {
					e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=true await=true job=%s", budget, jobID))
					if merged, om, awaited := e.awaitOpenReading(jobID, r); awaited {
						r = merged
						turnReading = om
					}
				}
			case pending != nil:
				// El render del propio turno usa el modelo primero; la lectura
				// — evidencia, nunca autoridad — arranca cuando el turno ya
				// habló (submitTurnEndDeferredReadings). Así el render y su
				// propia lectura dejan de competir por el mismo modelo local.
				*pending = append(*pending, pendingDeferredReading{req: r, ctxSnapshot: ctx})
				e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=true submit=after_turn", budget))
			default:
				if jobID, queueErr := e.deferredLinguistic.Submit(r, ctx); queueErr != nil {
					e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=false err=%q queue_err=%q", budget, err.Error(), queueErr.Error()))
				} else {
					e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=timeout deferred=true job=%s", budget, jobID))
				}
			}
		} else if err != nil {
			e.markLinguisticTrace(fmt.Sprintf("mode=sync budget=%s result=error err=%q", budget, err.Error()))
		}
	}
	// The linguistic adapter is allowed to enrich the turn, but it is not
	// guaranteed to emit every explicit grammatical relation. Preserve a
	// conservative semantic relation projection before the final interaction
	// evaluation so novel future/commitment language is not flattened to unknown.
	r = interaction.InferSemanticRelations(r)
	r.CommunicativeActs = interaction.ClassifyCommunicativeActs(r)
	if turnReading != nil {
		// Fase 2 C: la reclasificación textual no pisa el acto que vio la
		// lectura abierta; se re-aplica como acto derivado (jamás sobre
		// comandos, preguntas, correcciones o aprendizaje).
		r = openReadingActDerivation(r, *turnReading)
	}
	evaluation := interaction.EvaluateSemantic(r, ctx)
	if r.Confidence == 0 || evaluation.Confidence < r.Confidence {
		r.Confidence = evaluation.Confidence
	}
	if evaluation.Status == interaction.SemanticRejected {
		r.NeedsClarification = true
	}
	return r
}

func linguisticSyncBudget() time.Duration {
	const defaultBudget = 900 * time.Millisecond
	value := strings.TrimSpace(os.Getenv("AHMELIA_LINGUISTIC_SYNC_BUDGET_MS"))
	if value == "" {
		return defaultBudget
	}
	ms, err := strconv.Atoi(value)
	if err != nil || ms < 50 {
		return defaultBudget
	}
	if ms > 30000 {
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

// pendingDeferredReading is one semantic turn whose open reading must run
// AFTER the live turn has spoken: the same local model serves the visible
// response first, and the deferred enrichment starts when the turn is done.
type pendingDeferredReading struct {
	req         interaction.SemanticRequest
	ctxSnapshot interaction.Context
}

// openReadingAwaitBudget is the explicit in-turn budget for the bounded wait
// on the turn's own open reading (Fase 2 C). It is not a sync budget: the
// reading runs whole, and the budget only says how long the live turn is
// willing to wait for its typed act.
func openReadingAwaitBudget() time.Duration {
	const defaultBudget = 30 * time.Second
	value := strings.TrimSpace(os.Getenv("AHMELIA_OPEN_READING_AWAIT_MS"))
	if value == "" {
		return defaultBudget
	}
	ms, err := strconv.Atoi(value)
	if err != nil || ms < 1000 {
		return defaultBudget
	}
	if ms > 120000 {
		ms = 120000
	}
	return time.Duration(ms) * time.Millisecond
}

// openReadingAwaitEligible reports whether the turn's open reading can still
// supply the typed act the rules did not see: the rules classified nothing,
// the surface acts contain no executable/question/correction/learning act,
// and the turn is not the answer to Ahmelia's own open question (that answer
// belongs to the admission path and the reading would add no typed act).
func (e *Engine) openReadingAwaitEligible(r interaction.SemanticRequest) bool {
	// El carril de conversación (unknown/conversation) es donde viven los
	// turnos que las reglas no tiparon (elogios largos, frases sobre el
	// futuro de Ahmelia). Los actos de superficie siguen vetando la espera.
	if r.Kind != interaction.KindUnknown && r.Kind != interaction.KindConversation {
		return false
	}
	acts := interaction.CanonicalActs(r)
	if acts.Has(interaction.ActFeedback) {
		// The live rules already typed the act: the response renders its scene
		// without waiting for any reading.
		return false
	}
	if acts.Has(interaction.ActCommand) || acts.Has(interaction.ActControl) ||
		acts.Has(interaction.ActCorrection) || acts.Has(interaction.ActLearning) ||
		acts.Has(interaction.ActQuestion) {
		return false
	}
	if e != nil && e.conversationState != nil {
		// Durante la fase de entrada el turno todavía no se registró
		// (activeConversationTurnID es 0 entre turnos): el «turno actual» de la
		// admisión es el próximo ID secuencial. Sin esto, una respuesta a la
		// pregunta abierta parecería la de una pregunta vencida y la espera se
		// activaría sobre el carril de admisión.
		currentTurn := e.activeConversationTurnID
		if currentTurn == 0 {
			snap := e.conversationState.Snapshot()
			if len(snap.Turns) > 0 {
				currentTurn = snap.Turns[len(snap.Turns)-1].ID + 1
			} else {
				currentTurn = 1
			}
		}
		if _, open := e.conversationState.ActiveOpenQuestion(time.Now(), currentTurn); open {
			return false
		}
	}
	return true
}

// awaitOpenReading waits within openReadingAwaitBudget for the job this very
// turn submitted. When the reading returns the feedback act, it is merged so
// the renderer receives the typed act; any other outcome leaves the turn
// untouched, and the completed reading stays reusable through the signature
// cache. A failed reading is never an error of the turn.
func (e *Engine) awaitOpenReading(jobID string, r interaction.SemanticRequest) (interaction.SemanticRequest, *linguistics.OpenMeaning, bool) {
	if e == nil || e.deferredLinguistic == nil || strings.TrimSpace(jobID) == "" {
		return r, nil, false
	}
	budget := openReadingAwaitBudget()
	waitCtx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	result, ok := e.deferredLinguistic.AwaitJob(waitCtx, jobID)
	if !ok {
		e.markLinguisticTrace(fmt.Sprintf("mode=await budget=%s result=miss job=%s", budget, jobID))
		return r, nil, false
	}
	if result.Open == nil {
		e.markLinguisticTrace(fmt.Sprintf("mode=await budget=%s result=closed_contract job=%s", budget, jobID))
		return r, nil, false
	}
	om := *result.Open
	if om.Purpose != linguistics.OpenPurposeFeedback {
		e.markLinguisticTrace(fmt.Sprintf("mode=await budget=%s result=ok purpose=%s no_typed_act=true topic=%q job=%s", budget, om.Purpose, om.Topic, jobID))
		return r, nil, false
	}
	merged := mergeOpenMeaning(r, om)
	e.adoptOpenMeaning(result.Job.Request.Raw, om)
	e.markLinguisticTrace(fmt.Sprintf("mode=await budget=%s result=ok comprehension=open act=feedback purpose=%s topic=%q confidence=%.2f job=%s", budget, om.Purpose, om.Topic, om.Confidence, jobID))
	return merged, &om, true
}

// submitTurnEndDeferredReadings submits the deferred readings accumulated by
// the turn that just finished speaking. Order matters: the turn's own render
// uses the model first, and its open reading starts when the turn is done.
func (e *Engine) submitTurnEndDeferredReadings(t *processTurn) {
	if e == nil || t == nil || e.deferredLinguistic == nil || len(t.pendingDeferred) == 0 {
		return
	}
	for _, pending := range t.pendingDeferred {
		if strings.TrimSpace(pending.req.Raw) == "" {
			continue
		}
		jobID, err := e.deferredLinguistic.Submit(pending.req, pending.ctxSnapshot)
		if err != nil {
			e.markLinguisticTrace(fmt.Sprintf("mode=turn_end result=error err=%q", err.Error()))
			continue
		}
		e.markLinguisticTrace(fmt.Sprintf("mode=turn_end deferred=true job=%s", jobID))
	}
}

// resolveContextBoundary is the single Engine call-site for B96.36.
// Re-entering it after a semantic rewrite is allowed; the bridge itself remains
// unique and always resolves from the current ConversationState snapshot.
func (e *Engine) resolveContextBoundary(req interaction.SemanticRequest) (interaction.SemanticRequest, contextintelligence.Result) {
	if e == nil || e.contextIntelligence == nil || e.conversationState == nil {
		return req, contextintelligence.Result{}
	}
	resolution := e.contextIntelligence.Resolve(req, e.conversationState.Snapshot())
	return contextintelligence.ApplyToSemanticRequest(req, resolution), resolution
}

/* =======================
   PROCESS
======================= */

func autonomyLastInteraction(s *autonomy.AutonomousState) time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.LastInteraction()
}

func autonomyWaitingForUser(s *autonomy.AutonomousState) bool {
	if s == nil {
		return false
	}
	return s.WaitingForUser()
}

// Process preserves the historical API for callers that do not know their
// transport channel. Interactive adapters should use ProcessWithChannel so
// channel-sensitive continuity rules have explicit provenance.
func (e *Engine) Process(input string) {
	e.ProcessWithChannel(input, interaction.ChannelUnknown)
}

func (e *Engine) ProcessWithChannel(input string, channel interaction.InputChannel) {
	// Process owns only turn lifecycle and phase ordering. Terminal behavior is
	// delegated to the phase responsible for the corresponding authority.
	e.autonomyRuntimeMu.Lock()
	defer e.autonomyRuntimeMu.Unlock()
	if channel != "" && channel != interaction.ChannelUnknown {
		e.lastInputChannel = channel
	}

	tts.WaitUntilIdle()
	e.deliverBackgroundResults(input, channel)
	if flowTraceEnabled() {
		e.flowTrace = NewTurnFlowTrace(input)
		e.flowTrace.Mark("INPUT", input)
		defer func() {
			if e.flowTrace == nil {
				return
			}
			e.flowTrace.Mark("FINAL STATE", fmt.Sprintf("internal=%s autonomous=%s turn=%s", e.internalState.Current(), e.autonomousState.Execution(), e.conversationManager.TurnState()))
			e.flowTrace.PrintFinal()
			e.lastFlowTrace = e.flowTrace
			e.lastFlowTrace = e.flowTrace
			e.flowTrace = nil
		}()
	}

	fmt.Println("\n🎙️ ", input)
	e.pipeline.Enter(cognition.StageListen, input)
	e.internalState.Enter(cognition.StateListening, input)
	defer e.internalState.Enter(cognition.StateWaiting, "")
	e.shortMem.Commands = append(e.shortMem.Commands, input)

	t := &processTurn{input: input, channel: channel}
	// Fase 2 (2026-10-18): las lecturas diferidas de este turno se envían
	// cuando el turno ya habló — el render del propio turno usa el modelo
	// primero (openReadingAwaitEligible marca la única excepción, que espera
	// acotada en vivo). Este defer corre después de los cleanups del turno y
	// antes del cierre de la traza.
	defer e.submitTurnEndDeferredReadings(t)
	resumeCleanup := e.inputContextPhase(t)
	defer resumeCleanup()
	conversationCleanup := e.conversationStatePhase(t)
	defer conversationCleanup()

	if e.cancelBackgroundWorkPhase(t) {
		return
	}

	// Deliberación en sombra: se captura al cerrar el turno, después de que la
	// decisión del arnés existe, y corre en segundo plano. Registrar el defer
	// aquí (no antes) permite leer el turno y el estado final; al ser LIFO,
	// corre antes del cleanup conversacional y antes de las lecturas diferidas.
	defer e.scheduleShadowDeliberation(t)
	// Escena 1 (2026-11-04): si el parser determinista no consumió una frase de
	// descanso, el modelo puede interpretarla en segundo plano. El turno ya
	// respondió; el arnés decide si admite una restricción provisional.
	defer e.scheduleModelConsultation(t)

	// Explicit owner decisions (persistent mode, tastes and self-goals) are
	// deterministic acts of the harness. They run before a pending work
	// authorization can reinterpret the turn as a confirmation of something
	// else.
	if e.ownerGatePhase(t) {
		return
	}

	// A pending authorization is a continuation of an already-decided Work.
	// Consume it before evidence, history, Situation or Cognition can reinterpret
	// the confirmation and send it down an unrelated conversational route.
	if e.resumePendingAuthorizationIfConfirmed(t) {
		return
	}

	// INPUT → CONTEXT → EVIDENCE
	e.evidencePhase(t)

	// SITUATION → COGNITION → DIALOGUE
	e.situationPhase(t)
	if e.cognitionPhase(t) {
		return
	}
	e.dialoguePhase(t)

	// DECISION: workflow, metacognition, user model, initiative and the
	// canonical conversational response gate all live behind this boundary.
	if e.decisionPhase(t) {
		return
	}

	// EXECUTION: capability/production routing is attempted first. Only when
	// no earlier execution/response terminal path handled the turn do we enter
	// the legacy semantic-action compatibility tail.
	if e.preExecutionPhase(t) {
		return
	}

	autoCleanup, turnCleanup := e.executionLifecyclePhase()
	defer autoCleanup()
	defer turnCleanup()
	e.finishExecutionPhase(t)
}

/* =======================
   EXECUTE
======================= */

// authorityExecute is the SINGLE Engine-level execution entry point.
// Callers may propose or resolve a CapabilityRequest, but no specialist may
// cross the physical execution boundary directly. This method is the
// architectural bridge: CapabilityDecision -> AuthorityExecute -> Execute.
func (e *Engine) authorityExecute(decision interaction.CapabilityInteractionDecision, semanticReq ...interaction.SemanticRequest) production.AuthorityExecutionResult {
	return e.authorityExecuteWithOptions(decision, production.ExecutionOptions{Approval: autonomy.ApprovalNone, RecoveryAttempt: 1}, semanticReq...)
}

func (e *Engine) authorityExecuteWithOptions(decision interaction.CapabilityInteractionDecision, options production.ExecutionOptions, semanticReq ...interaction.SemanticRequest) production.AuthorityExecutionResult {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if e == nil || !decision.Executable || !decision.Request.HasOperation() {
		if e != nil && e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", "rejected: capability decision is not executable")
		}
		return production.AuthorityExecutionResult{}
	}
	// A post-execution repair is still a capability execution decision and must
	// cross this same authority. Cognition may establish that a materialized
	// result needs repair, but it cannot execute or select the provider.
	if decision.Request.Repair != nil {
		repair := decision.Request.Repair
		assessment := e.lastConversationalCognition.Repair
		if !assessment.Required || assessment.WorkID != repair.WorkID || assessment.Replacement != repair.Replacement {
			if e.flowTrace != nil {
				e.flowTrace.Mark("EXECUTION AUTHORITY", "repair rejected: cognition/repair contract mismatch")
			}
			return production.AuthorityExecutionResult{}
		}
		if e.productionRuntime == nil || e.workStore == nil {
			if e.flowTrace != nil {
				e.flowTrace.Mark("EXECUTION AUTHORITY", "repair rejected: production authority unavailable")
			}
			return production.AuthorityExecutionResult{}
		}
		work, ok := e.workStore.Get(repair.WorkID)
		if !ok || work.LastActionResult == nil || !work.LastActionResult.Success || !work.LastActionResult.Verified {
			if e.flowTrace != nil {
				e.flowTrace.Mark("EXECUTION AUTHORITY", "repair rejected: previous materialized result is not successfully verified")
			}
			return production.AuthorityExecutionResult{}
		}
		intent := production.RepairIntent{
			WorkID: repair.WorkID, Target: repair.Target, Replacement: repair.Replacement,
			PreviousResult: *work.LastActionResult, Confidence: repair.Confidence, Reason: repair.Reason,
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", fmt.Sprintf("repair accepted: work=%s producer=%s", intent.WorkID, intent.PreviousResult.Producer))
		}
		result := e.productionRuntime.RepairFromAuthority(ctx, intent)
		if len(semanticReq) > 0 {
			e.recordCapabilityExperience(decision, semanticReq[0], result, options.TaskID, "")
		}
		if len(semanticReq) > 0 {
			e.convergeActionOutcome(semanticReq[0], result)
		}
		if result.Success && result.Verified && e.conversationState != nil {
			e.conversationState.SetActiveObjective(repair.Replacement)
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("PRODUCTION REPAIR RESULT", fmt.Sprintf("success=%t verified=%t work=%s producer=%s", result.Success, result.Verified, result.WorkID, result.Producer))
		}
		if len(semanticReq) == 0 {
			return production.AuthorityResultFromAction(result)
		}
		_ = e.renderProductionActionResult(semanticReq[0], result)
		return production.AuthorityResultFromAction(result)
	}

	if !options.ExperimentalBypassCognition && !capabilityHandoffReady(e.lastConversationalCognition) && !options.ResumePendingAuthorization {
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", fmt.Sprintf("rejected by cognition: action=%s blocked=%t clarification=%t", e.lastConversationalCognition.Action.Primary, e.lastConversationalCognition.Action.Blocked, e.lastConversationalCognition.Action.NeedsClarification))
		}
		if len(semanticReq) > 0 && e.renderExecutionClarification(semanticReq[0]) {
			return production.AuthorityExecutionResult{Handled: true, OutcomeCode: "clarification_required"}
		}
		return production.AuthorityExecutionResult{}
	}

	// All physical execution now converges through the generic capability
	// resolver/provider boundary. There is intentionally no autonomous-only
	// executor and no Engine.execute fallback here. Autonomous requests carry
	// their same semantic action into capability discovery, so interactive and
	// autonomous execution share one authority and one provider registry.
	if e.productionRuntime == nil || e.capabilityResolver == nil {
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", "rejected: capability execution runtime unavailable")
		}
		return production.AuthorityExecutionResult{}
	}
	var request interaction.SemanticRequest
	if len(semanticReq) > 0 {
		request = semanticReq[0]
	}
	var route capability.Route
	if options.ResumePendingAuthorization {
		// Authorization continuation is not a new discovery decision. Rebuild
		// the exact persisted route and capability; never let current semantic
		// ranking substitute a different provider operation after confirmation.
		if e.capabilityRegistry == nil || options.ExpectedCapabilityID == "" || strings.TrimSpace(options.ExpectedOperation) == "" ||
			strings.TrimSpace(options.ExpectedProviderID) == "" || options.ExpectedRouteKind != capability.IntentProduction {
			if e.flowTrace != nil {
				e.flowTrace.Mark("EXECUTION AUTHORITY", "authorization resume rejected: persisted execution identity incomplete")
			}
			return production.AuthorityExecutionResult{}
		}
		candidate, exists := e.capabilityRegistry.Get(options.ExpectedCapabilityID)
		if !exists || candidate.Action != options.ExpectedOperation || !capabilityLaneMatchesExpected(candidate, options.ExpectedRouteKind) {
			if e.flowTrace != nil {
				e.flowTrace.Mark("EXECUTION AUTHORITY", "authorization resume rejected: persisted capability identity no longer matches registry")
			}
			return production.AuthorityExecutionResult{}
		}
		route = capability.Route{Kind: options.ExpectedRouteKind, Candidates: []capability.Capability{candidate}, Score: 100}
	} else {
		route = e.capabilityResolver.Resolve(decision, request)
	}
	if route.Kind != capability.IntentProduction && route.Kind != capability.IntentAction {
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", fmt.Sprintf("rejected: no executable capability route for operation=%s kind=%s", decision.Request.Operation, route.Kind))
		}
		return production.AuthorityExecutionResult{}
	}
	if len(route.Candidates) == 0 {
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", fmt.Sprintf("rejected: no registered provider for operation=%s", decision.Request.Operation))
		}
		return production.AuthorityExecutionResult{}
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("CAPABILITY DECISION", fmt.Sprintf("route=%s candidates=%d score=%d source=%s", route.Kind, len(route.Candidates), route.Score, decision.Request.Source))
		e.flowTrace.Mark("COGNITION→EXECUTION AUTHORITY", "provider execution begins")
	}
	if len(semanticReq) == 0 {
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", "rejected: semantic request missing at provider boundary")
		}
		return production.AuthorityExecutionResult{}
	}
	if options.ApprovalResolver == nil {
		options.ApprovalResolver = e.capabilityApprovalResolver
	}
	workID := strings.TrimSpace(options.WorkID)
	if route.Kind == capability.IntentProduction {
		resolvedWorkID, err := e.ensureWorkSessionForProductionContext(ctx, semanticReq[0], route, options.WorkID)
		if err != nil {
			if e.flowTrace != nil {
				e.flowTrace.Mark("WORK SESSION", "creation/recovery failed: "+err.Error())
			}
			return production.AuthorityExecutionResult{}
		}
		workID = resolvedWorkID
		options = workSessionExecutionOptions(options, workID)
		if e.flowTrace != nil && workID != "" {
			e.flowTrace.Mark("WORK SESSION", "persistent owner="+workID)
		}
	}
	capabilityID := ""
	if len(route.Candidates) > 0 {
		capabilityID = string(route.Candidates[0].ID)
	}
	if options.Async && route.Kind == capability.IntentProduction && len(semanticReq) > 0 && e.asyncProductionAllowed(route, options) {
		request := semanticReq[0]
		routeSnapshot := route
		jobOptions := options
		jobID := e.StartBackgroundJobForWork(ctx, "production:"+workID, workID, func(jobCtx context.Context) (production.ActionResult, error) {
			jobOptions.Context = jobCtx
			result := e.productionRuntime.ExecuteFromAuthority(jobCtx, request, routeSnapshot, jobOptions)
			if jobCtx.Err() == nil {
				e.autonomyRuntimeMu.Lock()
				if jobCtx.Err() == nil {
					e.recordCapabilityExperience(decision, request, result, jobOptions.TaskID, capabilityID)
					e.convergeActionOutcome(request, result)
				}
				e.autonomyRuntimeMu.Unlock()
			}
			return result, nil
		}, func(job BackgroundJob) {
			if job.Status == BackgroundJobCanceled {
				e.blockCanceledWork(workID)
				if !job.SuppressNotice {
					e.enqueueBackgroundResult(backgroundResult{
						JobID: job.ID, WorkID: workID, Err: errBackgroundJobCanceled, Canceled: true,
					})
				}
				return
			}
			if job.Status != BackgroundJobCompleted && job.Status != BackgroundJobFailed {
				return
			}
			e.enqueueBackgroundResult(backgroundResult{
				JobID: job.ID, WorkID: workID, Result: job.Result, Err: job.Err,
			})
		})
		if jobID == "" {
			return production.AuthorityExecutionResult{}
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("BACKGROUND JOB", "production started job="+jobID+" work="+workID)
		}
		return production.AuthorityExecutionResult{
			Handled: true, OutcomeStatus: production.OutcomeUnknown,
			WorkID: workID, BackgroundJobID: jobID,
			OutcomeCode: "working", Summary: "trabajo en curso",
		}
	}
	result := e.productionRuntime.ExecuteFromAuthority(ctx, semanticReq[0], route, options)
	e.recordCapabilityExperience(decision, semanticReq[0], result, options.TaskID, capabilityID)
	e.convergeActionOutcome(semanticReq[0], result)
	if e.flowTrace != nil {
		e.flowTrace.Mark("PROVIDER RESULT", fmt.Sprintf("route=%s success=%t verified=%t producer=%s recovery=%v authorization_required=%t", route.Kind, result.Success, result.Verified, result.Producer, result.Recovery, result.AuthorizationRequired))
	}
	if result.AuthorizationRequired {
		if e.flowTrace != nil {
			e.flowTrace.Mark("EXECUTION AUTHORITY", fmt.Sprintf("authorization required: capability=%s reason=%s", result.AuthorizationCapabilityID, result.AuthorizationReason))
		}
		// AuthorizationRequired is a terminal operational state, but it is not a
		// conversationally complete turn until the user is explicitly told what
		// is waiting. Previously the WorkSession was persisted correctly and the
		// authority returned Handled=true, yet no response was emitted. The next
		// spoken fragment then arrived with no authorization question to answer
		// and could be misrouted as ordinary conversation.
		if len(semanticReq) > 0 {
			capabilityID := strings.TrimSpace(string(result.AuthorizationCapabilityID))
			if capabilityID == "" {
				capabilityID = "la operación solicitada"
			}
			reason := strings.TrimSpace(result.AuthorizationReason)
			text := fmt.Sprintf("La tarea quedó preparada, pero necesito tu autorización explícita para ejecutar %s.", capabilityID)
			if reason != "" {
				text += " Motivo: " + reason + "."
			}
			text += " Puedes autorizarla explícitamente cuando quieras."
			e.renderCanonicalSystemResponse(semanticReq[0], "execution_authorization_required", text, interaction.MoveAsk, interaction.ResponseIntentClarify, true)
		}
		out := production.AuthorityResultFromAction(result)
		out.Handled = true
		return out
	}
	if result.Recovery != nil {
		handled := e.applyProductionRecovery(decision, semanticReq[0], result, *result.Recovery, options)
		// Recovery may execute a nested authority attempt. Return that final
		// persisted result to the original caller instead of the first failure
		// that merely triggered recovery.
		if handled && e.workStore != nil && strings.TrimSpace(result.WorkID) != "" {
			if work, ok := e.workStore.Get(result.WorkID); ok && work.LastActionResult != nil &&
				work.LastActionResult.AttemptNumber > result.AttemptNumber {
				out := production.AuthorityResultFromAction(*work.LastActionResult)
				out.Handled = true
				return out
			}
		}
		out := production.AuthorityResultFromAction(result)
		out.Handled = handled
		return out
	}
	if route.Kind == capability.IntentProduction {
		_ = e.renderProductionActionResult(semanticReq[0], result)
		out := production.AuthorityResultFromAction(result)
		out.Handled = true
		return out
	}
	_ = e.renderLocalActionResult(semanticReq[0], result)
	out := production.AuthorityResultFromAction(result)
	out.Handled = true
	return out

}

// asyncProductionAllowed keeps a suspended authorization request on the
// synchronous path. Async is only safe when the capability is automatic or
// the approval owner already granted it; otherwise the turn must ask.
func (e *Engine) asyncProductionAllowed(route capability.Route, options production.ExecutionOptions) bool {
	if len(route.Candidates) == 0 {
		return false
	}
	candidate := route.Candidates[0]
	if !strings.HasPrefix(string(candidate.ID), "programming.") {
		return false
	}
	if candidate.Permission == capability.PermissionAutomatic {
		return true
	}
	approval := options.Approval
	if options.ApprovalResolver != nil {
		approval = options.ApprovalResolver(candidate.ID)
	}
	return approval == autonomy.ApprovalGranted
}

// recordCapabilityExperience is the shared consequence-to-learning seam for
// interactive and autonomous work. Operational completion is retained, while
// competence confidence changes only when the environment was observed and
// the effect was verified.
func (e *Engine) recordCapabilityExperience(
	decision interaction.CapabilityInteractionDecision,
	request interaction.SemanticRequest,
	result production.ActionResult,
	taskID string,
	capabilityID string,
) {
	if e == nil || e.eventStore == nil || !result.Executed {
		return
	}
	action := strings.TrimSpace(decision.Request.Operation)
	if action == "" {
		action = strings.TrimSpace(request.Action.Name)
	}
	if action == "" {
		return
	}
	objective := request.Objective.Text
	instrumentID := result.Producer
	skillID := ""
	if e.workStore != nil && result.WorkID != "" {
		if work, ok := e.workStore.Get(result.WorkID); ok {
			if strings.TrimSpace(work.Objective.Text) != "" {
				objective = work.Objective.Text
			}
			for _, attempt := range work.Attempts {
				if attempt.Number != result.AttemptNumber {
					continue
				}
				if strings.TrimSpace(attempt.CapabilityID) != "" {
					capabilityID = attempt.CapabilityID
				}
				if strings.TrimSpace(attempt.SkillID) != "" {
					skillID = attempt.SkillID
				}
				if strings.TrimSpace(attempt.Operation) != "" {
					action = attempt.Operation
				}
				if strings.TrimSpace(attempt.Provider) != "" {
					instrumentID = attempt.Provider
				}
				break
			}
		}
	}
	// Older persisted attempts predate SkillID. Resolve their stable competence
	// identity from the current registry without rewriting the original record.
	if skillID == "" && e.capabilityRegistry != nil && strings.TrimSpace(capabilityID) != "" {
		if registered, ok := e.capabilityRegistry.Get(capability.ID(capabilityID)); ok {
			skillID = string(registered.SkillID)
		}
	}
	status := memoryintelligence.EvidenceUnknown
	switch {
	case result.OutcomeStatus == production.OutcomeUnknown:
		status = memoryintelligence.EvidenceUnknown
	case result.OutcomeStatus == production.OutcomeSucceeded && result.Success && result.Observed && result.Verified:
		status = memoryintelligence.EvidenceConfirmed
	case result.OutcomeStatus == production.OutcomeFailed && !result.Success && result.Observed:
		status = memoryintelligence.EvidenceContradicted
	case result.OutcomeStatus == "" && result.Observed && result.Verified && result.Success:
		status = memoryintelligence.EvidenceConfirmed
	case result.OutcomeStatus == "" && result.Observed && result.Verified && !result.Success:
		status = memoryintelligence.EvidenceContradicted
	case !result.Observed:
		status = memoryintelligence.EvidenceInsufficient
	}
	reason := strings.TrimSpace(result.FailureReason)
	if reason == "" {
		reason = strings.TrimSpace(result.VerificationReason)
	}
	if reason == "" {
		reason = strings.TrimSpace(result.Summary)
	}
	identity := ""
	if result.WorkID != "" && result.AttemptNumber > 0 {
		identity = fmt.Sprintf("work:%s:attempt:%d", result.WorkID, result.AttemptNumber)
	} else if taskID != "" {
		identity = "task:" + taskID
	}
	facts := make([]memoryintelligence.ObservedEffectFact, 0, len(result.ObservationFacts))
	for _, fact := range result.ObservationFacts {
		facts = append(facts, memoryintelligence.ObservedEffectFact{
			Subject: fact.Subject, Predicate: fact.Predicate, Expected: fact.Expected, Observed: fact.Observed,
		})
	}
	sessionID := result.WorkID
	if sessionID == "" {
		sessionID = taskID
	}
	if sessionID == "" {
		sessionID = "authority"
	}
	e.eventStore.AppendOnce(action, request.Action.Target, memoryintelligence.Outcome{
		Success:        result.Success,
		EvidenceStatus: status,
		Category:       strings.TrimSpace(result.OutcomeCode),
		Reason:         reason,
	}, sessionID, identity, memoryintelligence.ExperienceMetadata{
		Objective: objective, SkillID: skillID, CapabilityID: capabilityID,
		GenerationSource:  result.GenerationSource,
		TargetEnvironment: result.Project.Path, VerificationProfile: result.VerificationProfile,
		InstrumentID: instrumentID, WorkID: result.WorkID, AttemptNumber: result.AttemptNumber,
		ObservationSource: result.ObservationSource, VerificationSource: result.VerificationSource,
		ObservationDetails: result.ObservationDetails, ObservedFacts: facts,
	})
	if e.learningState != nil {
		e.learningState.UpdateFromReflection(e.eventStore.Snapshot())
		_ = e.learningState.Save(memoryintelligence.LearningStatePath)
	}
}

// capabilityLaneMatchesExpected checks the persisted route against the
// registered capability without rediscovering an operation from natural
// language. It is used only for authorization continuation.
func capabilityLaneMatchesExpected(c capability.Capability, kind capability.IntentKind) bool {
	switch kind {
	case capability.IntentProduction:
		return c.Lane == capability.LaneProduction || c.Lane == capability.LaneUnspecified
	case capability.IntentAction:
		return c.Lane == capability.LaneAction || c.Lane == capability.LaneUnspecified
	default:
		return false
	}
}

// capabilityHandoffReady is the final semantic coherence contract between
// Conversational Cognition and the provider-neutral capability boundary.
// Cognition never authorizes or selects a capability; it only establishes that
// the current turn is an intentional, non-blocked execution posture. The
// capability boundary remains responsible for resolving and executing the
// resulting operation. A mismatch must fail closed rather than allowing
// Runtime to execute while Cognition simultaneously says "clarify".
func capabilityHandoffReady(c cognition.ConversationalCognitionE2E) bool {
	return c.Valid &&
		c.Action.Primary == cognition.ActionRequestExecution &&
		c.Action.ExecutionIntent &&
		!c.Action.Blocked &&
		!c.Action.NeedsClarification &&
		!c.Uncertainty.BlocksAction &&
		!c.Conflict.BlocksAction
}

// isExplicitFilesystemAction marca las acciones cuyo resultado debe ser su
// efecto físico real (crear/renombrar/mover/buscar en disco). El atajo de
// "abrir carpeta especial" no debe dispararse para ellas: responder "abrí la
// carpeta" no es el efecto pedido y reportaría un éxito falso (T0 familia a).
func isExplicitFilesystemAction(action string) bool {
	switch action {
	case "crear_carpeta", "renombrar_archivo", "mover_archivo", "buscar_archivo":
		return true
	}
	return false
}

func (e *Engine) execute(action, target string, params map[string]string) (result production.ProviderResult) {
	// Registrar en contexto y reportar éxito a la memoria
	rawInput := ""
	if params != nil {
		rawInput = params["raw"]
	}
	outcomeCategory := ""
	outcomeReason := ""
	start := time.Now()
	result = production.ProviderResult{Producer: "core.local_action", OutcomeCode: "executing"}
	success := true

	// Bloque 8 — Confidence Tracking en vivo: ANTES de intentar ejecutar,
	// Learning (Bloque 7) decide qué estrategia usar según qué tanto le ha
	// funcionado esta Action en el EventLog acumulado (ActionConfidence).
	// Si viene fallando, escala de Primary → Secondary → Fallback en vez
	// de repetir ciegamente el mismo enfoque. Ver
	// memory_intelligence/learning.go (AdaptStrategy/DefaultStrategies).
	// Para Actions sin StrategySet definido, AdaptStrategy retorna
	// "desconocida" y el caso correspondiente simplemente la ignora.
	e.pipeline.Enter(cognition.StageStrategy, "AdaptStrategy("+action+")")
	e.internalState.Enter(cognition.StateThinking, "eligiendo estrategia")
	strategyUsed := e.learningState.AdaptStrategy(action)
	e.pipeline.Enter(cognition.StageExecute, action+" "+target)
	e.internalState.Enter(cognition.StateExecuting, action+" "+target)
	// Fase 6 (B72) — mismo punto donde internalState ya marca
	// StateExecuting: acá es donde el ciclo de vida de la TAREA
	// autónoma (distinto de "qué hace Ahmelia en este turno", ver la
	// nota de cabecera de autonomy/state.go) entra de verdad en
	// ExecExecuting. Best-effort: si por algún motivo no estaba en
	// Thinking (ej. execute() llamado sin pasar por el arranque normal
	// de Process()), no se aborta la ejecución real por un desajuste
	// de bookkeeping — se ignora el error y se sigue.
	_ = e.autonomousState.TransitionTo(autonomy.ExecExecuting)
	defer func() {
		// Sin bloque real de Verification (66) ni Failure Recovery (68)
		// conectado todavía — Executing solo puede llegar a Completed
		// pasando por Verifying en la tabla de transiciones (ver
		// autonomy/state.go), así que se atraviesa ese paso sin
		// detenerse ahí: es honesto documentarlo como "no hay
		// verificación real todavía" en vez de inventar una que no
		// existe, y como "no hay Recovering real todavía" — success/
		// error acá ya se reportan por otros canales (ReportSuccess/
		// ReportFailure, más abajo en este mismo defer).
		if e.autonomousState.Execution() == autonomy.ExecExecuting {
			_ = e.autonomousState.TransitionTo(autonomy.ExecVerifying)
			_ = e.autonomousState.TransitionTo(autonomy.ExecCompleted)
		}
	}()

	defer func() {
		result.Success = success
		if success {
			result.OutcomeCode = "completed"
			if result.VerificationSource == "" {
				result.VerificationSource = "core.local_action"
			}
		} else if result.OutcomeCode == "executing" || result.OutcomeCode == "" {
			result.OutcomeCode = "execution_failed"
		}
		if !success && result.FailureReason == "" {
			result.FailureReason = outcomeReason
		}
		result.Observed = false
		result.Verified = false
		if result.VerificationReason == "" {
			result.VerificationReason = "el proveedor local aún no tiene un verificador de estado del entorno conectado"
		}
		e.context.Add(rawInput, action, target)
		if rawInput != "" && success {
			// Antes: e.learner.ReportSuccess(rawInput) solo subía la
			// confianza de una entrada que YA existiera en
			// memory/long.json — si el comando nunca había pasado por
			// el flujo de "enséñame algo", esto no hacía nada. Por eso
			// memory_intelligence/ (RecallTopic) no encontraba nada:
			// los comandos normales nunca se guardaban. RecordUsage
			// crea la entrada si hace falta, para cada comando exitoso.
			e.longMem.RecordUsage(rawInput, action, target, params)
			e.learner.ReportSuccess(rawInput)

			// Bloque 2, Nivel 2 — Relaciones: Relate()/GetByTag() ya
			// existían en learning/pattern.go, construidos sobre
			// memory.LongMemory, pero nadie los llamaba (huérfanos,
			// mismo patrón que Timeline antes de Nivel 1). El propio
			// comentario de Relate() decía "el engine llama esto cuando
			// detecta que dos inputs producen la misma acción" — esto
			// es exactamente ese punto: si otra frase distinta ya
			// registrada apunta al mismo Action+Target, se conectan.
			for _, other := range e.longMem.RecallBySameCommand(action, target) {
				if shared.Normalize(other.Input) != shared.Normalize(rawInput) {
					e.learner.Relate(rawInput, other.Input)
				}
			}
		}
		// Memoria episódica: acumula la sesión completa (sin el límite
		// de 10 que tiene e.context), se resume una sola vez en
		// Shutdown(). Se registra tanto éxito como fallo — un episodio
		// también incluye lo que salió mal.
		e.pipeline.Enter(cognition.StageExperience, fmt.Sprintf("episode.Record(%s, %s, ok=%v)", action, target, success))
		e.internalState.Enter(cognition.StateLearning, "guardando experiencia")
		e.episode.Record(action, target, success)
		// Fase 1 (Observación): a diferencia de Episode (arriba), esto no
		// se resume ni se pierde al cerrar Ahmelia — es la base cruda de
		// la que Fase 2 (secuencias) y Fase 3 (patrones) recalculan.
		// e.episode.Start.Unix() como SessionID: mismo identificador que ya
		// usa episode.go para la key del resumen ("episodio_<start>"), así
		// que todos los eventos de esta sesión quedan agrupados igual que
		// el resumen que los describe.
		if action != "" && action != "desconocido" {
			e.eventStore.Append(action, target, memoryintelligence.Outcome{
				Success:        success,
				EvidenceStatus: memoryintelligence.EvidenceInsufficient,
				Category:       outcomeCategory,
				Reason:         outcomeReason,
			}, fmt.Sprintf("%d", e.episode.Start.Unix()))

			// Bloque 8 — cierra el ciclo: recalcula ActionConfidence con
			// este evento ya incluido (así la próxima llamada a
			// AdaptStrategy, aunque sea en el mismo minuto, ya lo ve) y
			// registra si la estrategia elegida funcionó. Se persiste de
			// inmediato, mismo criterio que EventStore.Append — si Ahmelia
			// se cierra abrupto, no se pierde la adaptación.
			// Bloque 93.3 — antes esto tocaba e.eventLog/e.learningState
			// directo sin ningún lock compartido con el ciclo autónomo
			// (autonomy.LearningMemoryAdapter, que sí tenía su propio mutex
			// pero no protegía contra esta goroutine). Ahora ambos pasan
			// por el mismo EventStore/LearningStore.
			e.pipeline.Enter(cognition.StageLearn, "UpdateFromReflection + NoteOutcome("+action+")")
			e.internalState.Enter(cognition.StateLearning, "actualizando aprendizaje")
			e.learningState.UpdateFromReflection(e.eventStore.Snapshot())
			// El resultado operativo no es evidencia del efecto ambiental;
			// esta ruta no actualiza adaptación ni eficacia de estrategia.
			_ = e.learningState.Save(memoryintelligence.LearningStatePath)

			// Bloque 10 — cualquier acción real puede haber cambiado el
			// mundo (una app que se abrió/cerró, un archivo movido, etc.)
			// o las confianzas de Learning recién actualizadas arriba.
			// Invalidate() es barato (solo pone un flag) — el recálculo
			// real recién pasa en el próximo Get().
			e.contextCache.Invalidate()
		}
		// Registrar en ActionHistory y en Reasoner (Capa 4)
		duration := int(time.Since(start).Milliseconds())
		e.history.Record(rawInput, action, target, "", success, duration)
		e.reasoner.RecordDecision(rawInput, action, target, 0.8, nil)
		_ = e.reasoner.Evaluate(rawInput, action, success)
	}()

	if action == "mover_archivo" {
		source := strings.TrimSpace(params["origen"])
		if source == "" {
			source = strings.TrimSpace(params["source"])
		}
		if source == "" && e.sess.HasFile() {
			source = e.sess.LastSelectedFile
		}
		if source == "" {
			success = false
			result.Summary = "No tengo ningún archivo seleccionado. Busca uno primero"
			return
		}
		destination := strings.TrimSpace(params["destino"])
		if destination == "" {
			destination = strings.TrimSpace(params["destination"])
		}
		if destination == "" {
			destination = target
		}
		destPath := destination
		if !filepath.IsAbs(destPath) {
			destKey := e.extractFolderTarget(destPath)
			if destKey == "" {
				success = false
				result.Summary = fmt.Sprintf("No conozco esa carpeta: %s", destination)
				return
			}
			destPath = e.folderDst[destKey]
		}
		params["origen"] = source
		params["destino"] = destPath
		if err := files.MoveFile(source, destPath); err != nil {
			success = false
			result.Summary = fmt.Sprintf("No pude mover el archivo: %v", err)
			return
		}
		result.Summary = personality.FileMoved(filepath.Base(source))
		e.sess.ClearFile()
		return
	}

	// El atajo "abre tu carpeta especial" (escritorio/descargas/…) es una
	// conveniencia conversacional: no puede interceptar acciones cuyo éxito
	// ES su efecto físico en disco. Antes, cualquier target con forma de
	// ruta que contuviera "desktop", "escritorio", "videos", etc. (todas
	// las rutas bajo el Desktop del usuario) retornaba acá "abrí la
	// carpeta" con éxito y la acción real nunca se ejecutaba (T0 familia a,
	// 2026-10-16).
	if !e.sess.WaitingForFileAction() && !e.sess.HasFile() && !isExplicitFilesystemAction(action) {
		if folderName, opened := system.TryOpenFolder(target); opened {
			result.Summary = personality.OpeningFolder(folderName)
			return
		}
	}

	switch action {

	case "abrir_archivo":
		if !e.sess.HasFile() {
			success = false
			result.Summary = "No tengo ningún archivo seleccionado. Busca uno primero"
			return
		}
		name := e.sess.LastSelectedFileName()
		if err := files.OpenFile(e.sess.LastSelectedFile); err != nil {
			success = false
			result.Summary = fmt.Sprintf("No pude abrir el archivo: %v", err)
			return
		}
		result.Summary = personality.FileActionOpened(name)
		e.sess.DoneWithAction()

	case "renombrar_archivo":
		source := strings.TrimSpace(params["origen"])
		if source == "" {
			source = strings.TrimSpace(params["source"])
		}
		if source == "" && e.sess.HasFile() {
			source = e.sess.LastSelectedFile
		}
		if source == "" {
			success = false
			result.Summary = "No tengo ningún archivo seleccionado. Busca uno primero"
			return
		}
		newName := strings.TrimSpace(params["new_name"])
		if newName == "" {
			newName = strings.TrimSpace(params["nombre_nuevo"])
		}
		if newName == "" {
			destination := strings.TrimSpace(params["destino"])
			if destination == "" {
				destination = strings.TrimSpace(params["destination"])
			}
			if destination != "" {
				newName = filepath.Base(destination)
			}
		}
		if newName == "" {
			success = false
			result.Summary = "No entendí el nombre. Di solo el nombre nuevo"
			return
		}
		oldName := filepath.Base(source)
		params["ruta"] = source
		params["nombre_nuevo"] = newName
		if err := files.RenameFile(source, newName); err != nil {
			success = false
			result.Summary = fmt.Sprintf("No pude renombrar: %v", err)
			return
		}
		result.Summary = personality.FileRenamed(oldName, newName)
		e.sess.ClearFile()

	case "saludo":
		// No existía ninguna acción de saludo antes — "hola cómo estás"
		// caía directo a incertidumbre. personality.Greeting() varía la
		// respuesta cada vez (pedido explícito), no siempre la misma.
		result.Summary = personality.Greeting()
		return

	case "abrir_programa":
		// Referencia implícita (Nivel 1, ver reference/resolver.go):
		// "ábrelo" queda, después de quitarle el verbo, con el pronombre
		// suelto ("lo") como target — no es el nombre de ningún
		// programa. Se resuelve contra la última app que WorldState
		// recuerda como referenciada, en vez de intentar buscar un
		// programa literalmente llamado "lo".
		if ref := e.refResolver.Resolve(target); ref.WasReference {
			if !ref.Found {
				success = false
				result.Summary = "No sé qué abrir, no tengo ninguna aplicación reciente"
				return
			}
			if ref.App.Status == environment.StatusAbierta {
				e.world.RegisterFocused(ref.App.Name)
				result.Summary = personality.AlreadyOpen(ref.App.Name)
				return
			}
			// Reabrir con lo que originalmente se le pidió abrir si lo
			// tenemos (Target), o si no con el nombre normalizado.
			if ref.App.Target != "" {
				target = ref.App.Target
			} else {
				target = ref.App.Name
			}
		}
		// 1. Edge especial
		if strings.Contains(target, "edge") {
			result.Summary = personality.OpeningEdge()
			_ = exec.Command("cmd", "/C", "start", "microsoft-edge:").Start()
			return
		}
		// 2. Detectar URL por target directo (youtube.com, github.com, etc.)
		if url, ok := resolveURLTarget(target); ok {
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}
			result.Summary = personality.OpeningURL(url)
			_ = exec.Command("cmd", "/C", "start", "", url).Start()
			return
		}
		// 3. Compatibilidad: params["url"] generado por classifier
		if url := params["url"]; url != "" {
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}
			result.Summary = personality.OpeningURL(url)
			_ = exec.Command("cmd", "/C", "start", "", url).Start()
			return
		}
		// 4. Buscar en índice de programas
		ruta := findProgram(target, e.idx)
		if ruta == "" {
			success = false
			outcomeCategory = memoryintelligence.CategoryProgramNotFound
			outcomeReason = "programa no encontrado: " + target
			e.learner.ReportFailure(rawInput, action, "programa no encontrado: "+target)
			result.Summary = personality.ProgramNotFound(target)
			return
		}
		exeName := strings.ToLower(filepath.Base(ruta))
		cleanName := strings.TrimSuffix(filepath.Base(ruta), ".exe")
		if isProcessRunning(exeName) {
			e.world.RegisterFocused(cleanName)
			result.Summary = personality.AlreadyOpen(cleanName)
			return
		}
		if k, ok := e.longMem.Recall(target); ok {
			result.Summary = personality.OpeningProgramWithUses(cleanName, k.Uses)
		} else {
			result.Summary = personality.OpeningProgram(cleanName)
		}
		e.world.RegisterOpened(cleanName, target)
		_ = exec.Command("cmd", "/C", "start", "", ruta).Start()

	case "buscar_archivo":
		result.Summary = personality.Searching(target)
		results, err := files.SearchFileHuman(target)
		if err != nil {
			success = false
			result.Summary = personality.FileNotFound(target)
			return
		}
		sessionFiles := make([]memory.SessionFile, len(results))
		for i, r := range results {
			sessionFiles[i] = memory.SessionFile{
				Path:         r.Path,
				Score:        r.Score,
				LastModified: r.LastModified,
			}
		}
		if len(results) == 1 {
			e.sess.FocusFile(results[0].Path)
			result.Summary = personality.AskFileAction(filepath.Base(results[0].Path))
		} else {
			e.sess.SetFileResults(sessionFiles)
			result.Summary = e.showFileList(sessionFiles)
			result.InputRequired = true
			result.OutcomeCode = "input_required"
		}

	case "crear_carpeta":
		// El clasificador ya reconocía "crea carpeta"/"nueva carpeta"
		// (shared/constants.go) y files.CreateFolder ya existía
		// (files/manager.go) — pero nada los conectaba, así que este
		// comando siempre caía al default (UnknownAction). De paso, esta
		// es la Action de ejemplo que usa todo memory_intelligence/learning.go,
		// así que es el caso real para probar Bloque 8: strategyUsed viene
		// de e.learningState.AdaptStrategy(action), calculado arriba,
		// ANTES de este switch.
		if target == "" {
			success = false
			outcomeCategory = memoryintelligence.CategoryFileNotFound
			outcomeReason = "sin nombre de carpeta"
			result.Summary = "¿Cómo se llama la carpeta?"
			return
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			success = false
			outcomeCategory = memoryintelligence.CategoryFileNotFound
			outcomeReason = "sin directorio home: " + err.Error()
			result.Summary = "No pude ubicar tu carpeta de usuario."
			return
		}
		folderPath := strings.TrimSpace(target)
		if !filepath.IsAbs(folderPath) {
			folderPath = filepath.Join(homeDir, "Desktop", folderPath)
		}
		params["ruta"] = folderPath

		switch strategyUsed {
		case "con powershell":
			// Secundaria (ActionConfidence < 40%): si el método directo
			// viene fallando, se prueba con PowerShell — motor distinto,
			// mismo objetivo. New-Item -Force no falla si ya existe, a
			// diferencia de os.MkdirAll+Stat de más abajo.
			err = exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf("New-Item -ItemType Directory -Force -Path '%s'", folderPath)).Run()
		case "validar permisos":
			// Fallback (ActionConfidence < 20%): antes de reintentar
			// cualquier método, valida que la carpeta padre exista y sea
			// escribible — si el problema real es permisos/ruta inválida,
			// mejor decirlo claro que reintentar el mismo fallo.
			parent := filepath.Dir(folderPath)
			if info, statErr := os.Stat(parent); statErr != nil || !info.IsDir() {
				success = false
				outcomeCategory = memoryintelligence.CategoryFileNotFound
				outcomeReason = "carpeta padre no existe: " + parent
				result.Summary = fmt.Sprintf("No puedo crearla: no existe %s", parent)
				return
			}
			err = files.CreateFolder(folderPath)
		default:
			// Primary ("directo") — o Action sin historial todavía.
			err = files.CreateFolder(folderPath)
		}

		if err != nil {
			success = false
			outcomeCategory = memoryintelligence.CategoryFileNotFound
			outcomeReason = fmt.Sprintf("error creando carpeta (%s): %v", strategyUsed, err)
			result.Summary = fmt.Sprintf("No pude crear la carpeta: %v", err)
			return
		}
		result.Summary = personality.FolderCreated(target)

	// ── CONTROL DEL SISTEMA (CORREGIDO) ──────────────────────────────
	case "volumen_subir":
		rawInput := params["raw"]
		if rawInput == "" {
			rawInput = target
		}
		if percent, ok := system.ExtractVolumePercent(rawInput); ok {
			if err := system.SetVolume(percent); err != nil {
				result.Summary = "No pude ajustar el volumen"
				return
			}
			result.Summary = fmt.Sprintf("Volumen al %d por ciento", percent)
		} else {
			if err := system.VolumeUp(); err != nil {
				success = false
				outcomeCategory = memoryintelligence.CategoryVolumeError
				outcomeReason = "error subir volumen: " + err.Error()
				e.learner.ReportFailure(rawInput, action, "error subir volumen: "+err.Error())
				result.Summary = "No pude subir el volumen"
				return
			}
			result.Summary = "Volumen subido"
		}

	case "volumen_bajar":
		if err := system.VolumeDown(); err != nil {
			success = false
			outcomeCategory = memoryintelligence.CategoryVolumeError
			outcomeReason = "error bajar volumen: " + err.Error()
			e.learner.ReportFailure(rawInput, action, "error bajar volumen: "+err.Error())
			result.Summary = "No pude bajar el volumen"
			return
		}
		result.Summary = "Volumen bajado"

	case "volumen_silencio":
		if err := system.Mute(); err != nil {
			result.Summary = "No pude silenciar"
			return
		}
		result.Summary = "Sin sonido"

	case "volumen_max":
		if err := system.VolumeMax(); err != nil {
			result.Summary = "No pude maximizar el volumen"
			return
		}
		result.Summary = "Volumen al máximo"

	case "volumen_set":
		rawInput := params["raw"]
		if rawInput == "" {
			rawInput = target
		}
		percent, ok := system.ExtractVolumePercent(rawInput)
		if !ok {
			// No es un error de ejecución, pero tampoco se completó nada
			// (no se cambió el volumen). success=false evita que
			// RecordUsage/Relate (execute(), bloque defer) guarden esto
			// como si fuera un comando exitoso — antes se colaba porque
			// success ya venía en true por defecto y este return no lo
			// tocaba.
			success = false
			outcomeCategory = memoryintelligence.CategoryClarificationNeeded
			outcomeReason = "pidió porcentaje de volumen sin especificarlo"
			e.sess.SetWaitingVolume()
			result.Summary = "¿A cuánto lo pongo?"
			return
		}
		if err := system.SetVolume(percent); err != nil {
			result.Summary = "No pude ajustar el volumen"
			return
		}
		result.Summary = fmt.Sprintf("Volumen al %d por ciento", percent)

	case "minimizar":
		if err := system.MinimizeAll(); err != nil {
			result.Summary = "No pude minimizar"
			return
		}
		result.Summary = "Todo minimizado"

	case "cerrar_ventana":
		// Referencia implícita ("ciérralo") — mismo resolvedor que usa
		// cambiar_ventana, para que "ciérralo" se refiera a la última
		// app referenciada en vez de caer directo al genérico de abajo.
		if ref := e.refResolver.Resolve(target); ref.WasReference {
			if ref.Found {
				target = ref.App.Name
			} else {
				target = ""
			}
		}

		if target != "" {
			// Cerrar la ventana NOMBRADA — no la que esté activa en
			// este instante. Antes esto no existía: cualquier "cierra
			// X" ignoraba X y mandaba Alt+F4 a la ventana activa, que
			// en un asistente de voz suele ser la propia consola de
			// Ahmelia (el usuario habla con la terminal al frente) —
			// resultado: Ahmelia se cerraba a sí misma.
			if err := system.CloseWindow(target); err != nil {
				e.learner.ReportFailure(rawInput, action, "ventana no encontrada al cerrar: "+target)
				fmt.Println("⚠️  CloseWindow falló:", err)
				result.Summary = "No encontré " + target + " para cerrar"
				return
			}
			e.world.RegisterClosed(target)
			result.Summary = "Cerrando " + target
			return
		}

		// Sin nombre y sin referencia previa resolvible: no hay forma
		// segura de saber qué ventana quiere cerrar el usuario. A
		// propósito YA NO se cae a "cerrar la ventana activa" — ese
		// fallback fue justo el que cerró la consola de Ahmelia por
		// error. Mejor preguntar que arriesgar cerrar algo equivocado.
		result.Summary = "¿Cerrar qué? Dime el nombre de la aplicación"

	case "cambiar_ventana":
		// Referencia implícita (Nivel 1): "vuelve a eso" no nombra la
		// app, se refiere a la última que WorldState recuerda como
		// referenciada. Sin esto, se intentaría cambiar a una ventana
		// literalmente llamada "eso".
		if ref := e.refResolver.Resolve(target); ref.WasReference {
			if !ref.Found {
				result.Summary = "No sé a qué te refieres, no tengo ninguna aplicación reciente"
				return
			}
			target = ref.App.Name
		}
		if target != "" {
			// Cambiar a ventana específica (ej: "cambia a chrome")
			if err := system.SwitchToWindow(target); err != nil {
				success = false
				outcomeCategory = memoryintelligence.CategoryWindowNotFound
				outcomeReason = "ventana no encontrada: " + target
				e.learner.ReportFailure(rawInput, action, "ventana no encontrada: "+target)
				fmt.Println("⚠️  SwitchToWindow falló:", err)
				result.Summary = "No encontré " + target
				return
			}
			e.world.RegisterFocused(target)
			result.Summary = "Cambiando a " + target
		} else {
			// Cambiar a siguiente ventana (Alt+Tab)
			if err := system.SwitchWindow(); err != nil {
				result.Summary = "No pude cambiar de ventana"
				return
			}
			result.Summary = "Cambiando ventana"
		}

	case "reproducir_media":
		vlcPath := findProgram("vlc", e.idx)
		if vlcPath == "" {
			result.Summary = personality.VLCNotInstalled()
			return
		}
		// Antes esto pasaba `target` (el texto crudo dicho, ej. "bohemian
		// rhapsody") directo como argumento a VLC, asumiendo que ya era
		// una ruta válida. Nunca lo era — VLC/Windows terminaba tratando
		// ese texto como un nombre de archivo a crear en vez de buscar la
		// canción real en la biblioteca de música. Ahora se busca de
		// verdad con SearchFileHuman (misma función que usa
		// buscar_archivo) en Música/Music/Descargas/etc., y solo se abre
		// en VLC si el archivo existe.
		if target == "" {
			result.Summary = "¿Qué quieres que reproduzca?"
			return
		}
		results, err := files.SearchFileHuman(target)
		if err != nil || len(results) == 0 {
			success = false
			outcomeCategory = memoryintelligence.CategoryFileNotFound
			outcomeReason = "archivo no encontrado: " + target
			result.Summary = personality.FileNotFound(target)
			return
		}
		mediaPath := results[0].Path
		e.world.RegisterOpened("vlc", mediaPath)
		args := []string{mediaPath}
		if params["type"] == "playlist" {
			result.Summary = personality.PlayingPlaylist()
			args = append([]string{"--playlist-autostart"}, args...)
		} else {
			result.Summary = personality.PlayingMediaNamed(filepath.Base(mediaPath))
		}
		if params["shuffle"] == "true" {
			args = append([]string{"--random"}, args...)
		}
		_ = exec.Command(vlcPath, args...).Start()

	default:
		success = false
		result.Summary = personality.UnknownAction()
	}
	return result
}

/* =======================
   HANDLE FILE ACTION
======================= */

func (e *Engine) handleFileAction(req interaction.SemanticRequest) {
	input := req.Raw
	if req.CommunicativeActs.Has(interaction.ActControl) || req.Action.Name == "salir" {
		e.sess.Reset()
		e.renderCanonicalSystemResponse(req, "file_action_control", personality.FileActionCancelled(), interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
		return
	}

	action := workflowFileAction(req.Action.Name)

	switch action {
	case "abrir":
		decision := interaction.CapabilityInteractionDecision{
			Executable: true,
			Request: interaction.CapabilityRequest{
				Operation:  "abrir_archivo",
				Parameters: map[string]string{"raw": input},
				Confidence: 1,
				Source:     "session.file_action",
			},
			Reason: "explicit file-open continuation",
		}
		_ = e.authorityExecute(decision, req)

	case "mover":
		dest := e.extractFolderTarget(input)
		if dest == "" {
			e.renderCanonicalSystemResponse(req, "file_action_missing_target", "¿A dónde lo muevo?", interaction.MoveAsk, interaction.ResponseIntentAsk, true)
			return
		}
		decision := interaction.CapabilityInteractionDecision{
			Executable: true,
			Request: interaction.CapabilityRequest{
				Operation:  "mover_archivo",
				Target:     dest,
				Parameters: map[string]string{"raw": input},
				Confidence: 1,
				Source:     "session.file_action",
			},
			Reason: "explicit file-move continuation",
		}
		_ = e.authorityExecute(decision, req)

	case "renombrar":
		e.renderCanonicalSystemResponse(req, "file_action_missing_name", "¿Cómo lo llamo?", interaction.MoveAsk, interaction.ResponseIntentAsk, true)
		e.sess.SetRenaming()

	default:
		dest := e.extractFolderTarget(input)
		if dest != "" {
			decision := interaction.CapabilityInteractionDecision{
				Executable: true,
				Request: interaction.CapabilityRequest{
					Operation:  "mover_archivo",
					Target:     dest,
					Parameters: map[string]string{"raw": input},
					Confidence: 1,
					Source:     "session.file_action",
				},
				Reason: "explicit file-move continuation",
			}
			_ = e.authorityExecute(decision, req)
			return
		}
		e.renderCanonicalSystemResponse(req, "file_action_unknown", "Di: abrir, mover o renombrar", interaction.MoveClarify, interaction.ResponseIntentClarify, true)
	}
}

/* =======================
   SHOW FILE LIST
======================= */

func (e *Engine) showFileList(results []memory.SessionFile) string {
	top := len(results)
	if top > 5 {
		top = 5
	}
	fmt.Printf("\n╔══════════════════════════════════════╗\n")
	fmt.Printf("║  ENCONTRADOS: %-22d║\n", len(results))
	fmt.Printf("╚══════════════════════════════════════╝\n")
	for i := 0; i < top; i++ {
		name := filepath.Base(results[i].Path)
		age := formatTimeAgo(results[i].LastModified)
		fmt.Printf("  %d. %s (%s)\n", i+1, name, age)
	}
	fmt.Println("  → Di el número (1-5) o \"cancela\"")

	spoken := personality.ManyFilesFound(len(results))
	for i := 0; i < top; i++ {
		spoken += ". " + personality.FileOption(i+1, filepath.Base(results[i].Path))
	}
	if len(results) > 5 {
		spoken += ". " + personality.MoreOptions()
	}
	return spoken
}

/* =======================
   REASON ABOUT QUESTION (Bloque 33 — Pipeline Cognitivo Unificado)
======================= */

// reasonAboutQuestion es el punto único de convergencia para responder
// una pregunta (IntentQuestion, confianza del clasificador < 0.6).
// Antes de este bloque, Knowledge Core y Cognitive Mind (Bloque 30)
// eran dos branches independientes dentro de Process(), cada uno con
// su propio e.sayAsync + return, y el fallback de Uncertainty era un
// tercero. Ahora los tres arman la respuesta ACÁ, y Process() hace un
// solo e.sayAsync con lo que esta función devuelva — "convergen en un
// solo punto antes de sayAsync", como se dejó anotado al cerrar el
// orden de Bloques 31→33→34→35→32.
//
// De paso, es el primer código que construye y llena un
// cognition.ReasoningContext (Bloque 31) — hasta acá era un contrato
// sin nadie que lo usara. rc vive y muere dentro de esta llamada: se
// llena para que Knowledge Core y Cognitive Mind compartan un solo
// objeto en vez de variables sueltas, pero no persiste entre turnos
// todavía — eso es Reasoning Cache (Bloque 32), que a propósito quedó
// último en el orden porque recién ahora existe un objeto único que
// cachear.
//
// Alcance de este bloque, documentado para no generar falsas
// expectativas: unifica los TRES caminos que hoy conviven en este
// punto puntual de Process() — Knowledge Core, Cognitive Mind y
// Uncertainty. El resto de Process() (los ~15 puntos de return de las
// ramas de sesión pendiente, las meta-preguntas de Fase 2.6/2.8
// —Bloques 9-19—, el aprendizaje de comandos nuevos) sigue exactamente
// igual: no son parte de "responder una pregunta" propiamente dicho, y
// unificarlos ahí sería un cambio mucho más amplio del que este bloque
// se propuso resolver. Tampoco se tocó refineResponseWithCapa6 (más
// abajo en este archivo) — sigue sin invocarse desde ningún lado, tal
// como estaba antes de este bloque; activarlo sin un caso de uso real
// sería crear, no integrar.
func selfReferentialQuestion(req interaction.SemanticRequest) bool {
	// Ownership is already established by B96.32 at the semantic boundary.
	// Do not maintain a second raw-text self-question classifier here: that
	// was a competing interpretation authority and was the reason questions
	// such as "¿qué haces?" could diverge from the canonical subject.
	return req.Subject == interaction.UserObservationSubjectSelf
}

func conversationalResponseEligible(r interaction.SemanticRequest, intent interaction.ResponseIntent) bool {
	// ResponseIntent is the canonical response authority. Do not maintain an
	// allow-list of communicative acts here: such a list silently becomes a
	// second response classifier and can strand valid intents (for example
	// correction/clarification) after cognition has already selected them.
	if intent.Kind == interaction.ResponseIntentNone || !intent.ShouldSpeak {
		return false
	}
	// Executable commands belong to the capability/runtime path. Control
	// turns are handled by the explicit control boundary before this gate.
	acts := interaction.CanonicalActs(r)
	if acts.Has(interaction.ActCommand) {
		return false
	}
	// Answer intents require a grounded consumer first (self knowledge, user
	// model, historical memory, deterministic reasoning, or another domain
	// provider). The generic response gate must not turn an unanswered factual
	// question into an acknowledgement such as "Entendido.". Other intents
	// can be rendered directly once the canonical intent/move have been chosen.
	return intent.Kind != interaction.ResponseIntentAnswer
}

func (e *Engine) setExpressionProfile(r interaction.SemanticRequest, mode *cognition.ConversationMode) {
	if e.responseComposer == nil {
		return
	}
	meaning := interaction.BuildSemanticMeaning(r)
	intent := e.lastResponseIntent
	formality, warmth, initiative := 0.5, 0.5, 0.5
	detail := "brief"
	if mode != nil {
		formality = mode.Formality
		warmth = mode.Warmth
		initiative = mode.Initiative
	}
	if e.contextAdapter != nil {
		detail = e.contextAdapter.PreferredLength
	}
	profile := interaction.BuildExpressionProfile(intent, meaning, formality, warmth, initiative, detail)
	e.responseComposer.SetExpressionProfile(profile)
}

// setExpressionProfileFromInput is the fallback used by older response
// paths that do not carry the full SemanticRequest at the call site yet.
// It deliberately remains conservative: the response pipeline can evolve
// without fabricating a language or conversational intent.
func (e *Engine) setExpressionProfileFromInput(input string) {
	if e.responseComposer == nil {
		return
	}
	r := interaction.SemanticRequest{Raw: input, Language: e.lastResponseIntent.Language}
	meaning := interaction.BuildSemanticMeaning(r)
	profile := interaction.BuildExpressionProfile(e.lastResponseIntent, meaning, 0.5, 0.5, 0.5, "brief")
	e.responseComposer.SetExpressionProfile(profile)
}

/* =======================
   REFINE RESPONSE WITH CAPA 6
======================= */

// refineResponseWithCapa6 transforma respuesta simple en respuesta refinada con lenguaje natural
func (e *Engine) refineResponseWithCapa6(req interaction.SemanticRequest, simpleAnswer string, confidence float64) string {
	// CAPA 6: Usar generador de lenguaje para expresar la respuesta de forma natural
	// Si la confianza es alta, expresar como opinión fuerte
	// Si es baja, expresar con reservas

	confidenceDesc := e.getConfidenceDescription(confidence)

	// Generar expresión natural de la opinión
	refinedResponse := e.languageGenerator.GenerateQuick(
		simpleAnswer,
		confidence,
		confidenceDesc,
	)

	// Bloque 18 (Fase 2.8) — tercer y último punto de Language
	// Generation que pasa por ContextAdapter antes de llegar a sayAsync.
	// Bloque 20 (Fase 3) — ResponseComposer decora la respuesta
	// con las 6 decisiones: Tone, Detail, Humor, MentionMemory,
	// AskFollowUp, ChallengeUser.
	e.setExpressionProfile(req, nil)
	currentTopic := req.Topic
	plan := e.responseComposer.CreatePlan(currentTopic)
	return e.responseComposer.ComposeResponse(refinedResponse, plan)
}

// getConfidenceDescription retorna descripción textual de confianza
func (e *Engine) getConfidenceDescription(confidence float64) string {
	switch {
	case confidence >= 0.9:
		return "muy alta"
	case confidence >= 0.8:
		return "alta"
	case confidence >= 0.7:
		return "buena"
	case confidence >= 0.6:
		return "moderada"
	case confidence >= 0.5:
		return "baja"
	default:
		return "muy baja"
	}
}

// applyConversationalAdaptation makes B96.33.11 observable to the response
// pipeline without promoting it into durable user memory. Only transient
// presentation parameters are affected here; execution authority remains
// outside cognition.
func (e *Engine) applyUserModelCommunicationProfile() {
	if e == nil || e.userModelStore == nil || e.contextAdapter == nil {
		return
	}
	profile, err := interaction.GetCommunicationProfile(e.userModelStore, interaction.UserModelActorCognition, interaction.UserModelQueryContext{})
	if err != nil {
		return
	}
	if value, ok := profile[string(interaction.CommunicationDetailTolerance)]; ok {
		switch strings.ToLower(strings.TrimSpace(value.Value)) {
		case "brief", "concise", "short", "breve", "conciso":
			e.contextAdapter.PreferredLength = "brief"
		case "detailed", "detallado", "detalle":
			e.contextAdapter.PreferredLength = "detailed"
		case "comprehensive", "completo", "exhaustive", "exhaustivo":
			e.contextAdapter.PreferredLength = "comprehensive"
		}
	}
}

func (e *Engine) applyConversationalAdaptation() {
	if e == nil || e.contextAdapter == nil {
		return
	}
	switch e.lastConversationalCognition.Adaptation.Detail {
	case cognition.AdaptationDetailConcise:
		e.contextAdapter.PreferredLength = "brief"
	case cognition.AdaptationDetailDetailed:
		e.contextAdapter.PreferredLength = "comprehensive"
	case cognition.AdaptationDetailStandard:
		e.contextAdapter.PreferredLength = "detailed"
	}
}

// persistUserObservations is the single Engine -> B96.34 mutation bridge.
// The Engine supplies semantic ownership/context; interaction owns domain
// extraction, admission, privacy, reconciliation, conflict resolution and
// canonical persistence. No domain-specific UserModel write remains in Core.
type userObservationPersistenceReport struct {
	Attempted int
	Handled   int
	Applied   int
	Rejected  int
	Errors    []string
}

func (r userObservationPersistenceReport) Successful() bool {
	return r.Attempted > 0 && r.Applied == r.Attempted && len(r.Errors) == 0
}

func (e *Engine) persistUserObservations(req interaction.SemanticRequest) userObservationPersistenceReport {
	report := userObservationPersistenceReport{}
	if e == nil || e.userModelStore == nil {
		return report
	}
	if req.Subject == "" {
		req.Subject = interaction.UserObservationSubjectUser
	}
	now := time.Now()
	observations := interaction.DetectUserObservationsFromSemantic(req, now, fmt.Sprintf("turn-%d", e.activeConversationTurnID))
	for _, observation := range observations {
		if observation.Subject != interaction.UserObservationSubjectUser {
			continue
		}
		report.Attempted++
		// Conversation Topic is not a conflict scope. A topic such as
		// "color" identifies what the turn is about; it does not mean
		// that the belief is scoped to a distinct context. B96.34.13 must
		// only receive an explicit context when cognition has actually
		// established one. Reusing req.Topic here creates a false context
		// split because the existing persisted belief has no corresponding
		// scope metadata, causing genuine contradictions to be treated as
		// separate contexts.
		observation.Context = ""
		outcome, handled, err := interaction.ApplyUserObservation(e.userModelStore, observation, now)
		if handled {
			report.Handled++
		}
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
			continue
		}
		if outcome.Applied {
			report.Applied++
		} else {
			report.Rejected++
		}
		// P4 — el posterior de preferencias escucha SOLO aceptaciones
		// explícitas del camino canónico (extensión, no segunda autoridad).
		if outcome.Applied && observation.Domain == interaction.UserModelDomainPreference {
			e.observeAcceptedPreference(observation, now)
		}
	}
	if report.Attempted > 0 && report.Applied > 0 && len(report.Errors) == 0 {
		if err := e.userModelStore.Save(interaction.UserModelPath, interaction.UserModelActorSystem); err != nil {
			report.Errors = append(report.Errors, err.Error())
		} else {
			// P4: el snapshot derivado no puede sobrevivir a una falla de la
			// autoridad canónica B96.34. Solo se persiste si el Save tuvo éxito.
			e.saveUserPreferenceSnapshot()
		}
	}
	return report
}

// persistPersonalAnswerObservation keeps a verified answer to Ahmelia's open
// question on the same canonical B96.34 path. The answer itself is not
// reinterpreted as an unrelated fact; provenance carries the real turns: the
// source is the turn of the user's utterance and the evidence is the turn
// where Ahmelia asked.
func (e *Engine) persistPersonalAnswerObservation(observation interaction.PersonalAnswer, sourceID string, evidenceID string) {
	if e == nil || e.userModelStore == nil || !observation.OK {
		return
	}
	now := time.Now()
	obs := interaction.PersonalAnswerToUserObservation(observation, now, sourceID, evidenceID)
	if obs.Subject != interaction.UserObservationSubjectUser {
		return
	}
	outcome, handled, err := interaction.ApplyUserObservation(e.userModelStore, obs, now)
	if handled && err == nil {
		// P4 — misma regla del puente: solo aceptaciones explícitas.
		if outcome.Applied && obs.Domain == interaction.UserModelDomainPreference {
			e.observeAcceptedPreference(obs, now)
		}
		// P4: misma regla que persistUserObservations; el derivado no se
		// guarda si la autoridad canónica no pudo persistir.
		if err := e.userModelStore.Save(interaction.UserModelPath, interaction.UserModelActorSystem); err == nil {
			e.saveUserPreferenceSnapshot()
		}
	}
}

// personalDataAskBack is the deterministic reconfirmation Ahmelia renders when
// a personal data query has no stored answer. It is the moment the open
// question is born: only an answer to it can enter the user model.
const personalDataAskBack = "Todavía no tengo ese dato guardado. ¿Me lo dices?"

// registerOpenPersonalQuestion opens the only admission door for a personal
// answer. The key/category come from the semantic resolver of the user's own
// query, never from a list of attribute names.
func (e *Engine) registerOpenPersonalQuestion(category, key string) {
	if e == nil || e.conversationState == nil {
		return
	}
	e.conversationState.RegisterOpenQuestion(interaction.OpenQuestionKindPersonalValue, category, key, personalDataAskBack, e.activeConversationTurnID)
}

// deliverLatePersonalQuery es la contraparte DIFERIDA de la puerta de datos
// personales de runSelfAndUserModelDecisions (turn_pipeline.go). Gap encontrado
// en la demo por voz del propietario (2026-10-17 19:10-19:29): con la
// comprensión abierta el turno vivo ya no espera la lectura (presupuesto sync
// ~900 ms) y la lectura llega después; cuando esa lectura tardía muestra una
// consulta sobre un dato propio del usuario que Ahmelia aún no tiene, la deuda
// honesta sigue siendo la reconfirmación — nunca una intervención sin
// contenido. La entrega ya pasó el gobierno de iniciativa; esta función solo
// decide el CONTENIDO y la puerta de admisión:
//
//   - consulta personal clara → registrar la pregunta abierta + ask-back;
//   - dato ya conocido → responder el valor (misma voz y contrato que sync);
//   - pregunta ya abierta → silencio (nada que deber);
//   - lectura con marcas restrictivas (B2) → silencio: la guarda puede
//     PROHIBIR la captura llana y su gesto, jamás autorizar otra cosa.
//
// Devuelve true cuando la entrega quedó bajo esta autoridad (hable o calle).
func (e *Engine) deliverLatePersonalQuery(result linguistics.DeferredResult) bool {
	if e == nil || e.conversationState == nil || e.userModelStore == nil || result.Open == nil {
		return false
	}
	om := *result.Open
	req := mergeOpenMeaning(result.Job.Request, om)
	key, ok := latePersonalKey(req)
	if !ok {
		return false
	}
	jobID := result.Job.ID
	// Suficiencia de deuda: apenas la entrega queda bajo esta autoridad (hable
	// o calle), el job no vuelve a ofrecerse. Sin esta marca la re-demo repetía
	// el mismo gesto en cada tick.
	assume := func() bool {
		e.deferredLinguistic.MarkDelivered(jobID)
		return true
	}
	// Marcas restrictivas: la misma guarda B2 del consumo aplicada al disparo
	// del ask-back. Puede prohibir; nunca autoriza.
	if blocked, why := openMarksBlockPlainCapture(linguistics.DeriveReading(om).Marks); blocked {
		e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=ask_back_skipped reason=marks:%s job=%s key=%q", why, jobID, key))
		return assume()
	}
	category := "attribute"
	domain := interaction.UserModelDomainAttribute
	if interaction.SemanticAttributeDomainForKey(key) == interaction.SemanticAttributeDomainPreference {
		category = "preference"
		domain = interaction.UserModelDomainPreference
	}
	// ¿Ya lo sabe? Entonces la deuda ya no es preguntar: es responder — misma
	// voz y contrato que el camino sync (un valor, o la honradez de varios).
	query := interaction.UserModelQueryContext{Topics: []string{key}}
	answerText := ""
	answerValue := ""
	answerAttr := ""
	answerConfidence := 0.0
	if domain == interaction.UserModelDomainPreference {
		if prefs, err := interaction.GetRelevantPreferences(e.userModelStore, interaction.UserModelActorCognition, query); err == nil {
			switch {
			case len(prefs) == 1:
				answerText = "Tu " + prefs[0].Key + " es " + prefs[0].Value + "."
				answerValue = prefs[0].Value
				answerAttr = prefs[0].Key
				answerConfidence = prefs[0].Confidence
			case len(prefs) > 1:
				values := make([]string, 0, len(prefs))
				for _, pref := range prefs {
					values = append(values, pref.Key+"="+pref.Value)
				}
				answerText = "Tengo más de un registro posible para esa preferencia: " + strings.Join(values, ", ") + ". No quiero elegir uno sin certeza."
				answerConfidence = .5
			}
		}
	} else {
		if attributes, err := interaction.GetRelevantAttributes(e.userModelStore, interaction.UserModelActorCognition, query); err == nil {
			switch {
			case len(attributes) == 1:
				answerText = "Tu " + attributes[0].Key + " es " + attributes[0].Value + "."
				answerValue = attributes[0].Value
				answerAttr = attributes[0].Key
				answerConfidence = attributes[0].Confidence
			case len(attributes) > 1:
				values := make([]string, 0, len(attributes))
				for _, attribute := range attributes {
					values = append(values, attribute.Key+"="+attribute.Value)
				}
				answerText = "Tengo más de un registro posible para ese dato: " + strings.Join(values, ", ") + ". No quiero elegir uno sin certeza."
				answerConfidence = .5
			}
		}
	}
	if strings.TrimSpace(answerText) != "" {
		// Vetar el doble habla: si el turno más reciente ya respondió ese mismo
		// valor (el camino sync lo hizo con el dato en mano), la entrega
		// diferida se cierra en silencio y el job queda consumido.
		if answerValue != "" && strings.Contains(e.conversationState.Snapshot().LastAssistant, answerValue) {
			e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=answer_known_skipped reason=already_spoken job=%s key=%q", jobID, key))
			return assume()
		}
		contract := interaction.NewResponseContract(req, e.lastResponseIntent, interaction.MoveAnswer)
		contract.Source = "user_model"
		contract.AddEvidence(answerText, "user_model", true, answerConfidence)
		if strings.TrimSpace(answerValue) != "" && strings.TrimSpace(answerAttr) != "" {
			contract.RenderScene = fase2ValueScene(answerAttr, answerValue)
		}
		e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=answer_known domain=%s key=%q job=%s", domain, jobID, key))
		if !e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot())) {
			e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=answer_known_render_refused job=%s key=%q", jobID, key))
		}
		return assume()
	}
	snap := e.conversationState.Snapshot()
	for _, q := range snap.OpenQuestions {
		if q.Status == interaction.OpenQuestionOpen && strings.EqualFold(strings.TrimSpace(q.Key), key) {
			e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=ask_back_skipped reason=already_open job=%s key=%q", jobID, key))
			return assume()
		}
	}
	// Anclaje: entre turnos activeConversationTurnID es 0; la pregunta debe
	// anclarse al último turno real del usuario para conservar su ventana de
	// frescura (misma semántica que la puerta sync).
	anchor := e.activeConversationTurnID
	if anchor == 0 && len(snap.Turns) > 0 {
		anchor = snap.Turns[len(snap.Turns)-1].ID
	}
	e.conversationState.RegisterOpenQuestion(interaction.OpenQuestionKindPersonalValue, category, key, personalDataAskBack, anchor)
	contract := interaction.NewResponseContract(req, e.lastResponseIntent, interaction.MoveAnswer)
	contract.Source = "user_model"
	contract.AddEvidence(personalDataAskBack, "user_model", true, .4)
	contract.RenderScene = fase2AskBackScene(key)
	e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=ask_back domain=%s key=%q job=%s anchor=%d", domain, jobID, key, anchor))
	if !e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot())) {
		e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=ask_back_render_refused job=%s key=%q", jobID, key))
	}
	return assume()
}

// deliverLateTypedAct cierra la entrega diferida cuando la lectura abierta
// alcanzó un acto tipado que el turno vivo no llegó a esperar (espera acotada
// agotada o lectura más lenta que su presupuesto): el acto de feedback entra
// al renderizador como la misma escena determinista del carril sync. Devuelve
// true cuando la entrega quedó bajo esta autoridad (habló o ya estaba dicha).
func (e *Engine) deliverLateTypedAct(result linguistics.DeferredResult) bool {
	if e == nil || result.Open == nil {
		return false
	}
	om := *result.Open
	if om.Purpose != linguistics.OpenPurposeFeedback {
		return false
	}
	req := mergeOpenMeaning(result.Job.Request, om)
	if !req.CommunicativeActs.Has(interaction.ActFeedback) {
		return false
	}
	jobID := result.Job.ID
	if e.deferredLinguistic != nil {
		e.deferredLinguistic.MarkDelivered(jobID)
	}
	e.adoptOpenMeaning(result.Job.Request.Raw, om)
	// Vetar el doble habla: si el turno vivo ya dijo la escena (la espera
	// acotada llegó a tiempo), la repetición no aporta nada. La marca se busca
	// en la respuesta actual y en los últimos turnos: entre la escena viva y
	// esta entrega puede haber pasado otro turno del usuario.
	if e.feedbackSceneAlreadySpoken() {
		e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=feedback_scene_skipped reason=already_spoken job=%s", jobID))
		return true
	}
	e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=feedback_scene purpose=%s topic=%q job=%s", om.Purpose, om.Topic, jobID))
	if !e.renderCanonicalSceneResponse(req, "deferred_linguistic_feedback",
		interaction.RenderScene{Scene: string(linguistics.MoveFeedbackRecibido)},
		"Gracias, lo tendré en cuenta",
		interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false) {
		e.markLinguisticTrace(fmt.Sprintf("mode=deferred result=feedback_scene_refused job=%s", jobID))
	}
	return true
}

// feedbackSceneAlreadySpoken reports whether the deterministic feedback scene
// was already spoken by the current turn or one of the last turns. It exists so
// the late delivery of a reading that the live turn already awaited does not
// repeat the scene that turn rendered.
func (e *Engine) feedbackSceneAlreadySpoken() bool {
	if e == nil || e.conversationState == nil {
		return false
	}
	snap := e.conversationState.Snapshot()
	if strings.Contains(snap.LastAssistant, "Gracias por decírmelo") {
		return true
	}
	first := len(snap.Turns) - 3
	if first < 0 {
		first = 0
	}
	for i := len(snap.Turns) - 1; i >= first; i-- {
		if strings.Contains(snap.Turns[i].Assistant, "Gracias por decírmelo") {
			return true
		}
	}
	return false
}

// latePersonalKey resuelve la clave del atributo propio que identifica una
// consulta personal en una lectura (la misma resolución que la puerta sync).
// ok=false significa que la lectura no es una consulta por un dato propio sin
// valor.
func latePersonalKey(req interaction.SemanticRequest) (string, bool) {
	attribute, ok := interaction.ResolveSemanticAttribute(req)
	if !ok || attribute.HasValue || attribute.Subject != interaction.UserObservationSubjectUser {
		return "", false
	}
	key := strings.TrimSpace(attribute.Key)
	if key == "" {
		return "", false
	}
	return key, true
}

// payReadyPersonalQueries paga la deuda personal de los jobs diferidos listos
// del más nuevo al más viejo: entrega cada consulta personal por la única
// puerta `deliverLatePersonalQuery` y marca el job — y su familia por la misma
// clave — para que jamás se re-ofrezca. Devuelve true cuando el tick quedó bajo
// esta deuda (sin guarda de rendición, sin arbitraje, sin invitación).
func (e *Engine) payReadyPersonalQueries() bool {
	if e == nil || e.deferredLinguistic == nil {
		return false
	}
	ready := e.deferredLinguistic.Ready()
	if len(ready) == 0 {
		return false
	}
	// Ready() llega ordenado del más antiguo al más nuevo: la consulta más
	// fresca es la que primero importa.
	for i := len(ready) - 1; i >= 0; i-- {
		result := ready[i]
		if result.Open == nil {
			continue
		}
		key, ok := latePersonalKey(mergeOpenMeaning(result.Job.Request, *result.Open))
		if !ok {
			continue
		}
		if !e.deliverLatePersonalQuery(result) {
			continue
		}
		// Toda la familia por la misma clave queda consumida: un solo habla,
		// ninguna repetición en ticks siguientes.
		for j := range ready {
			if j == i || ready[j].Open == nil {
				continue
			}
			familyKey, ok := latePersonalKey(mergeOpenMeaning(ready[j].Job.Request, *ready[j].Open))
			if ok && strings.EqualFold(familyKey, key) {
				e.deferredLinguistic.MarkDelivered(ready[j].Job.ID)
			}
		}
		return true
	}
	return false
}

// captureVerifiedPersonalAnswer is the only personal-answer admission path.
// The verified answer is stored with real provenance (source: the utterance
// turn; evidence: the turn where Ahmelia asked), the open question is closed
// and the answer acknowledged.
func (e *Engine) captureVerifiedPersonalAnswer(t *processTurn, answer interaction.PersonalAnswer, open interaction.OpenQuestion) bool {
	// Guarda BF1/BF2 (firma 18:38, consumo 18:47): si la lectura abierta de
	// ESTE enunciado trae marcas que prohíben el hecho llano (negation /
	// condition / modality≠affirmed / incompatibility), la respuesta no se
	// captura ni la pregunta se cierra. No es un fallo: es honradez — el
	// candidato queda marcado y el turno sigue su camino normal de respuesta.
	if blocked, why := e.plainCaptureBlockedByOpenMarks(t); blocked {
		e.markLinguisticTrace(fmt.Sprintf("mode=guard result=blocked reason=%s raw=%q", why, t.semanticReq.Raw))
		return false
	}
	if e.personalMemory != nil && e.conversationState != nil {
		snap := e.conversationState.Snapshot()
		e.personalMemory.Record(answer.Category, answer.Key, answer.Value, "answer_to_personal_question", snap.SessionID, e.activeConversationTurnID, answer.Confidence)
	}
	e.persistPersonalAnswerObservation(answer, fmt.Sprintf("turn-%d", e.activeConversationTurnID), fmt.Sprintf("turn-%d", open.AskedTurn))
	if e.conversationState != nil {
		e.conversationState.CloseOpenQuestion(open.ID, interaction.OpenQuestionAnswered, e.activeConversationTurnID)
	}
	ack := interaction.BuildAnswerAcknowledgement(answer.Value, open.Question)
	e.renderCanonicalSystemResponse(t.semanticReq, "answer_acknowledgement", ack, interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
	if e.conversationState != nil {
		e.conversationState.SetAssistantResponse(ack)
	}
	return true
}

/* =======================
   SAY / SAYASYNC
======================= */

func (e *Engine) say(msg string) {
	if !e.allowSpeechThisTurn(msg) {
		return
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("LANGUAGE", msg)
	}
	fmt.Println("💬", msg)
	if e.onResponse != nil {
		e.onResponse(msg)
	}
	if e.conversationTurnActive && e.conversationState != nil {
		e.conversationState.SetAssistantResponse(msg)
	}
	tts.SpeakSync(msg)
	e.sess.RefreshTimeout()
}

func (e *Engine) sayAsync(msg string) {
	if !e.allowSpeechThisTurn(msg) {
		return
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("LANGUAGE", msg)
	}
	fmt.Println("💬", msg)
	if e.onResponse != nil {
		e.onResponse(msg)
	}
	if e.conversationTurnActive && e.conversationState != nil {
		e.conversationState.SetAssistantResponse(msg)
	}
	tts.Speak(msg)
}

// allowSpeechThisTurn — invariante de Fase 2 (escena D): dentro del MISMO
// turno activo, el mismo contenido normalizado no se emite dos veces. La
// duplicación observada por el propietario (misma frase hablada dos veces en
// un turno) muere aquí; contenido distinto y turnos nuevos pasan igual que
// antes. Entre turnos no aplica: la supresión es por turno, no entre turnos.
func (e *Engine) allowSpeechThisTurn(msg string) bool {
	normalized := normalizeSpokenSpeech(msg)
	if normalized == "" || !e.conversationTurnActive || e.activeConversationTurnID == 0 {
		return true
	}
	e.speechMu.Lock()
	defer e.speechMu.Unlock()
	if e.lastSpokenTurnID == e.activeConversationTurnID && e.lastSpokenNormalized == normalized {
		if e.flowTrace != nil {
			e.flowTrace.Mark("LANGUAGE DEGRADATION", fmt.Sprintf("duplicate_speech_suppressed turn=%d text=%q", e.activeConversationTurnID, msg))
		}
		return false
	}
	e.lastSpokenTurnID = e.activeConversationTurnID
	e.lastSpokenNormalized = normalized
	return true
}

// normalizeSpokenSpeech collapses the mechanical surface so punctuation and
// spacing do not defeat the duplication guard.
func normalizeSpokenSpeech(msg string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(msg))), " ")
}

/* =======================
   EXTRACT FOLDER TARGET (MÉTODO DEL ENGINE)
======================= */

func (e *Engine) extractFolderTarget(input string) string {
	norm := normalizeInput(input)
	for key := range e.folderDst {
		if strings.Contains(norm, key) {
			return key
		}
	}
	return ""
}

// activateTopic — Integration Fix 2 (Topic & Knowledge Routing),
// ampliada en Fase 6 (B72/B76). Centraliza lo que significa "este es
// ahora el tema activo": hasta acá, cada sitio que quería marcar un
// tema llamaba a ConversationFlow.RecordTopic() Y a
// ConversationalMemory.Touch() (+ Save) por separado — dos llamadas
// que SIEMPRE tienen que ir juntas, pero nada obligaba a que así
// fuera. Ese fue exactamente el hueco detrás del bug de "profundiza en
// Blender": el punto donde Skill Builder termina de aprender un
// dominio no llamaba a ninguna de las dos, así que ConversationFlow se
// quedaba con el último tema puesto por comprehension.ConversationContext
// (que no sabe nada de dominios de conocimiento). Mismo motivo por el
// que ahora también refleja el tema en AutonomousState (72,
// SetActiveTopic) y en ConversationManager (76, SetCurrentTopic): sin
// esto, un tema activado por Skill Builder/un movimiento de reanudación nunca hubiera
// llegado a los bloques de autonomía — habrían seguido viendo el
// último tema que puso comprehension.ConversationContext, el mismo bug
// de fondo, un nivel más arriba. state/cm de autonomía son nil-safe
// (mismo criterio que flow/cm de abajo) para no romper los tests
// existentes que llaman a la función sin pasarlos. Versión libre de
// *Engine para poder llamarla desde los closures de knowledgeEngine
// (registrados en NewEngine, antes de que exista un *Engine sobre el
// que colgar un método).
func activateTopic(flow *language.ConversationFlow, cm *dialogue.ConversationalMemory, state *autonomy.AutonomousState, convMgr *autonomy.ConversationManager, topic string) {
	if topic == "" || flow == nil || cm == nil {
		return
	}
	flow.RecordTopic(topic)
	cm.Touch(topic)
	_ = cm.Save(dialogue.ConversationalMemoryPath)
	if state != nil {
		state.SetActiveTopic(topic)
	}
	if convMgr != nil {
		convMgr.SetCurrentTopic(topic)
	}
}

// activateTopic — método de conveniencia para el resto de Process(),
// donde e.conversationFlow/e.conversationalMemory/e.autonomousState/
// e.conversationManager ya están al alcance.
func (e *Engine) activateTopic(topic string) {
	activateTopic(e.conversationFlow, e.conversationalMemory, e.autonomousState, e.conversationManager, topic)
}

// isBareConfirmation — Integration Fix. Distingue una respuesta de
// PURO control de flujo ("confirmar", "sí", "dale", "adelante") de una
// que además trae contenido real ("sí, para modelado 3D"). Solo la
// primera debe tratarse como "seguí con el objetivo tal cual estaba"
// — anexarla al OriginalGoal fusionado ensuciaría el MainGoal con
// palabras que no describen nada del dominio ("Blender Confirmar").
// Se implementa restando las palabras de confirmación conocidas del
// input normalizado: si no queda nada sustantivo, es confirmación
// pura.
//
// Bug real encontrado en vivo — Integration Fix: "ha confirmado" (el
// reconocimiento de voz transcribiendo "sí, confirmado" o variantes)
// NO se detectaba como confirmación pura, porque "ha" no estaba en
// confirmationWords: se restaba "confirmado" pero quedaba "ha "
// sobrante, así que isBareConfirmation devolvía false y "ha
// confirmado" terminaba pegado al MainGoal fusionado ("Blender ha
// confirmado"), contaminando la Skill guardada con basura del propio
// flujo de diálogo. Se agregan las conjugaciones de "haber" que
// típicamente preceden a "confirmado"/"confirmar" en una respuesta
// afirmativa natural — no son palabras de confirmación en sí mismas,
// pero tampoco aportan nada sustantivo al objetivo, así que restarlas
// acá (y no agregarlas como confirmationWords "de verdad") mantiene
// la distinción honesta: siguen sin activar por sí solas una
// confirmación si aparecen solas ("ha" sin "confirmado" al lado NO
// debería, y no debe, confirmar nada — ver
// TestIsBareConfirmation_AuxiliarSoloNoConfirma).
func isBareConfirmation(input string) bool {
	norm := shared.Normalize(input)

	// Confirmaciones directas de control de flujo.
	if norm == "si" ||
		norm == "claro" ||
		norm == "por supuesto" ||
		norm == "dale" ||
		norm == "afirmativo" ||
		norm == "confirmar" ||
		norm == "adelante" ||
		norm == "ok" ||
		norm == "vale" ||
		norm == "correcto" ||
		norm == "dale adelante" {
		return true
	}

	// Variantes frecuentes provenientes de reconocimiento de voz:
	// "ha/he/han/hemos confirmado".
	parts := strings.Fields(norm)
	if len(parts) == 2 &&
		(parts[0] == "ha" ||
			parts[0] == "he" ||
			parts[0] == "han" ||
			parts[0] == "hemos") &&
		parts[1] == "confirmado" {
		return true
	}

	return false
}

func extractNumber(input string) (int, bool) {
	lower := strings.ToLower(input)
	numberWords := map[string]int{
		"primero": 1, "primer": 1, "uno": 1, "primera": 1,
		"segundo": 2, "dos": 2, "segunda": 2,
		"tercero": 3, "tercer": 3, "tres": 3, "tercera": 3,
		"cuarto": 4, "cuatro": 4, "cuarta": 4,
		"quinto": 5, "cinco": 5, "quinta": 5,
	}
	for word, num := range numberWords {
		if strings.Contains(lower, word) {
			return num, true
		}
	}
	for _, word := range strings.Fields(lower) {
		if n, err := strconv.Atoi(word); err == nil && n >= 1 && n <= 5 {
			return n, true
		}
	}
	return 0, false
}

// normalizeInput delega a shared.Normalize, la única versión de
// normalización del proyecto. Antes reimplementaba aquí el mismo
// strings.NewReplacer de acentos que ya existía en shared/normalize.go.
func normalizeInput(text string) string {
	return shared.Normalize(text)
}

func cleanNewName(input, currentName string) string {
	fillers := []string{
		"llamalo", "llamala", "llámalo", "llámala",
		"ponle el nombre", "el nombre es", "nombre",
		"ahora se llama", "se llama", "llamarse",
		"renombralo", "renombrala", "renombralo como", "como",
	}
	result := strings.ToLower(strings.TrimSpace(input))
	for _, f := range fillers {
		result = strings.ReplaceAll(result, strings.ToLower(f), "")
	}
	result = strings.TrimSpace(result)
	words := strings.Fields(result)
	if len(words) == 0 {
		return ""
	}
	for i, w := range words {
		if len(w) > 0 {
			runes := []rune(w)
			runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
			words[i] = string(runes)
		}
	}
	newBaseName := strings.Join(words, " ")
	if !strings.Contains(newBaseName, ".") {
		ext := filepath.Ext(currentName)
		if ext != "" {
			newBaseName = newBaseName + ext
		}
	}
	return newBaseName
}

func isProcessRunning(exeName string) bool {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", exeName)).Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(exeName))
}

func formatTimeAgo(t time.Time) string {
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days == 0:
		return "hoy"
	case days == 1:
		return "ayer"
	case days < 7:
		return fmt.Sprintf("hace %d días", days)
	default:
		w := days / 7
		switch {
		case w == 1:
			return "hace 1 semana"
		case w < 4:
			return fmt.Sprintf("hace %d semanas", w)
		default:
			m := days / 30
			if m == 1 {
				return "hace 1 mes"
			}
			return fmt.Sprintf("hace %d meses", m)
		}
	}
}

func findProgram(target string, idx ProgramIndex) string {
	normTarget := normalizeInput(target)
	if normTarget == "" {
		return ""
	}

	// Program resolution is a ranking problem, not a map lookup. A map is
	// deliberately unordered, so returning the first substring/fuzzy match
	// makes a request like "open unity" nondeterministic when the index
	// contains both "unity hub" and "unityhubhelper".
	//
	// Evidence hierarchy:
	//   exact name > exact token > token-prefix > substring > fuzzy.
	// Ties are deterministic and prefer the shorter/cleaner executable name.
	type candidate struct {
		name  string
		path  string
		score int
	}
	best := candidate{}

	better := func(c candidate) bool {
		if c.score != best.score {
			return c.score > best.score
		}
		if best.path == "" {
			return true
		}
		// Prefer the shorter display name and then the shorter path. This is
		// a deterministic fallback, not a semantic claim about which binary
		// is "more correct".
		cn := normalizeInput(c.name)
		bn := normalizeInput(best.name)
		if len(cn) != len(bn) {
			return len(cn) < len(bn)
		}
		if cn != bn {
			return cn < bn
		}
		return strings.ToLower(c.path) < strings.ToLower(best.path)
	}

	for name, path := range idx.Programs {
		normName := normalizeInput(name)
		if normName == "" {
			continue
		}

		score := 0
		switch {
		case normName == normTarget:
			score = 1000
		case hasExactProgramToken(normName, normTarget):
			score = 900
		case hasProgramTokenPrefix(normName, normTarget):
			score = 800
		case strings.Contains(normName, normTarget):
			score = 650
		default:
			fuzzy := fuzzyMatchScore(normTarget, normName)
			if fuzzy >= 50 {
				score = 500 + fuzzy
			}
		}
		if score == 0 {
			continue
		}
		c := candidate{name: name, path: path, score: score}
		if better(c) {
			best = c
		}
	}

	// Known aliases are additional evidence, but never override a direct
	// target match. This keeps "unity" focused on Unity while still
	// allowing "chrome" → "google chrome".
	if best.path == "" {
		for variantRank, variant := range getProgramVariants(target) {
			variant = normalizeInput(variant)
			if variant == "" || variant == normTarget {
				continue
			}
			for name, path := range idx.Programs {
				normName := normalizeInput(name)
				if !strings.Contains(normName, variant) {
					continue
				}
				score := 400 - variantRank
				c := candidate{name: name, path: path, score: score}
				if better(c) {
					best = c
				}
			}
		}
	}

	return best.path
}

// hasExactProgramToken reports whether target is a complete token in the
// program name. It prevents a short target such as "unity" from treating
// every name containing those letters as equally specific.
func hasExactProgramToken(name, target string) bool {
	for _, token := range strings.Fields(name) {
		if token == target {
			return true
		}
	}
	return false
}

// hasProgramTokenPrefix reports whether a token starts with target. This is
// useful for natural program names such as "unity hub": the user's short
// target "unity" is more specific evidence for that name than for a helper
// executable named "unityhubhelper".
func hasProgramTokenPrefix(name, target string) bool {
	for _, token := range strings.Fields(name) {
		if strings.HasPrefix(token, target) {
			return true
		}
	}
	return false
}

// isURLTarget detecta si el target es una URL (tiene dominio conocido)
func isURLTarget(target string) bool {
	_, ok := resolveURLTarget(target)
	return ok
}

// resolveURLTarget converts a human website alias ("youtube") or an
// explicit domain ("youtube.com") into an executable URL. This belongs to
// the execution-resolution boundary: interaction says "open this target";
// Core resolves whether that target denotes a web destination or a local
// program. It prevents the program index from fuzzy-matching "youtube" to an
// unrelated executable such as "vs_layout".
func resolveURLTarget(target string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(target))
	if t == "" {
		return "", false
	}
	if alias, ok := shared.URLAliasPhrases[t]; ok {
		t = alias + ".com"
	}
	if shared.URLKeywords[t] {
		domains := map[string]string{
			"youtube": "youtube.com", "google": "google.com", "facebook": "facebook.com",
			"twitter": "twitter.com", "whatsapp": "web.whatsapp.com", "instagram": "instagram.com",
			"tiktok": "tiktok.com", "reddit": "reddit.com", "github": "github.com",
			"gmail": "mail.google.com", "maps": "maps.google.com", "spotify": "open.spotify.com",
			"netflix": "netflix.com", "twitch": "twitch.tv", "discord": "discord.com",
			"telegram": "web.telegram.org", "linkedin": "linkedin.com", "wikipedia": "wikipedia.org",
			"chatgpt": "chatgpt.com", "claude": "claude.ai", "stackoverflow": "stackoverflow.com",
		}
		if url, ok := domains[t]; ok {
			return "https://" + url, true
		}
	}
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		return t, true
	}
	for _, d := range []string{".com", ".org", ".net", ".es", ".io", ".dev", ".tv", ".co", ".edu", ".gov"} {
		if strings.Contains(t, d) {
			return "https://" + t, true
		}
	}
	return "", false
}

// getProgramVariants retorna variantes de nombre para búsqueda flexible
func getProgramVariants(target string) []string {
	variants := []string{strings.ToLower(target)}
	t := strings.ToLower(target)

	knownVariants := map[string][]string{
		"chrome":  {"google chrome", "google-chrome", "chromium"},
		"edge":    {"microsoft edge", "msedge"},
		"firefox": {"mozilla firefox"},
		"vlc":     {"vlc media player", "videolan"},
		"vscode":  {"visual studio code", "vs code", "code"},
		"word":    {"microsoft word", "winword"},
		"excel":   {"microsoft excel"},
		"python":  {"python3"},
		"node":    {"node.js", "nodejs"},
	}

	if known, exists := knownVariants[t]; exists {
		variants = append(variants, known...)
	}
	if strings.Contains(t, "-") {
		variants = append(variants, strings.ReplaceAll(t, "-", " "))
	}
	if strings.Contains(t, " ") {
		variants = append(variants, strings.ReplaceAll(t, " ", "-"))
	}
	return variants
}

// fuzzyMatchScore retorna porcentaje de similitud 0-100
func fuzzyMatchScore(query, target string) int {
	if strings.Contains(target, query) {
		return 100
	}
	matches := 0
	lastIdx := 0
	for _, char := range query {
		idx := strings.IndexRune(target[lastIdx:], char)
		if idx == -1 {
			break
		}
		matches++
		lastIdx += idx + 1
	}
	if len(query) == 0 {
		return 0
	}
	return (matches * 100) / len(query)
}

// min retorna el mínimo entre dos números
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// B96.32.13 — Legacy Routing Migration.
// Production routing is now owned by the capability decision boundary and
// executed by integration.ProductionRuntime, which consumes the SemanticRequest
// plus an already-resolved production capability route. The Engine does not
// duplicate production provider routing here.

func registerEngineCapabilities(r *capability.Registry) {
	// Compatibility wrapper for older tests/callers. Runtime composition owns
	// the actual domain registration; the Engine does not duplicate it.
	_ = integration.RegisterCapabilities(r)
}

// programmingPlanFileChange keeps compatibility with the B96.20 integration
// tests while the domain wiring now lives in integration/.
func programmingPlanFileChange(root, projectID, path string, role programming.SourceFileRole, content, id string) programming.MultiFileChange {
	op := programming.SourceCreate
	if _, err := os.Stat(filepath.Join(root, path)); err == nil {
		op = programming.SourceWrite
	}
	return programming.MultiFileChange{
		ID: id, Operation: op,
		File:    programming.SourceFile{Path: path, Language: "Go", Role: role, ProjectID: projectID},
		Content: content,
	}
}

// InitiativeTrace returns a read-only technical trace for diagnostics.
// It never feeds back into initiative authorization.
// ReevaluateScheduledInitiatives consumes temporal triggers and re-enters the
// canonical B96.37.2 policy with the stored opportunity. It never speaks,
// generates content, or bypasses policy. The next normal Process() cycle may
// enrich the opportunity again with fresh domain signals.
func (e *Engine) ReevaluateScheduledInitiatives(now time.Time) ([]conversationalinitiative.InitiativeDecision, error) {
	if e == nil || e.initiativeScheduler == nil || e.conversationalInitiative == nil {
		return nil, fmt.Errorf("initiative scheduling runtime unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	triggered := e.initiativeScheduler.Due(now)
	decisions := make([]conversationalinitiative.InitiativeDecision, 0, len(triggered))
	for _, scheduled := range triggered {
		x, err := e.initiativeScheduler.BeginReevaluation(scheduled.ID, now)
		if err != nil {
			continue
		}
		decision := e.conversationalInitiative.Evaluate(x.Opportunity, now)
		if _, err = e.initiativeScheduler.CompleteReevaluation(x.ID, decision, now); err != nil {
			continue
		}
		decisions = append(decisions, decision)
		if e.initiativeLifecycleManager != nil {
			if lifecycle, found := e.initiativeLifecycleManager.FindByDecisionID(x.SourceDecisionID); found {
				if y, transitionErr := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleEvaluating, now, "B96.37.18", "scheduled initiative reevaluation"); transitionErr == nil {
					lifecycle = y
				}
				if y, transitionErr := e.initiativeLifecycleManager.RecordDecision(lifecycle.ID, decision, now); transitionErr == nil {
					lifecycle = y
				}
				if y, transitionErr := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleDecided, now, "B96.37.2", "scheduled reevaluation decision recorded"); transitionErr == nil {
					lifecycle = y
				}
				switch decision.Decision {
				case conversationalinitiative.Defer:
					if y, transitionErr := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleDeferred, now, "B96.37.2", "scheduled reevaluation deferred"); transitionErr == nil {
						lifecycle = y
					}
				case conversationalinitiative.Intervene:
					if y, transitionErr := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleIntervening, now, "B96.37.2", "scheduled reevaluation authorized intervention"); transitionErr == nil {
						lifecycle = y
					}
				}
				if e.initiativeObservability != nil {
					for _, tr := range e.initiativeObservability.BySchedule(x.ID) {
						_, _ = e.initiativeObservability.RecordDecision(tr.ID, decision, now)
						_, _ = e.initiativeObservability.RecordLifecycle(tr.ID, lifecycle, now)
					}
				}
			}
		}
	}
	return decisions, nil
}

func (e *Engine) InitiativeTrace(id string) (conversationalinitiative.InitiativeTrace, bool) {
	if e == nil || e.initiativeObservability == nil {
		return conversationalinitiative.InitiativeTrace{}, false
	}
	return e.initiativeObservability.Find(id)
}

// ObserveInitiativeOutcome records an outcome through B96.37.15 and mirrors it
// into B96.37.20. This is observation only; it cannot authorize a new turn.
func (e *Engine) ObserveInitiativeOutcome(d conversationalinitiative.InitiativeDecision, observation conversationalinitiative.OutcomeObservation) (conversationalinitiative.InitiativeOutcomeRecord, error) {
	if e == nil || e.initiativeOutcomeEvaluator == nil {
		return conversationalinitiative.InitiativeOutcomeRecord{}, fmt.Errorf("initiative outcome evaluator unavailable")
	}
	if observation.Initiative.ID == "" {
		observation.Initiative = d
	}
	out, err := e.initiativeOutcomeEvaluator.Evaluate(observation)
	if err != nil {
		return out, err
	}
	// B96.37.16 — outcome evidence must enter the live initiative learning
	// boundary. Learning observes; it never creates or authorizes a decision.
	if e.initiativeSignals != nil {
		if _, learnErr := e.initiativeSignals.ObserveInitiativeOutcome(out); learnErr != nil {
			return out, fmt.Errorf("initiative outcome recorded but learning integration failed: %w", learnErr)
		}
	}
	if e.initiativeLifecycleManager != nil {
		if lifecycle, found := e.initiativeLifecycleManager.FindByDecisionID(d.ID); found {
			state := conversationalinitiative.LifecycleOutcome
			if out.Outcome == conversationalinitiative.OutcomeFailed {
				state = conversationalinitiative.LifecycleFailed
			}
			if lifecycle.State == conversationalinitiative.LifecycleIntervening || lifecycle.State == conversationalinitiative.LifecycleExecuting {
				if x, transitionErr := e.initiativeLifecycleManager.Transition(lifecycle.ID, state, out.ObservedAt, "B96.37.15", "initiative outcome recorded"); transitionErr == nil {
					lifecycle = x
				}
				if state == conversationalinitiative.LifecycleOutcome {
					if x, transitionErr := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleResolved, out.ObservedAt, "B96.37.19", "initiative outcome resolved lifecycle"); transitionErr == nil {
						lifecycle = x
					}
				}
				if e.initiativeObservability != nil {
					for _, tr := range e.initiativeObservability.ByDecision(d.ID) {
						_, _ = e.initiativeObservability.RecordOutcome(tr.ID, out, out.ObservedAt)
						_, _ = e.initiativeObservability.RecordLifecycle(tr.ID, lifecycle, out.ObservedAt)
						_, _ = e.initiativeObservability.Complete(tr.ID, out.ObservedAt)
					}
				}
			}
		}
	}
	if e.initiativeObservability != nil {
		for _, tr := range e.initiativeObservability.ByDecision(d.ID) {
			if _, found := e.initiativeLifecycleManager.FindByDecisionID(d.ID); !found {
				_, _ = e.initiativeObservability.RecordOutcome(tr.ID, out, out.ObservedAt)
				_, _ = e.initiativeObservability.Complete(tr.ID, out.ObservedAt)
			}
		}
	}
	return out, nil
}

func constraintCondition(cs []conversationalinitiative.InitiativeConstraint) string {
	for _, c := range cs {
		if c.Active && c.Value != "" {
			return c.Value
		}
	}
	return ""
}
