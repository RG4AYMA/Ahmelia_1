package naturaldialogue

import "strings"

func Adapt(input string) AdaptationKind {
	x := strings.ToLower(input)
	if strings.Contains(x, "código") || strings.Contains(x, "programar") || strings.Contains(x, "blender") {
		return AdaptTechnical
	}
	if strings.Contains(x, "señor") || strings.Contains(x, "usted") {
		return AdaptFormal
	}
	if len(strings.Fields(x)) <= 5 {
		return AdaptShort
	}
	return AdaptNormal
}
func RhythmFor(input string) Rhythm {
	x := strings.ToLower(input)
	if strings.Contains(x, "paso a paso") || strings.Contains(x, "explica") {
		return Explanatory
	}
	if strings.Contains(x, "espera") || strings.Contains(x, "un momento") {
		return Paused
	}
	return ResponseTiming(input)
}
