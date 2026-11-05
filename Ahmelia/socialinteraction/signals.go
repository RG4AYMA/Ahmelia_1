package socialinteraction

import (
	"strings"
	"time"
)

type SignalNormalizer struct{}

func NewSignalNormalizer() *SignalNormalizer { return &SignalNormalizer{} }
func (n *SignalNormalizer) Normalize(input string) []Signal {
	l := strings.ToLower(input)
	out := []Signal{}
	add := func(name, val string, c float64) {
		out = append(out, Signal{Name: name, Value: val, Confidence: c, ObservedAt: time.Now()})
	}
	if hasAny(l, "hola", "buenos días", "buenas") {
		add("greeting", "present", .98)
	}
	if hasAny(l, "adiós", "chao", "nos vemos", "hasta luego") {
		add("farewell", "present", .98)
	}
	if hasAny(l, "gracias", "te agradezco") {
		add("thanks", "present", .97)
	}
	if hasAny(l, "perdón", "lo siento") {
		add("apology", "present", .96)
	}
	if hasAny(l, "felicidades", "felicitaciones") {
		add("congratulation", "present", .96)
	}
	if hasAny(l, "jaja", "jajaja", "😂") {
		add("humor", "present", .90)
	}
	return out
}
