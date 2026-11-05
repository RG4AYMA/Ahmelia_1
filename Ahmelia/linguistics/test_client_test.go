package linguistics

import (
	"strings"
)

// testLanguageClient keeps unit tests deterministic and independent from
// Ollama/Phi-4-mini. The real Ollama adapter is covered by an opt-in
// integration test.
type testLanguageClient struct {
	response  string
	err       error
	available bool
	prompts   []string
}

func (c *testLanguageClient) Generate(prompt string) (string, error) {
	c.prompts = append(c.prompts, prompt)
	if c.err != nil {
		return "", c.err
	}
	if c.response != "" {
		return c.response, nil
	}

	input := strings.ToLower(prompt)
	// The prompt contains many examples, so inspect only the final user
	// input section rather than matching against the whole prompt.
	if marker := strings.LastIndex(input, "entrada del usuario:"); marker >= 0 {
		input = strings.TrimSpace(input[marker+len("entrada del usuario:"):])
	}
	switch {
	case strings.Contains(input, "¿qué hora es?"):
		return `{"intent":"question","entities":[],"references":[],"constraints":[],"meaning":"Pregunta sobre la hora actual","speech_act":"question","topic":"hora","question_type":"time","directness":"question","user_state_signals":[],"correction_target":"","confidence":1}`, nil
	case strings.Contains(input, "continúa con lo que hacíamos ayer en blender"):
		return `{"intent":"command","entities":["Blender"],"references":["lo que hacíamos ayer"],"constraints":[],"meaning":"Continuar con el trabajo realizado anteriormente en Blender","speech_act":"continue","topic":"Blender","question_type":"","directness":"direct_command","user_state_signals":[],"correction_target":"","confidence":0.9}`, nil
	case strings.Contains(input, "abre el proyecto de unity pero no ejecutes el juego"):
		return `{"intent":"command","entities":["proyecto de Unity"],"references":[],"constraints":["no ejecutes el juego"],"meaning":"Instrucción para abrir el proyecto de Unity sin ejecutar el juego","speech_act":"command","topic":"Unity","question_type":"","directness":"direct_command","user_state_signals":[],"correction_target":"","confidence":0.95}`, nil
	default:
		return `{"intent":"command","entities":["Blender"],"references":[],"constraints":[],"meaning":"Instrucción para abrir Blender","speech_act":"command","topic":"Blender","question_type":"","directness":"direct_command","user_state_signals":[],"correction_target":"","confidence":0.95}`, nil
	}
}

func (c *testLanguageClient) Available() bool { return c.available }
