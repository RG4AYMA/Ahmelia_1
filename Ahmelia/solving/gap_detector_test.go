package solving

import (
	"testing"
)

// MockSystemCapabilities simula las respuestas del sistema para pruebas unitarias.
type MockSystemCapabilities struct {
	skills     map[string]bool
	executable map[string]bool
	resources  map[string]bool
}

func (m *MockSystemCapabilities) HasSkill(action string) bool {
	return m.skills[action]
}

func (m *MockSystemCapabilities) HasExecutableAction(action string) bool {
	return m.executable[action]
}

func (m *MockSystemCapabilities) HasResource(resourceKey string) (bool, string) {
	if m.resources[resourceKey] {
		return true, ""
	}
	return false, "recurso no encontrado en el sistema"
}

// TestDetectGap_KnowledgeGap confirma que si no existe la Skill se marca KnowledgeGap.
func TestDetectGap_KnowledgeGap(t *testing.T) {
	sys := &MockSystemCapabilities{
		skills:     map[string]bool{"crear_carpeta": false},
		executable: map[string]bool{"crear_carpeta": true},
	}

	gap := DetectGap("crear_carpeta", nil, sys)

	if gap.Type != KnowledgeGap {
		t.Errorf("esperaba KnowledgeGap, dio %q", gap.Type)
	}
	if !gap.HasGap() {
		t.Error("HasGap() debería ser verdadero")
	}
}

// TestDetectGap_CapabilityGap confirma que si existe la Skill pero no la función local, es CapabilityGap.
func TestDetectGap_CapabilityGap(t *testing.T) {
	sys := &MockSystemCapabilities{
		skills:     map[string]bool{"reproducir_audio": true},
		executable: map[string]bool{"reproducir_audio": false}, // No hay código en Go aún
	}

	gap := DetectGap("reproducir_audio", nil, sys)

	if gap.Type != CapabilityGap {
		t.Errorf("esperaba CapabilityGap, dio %q", gap.Type)
	}
}

// TestDetectGap_ResourceGap confirma que si falta un archivo/binario, es ResourceGap.
func TestDetectGap_ResourceGap(t *testing.T) {
	sys := &MockSystemCapabilities{
		skills:     map[string]bool{"convertir_video": true},
		executable: map[string]bool{"convertir_video": true},
		resources:  map[string]bool{"ffmpeg": false}, // Falta el ejecutable en el SO
	}

	gap := DetectGap("convertir_video", []string{"ffmpeg"}, sys)

	if gap.Type != ResourceGap {
		t.Errorf("esperaba ResourceGap, dio %q", gap.Type)
	}
}

// TestDetectGap_SinGap confirma el flujo sin bloqueos.
func TestDetectGap_SinGap(t *testing.T) {
	sys := &MockSystemCapabilities{
		skills:     map[string]bool{"mover_archivo": true},
		executable: map[string]bool{"mover_archivo": true},
		resources:  map[string]bool{"archivo.txt": true},
	}

	gap := DetectGap("mover_archivo", []string{"archivo.txt"}, sys)

	if gap.HasGap() {
		t.Errorf("no se esperaba ninguna brecha, pero se detectó %q", gap.Type)
	}
}
