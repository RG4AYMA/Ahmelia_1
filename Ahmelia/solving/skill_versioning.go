package solving

import (
	"ahmelia/interaction"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// ============================================================================
// SKILL_VERSIONING.GO — Bloque 59, Fase 4.1
// ============================================================================
//
// Skill Update Detector (58) compara y dice si una nota cambió, pero a
// propósito NO actúa — Check() nunca toca skillLibrary. La consecuencia
// directa: hoy no existe ningún camino en el proyecto donde una nota
// vieja se reemplace sin perderse — SkillLibrary.Install() (54)
// siempre pisa la entrada anterior en el mapa, sin dejar rastro de qué
// decía antes ni de cuándo se armó por primera vez. Skill Versioning
// es el que decide qué hacer con un cambio ya detectado: en vez de
// pisar en silencio, archiva la nota anterior completa antes de
// instalar la nueva.
//
// "Integrar, no crear": no reimplementa la comparación — reusa
// UpdateDetector.Check() (58) tal cual para saber SI hace falta
// versionar. Solo actúa cuando Check().NeedsUpdate() es true; si no
// cambió nada, no archiva nada y no toca skill_versions.json — un
// archivo que crece en cada consulta, cambie o no la fuente, sería
// indistinguible de "esto se actualiza todo el tiempo" cuando en
// realidad nunca cambió.
//
// LÍMITE HONESTO: esto NO decide si una versión vieja sigue siendo
// "mejor" o "peor" que la nueva — no hay forma real de juzgar eso sin
// evaluar contenido, y este proyecto no inventa una métrica de calidad
// de texto que no tiene fundamento. Tampoco resuelve qué pasa si DOS
// fuentes autorizadas para el mismo dominio dicen cosas distintas
// entre sí — eso es un conflicto de fuentes, no una versión en el
// tiempo de UNA fuente, y es trabajo de Knowledge Conflict Resolver
// (60). Acá el eje es puramente temporal: qué decía la nota ANTES,
// qué dice AHORA, y desde cuándo existe una skill en absoluto.

// SkillVersion es una fotografía de una Skill en el momento en que
// dejó de ser la versión vigente — mismo criterio de campos que
// Skill (54, incluida la estructura extraída: WhenToUse/Steps/
// Pitfalls, para que archivar una versión vieja no pierda lo que se
// había podido extraer de ella), más SupersededAt (cuándo se archivó,
// no cuándo se construyó originalmente; eso ya lo tiene BuiltAt).
//
// Integration Fix — auditoría del Bloque 59: ProcedureTitle,
// FailureRecovery, ExpectedOutcome, Operations y UpdatedAt faltaban
// acá — se agregaron a Skill (54) DESPUÉS de que este struct se
// escribiera la primera vez, así que archivar una versión vieja
// perdía esos campos en silencio (ej. quedaba sin registro de que la
// versión anterior tenía 3 Operations Wired, o de cuándo se había
// refrescado por última vez antes de superarse). Mismo principio que
// ya aplicamos en 55/57: usar lo que 54 sabe hoy, no lo que sabía
// cuando se escribió este archivo.
type SkillVersion struct {
	Problem       string
	WhenToUse     []string
	Steps         []string
	Pitfalls      []string
	Prerequisites []string

	FailureRecovery []string
	ExpectedOutcome []string
	Limitations     []string
	Operations      []Operation
	ProcedureTitle  string

	Content      string
	State        SkillState
	Sources      []string
	Corroborated bool
	BuiltAt      string // cuándo se armó ESTA versión (Skill.BuiltAt original, no se pisa)
	UpdatedAt    string // último refresco de ESTA versión antes de ser superada
	SupersededAt string // cuándo dejó de ser la vigente
}

// VersionHistory acumula las versiones ARCHIVADAS de un SkillID — la
// vigente no vive acá, vive en SkillLibrary (54); esto es solo lo que
// quedó atrás.
type VersionHistory struct {
	SkillID  string
	Versions []SkillVersion // orden cronológico: la más vieja primero
}

// FirstLearnedAt devuelve cuándo se aprendió este skill por primera
// vez — BuiltAt de la versión archivada más antigua si hay historial;
// si no hay ninguna versión archivada todavía, no hay forma honesta de
// saberlo desde acá (esa fecha vive en la Skill vigente de
// SkillLibrary, que este módulo no consulta por sí solo) — devuelve
// false, no una fecha inventada.
func (vh *VersionHistory) FirstLearnedAt() (string, bool) {
	if vh == nil || len(vh.Versions) == 0 {
		return "", false
	}
	return vh.Versions[0].BuiltAt, true
}

// Describe narra el historial archivado de un SkillID.
func (vh *VersionHistory) Describe() string {
	if vh == nil || len(vh.Versions) == 0 {
		return fmt.Sprintf("%s: sin versiones archivadas todavía — nunca se detectó un cambio real, o nunca se lo versionó.", vh.SkillID)
	}
	first, _ := vh.FirstLearnedAt()
	return fmt.Sprintf("%s: %d versión(es) archivada(s); la más vieja se aprendió el %s.", vh.SkillID, len(vh.Versions), first)
}

// SkillVersionStore persiste los VersionHistory por SkillID — mismo
// criterio exacto que SkillLibrary (54): mapa cargado de disco al
// arrancar, guardado de inmediato en cada actualización.
type SkillVersionStore struct {
	Histories map[string]*VersionHistory `json:"histories"`
}

// SkillVersionStorePath — mismo criterio que SkillLibraryPath (54).
const SkillVersionStorePath = "memory/skill_versions.json"

// NewSkillVersionStore arma un store vacío.
func NewSkillVersionStore() *SkillVersionStore {
	return &SkillVersionStore{Histories: make(map[string]*VersionHistory)}
}

// LoadSkillVersionStore carga skill_versions.json si existe; si no,
// arranca vacío — mismo patrón que LoadSkillLibrary (54).
func LoadSkillVersionStore(path string) *SkillVersionStore {
	if data, err := ioutil.ReadFile(path); err == nil {
		var s SkillVersionStore
		if err := json.Unmarshal(data, &s); err == nil && s.Histories != nil {
			return &s
		}
	}
	return NewSkillVersionStore()
}

// Save persiste el store a disco.
func (s *SkillVersionStore) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// History devuelve el historial guardado para un SkillID, si existe.
func (s *SkillVersionStore) History(skillID string) (*VersionHistory, bool) {
	h, ok := s.Histories[skillID]
	return h, ok
}

// ArchiveVersion agrega ver directamente al historial de skillID — la
// primitiva real de persistencia; Archive() (abajo) es un atajo que
// arma la SkillVersion a partir de una Skill para quien no la
// tiene ya construida (tests, uso directo). No-op si ver es nil.
func (s *SkillVersionStore) ArchiveVersion(skillID string, ver *SkillVersion) {
	if ver == nil || skillID == "" {
		return
	}
	h, ok := s.Histories[skillID]
	if !ok {
		h = &VersionHistory{SkillID: skillID}
		s.Histories[skillID] = h
	}
	h.Versions = append(h.Versions, *ver)
}

// Archive agrega old como versión archivada de skillID — no-op si old
// es nil, mismo criterio defensivo que Install() (54) con note nil.
func (s *SkillVersionStore) Archive(skillID string, old *Skill) {
	if old == nil || skillID == "" {
		return
	}
	s.ArchiveVersion(skillID, &SkillVersion{
		Problem:         old.Problem,
		WhenToUse:       old.WhenToUse,
		Steps:           old.Steps,
		Pitfalls:        old.Pitfalls,
		Prerequisites:   old.Prerequisites,
		FailureRecovery: old.FailureRecovery,
		ExpectedOutcome: old.ExpectedOutcome,
		Limitations:     old.Limitations,
		Operations:      old.Operations,
		ProcedureTitle:  old.ProcedureTitle,
		Content:         old.Content,
		State:           old.State,
		Sources:         old.Sources,
		Corroborated:    old.Corroborated,
		BuiltAt:         old.BuiltAt,
		UpdatedAt:       old.UpdatedAt,
		SupersededAt:    time.Now().Format(time.RFC3339),
	})
}

// Describe arma el resumen de todo el store — orden estable por
// SkillID, mismo criterio que SkillLibrary.Describe() (54).
func (s *SkillVersionStore) Describe() string {
	if len(s.Histories) == 0 {
		return "Todavía no hay ninguna versión archivada — ninguna nota fue reemplazada todavía."
	}
	ids := make([]string, 0, len(s.Histories))
	for id := range s.Histories {
		ids = append(ids, id)
	}
	// orden estable, mismo criterio que SkillLibrary.Describe() (54):
	// alfabético, para que la salida no dependa del orden de un mapa.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j-1] > ids[j]; j-- {
			ids[j-1], ids[j] = ids[j], ids[j-1]
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d skill(s) con historial:\n", len(ids))
	for _, id := range ids {
		b.WriteString("  — " + s.Histories[id].Describe() + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Versioner decide qué hacer con un cambio ya detectado por
// UpdateDetector (58): si cambió de verdad, archiva la nota vieja
// antes de que quien llame instale la nueva. Sin estado propio — la
// persistencia vive en SkillVersionStore, no acá, mismo criterio que
// SkillBuilder (54) vs SkillLibrary.
type Versioner struct{}

// NewVersioner arma el versionador.
func NewVersioner() *Versioner {
	return &Versioner{}
}

// Update corre UpdateDetector.Check() (58) sobre item/existing/vs, y
// SOLO si de verdad cambió Y ese cambio movió al menos un campo
// estructurado (check.StructuralChange), arma la SkillVersion
// archivada a partir de existing. No instala nada en SkillLibrary ni
// guarda el store en disco — eso es responsabilidad de quien llama
// (mismo patrón que SkillBuilder.Build() (54), que arma pero no
// persiste). Devuelve el *UpdateCheck completo (para narrar el
// motivo) y, si corresponde versionar, la SkillVersion ya armada; nil
// en la versión si no hubo cambio, no había nota existente, o el
// cambio fue puramente cosmético.
//
// Integration Fix — antes, CUALQUIER Content distinto archivaba una
// versión nueva, incluida una reformulación sin cambio de sustancia
// (el propio bloque documenta que "no debería crear una nueva versión
// por cualquier cambio cosmético" — la vara para decidir eso YA la
// calcula 58 con StructuralChange, no hacía falta inventar una nueva.
// El Content SIGUE actualizándose en SkillLibrary igual (responsabilidad
// de quien llama, ver arriba) — lo único que cambia es que no queda
// una entrada de historial por cada retoque de redacción de la fuente.
func (v *Versioner) Update(item LearningItem, existing *Skill, vs *ValidationSet) (*UpdateCheck, *SkillVersion) {
	check := NewUpdateDetector().Check(item, existing, vs)
	if !check.NeedsUpdate() || !check.StructuralChange {
		return check, nil
	}
	ver := &SkillVersion{
		Problem:         existing.Problem,
		WhenToUse:       existing.WhenToUse,
		Steps:           existing.Steps,
		Pitfalls:        existing.Pitfalls,
		Prerequisites:   existing.Prerequisites,
		FailureRecovery: existing.FailureRecovery,
		ExpectedOutcome: existing.ExpectedOutcome,
		Limitations:     existing.Limitations,
		Operations:      existing.Operations,
		ProcedureTitle:  existing.ProcedureTitle,
		Content:         existing.Content,
		State:           existing.State,
		Sources:         existing.Sources,
		Corroborated:    existing.Corroborated,
		BuiltAt:         existing.BuiltAt,
		UpdatedAt:       existing.UpdatedAt,
		SupersededAt:    time.Now().Format(time.RFC3339),
	}
	return check, ver
}

// ── Triggers de consulta ──────────────────────────────────────────────

// skillVersionUpdateTriggers dispara la cadena completa que SÍ
// actualiza: Discover → Fetch → Validate → Check (58) → si cambió,
// Archive (59) → Install (54) → Save — distinto de updateCheckTriggers
// (58), que solo detecta y nunca escribe nada.
var skillVersionUpdateTriggers = []string{
	"actualiza la nota sobre",
	"renueva la nota de estudio sobre",
	"versiona la nota sobre",
}

// ParseSkillVersionUpdateQuery detecta si se pide actualizar (y
// archivar la versión anterior de) una nota — distinto de
// ParseUpdateCheckQuery (58, solo compara, nunca escribe) y de
// ParseSkillBuildQuery (54, construye/pisa sin comparar ni archivar
// nada de lo anterior).
func ParseSkillVersionUpdateQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillVersionUpdate)
}

// skillVersionHistoryTriggers consulta el historial YA archivado — sin
// tocar la red, mismo espíritu que skillBuilderTriggers (54).
var skillVersionHistoryTriggers = []string{
	"cuantas versiones tiene la nota sobre",
	"cuando aprendiste por primera vez sobre",
	"el historial de versiones de",
}

// ParseSkillVersionHistoryQuery detecta si se pregunta por el
// historial archivado de una skill puntual.
func ParseSkillVersionHistoryQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillVersionHistory)
}
