package ownership

import (
	"strings"

	"ahmelia/shared"
)

// Subject identifies the semantic owner/referent of a person-related turn.
// It is deliberately independent of any domain such as user-model,
// conversation, drones, 3D, or language generation.
type Subject string

const (
	SubjectUnknown Subject = ""
	SubjectUser    Subject = "user"
	SubjectSelf    Subject = "self"
)

// Evidence is the smallest explainable unit produced by the Ownership
// Resolver. It is descriptive evidence only; it never authorizes behavior.
type Evidence struct {
	Signal     string
	Token      string
	Confidence float64
	Provenance string
}

// Resolution is a graded ownership result. Unknown is a valid outcome and is
// preferred over silently assigning ownership when evidence conflicts or is
// insufficient.
type Resolution struct {
	Subject    Subject
	Confidence float64
	Evidence   []Evidence
	Ambiguous  bool
}

var userPronouns = map[string]bool{
	"yo":      true,
	"mi":      true,
	"mis":     true,
	"me":      true,
	"conmigo": true,
}

var selfPronouns = map[string]bool{
	"tu":      true,
	"tus":     true,
	"te":      true,
	"ti":      true,
	"contigo": true,
}

// Person-marked Spanish verb forms used only when explicit pronouns are absent.
// These are grammatical signals, not domain vocabulary: adding "animal", "color",
// "nombre", or any other noun must never require changing this resolver.
var firstPersonVerbFallback = map[string]bool{
	"tengo":    true,
	"quiero":   true,
	"puedo":    true,
	"debo":     true,
	"soy":      true,
	"estoy":    true,
	"hice":     true,
	"he":       true,
	"conozco":  true,
	"prefiero": true,
	"necesito": true,
	"recuerdo": true,
	"entiendo": true,
	"busco":    true,
	"pienso":   true,
}

var secondPersonVerbFallback = map[string]bool{
	"estas":   true,
	"eres":    true,
	"haces":   true,
	"tienes":  true,
	"tenes":   true,
	"puedes":  true,
	"sabes":   true,
	"quieres": true,
	"piensas": true,
	"conoces": true,
	"vas":     true,
}

func isInterrogativeWord(token string) bool {
	switch token {
	case "que", "como", "cual", "cuales", "cuanto", "cuantos", "cuanta", "cuantas", "donde", "cuando", "por":
		return true
	default:
		return false
	}
}

// resolveQuestionPossessiveOwner handles a grammatical distinction that a
// flat pronoun-counting rule cannot represent: in "tu sabes cual es mi color",
// "tu" addresses Ahmelia, while "mi" owns the attribute being asked about.
// The possessive inside the interrogative complement is therefore the semantic
// referent of the query. This is generic grammar; it never names an attribute.
func resolveQuestionPossessiveOwner(tokens []string) (Resolution, bool) {
	for i, token := range tokens {
		var subject Subject
		switch token {
		case "mi", "mis", "my":
			subject = SubjectUser
		case "tu", "tus", "your":
			subject = SubjectSelf
		default:
			continue
		}
		// The possessive is a query referent when it follows the copular
		// interrogative complement "cual es X" / "que es X". This prevents a
		// leading address marker such as "tu" in "tu sabes..." from creating
		// a false ownership conflict.
		if i >= 2 && (tokens[i-1] == "es" || tokens[i-1] == "is") {
			return Resolution{
				Subject: subject, Confidence: 0.98,
				Evidence: []Evidence{{
					Signal: "interrogative_possessive", Token: token,
					Confidence: 0.98, Provenance: "ownership.grammar",
				}},
			}, true
		}
	}
	return Resolution{}, false
}

// Affection constructions ("me gusta X" / "te gustaría X") place the
// experiencer in the dative clitic. When a question is addressed to Ahmelia
// ("¿te gustaría ver mi proyecto?"), the clitic owns the turn even though the
// complement carries user possessions. The roots form a closed grammatical
// family (dative-experiencer verbs), never domain vocabulary.
var affectionVerbRoots = []string{
	"gust", "encant", "interes", "import", "molest", "apetec", "agrad", "disgust", "parec", "falt",
}

func isAffectionVerbToken(token string) bool {
	for _, root := range affectionVerbRoots {
		if len(token) > len(root) && strings.HasPrefix(token, root) {
			return true
		}
	}
	return false
}

// resolveAffectionAddressing only resolves the 2nd-person clitic ("te"): the
// question is addressed to Ahmelia. The symmetric first-person statement
// ("me gusta tu color") keeps its abstention by design (negative control of
// the resolver): a mixed statement does not assign ownership.
func resolveAffectionAddressing(tokens []string) (Resolution, bool) {
	for i := 1; i < len(tokens); i++ {
		if tokens[i-1] != "te" || !isAffectionVerbToken(tokens[i]) {
			continue
		}
		return Resolution{
			Subject:    SubjectSelf,
			Confidence: 0.85,
			Evidence: []Evidence{{
				Signal: "affection_verb_dative", Token: tokens[i],
				Confidence: 0.85, Provenance: "ownership.grammar",
			}},
		}, true
	}
	return Resolution{}, false
}

// rootPersonVerb maps conjugated forms by ROOT + person ending so the
// conditional/future forms the full-word lists do not enumerate ("podrías",
// "harías", "tendrías") still resolve. 2nd person → self; 1st person future
// ("podré", "haré") → user. First/third-person homographs ("podría") abstain.
func rootPersonVerb(token string) (Subject, bool) {
	roots := []string{"podr", "har", "tendr", "querr", "vendr", "sabr", "dir"}
	for _, root := range roots {
		if !strings.HasPrefix(token, root) {
			continue
		}
		switch strings.TrimPrefix(token, root) {
		case "as", "ias":
			return SubjectSelf, true
		case "e":
			return SubjectUser, true
		}
	}
	return SubjectUnknown, false
}

// Resolve returns the current turn's semantic ownership evidence. It is
// intentionally independent of punctuation so declarations, proposals,
// feedback, and commands receive the same treatment as questions.
func Resolve(text string) Resolution {
	normalized := shared.Normalize(text)
	if normalized == "" {
		return Resolution{}
	}

	tokens := strings.Fields(normalized)
	if resolution, ok := resolveQuestionPossessiveOwner(tokens); ok {
		return resolution
	}
	userHits := make([]Evidence, 0, 2)
	selfHits := make([]Evidence, 0, 2)

	for _, token := range tokens {
		switch {
		case userPronouns[token]:
			userHits = append(userHits, Evidence{
				Signal: "first_person_pronoun", Token: token,
				Confidence: 0.99, Provenance: "ownership.pronoun",
			})
		case selfPronouns[token]:
			selfHits = append(selfHits, Evidence{
				Signal: "second_person_pronoun", Token: token,
				Confidence: 0.99, Provenance: "ownership.pronoun",
			})
		}
	}

	if len(userHits) > 0 && len(selfHits) > 0 {
		// Un complemento con posesiones del usuario no contradice una pregunta
		// dirigida a Ahmelia: el clítico de dativo resuelve el titular del
		// turno ("te gustaría ver mi proyecto" → self). La abstención se
		// conserva para el caso simétrico en primera persona.
		if resolution, ok := resolveAffectionAddressing(tokens); ok {
			return resolution
		}
		return Resolution{
			Subject:    SubjectUnknown,
			Confidence: 0.20,
			Evidence:   append(userHits, selfHits...),
			Ambiguous:  true,
		}
	}
	if len(userHits) > 0 {
		return Resolution{Subject: SubjectUser, Confidence: 0.99, Evidence: userHits}
	}
	if len(selfHits) > 0 {
		return Resolution{Subject: SubjectSelf, Confidence: 0.99, Evidence: selfHits}
	}

	// First-person verb morphology is a grammatical signal, not domain
	// vocabulary. "tengo" keeps user-owned queries such as "que nivel tengo"
	// working without adding a list entry for Blender, color, or any other noun.
	for i, token := range tokens {
		if firstPersonVerbFallback[token] {
			return Resolution{
				Subject: SubjectUser, Confidence: 0.80, Evidence: []Evidence{{
					Signal: "first_person_verb", Token: token,
					Confidence: 0.80, Provenance: "ownership.grammar",
				}},
			}
		}
		if subject, ok := rootPersonVerb(token); ok && subject == SubjectUser {
			return Resolution{
				Subject: SubjectUser, Confidence: 0.78, Evidence: []Evidence{{
					Signal: "root_person_verb", Token: token,
					Confidence: 0.78, Provenance: "ownership.grammar",
				}},
			}
		}
		// "se" is ambiguous by itself after accent normalization (it can be
		// reflexive/impersonal), so only treat it as first-person in a
		// question-shaped grammatical construction such as "que se sobre..."
		// produced from "que se sobre..."/"que sé sobre...".
		if token == "se" && i > 0 && isInterrogativeWord(tokens[i-1]) && i+1 < len(tokens) {
			if tokens[i+1] == "sobre" || tokens[i+1] == "de" || tokens[i+1] == "en" || tokens[i+1] == "acerca" {
				return Resolution{
					Subject: SubjectUser, Confidence: 0.80, Evidence: []Evidence{{
						Signal: "first_person_verb", Token: token,
						Confidence: 0.80, Provenance: "ownership.grammar",
					}},
				}
			}
		}
	}

	// Compatibility for ordinary second-person questions/utterances that omit
	// the pronoun. This is still grammatical evidence and remains independent
	// of domain vocabulary. It is weaker than explicit pronouns by design.
	for _, token := range tokens {
		if secondPersonVerbFallback[token] {
			return Resolution{
				Subject:    SubjectSelf,
				Confidence: 0.80,
				Evidence: []Evidence{{
					Signal: "second_person_verb", Token: token,
					Confidence: 0.80, Provenance: "ownership.grammar",
				}},
			}
		}
		if subject, ok := rootPersonVerb(token); ok && subject == SubjectSelf {
			return Resolution{
				Subject:    SubjectSelf,
				Confidence: 0.78,
				Evidence: []Evidence{{
					Signal: "root_person_verb", Token: token,
					Confidence: 0.78, Provenance: "ownership.grammar",
				}},
			}
		}
	}

	return Resolution{}
}
