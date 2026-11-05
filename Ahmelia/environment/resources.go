package environment

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ============================================================================
// RESOURCES.GO — World Model v2: estado del sistema (CPU/RAM/GPU/batería)
// ============================================================================
//
// world_model.go (v1) responde "qué estás haciendo" con contexto de trabajo
// (app/proyecto/archivo). Esto es un eje distinto: no de qué hablas, sino
// en qué CONDICIONES está la máquina ahora mismo — lo que el Reasoner
// necesita para decisiones tipo "no es buen momento para compilar" (CPU ya
// al 95%) o "cuidado, batería baja" (Reasoner, no un dashboard).
//
// A propósito NO se mete esto en WorldSnapshot()/Describe(): esas dos
// corren en cada "qué estás haciendo" y deben seguir siendo instantáneas.
// GetSystemResources() dispara un proceso de PowerShell (y opcionalmente
// nvidia-smi) — cuesta más, así que se consulta solo cuando alguien
// pregunta específicamente por esto o el Reasoner lo necesita para juzgar
// algo, no en cada lectura de contexto general.
//
// Igual que el resto de environment/: conservador. Si una fuente no está
// disponible (sin batería porque es de escritorio, sin nvidia-smi porque
// no es GPU NVIDIA), el campo correspondiente queda en su estado "no
// disponible" explícito — no se inventa un número.

// SystemResources es la foto de "en qué condiciones está la máquina"
// en el momento en que se pidió. No se cachea: cada llamada a
// GetSystemResources() vuelve a medir.
type SystemResources struct {
	CPUPercent float64 // 0-100, promedio de todos los núcleos

	RAMUsedGB  float64
	RAMTotalGB float64
	RAMPercent float64 // 0-100

	HasBattery      bool // false en la mayoría de PCs de escritorio
	BatteryPercent  int
	BatteryCharging bool

	HasGPU          bool // false si no hay nvidia-smi disponible
	GPUName         string
	GPUUsagePercent float64
	GPUMemUsedMB    float64
	GPUMemTotalMB   float64
}

// GetSystemResources mide CPU/RAM/batería/GPU ahora mismo. Cada fuente se
// consulta por separado y con su propio manejo de error — que falle una
// (ej. no hay GPU NVIDIA) no debe tumbar a las demás.
func GetSystemResources() (*SystemResources, error) {
	res := &SystemResources{}

	if err := fillCPUAndRAM(res); err != nil {
		return nil, fmt.Errorf("no se pudo leer CPU/RAM: %w", err)
	}

	// Batería: error o salida vacía es NORMAL en un desktop (no hay
	// batería que reportar) — no se trata como fallo real.
	fillBattery(res)

	// GPU: igual, error o nvidia-smi ausente es normal si no hay GPU
	// NVIDIA — no es un fallo real, HasGPU queda en false.
	fillGPU(res)

	return res, nil
}

// fillCPUAndRAM usa Get-CimInstance (WMI) — mismo mecanismo ya usado en
// system/control.go para audio, sin dependencias externas nuevas.
func fillCPUAndRAM(res *SystemResources) error {
	// LoadPercentage de Win32_Processor: si hay varios núcleos/procesadores
	// lógicos reportados como instancias separadas, se promedian.
	// Win32_OperatingSystem da memoria libre/total en KB.
	script := `
$cpu = (Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average
$os = Get-CimInstance Win32_OperatingSystem
$totalKB = $os.TotalVisibleMemorySize
$freeKB = $os.FreePhysicalMemory
Write-Output "$cpu|$totalKB|$freeKB"
`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) != 3 {
		return fmt.Errorf("salida inesperada de PowerShell: %q", string(out))
	}
	cpu, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return err
	}
	totalKB, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return err
	}
	freeKB, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return err
	}

	res.CPUPercent = cpu
	res.RAMTotalGB = totalKB / 1024 / 1024
	usedKB := totalKB - freeKB
	res.RAMUsedGB = usedKB / 1024 / 1024
	if totalKB > 0 {
		res.RAMPercent = (usedKB / totalKB) * 100
	}
	return nil
}

// fillBattery deja HasBattery=false sin error si la máquina no tiene
// batería (desktop) — Win32_Battery simplemente no devuelve instancias.
func fillBattery(res *SystemResources) {
	script := `
$b = Get-CimInstance Win32_Battery | Select-Object -First 1
if ($b) { Write-Output "$($b.EstimatedChargeRemaining)|$($b.BatteryStatus)" }
`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return
	}
	parts := strings.Split(line, "|")
	if len(parts) != 2 {
		return
	}
	pct, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return
	}
	status := strings.TrimSpace(parts[1])
	res.HasBattery = true
	res.BatteryPercent = pct
	// BatteryStatus == "2" es "Charging" en WMI (ver docs de
	// Win32_Battery.BatteryStatus). El resto de valores (1,3,4,5...) son
	// distintos estados de "no cargando" — no hace falta distinguirlos
	// más fino que charging/no-charging para lo que el Reasoner necesita.
	res.BatteryCharging = status == "2"
}

// fillGPU usa nvidia-smi directamente — ya asumido presente porque el
// resto del proyecto (ai/Voice/voice.py) ya depende de una GPU NVIDIA
// para Whisper. Si no está (no es máquina NVIDIA, o nvidia-smi no está
// en el PATH), HasGPU queda en false sin error — es un dato opcional,
// no todas las máquinas donde corra Ahmelia tendrán GPU NVIDIA.
func fillGPU(res *SystemResources) {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return
	}
	parts := strings.Split(line, ",")
	if len(parts) != 4 {
		return
	}
	name := strings.TrimSpace(parts[0])
	usage, err1 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	memUsed, err2 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	memTotal, err3 := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}
	res.HasGPU = true
	res.GPUName = name
	res.GPUUsagePercent = usage
	res.GPUMemUsedMB = memUsed
	res.GPUMemTotalMB = memTotal
}

// Describe arma la narrativa en español — mismo criterio que
// WorldSnapshot.Describe(): una sola frase coherente, no campos sueltos.
func (r *SystemResources) Describe() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("CPU al %.0f%%, RAM al %.0f%% (%.1f de %.1f GB)",
		r.CPUPercent, r.RAMPercent, r.RAMUsedGB, r.RAMTotalGB))

	if r.HasGPU {
		b.WriteString(fmt.Sprintf(", GPU %s al %.0f%%", r.GPUName, r.GPUUsagePercent))
	}

	if r.HasBattery {
		if r.BatteryCharging {
			b.WriteString(fmt.Sprintf(", batería al %d%% y cargando", r.BatteryPercent))
		} else {
			b.WriteString(fmt.Sprintf(", batería al %d%%", r.BatteryPercent))
		}
	}

	return b.String()
}

// IsBusy es la lectura que de verdad le sirve al Reasoner para decidir
// cosas como "no es buen momento para compilar" — un umbral simple y
// explícito en vez de que cada consumidor invente su propio criterio
// leyendo CPUPercent/RAMPercent por separado.
func (r *SystemResources) IsBusy() bool {
	return r.CPUPercent >= 85 || r.RAMPercent >= 90
}
