package naturaldialogue

import "strings"

func Continuity(prev, input string) bool {
	p := strings.TrimSpace(prev)
	if p == "" {
		return false
	}
	return !ExplicitNewTopic(input)
}
func ExplicitNewTopic(s string) bool {
	x := strings.ToLower(strings.TrimSpace(s))
	for _, k := range []string{"cambiemos de tema", "hablemos de otra cosa", "otro tema", "cambiemos a"} {
		if strings.Contains(x, k) {
			return true
		}
	}
	return false
}
func TopicFlow(prev, input string) TopicKind {
	if strings.TrimSpace(prev) == "" {
		return CurrentTopic
	}
	if ExplicitNewTopic(input) {
		return NewTopic
	}
	return CurrentTopic
}
