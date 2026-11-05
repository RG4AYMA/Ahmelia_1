package socialinteraction

import "time"

type InteractionKind string

const (
	Task          InteractionKind = "TASK"
	Informational InteractionKind = "INFORMATIONAL"
	Social        InteractionKind = "SOCIAL"
	Mixed         InteractionKind = "MIXED"
)

type Formality string

const (
	VeryFormal Formality = "VERY_FORMAL"
	Formal     Formality = "FORMAL"
	Neutral    Formality = "NEUTRAL"
	Casual     Formality = "CASUAL"
	VeryCasual Formality = "VERY_CASUAL"
)

type SocialIntent string

const (
	Greeting                SocialIntent = "GREETING"
	Farewell                SocialIntent = "FAREWELL"
	Thanking                SocialIntent = "THANKING"
	Apology                 SocialIntent = "APOLOGY"
	Congratulation          SocialIntent = "CONGRATULATION"
	Acknowledgment          SocialIntent = "ACKNOWLEDGMENT"
	CheckIn                 SocialIntent = "CHECK_IN"
	SocialComment           SocialIntent = "SOCIAL_COMMENT"
	Humor                   SocialIntent = "HUMOR"
	PlayfulInteraction      SocialIntent = "PLAYFUL_INTERACTION"
	EmotionalSupportRequest SocialIntent = "EMOTIONAL_SUPPORT_REQUEST"
	SocialInvitation        SocialIntent = "SOCIAL_INVITATION"
	UnknownIntent           SocialIntent = "UNKNOWN"
)

type SocialState string

const (
	StateNeutral     SocialState = "NEUTRAL"
	StateOpening     SocialState = "OPENING"
	StateEngaged     SocialState = "ENGAGED"
	StateWindingDown SocialState = "WINDING_DOWN"
	StateClosing     SocialState = "CLOSING"
	StateRepairing   SocialState = "REPAIRING"
	StateFormal      SocialState = "FORMAL"
	StateInformal    SocialState = "INFORMAL"
	StatePlayful     SocialState = "PLAYFUL"
	StateSerious     SocialState = "SERIOUS"
)

type Participation string

const (
	Acknowledge Participation = "ACKNOWLEDGE"
	Respond     Participation = "RESPOND"
	Support     Participation = "SUPPORT"
	React       Participation = "REACT"
	Participate Participation = "PARTICIPATE"
	Close       Participation = "CLOSE"
	Repair      Participation = "REPAIR"
)

type Context struct {
	ParticipantCount int
	Channel          string
	Activity         string
	Formality        Formality
	Public           bool
	Group            bool
	Purpose          InteractionKind
	Topic            string
}
type Signal struct {
	Name       string
	Value      string
	Confidence float64
	ObservedAt time.Time
}
type Cue struct {
	Name       string
	Confidence float64
	Evidence   []string
}
type Boundary struct {
	Allowed bool
	Reason  string
}
type Assessment struct {
	Kind          InteractionKind
	Context       Context
	Signals       []Signal
	Cues          []Cue
	Intent        SocialIntent
	State         SocialState
	Formality     Formality
	Participation Participation
	Boundaries    []Boundary
	Confidence    float64
}
type ResponsePlan struct {
	Participation  Participation
	Intent         SocialIntent
	State          SocialState
	Formality      Formality
	Brief          bool
	RepairRequired bool
	Constraints    []string
}
