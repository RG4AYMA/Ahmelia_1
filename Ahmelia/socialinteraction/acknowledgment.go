package socialinteraction

type AcknowledgmentPolicy struct{}

func NewAcknowledgmentPolicy() *AcknowledgmentPolicy { return &AcknowledgmentPolicy{} }
func (p *AcknowledgmentPolicy) Required(intent SocialIntent) bool {
	switch intent {
	case Greeting, Farewell, Thanking, Apology, Congratulation, Acknowledgment:
		return true
	}
	return false
}
