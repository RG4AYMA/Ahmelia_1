package socialinteraction

import "testing"

func TestModelsAreStable(t *testing.T) {
	if Greeting != "GREETING" || StateEngaged != "ENGAGED" || Casual != "CASUAL" {
		t.Fatal("social model constants changed")
	}
}
