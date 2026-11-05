package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

/* =======================
   VOLUMEN
   Usa nircmd.exe si está disponible (más fiable).
   Fallback: keybd_event vía P/Invoke a user32.dll — la misma técnica
   que ya usa SwitchWindow() más abajo para simular Alt+Tab.
   (El fallback anterior usaba SendKeys([char]175) etc., que NO
   presiona teclas virtuales: SendKeys interpreta esos números como
   el carácter Unicode correspondiente e intenta "escribirlo" en la
   ventana activa. Por eso nunca cambiaba el volumen de verdad aunque
   Go no reportara ningún error.)
======================= */

const (
	volumeStep        = 6553 // ~10% del rango 0-65535 (solo para nircmd)
	volumeStepPercent = 5    // paso de VolumeUp/VolumeDown en porcentaje
	volumeMax         = 65535

	vkVolumeMute = 0xAD
	vkVolumeDown = 0xAE
	vkVolumeUp   = 0xAF
)

const keybdEventTypeDef = `using System;using System.Runtime.InteropServices;public class KB{[DllImport("user32.dll")]public static extern void keybd_event(byte bVk,byte bScan,uint dwFlags,int dwExtraInfo);}`

// GetForegroundProcess y su respaldo PowerShell viven ahora en
// foreground.go (camino rápido por syscalls, 2026-10-17): el latido N0
// necesita reconciliar el foco a 1 Hz y un spawn de PowerShell (~300-500 ms)
// por lectura lo hacía imposible. Misma firma y semántica de siempre.

// windowFocusTypeDef expone las llamadas reales de Win32 para traer una
// ventana al frente: ShowWindow (por si estaba minimizada — con
// SW_RESTORE=9), SetForegroundWindow (el foco real) y keybd_event, que
// se usa para el truco de simular Alt antes de pedir el foco.
//
// Ese truco hace falta porque Windows tiene una restricción a propósito
// ("foreground lock"): un proceso en segundo plano —como Ahmelia, que
// escucha por voz sin tener el foco— normalmente NO puede robarle el
// foco a la ventana activa con un simple SetForegroundWindow. Simular
// una pulsación de Alt justo antes es el workaround estándar: hace que
// Windows trate la siguiente petición de foco como si viniera de una
// interacción del usuario.
// IsIconic se agrega para poder preguntar si la ventana está MINIMIZADA
// antes de decidir si hace falta restaurarla. Antes llamábamos siempre a
// ShowWindow(h, SW_RESTORE) sin preguntar nada — y SW_RESTORE no solo
// des-minimiza: si la ventana ya estaba MAXIMIZADA, SW_RESTORE la regresa
// a su tamaño "normal" (no maximizado), que es justo el "cambio de
// resolución" que se reportó. La ventana no cambiaba de resolución de
// pantalla; cambiaba de tamaño de ventana porque la desmaximizábamos sin
// necesidad cada vez que le dábamos foco.
// windowFocusTypeDef v3 — dos problemas combinados, no uno solo:
//
//  1. Antes buscábamos la ventana con Get-Process.MainWindowHandle. Eso
//     fallaba en silencio con el Administrador de Tareas moderno de
//     Windows 10/11: a veces no expone MainWindowHandle de la forma
//     clásica que espera Get-Process (viene alojado distinto), así que
//     $proc salía null y el bloque `if ($proc) {...}` no hacía nada —
//     pero PowerShell no lo trataba como error. Fix: enumerar las
//     ventanas REALES del sistema con EnumWindows y buscar por título
//     visible — la misma fuente que ya sabemos que encuentra el
//     Administrador de Tareas, porque tasklist /v lo encuentra por su
//     título.
//
//  2. Incluso encontrando la ventana correcta y llamando a
//     SetForegroundWindow con éxito (devuelve true), Windows puede
//     seguir negando el robo de foco en builds recientes: el truco
//     clásico de "simular una pulsación de Alt antes" (keybd_event)
//     para engañar el "foreground lock" es cada vez menos confiable —
//     depende de timing exacto y Microsoft lo ha ido restringiendo.
//     La técnica más robusta es AttachThreadInput: adjuntar la cola de
//     entrada del hilo actual (PowerShell) a la del hilo dueño de la
//     ventana que tiene el foco AHORA MISMO. Mientras están adjuntos,
//     Windows trata a ambos hilos como si fueran "el mismo" para efectos
//     de foco, así que SetForegroundWindow ya no se considera un robo de
//     foco de un proceso ajeno — y funciona de forma consistente.
const windowFocusTypeDef = `using System;using System.Text;using System.Runtime.InteropServices;
public class WinFocus{
  public delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);
  [DllImport("user32.dll")] public static extern int GetWindowTextLength(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd,int nCmdShow);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool BringWindowToTop(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint idAttach, uint idAttachTo, bool fAttach);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();

  [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr hwnd, int dwAttribute, out int pvAttribute, int cbAttribute);
  [DllImport("user32.dll")] public static extern IntPtr GetWindow(IntPtr hWnd, uint uCmd);
  const int DWMWA_CLOAKED = 14;
  const uint GW_OWNER = 4;

  static bool IsCloaked(IntPtr hWnd){
    int cloaked;
    int hr = DwmGetWindowAttribute(hWnd, DWMWA_CLOAKED, out cloaked, sizeof(int));
    return hr == 0 && cloaked != 0;
  }

  // Busca la ventana cuyo TITULO o nombre de PROCESO contenga
  // needle/procNeedle (ambos ya en minúsculas), PERO filtra candidatas
  // que IsWindowVisible marca como visibles y en realidad no lo son:
  // ventanas "cloaked" por el compositor de Windows (DWM) — algo común
  // en apps modernas (WinUI3/paquete), como el Administrador de Tareas
  // reescrito en Windows 11 — y ventanas con "owner" (no son ventanas
  // de nivel superior reales, tipo tooltips o paneles auxiliares).
  // Si no hay ninguna candidata "limpia", cae a la primera que haya
  // encontrado igual, para no quedarse sin nada.
  //
  // Devuelve un reporte de diagnóstico (no solo true/false) porque la
  // vez anterior SetForegroundWindow devolvía true sin que la ventana
  // realmente pasara al frente — así que ahora, después de pedir el
  // foco, se vuelve a preguntar con GetForegroundWindow() cuál es la
  // ventana real en foco y se compara contra la que se pidió: esa
  // comparación es la única fuente de verdad confiable.
  public static string FocusByTitleOrProcess(string needle, string procNeedle){
    // Mismos 4 niveles de prioridad que CloseByTitleOrProcess (ver su
    // comentario) — coincidencia por PROCESO real gana sobre título
    // incidental. Antes de este fix, "cambia a code" podía terminar
    // enfocando una pestaña de Chrome cuyo título mencionara "code" de
    // pura casualidad, en vez de Visual Studio Code.
    IntPtr procClean = IntPtr.Zero, titleClean = IntPtr.Zero, procAny = IntPtr.Zero, titleAny = IntPtr.Zero;
    string procCleanT = "", titleCleanT = "", procAnyT = "", titleAnyT = "";
    int candidateCount = 0;
    EnumWindows(delegate(IntPtr hWnd, IntPtr lParam){
      if (!IsWindowVisible(hWnd)) return true;
      int len = GetWindowTextLength(hWnd);
      string title = "";
      bool titleMatch = false;
      if (len > 0) {
        var sb = new StringBuilder(len + 1);
        GetWindowText(hWnd, sb, sb.Capacity);
        title = sb.ToString();
        titleMatch = title.ToLower().Contains(needle);
      }
      bool procMatch = false;
      if (procNeedle.Length > 0) {
        uint pid;
        GetWindowThreadProcessId(hWnd, out pid);
        try {
          var p = System.Diagnostics.Process.GetProcessById((int)pid);
          procMatch = p.ProcessName.ToLower().Contains(procNeedle);
        } catch { }
      }
      if (!titleMatch && !procMatch) return true;

      candidateCount++;
      bool dirty = IsCloaked(hWnd) || GetWindow(hWnd, GW_OWNER) != IntPtr.Zero;
      if (procMatch && !dirty && procClean == IntPtr.Zero) { procClean = hWnd; procCleanT = title; }
      else if (!procMatch && titleMatch && !dirty && titleClean == IntPtr.Zero) { titleClean = hWnd; titleCleanT = title; }
      else if (procMatch && procAny == IntPtr.Zero) { procAny = hWnd; procAnyT = title; }
      else if (titleAny == IntPtr.Zero) { titleAny = hWnd; titleAnyT = title; }
      return true; // seguir enumerando: queremos TODAS las candidatas, no solo la primera
    }, IntPtr.Zero);

    IntPtr found = IntPtr.Zero; string foundTitle = ""; bool viaProc = false;
    if (procClean != IntPtr.Zero) { found = procClean; foundTitle = procCleanT; viaProc = true; }
    else if (titleClean != IntPtr.Zero) { found = titleClean; foundTitle = titleCleanT; }
    else if (procAny != IntPtr.Zero) { found = procAny; foundTitle = procAnyT; viaProc = true; }
    else if (titleAny != IntPtr.Zero) { found = titleAny; foundTitle = titleAnyT; }
    if (found == IntPtr.Zero) return "NOTFOUND|candidatos=0";

    bool wasIconic = IsIconic(found);
    if (wasIconic) ShowWindow(found, 9);

    // AttachThreadInput: la técnica robusta para pedir el foco desde un
    // proceso que no lo tiene, en vez de solo simular Alt.
    IntPtr fg = GetForegroundWindow();
    uint fgThread = 0;
    if (fg != IntPtr.Zero) { GetWindowThreadProcessId(fg, out fgThread); }
    uint curThread = GetCurrentThreadId();
    bool attached = false;
    if (fgThread != 0 && fgThread != curThread) {
      attached = AttachThreadInput(curThread, fgThread, true);
    }
    BringWindowToTop(found);
    bool apiOk = SetForegroundWindow(found);
    System.Threading.Thread.Sleep(80);
    if (attached) { AttachThreadInput(curThread, fgThread, false); }

    // Verificación real: ¿la ventana en foco AHORA es la que pedimos?
    IntPtr fgAfter = GetForegroundWindow();
    bool reallyFocused = (fgAfter == found);

    string status = reallyFocused ? "OK" : "APIOK_PERO_NO_QUEDO_EN_FOCO";
    return status + "|candidatos=" + candidateCount +
      "|elegida=[" + foundTitle + "]" +
      "|via_proceso=" + viaProc +
      "|estaba_minimizada=" + wasIconic +
      "|attach_ok=" + attached +
      "|setforeground_api=" + apiOk +
      "|foco_real_despues=" + reallyFocused;
  }

  // CloseByTitleOrProcess: mismo criterio de búsqueda que
  // FocusByTitleOrProcess (título o proceso, filtrando "cloaked"/con
  // owner), pero para CERRAR — manda WM_CLOSE a la ventana encontrada
  // por NOMBRE, nunca a la que esté activa/enfocada en ese momento.
  // Antes "cierra X" ignoraba X por completo y hacía Alt+F4 sobre lo
  // que fuera la ventana activa — si esa ventana era la propia consola
  // de Ahmelia (el caso típico: el usuario habla por voz con la
  // terminal al frente), cerraba Ahmelia misma.
  const uint WM_CLOSE = 0x0010;
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hWnd, uint Msg, IntPtr wParam, IntPtr lParam);

  public static string CloseByTitleOrProcess(string needle, string procNeedle){
    // Cuatro niveles de prioridad, de mejor a peor:
    //  1) procClean: coincide por PROCESO (el dato más confiable — el
    //     título de OTRA ventana puede mencionar "code" por casualidad,
    //     pero el proceso que en verdad se llama code.exe no miente) Y
    //     está visible/no-cloaked/sin owner.
    //  2) titleClean: coincide por título, visible/limpia, pero el
    //     proceso no matchea (o no se pudo resolver procNeedle).
    //  3) procAny / 4) titleAny: mismas coincidencias pero en ventanas
    //     "sucias" (cloaked o con owner) — se usan solo si no hay nada
    //     mejor, para no quedarse sin candidato.
    // Antes esto no distinguía niveles: cualquier título que contuviera
    // needle entraba al mismo grupo que un match de proceso real, y
    // ganaba el primero que apareciera en el orden de EnumWindows (Z-
    // order) — así fue como "cierra code" cerró una pestaña de Chrome
    // titulada "...code review..." en vez de Visual Studio Code.
    IntPtr procClean = IntPtr.Zero, titleClean = IntPtr.Zero, procAny = IntPtr.Zero, titleAny = IntPtr.Zero;
    string procCleanT = "", titleCleanT = "", procAnyT = "", titleAnyT = "";
    int candidateCount = 0;
    EnumWindows(delegate(IntPtr hWnd, IntPtr lParam){
      if (!IsWindowVisible(hWnd)) return true;
      int len = GetWindowTextLength(hWnd);
      string title = "";
      bool titleMatch = false;
      if (len > 0) {
        var sb = new StringBuilder(len + 1);
        GetWindowText(hWnd, sb, sb.Capacity);
        title = sb.ToString();
        titleMatch = title.ToLower().Contains(needle);
      }
      bool procMatch = false;
      if (procNeedle.Length > 0) {
        uint pid;
        GetWindowThreadProcessId(hWnd, out pid);
        try {
          var p = System.Diagnostics.Process.GetProcessById((int)pid);
          procMatch = p.ProcessName.ToLower().Contains(procNeedle);
        } catch { }
      }
      if (!titleMatch && !procMatch) return true;

      candidateCount++;
      bool dirty = IsCloaked(hWnd) || GetWindow(hWnd, GW_OWNER) != IntPtr.Zero;
      if (procMatch && !dirty && procClean == IntPtr.Zero) { procClean = hWnd; procCleanT = title; }
      else if (!procMatch && titleMatch && !dirty && titleClean == IntPtr.Zero) { titleClean = hWnd; titleCleanT = title; }
      else if (procMatch && procAny == IntPtr.Zero) { procAny = hWnd; procAnyT = title; }
      else if (titleAny == IntPtr.Zero) { titleAny = hWnd; titleAnyT = title; }
      return true;
    }, IntPtr.Zero);

    IntPtr found = IntPtr.Zero; string foundTitle = "";
    if (procClean != IntPtr.Zero) { found = procClean; foundTitle = procCleanT; }
    else if (titleClean != IntPtr.Zero) { found = titleClean; foundTitle = titleCleanT; }
    else if (procAny != IntPtr.Zero) { found = procAny; foundTitle = procAnyT; }
    else if (titleAny != IntPtr.Zero) { found = titleAny; foundTitle = titleAnyT; }
    if (found == IntPtr.Zero) return "NOTFOUND|candidatos=0";

    // PostMessage(WM_CLOSE), no TerminateProcess: es el mismo mensaje
    // que Windows manda al hacer clic en la "X" de la ventana — la app
    // puede interceptarlo (preguntar "¿guardar cambios?"), no es un
    // kill forzado.
    bool sent = PostMessage(found, WM_CLOSE, IntPtr.Zero, IntPtr.Zero);
    return (sent ? "OK" : "POSTMESSAGE_FALLO") + "|candidatos=" + candidateCount + "|elegida=[" + foundTitle + "]";
  }
}`

// audioTypeDef define un wrapper mínimo sobre IAudioEndpointVolume (Core
// Audio API de Windows) vía COM. Esto reemplaza el método anterior de
// "adivinar" el volumen a base de pulsaciones de teclas multimedia
// (que asumía ~2% por pulsación, un número que ni siquiera es igual en
// todos los Windows) por lectura/escritura DIRECTA y exacta del volumen
// real del sistema (0-100), sin depender de que nircmd.exe esté instalado.
const audioTypeDef = `using System;using System.Runtime.InteropServices;
[Guid("5CDF2C82-841E-4546-9722-0CF74078229A"),InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IAudioEndpointVolume{int f();int g();int h();int i();int SetMasterVolumeLevelScalar(float fLevel,Guid ctx);int j();int GetMasterVolumeLevelScalar(out float pfLevel);int k();int l();int m();int n();int SetMute(bool bMute,Guid ctx);int GetMute(out bool pbMute);}
[Guid("D666063F-1587-4E43-81F1-B948E807363F"),InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IMMDevice{int Activate(ref Guid id,int clsCtx,IntPtr activationParams,out IAudioEndpointVolume aev);}
[Guid("A95664D2-9614-4F35-A746-DE8DB63617E6"),InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IMMDeviceEnumerator{int f();int GetDefaultAudioEndpoint(int dataFlow,int role,out IMMDevice endpoint);}
[ComImport,Guid("BCDE0395-E52F-467C-8E3D-C4579291692E")]
class MMDeviceEnumeratorComObject{}
public class AudioVol{
  static IAudioEndpointVolume Vol(){
    var enumerator=new MMDeviceEnumeratorComObject() as IMMDeviceEnumerator;
    IMMDevice dev=null;enumerator.GetDefaultAudioEndpoint(0,1,out dev);
    Guid iid=typeof(IAudioEndpointVolume).GUID;
    IAudioEndpointVolume epv=null;dev.Activate(ref iid,23,IntPtr.Zero,out epv);
    return epv;
  }
  public static int GetVolumePercent(){float v;Vol().GetMasterVolumeLevelScalar(out v);return (int)Math.Round(v*100);}
  public static bool GetMuted(){bool m;Vol().GetMute(out m);return m;}
  public static void SetVolumePercent(int percent){float v=percent/100f;Vol().SetMasterVolumeLevelScalar(v,Guid.Empty);}
  public static void SetMuted(bool m){Vol().SetMute(m,Guid.Empty);}
}`

// runAudioScript compila (una vez por llamada) el helper de arriba y
// ejecuta una expresión sobre él. Devuelve lo que esa expresión imprima.
func runAudioScript(expr string) (string, error) {
	script := fmt.Sprintf(`Add-Type -TypeDefinition '%s' -Language CSharp;%s`, audioTypeDef, expr)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	return strings.TrimSpace(string(out)), err
}

// getVolumePercent lee el volumen REAL actual del sistema (0-100).
func getVolumePercent() (int, error) {
	out, err := runAudioScript("[AudioVol]::GetVolumePercent()")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

// GetVolumePercent exposes the same exact Core Audio reading used by the
// volume mutators so the post-action observer reads the real Windows state.
func GetVolumePercent() (int, error) {
	return getVolumePercent()
}

// GetMuted reads the actual mute state from the Windows Core Audio endpoint.
func GetMuted() (bool, error) {
	out, err := runAudioScript("[AudioVol]::GetMuted()")
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(out), "True"), nil
}

// setVolumePercentPrecise fija el volumen exacto vía Core Audio API.
func setVolumePercentPrecise(percent int) error {
	_, err := runAudioScript(fmt.Sprintf("[AudioVol]::SetVolumePercent(%d)", percent))
	return err
}

// setMutedPrecise silencia/des-silencia vía Core Audio API.
func setMutedPrecise(muted bool) error {
	val := "false"
	if muted {
		val = "true"
	}
	_, err := runAudioScript(fmt.Sprintf("[AudioVol]::SetMuted($%s)", val))
	return err
}

// pressMediaKey simula "times" pulsaciones de la tecla multimedia vk
// (keydown + keyup) usando keybd_event, la única forma fiable de
// disparar teclas de volumen/mute sin depender de nircmd.exe.
func pressMediaKey(vk byte, times int) error {
	var presses strings.Builder
	for i := 0; i < times; i++ {
		presses.WriteString(fmt.Sprintf(
			`[KB]::keybd_event(0x%X,0,0,0);[KB]::keybd_event(0x%X,0,2,0);`, vk, vk))
	}
	script := fmt.Sprintf(`Add-Type -TypeDefinition '%s';%s`, keybdEventTypeDef, presses.String())
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Run()
}

// SetVolume establece el volumen absoluto (0-100).
func SetVolume(percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	if err := setVolumePercentPrecise(percent); err == nil {
		return nil
	}

	raw := int(float64(percent) / 100.0 * volumeMax)
	if err := exec.Command("nircmd.exe", "setsysvolume", strconv.Itoa(raw)).Run(); err == nil {
		return nil
	}

	if err := pressMediaKey(vkVolumeDown, 50); err != nil {
		return err
	}
	stepsUp := percent / 2
	return pressMediaKey(vkVolumeUp, stepsUp)
}

// VolumeUp sube el volumen exactamente volumeStepPercent (5%).
// Lee el volumen real actual y lo fija +5, en vez de asumir cuánto
// sube cada pulsación de tecla multimedia (que variaba según el equipo).
func VolumeUp() error {
	if current, err := getVolumePercent(); err == nil {
		target := current + volumeStepPercent
		if target > 100 {
			target = 100
		}
		return setVolumePercentPrecise(target)
	}
	if err := exec.Command("nircmd.exe", "changesysvolume", strconv.Itoa(volumeStep)).Run(); err == nil {
		return nil
	}
	return pressMediaKey(vkVolumeUp, 3)
}

// VolumeDown baja el volumen exactamente volumeStepPercent (5%).
func VolumeDown() error {
	if current, err := getVolumePercent(); err == nil {
		target := current - volumeStepPercent
		if target < 0 {
			target = 0
		}
		return setVolumePercentPrecise(target)
	}
	if err := exec.Command("nircmd.exe", "changesysvolume", strconv.Itoa(-volumeStep)).Run(); err == nil {
		return nil
	}
	return pressMediaKey(vkVolumeDown, 3)
}

// Mute silencia el sistema
func Mute() error {
	if err := setMutedPrecise(true); err == nil {
		return nil
	}
	if err := exec.Command("nircmd.exe", "mutesysvolume", "1").Run(); err == nil {
		return nil
	}
	return pressMediaKey(vkVolumeMute, 1)
}

// Unmute desactiva el silencio
func Unmute() error {
	if err := setMutedPrecise(false); err == nil {
		return nil
	}
	if err := exec.Command("nircmd.exe", "mutesysvolume", "0").Run(); err == nil {
		return nil
	}
	return pressMediaKey(vkVolumeMute, 1)
}

// VolumeMax sube al máximo
func VolumeMax() error {
	if err := setVolumePercentPrecise(100); err == nil {
		return nil
	}
	if err := exec.Command("nircmd.exe", "setsysvolume", strconv.Itoa(volumeMax)).Run(); err == nil {
		return nil
	}
	return pressMediaKey(vkVolumeUp, 50)
}

/* =======================
   ESCRITORIO / VENTANAS
======================= */

// MinimizeAll minimiza todas las ventanas (equivale a Win+D)
func MinimizeAll() error {
	// Shell.Application es lo más fiable en Win10
	err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(New-Object -ComObject Shell.Application).MinimizeAll()`).Run()
	if err != nil {
		// Fallback: simular Win+D
		return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			`Add-Type -AssemblyName System.Windows.Forms; `+
				`[System.Windows.Forms.SendKeys]::SendWait("^{ESC}")`).Run()
	}
	return nil
}

// CloseActiveWindow cierra la ventana activa (Alt+F4)
func CloseActiveWindow() error {
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Add-Type -AssemblyName System.Windows.Forms; `+
			`[System.Windows.Forms.SendKeys]::SendWait("%{F4}")`).Run()
}

// KillProcess termina un proceso por nombre
func KillProcess(name string) error {
	name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	return exec.Command("taskkill", "/f", "/im", name+".exe").Run()
}

/* =======================
   EXTRAER PORCENTAJE DE VOLUMEN
   Para comandos como "pon el volumen al 60"
======================= */

func ExtractVolumePercent(input string) (int, bool) {
	words := strings.Fields(strings.ToLower(input))
	for i, w := range words {
		// Buscar número en "al 60", "a 60", "60 por ciento", "60%"
		w = strings.TrimSuffix(w, "%")
		if n, err := strconv.Atoi(w); err == nil && n >= 0 && n <= 100 {
			return n, true
		}
		// "al" o "a" seguido de número
		if (w == "al" || w == "a") && i+1 < len(words) {
			next := strings.TrimSuffix(words[i+1], "%")
			if n, err := strconv.Atoi(next); err == nil && n >= 0 && n <= 100 {
				return n, true
			}
		}
	}
	return 0, false
}

/* =======================
   CAMBIAR VENTANA
======================= */

// SwitchWindow hace Alt+Tab (siguiente ventana)
func SwitchWindow() error {
	ps := fmt.Sprintf(`Add-Type -TypeDefinition '%s';[KB]::keybd_event(0x12,0,0,0);[KB]::keybd_event(0x09,0,0,0);Start-Sleep -Milliseconds 150;[KB]::keybd_event(0x09,0,2,0);[KB]::keybd_event(0x12,0,2,0)`, keybdEventTypeDef)
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Run()
}

// SwitchToWindow intenta cambiar a una ventana específica por nombre de proceso
func SwitchToWindow(target string) error {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return fmt.Errorf("target vacío")
	}

	// Buscar proceso con tasklist
	out, err := exec.Command("tasklist", "/v").Output()
	if err != nil {
		return fmt.Errorf("no se pudo listar procesos")
	}

	procName := ""
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		// Comparar solo contra el NOMBRE DEL PROCESO (primera columna),
		// no la línea completa — tasklist /v también incluye el título
		// de la ventana en cada línea, y ese título puede contener el
		// texto buscado por pura casualidad (ej. una pestaña de Chrome
		// titulada "... code review ..." calzaba con target="code"
		// antes de este fix, aunque el proceso fuera chrome.exe, no
		// Code.exe).
		if strings.Contains(strings.ToLower(parts[0]), target) {
			procName = parts[0]
			break
		}
	}

	if procName == "" {
		return fmt.Errorf("proceso no encontrado: %s", target)
	}

	// Traer la ventana al frente de verdad. Ver comentario de
	// windowFocusTypeDef arriba para el detalle de los bugs que esto
	// corrige (búsqueda por MainWindowHandle, ventanas "cloaked" de apps
	// modernas, y truco de Alt poco confiable). FocusByTitleOrProcess ya
	// no solo confía en que SetForegroundWindow diga que sí — vuelve a
	// preguntar cuál es la ventana en foco de verdad después y compara.
	procClean := strings.TrimSuffix(strings.ToLower(procName), ".exe")
	psCmd := `Add-Type -TypeDefinition '` + windowFocusTypeDef + `'; ` +
		`[WinFocus]::FocusByTitleOrProcess('` + escapePS(target) + `', '` + escapePS(procClean) + `')`

	psOut, psErr := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd).CombinedOutput()
	report := strings.TrimSpace(string(psOut))
	// Log siempre, no solo en error: el reporte trae el diagnóstico
	// completo (cuántas candidatas había, si la ventana elegida estaba
	// "cloaked", si quedó minimizada, si el foco quedó puesto de
	// verdad) que necesitamos para depurar sin adivinar a ciegas.
	fmt.Println("🪟 FocusByTitleOrProcess:", report)
	if psErr != nil {
		return fmt.Errorf("powershell falló cambiando a %q: %v (%s)", target, psErr, report)
	}
	if strings.HasPrefix(report, "NOTFOUND") {
		return fmt.Errorf("ventana no encontrada: %s (%s)", target, report)
	}
	if strings.HasPrefix(report, "APIOK_PERO_NO_QUEDO_EN_FOCO") {
		return fmt.Errorf("se encontró la ventana pero Windows no le dio el foco: %s (%s)", target, report)
	}
	return nil
}

// escapePS escapa comillas simples para insertar texto de forma segura
// dentro de un string literal de PowerShell ('...').
func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// CloseWindow cierra la ventana identificada por target (nombre o
// título), NO la que esté activa en este instante — mismo criterio de
// búsqueda que SwitchToWindow (tasklist para el nombre de proceso,
// EnumWindows/CloseByTitleOrProcess para encontrar la ventana real).
// Antes de esto, "cierra X" ignoraba X y hacía Alt+F4 sobre la ventana
// activa — que en un asistente de voz suele ser la propia consola de
// Ahmelia, con resultados catastróficos (ver comentario en engine.go).
func CloseWindow(target string) error {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return fmt.Errorf("target vacío")
	}

	out, err := exec.Command("tasklist", "/v").Output()
	procName := ""
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			// Ver comentario equivalente en SwitchToWindow: solo el
			// nombre de proceso (primera columna), no la línea
			// completa — el título de OTRA ventana puede contener el
			// texto buscado por casualidad.
			if strings.Contains(strings.ToLower(parts[0]), target) {
				procName = parts[0]
				break
			}
		}
	}
	procClean := strings.TrimSuffix(strings.ToLower(procName), ".exe")

	psCmd := `Add-Type -TypeDefinition '` + windowFocusTypeDef + `'; ` +
		`[WinFocus]::CloseByTitleOrProcess('` + escapePS(target) + `', '` + escapePS(procClean) + `')`

	psOut, psErr := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd).CombinedOutput()
	report := strings.TrimSpace(string(psOut))
	fmt.Println("🪟 CloseByTitleOrProcess:", report)
	if psErr != nil {
		return fmt.Errorf("powershell falló cerrando %q: %v (%s)", target, psErr, report)
	}
	if strings.HasPrefix(report, "NOTFOUND") {
		return fmt.Errorf("ventana no encontrada: %s (%s)", target, report)
	}
	return nil
}
