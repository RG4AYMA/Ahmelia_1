# -*- coding: utf-8 -*-
import sys
import io

# Forzar UTF-8 en Windows
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

import speech_recognition as sr
import time
import os
import difflib
import json

from turn_aggregator import capture_turn, join_turn

# ============================================================================
# CONFIG
# ============================================================================
#
# NOTA (solución provisional mientras se resuelve el problema de fuente de
# alimentación de la GPU): esta versión usa la API gratuita de Google
# (recognize_google, vía speech_recognition) en vez de faster-whisper local
# — no necesita GPU ni carga un modelo en RAM, así que la PC va más liviana.
# A cambio, se pierde el sesgo de vocabulario que sí tenía voice.py
# (KNOWN_VOCAB/INITIAL_PROMPT, pensado para que no "corrija" nombres como
# Unity/VLC/ChatGPT hacia palabras de diccionario) — recognize_google no
# admite ese tipo de hint. También necesita internet siempre, y es un
# endpoint no oficial de Google (sin garantía de límites de uso ni de que
# siga funcionando igual a futuro). Decisión consciente, no un descuido.

# Variantes del wake word — mismas que voice.py, y se usan igual para
# matching difuso (ver fuzzy_wake_match), así que no hace falta listar
# todas las formas en que se podría transcribir mal.
WAKE_WORDS = ["ahmelia", "amelia", "a melia", "jamelia", "hamelia"]

# Qué tan parecida tiene que ser la primera palabra reconocida a algún wake
# word para aceptarla igual (0.0 = cualquier cosa vale, 1.0 = exacto).
WAKE_WORD_FUZZY_THRESHOLD = 0.72

# Ruta relativa al mismo archivo que memory/session.go escribe (voiceModeFile)
# cada vez que Session cambia de estado: "direct" mientras hay una pregunta
# pendiente (WaitingForVolumeSet, WaitingForLearning, WaitingForRenaming,
# etc.), "wake" en idle. Sin esto, cada respuesta a una pregunta ("¿A cuánto
# lo pongo?", "¿Qué quieres enseñarme?") necesitaría repetir el wake word
# igual que un comando nuevo — ver el mismo mecanismo en voice.py.
VOICE_MODE_FILE = "memory/voice_mode.txt"

# Archivo para detectar si TTS está activo
TTS_ACTIVE_FILE = "memory/tts_active.txt"
VOICE_TURN_ACTIVE_FILE = "memory/voice_turn_active.txt"


def emit_turn(text):
    """Emit exactly one transport frame; embedded newlines stay inside JSON."""
    text = text.strip()
    if text:
        print(json.dumps({"type": "turn", "text": text}, ensure_ascii=False), flush=True)


def log(msg):
    """Imprime a stderr (Go ignora esto)"""
    print(msg, file=sys.stderr, flush=True)


def wait_for_tts_to_finish():
    """Espera a que el TTS termine antes de escuchar"""
    waited = False
    for _ in range(20):  # máx 2 segundos esperando que aparezca
        if os.path.exists(TTS_ACTIVE_FILE):
            waited = True
            break
        time.sleep(0.1)

    if not waited:
        return  # TTS no estaba activo

    while os.path.exists(TTS_ACTIVE_FILE):
        time.sleep(0.05)  # polling cada 50ms

    # Buffer de silencio después del TTS para que no capture el final
    time.sleep(0.8)


def wait_for_turn_response():
    """Do not reopen the microphone while Go is still processing this turn.

    The TTS marker appears only when speech starts. Without a separate turn
    lifecycle marker, a slow cognitive response leaves a gap in which this
    always-on recognizer can capture Ahmelia's eventual TTS as user input.
    """
    deadline = time.monotonic() + 10.0
    while (not os.path.exists(VOICE_TURN_ACTIVE_FILE)
           and not os.path.exists(TTS_ACTIVE_FILE)
           and time.monotonic() < deadline):
        time.sleep(0.05)

    while (os.path.exists(VOICE_TURN_ACTIVE_FILE)
           or os.path.exists(TTS_ACTIVE_FILE)):
        time.sleep(0.05)

    time.sleep(0.8)


def read_voice_mode():
    """Lee el modo actual escrito por Go (memory/session.go setVoiceMode).
    "direct" = hay una pregunta pendiente y la siguiente respuesta del
    usuario no necesita wake word. Ante cualquier problema leyendo el
    archivo (no existe todavía, carrera con la escritura, etc.) se asume
    "wake" — el modo seguro, nunca al revés."""
    try:
        with open(VOICE_MODE_FILE, "r", encoding="utf-8") as f:
            return f.read().strip()
    except Exception:
        return "wake"


def fuzzy_wake_match(first_words: str):
    """Compara las primeras palabras reconocidas contra WAKE_WORDS con
    tolerancia a errores de transcripción (Levenshtein vía difflib), no
    solo substring exacto. Devuelve el wake word que matcheó, o None."""
    for wake in WAKE_WORDS:
        ratio = difflib.SequenceMatcher(None, first_words, wake).ratio()
        if ratio >= WAKE_WORD_FUZZY_THRESHOLD:
            return wake
    return None


def extract_command(texto: str):
    """Busca el wake word (exacto primero, difuso como respaldo) y devuelve
    lo que queda como comando. Devuelve (wake_word, comando) o (None, None)
    si no hay wake word."""
    texto = texto.strip()
    if not texto:
        return None, None

    # 1) Match exacto en cualquier parte del texto (rápido, caso común)
    for wake in WAKE_WORDS:
        if wake in texto:
            comando = texto.replace(wake, "", 1).strip()
            return wake, comando

    # 2) Respaldo difuso: solo miramos la primera palabra/las dos primeras
    # (el wake word casi siempre abre la frase).
    palabras = texto.split()
    for n in (1, 2):
        if len(palabras) >= n:
            candidato = " ".join(palabras[:n])
            wake = fuzzy_wake_match(candidato)
            if wake:
                comando = " ".join(palabras[n:]).strip()
                return wake, comando

    return None, None


def escuchar():
    """Escucha un turno conversacional completo, no una sola frase."""
    r = sr.Recognizer()
    # Let turn_aggregator own the boundary; avoid a short recognizer phrase
    # limit manufacturing artificial turns during long user objectives.
    r.pause_threshold = 0.9
    r.non_speaking_duration = 0.6
    r.phrase_threshold = 0.25

    try:
        try:
            mic = sr.Microphone(device_index=0)
        except Exception:
            mic = sr.Microphone()

        with mic as source:
            log("🎤 Escuchando...")
            r.adjust_for_ambient_noise(source, duration=0.5)

            def transcribe(audio):
                try:
                    return r.recognize_google(audio, language="es-ES").strip().lower()
                except sr.UnknownValueError:
                    return ""
                except sr.RequestError as e:
                    raise RuntimeError(f"Error de conexión: {e}")

            try:
                segmentos = capture_turn(r, source, transcribe)
            except RuntimeError as e:
                log(f"❌ {e}")
                return ""

        texto = join_turn(segmentos)
        if not texto:
            log("❌ No entendí")
            return ""

        log(f"📝 Turno reconocido: '{texto}'")
        wake, comando = extract_command(texto)

        if wake:
            log(f"✅ Wake word detectado: '{wake}' -> Comando: '{comando}'")
            wait_for_tts_to_finish()
            emit_turn(comando)
            wait_for_turn_response()
            return comando

        if read_voice_mode() == "direct":
            log(f"↪️  Modo directo (pregunta pendiente) -> Turno: '{texto}'")
            wait_for_tts_to_finish()
            emit_turn(texto)
            wait_for_turn_response()
            return texto

        return ""

    except Exception as e:
        log(f"❌ Error micrófono: {e}")
        return ""


# LOOP PRINCIPAL - SOLO STDERR PARA LOGS
if __name__ == "__main__":
    log("=== AHMELIA VOICE ACTIVADO (Google Speech Recognition) ===")
    log("💡 Puedes decir: Amelia, Ahmelia, A melia")
    log("📌 Ejemplo: 'Amelia abre VLC' o 'Amelia salir'")
    log("")

    while True:
        try:
            wait_for_tts_to_finish()
            escuchar()

        except KeyboardInterrupt:
            log("\n👋 Saliendo...")
            break
        except Exception as e:
            log(f"❌ Error inesperado: {e}")
            time.sleep(1)
