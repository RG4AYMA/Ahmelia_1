package core

import (
	"os"
	"strings"
	"testing"
)

func TestB9635_EvidenceAuthorityAndReferenceBridge(t *testing.T) {
	resolver, err := os.ReadFile("../dialogue/historical_memory_resolver.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(resolver)
	for _, required := range []string{
		"EvidencePrimary", "EvidenceSecondary", "EvidenceDerived", "EvidenceMeta", "EvidenceNonEvidence",
		"ResolveHistoricalReferenceQuery", "isHistoricalRetrievalMeta", "EvidenceQuestion",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("B96.35 authority boundary missing %q", required)
		}
	}
	engine, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	et := string(engine)
	collector := strings.Index(et, "t.responseSpecialistResults = e.collectResponseSpecialistResults(")
	contribution := strings.Index(et, "t.responseSpecialistResults.Contributions(time.Now())")
	if collector < 0 || contribution < 0 {
		t.Fatal("B96.35 historical evidence is not collected before it becomes a Situation contribution")
	}
	process, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	pt := string(process)
	sequence := []string{"e.evidencePhase(t)", "e.situationPhase(t)", "e.cognitionPhase(t)", "e.dialoguePhase(t)", "e.decisionPhase(t)"}
	last := -1
	for _, stage := range sequence {
		current := strings.Index(pt, stage)
		if current <= last {
			t.Fatalf("historical evidence no longer follows shared turn lifecycle order at %q", stage)
		}
		last = current
	}
	specialist := strings.Index(pt, "e.renderSpecialistResponseResults(t.semanticReq, t.responseSpecialistResults)")
	responseGate := strings.Index(pt, "if conversationalResponseEligible(t.semanticReq, e.lastResponseIntent)")
	if specialist < 0 || responseGate < 0 || specialist > responseGate {
		t.Fatal("historical evidence is not handled before the generic response fallback")
	}
	resolver, err = os.ReadFile("../core/response_specialist_results.go")
	if err != nil {
		t.Fatal(err)
	}
	rt := string(resolver)
	for _, required := range []string{"ResolveHistoricalReferenceQuery", "ResolveHistoricalRecallExcluding", "conversationEpisodeID"} {
		if !strings.Contains(rt, required) {
			t.Fatalf("B96.35 collector/result layer missing %q", required)
		}
	}
}
