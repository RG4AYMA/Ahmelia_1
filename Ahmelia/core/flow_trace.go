package core

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"ahmelia/interaction"
)

// TurnFlowTrace is an intentionally small diagnostic trace for one live human
// turn. It observes the data/state boundary; it never participates in a
// decision. Enable with AHMELIA_FLOW_TRACE=1.
type TurnFlowTrace struct {
	mu     sync.Mutex
	Input  string
	Start  time.Time
	stages []string
}

func NewTurnFlowTrace(input string) *TurnFlowTrace {
	return &TurnFlowTrace{Input: input, Start: time.Now(), stages: make([]string, 0, 16)}
}

func flowTraceEnabled() bool { return os.Getenv("AHMELIA_FLOW_TRACE") == "1" }

func (t *TurnFlowTrace) Mark(stage, detail string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	detail = strings.TrimSpace(detail)
	if detail != "" {
		t.stages = append(t.stages, fmt.Sprintf("[%02d] %s | %s", len(t.stages)+1, stage, detail))
	} else {
		t.stages = append(t.stages, fmt.Sprintf("[%02d] %s", len(t.stages)+1, stage))
	}
}

func (t *TurnFlowTrace) Lines() []string {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.stages...)
}

func (t *TurnFlowTrace) Snapshot() []string {
	return t.Lines()
}

func (t *TurnFlowTrace) PrintFinal() {
	if t == nil {
		return
	}
	for _, line := range t.Lines() {
		fmt.Println("🔎", line)
	}
}

func formatActs(acts interaction.CommunicativeActs) string {
	secondary := make([]string, len(acts.Secondary))
	for i, a := range acts.Secondary {
		secondary[i] = string(a)
	}
	return fmt.Sprintf("primary=%s secondary=%s confidence=%.2f ambiguous=%t", acts.Primary, strings.Join(secondary, ","), acts.Confidence, acts.Ambiguous)
}
