package interaction

// GeneratePersonalQuestion is retained as a compatibility API for callers
// that specifically want a personal-learning question. The general
// B96.22.21 generator remains the single owner of question generation.
func GeneratePersonalQuestion(profile *UserProfile, ctx ConversationSnapshot) (string, bool) {
	if profile == nil {
		return "", false
	}
	candidates := DefaultQuestionGenerator().Generate(QuestionGenerationContext{
		Request:  SemanticRequest{Kind: KindConversation},
		Snapshot: ctx,
		Profile:  profile,
	})
	for _, candidate := range candidates {
		if candidate.Purpose == QuestionPurposeLearn {
			return candidate.Question, true
		}
	}
	return "", false
}
