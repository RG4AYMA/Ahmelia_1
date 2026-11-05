package naturaldialogue

import "ahmelia/interaction"

type TurnOwner string

const (
	UserTurn       TurnOwner = "USER_TURN"
	AhmeliaTurn    TurnOwner = "AHMELIA_TURN"
	WaitingForUser TurnOwner = "WAITING_FOR_USER"
)

type TurnAction string

const (
	Continue TurnAction = "CONTINUE"
	Yield    TurnAction = "YIELD"
	Close    TurnAction = "CLOSE"
)

type FlowState string

const (
	Open          FlowState = "OPEN"
	Engaged       FlowState = "ENGAGED"
	Continuing    FlowState = "CONTINUING"
	Transitioning FlowState = "TRANSITIONING"
	Closing       FlowState = "CLOSING"
)

type TopicKind string

const (
	CurrentTopic TopicKind = "CURRENT_TOPIC"
	RelatedTopic TopicKind = "RELATED_TOPIC"
	NewTopic     TopicKind = "NEW_TOPIC"
	ReturnTopic  TopicKind = "RETURN_TO_TOPIC"
	TopicClosure TopicKind = "TOPIC_CLOSURE"
)

type FollowUpKind string

const (
	FollowExpected FollowUpKind = "FOLLOW_UP_EXPECTED"
	FollowOptional FollowUpKind = "OPTIONAL"
	NoFollow       FollowUpKind = "NO_FOLLOW_UP"
)

type QuestionKind string

const (
	Question              QuestionKind = "QUESTION"
	MultiQuestion         QuestionKind = "MULTI_QUESTION"
	Rhetorical            QuestionKind = "RHETORICAL"
	ClarificationQuestion QuestionKind = "CLARIFICATION"
	InformationRequest    QuestionKind = "INFORMATION_REQUEST"
	SocialQuestion        QuestionKind = "SOCIAL_QUESTION"
	NotQuestion           QuestionKind = "NOT_QUESTION"
)

type LengthKind string

const (
	Short    LengthKind = "SHORT"
	Normal   LengthKind = "NORMAL"
	Detailed LengthKind = "DETAILED"
)

type AdaptationKind string

const (
	AdaptShort     AdaptationKind = "SHORT"
	AdaptNormal    AdaptationKind = "NORMAL"
	AdaptDetailed  AdaptationKind = "DETAILED"
	AdaptTechnical AdaptationKind = "TECHNICAL"
	AdaptCasual    AdaptationKind = "CASUAL"
	AdaptFormal    AdaptationKind = "FORMAL"
)

type Rhythm string

const (
	Fast         Rhythm = "FAST"
	NormalRhythm Rhythm = "NORMAL"
	Slow         Rhythm = "SLOW"
	Paused       Rhythm = "PAUSED"
	BackAndForth Rhythm = "BACK_AND_FORTH"
	Explanatory  Rhythm = "EXPLANATORY"
)

type ClosingReason string

const (
	CloseRequest   ClosingReason = "CLOSE_REQUEST"
	NaturalEnd     ClosingReason = "NATURAL_END"
	Farewell       ClosingReason = "FAREWELL"
	NoContinuation ClosingReason = "NO_CONTINUATION"
)

type Thread struct {
	ID, Topic string
	Active    bool
}
type DialogueContext struct {
	CurrentTopic, PreviousTopic string
	Threads                     []Thread
	Turn                        TurnOwner
	Flow                        FlowState
	Carryover                   []string
	Formality                   string
	Social, Task                bool
}

type Decision struct {
	// Move is the B96.39 conversational movement selected from the already
	// established semantic/response contract. It contains no wording and
	// never authorizes initiative or capability execution.
	Move                                    interaction.ResponseMove
	Turn                                    TurnOwner
	Action                                  TurnAction
	Flow                                    FlowState
	Topic                                   TopicKind
	FollowUp                                FollowUpKind
	Question                                QuestionKind
	Length                                  LengthKind
	Adaptation                              AdaptationKind
	Rhythm                                  Rhythm
	Clarify, Corrected, Closing, Completion bool
	CloseReason                             ClosingReason
	// RequiresDirectTurn is the hard hand-off from Natural Dialogue to Initiative:
	// the current user turn must be satisfied before any proactive candidate can compete.
	RequiresDirectTurn bool
	// IsAnswerToPreviousTurn records discourse structure without storing memory.
	IsAnswerToPreviousTurn bool
	// AllowProactiveFollowUp is a structural permission, never an initiative decision.
	AllowProactiveFollowUp bool
	Reason                 string
}

func RequestRequiresDirectTurn(r interaction.SemanticRequest) bool {
	acts := interaction.CanonicalActs(r)
	if acts.Has(interaction.ActQuestion) || acts.Has(interaction.ActCommand) || acts.Has(interaction.ActControl) || acts.Has(interaction.ActCorrection) || r.NeedsClarification {
		return true
	}
	return false
}
