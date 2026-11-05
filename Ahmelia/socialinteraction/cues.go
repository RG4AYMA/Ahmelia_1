package socialinteraction

type CueAnalyzer struct{}

func NewCueAnalyzer() *CueAnalyzer { return &CueAnalyzer{} }
func (a *CueAnalyzer) Analyze(input string, signals []Signal) []Cue {
	cues := []Cue{}
	for _, s := range signals {
		cues = append(cues, Cue{Name: s.Name, Confidence: s.Confidence, Evidence: []string{s.Value}})
	}
	if len(input) < 12 {
		cues = append(cues, Cue{Name: "brevity", Confidence: .70, Evidence: []string{"short_input"}})
	}
	return cues
}
