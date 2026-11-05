package memory

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"time"

	"ahmelia/shared"
)

const LongMemoryPath = "memory/long.json"

/* =======================
   TIPOS
======================= */

type KnowType string

const (
	KnowCommand    KnowType = "comando"     // Algo que Ahmelia ejecuta
	KnowPreference KnowType = "preferencia" // Gustos/hábitos de Adrian
	KnowError      KnowType = "error"       // Algo que salió mal
	KnowConcept    KnowType = "concepto"    // Hecho/dato enseñado por Adrian
	KnowContext    KnowType = "contexto"    // Resumen de una sesión
	KnowObjective  KnowType = "objetivo"    // Meta declarada por Adrian
)

type KnowSource string

const (
	SourceTaught   KnowSource = "enseñado"  // Adrian lo enseñó explícitamente
	SourceLearned  KnowSource = "aprendido" // Ahmelia lo dedujo del uso
	SourceInferred KnowSource = "inferido"  // Ahmelia lo infirió del contexto
)

/* =======================
   ESTRUCTURA PRINCIPAL
======================= */

type Knowledge struct {
	// --- Identidad ---
	ID    string   `json:"id"`
	Type  KnowType `json:"type"`
	Input string   `json:"input"`

	// --- Acción (para comandos) ---
	Response   string            `json:"response"`   // Legacy
	Action     string            `json:"action"`
	Target     string            `json:"target"`
	Parameters map[string]string `json:"parameters"`

	// --- Metadatos de calidad ---
	Importance float64    `json:"importance"` // 0.0 - 1.0
	Confidence float64    `json:"confidence"` // 0.0 - 1.0
	Source     KnowSource `json:"source"`

	// --- Uso ---
	Uses       int       `json:"uses"`
	LastUsed   time.Time `json:"last_used"`
	CreatedAt  time.Time `json:"created_at"`

	// --- Relaciones ---
	Tags      []string `json:"tags"`      // Temas: ["volumen", "audio", "sistema"]
	Relations []string `json:"relations"` // IDs de Knowledge relacionados
}

type LongMemory struct {
	Data    map[string]*Knowledge `json:"data"`
	Version int                   `json:"version"` // Para migraciones futuras
}

/* =======================
   CARGAR / GUARDAR
======================= */

func LoadLongMemory() *LongMemory {
	mem := &LongMemory{
		Data:    make(map[string]*Knowledge),
		Version: 2,
	}

	file, err := os.ReadFile(LongMemoryPath)
	if err == nil {
		if jerr := json.Unmarshal(file, mem); jerr != nil {
			// B96.45: un archivo ilegible se APARTA, nunca se sobrescribe en silencio.
			_, _ = QuarantineCorrupt(LongMemoryPath)
			mem = &LongMemory{Data: make(map[string]*Knowledge), Version: 2}
			if bk, berr := os.ReadFile(LongMemoryPath + ".bak"); berr == nil {
				if json.Unmarshal(bk, mem) != nil {
					mem = &LongMemory{Data: make(map[string]*Knowledge), Version: 2}
				}
			}
		}
		mem.migrate()
		mem.migrateVolumeTargets()
	}

	return mem
}

// SaveErr persiste de forma atómica y devuelve el error (Save lo descartaba).
func (m *LongMemory) SaveErr() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(LongMemoryPath, data, 0o644)
}

func (m *LongMemory) Save() { _ = m.SaveErr() }

func SaveLongMemory(m *LongMemory) {
	m.Save()
}

/* =======================
   MIGRACIÓN AUTOMÁTICA
   Convierte entradas planas (v1) al formato nuevo (v2)
   sin perder ningún dato existente.
======================= */

func (m *LongMemory) migrate() {
	changed := false
	for key, k := range m.Data {
		// Si ya tiene ID asignado, ya fue migrado
		if k.ID != "" {
			continue
		}

		k.ID = key
		k.CreatedAt = k.LastUsed // mejor estimación disponible

		// Inferir Type desde la acción
		switch {
		case k.Action != "":
			k.Type = KnowCommand
		case k.Response != "":
			k.Type = KnowConcept
		default:
			k.Type = KnowCommand
		}

		// Confidence inicial basada en uso
		k.Confidence = math.Min(0.5+float64(k.Uses)*0.05, 0.95)

		// Importancia inicial — errores conocidos = alta
		k.Importance = 0.5
		if k.Type == KnowError {
			k.Importance = 0.9
		}

		k.Source = SourceTaught
		k.Tags = shared.InferTags(k.Input, k.Action)
		k.Relations = []string{}

		changed = true
	}

	if changed {
		m.Version = 2
		m.Save()
	}
}

/* =======================
   MIGRACIÓN: TARGETS DE VOLUMEN (v2 -> v3)
   El fix de extractTarget (classifier.go) hace que las acciones
   volumen_subir/bajar/silencio/max/set devuelvan Target="" desde ahora,
   en vez del input completo (bug que rompía RecallBySameCommand/Relate,
   ver comentario en extractTarget). Pero ese fix solo afecta comandos
   NUEVOS: cualquier entrada guardada en long.json ANTES del fix (por
   RecordUsage o por enseñanza explícita) quedó con el Target viejo
   ("baja el volumen", "baja", etc.) y no se autocorrige sola — para que
   se actualizara haría falta volver a ejecutar exactamente esa frase
   por voz. Este paso normaliza esas entradas una sola vez, para que
   Relate()/RecallBySameCommand encuentren coincidencias entre comandos
   de volumen guardados antes y después del fix.
======================= */

var volumeActions = map[string]bool{
	"volumen_subir":    true,
	"volumen_bajar":    true,
	"volumen_silencio": true,
	"volumen_max":      true,
	"volumen_set":      true,
}

func (m *LongMemory) migrateVolumeTargets() {
	if m.Version >= 3 {
		return
	}

	for _, k := range m.Data {
		if volumeActions[k.Action] && k.Target != "" {
			k.Target = ""
		}
	}

	// Se guarda siempre (haya habido o no entradas que corregir) para
	// que Version=3 quede en disco y esta migración no se repita en
	// cada arranque comparando contra un archivo que nunca la registró.
	m.Version = 3
	m.Save()
}

/* =======================
   APRENDER ACCIÓN
======================= */

/* =======================
   REGISTRAR USO (automático, no enseñanza)
   A diferencia de LearnAction (pensado para el flujo explícito de
   "enséñame algo", vía sess.LearningInput), esto se llama en CADA
   comando que se ejecuta con éxito — es lo que hace falta para que
   memory_intelligence/ tenga algo real que recordar. Se marca
   Source: SourceLearned (Ahmelia lo dedujo de usarlo), no SourceTaught
   (nadie se lo enseñó a propósito) — la distinción ya existía en el
   tipo KnowSource, solo faltaba un método que la usara correctamente
   para este caso.
======================= */

func (m *LongMemory) RecordUsage(input, action, target string, params map[string]string) {
	key := shared.Normalize(input)

	if k, ok := m.Data[key]; ok {
		k.Action = action
		k.Target = target
		k.Parameters = params
		k.Uses++
		k.LastUsed = time.Now()
		k.Confidence = math.Min(k.Confidence+0.03, 0.97)
		if len(k.Tags) == 0 {
			k.Tags = shared.InferTags(input, action)
		}
	} else {
		m.Data[key] = &Knowledge{
			ID:         key,
			Type:       KnowCommand,
			Input:      input,
			Action:     action,
			Target:     target,
			Parameters: params,
			Importance: 0.4,
			Confidence: 0.6,
			Source:     SourceLearned,
			Uses:       1,
			LastUsed:   time.Now(),
			CreatedAt:  time.Now(),
			Tags:       shared.InferTags(input, action),
			Relations:  []string{},
		}
	}

	m.Save()
}

/* =======================
   APRENDER ACCIÓN (ENSEÑANZA EXPLÍCITA)
======================= */

func (m *LongMemory) LearnAction(input, action, target string, params map[string]string) {
	key := shared.Normalize(input)

	if k, ok := m.Data[key]; ok {
		k.Action = action
		k.Target = target
		k.Parameters = params
		k.Uses++
		k.LastUsed = time.Now()
		// Confianza sube con cada uso exitoso, tope en 0.97
		k.Confidence = math.Min(k.Confidence+0.03, 0.97)
		k.Tags = shared.InferTags(input, action)
	} else {
		m.Data[key] = &Knowledge{
			ID:         key,
			Type:       KnowCommand,
			Input:      input,
			Action:     action,
			Target:     target,
			Parameters: params,
			Importance: 0.5,
			Confidence: 0.6,
			Source:     SourceTaught,
			Uses:       1,
			LastUsed:   time.Now(),
			CreatedAt:  time.Now(),
			Tags:       shared.InferTags(input, action),
			Relations:  []string{},
		}
	}

	m.Save()
}

/* =======================
   APRENDER TEXTO (legacy)
======================= */

func (m *LongMemory) Learn(input, response string) {
	key := shared.Normalize(input)

	if k, ok := m.Data[key]; ok {
		k.Response = response
		k.Uses++
		k.LastUsed = time.Now()
		k.Confidence = math.Min(k.Confidence+0.03, 0.97)
	} else {
		m.Data[key] = &Knowledge{
			ID:        key,
			Type:      KnowConcept,
			Input:     input,
			Response:  response,
			Importance: 0.4,
			Confidence: 0.5,
			Source:    SourceTaught,
			Uses:      1,
			LastUsed:  time.Now(),
			CreatedAt: time.Now(),
			Tags:      shared.InferTags(input, ""),
			Relations: []string{},
		}
	}

	m.Save()
}

/* =======================
   REGISTRAR ERROR
   Cuando una acción falla, la Capa 3 de Razonamiento
   puede consultarlo para no repetir el mismo error.
======================= */

func (m *LongMemory) RegisterError(input, action, reason string) {
	key := "error_" + shared.Normalize(input)

	m.Data[key] = &Knowledge{
		ID:         key,
		Type:       KnowError,
		Input:      input,
		Action:     action,
		Response:   reason,
		Importance: 0.9, // Errores = alta importancia
		Confidence: 0.0,
		Source:     SourceLearned,
		Uses:       1,
		LastUsed:   time.Now(),
		CreatedAt:  time.Now(),
		Tags:       shared.InferTags(input, action),
		Relations:  []string{},
	}

	m.Save()
}

/* =======================
   RECORDAR
======================= */

func (m *LongMemory) Recall(input string) (*Knowledge, bool) {
	key := shared.Normalize(input)

	if k, ok := m.Data[key]; ok {
		k.Uses++
		k.LastUsed = time.Now()
		m.Save()
		return k, true
	}

	return nil, false
}

/* =======================
   BUSCAR POR TAG
   Para la Capa 3 — recuperar todo lo relacionado a un tema.
   Ej: RecallByTag("audio") → todos los Knowledge de volumen
======================= */

/* =======================
   BUSCAR POR TARGET
   Los Tags son por categoría ("programa", "audio" — ver TagMappings),
   NUNCA por nombre de app o archivo. Para "¿qué hicimos en Unity?" hace
   falta buscar por lo que de verdad guarda el nombre: Knowledge.Target
   (ej: "unity"), no Tags.
======================= */

func (m *LongMemory) RecallByTarget(target string) []*Knowledge {
	target = shared.Normalize(target)
	results := []*Knowledge{}
	if target == "" {
		return results
	}
	for _, k := range m.Data {
		if strings.Contains(shared.Normalize(k.Target), target) {
			results = append(results, k)
		}
	}
	return results
}

/* =======================
   BUSCAR POR TAG
======================= */

func (m *LongMemory) RecallByTag(tag string) []*Knowledge {
	results := []*Knowledge{}
	for _, k := range m.Data {
		for _, t := range k.Tags {
			if t == tag {
				results = append(results, k)
				break
			}
		}
	}
	return results
}

/* =======================
   BUSCAR POR TIPO
======================= */

func (m *LongMemory) RecallByType(ktype KnowType) []*Knowledge {
	results := []*Knowledge{}
	for _, k := range m.Data {
		if k.Type == ktype {
			results = append(results, k)
		}
	}
	return results
}

/* =======================
   BUSCAR POR MISMA ACCIÓN+TARGET
   Para conectar automáticamente comandos que hacen lo mismo dicho de
   formas distintas (Bloque 2, Nivel 2 — Relaciones). A diferencia de
   RecallByTag (por tema) o RecallByTarget (por texto parcial de
   programa/archivo), esto es exacto: dos Knowledge con el mismo
   Action+Target SON el mismo comando, sin importar qué tan distinto
   se vea el texto ("sube el volumen" vs "más fuerte").
======================= */

func (m *LongMemory) RecallBySameCommand(action, target string) []*Knowledge {
	results := []*Knowledge{}
	if action == "" {
		return results
	}
	for _, k := range m.Data {
		if k.Action == action && k.Target == target {
			results = append(results, k)
		}
	}
	return results
}

/* =======================
   BUSCAR POR ID
   Relations guarda IDs (claves normalizadas), no objetos — para
   recorrer de un Knowledge a sus relacionados hace falta resolver
   cada ID de vuelta a su Knowledge. A diferencia de Recall(), esto
   NO cuenta como un "uso" (no incrementa Uses ni pisa LastUsed):
   es solo navegación, no una consulta directa del usuario.
======================= */

func (m *LongMemory) GetByID(id string) (*Knowledge, bool) {
	k, ok := m.Data[id]
	return k, ok
}

/* =======================
   RECORDAR CON CONFIANZA MÍNIMA
   La Capa 3 puede pedir solo conocimiento confiable.
   Ej: RecallConfident("sube el volumen", 0.7)
======================= */

func (m *LongMemory) RecallConfident(input string, minConfidence float64) (*Knowledge, bool) {
	k, ok := m.Recall(input)
	if !ok {
		return nil, false
	}
	if k.Confidence < minConfidence {
		return nil, false
	}
	return k, true
}

/* =======================
   ACTUALIZAR CONFIANZA
   Llamar desde engine cuando una acción falla:
   longMem.UpdateConfidence(input, false)
======================= */

func (m *LongMemory) UpdateConfidence(input string, success bool) {
	key := shared.Normalize(input)
	k, ok := m.Data[key]
	if !ok {
		return
	}

	if success {
		k.Confidence = math.Min(k.Confidence+0.05, 0.97)
	} else {
		k.Confidence = math.Max(k.Confidence-0.1, 0.0)
		if k.Confidence < 0.2 {
			k.Importance = math.Min(k.Importance+0.1, 1.0) // error frecuente = más importante recordar
		}
	}

	m.Save()
}

/* =======================
   RELACIONAR
   Conecta dos Knowledge entre sí para la Capa 3.
   Ej: Relate("baja", "baja el volumen")
======================= */

func (m *LongMemory) Relate(inputA, inputB string) {
	keyA := shared.Normalize(inputA)
	keyB := shared.Normalize(inputB)

	if kA, ok := m.Data[keyA]; ok {
		if !shared.Contains(kA.Relations, keyB) {
			kA.Relations = append(kA.Relations, keyB)
		}
	}
	if kB, ok := m.Data[keyB]; ok {
		if !shared.Contains(kB.Relations, keyA) {
			kB.Relations = append(kB.Relations, keyA)
		}
	}

	m.Save()
}



/* =======================
   ESTADÍSTICAS
======================= */

func (m *LongMemory) GetStats() map[string]interface{} {
	totalUses := 0
	byType := map[string]int{}
	avgConfidence := 0.0

	for _, k := range m.Data {
		totalUses += k.Uses
		byType[string(k.Type)]++
		avgConfidence += k.Confidence
	}

	total := len(m.Data)
	if total > 0 {
		avgConfidence /= float64(total)
	}

	return map[string]interface{}{
		"total_entries":    total,
		"total_uses":       totalUses,
		"by_type":          byType,
		"avg_confidence":   math.Round(avgConfidence*100) / 100,
	}
}

func (m *LongMemory) Clear() {
	m.Data = make(map[string]*Knowledge)
	m.Save()
}
