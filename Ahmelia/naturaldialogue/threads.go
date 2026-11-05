package naturaldialogue

import "strings"

type ThreadManager struct{ threads map[string]Thread }

func NewThreadManager() *ThreadManager { return &ThreadManager{threads: map[string]Thread{}} }
func (m *ThreadManager) Switch(topic string) Thread {
	id := strings.ToLower(strings.TrimSpace(topic))
	for k, t := range m.threads {
		t.Active = k == id
		m.threads[k] = t
	}
	t := m.threads[id]
	t.ID = id
	t.Topic = topic
	t.Active = true
	m.threads[id] = t
	return t
}
func (m *ThreadManager) List() []Thread {
	r := make([]Thread, 0, len(m.threads))
	for _, t := range m.threads {
		r = append(r, t)
	}
	return r
}
func Carryover(prevTopic, input string) []string {
	if prevTopic == "" || ExplicitNewTopic(input) {
		return nil
	}
	return []string{prevTopic}
}
