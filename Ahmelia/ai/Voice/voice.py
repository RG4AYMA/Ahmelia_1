# -*- coding: utf-8 -*-
import sys
import json
import io

# Forzar UTF-8 en Windows
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

import speech_recognition as sr
import numpy as np
import time
import os
import re
import difflib

from turn_aggregator import capture_turn, join_turn

# DEBE ir antes de importar faster_whisper/huggingface_hub: en Windows,
# sin Modo de Desarrollador activado o permisos de administrador, crear
# symlinks falla (WinError 1314 - "El cliente no dispone de un privilegio
# requerido"). Esto le dice a huggingface_hub que copie los archivos del
# modelo en vez de enlazarlos — usa un poco más de disco, pero funciona
# sin permisos especiales.
os.environ["HF_HUB_DISABLE_SYMLINKS"] = "1"

from faster_whisper import WhisperModel

# ============================================================================
# CONFIG
# ============================================================================

# Variantes del wake word — se usan también para matching difuso (ver
# fuzzy_wake_match), así que no hace falta listar TODAS las formas en que
# Whisper podría transcribirlas mal: con Levenshtein alcanza.
WAKE_WORDS = ["ahmelia", "amelia", "a melia", "jamelia", "hamelia"]

# Qué tan parecida tiene que ser la primera palabra reconocida a algún wake
# word para aceptarla igual (0.0 = cualquier cosa vale, 1.0 = exacto).
# 0.6 tolera 1-2 letras distintas en una palabra de 6-7 letras ("amalia",
# "emilia" todavía califican; "estrella" no).
WAKE_WORD_FUZZY_THRESHOLD = 0.72

# Modelo de Whisper: "tiny" (~150-200MB, rápido, menos preciso) / "base"
# / "small" (mejor precisión, en GPU es rápido y ya no pesa en la RAM
# del sistema como pesaba en CPU) / "medium" (más preciso todavía, con
# GPU sigue siendo viable). Con GPU disponible, "small" pasó a ser el
# punto por defecto — el costo que antes era RAM del sistema ahora es
# VRAM de la GPU.
WHISPER_MODEL_SIZE = "small"

# Con GPU NVIDIA disponible: device="cuda". El compute_type queda en
# "int8" (no "float16") a propósito — float16 en CTranslate2 necesita
# GPU con compute capability >= 7.0 (Turing en adelante); Pascal (GTX
# 10xx, incluida la 1050 Ti, compute capability 6.1) no lo soporta bien
# y puede fallar al cargar o rendir peor que int8. int8 anda en
# cualquier GPU CUDA y en Pascal es la opción rápida real.
WHISPER_DEVICE = "cuda"
WHISPER_COMPUTE_TYPE = "int8"

# Ruta relativa al mismo archivo que memory/session.go escribe
# (voiceModeFile) cada vez que Session cambia de estado: "direct" mientras
# hay una pregunta pendiente (WaitingForVolumeSet, WaitingForLearning,
# WaitingForRenaming, etc.), "wake" en idle. Antes nada del lado de Python
# leía este archivo — Go lo escribía religiosamente pero se perdía, así
# que toda respuesta a una pregunta ("¿A cuánto lo pongo?", "¿Qué quieres
# enseñarme?") tenía que repetir el wake word igual que un comando nuevo.
VOICE_MODE_FILE = "memory/voice_mode.txt"

# Vocabulario conocido (de environment/friendly.go) para sesgar a Whisper
# hacia estos nombres en vez de que intente "corregir" VLC, Unity, etc.
# hacia palabras de diccionario general. Si agregas apps nuevas a
# friendly.go, agrégalas también acá.
KNOWN_VOCAB = [
    "Amelia", "Ahmelia",
    "Chrome", "Edge", "Firefox", "Opera", "Brave",
    "Visual Studio Code", "Visual Studio", "Unity", "Unity Hub", "Blender",
    "PyCharm", "IntelliJ IDEA",
    "Steam", "Epic Games", "GOG Galaxy", "Battle.net", "Origin", "EA App",
    "Ubisoft Connect",
    "VLC", "Spotify", "OBS Studio",
    "Word", "Excel", "PowerPoint", "Acrobat", "Adobe Reader",
    "WinRAR", "7-Zip", "Discord", "PowerShell",
    # Sitios/servicios web que se abren como URL (shared.URLKeywords en
    # Go) — nombres en inglés o siglas que un modelo en español puede
    # "traducir" mal si no los ve como vocabulario esperado.
    "ChatGPT", "YouTube", "GitHub", "Gmail", "WhatsApp", "TikTok",
    "LinkedIn", "Wikipedia", "Stack Overflow", "Claude",
]

INITIAL_PROMPT = (
    "Comandos de voz en español para un asistente llamado Amelia. "
    "Aplicaciones mencionadas: " + ", ".join(KNOWN_VOCAB) + ". "
    "Ejemplos: Amelia abre VLC. Amelia qué tienes abierto. "
    "Amelia sube el volumen. Amelia cambia a Chrome. Amelia salir. "
    "Amelia abre ChatGPT."
)

# Archivo para detectar si TTS está activo
TTS_ACTIVE_FILE = "memory/tts_active.txt"

# ============================================================================
# LOGGING
# ============================================================================

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


# ============================================================================
# WHISPER — cargado UNA sola vez al arrancar, no en cada escucha (cargar el
# modelo tarda varios segundos; hacerlo por comando sería insoportable)
# ============================================================================

def _load_model(device, compute_type):
    return WhisperModel(WHISPER_MODEL_SIZE, device=device, compute_type=compute_type)


log(f"🧠 Cargando modelo Whisper ({WHISPER_MODEL_SIZE}, device={WHISPER_DEVICE})...")
try:
    _model = _load_model(WHISPER_DEVICE, WHISPER_COMPUTE_TYPE)
    _on_gpu = (WHISPER_DEVICE == "cuda")
    log("✅ Modelo Whisper listo (GPU)")
except Exception as e:
    # Causa más común: falta CUDA Toolkit / cuDNN instalados aparte de
    # la librería de Python, o la GPU está ocupada/no disponible en
    # este momento. En vez de tumbar todo el programa, cae a CPU con
    # int8 — más lento, pero Ahmelia sigue funcionando.
    log(f"⚠️  No se pudo cargar Whisper en GPU ({e}). Cayendo a CPU...")
    _model = _load_model("cpu", "int8")
    _on_gpu = False
    log("✅ Modelo Whisper listo (CPU, fallback)")


def audio_to_float32(audio: sr.AudioData) -> np.ndarray:
    """Convierte AudioData de speech_recognition a un array float32 mono
    a 16kHz — el formato que espera Whisper. get_raw_data con
    convert_rate/convert_width hace el resample y la conversión de ancho
    de muestra por nosotros, sin depender de que el micrófono ya capture
    justo a 16kHz."""
    raw = audio.get_raw_data(convert_rate=16000, convert_width=2)  # PCM16 mono 16kHz
    samples = np.frombuffer(raw, dtype=np.int16).astype(np.float32)
    return samples / 32768.0


def read_voice_mode():
    """Lee el modo actual escrito por Go (memory/session.go setVoiceMode).
    "direct" = hay una pregunta pendiente (WaitingForVolumeSet,
    WaitingForLearning, WaitingForRenaming, WaitingForFileAction...) y la
    siguiente respuesta del usuario no necesita wake word. Ante cualquier
    problema leyendo el archivo (no existe todavía, carrera con la
    escritura, etc.) se asume "wake" — el modo seguro, nunca al revés."""
    try:
        with open(VOICE_MODE_FILE, "r", encoding="utf-8") as f:
            return f.read().strip()
    except Exception:
        return "wake"


def fuzzy_wake_match(first_words: str):
    """Compara las primeras palabras reconocidas contra WAKE_WORDS con
    tolerancia a errores de transcripción (Levenshtein vía difflib),
    no solo substring exacto. Devuelve el wake word que matcheó, o None.
    Esto es la red de seguridad para cuando Whisper igual se equivoca
    (menos seguido que Google, pero puede pasar)."""
    for wake in WAKE_WORDS:
        ratio = difflib.SequenceMatcher(None, first_words, wake).ratio()
        if ratio >= WAKE_WORD_FUZZY_THRESHOLD:
            return wake
    return None


def extract_command(texto: str):
    """Busca el wake word (exacto primero, difuso como respaldo) y
    devuelve lo que queda como comando. Devuelve (wake_word, comando) o
    (None, None) si no hay wake word."""
    texto = texto.strip()
    if not texto:
        return None, None

    # 1) Match exacto en cualquier parte del texto (rápido, caso común)
    for wake in WAKE_WORDS:
        if wake in texto:
            comando = texto.replace(wake, "", 1).strip()
            return wake, comando

    # 2) Respaldo difuso: solo miramos la PRIMERA palabra/las dos
    # primeras palabras (el wake word casi siempre abre la frase). No
    # aplicamos fuzzy matching a la frase completa porque compararía
    # mal contra strings largos y daría falsos positivos.
    palabras = texto.split()
    for n in (1, 2):
        if len(palabras) >= n:
            candidato = " ".join(palabras[:n])
            wake = fuzzy_wake_match(candidato)
            if wake:
                comando = " ".join(palabras[n:]).strip()
                return wake, comando

    return None, None


def transcribe_audio(samples):
    """Transcribe con el modelo actual. Si falla por un problema de GPU
    en tiempo real (ej. falta cublas64_12.dll — pasa aunque la carga
    inicial del modelo haya funcionado, porque esa carga no siempre
    toca la GPU de verdad, la primera transcripción sí), recarga el
    modelo en CPU UNA vez y reintenta ahí. Sin esto, una excepción
    nativa de CUDA sin manejar puede tumbar todo el proceso de voz."""
    global _model, _on_gpu

    try:
        segments, _info = _model.transcribe(
            samples,
            language="es",
            initial_prompt=INITIAL_PROMPT,
            vad_filter=True,  # recorta silencio, ayuda con audio corto
        )
        return "".join(seg.text for seg in segments).strip().lower()
    except Exception as e:
        if not _on_gpu:
            raise  # ya estamos en CPU, no hay a dónde más bajar
        log(f"⚠️  Falló la transcripción en GPU ({e}). Recargando en CPU...")
        _model = _load_model("cpu", "int8")
        _on_gpu = False
        log("✅ Modelo Whisper recargado en CPU — reintentando este comando")
        segments, _info = _model.transcribe(
            samples,
            language="es",
            initial_prompt=INITIAL_PROMPT,
            vad_filter=True,
        )
        return "".join(seg.text for seg in segments).strip().lower()


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
            mic = sr.Microphone(device_index=0, sample_rate=16000)
        except Exception:
            mic = sr.Microphone(sample_rate=16000)

        with mic as source:
            log("🎤 Escuchando...")
            r.adjust_for_ambient_noise(source, duration=0.5)

            def transcribe(audio):
                samples = audio_to_float32(audio)
                texto = transcribe_audio(samples).strip()
                return re.sub(r"[¿?¡!,.;:]", "", texto).strip()

            try:
                segmentos = capture_turn(r, source, transcribe)
            except Exception as e:
                log(f"❌ Error de transcripción: {e}")
                return ""

        texto = join_turn(segmentos)
        if not texto:
            log("❌ No entendí")
            return ""

        log(f"📝 Turno reconocido: '{texto}'")
        wake, comando = extract_command(texto)

        if wake:
            log(f"✅ Wake word detectado: '{wake}' -> Comando: '{comando}'")
            emit_turn(comando)
            wait_for_tts_to_finish()
            return comando

        if read_voice_mode() == "direct":
            log(f"↪️  Modo directo (pregunta pendiente) -> Turno: '{texto}'")
            emit_turn(texto)
            wait_for_tts_to_finish()
            return texto

        return ""

    except Exception as e:
        log(f"❌ Error micrófono: {e}")
        return ""


# LOOP PRINCIPAL - SOLO STDERR PARA LOGS
if __name__ == "__main__":
    log("=== AHMELIA VOICE ACTIVADO (Whisper local) ===")
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
