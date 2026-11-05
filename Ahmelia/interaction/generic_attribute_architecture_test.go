package interaction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenericAttributeArchitectureHasNoPerAttributeRuntimeCases(t *testing.T) {
	files := []string{
		"user_model_query.go",
		"user_observation.go",
		"semantic_attributes.go",
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ToLower(string(data))
		for _, forbidden := range []string{"favorite_color", "favorite_drink", "favorite_sweet"} {
			if strings.Contains(s, forbidden) {
				t.Fatalf("%s still contains legacy per-attribute runtime identifier %q", name, forbidden)
			}
		}
	}
}
