package interaction

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateInteractionSemanticBoundary(t *testing.T) {
	data, err := os.ReadFile("semantic_queries.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: this file is the single intentional lexical boundary where
	// human wording becomes a typed SemanticQueryType. It is not a decision or
	// response gate; it only projects language into the existing Meaning contract.
	if !strings.Contains(src, "func classifySemanticQuery(n string) SemanticQueryType") {
		t.Fatal("canonical semantic query boundary missing")
	}
	if strings.Count(src, "func classifySemanticQuery(n string) SemanticQueryType") != 1 {
		t.Fatal("multiple semantic query classifiers detected")
	}
}

func TestSemanticQueryUnseenVariantsAndEnvironmentBoundary(t *testing.T) {
	cases := []struct {
		input     string
		query     SemanticQueryType
		attribute string
		temporal  bool
		topic     string
	}{
		{"analiza este objetivo: crear un juego", QuerySolvingGoalAnalysis, "", false, "crear un juego"},
		{"¿qué hicimos ayer en Unity?", QueryHistoryTimeline, "", true, "unity"},
		{"¿cuánta RAM está usando?", QueryEnvironmentResources, "ram", false, "usando"},
		{"¿qué ventanas hay abiertas?", QueryEnvironmentOpenApps, "open_apps", false, ""},
		{"¿en qué proyecto estoy trabajando?", QueryEnvironmentActiveWork, "active_project", false, ""},
		{"¿en qué archivo estoy trabajando?", QueryEnvironmentActiveWork, "active_file", false, ""},
	}
	for _, tc := range cases {
		r := Analyze(tc.input, Context{})
		if r.Meaning.Query != tc.query {
			t.Errorf("%q: query=%q want %q", tc.input, r.Meaning.Query, tc.query)
		}
		if r.Meaning.Attribute != tc.attribute {
			t.Errorf("%q: attribute=%q want %q", tc.input, r.Meaning.Attribute, tc.attribute)
		}
		if r.Temporal.Known != tc.temporal {
			t.Errorf("%q: temporal known=%t want %t", tc.input, r.Temporal.Known, tc.temporal)
		}
		if tc.topic != "" && !strings.Contains(strings.ToLower(r.Topic), tc.topic) {
			t.Errorf("%q: topic=%q want it to retain %q", tc.input, r.Topic, tc.topic)
		}
	}

	// G3.3 false-positive guard: the C category classifier must not be
	// hijacked by a generic "sistema" phrase while the environment query lane
	// is deciding whether the request is actually about open applications.
	for _, input := range []string{
		"que programas de sistema tienes abiertos",
		"que aplicaciones del sistema tienes abiertas",
	} {
		r := Analyze(input, Context{})
		if r.Meaning.Query == QueryEnvironmentOpenApps {
			t.Errorf("%q incorrectly claimed by environment_open_apps", input)
		}
	}
}
