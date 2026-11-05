package selfknowledge

// Explainer es la interfaz que todos los módulos de Ahmelia deben implementar
// Permite que cualquier componente pueda explicar qué hace
type Explainer interface {
	// Name retorna el nombre del módulo
	Name() string

	// Description retorna una descripción de qué hace el módulo
	Description() string

	// Capabilities retorna qué es capaz de hacer
	Capabilities() []string

	// CurrentState retorna el estado actual del módulo
	CurrentState() map[string]interface{}

	// ExplainDecision explica por qué tomó una decisión específica
	ExplainDecision(context map[string]interface{}) string
}

// ModuleInfo contiene información sobre un módulo
type ModuleInfo struct {
	Name         string
	Description  string
	Capabilities []string
	Status       string // activo, inactivo, error
	LastUpdate   string
}

// ModuleRegistry es un registro de todos los módulos que pueden explicarse
type ModuleRegistry struct {
	modules map[string]Explainer
}

// NewModuleRegistry crea un registro nuevo
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]Explainer),
	}
}

// Register agrega un módulo al registro
func (mr *ModuleRegistry) Register(explainer Explainer) {
	mr.modules[explainer.Name()] = explainer
}

// GetInfo obtiene información sobre un módulo específico
func (mr *ModuleRegistry) GetInfo(moduleName string) *ModuleInfo {
	explainer, exists := mr.modules[moduleName]
	if !exists {
		return nil
	}

	return &ModuleInfo{
		Name:         explainer.Name(),
		Description:  explainer.Description(),
		Capabilities: explainer.Capabilities(),
		Status:       "activo",
	}
}

// ListModules retorna la lista de todos los módulos registrados
func (mr *ModuleRegistry) ListModules() []ModuleInfo {
	modules := []ModuleInfo{}
	for _, explainer := range mr.modules {
		modules = append(modules, ModuleInfo{
			Name:         explainer.Name(),
			Description:  explainer.Description(),
			Capabilities: explainer.Capabilities(),
			Status:       "activo",
		})
	}
	return modules
}

// GetDescription obtiene la descripción de un módulo
func (mr *ModuleRegistry) GetDescription(moduleName string) string {
	explainer, exists := mr.modules[moduleName]
	if !exists {
		return "módulo no encontrado"
	}
	return explainer.Description()
}

// GetCapabilities obtiene las capacidades de un módulo
func (mr *ModuleRegistry) GetCapabilities(moduleName string) []string {
	explainer, exists := mr.modules[moduleName]
	if !exists {
		return nil
	}
	return explainer.Capabilities()
}
