package tools

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"ahmelia/capability"
)

// ============================================================================
// TOOLS/ — Bloque 94.4 (Tool Execution Foundation)
// ============================================================================
//
// B94.4 convierte el contrato declarativo de B94.1-B94.3 en una frontera de
// ejecución real sin acoplar todavía herramientas concretas al núcleo.
//
// Flujo canónico:
//
//   Capability -> Tool -> Action -> Executor -> Environment
//
// Capability sigue describiendo QUÉ puede hacerse y qué permiso requiere.
// Tool declara QUÉ implementación sabe atender esa Capability.
// Action es la instancia concreta de una ejecución.
// Executor contiene la frontera que realmente ejecutará la Action.
// Environment representa el mundo sobre el que esa ejecución opera.
//
// Este bloque NO implementa todavía Filesystem, Terminal, Process, Browser,
// Python, Git, Unity, Blender, etc. Solo crea el contrato que todos ellos
// podrán implementar después. Tampoco decide permisos: eso continúa en
// B94.3. Tampoco observa/verifica el resultado: eso corresponde a B94.5.

// ID identifica una Tool registrada.
type ID string

// Action representa una ejecución concreta derivada de una Capability.
// Inputs son los valores de esta ejecución, no la declaración de inputs de
// capability.Capability.
type Action struct {
	ID           string
	CapabilityID capability.ID
	Name         string
	Inputs       map[string]string
}

// Validate comprueba el mínimo necesario para entregar una Action a un
// Executor. La semántica de Inputs pertenece a la Capability correspondiente.
func (a Action) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("tool action: ID vacío")
	}
	if a.CapabilityID == "" {
		return fmt.Errorf("tool action: CapabilityID vacío")
	}
	if a.Name == "" {
		return fmt.Errorf("tool action: Name vacío")
	}
	return nil
}

// Result normaliza el resultado inmediato de un Executor. B94.5 podrá usar
// este resultado como una primera pieza de evidencia, pero no se confunde
// con una verificación del entorno.
type Result struct {
	ActionID string
	Success  bool
	Output   string
	Err      error
}

// Environment es la frontera mínima con el mundo real. Un Environment
// concreto puede representar el sistema operativo, una sesión de Unity,
// Blender, un repositorio Git, un navegador, etc.
type Environment interface {
	Name() string
}

// Executor es la única interfaz que tiene permiso conceptual para cruzar la
// frontera Tool -> mundo. B94.4 solo define el contrato; las implementaciones
// concretas llegarán después.
type Executor interface {
	Execute(context.Context, Action, Environment) Result
}

// Tool vincula una o más Capabilities con un Executor concreto.
//
// Una Tool no es una Capability: una Capability es una capacidad declarativa;
// una Tool es el mecanismo disponible para materializarla.
type Tool interface {
	ID() ID
	Capabilities() []capability.ID
	Executor() Executor
}

// Registry contiene las Tools disponibles para ejecución. Al igual que
// capability.Registry, es infraestructura de descubrimiento y no sustituye
// la autorización de B94.3.
type Registry struct {
	mu    sync.RWMutex
	tools map[ID]Tool
}

// NewRegistry crea un Registry vacío.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[ID]Tool)}
}

// Register agrega una Tool. IDs duplicados se rechazan para evitar que una
// implementación pueda reemplazarse silenciosamente durante la ejecución.
func isNilExecutor(e Executor) bool {
	if e == nil {
		return true
	}
	v := reflect.ValueOf(e)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (r *Registry) Register(t Tool) error {
	if r == nil {
		return fmt.Errorf("tool registry: registry nil")
	}
	if t == nil || t.ID() == "" {
		return fmt.Errorf("tool registry: tool inválido")
	}
	if isNilExecutor(t.Executor()) {
		return fmt.Errorf("tool registry: tool %q no tiene executor", t.ID())
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[t.ID()]; exists {
		return fmt.Errorf("tool registry: tool %q ya está registrada", t.ID())
	}

	r.tools[t.ID()] = t
	return nil
}

// Get recupera una Tool por ID.
func (r *Registry) Get(id ID) (Tool, bool) {
	if r == nil {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[id]
	return t, ok
}

// List devuelve las Tools en orden determinista.
func (r *Registry) List() []Tool {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID() < out[j].ID()
	})
	return out
}

// FindCapability devuelve las Tools capaces de atender una Capability.
// La búsqueda es exacta: no hace inferencia semántica.
func (r *Registry) FindCapability(id capability.ID) []Tool {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Tool, 0)
	for _, t := range r.tools {
		for _, supported := range t.Capabilities() {
			if supported == id {
				out = append(out, t)
				break
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID() < out[j].ID()
	})
	return out
}

// Runner implementa el camino mínimo de B94.4:
// CapabilityID -> Tool -> Executor -> Environment.
//
// La Action ya debe haber sido construida por la capa superior. Runner no
// concede permisos ni verifica el resultado; únicamente resuelve la Tool y
// entrega la Action al Executor correspondiente.
type Runner struct {
	registry *Registry
}

// NewRunner crea un Runner sobre un Registry de Tools.
func NewRunner(registry *Registry) *Runner {
	return &Runner{registry: registry}
}

// Execute resuelve la Tool para la Capability de la Action y delega la
// ejecución. Un error de resolución no llega al Executor.
func (r *Runner) Execute(ctx context.Context, action Action, env Environment) Result {
	if r == nil || r.registry == nil {
		return Result{ActionID: action.ID, Err: fmt.Errorf("tool runner: registry nil")}
	}
	if err := action.Validate(); err != nil {
		return Result{ActionID: action.ID, Err: err}
	}
	if env == nil {
		return Result{ActionID: action.ID, Err: fmt.Errorf("tool runner: environment nil")}
	}

	tools := r.registry.FindCapability(action.CapabilityID)
	if len(tools) == 0 {
		return Result{ActionID: action.ID, Err: fmt.Errorf("tool runner: ninguna tool puede ejecutar capability %q", action.CapabilityID)}
	}
	if len(tools) > 1 {
		return Result{ActionID: action.ID, Err: fmt.Errorf("tool runner: capability %q tiene %d tools compatibles; selección ambigua", action.CapabilityID, len(tools))}
	}

	return tools[0].Executor().Execute(ctx, action, env)
}
