package programming

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestEvaluateCreateCompatibilityStates(t *testing.T) {
	change := MultiFileChange{
		ID:        "main",
		Operation: SourceCreate,
		File:      SourceFile{Path: "main.go", ProjectID: "p"},
		Content:   "package main\n",
	}

	correct := ObservedFile{
		Source: "filesystem",
		Path:   "main.go",
		Exists: true,
	}
	sum := sha256.Sum256([]byte(change.Content))
	correct.SHA256 = hex.EncodeToString(sum[:])

	cases := []struct {
		name     string
		observed ObservedFile
		want     bool
	}{
		{name: "absent", observed: ObservedFile{Source: "filesystem", Path: "main.go"}, want: false},
		{name: "different_sha", observed: ObservedFile{Source: "filesystem", Path: "main.go", Exists: true, SHA256: "different"}, want: false},
		{name: "expected_sha", observed: correct, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			judgment := EvaluateCreateCompatibility(change, tc.observed)
			if judgment.Compatible != tc.want {
				t.Fatalf("compatible=%v, want %v; judgment=%+v", judgment.Compatible, tc.want, judgment)
			}
		})
	}
}

func TestCompatibilityJudgmentIsJSONPersistible(t *testing.T) {
	original := CompatibilityJudgment{Compatible: true}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var recovered CompatibilityJudgment
	if err := json.Unmarshal(encoded, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered != original {
		t.Fatalf("recovered judgment=%+v, want %+v", recovered, original)
	}
}
