package naturaldialogue

import "strings"

func NeedsClarification(input string) bool {
	x := strings.ToLower(input)
	return strings.Contains(x, "no sé") || strings.Contains(x, "no entiendo") || strings.Contains(x, "eso") && len(strings.Fields(x)) <= 2
}
func IsCorrection(input string) bool {
	x := strings.ToLower(input)
	for _, k := range []string{"no, ", "me equivoqué", "quise decir", "en realidad", "corrección"} {
		if strings.Contains(x, k) {
			return true
		}
	}
	return false
}
func Transition(prev, input string) bool { return prev != "" && ExplicitNewTopic(input) }
func DetectClosing(input string) (bool, ClosingReason) {
	x := strings.ToLower(input)
	for _, k := range []string{"adiós", "adios", "chao", "nos vemos", "me voy", "hasta luego"} {
		if strings.Contains(x, k) {
			return true, Farewell
		}
	}
	return false, ""
}
