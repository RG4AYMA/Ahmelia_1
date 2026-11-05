package integration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestOpenCodeAgentConfigUsesStructuredDenyPermissions(t *testing.T) {
	data, err := buildOpenCodeAgentConfig()
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Agent map[string]struct {
			Permission map[string]string `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	permissions := config.Agent[openCodeAgentName].Permission
	if permissions["*"] != "deny" {
		t.Fatalf("agent permissions=%v; want wildcard deny map", permissions)
	}
}

func TestOpenCodeUsesConfiguredDefaultUnlessModelWasExplicitlySet(t *testing.T) {
	defaultArgs := openCodeRunArgs("", "workspace", "objective.txt")
	if reflect.DeepEqual(defaultArgs, openCodeRunArgs("provider/model", "workspace", "objective.txt")) {
		t.Fatal("explicit model selection was not distinguished")
	}
	for _, arg := range defaultArgs {
		if arg == "--model" {
			t.Fatalf("default run overrides OpenCode's configured model: %v", defaultArgs)
		}
	}
	explicitArgs := openCodeRunArgs("provider/model", "workspace", "objective.txt")
	if !containsPair(explicitArgs, "--model", "provider/model") {
		t.Fatalf("explicit model missing from args: %v", explicitArgs)
	}
	if got := (&OpenCodeCodeGenerator{}).GenerationSource(); got != "opencode:configured-default" {
		t.Fatalf("default generation source=%q", got)
	}
}

// TestOpenCodeRunArgsMatchInstalledCLI protege el contrato real del CLI: la
// versión instalada no acepta --dir. El directorio de trabajo se pasa por
// cmd.Dir, no por argumento; un flag inventado hacía fallar toda síntesis real.
func TestOpenCodeRunArgsMatchInstalledCLI(t *testing.T) {
	args := openCodeRunArgs("provider/model", "workspace", "objective.txt")
	for _, forbidden := range []string{"--dir", "--cwd"} {
		for _, arg := range args {
			if arg == forbidden {
				t.Fatalf("unsupported %s flag in OpenCode args: %v", forbidden, args)
			}
		}
	}
	if !containsPair(args, "--file", "objective.txt") {
		t.Fatalf("objective attachment missing: %v", args)
	}
	if !containsPair(args, "--format", "json") {
		t.Fatalf("structured output missing: %v", args)
	}
}

func TestOpenCodeFailureDiagnosticRetainsCauseAndRedactsCredentials(t *testing.T) {
	got := summarizeOpenCodeStderr("\x1b[31mProvider auth failed: api_key=sk-1234567890abcdefghijkl\x1b[0m\nRun opencode auth login")
	if !strings.Contains(got, "Provider auth failed") || !strings.Contains(got, "opencode auth login") {
		t.Fatalf("diagnostic lost actionable stderr: %q", got)
	}
	if strings.Contains(got, "1234567890abcdefghijkl") || strings.Contains(got, "sk-[redacted]") {
		t.Fatalf("diagnostic leaked a credential: %q", got)
	}
}

func containsPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
