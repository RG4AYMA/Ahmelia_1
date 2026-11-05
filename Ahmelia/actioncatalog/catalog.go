package actioncatalog

import "sort"

// PermissionLevel is the canonical declared authorization posture for a
// capability/action. It is intentionally independent of the runtime authority
// implementation so this leaf package can be consumed by classifiers,
// capability registries and execution adapters without import cycles.
type PermissionLevel string

const (
	PermissionAutomatic PermissionLevel = "automatic"
	PermissionNotify    PermissionLevel = "notify"
	PermissionAskFirst  PermissionLevel = "ask_first"
)

func (p PermissionLevel) IsValid() bool {
	switch p {
	case PermissionAutomatic, PermissionNotify, PermissionAskFirst:
		return true
	default:
		return false
	}
}

// SafetyTier is the canonical risk classification for executable actions.
type SafetyTier string

const (
	SafetyTierSafe        SafetyTier = "safe"
	SafetyTierReversible  SafetyTier = "reversible"
	SafetyTierDestructive SafetyTier = "destructive"
	SafetyTierSensitive   SafetyTier = "sensitive"
)

// ActionDefinition is the canonical semantic/operational definition of a
// local action. Every downstream view (classifier triggers, labels, safety,
// wired/standalone availability) derives from this definition.
type ActionDefinition struct {
	Action               string
	Description          string
	Label                string
	Triggers             []string
	Safety               SafetyTier
	Permission           PermissionLevel
	EngineWired          bool
	StandaloneExecutable bool
	VisibleCapability    bool
}

func BuiltinActions() []ActionDefinition {
	defs := []ActionDefinition{
		{Action: "saludo", Description: "realizar una acción de saludo", Label: "Saludar", Triggers: []string{"hola", "buenas", "que tal", "qué tal", "como estas", "cómo estás", "como te va", "cómo te va"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true},
		{Action: "abrir_programa", Description: "abrir un programa o recurso", Label: "Abrir programas y URLs", Triggers: []string{"abre", "abrir", "ejecuta", "lanza", "inicia", "open", "launch", "start", "run"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "buscar_archivo", Description: "buscar un archivo", Label: "Buscar archivos", Triggers: []string{"busca", "encuentra", "localiza", "donde esta", "donde", "find file", "search file", "find the file", "search for the file"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "mover_archivo", Description: "mover un archivo", Label: "Mover archivos", Triggers: []string{"mueve", "traslada", "move", "move file", "move the file"}, Safety: SafetyTierReversible, Permission: PermissionAutomatic, EngineWired: true, StandaloneExecutable: true, VisibleCapability: true},
		{Action: "renombrar_archivo", Description: "renombrar un archivo", Label: "Renombrar archivos", Triggers: []string{"renombra", "cambia nombre", "rename", "rename file"}, Safety: SafetyTierReversible, Permission: PermissionAutomatic, EngineWired: true, StandaloneExecutable: true, VisibleCapability: true},
		{Action: "crear_carpeta", Description: "crear una carpeta", Label: "Crear carpetas", Triggers: []string{"crea carpeta", "nueva carpeta", "create folder", "make a folder", "new folder"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, StandaloneExecutable: true, VisibleCapability: true},
		{Action: "reproducir_media", Description: "reproducir un medio", Label: "Reproducir música y medios", Triggers: []string{"reproduce", "pon", "musica", "cancion", "play", "play music", "play song"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "salir", Description: "cerrar Ahmelia", Label: "Salir de Ahmelia", Triggers: []string{"salir", "apagar", "adios", "terminar", "exit", "quit", "goodbye"}, Safety: SafetyTierReversible, Permission: PermissionAutomatic, EngineWired: true},
		{Action: "volumen_subir", Description: "aumentar el volumen actual; NO silenciar ni reducir", Label: "Controlar el volumen", Triggers: []string{"sube", "subir", "aumenta", "mas volumen", "mas alto", "sube volumen", "sube el volumen"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "volumen_bajar", Description: "disminuir/reducir el volumen actual; NO silenciar", Label: "Controlar el volumen", Triggers: []string{"baja", "bajar", "reduce", "menos volumen", "mas bajo", "baja volumen", "baja el volumen"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "volumen_silencio", Description: "silenciar o mutear el audio; SOLO cuando se pide silencio/mute, NO bajar el volumen", Label: "Controlar el volumen", Triggers: []string{"silencio", "mutea", "silenciar", "sin sonido", "mute"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "volumen_max", Description: "llevar el volumen al máximo", Label: "Controlar el volumen", Triggers: []string{"maximo", "al maximo", "a tope", "volumen maximo"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "volumen_set", Description: "ajustar el volumen a un porcentaje", Label: "Controlar el volumen", Triggers: []string{"volumen", "pon el volumen", "volumen al"}, Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "minimizar", Description: "minimizar las ventanas", Label: "Controlar ventanas", Triggers: []string{"minimiza", "esconde", "escritorio", "minimize", "hide window"}, Safety: SafetyTierReversible, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "cambiar_ventana", Description: "cambiar a una ventana", Label: "Controlar ventanas", Triggers: []string{"cambia de ventana", "cambia ventana", "siguiente ventana", "otra ventana", "alt tab", "cambiar ventana", "cambia a", "ha cambiado ventana", "siguiente app", "cambiar app", "cambiar de app", "vuelve a", "regresa a", "vuelvelo", "vuelve al"}, Safety: SafetyTierReversible, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "cerrar_ventana", Description: "cerrar una ventana", Label: "Controlar ventanas", Triggers: []string{"cierra", "cerrar", "close", "close window"}, Safety: SafetyTierReversible, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
		{Action: "abrir_archivo", Description: "abrir un archivo seleccionado", Label: "Abrir archivos", Safety: SafetyTierSafe, Permission: PermissionAutomatic, EngineWired: true, VisibleCapability: true},
	}
	for i := range defs {
		defs[i].Triggers = append([]string(nil), defs[i].Triggers...)
	}
	return defs
}

func ActionTriggers() map[string][]string {
	result := make(map[string][]string)
	for _, def := range BuiltinActions() {
		if len(def.Triggers) > 0 {
			result[def.Action] = append([]string(nil), def.Triggers...)
		}
	}
	return result
}

func ActionLabels() map[string]string {
	result := make(map[string]string)
	for _, def := range BuiltinActions() {
		if def.Label != "" {
			result[def.Action] = def.Label
		}
	}
	return result
}

func ActionSkipInCapabilities() map[string]bool {
	result := make(map[string]bool)
	for _, def := range BuiltinActions() {
		if !def.VisibleCapability {
			result[def.Action] = true
		}
	}
	return result
}

func WiredActions() map[string]bool {
	result := make(map[string]bool)
	for _, def := range BuiltinActions() {
		if def.EngineWired {
			result[def.Action] = true
		}
	}
	return result
}

func ActionSafety() map[string]SafetyTier {
	result := make(map[string]SafetyTier)
	for _, def := range BuiltinActions() {
		result[def.Action] = def.Safety
	}
	return result
}

func StandaloneExecutableActions() map[string]bool {
	result := make(map[string]bool)
	for _, def := range BuiltinActions() {
		if def.StandaloneExecutable {
			result[def.Action] = true
		}
	}
	return result
}

func ActionDefinitionFor(action string) (ActionDefinition, bool) {
	for _, def := range BuiltinActions() {
		if def.Action == action {
			return def, true
		}
	}
	return ActionDefinition{}, false
}

func SortedActionNames() []string {
	names := make([]string, 0, len(BuiltinActions()))
	for _, def := range BuiltinActions() {
		names = append(names, def.Action)
	}
	sort.Strings(names)
	return names
}
