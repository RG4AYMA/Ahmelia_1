package linguistics

import (
	"os"
	"testing"
)

func TestNewOllamaClientUsesConfigurableURLAndModel(t *testing.T) {
	t.Setenv("AHMELIA_OLLAMA_URL", "http://127.0.0.1:9999/")
	t.Setenv("AHMELIA_OLLAMA_MODEL", "qwen2.5:3b")
	c := NewOllamaClient()
	if c.BaseURL != "http://127.0.0.1:9999/" {
		t.Fatalf("BaseURL=%q", c.BaseURL)
	}
	if c.Model != "qwen2.5:3b" {
		t.Fatalf("Model=%q", c.Model)
	}
}

func TestNewOllamaClientDefaultsToIPv4Loopback(t *testing.T) {
	_ = os.Unsetenv("AHMELIA_OLLAMA_URL")
	_ = os.Unsetenv("AHMELIA_OLLAMA_MODEL")
	c := NewOllamaClient()
	if c.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("BaseURL=%q", c.BaseURL)
	}
	if c.Model != "qwen3:4b-instruct-2507-q4_K_M" {
		t.Fatalf("Model=%q", c.Model)
	}
}
