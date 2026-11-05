package system

import (
	"ahmelia/comprehension"
	"ahmelia/learning"
	"ahmelia/memory"
)

// LearningSystem es una fachada sobre learning.Manager. Existe para no
// romper los imports y llamadas que ya hace Engine (system.NewLearningSystem,
// e.learner.LearnAction, e.learner.Ask, e.learner.ReportSuccess/Failure, etc.).
//
// Cuando el resto del proyecto llame directamente a learning.NewManager(),
// este archivo puede eliminarse sin dejar nada huérfano.
type LearningSystem struct {
	manager *learning.Manager
}

// NewLearningSystem crea la fachada envolviendo el Manager real.
func NewLearningSystem(mem *memory.LongMemory) *LearningSystem {
	return &LearningSystem{manager: learning.NewManager(mem)}
}

func (l *LearningSystem) Ask(input string) string {
	return l.manager.Ask(input)
}

func (l *LearningSystem) Learn(input, response string) {
	l.manager.Learn(input, response)
}

func (l *LearningSystem) LearnAction(input, action, target string, params map[string]string) {
	l.manager.LearnAction(input, action, target, params)
}

func (l *LearningSystem) GetKnowledge(input string) (*memory.Knowledge, bool) {
	return l.manager.GetKnowledge(input)
}

func (l *LearningSystem) GetConfident(input string, minConfidence float64) (*memory.Knowledge, bool) {
	return l.manager.GetConfident(input, minConfidence)
}

func (l *LearningSystem) GetByTag(tag string) []*memory.Knowledge {
	return l.manager.GetByTag(tag)
}

func (l *LearningSystem) GetByType(ktype memory.KnowType) []*memory.Knowledge {
	return l.manager.GetByType(ktype)
}

func (l *LearningSystem) ReportSuccess(input string) {
	l.manager.ReportSuccess(input)
}

func (l *LearningSystem) ReportFailure(input, action, reason string) {
	l.manager.ReportFailure(input, action, reason)
}

func (l *LearningSystem) Relate(inputA, inputB string) {
	l.manager.Relate(inputA, inputB)
}

func (l *LearningSystem) GetStats() map[string]interface{} {
	return l.manager.GetStats()
}

func (l *LearningSystem) DumpContextToMemory(interactions []comprehension.Interaction) {
	l.manager.DumpContextToMemory(interactions)
}

func (l *LearningSystem) LoadContextFromMemory() []comprehension.Interaction {
	return l.manager.LoadContextFromMemory()
}
