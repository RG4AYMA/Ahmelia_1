package planning

import (
	"fmt"
	"sort"
	"time"
)

// ScheduleItem es un elemento en la agenda mental
type ScheduleItem struct {
	ID           string
	Title        string
	Type         string // "goal", "task", "reminder"
	Priority     GoalPriority
	DueDate      time.Time
	CreatedAt    time.Time
	Status       string // "pending", "in_progress", "completed"
	TimeSlot     time.Time
	Estimated    time.Duration
	Dependencies []string // Otros items de los que depende
}

// Scheduler gestiona la agenda mental y priorización
type Scheduler struct {
	items       map[string]*ScheduleItem
	goals       map[string]*Goal
	currentItem *ScheduleItem
}

// NewScheduler crea un nuevo planificador de agenda
func NewScheduler() *Scheduler {
	return &Scheduler{
		items: make(map[string]*ScheduleItem),
		goals: make(map[string]*Goal),
	}
}

// AddItem añade un item a la agenda
func (s *Scheduler) AddItem(item *ScheduleItem) {
	s.items[item.ID] = item
}

// RemoveItem elimina un item de la agenda
func (s *Scheduler) RemoveItem(itemID string) {
	delete(s.items, itemID)
}

// GetPrioritizedList retorna los items ordenados por prioridad
func (s *Scheduler) GetPrioritizedList() []*ScheduleItem {
	items := []*ScheduleItem{}
	for _, item := range s.items {
		if item.Status != "completed" {
			items = append(items, item)
		}
	}

	// Ordenar por:
	// 1. Prioridad (descendente)
	// 2. Fecha de vencimiento (ascendente)
	// 3. Dependencias (primero los que no dependen de nada)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}

		if items[i].DueDate != items[j].DueDate {
			return items[i].DueDate.Before(items[j].DueDate)
		}

		return len(items[i].Dependencies) < len(items[j].Dependencies)
	})

	return items
}

// GetCriticalItems retorna solo los items críticos/urgentes
func (s *Scheduler) GetCriticalItems() []*ScheduleItem {
	critical := []*ScheduleItem{}
	for _, item := range s.items {
		if item.Status != "completed" && (item.Priority == PriorityCritical || item.Priority == PriorityUrgent) {
			critical = append(critical, item)
		}
	}
	return critical
}

// GetNextItem retorna el siguiente item a ejecutar
func (s *Scheduler) GetNextItem() *ScheduleItem {
	list := s.GetPrioritizedList()
	if len(list) == 0 {
		return nil
	}

	// Retornar el primer item que puede iniciarse
	for _, item := range list {
		if s.CanStartItem(item) {
			return item
		}
	}

	return nil
}

// CanStartItem verifica si todas las dependencias están completadas
func (s *Scheduler) CanStartItem(item *ScheduleItem) bool {
	for _, depID := range item.Dependencies {
		if depItem, ok := s.items[depID]; ok {
			if depItem.Status != "completed" {
				return false
			}
		}
	}
	return true
}

// StartItem marca un item como en progreso
func (s *Scheduler) StartItem(itemID string) error {
	if item, ok := s.items[itemID]; ok {
		if s.CanStartItem(item) {
			item.Status = "in_progress"
			item.TimeSlot = time.Now()
			s.currentItem = item
			return nil
		}
		return fmt.Errorf("item has unmet dependencies")
	}
	return fmt.Errorf("item not found")
}

// CompleteItem marca un item como completado
func (s *Scheduler) CompleteItem(itemID string) error {
	if item, ok := s.items[itemID]; ok {
		item.Status = "completed"
		if s.currentItem != nil && s.currentItem.ID == itemID {
			s.currentItem = nil
		}
		return nil
	}
	return fmt.Errorf("item not found")
}

// GetMentalAgenda retorna la agenda mental en formato texto
func (s *Scheduler) GetMentalAgenda() string {
	agenda := "📋 AGENDA MENTAL:\n\n"

	// Items activos
	critical := s.GetCriticalItems()
	if len(critical) > 0 {
		agenda += "🔴 CRÍTICO:\n"
		for _, item := range critical {
			agenda += fmt.Sprintf("  • %s (Vence: %s)\n", item.Title, item.DueDate.Format("02/01"))
		}
		agenda += "\n"
	}

	// Items prioritarios
	all := s.GetPrioritizedList()
	if len(all) > 0 {
		agenda += "📝 PRÓXIMAS TAREAS:\n"
		limit := 5
		if len(all) < limit {
			limit = len(all)
		}
		for i := 0; i < limit; i++ {
			item := all[i]
			status := "⏳"
			if item.Status == "in_progress" {
				status = "⚙️"
			}
			agenda += fmt.Sprintf("  %s %d. %s\n", status, i+1, item.Title)
		}
	}

	// Resumen
	completed := 0
	pending := 0
	inProgress := 0
	for _, item := range s.items {
		if item.Status == "completed" {
			completed++
		} else if item.Status == "in_progress" {
			inProgress++
		} else {
			pending++
		}
	}

	agenda += fmt.Sprintf("\n📊 RESUMEN: %d completadas, %d en progreso, %d pendientes\n", completed, inProgress, pending)

	return agenda
}

// DetectUrgencies detecta items que necesitan atención inmediata
func (s *Scheduler) DetectUrgencies() []string {
	urgencies := []string{}
	now := time.Now()

	for _, item := range s.items {
		if item.Status == "completed" {
			continue
		}

		// Menos de 1 hora al vencimiento
		if item.DueDate.Sub(now) < time.Hour && item.DueDate.Sub(now) > 0 {
			urgencies = append(urgencies, fmt.Sprintf("⚠️  %s vence en menos de 1 hora", item.Title))
		}

		// Vencido
		if item.DueDate.Before(now) {
			urgencies = append(urgencies, fmt.Sprintf("🔴 %s está VENCIDO", item.Title))
		}

		// Item crítico sin dependencias resueltas
		if item.Priority == PriorityCritical && !s.CanStartItem(item) {
			for _, depID := range item.Dependencies {
				if depItem, ok := s.items[depID]; ok {
					if depItem.Status != "completed" {
						urgencies = append(urgencies, fmt.Sprintf("🔴 %s bloqueado por: %s", item.Title, depItem.Title))
					}
				}
			}
		}
	}

	return urgencies
}

// EstimateWorkload estima la carga de trabajo total
func (s *Scheduler) EstimateWorkload() time.Duration {
	total := time.Duration(0)
	for _, item := range s.items {
		if item.Status != "completed" {
			total += item.Estimated
		}
	}
	return total
}

// GetContextForItem retorna el contexto completo de un item
func (s *Scheduler) GetContextForItem(itemID string) string {
	if item, ok := s.items[itemID]; ok {
		context := fmt.Sprintf("ITEM: %s\n", item.Title)
		context += fmt.Sprintf("Prioridad: %v\n", item.Priority)
		context += fmt.Sprintf("Estimado: %.1f horas\n", item.Estimated.Hours())
		context += fmt.Sprintf("Vencimiento: %s\n", item.DueDate.Format("02/01 15:04"))

		if len(item.Dependencies) > 0 {
			context += "\nDependencias:\n"
			for _, depID := range item.Dependencies {
				if depItem, ok := s.items[depID]; ok {
					context += fmt.Sprintf("  - %s (%s)\n", depItem.Title, depItem.Status)
				}
			}
		}

		return context
	}
	return ""
}

// GetCapacity retorna el porcentaje de capacidad disponible
func (s *Scheduler) GetCapacity() float64 {
	// Asumir 8 horas de trabajo disponibles
	workingHoursPerDay := 8.0

	totalPending := time.Duration(0)
	for _, item := range s.items {
		if item.Status != "completed" {
			totalPending += item.Estimated
		}
	}

	pendingHours := totalPending.Hours()
	capacity := 1.0 - (pendingHours / workingHoursPerDay)

	if capacity < 0 {
		capacity = 0
	}
	if capacity > 1 {
		capacity = 1
	}

	return capacity
}

// SuggestLoad da recomendaciones sobre la carga
func (s *Scheduler) SuggestLoad() string {
	capacity := s.GetCapacity()
	workload := s.EstimateWorkload()

	if capacity > 0.7 {
		return fmt.Sprintf("✅ Tienes capacidad disponible (%.0f%%). Puedes asumir más tareas.", capacity*100)
	} else if capacity > 0.3 {
		return fmt.Sprintf("⚠️  Estás algo ocupado (%.0f%% libre). Sé selectivo.", capacity*100)
	} else {
		return fmt.Sprintf("🔴 SOBRECARGADO. Solo %.0f%% de capacidad libre. Workload: %.1f horas.", capacity*100, workload.Hours())
	}
}
