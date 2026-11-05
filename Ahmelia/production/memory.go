package production

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"ahmelia/memory"
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/planning"
)

// B95.13 — Production Memory
//
// Production does not invent another memory database. This boundary adapts
// the existing LongMemory/EventLog so production can persist outcomes and
// recoverable experience while the existing memory systems remain owners of
// their data.

type ProductionMemoryRecord struct {
	ProductionID string
	GoalID       string
	TaskID       string
	Action       string
	Target       string
	Success      bool
	Summary      string
	Timestamp    time.Time
}

type ProductionMemory interface {
	Remember(ProductionMemoryRecord) error
	RecallProduction(string) []ProductionMemoryRecord
	RecallTask(string) []ProductionMemoryRecord
}

// MemoryAdapter is the Production boundary over existing memory systems.
// LongMemory stores durable knowledge; EventLog stores the raw outcome trail.
type MemoryAdapter struct {
	mu       sync.RWMutex
	long     *memory.LongMemory
	eventLog *memoryintelligence.EventLog
}

func NewMemoryAdapter(long *memory.LongMemory, eventLog *memoryintelligence.EventLog) (*MemoryAdapter, error) {
	if long == nil {
		return nil, fmt.Errorf("production memory: long memory nil")
	}
	if eventLog == nil {
		return nil, fmt.Errorf("production memory: event log nil")
	}
	if long.Data == nil {
		long.Data = make(map[string]*memory.Knowledge)
	}
	return &MemoryAdapter{long: long, eventLog: eventLog}, nil
}

func (m *MemoryAdapter) Remember(record ProductionMemoryRecord) error {
	if m == nil {
		return fmt.Errorf("production memory: adapter nil")
	}
	if record.ProductionID == "" || record.GoalID == "" || record.TaskID == "" {
		return fmt.Errorf("production memory: production, goal y task son obligatorios")
	}
	if record.Action == "" {
		return fmt.Errorf("production memory: action vacía")
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := productionMemoryKey(record.ProductionID, record.TaskID)
	existing, ok := m.long.Data[key]
	if !ok {
		existing = &memory.Knowledge{
			ID:         key,
			Type:       memory.KnowContext,
			Input:      record.Summary,
			Importance: 0.6,
			Confidence: 0.5,
			Source:     memory.SourceLearned,
			CreatedAt:  record.Timestamp,
			Tags:       []string{"production", "task:" + record.TaskID, "goal:" + record.GoalID},
			Relations:  []string{},
		}
		m.long.Data[key] = existing
	}

	existing.Input = record.Summary
	existing.Action = record.Action
	existing.Target = record.Target
	existing.Uses++
	existing.LastUsed = record.Timestamp
	if record.Success {
		existing.Confidence = minFloat(existing.Confidence+0.1, 0.97)
	} else {
		existing.Type = memory.KnowError
		existing.Confidence = minFloat(existing.Confidence, 0.2)
		existing.Response = record.Summary
	}
	existing.Parameters = map[string]string{
		"production_id": record.ProductionID,
		"goal_id":       record.GoalID,
		"task_id":       record.TaskID,
	}

	outcome := memoryintelligence.Outcome{Success: record.Success}
	if !record.Success {
		outcome.Category = "production_failure"
		outcome.Reason = record.Summary
	}
	m.eventLog.Record(record.Action, record.Target, outcome, record.ProductionID)
	m.long.Save()
	return nil
}

func (m *MemoryAdapter) RecallProduction(productionID string) []ProductionMemoryRecord {
	if m == nil || productionID == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recallLocked(func(k *memory.Knowledge) bool {
		return k.Parameters["production_id"] == productionID
	})
}

func (m *MemoryAdapter) RecallTask(taskID string) []ProductionMemoryRecord {
	if m == nil || taskID == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recallLocked(func(k *memory.Knowledge) bool {
		return k.Parameters["task_id"] == taskID
	})
}

func (m *MemoryAdapter) recallLocked(match func(*memory.Knowledge) bool) []ProductionMemoryRecord {
	out := make([]ProductionMemoryRecord, 0)
	for _, k := range m.long.Data {
		if k == nil || !match(k) {
			continue
		}
		success := k.Type != memory.KnowError
		out = append(out, ProductionMemoryRecord{
			ProductionID: k.Parameters["production_id"],
			GoalID:       k.Parameters["goal_id"],
			TaskID:       k.Parameters["task_id"],
			Action:       k.Action,
			Target:       k.Target,
			Success:      success,
			Summary:      k.Input,
			Timestamp:    k.LastUsed,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].TaskID < out[j].TaskID
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

func productionMemoryKey(productionID, taskID string) string {
	clean := func(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), " ", "_") }
	return "production_" + clean(productionID) + "_task_" + clean(taskID)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// BindProductionMemory creates the adapter using the already-owned memory
// instances. Production receives a boundary, not ownership of either store.
func BindProductionMemory(long *memory.LongMemory, eventLog *memoryintelligence.EventLog) (ProductionMemory, error) {
	return NewMemoryAdapter(long, eventLog)
}

// MemorySummary exposes the amount of production knowledge persisted without
// leaking the underlying storage representation.
type MemorySummary struct {
	ProductionID string
	Records      int
}

func SummarizeProductionMemory(mem ProductionMemory, productionID string) (MemorySummary, error) {
	if mem == nil {
		return MemorySummary{}, fmt.Errorf("production memory: memory nil")
	}
	if productionID == "" {
		return MemorySummary{}, fmt.Errorf("production memory: production ID vacío")
	}
	records := mem.RecallProduction(productionID)
	return MemorySummary{ProductionID: productionID, Records: len(records)}, nil
}

// Keep planning imported by the production memory boundary's public
// documentation and ensure this block remains conceptually tied to existing
// production Tasks rather than creating a second task model.
var _ *planning.Task
