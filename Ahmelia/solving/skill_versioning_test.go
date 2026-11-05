package solving

import (
	"ahmelia/interaction"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersioner_SinNotaExistenteNoVersionaNada confirma que, sin nota
// guardada, Update() no arma ninguna versión para archivar — mismo
// criterio de "no aplica" que UpdateDetector (58).
func TestVersioner_SinNotaExistenteNoVersionaNada(t *testing.T) {
	v := NewVersioner()
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	check, ver := v.Update(item, nil, nil)

	if check.NeedsUpdate() {
		t.Fatal("sin nota existente, NeedsUpdate() debería ser false")
	}
	if ver != nil {
		t.Error("sin nota existente, no debería armarse ninguna SkillVersion para archivar")
	}
}

// TestVersioner_ContenidoIdenticoNoArchivaNada confirma que, si el
// contenido no cambió, Update() no arma versión — versionar algo que
// no cambió llenaría el historial de ruido.
func TestVersioner_ContenidoIdenticoNoArchivaNada(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] mismo contenido de siempre.", BuiltAt: "2026-01-01T00:00:00Z"}
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", "mismo contenido de siempre."), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	v := NewVersioner()
	check, ver := v.Update(item, existing, vs)

	if check.NeedsUpdate() {
		t.Fatal("con contenido idéntico, NeedsUpdate() debería ser false")
	}
	if ver != nil {
		t.Error("con contenido idéntico, no debería armarse ninguna SkillVersion")
	}
}

// TestVersioner_ContenidoDistintoArchivaVersionVieja confirma el caso
// central: si el contenido cambió Y ese cambio es estructural (acá,
// la fuente nueva trae una lista de pasos que la vieja no tenía),
// Update() arma una SkillVersion con el Content/BuiltAt de la nota
// VIEJA (no de la nueva) para archivar.
func TestVersioner_ContenidoDistintoArchivaVersionVieja(t *testing.T) {
	existing := &Skill{
		SkillID: "Go", Label: "aprender go",
		Content: "[Go Docs] contenido viejo de hace meses.",
		Sources: []string{"Go Docs"}, Corroborated: false,
		BuiltAt: "2026-01-01T00:00:00Z",
	}
	nuevoTexto := "Para instalar Go, segui estos pasos:\n" +
		"1. Descarga el instalador\n" +
		"2. Ejecuta el instalador\n"
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", nuevoTexto), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	v := NewVersioner()
	check, ver := v.Update(item, existing, vs)

	if !check.NeedsUpdate() {
		t.Fatal("con contenido distinto, NeedsUpdate() debería ser true")
	}
	if check.Candidate == nil {
		t.Fatal("esperaba que Candidate quedara poblado con lo que Build() armó ahora")
	}
	if ver == nil {
		t.Fatal("con un cambio estructural real (pasos nuevos), esperaba una SkillVersion armada para archivar")
	}
	if ver.Content != existing.Content {
		t.Errorf("la versión archivada debería tener el Content VIEJO, obtuve: %q", ver.Content)
	}
	if ver.BuiltAt != existing.BuiltAt {
		t.Errorf("la versión archivada debería conservar el BuiltAt original, obtuve: %q", ver.BuiltAt)
	}
	if ver.SupersededAt == "" {
		t.Error("esperaba que SupersededAt quedara poblado al archivar")
	}
}

// TestVersioner_CambioCosmeticoNoArchivaPeroActualizaCandidate
// (Integration Fix) confirma la corrección del hallazgo real: antes,
// CUALQUIER Content distinto archivaba una versión — incluida una
// reformulación sin ningún cambio de sustancia (mismos 0
// Steps/Prerequisites/etc. de los dos lados, solo texto reescrito).
// Ahora eso NO archiva nada (ver == nil), pero check.Candidate sigue
// poblado — quien llama puede refrescar el Content igual, solo no se
// ensucia el historial con una entrada sin sustancia.
func TestVersioner_CambioCosmeticoNoArchivaPeroActualizaCandidate(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] Go es un lenguaje de programación viejo.", BuiltAt: "2026-01-01T00:00:00Z"}
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", "Go es un lenguaje de programación moderno."), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	check, ver := NewVersioner().Update(item, existing, vs)

	if !check.NeedsUpdate() {
		t.Fatal("el Content sí cambió, NeedsUpdate() debería ser true")
	}
	if ver != nil {
		t.Error("un cambio puramente cosmético (sin campos estructurados distintos) no debería archivar ninguna versión")
	}
	if check.Candidate == nil {
		t.Error("Candidate debería seguir poblado — quien llama puede refrescar el Content igual, solo no se archiva")
	}
}

// TestSkillVersionStore_ArchiveVersion_AcumulaHistorial confirma que
// archivar dos veces acumula ambas versiones, en orden cronológico.
func TestSkillVersionStore_ArchiveVersion_AcumulaHistorial(t *testing.T) {
	s := NewSkillVersionStore()
	s.ArchiveVersion("Go", &SkillVersion{Content: "v1", BuiltAt: "2026-01-01T00:00:00Z", SupersededAt: "2026-02-01T00:00:00Z"})
	s.ArchiveVersion("Go", &SkillVersion{Content: "v2", BuiltAt: "2026-02-01T00:00:00Z", SupersededAt: "2026-03-01T00:00:00Z"})

	h, ok := s.History("Go")
	if !ok {
		t.Fatal("esperaba encontrar historial para 'Go'")
	}
	if len(h.Versions) != 2 {
		t.Fatalf("esperaba 2 versiones archivadas, obtuve %d", len(h.Versions))
	}
	if h.Versions[0].Content != "v1" || h.Versions[1].Content != "v2" {
		t.Error("esperaba orden cronológico: la más vieja primero")
	}
}

// TestSkillVersionStore_ArchiveVersion_NilNoRompe confirma el criterio
// defensivo: archivar nil no crea entradas fantasma.
func TestSkillVersionStore_ArchiveVersion_NilNoRompe(t *testing.T) {
	s := NewSkillVersionStore()
	s.ArchiveVersion("Go", nil)

	if _, ok := s.History("Go"); ok {
		t.Error("archivar nil no debería crear ningún historial")
	}
}

// TestVersionHistory_FirstLearnedAt confirma que FirstLearnedAt()
// devuelve el BuiltAt de la versión más vieja, y false sin historial.
func TestVersionHistory_FirstLearnedAt(t *testing.T) {
	var empty *VersionHistory
	if _, ok := empty.FirstLearnedAt(); ok {
		t.Error("sin historial (nil), FirstLearnedAt() debería devolver false")
	}

	h := &VersionHistory{SkillID: "Go", Versions: []SkillVersion{
		{Content: "v1", BuiltAt: "2026-01-01T00:00:00Z"},
		{Content: "v2", BuiltAt: "2026-02-01T00:00:00Z"},
	}}
	first, ok := h.FirstLearnedAt()
	if !ok || first != "2026-01-01T00:00:00Z" {
		t.Errorf("esperaba el BuiltAt de la versión más vieja, obtuve %q, ok=%v", first, ok)
	}
}

// TestSkillVersionStore_SaveLoad_Roundtrip confirma persistencia real
// a disco, mismo patrón que los tests de SkillLibrary (54).
func TestSkillVersionStore_SaveLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill_versions.json")

	s := NewSkillVersionStore()
	s.ArchiveVersion("Python", &SkillVersion{Content: "contenido viejo", BuiltAt: "2026-01-01T00:00:00Z", SupersededAt: "2026-02-01T00:00:00Z"})
	if err := s.Save(path); err != nil {
		t.Fatalf("Save() falló: %v", err)
	}

	loaded := LoadSkillVersionStore(path)
	h, ok := loaded.History("Python")
	if !ok {
		t.Fatal("esperaba encontrar el historial de 'Python' tras recargar")
	}
	if len(h.Versions) != 1 || h.Versions[0].Content != "contenido viejo" {
		t.Error("el historial recargado no coincide con lo guardado")
	}
}

// TestLoadSkillVersionStore_ArchivoInexistenteArrancaVacio confirma el
// criterio defensivo: sin archivo, arranca vacío, no rompe.
func TestLoadSkillVersionStore_ArchivoInexistenteArrancaVacio(t *testing.T) {
	s := LoadSkillVersionStore("/ruta/que/no/existe/skill_versions.json")
	if len(s.Histories) != 0 {
		t.Error("sin archivo, esperaba un store vacío")
	}
}

// TestParseSkillVersionUpdateQuery_DistintoDeUpdateCheckYSkillBuild
// confirma que el trigger de actualizar-y-versionar no se pisa con
// Skill Update Detector (58, solo detecta) ni Skill Build (54,
// construye/pisa sin archivar nada).
func TestParseSkillVersionUpdateQuery_DistintoDeUpdateCheckYSkillBuild(t *testing.T) {
	if !ParseSkillVersionUpdateQuery(interaction.Analyze("actualiza la nota sobre go", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Skill Versioning (actualizar)")
	}
	if ParseSkillVersionUpdateQuery(interaction.Analyze("sigue actualizada la nota sobre go", interaction.Context{})) {
		t.Error("un pedido de Skill Update Detector (58) no debería disparar Skill Versioning")
	}
	if ParseSkillVersionUpdateQuery(interaction.Analyze("guarda una nota sobre go", interaction.Context{})) {
		t.Error("un pedido de Skill Build (54) no debería disparar Skill Versioning")
	}
}

// TestParseSkillVersionHistoryQuery_DistintoDeSkillBuilderQuery
// confirma que el trigger de historial no se pisa con Skill
// Builder (54, consulta la biblioteca vigente, no el historial).
func TestParseSkillVersionHistoryQuery_DistintoDeSkillBuilderQuery(t *testing.T) {
	if !ParseSkillVersionHistoryQuery(interaction.Analyze("cuantas versiones tiene la nota sobre go", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de historial de Skill Versioning")
	}
	if ParseSkillVersionHistoryQuery(interaction.Analyze("que notas de estudio tienes", interaction.Context{})) {
		t.Error("un pedido de Skill Builder (54) no debería disparar el historial de Skill Versioning")
	}
}

// TestVersionHistory_Describe_SinVersiones confirma el mensaje cuando
// no hay nada archivado todavía.
func TestVersionHistory_Describe_SinVersiones(t *testing.T) {
	h := &VersionHistory{SkillID: "Go"}
	desc := h.Describe()
	if !strings.Contains(desc, "sin versiones archivadas") {
		t.Errorf("esperaba el mensaje de \"sin versiones\", obtuve: %q", desc)
	}
}
