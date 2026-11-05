package interaction

import (
	"regexp"
	"strings"

	"ahmelia/shared"
)

// SemanticAttributeReference is the language-level contract for a person-owned
// attribute mention. It intentionally contains a free-form key: adding a new
// attribute must not require a new switch/case in User Model or SelfKnowledge.
//
// The reference is evidence only. It does not decide persistence, privacy,
// conflict resolution, or response generation.
type SemanticAttributeReference struct {
	Subject    UserObservationSubject
	Key        string
	Value      string
	HasValue   bool
	Confidence float64
	Evidence   string
}

// SemanticAttributeDomain describes the storage family selected from the
// semantic shape of an attribute key. Preference is a domain, not a list of
// preference attributes: any key can belong to it without adding code.
type SemanticAttributeDomain string

const (
	SemanticAttributeDomainAttribute  SemanticAttributeDomain = "attribute"
	SemanticAttributeDomainPreference SemanticAttributeDomain = "preference"
)

var (
	attributeDeclarationRE                    = regexp.MustCompile(`(?i)^\s*(mi|my)\s+(.+?)\s+(es|is)\s+(.+?)\s*[.!?]*\s*$`)
	attributeQuestionRE                       = regexp.MustCompile(`(?i)^\s*(?:cual\s+es|que\s+es|dime)\s+(mi|mis|my|tu|tus|your)\s+(.+?)\s*[?!.]*\s*$`)
	indirectAttributeQuestionRE               = regexp.MustCompile(`(?i)^\s*(?:nada\s+)?(?:queria|quiero)\s+saber\s+(?:si\s+)?(?:sabes|sabias|do\s+you\s+know)\s+(mi|mis|my|tu|tus|your)\s+(.+?)\s*[?!.]*\s*$`)
	knownAttributeQuestionRE                  = regexp.MustCompile(`(?i)^\s*(?:sabes|do\s+you\s+know)\s+(mi|mis|my|tu|tus|your)\s+(.+?)\s*[?!.]*\s*$`)
	knownAttributeQuestionWithInterrogativeRE = regexp.MustCompile(`(?i)^\s*(?:tu\s+)?(?:sabes|sabias|do\s+you\s+know)\s+(?:cual\s+es|que\s+es|what\s+is|which\s+is)\s+(mi|mis|my|tu|tus|your)\s+(.+?)\s*[?!.]*\s*$`)
	selfQuestionRE                            = regexp.MustCompile(`(?i)^\s*(?:tienes|tenes|have\s+you|do\s+you\s+have)\s+(.+?)\s*[?!.]*\s*$`)
	userQuestionRE                            = regexp.MustCompile(`(?i)^\s*(?:tengo|do\s+i\s+have)\s+(.+?)\s*[?!.]*\s*$`)
	nameDeclarationRE                         = regexp.MustCompile(`(?i)^\s*(me\s+llamo|my\s+name\s+is)\s+(.+?)\s*[.!?]*\s*$`)
	nameQuestionRE                            = regexp.MustCompile(`(?i)^\s*(?:como\s+me\s+llamo|what(?:'s|\s+is)\s+my\s+name)\s*[?!.]*\s*$`)
)

// ResolveSemanticAttribute consumes structured semantic relations first. If
// the linguistic adapter did not provide one, it applies only generic
// grammatical constructions. It never contains a list of attribute names.
func ResolveSemanticAttribute(req SemanticRequest) (SemanticAttributeReference, bool) {
	for _, relation := range req.Relations {
		predicate := normalizeSemanticKey(relation.Predicate)
		subject := subjectFromRelationSource(relation.Source)
		if subject == UserObservationSubjectUser || subject == UserObservationSubjectSelf {
			switch predicate {
			case "declares_attribute", "declares_preference":
				key := normalizeAttributeKey(relation.Target)
				if key == "" {
					continue
				}
				value := strings.TrimSpace(relation.Value)
				return SemanticAttributeReference{
					Subject: subject, Key: key, Value: value, HasValue: value != "", Confidence: clampConfidence(relation.Confidence), Evidence: relation.Evidence,
				}, true
			case "asks_about_attribute", "asks_about_preference":
				key := normalizeAttributeKey(relation.Target)
				if key == "" {
					continue
				}
				return SemanticAttributeReference{
					Subject: subject, Key: key, Confidence: clampConfidence(relation.Confidence), Evidence: relation.Evidence,
				}, true
			}
		}
	}

	raw := strings.TrimSpace(req.Raw)
	if raw == "" {
		raw = strings.TrimSpace(req.Normalized)
	}
	if raw == "" {
		return SemanticAttributeReference{}, false
	}
	surfaceRaw := strings.TrimSpace(strings.TrimLeft(raw, "¿¡ "))
	surface := strings.TrimSpace(strings.TrimLeft(shared.Normalize(raw), "¿¡ "))

	if m := attributeDeclarationRE.FindStringSubmatch(surfaceRaw); len(m) == 5 {
		subject := UserObservationSubjectUser
		if strings.EqualFold(m[1], "my") {
			subject = UserObservationSubjectUser
		}
		key := normalizeAttributeKey(m[2])
		value := cleanSemanticAttributeValue(m[4])
		if key != "" && value != "" {
			return SemanticAttributeReference{Subject: subject, Key: key, Value: value, HasValue: true, Confidence: .99, Evidence: raw}, true
		}
	}

	if m := attributeQuestionRE.FindStringSubmatch(surface); len(m) == 3 {
		subject := subjectFromPossessive(m[1])
		key := normalizeAttributeKey(m[2])
		key = stripAttributeOwnerPrefix(key)
		if key != "" && (subject == UserObservationSubjectUser || subject == UserObservationSubjectSelf) {
			return SemanticAttributeReference{Subject: subject, Key: key, Confidence: .99, Evidence: raw}, true
		}
	}
	if m := indirectAttributeQuestionRE.FindStringSubmatch(surface); len(m) == 3 {
		subject := subjectFromPossessive(m[1])
		key := stripAttributeOwnerPrefix(m[2])
		if key != "" && (subject == UserObservationSubjectUser || subject == UserObservationSubjectSelf) {
			return SemanticAttributeReference{Subject: subject, Key: key, Confidence: .96, Evidence: raw}, true
		}
	}
	if m := knownAttributeQuestionWithInterrogativeRE.FindStringSubmatch(surface); len(m) == 3 {
		subject := subjectFromPossessive(m[1])
		key := stripAttributeOwnerPrefix(m[2])
		if key != "" && (subject == UserObservationSubjectUser || subject == UserObservationSubjectSelf) {
			return SemanticAttributeReference{Subject: subject, Key: key, Confidence: .96, Evidence: raw}, true
		}
	}
	if m := knownAttributeQuestionRE.FindStringSubmatch(surface); len(m) == 3 {
		subject := subjectFromPossessive(m[1])
		key := stripAttributeOwnerPrefix(m[2])
		if key != "" && (subject == UserObservationSubjectUser || subject == UserObservationSubjectSelf) {
			return SemanticAttributeReference{Subject: subject, Key: key, Confidence: .96, Evidence: raw}, true
		}
	}
	normalizedQuestion := strings.TrimSpace(shared.Normalize(strings.TrimLeft(raw, "¿¡ ")))
	for _, ownerPrefix := range []string{"tu "} {
		if strings.HasPrefix(normalizedQuestion, ownerPrefix) {
			normalizedQuestion = strings.TrimSpace(strings.TrimPrefix(normalizedQuestion, ownerPrefix))
			break
		}
	}
	if m := selfQuestionRE.FindStringSubmatch(normalizedQuestion); len(m) == 2 && req.Subject == UserObservationSubjectSelf {
		key := normalizeAttributeKey(m[1])
		key = stripAttributeOwnerPrefix(key)
		if key != "" {
			return SemanticAttributeReference{Subject: UserObservationSubjectSelf, Key: key, Confidence: .95, Evidence: raw}, true
		}
	}
	if m := userQuestionRE.FindStringSubmatch(shared.Normalize(raw)); len(m) == 2 {
		key := normalizeAttributeKey(m[1])
		key = stripAttributeOwnerPrefix(key)
		if key != "" {
			return SemanticAttributeReference{Subject: UserObservationSubjectUser, Key: key, Confidence: .95, Evidence: raw}, true
		}
	}

	if m := nameDeclarationRE.FindStringSubmatch(surfaceRaw); len(m) == 3 {
		return SemanticAttributeReference{Subject: UserObservationSubjectUser, Key: "nombre", Value: cleanSemanticAttributeValue(m[2]), HasValue: true, Confidence: .99, Evidence: raw}, true
	}
	if nameQuestionRE.MatchString(surface) {
		return SemanticAttributeReference{Subject: UserObservationSubjectUser, Key: "nombre", Confidence: .99, Evidence: raw}, true
	}

	// When the surface grammar does not carry its own owner marker, a prior
	// semantic ownership result may still be reused. This is safe only for a
	// free-form key extracted by a generic question form; no attribute list is
	// consulted here.
	if req.Subject == UserObservationSubjectUser || req.Subject == UserObservationSubjectSelf {
		if key, ok := extractOwnerlessAttributeQuestion(req.Normalized); ok {
			return SemanticAttributeReference{Subject: req.Subject, Key: key, Confidence: req.SubjectConfidence, Evidence: req.Raw}, true
		}
	}

	return SemanticAttributeReference{}, false
}

func extractOwnerlessAttributeQuestion(normalized string) (string, bool) {
	n := strings.TrimSpace(shared.Normalize(normalized))
	for _, prefix := range []string{"cual es ", "que es ", "dime "} {
		if strings.HasPrefix(n, prefix) {
			key := normalizeAttributeKey(strings.TrimSpace(strings.TrimSuffix(n[len(prefix):], "?")))
			key = stripAttributeOwnerPrefix(key)
			if key != "" {
				return key, true
			}
		}
	}
	return "", false
}

func subjectFromPossessive(value string) UserObservationSubject {
	switch strings.ToLower(strings.TrimSpace(shared.Normalize(value))) {
	case "mi", "mis", "my":
		return UserObservationSubjectUser
	case "tu", "tus", "your":
		return UserObservationSubjectSelf
	default:
		return UserObservationSubject("")
	}
}

func subjectFromRelationSource(source string) UserObservationSubject {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "user", "usuario", "me", "yo":
		return UserObservationSubjectUser
	case "self", "ahmelia", "assistant":
		return UserObservationSubjectSelf
	default:
		return UserObservationSubject("")
	}
}

func normalizeAttributeKey(value string) string {
	key := strings.TrimSpace(strings.Trim(value, "?.!,:;\"'"))
	if key == "" {
		return ""
	}
	key = strings.TrimSpace(shared.Normalize(key))
	key = strings.Trim(key, "?.!,:;\"'")
	key = strings.Join(strings.Fields(key), " ")
	return key
}

func stripAttributeOwnerPrefix(key string) string {
	key = normalizeAttributeKey(key)
	for _, prefix := range []string{"mi ", "mis ", "my ", "tu ", "tus ", "your "} {
		if strings.HasPrefix(key, prefix) {
			return normalizeAttributeKey(strings.TrimSpace(strings.TrimPrefix(key, prefix)))
		}
	}
	return key
}

func normalizeSemanticKey(value string) string {
	return strings.TrimSpace(shared.Normalize(value))
}

func cleanSemanticAttributeValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " \t\r\n.,;:!?¿¡\"")
	return value
}

func clampConfidence(value float64) float64 {
	if value <= 0 {
		return .5
	}
	if value > 1 {
		return 1
	}
	return value
}

// SemanticAttributeDomainForKey is intentionally generic. Preference is
// inferred from the linguistic qualifier itself, never from specific
// attributes such as color, drink, or dessert.
func SemanticAttributeDomainForKey(key string) SemanticAttributeDomain {
	n := " " + strings.TrimSpace(shared.Normalize(key)) + " "
	for _, marker := range []string{" favorito ", " favorita ", " favoritos ", " favoritas ", " preferido ", " preferida ", " preferred ", " favorite ", " favorites ", " preference ", " preferencias "} {
		if strings.Contains(n, marker) {
			return SemanticAttributeDomainPreference
		}
	}
	return SemanticAttributeDomainAttribute
}
