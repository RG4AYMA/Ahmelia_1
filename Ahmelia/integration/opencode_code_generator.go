package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	openCodeAgentName = "ahmelia-code-synthesis"
	openCodeMaxOutput = 16 << 20
)

var (
	openCodeANSISequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	openCodeBearerSecret = regexp.MustCompile(`(?i)bearer\s+\S+`)
	openCodeNamedSecret  = regexp.MustCompile(`(?i)(api[_ -]?key|token|password|secret)(["']?\s*[:=]\s*["']?)[^\s,"';]+`)
	openCodeProviderKey  = regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_-]{12,}|nvapi-[a-z0-9_-]{12,})\b`)
)

// OpenCodeCodeGenerator invokes the installed OpenCode CLI in non-interactive
// mode. The temporary agent denies every tool permission: OpenCode can return
// a code proposal but cannot edit files, run commands, or bypass Ahmelia's
// authorization, materialization and verification path.
type OpenCodeCodeGenerator struct {
	Executable string
	Model      string
	Timeout    time.Duration
}

func NewOpenCodeCodeGeneratorFromEnv() *OpenCodeCodeGenerator {
	executable := strings.TrimSpace(os.Getenv("AHMELIA_OPENCODE_BIN"))
	if executable == "" {
		executable = "opencode"
	}
	model := strings.TrimSpace(os.Getenv("AHMELIA_OPENCODE_MODEL"))
	return &OpenCodeCodeGenerator{Executable: executable, Model: model, Timeout: 5 * time.Minute}
}

// GenerationSource identifies the external code proposal source. It is kept
// separate from the programming provider that applies and verifies the result.
func (g *OpenCodeCodeGenerator) GenerationSource() string {
	if g == nil {
		return "opencode:unknown"
	}
	model := strings.TrimSpace(g.Model)
	if model == "" {
		return "opencode:configured-default"
	}
	return "opencode:" + model
}

func (g *OpenCodeCodeGenerator) GenerateStructuredContext(ctx context.Context, prompt string, _ json.RawMessage, _ int) (string, error) {
	if g == nil {
		return "", fmt.Errorf("OpenCode code generator is nil")
	}
	executable := strings.TrimSpace(g.Executable)
	if executable == "" {
		executable = "opencode"
	}
	executable, err := resolveOpenCodeExecutable(executable)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(g.Model)
	if model != "" && (!strings.Contains(model, "/") || strings.ContainsAny(model, "\r\n\t ")) {
		return "", fmt.Errorf("OpenCode model must use provider/model format")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("OpenCode code synthesis prompt is empty")
	}

	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workingDir, err := os.MkdirTemp("", "ahmelia-opencode-")
	if err != nil {
		return "", fmt.Errorf("OpenCode temporary workspace: %w", err)
	}
	defer os.RemoveAll(workingDir)
	promptPath := filepath.Join(workingDir, "objective.txt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0600); err != nil {
		return "", fmt.Errorf("OpenCode objective staging: %w", err)
	}

	configContent, err := buildOpenCodeAgentConfig()
	if err != nil {
		return "", fmt.Errorf("OpenCode agent configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workingDir, "opencode.json"), configContent, 0600); err != nil {
		return "", fmt.Errorf("OpenCode agent configuration staging: %w", err)
	}

	args := openCodeRunArgs(model, workingDir, promptPath)
	cmd := exec.CommandContext(callCtx, executable, args...)
	cmd.Dir = workingDir
	var stdout cappedBuffer
	stdout.limit = openCodeMaxOutput
	var stderr cappedBuffer
	stderr.limit = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if callCtx.Err() != nil {
			return "", fmt.Errorf("OpenCode code synthesis: %w", callCtx.Err())
		}
		if stdout.overflow {
			return "", fmt.Errorf("OpenCode code synthesis output exceeded %d bytes", openCodeMaxOutput)
		}
		diagnostic := summarizeOpenCodeStderr(stderr.String())
		if diagnostic == "" {
			return "", fmt.Errorf("OpenCode code synthesis command failed: %w", err)
		}
		return "", fmt.Errorf("OpenCode code synthesis command failed: %w; stderr: %s", err, diagnostic)
	}
	if stdout.overflow {
		return "", fmt.Errorf("OpenCode code synthesis output exceeded %d bytes", openCodeMaxOutput)
	}
	text, err := openCodeResponseText(stdout.Bytes())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("OpenCode returned no synthesis text")
	}
	return text, nil
}

func buildOpenCodeAgentConfig() ([]byte, error) {
	return json.Marshal(map[string]any{
		"agent": map[string]any{
			openCodeAgentName: map[string]any{
				"mode":        "primary",
				"description": "Return a structured code proposal for Ahmelia without using tools",
				"prompt":      "Return only the requested JSON object. Do not call tools or perform actions.",
				"permission":  map[string]string{"*": "deny"},
			},
		},
	})
}

func openCodeRunArgs(model, workingDir, promptPath string) []string {
	// The installed CLI does not expose --dir; the child process already runs
	// with cmd.Dir=workingDir. Passing an unsupported flag makes every real
	// synthesis fail before OpenCode can propose anything.
	_ = workingDir
	args := []string{"run", "--format", "json"}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	return append(args,
		"--agent", openCodeAgentName,
		"--file", promptPath,
		"Return the attached objective as one JSON object with a top-level files array. Do not use Markdown.",
	)
}

func summarizeOpenCodeStderr(stderr string) string {
	value := openCodeANSISequence.ReplaceAllString(strings.TrimSpace(stderr), "")
	value = openCodeBearerSecret.ReplaceAllString(value, "Bearer [redacted]")
	value = openCodeNamedSecret.ReplaceAllString(value, "$1$2[redacted]")
	value = openCodeProviderKey.ReplaceAllString(value, "[redacted-provider-key]")
	lines := strings.FieldsFunc(value, func(r rune) bool { return r == '\r' || r == '\n' })
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	value = strings.TrimSpace(strings.Join(lines, " | "))
	if len(value) > 1200 {
		value = value[len(value)-1200:]
	}
	return value
}

type openCodeEventPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openCodeEvent struct {
	Type       string            `json:"type"`
	Part       openCodeEventPart `json:"part"`
	Properties struct {
		Part openCodeEventPart `json:"part"`
	} `json:"properties"`
}

func openCodeResponseText(output []byte) (string, error) {
	var result strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64<<10), openCodeMaxOutput)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event openCodeEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Type != "text" && event.Type != "message.part.updated" {
			continue
		}
		part := event.Part
		if part.Text == "" {
			part = event.Properties.Part
		}
		if part.Type == "text" || (part.Type == "" && part.Text != "") {
			result.WriteString(part.Text)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("OpenCode response stream: %w", err)
	}
	if result.Len() == 0 {
		// Keep compatibility with CLI builds that emit a single JSON value.
		var response struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(output), &response); err == nil && response.Text != "" {
			return response.Text, nil
		}
		return "", fmt.Errorf("OpenCode response contained no text event")
	}
	return result.String(), nil
}

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func resolveOpenCodeExecutable(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("OpenCode CLI %q not found: %w", name, err)
	}
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(path), ".cmd") {
		// npm installs a PowerShell shim and a .cmd shim. Go cannot execute the
		// .cmd shim directly, so invoke the native executable it delegates to.
		native := filepath.Join(filepath.Dir(path), "node_modules", "@opencode", "cli", "bin", "opencode.exe")
		if _, err := os.Stat(native); err != nil {
			return "", fmt.Errorf("OpenCode launcher %q found but its native executable is unavailable: %w", path, err)
		}
		return native, nil
	}
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(path), ".ps1") {
		return "", fmt.Errorf("OpenCode resolved to a PowerShell script; set AHMELIA_OPENCODE_BIN to the native opencode.exe")
	}
	return path, nil
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.overflow = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.overflow = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
