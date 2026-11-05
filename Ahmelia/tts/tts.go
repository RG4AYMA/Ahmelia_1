package tts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const ttsActiveFile = "memory/tts_active.txt"
const turnActiveFile = "memory/voice_turn_active.txt"

func ttsScriptPath() string {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		// runtime.Caller apunta a <project>/tts/tts.go; el script vive en
		// <project>/ai/tts/tts.py. Así el path no depende del cwd de go test,
		// Wails ni del ejecutable.
		root := filepath.Dir(filepath.Dir(file))
		return filepath.Join(root, "ai", "tts", "tts.py")
	}
	return filepath.Join("ai", "tts", "tts.py")
}

// charsPerSec: velocidad de pyttsx3 a rate=175 palabras/min (~5 chars/palabra)
const charsPerSec = 14.6

// bufferSec: margen extra después de que el audio termina físicamente en el auricular
const bufferSec = 0.6

var activityMu sync.Mutex
var speechMu sync.Mutex
var activeSpeech int
var activeTurns int
var speechIdle = closedChannel()

func closedChannel() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

/* =======================
   SEÑAL DE ACTIVIDAD
   Escribe la duración estimada del audio en el archivo.
   voice.py la lee y espera exactamente ese tiempo antes de escuchar.
======================= */

func setActive(text string) {
	activityMu.Lock()
	defer activityMu.Unlock()
	if activeSpeech == 0 {
		speechIdle = make(chan struct{})
	}
	activeSpeech++
	// The cross-process signal represents the lifecycle of the whole speech
	// queue, not an individual utterance. Its numeric payload is retained for
	// compatibility; voice.py only needs the file to exist until the queue is
	// empty.
	if activeSpeech == 1 {
		duration := float64(len(text))/charsPerSec + bufferSec
		_ = os.WriteFile(ttsActiveFile, []byte(fmt.Sprintf("%.2f", duration)), 0644)
	}
}

func setInactive() {
	activityMu.Lock()
	defer activityMu.Unlock()
	if activeSpeech > 0 {
		activeSpeech--
	}
	if activeSpeech == 0 {
		_ = os.Remove(ttsActiveFile)
		close(speechIdle)
	}
}

// BeginVoiceTurn marks the whole input-to-response lifecycle for the
// cross-process microphone adapter. TTS-only signaling is insufficient while
// cognition is still processing and has not started speaking yet.
func BeginVoiceTurn() error {
	activityMu.Lock()
	defer activityMu.Unlock()
	if activeTurns == 0 {
		if err := os.WriteFile(turnActiveFile, []byte("active"), 0644); err != nil {
			return err
		}
	}
	activeTurns++
	return nil
}

// EndVoiceTurn clears one reserved input lifecycle. Multiple voice turns may
// already be queued in the host channel, so the marker remains until all are
// processed and their deferred releases have run.
func EndVoiceTurn() {
	activityMu.Lock()
	defer activityMu.Unlock()
	if activeTurns > 0 {
		activeTurns--
	}
	if activeTurns == 0 {
		_ = os.Remove(turnActiveFile)
	}
}

// WaitUntilIdle blocks until every queued/active speech utterance has fully
// left the physical speech lifecycle. It is intentionally separate from
// SpeakSync: callers only need the boundary, not another utterance.
func WaitUntilIdle() {
	activityMu.Lock()
	ch := speechIdle
	activityMu.Unlock()
	<-ch
}

/* =======================
   LIMPIEZA DE TEXTO
======================= */

func clean(text string) string {
	text = strings.ReplaceAll(text, `"`, "")
	text = strings.ReplaceAll(text, `'`, "")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	return strings.TrimSpace(text)
}

/* =======================
   SPEAK SYNC — bloquea hasta que el audio termina
======================= */

func SpeakSync(text string) {
	if text == "" || os.Getenv("AHMELIA_DISABLE_TTS") == "1" {
		return
	}
	text = clean(text)
	speechMu.Lock()
	defer speechMu.Unlock()
	setActive(text)
	defer setInactive()

	cmd := exec.Command("python", ttsScriptPath(), text)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "TTS: %v — %s\n", err, strings.TrimSpace(string(out)))
	}
}

/* =======================
   SPEAK ASYNC — no bloquea
======================= */

func Speak(text string) {
	if text == "" || os.Getenv("AHMELIA_DISABLE_TTS") == "1" {
		return
	}

	text = clean(text)
	// Reserve the physical speech channel before launching the goroutine. This
	// makes the lifecycle signal visible to voice.py immediately and keeps all
	// queued utterances behind the same mutex.
	setActive(text)
	go func() {
		speechMu.Lock()
		defer speechMu.Unlock()
		defer setInactive()

		cmd := exec.Command("python", ttsScriptPath(), text)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TTS: %v\n", err)
		}
	}()
}

/* =======================
   FRASES PREDEFINIDAS
======================= */

func SaludoInicial()                  { Speak("Hola, soy Ahmelia. Estoy lista para ayudarte") }
func AbrirPrograma(nombre string)     { Speak("Abriendo " + nombre) }
func Pregunta(text string)            { Speak(text) }
func Aprendido()                      { Speak("Entendido, lo recordaré") }
func Despedida()                      { SpeakSync("Hasta luego") }
func NoEntiendo()                     { Speak("No entendí eso") }
func Confirmacion()                   { Speak("Listo") }
func BuscandoArchivo(nombre string)   { Speak("Buscando " + nombre) }
func ArchivoEncontrado(nombre string) { Speak("Encontré " + nombre) }
func ArchivoNoEncontrado()            { Speak("No encontré ese archivo") }
