"""
volume.py — Control de volumen del sistema para Ahmelia.
Llamado desde control.go: python ai/volume.py <cmd> [value]

Comandos:
  set <0-100>   Volumen absoluto
  up            +10%
  down          -10%
  mute          Silenciar
  unmute        Quitar silencio
  max           Volumen al 100%
"""

import sys
import os

# ── Intentar pycaw (más fiable, sin dependencias de UI) ─────────────────────
def get_volume_control():
    from pycaw.pycaw import AudioUtilities, IAudioEndpointVolume
    from ctypes import cast, POINTER
    from comtypes import CLSCTX_ALL
    devices = AudioUtilities.GetSpeakers()
    interface = devices.Activate(IAudioEndpointVolume._iid_, CLSCTX_ALL, None)
    return cast(interface, POINTER(IAudioEndpointVolume))


def run_pycaw(cmd, value=None):
    vol = get_volume_control()
    if cmd == "set":
        scalar = max(0.0, min(1.0, float(value) / 100.0))
        vol.SetMasterVolumeLevelScalar(scalar, None)
    elif cmd == "up":
        current = vol.GetMasterVolumeLevelScalar()
        vol.SetMasterVolumeLevelScalar(min(1.0, current + 0.10), None)
    elif cmd == "down":
        current = vol.GetMasterVolumeLevelScalar()
        vol.SetMasterVolumeLevelScalar(max(0.0, current - 0.10), None)
    elif cmd == "mute":
        vol.SetMute(1, None)
    elif cmd == "unmute":
        vol.SetMute(0, None)
    elif cmd == "max":
        vol.SetMasterVolumeLevelScalar(1.0, None)


# ── Fallback nircmd ──────────────────────────────────────────────────────────
STEP = 6553   # ~10% de 65535
MAX  = 65535

def run_nircmd(cmd, value=None):
    import subprocess
    if cmd == "set":
        raw = int(float(value) / 100.0 * MAX)
        subprocess.run(["nircmd.exe", "setsysvolume", str(raw)], check=True)
    elif cmd == "up":
        subprocess.run(["nircmd.exe", "changesysvolume", str(STEP)], check=True)
    elif cmd == "down":
        subprocess.run(["nircmd.exe", "changesysvolume", str(-STEP)], check=True)
    elif cmd == "mute":
        subprocess.run(["nircmd.exe", "mutesysvolume", "1"], check=True)
    elif cmd == "unmute":
        subprocess.run(["nircmd.exe", "mutesysvolume", "0"], check=True)
    elif cmd == "max":
        subprocess.run(["nircmd.exe", "setsysvolume", str(MAX)], check=True)


# ── Fallback PowerShell (Win10, sin herramientas externas) ──────────────────
def run_powershell(cmd, value=None):
    import subprocess

    # Mapa de teclas de volumen (VK codes)
    # 0xAF = VolumeUp, 0xAE = VolumeDown, 0xAD = VolumeMute
    key_map = {
        "up":     "[System.Windows.Forms.SendKeys]::SendWait([char]0xAF)",
        "down":   "[System.Windows.Forms.SendKeys]::SendWait([char]0xAE)",
        "mute":   "[System.Windows.Forms.SendKeys]::SendWait([char]0xAD)",
        "unmute": "[System.Windows.Forms.SendKeys]::SendWait([char]0xAD)",
    }

    if cmd in key_map:
        script = f"Add-Type -AssemblyName System.Windows.Forms; {key_map[cmd]}"
        subprocess.run(["powershell", "-NoProfile", "-NonInteractive", "-Command", script], check=True)
    elif cmd in ("set", "max"):
        # Para set/max sin pycaw ni nircmd: primero bajar al mínimo, luego subir
        pct = int(value) if value else 100
        # Bajar 50 pasos, luego subir los que corresponden
        steps_up = round(pct / 2)  # cada pulsación ≈ 2%
        script = (
            "Add-Type -AssemblyName System.Windows.Forms; "
            f"for($i=0;$i -lt 50;$i++){{[System.Windows.Forms.SendKeys]::SendWait([char]0xAE)}}; "
            f"for($i=0;$i -lt {steps_up};$i++){{[System.Windows.Forms.SendKeys]::SendWait([char]0xAF)}}"
        )
        subprocess.run(["powershell", "-NoProfile", "-NonInteractive", "-Command", script], check=True)


# ── Entrada principal ────────────────────────────────────────────────────────
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Uso: volume.py <set|up|down|mute|unmute|max> [value]")
        sys.exit(1)

    cmd   = sys.argv[1].lower()
    value = sys.argv[2] if len(sys.argv) > 2 else None

    # Intentar métodos en orden de fiabilidad
    for method in [run_pycaw, run_nircmd, run_powershell]:
        try:
            method(cmd, value)
            sys.exit(0)          # éxito — salir limpiamente
        except Exception:
            continue             # falló — intentar el siguiente

    print(f"ERROR: No pude controlar el volumen ({cmd})", file=sys.stderr)
    sys.exit(1)
