package linguistics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaGenerateUsesInterpretationJSONSchema(t *testing.T) {
	var captured generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"{\"intent\":\"question\",\"entities\":[],\"relations\":[],\"references\":[],\"constraints\":[],\"meaning\":\"test\",\"confidence\":1,\"speech_act\":\"question\",\"topic\":\"\",\"question_type\":\"general\",\"directness\":\"question\",\"user_state_signals\":[],\"correction_target\":\"\"}"}`))
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	client.Model = "test"
	client.client = server.Client()

	if _, err := client.GenerateContext(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}

	if string(captured.Format) == `"json"` || len(captured.Format) == 0 {
		t.Fatalf("expected JSON Schema in format, got %s", string(captured.Format))
	}
	var schema struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties"`
		Required   []string               `json:"required"`
	}
	if err := json.Unmarshal(captured.Format, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" {
		t.Fatalf("schema type=%q", schema.Type)
	}
	for _, field := range []string{"intent", "entities", "relations", "meaning", "confidence"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("schema missing %q", field)
		}
	}
	if len(schema.Required) != 13 {
		t.Fatalf("required fields=%d, want 13", len(schema.Required))
	}
}
