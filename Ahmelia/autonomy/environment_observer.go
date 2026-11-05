package autonomy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

//
// B85 — Observación Real del Entorno
//

// EnvironmentSnapshot representa una fotografía del entorno en un instante.
type WindowObservation struct {
	ProcessID   int    `json:"process_id"`
	ProcessName string `json:"process_name"`
	Title       string `json:"title"`
	Focused     bool   `json:"focused"`
}

// EnvironmentSnapshot es una lectura por dominios. Un dominio ausente de
// ObservedDomains no se puede usar para inferir cierres o desapariciones.
type EnvironmentSnapshot struct {
	Timestamp          time.Time
	CoverageKnown      bool `json:"coverage_known"`
	ObservedDomains    []string
	UnavailableDomains []string
	Windows            []WindowObservation

	FocusedApp string
	OpenApps   []string
	Files      []string
	Processes  []string

	CPUUsage float64
	RAMUsage float64
}

// EnvironmentInspector es el contrato que implementará environment.Manager (B86).
type EnvironmentSnapshotInspector interface {
	Snapshot(ctx context.Context) (EnvironmentSnapshot, error)
}

// EnvironmentObserver observa cambios del entorno y los transforma en ContextObservation.
type EnvironmentObserver struct {
	mu sync.RWMutex

	inspector EnvironmentSnapshotInspector
	last      *EnvironmentSnapshot
}

func NewEnvironmentObserver(inspector EnvironmentSnapshotInspector) *EnvironmentObserver {
	return &EnvironmentObserver{
		inspector: inspector,
	}
}

// Observe genera observaciones nuevas comparando snapshots consecutivos.
func (o *EnvironmentObserver) Observe(ctx context.Context) ([]ContextObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if o.inspector == nil {
		return nil, nil
	}

	current, err := o.inspector.Snapshot(ctx)
	if err != nil {
		return nil, err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if o.last == nil {
		copySnapshot := cloneEnvironmentSnapshot(current)
		o.last = &copySnapshot
		// The first observation is a baseline, not an absence of information.
		// Without it, the cognitive cycle starts blind and only receives deltas
		// after a later environmental change.
		return []ContextObservation{{
			Type:   TypeSystemSignal,
			Source: "environment",
			Payload: map[string]interface{}{
				"event":               "baseline_snapshot",
				"focused_app":         current.FocusedApp,
				"open_apps":           append([]string(nil), current.OpenApps...),
				"files":               append([]string(nil), current.Files...),
				"processes":           append([]string(nil), current.Processes...),
				"windows":             append([]WindowObservation(nil), current.Windows...),
				"coverage_known":      current.CoverageKnown,
				"observed_domains":    append([]string(nil), current.ObservedDomains...),
				"unavailable_domains": append([]string(nil), current.UnavailableDomains...),
				"cpu_usage":           current.CPUUsage,
				"ram_usage":           current.RAMUsage,
			},
			Timestamp: current.Timestamp,
		}}, nil
	}

	previous := *o.last
	copySnapshot := cloneEnvironmentSnapshot(current)
	o.last = &copySnapshot

	var observations []ContextObservation

	// Aplicación enfocada.
	if domainObserved(previous, "focus") && domainObserved(current, "focus") && previous.FocusedApp != current.FocusedApp {
		observations = append(observations, ContextObservation{
			Type:      TypeAppFocused,
			Source:    "environment",
			Payload:   map[string]interface{}{"app": current.FocusedApp},
			Timestamp: current.Timestamp,
		})
	}

	// Aplicaciones abiertas/cerradas. Solo se infiere cierre si ambos snapshots
	// contienen una enumeración completa del dominio de ventanas.
	if domainObserved(previous, "windows") && domainObserved(current, "windows") {
		for _, app := range difference(previous.OpenApps, current.OpenApps) {
			observations = append(observations, ContextObservation{
				Type:      TypeSystemSignal,
				Source:    "environment",
				Payload:   map[string]interface{}{"event": "opened", "app": app},
				Timestamp: current.Timestamp,
			})
		}
		for _, app := range difference(current.OpenApps, previous.OpenApps) {
			observations = append(observations, ContextObservation{Type: TypeSystemSignal, Source: "environment", Payload: map[string]interface{}{"event": "closed", "app": app}, Timestamp: current.Timestamp})
		}
	}

	// Archivos nuevos detectados.
	for _, file := range difference(previous.Files, current.Files) {
		observations = append(observations, ContextObservation{
			Type:      TypeFileChanged,
			Source:    "environment",
			Payload:   map[string]interface{}{"file": file},
			Timestamp: current.Timestamp,
		})
	}

	// Procesos iniciados/detenidos: la identidad incluye PID cuando el adaptador
	// la proporciona, por lo que dos instancias del mismo ejecutable no se confunden.
	if domainObserved(previous, "processes") && domainObserved(current, "processes") {
		for _, proc := range difference(previous.Processes, current.Processes) {
			observations = append(observations, ContextObservation{
				Type:      TypeSystemSignal,
				Source:    "environment",
				Payload:   map[string]interface{}{"event": "process_started", "process": proc},
				Timestamp: current.Timestamp,
			})
		}
		for _, proc := range difference(current.Processes, previous.Processes) {
			observations = append(observations, ContextObservation{Type: TypeSystemSignal, Source: "environment", Payload: map[string]interface{}{"event": "process_stopped", "process": proc}, Timestamp: current.Timestamp})
		}
	}

	// Cambios de ventanas (PID + título), sin asumir que un cambio de título
	// equivale a un cambio de proceso.
	if domainObserved(previous, "windows") && domainObserved(current, "windows") {
		prev := windowIndex(previous.Windows)
		cur := windowIndex(current.Windows)
		for key, window := range cur {
			if old, ok := prev[key]; !ok {
				observations = append(observations, ContextObservation{Type: TypeSystemSignal, Source: "environment", Payload: map[string]interface{}{"event": "window_opened", "process_id": window.ProcessID, "process": window.ProcessName, "title": window.Title, "focused": window.Focused}, Timestamp: current.Timestamp})
			} else if old.Title != window.Title || old.Focused != window.Focused {
				observations = append(observations, ContextObservation{Type: TypeSystemSignal, Source: "environment", Payload: map[string]interface{}{"event": "window_changed", "process_id": window.ProcessID, "process": window.ProcessName, "previous_title": old.Title, "title": window.Title, "focused": window.Focused}, Timestamp: current.Timestamp})
			}
		}
		for key, window := range prev {
			if _, ok := cur[key]; !ok {
				observations = append(observations, ContextObservation{Type: TypeSystemSignal, Source: "environment", Payload: map[string]interface{}{"event": "window_closed", "process_id": window.ProcessID, "process": window.ProcessName, "title": window.Title}, Timestamp: current.Timestamp})
			}
		}
	}

	// Recursos del sistema.
	if domainObserved(previous, "resources") && domainObserved(current, "resources") && (significantChange(previous.CPUUsage, current.CPUUsage) ||
		significantChange(previous.RAMUsage, current.RAMUsage)) {

		observations = append(observations, ContextObservation{
			Type:      TypeSystemSignal,
			Source:    "environment",
			Payload:   map[string]interface{}{"event": "resources_changed", "cpu": current.CPUUsage, "ram": current.RAMUsage},
			Timestamp: current.Timestamp,
		})
	}

	return observations, nil
}

func (o *EnvironmentObserver) LastSnapshot() *EnvironmentSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.last == nil {
		return nil
	}

	copy := cloneEnvironmentSnapshot(*o.last)
	return &copy
}

func cloneEnvironmentSnapshot(snapshot EnvironmentSnapshot) EnvironmentSnapshot {
	snapshot.OpenApps = append([]string(nil), snapshot.OpenApps...)
	snapshot.Files = append([]string(nil), snapshot.Files...)
	snapshot.Processes = append([]string(nil), snapshot.Processes...)
	snapshot.Windows = append([]WindowObservation(nil), snapshot.Windows...)
	snapshot.ObservedDomains = append([]string(nil), snapshot.ObservedDomains...)
	snapshot.UnavailableDomains = append([]string(nil), snapshot.UnavailableDomains...)
	return snapshot
}

func domainObserved(snapshot EnvironmentSnapshot, domain string) bool {
	// Compatibilidad con inspectores antiguos: sin metadatos, el snapshot se
	// interpreta como completo, tal como establecía el contrato previo.
	if !snapshot.CoverageKnown {
		return true
	}
	for _, observed := range snapshot.ObservedDomains {
		if observed == domain {
			return true
		}
	}
	return false
}

func windowIndex(windows []WindowObservation) map[string]WindowObservation {
	index := make(map[string]WindowObservation, len(windows))
	for _, window := range windows {
		key := fmt.Sprintf("%d", window.ProcessID)
		if window.ProcessID <= 0 {
			key = strings.ToLower(window.ProcessName) + "|" + window.Title
		}
		index[key] = window
	}
	return index
}

// ---------- Helpers ----------

func difference(previous, current []string) []string {
	exists := make(map[string]struct{}, len(previous))

	for _, v := range previous {
		exists[v] = struct{}{}
	}

	var diff []string

	for _, v := range current {
		if _, ok := exists[v]; !ok {
			diff = append(diff, v)
		}
	}

	return diff
}

func significantChange(previous, current float64) bool {
	const threshold = 5.0
	delta := previous - current
	if delta < 0 {
		delta = -delta
	}
	return delta >= threshold
}
