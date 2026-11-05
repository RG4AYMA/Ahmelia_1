package capability

import (
	"fmt"
	"sort"
	"sync"
)

// ============================================================================
// REGISTRY.GO — Bloque 94.2 (Capability Registry)
// ============================================================================
//
// B94.1 formalizó Capability como contrato declarativo. Antes de B94.2,
// todavía no existía un lugar único desde el cual preguntar:
//
//	"¿Qué puedo hacer?"
//
// Registry cierra ese hueco: las Capability pasan de ser valores sueltos a
// recursos descubribles por ID, dominio y acción.
//
// PRINCIPIOS:
//   - El Registry es autoridad de DESCUBRIMIENTO, no de ejecución.
//   - ID es único. Registrar dos Capability con el mismo ID es un error:
//     reemplazar silenciosamente una definición podría cambiar permisos,
//     inputs o executor sin que nadie lo note.
//   - List devuelve orden determinista para que UI, logs y tests no dependan
//     del orden aleatorio de un map.
//   - Get devuelve una copia del valor y copia sus slices, evitando que un
//     caller mutile accidentalmente el contrato registrado.
//   - El mutex permite registrar/consultar desde componentes concurrentes
//     sin convertir el Registry en una fuente de carreras.
//   - No se importan autonomy, actions ni shared: el Registry sigue siendo
//     infraestructura declarativa y no crea ciclos de dependencia.
//
// LO QUE NO HACE:
//   - No ejecuta Action.
//   - No decide permisos (B94.3).
//   - No comprueba Preconditions (B94.4/B94.5).
//   - No afirma que una Capability esté realmente implementada: esa
//     correspondencia sigue perteneciendo al executor real de cada dominio.
//
// El resultado es un vocabulario común para que consumidores futuros puedan
// descubrir capacidades sin conocer dónde fueron implementadas.
type Registry struct {
	mu           sync.RWMutex
	capabilities map[ID]Capability
}

// NewRegistry crea un Registry vacío.
func NewRegistry() *Registry {
	return &Registry{
		capabilities: make(map[ID]Capability),
	}
}

// Register agrega una Capability al Registry.
//
// La operación es deliberadamente fail-closed:
//   - una Capability inválida no entra;
//   - un ID duplicado no reemplaza silenciosamente la definición existente.
func (r *Registry) Register(c Capability) error {
	if r == nil {
		return fmt.Errorf("capability registry: registry nil")
	}
	if err := c.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.capabilities[c.ID]; exists {
		return fmt.Errorf("capability registry: la capability %q ya está registrada", c.ID)
	}

	r.capabilities[c.ID] = cloneCapability(c)
	return nil
}

// MustRegister registra una Capability y hace panic si el contrato es
// inválido o el ID ya existe. Es útil únicamente para inicialización estática
// de capacidades conocidas por el programa.
func (r *Registry) MustRegister(c Capability) {
	if err := r.Register(c); err != nil {
		panic(err)
	}
}

// Get busca una Capability por ID.
//
// La segunda devolución distingue "encontrada" de un valor cero. La
// Capability devuelta es una copia profunda de sus slices.
func (r *Registry) Get(id ID) (Capability, bool) {
	if r == nil {
		return Capability{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.capabilities[id]
	if !ok {
		return Capability{}, false
	}
	return cloneCapability(c), true
}

// List devuelve todas las Capability registradas en orden determinista:
// primero por Domain y después por ID.
//
// La lista es un snapshot: modificar el resultado no modifica el Registry.
func (r *Registry) List() []Capability {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Capability, 0, len(r.capabilities))
	for _, c := range r.capabilities {
		result = append(result, cloneCapability(c))
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Domain != result[j].Domain {
			return result[i].Domain < result[j].Domain
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// ByDomain devuelve las Capability de un dominio en orden determinista.
func (r *Registry) ByDomain(domain Domain) []Capability {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Capability, 0)
	for _, c := range r.capabilities {
		if c.Domain == domain {
			result = append(result, cloneCapability(c))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Find devuelve las Capability que corresponden simultáneamente a un
// dominio y una Action. Es deliberadamente una búsqueda exacta: no hace
// matching semántico ni intenta inferir acciones.
func (r *Registry) Find(domain Domain, action string) []Capability {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Capability, 0)
	for _, c := range r.capabilities {
		if c.Domain == domain && c.Action == action {
			result = append(result, cloneCapability(c))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Has informa si existe una Capability con el ID indicado.
func (r *Registry) Has(id ID) bool {
	if r == nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.capabilities[id]
	return ok
}

// Count devuelve cuántas Capability hay registradas.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.capabilities)
}

// cloneCapability evita exponer slices internos del Registry.
func cloneCapability(c Capability) Capability {
	c.Inputs = cloneParameters(c.Inputs)
	c.Outputs = cloneParameters(c.Outputs)
	c.Preconditions = append([]string(nil), c.Preconditions...)
	c.FailureModes = append([]FailureMode(nil), c.FailureModes...)
	return c
}

func cloneParameters(params []Parameter) []Parameter {
	if params == nil {
		return nil
	}
	return append([]Parameter(nil), params...)
}
