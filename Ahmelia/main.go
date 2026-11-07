package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ahmelia/competence"
	"ahmelia/core"
	"ahmelia/inputprotocol"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
	"ahmelia/system"
	"ahmelia/tts"
	"ahmelia/voiceprotocol"
)

// wakeWords — misma lista que ai/Voice/voice.py y
// ai/Voice/voice_google_backup.py (WAKE_WORDS), para que la entrada de
// texto reconozca las mismas variantes que la de voz.
var wakeWords = []string{"ahmelia", "amelia", "a melia", "jamelia", "hamelia"}

// stripWakeWord — Integration Fix: bug real encontrado en vivo. El
// texto no NECESITA wake word (ver el comentario más abajo, en el
// lector de stdin) — pero si el usuario la escribe de costumbre (lo
// más natural viniendo de usar la voz), antes quedaba pegada al
// comando tal cual: "amelia como instalo go" se buscaba como Skill
// literal "Amelia como instalo go", que por supuesto no existe. La
// voz nunca sufre esto porque voice_google_backup.py ya hace este
// mismo strip en Python antes de mandar el comando a Go — esto es
// el equivalente para cuando el comando entra por teclado en vez de
// por micrófono. Case-insensitive, sin fuzzy matching (a diferencia
// de Python): el texto tipeado no tiene ruido de transcripción, así
// que un match exacto de la primera palabra alcanza.
func stripWakeWord(text string) string {
	lower := strings.ToLower(text)
	for _, w := range wakeWords {
		if lower == w {
			return ""
		}
		if strings.HasPrefix(lower, w+" ") {
			return strings.TrimSpace(text[len(w):])
		}
	}
	return text
}

// textInputCoalesceIdle is the transport-level boundary for pasted/multiline
// stdin. Newlines are content; a short idle gap separates interactive text turns.
func textInputCoalesceIdle() time.Duration {
	const (
		defaultIdle = 150 * time.Millisecond
		minIdle     = 20 * time.Millisecond
		maxIdle     = 2 * time.Second
	)
	v := strings.TrimSpace(os.Getenv("AHMELIA_TEXT_COALESCE_IDLE"))
	if v == "" {
		return defaultIdle
	}
	ms, err := strconv.Atoi(v)
	if err != nil {
		return defaultIdle
	}
	if ms < int(minIdle/time.Millisecond) {
		ms = int(minIdle / time.Millisecond)
	}
	if ms > int(maxIdle/time.Millisecond) {
		ms = int(maxIdle / time.Millisecond)
	}
	return time.Duration(ms) * time.Millisecond
}

func main() {
	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║        AHMELIA v3.1                  ║")
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Println()

	// UTF-8 en consola Windows
	_ = exec.Command("cmd", "/C", "chcp", "65001").Run()

	// ─── Inicializar módulos ───────────────────────────────────────
	longMem := memory.LoadLongMemory()
	shortMem := core.LoadShortMemory()
	sess := memory.NewSession()

	// ─── SelfKnowledge: Ahmelia entiende su propio estado ────────
	history := selfknowledge.NewActionHistory(20)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "v3.1")

	// T4 cable vivo (día de integración 2026-10-19) — "¿qué sabes hacer?"
	// responde con Standing derivado del ledger (competence.Overlay →
	// selfknowledge.SelfQuery.AttachCompetence, self_query.go:41). El punto
	// de conexión existía y era nil-safe; solo faltaba invocarlo. Ledger
	// persistente en memory/competence_ledger.json: sin archivo = ledger
	// vacía y la respuesta conserva la lista declarada como piso, con aviso
	// honesto de que no hay práctica verificada; ledger ilegible = continúa
	// en memoria sin bloquear el arranque. Cambio de conducta declarado
	// (directiva P2): no es un "piso idéntico", es piso declarado + aviso.
	compLedger, compLedgerErr := competence.Load("memory/competence_ledger.json")
	if compLedgerErr != nil {
		fmt.Printf("⚠️ [T4] ledger de competencia ilegible (%v) — arranca sin evidencia de competencia\n", compLedgerErr)
		compLedger, _ = competence.Load("")
	}
	compOverlay := competence.NewOverlay(compLedger, nil, nil, competence.DefaultPolicy())
	selfQuery.AttachCompetence(compOverlay)

	// ─── Engine: toda la lógica de decisión ───────────────────────
	engine := core.NewEngine(sess, longMem, &shortMem, history, selfQuery)
	// Persistencia/reconciliación: un intento que quedó running/unverified por
	// un cierre del proceso se bloquea y se anuncia; nunca se repite a ciegas.
	if recovered := engine.RecoverInterruptedWork(); recovered > 0 {
		fmt.Printf("⚠️ [reconciliación] %d trabajo(s) interrumpido(s) por reinicio; bloqueados hasta reconciliar\n", recovered)
	}
	// Hito "van juntos" (2026-10-17) — comprensión abierta EN VIVO: el oído
	// lingüístico pasa del contrato cerrado al significado abierto (directiva
	// del propietario 16:04). Suites y fixtures conservan el contrato cerrado.
	engine.EnableOpenComprehension()
	// B96.40.2 — el modelo entra en Razonar/Estrategia en modo sombra: recibe
	// el espacio de trabajo del turno ya cerrado, propone lectura/estrategia/
	// crítica y el árbitro registra la relación con la decisión del arnés. No
	// altera la respuesta ni ejecuta nada.
	engine.EnableShadowDeliberation()
	// Escena 1 (2026-11-04) — consulta acotada en segundo plano: el modelo
	// interpreta frases de descanso que el parser explícito no reconoció. El
	// arnés solo admite una restricción provisional con fuente
	// confirmed_inference; nunca cambia el turno ni ejecuta.
	engine.EnableModelConsultation()
	// Opt-in: long programming production runs in the background and its
	// result is announced at the start of the next turn. Default stays
	// synchronous until the notification/reconciliation contract is complete.
	if os.Getenv("AHMELIA_ASYNC_PRODUCTION") == "1" {
		engine.EnableAsyncProduction()
	}
	engine.EnableAgentGoals()
	engine.EnableSelfTastes()
	// Paso 3.4 (2026-11-02) — la postura durable del propietario se lee al
	// arrancar y la aplica el arnés; el modelo solo la ve y puede sugerirla.
	engine.EnablePersistentMode()
	// Escena 1 (2026-11-04) — prohibiciones temporales del propietario. Una
	// restricción de trabajo resta iniciativa propia hasta fin del día; no es
	// un modo ni bloquea una orden autorizada. El modelo no las crea ni las
	// levanta.
	engine.EnableOwnerRestrictions()
	if note := engine.LastOwnerRestrictionNote(); note != "" {
		fmt.Println("⚠️ RESTRICCIÓN DEL PROPIETARIO:", note)
	}
	// Escena 1 (2026-11-05) — estado efímero y sensible del propietario:
	// dolor/cansancio con TTL y receptividad. No es un hecho médico del
	// UserModel y no bloquea órdenes explícitas.
	engine.EnableOwnerState()
	if note := engine.LastOwnerStateNote(); note != "" {
		fmt.Println("⚠️ ESTADO DEL PROPIETARIO:", note)
	}
	// Paso 4.1 (2026-11-04) — el arnés declara qué debería observarse antes
	// de ejecutar una acción local y emite un veredicto durable después de
	// observar. El modelo solo lee el acta y puede sugerir una hipótesis.
	engine.EnableExpectationLedger()
	if err := engine.LastExpectationError(); err != "" {
		fmt.Println("⚠️ ACTA DE EXPECTATIVA:", err)
	}

	// P2 feed idempotente (directiva DeepSeek Paso 1.2, 19/10) — la ledger de
	// competencia se reconstruye desde TRABAJO REAL persistido
	// (memory/production_work.json): intentos verificados y fallidos, dedup
	// por Token estable (dos boots no duplican). Sin sesiones de trabajo la
	// ledger queda como estaba — sin invento de evidencia.
	if ws := engine.WorkStore(); ws != nil {
		n, feedErr := integration.RebuildCompetenceAttempts(compLedger, ws.List())
		if feedErr != nil {
			fmt.Printf("⚠️ [T4] feed de competencia no pudo reconstruirse (%v) — ledger continúa sin trabajo proyectado\n", feedErr)
		} else if n > 0 {
			fmt.Printf("🧩 [T4] competencia reconstruida desde trabajo real: %d intentos nuevos\n", n)
		}
	}
	// P2.3 (T4-M5) — la misma overlay alimenta la lectura de competencia del
	// modelo compartido de situación (Contribution independiente; sin
	// evidencia declara la ausencia honestamente, nunca inventa capacidades).
	engine.SetCompetenceOverlay(compOverlay)

	// Fase 6 (B72-77) — arranca el bucle autónomo en background
	// (74→72/73→75→76 en tick, ver AutonomousLoop). appCtx es el
	// contexto raíz del proceso completo: se cancela desde acá mismo
	// al salir con "amelia salir" (abajo, después del loop principal)
	// y desde el manejador de señales (Ctrl+C/SIGTERM, más abajo) —
	// Engine.Shutdown() también lo corta si algún caller externo la
	// llama directo, así que cancelar duplicado acá es seguro (cancel
	// de un context.Context ya cancelado no hace nada).
	appCtx, cancelAutonomy := context.WithCancel(context.Background())
	defer cancelAutonomy()
	directLLMBypassMode := strings.EqualFold(strings.TrimSpace(os.Getenv("AHMELIA_DIRECT_LLM_BYPASS")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("AHMELIA_DIRECT_LLM_BYPASS")), "true")
	// Diagnostic bypass: isolate the model from normal orchestration.
	// Physical execution remains behind the canonical authority/guard.
	if !directLLMBypassMode {
		engine.StartDeferredLinguistic(appCtx)
		engine.StartAutonomy(appCtx)
		engine.StartConversationalInitiative(appCtx)
		// Latido N0+N1 v0 (2026-10-17) — continuidad temporal: percepción
		// de foco (N0, 1 s, syscalls) + digestión del mundo (N1, 10 s) +
		// driver del gobernador. Cero LLM; no toma locks del turno. Solo la
		// raíz viva lo arranca: las suites no cambian de comportamiento.
		engine.StartHeartbeat(appCtx)
	} else {
		fmt.Println("⚠️ DIRECT LLM BYPASS | deferred linguistics/autonomy/conversational initiative disabled")
	}

	// Si cierras con Ctrl+C (o la terminal manda SIGTERM) en vez de decir
	// "amelia salir", esto guarda igual el contexto y la memoria — antes
	// se perdía todo porque ese guardado solo vivía dentro del comando
	// "salir" hablado.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n💾 Cerrando: guardando contexto y memoria...")
		cancelAutonomy()
		engine.Shutdown()
		os.Exit(0)
	}()

	// ─── Restaurar contexto de sesión anterior ───────────────────
	// Ahmelia recupera las últimas 10 interacciones de memoria larga
	// para saber qué estaba haciendo antes de cerrar.
	learner := system.NewLearningSystem(longMem)
	previousInteractions := learner.LoadContextFromMemory()
	if len(previousInteractions) > 0 {
		fmt.Printf("📚 Recuperado contexto: %d interacciones previas\n", len(previousInteractions))

		// Alimentar el tema activo / historial de conversación del engine
		engine.RestoreContext(previousInteractions)

		// Alimentar también el ActionHistory: "¿en qué estábamos?" lee de
		// aquí (selfknowledge.answerPreviousContext), y antes se quedaba
		// vacío en cada reinicio aunque la memoria larga sí tuviera el dato.
		for _, i := range previousInteractions {
			history.Record(i.Input, i.Action, i.Target, "", true, 0)
		}
	}

	type inputEnvelope struct {
		text             string
		channel          interaction.InputChannel
		voiceTurnTracked bool
	}
	comandos := make(chan inputEnvelope, 5)

	// GLM 20/10 (compuertas V6/V1 en vivo): puerta headless — con
	// AHMELIA_DISABLE_VOICE=1 NO se lanza el oyente de voz; sin la
	// variable el arranque es byte a byte el de siempre (voz activada).
	// El canal `comandos` no se cierra en esta rama a propósito: el fin
	// del lazo principal sigue patrocinado por ExitRequested («amelia
	// salir»), igual que con la voz viva.
	var vozCmd *exec.Cmd
	if os.Getenv("AHMELIA_DISABLE_VOICE") != "1" {
		// ─── Proceso de voz ───────────────────────────────────────────
		// 🔧 RUTA CORREGIDA
		// PROVISIONAL: apunta a voice_google_backup.py (Google Speech
		// Recognition) en vez de voice.py (Whisper local) mientras se
		// resuelve el problema de fuente de alimentación de la GPU. Volver a
		// "ai/Voice/voice.py" cuando el hardware esté resuelto, si se quiere
		// recuperar el sesgo de vocabulario (KNOWN_VOCAB/INITIAL_PROMPT) y el
		// reconocimiento offline — ver el comentario de cabecera en
		// voice_google_backup.py para el detalle del trade-off.
		voiceCmd := exec.Command("python", "ai/Voice/voice_google_backup.py")
		stdout, _ := voiceCmd.StdoutPipe()
		voiceCmd.Stderr = os.Stderr

		if err := voiceCmd.Start(); err != nil {
			fmt.Println("Error iniciando voz:", err)
			return
		}
		vozCmd = voiceCmd

		go func() {
			scanner := bufio.NewScanner(stdout)
			// Voice output is JSONL-framed. JSON escapes embedded newlines, so
			// Scanner sees exactly one physical line per conversational turn.
			for scanner.Scan() {
				text, err := voiceprotocol.Decode(scanner.Text())
				if err != nil {
					fmt.Fprintln(os.Stderr, "voice transport:", err)
					continue
				}
				if err := tts.BeginVoiceTurn(); err != nil {
					fmt.Fprintln(os.Stderr, "voice turn marker:", err)
					continue
				}
				comandos <- inputEnvelope{text: text, channel: interaction.ChannelVoice, voiceTurnTracked: true}
			}
			close(comandos)
		}()
	}

	// ─── Entrada por texto ──────────────────────────────────────────
	// Integration Fix — la voz depende de Google Speech Recognition
	// (necesita Internet, ver el comentario de más arriba sobre
	// voice_google_backup.py), así que hasta que vuelva Whisper local
	// escribir es la ÚNICA forma de probar Ahmelia con Internet
	// realmente desconectado. No es un "modo texto" con lógica
	// propia — es un segundo lector que empuja al MISMO canal que ya
	// alimenta a engine.Process(): main.go sigue sin pensar, y
	// Process() no sabe ni le importa si el texto vino de voz o de
	// teclado. A diferencia de la voz, el texto NO necesita wake word
	// ("amelia") — esa detección filtra ruido de un micrófono siempre
	// escuchando, un problema que no existe al escribir.
	go func() {
		lines := make(chan string, 32)
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				lines <- scanner.Text()
			}
			close(lines)
		}()

		idle := textInputCoalesceIdle()
		inputprotocol.CoalesceTextLines(lines, idle, func(text string) {
			if text = strings.TrimSpace(text); text != "" {
				comandos <- inputEnvelope{text: stripWakeWord(text), channel: interaction.ChannelText}
			}
		})
		// Sin close(comandos) acá a propósito: stdin y voz comparten
		// el canal; el EOF de uno no implica que el otro haya terminado.
	}()

	time.Sleep(500 * time.Millisecond)
	tts.SaludoInicial()
	// La iniciativa necesita conocer el canal real para poder hablar sin un turno
	// previo del usuario. Se activa con AHMELIA_STARTUP_INITIATIVE=1 hasta que la
	// iniciativa tenga contenido propio que decir.
	if os.Getenv("AHMELIA_STARTUP_INITIATIVE") == "1" {
		engine.SetStartupChannel(interaction.ChannelVoice)
	}
	fmt.Println("⌨️  También podés escribir un comando acá y presionar Enter (sin necesidad de decir \"amelia\")")

	// ─── Loop principal ────────────────────────────────────────────
	directLLMBypass := directLLMBypassMode
	maxBypassSteps := 1
	if raw := strings.TrimSpace(os.Getenv("AHMELIA_DIRECT_LLM_MAX_STEPS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxBypassSteps = parsed
		}
	}
	if directLLMBypass {
		fmt.Printf("\n⚠️ DIRECT LLM BYPASS ACTIVE | max_steps=%d | normal cognition layers are skipped for this experiment\n", maxBypassSteps)
	}
	for item := range comandos {
		func() {
			if item.voiceTurnTracked {
				defer tts.EndVoiceTurn()
			}
			if directLLMBypass {
				result, err := engine.ProcessDirectLLMBypass(context.Background(), item.text, item.channel, maxBypassSteps)
				if err != nil {
					fmt.Println("[DIRECT LLM ERROR]", err)
					return
				}
				if strings.TrimSpace(result.Response) != "" {
					fmt.Println("💬", result.Response)
					tts.Speak(result.Response)
				}
				if result.Decision.Action != nil {
					fmt.Printf("🔎 [DIRECT LLM] status=%s capability=%s operation=%s target=%s confidence=%.2f\n", result.Decision.Status, result.Decision.Action.CapabilityID, result.Decision.Action.Operation, result.Decision.Action.Target, result.Decision.Confidence)
				}
				if result.Result.Handled {
					fmt.Printf("🔎 [DIRECT LLM RESULT] executed=%t success=%t outcome_status=%s verified=%t observed=%t auth_required=%t failure=%s\n", result.Result.Executed, result.Result.Succeeded, result.Result.OutcomeStatus, result.Result.Verified, result.Result.Observed, result.Result.AuthorizationRequired, result.Result.FailureReason)
				}
				return
			}
			engine.ProcessWithChannel(item.text, item.channel)
		}()
		if engine.ExitRequested() {
			break
		}
	}

	if engine.ExitRequested() {
		// H7 (2026-10-17) — «salir» ya guardó todo (Shutdown dentro de la
		// ruta de cierre); acá termina el proceso y se lleva el oyente de
		// voz. Antes, este lazo quedaba vivo esperando a la voz aunque
		// Ahmelia ya se hubiera despedido, y había que matarla a mano.
		// GLM 20/10: con AHMELIA_DISABLE_VOICE=1 no hay proceso de voz
		// que matar — nil-guard (antes desreferenciaba sin pregunta).
		if vozCmd != nil && vozCmd.Process != nil {
			_ = vozCmd.Process.Kill()
		}
		fmt.Println("👋 Sesión cerrada a pedido («salir») — Ahmelia terminó el proceso")
	} else {
		fmt.Println("Proceso de voz terminado")
	}
	core.SaveShortMemory(shortMem)
}
