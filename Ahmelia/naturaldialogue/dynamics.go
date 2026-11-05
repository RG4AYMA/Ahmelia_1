package naturaldialogue

import "strings"

func ResponseTiming(input string) Rhythm {
	n := len(strings.Fields(input))
	if n <= 3 {
		return Fast
	}
	if n >= 30 {
		return Explanatory
	}
	return NormalRhythm
}
func ResponseLength(input string) LengthKind {
	n := len(strings.Fields(input))
	if n <= 6 {
		return Short
	}
	if n >= 35 {
		return Detailed
	}
	return Normal
}
func FollowUp(input string) FollowUpKind {
	if ExplicitNewTopic(input) || strings.Contains(strings.ToLower(input), "adiós") || strings.Contains(strings.ToLower(input), "chao") {
		return NoFollow
	}
	if strings.Contains(input, "?") {
		return FollowExpected
	}
	return FollowOptional
}
func QuestionType(input string) QuestionKind {
	x := strings.ToLower(strings.TrimSpace(input))
	if strings.Count(x, "?") > 1 {
		return MultiQuestion
	}
	if strings.Contains(x, "no entiendo") || strings.HasPrefix(x, "qué quieres decir") || strings.HasPrefix(x, "que quieres decir") {
		return ClarificationQuestion
	}
	if strings.Contains(x, "?") {
		return Question
	}
	if strings.HasPrefix(x, "qué ") || strings.HasPrefix(x, "que ") || strings.HasPrefix(x, "cómo ") || strings.HasPrefix(x, "como ") {
		return InformationRequest
	}
	return NotQuestion
}
func CompleteAnswer(input string) bool { return strings.TrimSpace(input) != "" }
