package interaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// extractPersonalObservationValue keeps the semantic value of a personal
// declaration while discarding common discourse tails. A declaration such as
// "mi color favorito es negro, ya te lo había dicho" must store "negro", not
// the whole conversational sentence. This is deliberately conservative: it
// cuts only at punctuation or explicit discourse-boundary phrases, so values
// such as "azul y verde" remain intact.
func extractPersonalObservationValue(raw string) string {
	value := strings.TrimSpace(strings.TrimRight(raw, ".!?¿!"))
	lower := strings.ToLower(value)
	boundaries := []string{
		",", ";", ":", " porque ", " aunque ", " pero ",
		" ya te lo había dicho", " ya te lo habia dicho",
		" como te dije", " como siempre", " de nuevo",
		" otra vez",
	}
	cut := len(value)
	for _, marker := range boundaries {
		idx := strings.Index(lower, marker)
		if idx >= 0 && idx < cut {
			cut = idx
		}
	}
	value = strings.TrimSpace(value[:cut])
	value = strings.TrimRight(value, ".!?¿! ")
	return value
}

// UserProfile is Ahmelia's evolving model of a person. It stores observations
// as evidence rather than pretending every statement is an immutable fact.
type UserProfile struct {
	mu sync.RWMutex

	PrimaryAuthority AuthorityIdentity      `json:"primary_authority"`
	Preferences      map[string]ProfileFact `json:"preferences"`
	Habits           map[string]ProfileFact `json:"habits"`
	Interests        map[string]ProfileFact `json:"interests"`
	Routines         map[string]ProfileFact `json:"routines"`
	Notes            map[string]ProfileFact `json:"notes"`
	Communication    map[string]ProfileFact `json:"communication"`
	Goals            map[string]ProfileFact `json:"goals"`
}

type ProfileFact struct {
	Value      string    `json:"value"`
	Confidence float64   `json:"confidence"`
	Evidence   int       `json:"evidence"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Source     string    `json:"source"`
	Status     string    `json:"status"`
}

type AuthorityIdentity struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Aliases     []string `json:"aliases"`
	Role        string   `json:"role"`
}

func NewUserProfile() *UserProfile {
	return &UserProfile{
		PrimaryAuthority: AuthorityIdentity{
			ID: "ayma240", DisplayName: "Adrián Marín",
			Aliases: []string{"ayma240", "adrian marin", "adrían marín"},
			Role:    "primary_authority",
		},
		Preferences: map[string]ProfileFact{},
		Habits:      map[string]ProfileFact{},
		Interests:   map[string]ProfileFact{},
		Routines:    map[string]ProfileFact{},
		Notes:       map[string]ProfileFact{},
	}
}

func LoadUserProfile(path string) *UserProfile {
	data, err := os.ReadFile(path)
	if err != nil {
		return NewUserProfile()
	}
	var p UserProfile
	if json.Unmarshal(data, &p) != nil {
		return NewUserProfile()
	}
	p.ensureMaps()
	if p.PrimaryAuthority.ID == "" {
		p.PrimaryAuthority = NewUserProfile().PrimaryAuthority
	}
	return &p
}

func (p *UserProfile) ensureMaps() {
	if p.Preferences == nil {
		p.Preferences = map[string]ProfileFact{}
	}
	if p.Habits == nil {
		p.Habits = map[string]ProfileFact{}
	}
	if p.Interests == nil {
		p.Interests = map[string]ProfileFact{}
	}
	if p.Routines == nil {
		p.Routines = map[string]ProfileFact{}
	}
	if p.Notes == nil {
		p.Notes = map[string]ProfileFact{}
	}
	if p.Communication == nil {
		p.Communication = map[string]ProfileFact{}
	}
	if p.Goals == nil {
		p.Goals = map[string]ProfileFact{}
	}
}

func (p *UserProfile) Save(path string) error {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (p *UserProfile) Remember(category, key, value, source string, confidence float64) {
	if p == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureMaps()
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	now := time.Now()
	var dst map[string]ProfileFact
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "preference", "preferences", "gusto", "gustos":
		dst = p.Preferences
	case "habit", "habits", "habito", "habitos":
		dst = p.Habits
	case "interest", "interests", "interes", "intereses":
		dst = p.Interests
	case "routine", "routines", "rutina", "rutinas":
		dst = p.Routines
	case "communication", "comunicacion", "comunicación":
		dst = p.Communication
	case "goal", "goals", "objetivo", "objetivos":
		dst = p.Goals
	default:
		dst = p.Notes
	}
	k := strings.ToLower(strings.TrimSpace(key))
	old, ok := dst[k]
	if !ok {
		dst[k] = ProfileFact{Value: strings.TrimSpace(value), Confidence: confidence, Evidence: 1,
			FirstSeen: now, LastSeen: now, Source: source, Status: "active"}
		return
	}
	if strings.EqualFold(old.Value, value) {
		old.Evidence++
		old.Confidence = combineConfidence(old.Confidence, confidence, old.Evidence)
		old.LastSeen = now
		dst[k] = old
		return
	}
	// A contradictory observation is retained by replacing only when the new
	// evidence is materially stronger; this prevents one casual sentence from
	// rewriting a stable preference.
	if confidence >= old.Confidence+0.20 {
		old.Value = strings.TrimSpace(value)
		old.Confidence = confidence
		old.Evidence++
		old.LastSeen = now
		old.Source = source
		dst[k] = old
	}
}

func combineConfidence(old, incoming float64, evidence int) float64 {
	if evidence <= 1 {
		return incoming
	}
	v := old + (incoming-old)*0.25
	if v > 0.99 {
		v = 0.99
	}
	return v
}

func (p *UserProfile) Recall(category, key string) (ProfileFact, bool) {
	if p == nil {
		return ProfileFact{}, false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	target := strings.ToLower(strings.TrimSpace(key))
	var dst map[string]ProfileFact
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "preference", "preferences":
		dst = p.Preferences
	case "habit", "habits":
		dst = p.Habits
	case "interest", "interests":
		dst = p.Interests
	case "routine", "routines":
		dst = p.Routines
	case "communication", "comunicacion", "comunicación":
		dst = p.Communication
	case "goal", "goals", "objetivo", "objetivos":
		dst = p.Goals
	default:
		dst = p.Notes
	}
	f, ok := dst[target]
	return f, ok
}

// UserModelSnapshot is an immutable view used by reasoning/interaction layers.
// It deliberately contains only copies, so reading the model can never mutate
// the live profile.
type UserModelSnapshot struct {
	PrimaryAuthority AuthorityIdentity
	Preferences      map[string]ProfileFact
	Habits           map[string]ProfileFact
	Interests        map[string]ProfileFact
	Routines         map[string]ProfileFact
	Communication    map[string]ProfileFact
	Goals            map[string]ProfileFact
	Notes            map[string]ProfileFact
}

func (p *UserProfile) Snapshot() UserModelSnapshot {
	out := UserModelSnapshot{
		Preferences: map[string]ProfileFact{}, Habits: map[string]ProfileFact{},
		Interests: map[string]ProfileFact{}, Routines: map[string]ProfileFact{},
		Communication: map[string]ProfileFact{}, Goals: map[string]ProfileFact{},
		Notes: map[string]ProfileFact{},
	}
	if p == nil {
		return out
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	out.PrimaryAuthority = p.PrimaryAuthority
	copyFacts := func(src map[string]ProfileFact, dst map[string]ProfileFact) {
		for k, v := range src {
			dst[k] = v
		}
	}
	copyFacts(p.Preferences, out.Preferences)
	copyFacts(p.Habits, out.Habits)
	copyFacts(p.Interests, out.Interests)
	copyFacts(p.Routines, out.Routines)
	copyFacts(p.Communication, out.Communication)
	copyFacts(p.Goals, out.Goals)
	copyFacts(p.Notes, out.Notes)
	return out
}

// HasReliable returns true only when a profile fact has enough evidence to be
// treated as a useful working belief. This prevents a single weak observation
// from silently becoming a permanent "truth".
func (p *UserProfile) HasReliable(category, key string) bool {
	f, ok := p.Recall(category, key)
	return ok && f.Status != "superseded" && f.Confidence >= 0.70
}

// Learn records explicit user-provided information using a named source.
// It is the domain-neutral learning boundary for the User Model.
func (p *UserProfile) Learn(obs PersonalObservation, source string) {
	if p == nil || !obs.OK {
		return
	}
	if strings.TrimSpace(source) == "" {
		source = "conversation"
	}
	p.Remember(obs.Category, obs.Key, obs.Value, source, obs.Confidence)
}

const UserProfilePath = "memory/user_profile.json"

// PersonalObservation is a candidate extracted from an explicit self-report.
type PersonalObservation struct {
	OK         bool
	Category   string
	Key        string
	Value      string
	Confidence float64
}

func DetectPersonalObservation(raw string) PersonalObservation {
	ref, ok := ResolveSemanticAttribute(SemanticRequest{Raw: raw, Normalized: raw, Subject: UserObservationSubjectUser, SubjectConfidence: .99})
	if ok && ref.HasValue && ref.Subject == UserObservationSubjectUser {
		category := string(SemanticAttributeDomainAttribute)
		if SemanticAttributeDomainForKey(ref.Key) == SemanticAttributeDomainPreference {
			category = string(SemanticAttributeDomainPreference)
		}
		value := extractPersonalObservationValue(ref.Value)
		return PersonalObservation{Category: category, Key: ref.Key, Value: value, Confidence: ref.Confidence, OK: strings.TrimSpace(ref.Key) != "" && strings.TrimSpace(value) != ""}
	}

	lower := strings.ToLower(strings.TrimSpace(raw))
	for _, prefix := range []string{"prefiero que me hables ", "me gusta que me hables ", "quiero que me hables "} {
		if strings.HasPrefix(lower, prefix) {
			value := extractPersonalObservationValue(strings.TrimSpace(raw[len(prefix):]))
			if value != "" {
				return PersonalObservation{Category: "communication", Key: "preferred_response_style", Value: value, Confidence: .99, OK: true}
			}
		}
	}
	return PersonalObservation{}
}
