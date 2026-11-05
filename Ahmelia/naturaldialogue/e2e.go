package naturaldialogue

import "ahmelia/interaction"

type E2E struct {
	I     *Integration
	Topic string
	Turn  TurnOwner
}

func NewE2E() *E2E { return &E2E{I: NewIntegration(), Turn: WaitingForUser} }
func (e *E2E) Run(input string) Decision {
	r := interaction.SemanticRequest{Raw: input, Normalized: input, Kind: interaction.KindConversation}
	if QuestionType(input) == Question || QuestionType(input) == MultiQuestion || QuestionType(input) == InformationRequest {
		r.Kind = interaction.KindQuestion
		r.SpeechAct = interaction.SpeechQuestion
	}
	d := e.I.EvaluateRequest(r, interaction.ConversationSnapshot{ActiveTopic: e.Topic}, e.Turn)
	if d.Topic == NewTopic {
		e.Topic = "new"
	}
	if d.Action == Continue {
		e.Turn = AhmeliaTurn
	}
	return d
}
