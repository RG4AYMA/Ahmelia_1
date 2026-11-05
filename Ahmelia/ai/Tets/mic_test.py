import speech_recognition as sr
import sys

WAKE_WORD = "ahmelia"

print("=== TEST DE VOZ PARA AHMELIA ===", file=sys.stderr)
print("Di 'Ahmelia hola' para probar", file=sys.stderr)
print("", file=sys.stderr)

r = sr.Recognizer()
mic = sr.Microphone()

while True:
    try:
        with mic as source:
            print("🎤 Escuchando...", file=sys.stderr)
            r.adjust_for_ambient_noise(source, duration=0.5)
            audio = r.listen(source, timeout=10, phrase_time_limit=10)
        
        print("🔄 Procesando...", file=sys.stderr)
        texto = r.recognize_google(audio, language="es-CO").lower()
        print(f"📝 Reconocido: '{texto}'", file=sys.stderr)
        
        if WAKE_WORD in texto:
            comando = texto.replace(WAKE_WORD, "").strip()
            print(f"✅ Comando válido: '{comando}'", file=sys.stderr)
            # Esta línea es la que Go debe recibir
            print(comando)
            sys.stdout.flush()
        else:
            print(f"⚠️ No contiene wake word '{WAKE_WORD}'", file=sys.stderr)
            
    except sr.WaitTimeoutError:
        print("⏱️ Timeout - no se detectó voz", file=sys.stderr)
    except sr.UnknownValueError:
        print("❌ No entendí lo que dijiste", file=sys.stderr)
    except sr.RequestError as e:
        print(f"❌ Error de servicio: {e}", file=sys.stderr)
    except KeyboardInterrupt:
        print("\n👋 Saliendo...", file=sys.stderr)
        break