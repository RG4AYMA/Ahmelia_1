package interaction

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"ahmelia/ownership"
)

// UserObservation is the domain-neutral bridge between semantic interaction
// and B96.34. It carries ownership explicitly so "my" never has to be
// re-inferred by downstream consumers. Domain admission remains owned by the
// corresponding B96.34 domain policy; this type only describes the observed
// candidate and its provenance.
type UserObservation struct {
	Subject    UserObservationSubject
	Domain     UserModelDomain
	Key        string
	Value      string
	Confidence float64
	Provenance UserModelSourceRef
	Evidence   string
	Context    string
	ObservedAt time.Time
}

type UserObservationSubject = ownership.Subject

const (
	UserObservationSubjectUser = ownership.SubjectUser
	UserObservationSubjectSelf = ownership.SubjectSelf
)

var (
	capabilityObservationRE = regexp.MustCompile(`(?i)^mi\s+nivel\s+(?:de|en|sobre)\s+(.+?)\s+es\s+(.+)$`)
	knowledgeObservationRE  = regexp.MustCompile(`(?i)^(?:mi\s+conocimiento\s+(?:de|sobre|en)|mi\s+nivel\s+de\s+conocimiento\s+(?:de|sobre|en))\s+(.+?)\s+es\s+(.+)$`)
	goalObservationRE       = regexp.MustCompile(`(?i)^mi\s+(?:principal\s+)?objetivo\s+es\s+(.+)$`)
	attributeObservationRE  = regexp.MustCompile(`(?i)^mi\s+([a-záéíóúñ_][a-záéíóúñ0-9_ ]*)\s+es\s+(.+)$`)
)

// DetectUserObservations extracts only explicit first-person declarations.
// It is intentionally deterministic: an LLM may enrich linguistic meaning,
// but it does not get authority to invent durable user facts.
func DetectUserObservations(raw string, now time.Time, sourceID string) []UserObservation {
	text := strings.TrimSpace(raw)
	if text == "" || isLikelyQuestion(text) {
		return nil
	}
	if sourceID == "" {
		sourceID = "conversation"
	}
	source := UserModelSourceRef{
		Kind: UserSourceExplicitUser, SourceID: sourceID, ActorID: "user", ObservedAt: now,
	}

	// Domain-specific B96.34 grammars run before the generic attribute
	// fallback. They are semantic domains (goal, capability, knowledge), not
	// enumerations of individual attributes.
	if dim, value, ok := detectCommunicationObservation(text); ok {
		return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainCommunicationProfile, Key: string(dim), Value: value, Confidence: .99, Provenance: source, Evidence: text, ObservedAt: now}}
	}
	if m := knowledgeObservationRE.FindStringSubmatch(text); len(m) == 3 {
		level, ok := normalizeKnowledgeLevel(m[2])
		if ok {
			return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainKnowledge, Key: strings.TrimSpace(m[1]), Value: string(level), Confidence: 0.99, Provenance: source, Evidence: text, ObservedAt: now}}
		}
	}
	if m := capabilityObservationRE.FindStringSubmatch(text); len(m) == 3 {
		level, ok := normalizeCapabilityLevel(m[2])
		if ok {
			return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainCapability, Key: strings.TrimSpace(m[1]), Value: string(level), Confidence: 0.99, Provenance: source, Evidence: text, ObservedAt: now}}
		}
	}
	if m := goalObservationRE.FindStringSubmatch(text); len(m) == 2 {
		goal := strings.TrimSpace(m[1])
		if goal != "" {
			return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainGoal, Key: goal, Value: string(UserGoalCategoryActive), Confidence: 0.99, Provenance: source, Evidence: text, ObservedAt: now}}
		}
	}

	// All remaining explicit "mi/my ... is/es ..." declarations are generic
	// key/value attributes or preferences. The key comes directly from the
	// semantic content, so unseen attributes need no code change.
	if attribute, ok := ResolveSemanticAttribute(SemanticRequest{Raw: text, Normalized: text, Subject: UserObservationSubjectUser, SubjectConfidence: .99}); ok && attribute.HasValue {
		domain := UserModelDomainAttribute
		if SemanticAttributeDomainForKey(attribute.Key) == SemanticAttributeDomainPreference {
			domain = UserModelDomainPreference
		}
		return []UserObservation{{Subject: UserObservationSubjectUser, Domain: domain, Key: attribute.Key, Value: attribute.Value, Confidence: attribute.Confidence, Provenance: source, Evidence: text, ObservedAt: now}}
	}

	return nil
}

// DetectUserObservationsFromSemantic consumes the already-established
// interaction contract. Generic user attributes/preferences come directly
// from the structured semantic relation/grammar bridge; downstream code never
// re-classifies the raw sentence to decide what kind of fact it is.
func DetectUserObservationsFromSemantic(req SemanticRequest, now time.Time, sourceID string) []UserObservation {
	acts := CanonicalActs(req)
	if req.Subject != UserObservationSubjectUser {
		return nil
	}
	if !acts.Has(ActDeclaration) && !acts.Has(ActObservation) && !acts.Has(ActInform) {
		return nil
	}

	// B96.34 domain policies own specialized semantic domains (goal,
	// capability, knowledge, communication profile). They receive an already
	// classified turn and only map the domain's established grammar into the
	// canonical observation contract. They never enumerate individual
	// attributes such as name, city, color, drink, etc.
	if observation := detectSpecializedUserObservation(req.Raw, now, sourceID); len(observation) > 0 {
		return observation
	}

	// The generic attribute path is deliberately second. This means a domain
	// such as "mi objetivo es..." cannot be swallowed by the free-form
	// attribute grammar, while every otherwise-unseen attribute still flows
	// through the same semantic key/value contract used by query and storage.
	if attribute, ok := ResolveSemanticAttribute(req); ok && attribute.HasValue && attribute.Subject == UserObservationSubjectUser {
		if sourceID == "" {
			sourceID = "conversation"
		}
		source := UserModelSourceRef{Kind: UserSourceExplicitUser, SourceID: sourceID, ActorID: "user", ObservedAt: now}
		domain := UserModelDomainAttribute
		if SemanticAttributeDomainForKey(attribute.Key) == SemanticAttributeDomainPreference {
			domain = UserModelDomainPreference
		}
		return []UserObservation{{Subject: UserObservationSubjectUser, Domain: domain, Key: attribute.Key, Value: attribute.Value, Confidence: attribute.Confidence, Provenance: source, Evidence: attribute.Evidence, ObservedAt: now}}
	}

	return nil
}

func detectSpecializedUserObservation(raw string, now time.Time, sourceID string) []UserObservation {
	text := strings.TrimSpace(raw)
	if text == "" || isLikelyQuestion(text) {
		return nil
	}
	if sourceID == "" {
		sourceID = "conversation"
	}
	source := UserModelSourceRef{
		Kind: UserSourceExplicitUser, SourceID: sourceID, ActorID: "user", ObservedAt: now,
	}
	if dim, value, ok := detectCommunicationObservation(text); ok {
		return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainCommunicationProfile, Key: string(dim), Value: value, Confidence: .99, Provenance: source, Evidence: text, ObservedAt: now}}
	}
	if m := knowledgeObservationRE.FindStringSubmatch(text); len(m) == 3 {
		level, ok := normalizeKnowledgeLevel(m[2])
		if ok {
			return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainKnowledge, Key: strings.TrimSpace(m[1]), Value: string(level), Confidence: .99, Provenance: source, Evidence: text, ObservedAt: now}}
		}
	}
	if m := capabilityObservationRE.FindStringSubmatch(text); len(m) == 3 {
		level, ok := normalizeCapabilityLevel(m[2])
		if ok {
			return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainCapability, Key: strings.TrimSpace(m[1]), Value: string(level), Confidence: .99, Provenance: source, Evidence: text, ObservedAt: now}}
		}
	}
	if m := goalObservationRE.FindStringSubmatch(text); len(m) == 2 {
		goal := strings.TrimSpace(m[1])
		if goal != "" {
			return []UserObservation{{Subject: UserObservationSubjectUser, Domain: UserModelDomainGoal, Key: goal, Value: string(UserGoalCategoryActive), Confidence: .99, Provenance: source, Evidence: text, ObservedAt: now}}
		}
	}
	return nil
}

// ApplyUserObservation is the single mutation dispatcher from an explicit
// user observation into the canonical B96.34 E2E path. No domain writes are
// performed here directly; every accepted candidate goes through
// RunUserModelUpdateE2E so privacy, reconciliation, conflict resolution and
// canonical store replacement remain centralized.
func ApplyUserObservation(store *UserModelStore, observation UserObservation, now time.Time) (UserModelUpdateE2EOutcome, bool, error) {
	if store == nil {
		return UserModelUpdateE2EOutcome{}, false, fmt.Errorf("user observation: nil store")
	}
	if observation.Subject != UserObservationSubjectUser {
		return UserModelUpdateE2EOutcome{}, false, nil
	}
	if strings.TrimSpace(observation.Key) == "" || strings.TrimSpace(observation.Value) == "" {
		return UserModelUpdateE2EOutcome{}, false, fmt.Errorf("user observation: key and value are required")
	}

	item, accepted, err := userObservationToUpdateItem(observation)
	if err != nil || !accepted {
		return UserModelUpdateE2EOutcome{}, true, err
	}
	category := observationPrivacyCategory(observation.Domain)
	outcome, err := RunUserModelUpdateE2E(
		store, UserModelActorSystem,
		UserModelUpdateSignal{Domain: observation.Domain, Category: category, Item: item, Context: observation.Context},
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), now,
	)
	if err != nil {
		return outcome, true, err
	}
	return outcome, true, nil
}

func userObservationToUpdateItem(observation UserObservation) (UserModelUpdateItem, bool, error) {
	now := observation.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	source := observation.Provenance
	switch observation.Domain {
	case UserModelDomainPreference:
		record, accepted, err := EvaluateUserPreferenceUpdate(UserPreferenceUpdate{Key: observation.Key, Value: observation.Value, Strength: 1, Confidence: observation.Confidence, Source: source, Now: now}, DefaultUserPreferencePolicy())
		if err != nil || !accepted {
			return UserModelUpdateItem{}, accepted, err
		}
		return PreferenceUpdateItem(record), true, nil
	case UserModelDomainAttribute:
		record, accepted, err := EvaluateUserAttributeUpdate(UserAttributeUpdate{Key: observation.Key, Value: observation.Value, Confidence: observation.Confidence, Source: source, Now: now}, DefaultUserAttributePolicy())
		if err != nil || !accepted {
			return UserModelUpdateItem{}, accepted, err
		}
		return AttributeUpdateItem(record), true, nil
	case UserModelDomainGoal:
		category := UserGoalCategory(observation.Value)
		record, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{Goal: observation.Key, Category: category, Confidence: observation.Confidence, Source: source, Now: now}, DefaultUserGoalPolicy())
		if err != nil || !accepted {
			return UserModelUpdateItem{}, accepted, err
		}
		return GoalUpdateItem(record), true, nil
	case UserModelDomainCapability:
		level := UserCapabilityLevel(observation.Value)
		record, accepted, err := EvaluateUserCapabilityUpdate(UserCapabilityUpdate{Capability: observation.Key, Level: level, Confidence: observation.Confidence, Source: source, Now: now}, DefaultUserCapabilityPolicy())
		if err != nil || !accepted {
			return UserModelUpdateItem{}, accepted, err
		}
		return CapabilityUpdateItem(record), true, nil
	case UserModelDomainKnowledge:
		level := UserKnowledgeLevel(observation.Value)
		record, accepted, err := EvaluateUserKnowledgeUpdate(UserKnowledgeUpdate{Topic: observation.Key, Level: level, Confidence: observation.Confidence, Source: source, Now: now}, DefaultUserKnowledgePolicy())
		if err != nil || !accepted {
			return UserModelUpdateItem{}, accepted, err
		}
		return KnowledgeUpdateItem(record), true, nil
	case UserModelDomainCommunicationProfile:
		value := UserCommunicationProfileValue{Dimension: UserCommunicationDimension(observation.Key), Value: observation.Value, Confidence: observation.Confidence, Source: source, Status: UserModelStatusActive, UpdatedAt: now}
		if err := ValidateUserCommunicationProfileValue(value); err != nil {
			return UserModelUpdateItem{}, false, err
		}
		return UserModelUpdateItem{Key: observation.Key, Value: observation.Value, Confidence: observation.Confidence, EvidenceCount: 1, Provenance: source, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now}, true, nil
	case UserModelDomainInteractionPattern:
		return UserModelUpdateItem{Key: observation.Key, Value: observation.Key, Confidence: observation.Confidence, EvidenceCount: 1, Provenance: source, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now}, true, nil
	default:
		return UserModelUpdateItem{}, false, fmt.Errorf("user observation: unsupported domain %q", observation.Domain)
	}
}

func observationPrivacyCategory(domain UserModelDomain) UserModelDataCategory {
	switch domain {
	case UserModelDomainPreference:
		return CategoryExplicitPreference
	case UserModelDomainCommunicationProfile:
		return CategoryInteractionPreference
	case UserModelDomainGoal:
		return CategoryRelevantGoal
	case UserModelDomainCapability:
		return CategoryKnownCapability
	case UserModelDomainKnowledge:
		return CategoryKnownKnowledge
	case UserModelDomainAttribute:
		return CategoryKnownAttribute
	case UserModelDomainInteractionPattern:
		return CategoryCollaborationContext
	default:
		return UserModelDataCategory(domain)
	}
}

func isLikelyQuestion(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.HasSuffix(lower, "?") || strings.HasPrefix(lower, "qué ") || strings.HasPrefix(lower, "que ") || strings.HasPrefix(lower, "cuál ") || strings.HasPrefix(lower, "cual ") || strings.HasPrefix(lower, "cómo ") || strings.HasPrefix(lower, "como ") || strings.HasPrefix(lower, "cuánto ") || strings.HasPrefix(lower, "cuanto ")
}

// PersonalAnswerToUserObservation converts an answer that is grounded by an
// open personal question of Ahmelia into the same domain-neutral observation
// path. sourceID is the turn of the user's utterance; evidenceID is the turn
// where Ahmelia asked, so provenance never points at the wrong turn.
func PersonalAnswerToUserObservation(answer PersonalAnswer, now time.Time, sourceID string, evidenceID string) UserObservation {
	source := UserModelSourceRef{Kind: UserSourceExplicitUser, SourceID: sourceID, ActorID: "user", EvidenceID: evidenceID, ObservedAt: now}
	switch answer.Category {
	case "preference":
		return UserObservation{Subject: UserObservationSubjectUser, Domain: UserModelDomainPreference, Key: answer.Key, Value: answer.Value, Confidence: answer.Confidence, Provenance: source, Evidence: answer.Value, ObservedAt: now}
	case "attribute":
		if strings.TrimSpace(answer.Key) == "" || strings.TrimSpace(answer.Value) == "" {
			return UserObservation{}
		}
		return UserObservation{Subject: UserObservationSubjectUser, Domain: UserModelDomainAttribute, Key: answer.Key, Value: answer.Value, Confidence: answer.Confidence, Provenance: source, Evidence: answer.Value, ObservedAt: now}
	case "capability_answer":
		level, ok := normalizeCapabilityLevel(answer.Value)
		if !ok || strings.TrimSpace(answer.Key) == "" {
			return UserObservation{}
		}
		return UserObservation{Subject: UserObservationSubjectUser, Domain: UserModelDomainCapability, Key: answer.Key, Value: string(level), Confidence: answer.Confidence, Provenance: source, Evidence: answer.Value, ObservedAt: now}
	case "knowledge_answer":
		level, ok := normalizeKnowledgeLevel(answer.Value)
		if !ok || strings.TrimSpace(answer.Key) == "" {
			return UserObservation{}
		}
		return UserObservation{Subject: UserObservationSubjectUser, Domain: UserModelDomainKnowledge, Key: answer.Key, Value: string(level), Confidence: answer.Confidence, Provenance: source, Evidence: answer.Value, ObservedAt: now}
	case "goal_answer":
		if strings.TrimSpace(answer.Value) == "" {
			return UserObservation{}
		}
		return UserObservation{Subject: UserObservationSubjectUser, Domain: UserModelDomainGoal, Key: strings.TrimSpace(answer.Value), Value: string(UserGoalCategoryActive), Confidence: answer.Confidence, Provenance: source, Evidence: answer.Value, ObservedAt: now}
	}
	return UserObservation{}
}

func normalizeCapabilityLevel(raw string) (UserCapabilityLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "experto", "expert":
		return UserCapabilityExpert, true
	case "avanzado", "advanced":
		return UserCapabilityProficient, true
	case "proficiente", "proficient":
		return UserCapabilityProficient, true
	case "independiente", "independent":
		return UserCapabilityIndependent, true
	case "asistido", "assisted":
		return UserCapabilityAssisted, true
	case "ninguno", "ninguna", "none":
		return UserCapabilityNone, true
	default:
		return "", false
	}
}

func normalizeKnowledgeLevel(raw string) (UserKnowledgeLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "experto", "expert":
		return UserKnowledgeExpert, true
	case "avanzado", "advanced":
		return UserKnowledgeAdvanced, true
	case "trabajando", "working":
		return UserKnowledgeWorking, true
	case "familiar", "familiarizado", "familiarizada":
		return UserKnowledgeFamiliar, true
	case "consciente", "aware":
		return UserKnowledgeAware, true
	case "desconocido", "unknown":
		return UserKnowledgeUnknown, true
	default:
		return "", false
	}
}

func detectCommunicationObservation(raw string) (UserCommunicationDimension, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(lower, "direct"):
		return CommunicationDirectness, raw, true
	case strings.Contains(lower, "breve"), strings.Contains(lower, "conciso"), strings.Contains(lower, "corto"):
		return CommunicationDetailTolerance, raw, true
	case strings.Contains(lower, "detall"), strings.Contains(lower, "completo"), strings.Contains(lower, "exhaustivo"):
		return CommunicationDetailTolerance, raw, true
	case strings.Contains(lower, "técnic"), strings.Contains(lower, "tecnic"):
		return CommunicationTechnicalDepth, raw, true
	default:
		return "", "", false
	}
}
