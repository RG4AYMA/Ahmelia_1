package memory

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// session.go — única fuente de verdad del estado conversacional.
// REGLA DE ORO: nunca tocar sess.LastSelectedFile directamente desde main.go.
// Usar solo los métodos de esta struct.

const voiceModeFile = "memory/voice_mode.txt"
const sessionTimeout = 45 * time.Second

/* =======================
   ESTADOS
   Solo uno activo a la vez. stateIdle = esperando comando nuevo.
======================= */

type sessionState int

const (
	stateIdle                sessionState = iota
	stateFileSelect                       // mostrando lista, esperando número
	stateFileAction                       // archivo elegido, esperando qué hacer
	stateRenaming                         // esperando nombre nuevo
	stateLearning                         // esperando qué acción enseñar
	stateVolumeSet                        // "¿A cuánto lo pongo?" — esperando un número
	stateAskingClarification              // Integration Fix — B96.39 clarification movement was selected
	// algo sobre un objetivo de aprendizaje ambiguo; esperando la
	// aclaración del usuario para fusionarla con el objetivo original
	// y reanalizar, en vez de tratarla como un comando nuevo (ver
	// PendingClarification, abajo).
)

/* =======================
   SESSION
   LastSelectedFile persiste entre acciones mientras no se llame ClearFile().
   Esto permite: buscar → seleccionar → abrir → "muévelo" (sin buscar de nuevo).
======================= */

type Session struct {
	state      sessionState
	stateSetAt time.Time

	// Resultados de búsqueda activos (solo válidos en stateFileSelect)
	FileResults []SessionFile

	// Archivo en foco — persiste en stateIdle si el usuario no canceló
	LastSelectedFile string

	// Aprendizaje
	LearningInput string

	// Integration Fix — pendiente mientras state == stateAskingClarification.
	// Guarda lo necesario para que el turno siguiente no tenga que
	// adivinar qué estaba pasando: ya sabe qué quería el usuario, por
	// qué se preguntó, y qué le faltaba.
	pendingClarification *PendingClarification

	// skipNextAsk: se consume una sola vez (ver ConsumeSkipNextAsk).
	// Se activa después de resolver una PendingClarification para que
	// el reanálisis del objetivo fusionado no pueda volver a disparar
	// MoveAsk sobre el mismo objetivo — eso es lo que garantiza que
	// Ahmelia nunca quede atrapada en un bucle de aclaraciones.
	skipNextAsk bool
}

type SessionFile struct {
	Path         string
	Score        int
	LastModified time.Time
}

// SessionSnapshot exposes workflow state as read-only evidence. It is a
// projection of Session, not a second owner of state or a decision engine.
type SessionSnapshot struct {
	State                string
	FileResults          []SessionFile
	LastSelectedFile     string
	LearningInput        string
	PendingClarification *PendingClarification
}

func (s *Session) Snapshot() SessionSnapshot {
	if s == nil {
		return SessionSnapshot{State: "idle"}
	}
	results := append([]SessionFile(nil), s.FileResults...)
	var pending *PendingClarification
	if s.pendingClarification != nil {
		p := *s.pendingClarification
		p.PendingAssumptions = append([]string(nil), p.PendingAssumptions...)
		pending = &p
	}
	return SessionSnapshot{
		State:                strings.ToLower(sessionStateName(s.state)),
		FileResults:          results,
		LastSelectedFile:     s.LastSelectedFile,
		LearningInput:        s.LearningInput,
		PendingClarification: pending,
	}
}

func sessionStateName(state sessionState) string {
	switch state {
	case stateFileSelect:
		return "file_select"
	case stateFileAction:
		return "file_action"
	case stateRenaming:
		return "renaming"
	case stateLearning:
		return "learning"
	case stateVolumeSet:
		return "volume_set"
	case stateAskingClarification:
		return "asking_clarification"
	default:
		return "idle"
	}
}

// PendingClarification — lo que la frontera de clarificación B96.22
// dejó pendiente al preguntar. OriginalGoal es el texto CRUDO del
// comando tal como lo dijo el usuario (dialogue.Decision.RawGoal) —
// deliberadamente NO analysis.MainGoal (que ya viene despojado de
// "aprende"/"aprender" por stripTrigger). Guardar el MainGoal en vez
// del crudo fue el bug real de la primera versión de este mecanismo:
// el objetivo fusionado ("Blender" + "confirmar" = "Blender
// confirmar") perdía el verbo disparador y dejaba de ser reconocible
// para looksLikeLearningGoal/ParseSkillBuildQuery corriente abajo —
// caía al manejo genérico de comando desconocido en vez de seguir la
// cadena de aprendizaje. Con el crudo ("aprende blender"), la fusión
// preserva el verbo sin importar qué responda el usuario.
// PendingAssumptions es lo que Goal Analyzer no pudo confirmar y
// motivó la pregunta — metadata para diagnóstico y para bloques
// futuros (Conversation Memory, Initiative), no imprescindible para
// el mecanismo en sí.
type PendingClarification struct {
	OriginalGoal       string
	PendingAssumptions []string
	Timestamp          time.Time
}

/* =======================
   CONSTRUCTOR
======================= */

func NewSession() *Session {
	s := &Session{}
	s.setVoiceMode("wake")
	return s
}

/* =======================
   CONSULTAS DE ESTADO (solo lectura)
======================= */

func (s *Session) WaitingForFileSelection() bool { return s.state == stateFileSelect }
func (s *Session) WaitingForFileAction() bool    { return s.state == stateFileAction }
func (s *Session) WaitingForRenaming() bool      { return s.state == stateRenaming }
func (s *Session) WaitingForLearning() bool      { return s.state == stateLearning }
func (s *Session) WaitingForVolumeSet() bool     { return s.state == stateVolumeSet }
func (s *Session) WaitingForClarification() bool { return s.state == stateAskingClarification }
func (s *Session) IsActive() bool                { return s.state != stateIdle }
func (s *Session) HasFile() bool                 { return s.LastSelectedFile != "" }

/* =======================
   TRANSICIONES DE ESTADO
   Cada método es la única forma de entrar a ese estado.
======================= */

// SetFileResults: búsqueda terminó, mostrar lista al usuario
func (s *Session) SetFileResults(results []SessionFile) {
	s.FileResults = results
	s.state = stateFileSelect
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
}

// SelectFile: usuario eligió número → archivo en foco, esperar acción
func (s *Session) SelectFile(index int) (string, bool) {
	if index < 1 || index > len(s.FileResults) {
		return "", false
	}
	s.LastSelectedFile = s.FileResults[index-1].Path
	s.FileResults = nil // liberar memoria
	s.state = stateFileAction
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
	return s.LastSelectedFile, true
}

// FocusFile: poner archivo en foco directamente (resultado único de búsqueda)
func (s *Session) FocusFile(path string) {
	s.LastSelectedFile = path
	s.state = stateFileAction
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
}

// SetRenaming: esperando que el usuario diga el nombre nuevo
func (s *Session) SetRenaming() {
	s.state = stateRenaming
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
}

// SetLearning: esperando que el usuario enseñe un comando nuevo
func (s *Session) SetLearning(input string) {
	s.LearningInput = input
	s.state = stateLearning
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
}

// SetWaitingVolume: se preguntó "¿A cuánto lo pongo?" — el siguiente input
// (si es un número 0-100) debe interpretarse como el porcentaje, no volver
// a pasar por el clasificador completo. Sin este estado, "a 50" dicho como
// respuesta caía como comando nuevo sin contexto y no se entendía.
func (s *Session) SetWaitingVolume() {
	s.state = stateVolumeSet
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
}

// ClearVolumeWait: se resolvió (o se abandonó) la pregunta del porcentaje
func (s *Session) ClearVolumeWait() {
	s.state = stateIdle
	s.setVoiceMode("wake")
}

// SetAskingClarification: B96.39 clarification movement acaba de hacer la
// pregunta — guardar el objetivo original y los supuestos que la
// motivaron para que el siguiente turno fusione en vez de reanalizar
// desde cero. Mismo patrón que SetWaitingVolume/SetLearning.
func (s *Session) SetAskingClarification(originalGoal string, assumptions []string) {
	s.pendingClarification = &PendingClarification{
		OriginalGoal:       originalGoal,
		PendingAssumptions: assumptions,
		Timestamp:          time.Now(),
	}
	s.state = stateAskingClarification
	s.stateSetAt = time.Now()
	s.setVoiceMode("direct")
}

// GetPendingClarification: lo que se guardó en SetAskingClarification.
// nil si no hay ninguna pendiente — el llamador debe chequear
// WaitingForClarification() antes (mismo criterio defensivo que el
// resto del paquete).
func (s *Session) GetPendingClarification() *PendingClarification {
	return s.pendingClarification
}

// ClearClarification: la aclaración ya se usó (fusionada y reanalizada)
// o se abandonó (corrección/cancelación) — volver a idle y activar
// skipNextAsk para que ESE turno de reanálisis no pueda reabrir el
// mismo bucle.
func (s *Session) ClearClarification() {
	s.pendingClarification = nil
	s.state = stateIdle
	s.skipNextAsk = true
	s.setVoiceMode("wake")
}

// ConsumeSkipNextAsk: se lee UNA sola vez y se apaga sola — evita que
// una ruta legacy vuelva a disparar la misma aclaración sobre el objetivo recién
// fusionado. Cualquier turno posterior (ya sin relación con esta
// clarificación) vuelve a poder preguntar con normalidad.
func (s *Session) ConsumeSkipNextAsk() bool {
	if !s.skipNextAsk {
		return false
	}
	s.skipNextAsk = false
	return true
}

// RefreshTimeout: reiniciar contador (llamar después de que el TTS termina)
func (s *Session) RefreshTimeout() {
	s.stateSetAt = time.Now()
}

/* =======================
   COMPLETAR ACCIONES
   La diferencia clave:
   - DoneWithAction(): volver a idle SIN borrar LastSelectedFile
     → el usuario puede seguir operando sobre el mismo archivo
   - ClearFile(): borrar el archivo del foco (mover/renombrar lo mueven de lugar)
   - Reset(): cancelación explícita, borra todo
======================= */

// DoneWithAction: acción completada, volver a idle.
// LastSelectedFile SE MANTIENE — el usuario puede hacer otra acción sobre el mismo archivo.
func (s *Session) DoneWithAction() {
	s.state = stateIdle
	s.LearningInput = ""
	s.setVoiceMode("wake")
}

// ClearFile: el archivo fue movido o renombrado — ya no existe en la ruta anterior.
// Llamar SOLO después de mover o renombrar exitosamente.
func (s *Session) ClearFile() {
	s.LastSelectedFile = ""
	s.state = stateIdle
	s.setVoiceMode("wake")
}

// ClearLearning: aprendizaje completado
func (s *Session) ClearLearning() {
	s.LearningInput = ""
	s.state = stateIdle
	s.setVoiceMode("wake")
}

// Reset: cancelación explícita por el usuario o timeout prolongado.
// Borra TODO el contexto.
func (s *Session) Reset() {
	s.state = stateIdle
	s.FileResults = nil
	s.LastSelectedFile = ""
	s.LearningInput = ""
	s.pendingClarification = nil
	s.skipNextAsk = false
	s.setVoiceMode("wake")
}

/* =======================
   TIMEOUT
======================= */

func (s *Session) IsExpired() bool {
	if s.state == stateIdle {
		return false
	}
	return time.Since(s.stateSetAt) > sessionTimeout
}

/* =======================
   CONTROL DE VOZ
======================= */

func (s *Session) setVoiceMode(mode string) {
	_ = os.WriteFile(voiceModeFile, []byte(mode), 0644)
}

/* =======================
   CONTINUIDAD DE CONVERSACIÓN
======================= */

// SetVoiceContinuation switches the voice channel to direct mode while a
// conversational continuation is structurally open. It does not create or
// own an objective; it only exposes session continuity to the channel adapter.
func (s *Session) SetVoiceContinuation() {
	if s == nil {
		return
	}
	s.setVoiceMode("direct")
}

// RestoreVoiceMode returns the channel to the normal mode represented by the
// existing Session state. An active legacy clarification/file/learning flow
// remains direct; otherwise the microphone again requires its wake word.
func (s *Session) RestoreVoiceMode() {
	if s == nil {
		return
	}
	if s.IsActive() {
		s.setVoiceMode("direct")
		return
	}
	s.setVoiceMode("wake")
}

/* =======================
   UTILIDADES
======================= */

func (s *Session) LastSelectedFileName() string {
	if s.LastSelectedFile == "" {
		return ""
	}
	return filepath.Base(s.LastSelectedFile)
}
