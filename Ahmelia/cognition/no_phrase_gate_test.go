package cognition

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateCognitionG2Conflict(t *testing.T) {
	data, err := os.ReadFile("conversational_conflict.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: provenance identity may be derived from typed fields; the G2
	// detector itself must never read Raw/Normalized or use phrase matching.
	if strings.Contains(src, "model.Request.Raw") || strings.Contains(src, "model.Request.Normalized") {
		t.Fatal("G2 conflict detector reads raw/normalized user text")
	}
	if strings.Contains(src, "strings.Contains(model.Request") || strings.Contains(src, "strings.HasPrefix(model.Request") {
		t.Fatal("G2 conflict detector contains a raw-language phrase gate")
	}
	if !strings.Contains(src, "contradicts_authoritative_evidence") || !strings.Contains(src, "EvidenceIDs") {
		t.Fatal("G2 structural finding contract is missing")
	}
}
