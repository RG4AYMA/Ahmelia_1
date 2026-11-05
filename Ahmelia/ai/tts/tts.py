# -*- coding: utf-8 -*-
import sys
import io

# Forzar UTF-8 en Windows
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

import pyttsx3
import time
import os

# Archivo para señalar que TTS está activo
TTS_ACTIVE_FILE = "memory/tts_active.txt"

def hablar(texto):
    """Habla el texto usando pyttsx3"""
    if not texto:
        return
    
    try:
        # Inicializar engine
        engine = pyttsx3.init()
        
        # Configuración
        engine.setProperty("rate", 175)  # Velocidad (palabras por minuto)
        
        # Intentar usar voces en español
        voces = engine.getProperty("voices")
        for voz in voces:
            if "spanish" in voz.languages[0].lower() or "español" in voz.name.lower():
                engine.setProperty("voice", voz.id)
                break
        
        # Hablar
        engine.say(texto)
        engine.runAndWait()
        
    except Exception as e:
        print(f"[TTS ERROR] {e}", file=sys.stderr)
        sys.exit(1)

def main():
    # El texto viene como argumento desde Go
    if len(sys.argv) < 2:
        sys.exit(0)
    
    texto = " ".join(sys.argv[1:])
    
    # DEBUG
    print(f"[TTS DEBUG] Hablando: {texto[:50]}...", file=sys.stderr)
    
    # Hablar
    hablar(texto)
    
    print(f"[TTS DEBUG] ✅ Completado", file=sys.stderr)

if __name__ == "__main__":
    main()
