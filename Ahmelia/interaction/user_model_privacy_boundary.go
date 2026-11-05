package interaction

import (
	"errors"
	"fmt"
	"strings"
)

// B96.34.14 — User Model Privacy / Boundary Model.
//
// Aunque no sea un sistema de seguridad completo, este bloque pone el
// contrato: el User Model debe tener límites.
//
//	Allowed
//	├── explicit preferences
//	├── interaction preferences
//	├── relevant goals
//	├── known capabilities
//	└── collaboration context
//
//	Restricted
//	├── unnecessary sensitive inference
//	├── speculative attributes
//	└── permanent conclusions from weak evidence
//
// Y además: Ahmelia debe poder representar "no sé". Eso es tan importante
// como poder representar "sé".
//
// This block does not introduce a new belief domain, does not decide any
// domain's own admission policy (B96.34.2..34.8 keep that), and does not
// mutate UserModel. It sits where every earlier block already pointed:
// B96.34.12 deferred to it for the "no sé" contract, B96.34.13 deferred to
// it by the same name, and B96.34.11's confidence/evidence gating narrows
// what reaches here without replacing it — B96.34.11 asks "is this proposal
// internally well-formed enough to be a candidate at all", this block asks
// "is this candidate the kind of thing the User Model is allowed to hold,
// coming from where it came from". Both must say yes.
//
// Restricted is not "never store". A sensitive topic disclosed explicitly by
// the user is not an inference — it is exactly the "explicit preferences /
// relevant goals / collaboration context" the Allowed list already welcomes,
// still filed through the ordinary domain and update-policy path with the
// weight UserSourceExplicitUser already carries everywhere else in B96.34.
// What this block refuses is a sensitive belief the model itself proposed —
// an unnecessary sensitive inference — regardless of how confident the
// inference claims to be.

// UserModelDataCategory classifies what an admitted UserModel item is about,
// for the Allowed/Restricted boundary this block owns. It is not a query
// filter (B96.34.12 owns relevance) and not an admission gate on its own
// domain's shape (each B96.34.x domain block owns that).
type UserModelDataCategory string

const (
	CategoryExplicitPreference    UserModelDataCategory = "explicit_preference"
	CategoryInteractionPreference UserModelDataCategory = "interaction_preference"
	CategoryRelevantGoal          UserModelDataCategory = "relevant_goal"
	CategoryKnownCapability       UserModelDataCategory = "known_capability"
	CategoryCollaborationContext  UserModelDataCategory = "collaboration_context"
	CategoryKnownKnowledge        UserModelDataCategory = "known_knowledge"
	CategoryKnownAttribute        UserModelDataCategory = "known_attribute"
)

// AllowedUserModelCategories is the fixed Allowed list from the B96.34.14
// spec. It is exhaustive by design: a category outside this set is not
// something this block's default policy admits, on the theory that a new
// admitted category should be a deliberate spec change, not a silent side
// effect of some future caller passing a new string.
func AllowedUserModelCategories() []UserModelDataCategory {
	return []UserModelDataCategory{
		CategoryExplicitPreference,
		CategoryInteractionPreference,
		CategoryRelevantGoal,
		CategoryKnownCapability,
		CategoryCollaborationContext,
		CategoryKnownKnowledge,
		CategoryKnownAttribute,
	}
}

func isAllowedUserModelCategory(category UserModelDataCategory) bool {
	for _, c := range AllowedUserModelCategories() {
		if c == category {
			return true
		}
	}
	return false
}

// UserModelRestrictionReason names which Restricted concern this block
// applied. Exactly one applies per rejection; a candidate can independently
// fail more than one rule, but EvaluateUserModelPrivacyBoundary reports the
// first it finds, in the order the spec lists them.
type UserModelRestrictionReason string

const (
	RestrictionNone                   UserModelRestrictionReason = ""
	RestrictionSensitiveInference     UserModelRestrictionReason = "unnecessary_sensitive_inference"
	RestrictionSpeculativeAttribute   UserModelRestrictionReason = "speculative_attribute"
	RestrictionWeakEvidenceConclusion UserModelRestrictionReason = "permanent_conclusion_from_weak_evidence"
	RestrictionUnclassifiedCategory   UserModelRestrictionReason = "unclassified_category"
)

// sensitiveTopics maps a human-readable topic label to the substrings this
// block matches against a candidate's Key and Value (case-insensitive,
// trimmed). This is a keyword boundary, not a classifier: it exists to catch
// the sensitive-inference case, not to judge whether a topic is sensitive in
// some deeper sense. Matching text is what makes RestrictionSensitiveInference
// auditable — a caller disputing a rejection can see exactly which keyword
// fired instead of trusting an opaque model judgment.
var sensitiveTopics = map[string][]string{
	"health": {
		"health", "medical", "illness", "disease", "diagnosis", "diagnosed",
		"medication", "therapy", "mental_health", "mental health", "disorder",
		"disability", "salud", "enfermedad", "diagnóstico", "medicamento",
	},
	"sexual_orientation_or_gender_identity": {
		"sexual_orientation", "sexual orientation", "gender_identity",
		"gender identity", "orientación sexual", "identidad de género",
	},
	"religion": {
		"religion", "religious", "faith", "religión", "creencia religiosa",
	},
	"political_affiliation": {
		"political", "politics", "party_affiliation", "ideology",
		"política", "afiliación política", "ideología",
	},
	"immigration_status": {
		"immigration_status", "immigration status", "visa_status",
		"estatus migratorio", "estado migratorio",
	},
	"criminal_history": {
		"criminal", "arrest", "conviction", "crime", "antecedentes penales",
	},
	"financial_details": {
		"income", "salary", "debt", "bankruptcy", "financial_hardship",
		"ingresos", "salario", "deuda", "bancarrota",
	},
	"addiction_or_recovery": {
		"addiction", "substance_abuse", "alcoholism", "recovery_program",
		"adicción", "abuso de sustancias",
	},
	"family_conflict_or_abuse": {
		"domestic_violence", "family_conflict", "abuse", "abuso",
		"violencia doméstica",
	},
	"ethnicity_or_national_origin": {
		"ethnicity", "race", "national_origin", "etnia", "origen nacional",
	},
}

// ClassifySensitiveTopic reports whether key or value contains a keyword
// this block treats as a sensitive topic, and which topic label matched
// first. It never inspects Provenance or Confidence — those decide what to
// do with the match, not whether one exists.
func ClassifySensitiveTopic(key, value string) (matched bool, topic string) {
	haystack := strings.ToLower(strings.TrimSpace(key) + " " + strings.TrimSpace(value))
	if haystack == "" {
		return false, ""
	}
	// Deterministic order: map iteration is randomized in Go, and this
	// function's result must not depend on iteration order when a
	// candidate happens to match more than one topic.
	for _, topicKey := range sensitiveTopicOrder {
		for _, kw := range sensitiveTopics[topicKey] {
			if strings.Contains(haystack, kw) {
				return true, topicKey
			}
		}
	}
	return false, ""
}

var sensitiveTopicOrder = []string{
	"health",
	"sexual_orientation_or_gender_identity",
	"religion",
	"political_affiliation",
	"immigration_status",
	"criminal_history",
	"financial_details",
	"addiction_or_recovery",
	"family_conflict_or_abuse",
	"ethnicity_or_national_origin",
}

// UserModelPrivacyPolicy is intentionally the only tunable in this block,
// matching B96.34.11's and B96.34.13's single-knob style.
type UserModelPrivacyPolicy struct {
	// MinimumConfidenceForActive is the floor a proposal's Confidence must
	// clear to enter as Active, independent of evidence count or source
	// priority. This is the "permanent conclusions from weak evidence"
	// rule: B96.34.11's guardSingleObservationPromotion already catches a
	// single Observed/Inferred occurrence, but a proposal can clear that bar
	// (evidence_count >= 2) while still carrying low confidence — this
	// closes that gap for every source kind, ExplicitUser included, since a
	// low-confidence explicit statement ("creo que tal vez prefiero...") is
	// still a weak conclusion.
	MinimumConfidenceForActive float64
}

func DefaultUserModelPrivacyPolicy() UserModelPrivacyPolicy {
	return UserModelPrivacyPolicy{MinimumConfidenceForActive: 0.5}
}

// UserModelPrivacyDecision documents what this block decided about one
// candidate, for the B96.34.10 audit trail.
type UserModelPrivacyDecision struct {
	Allowed  bool
	Category UserModelDataCategory
	Reason   UserModelRestrictionReason
	Detail   string
	// DowngradeToUncertain is set when the candidate is not rejected outright
	// but must not enter as Active. The caller (the domain's own update path,
	// upstream of B96.34.11) is expected to resubmit with Status set to
	// UserModelStatusUncertain rather than discard the proposal — a
	// low-confidence belief is still "no sé con certeza", not "nada".
	DowngradeToUncertain bool
}

// EvaluateUserModelPrivacyBoundary is the single entry point this block
// offers. category is supplied by the caller — ultimately the domain block
// proposing the item — the same way B96.34.13's Context is caller-supplied
// rather than inferred: this block never guesses what an item is "about"
// beyond the keyword check ClassifySensitiveTopic already makes explicit.
//
// It does not mutate item and does not touch UserModel; it only answers
// whether item may proceed toward B96.34.11's ReconcileUserModelUpdate.
func EvaluateUserModelPrivacyBoundary(item UserModelUpdateItem, category UserModelDataCategory, policy UserModelPrivacyPolicy) (UserModelPrivacyDecision, error) {
	if err := ValidateUserModelUpdateItem(item); err != nil {
		return UserModelPrivacyDecision{}, err
	}
	if policy.MinimumConfidenceForActive < 0 || policy.MinimumConfidenceForActive > 1 {
		return UserModelPrivacyDecision{}, errors.New("user model privacy boundary: minimum confidence for active must be between 0 and 1")
	}
	if !isAllowedUserModelCategory(category) {
		return UserModelPrivacyDecision{
			Allowed: false, Category: category, Reason: RestrictionUnclassifiedCategory,
			Detail: fmt.Sprintf("category %q is not one of the Allowed collaboration categories", category),
		}, nil
	}

	if matched, topic := ClassifySensitiveTopic(item.Key, item.Value); matched {
		if item.Provenance.Kind != UserSourceExplicitUser {
			return UserModelPrivacyDecision{
				Allowed: false, Category: category, Reason: RestrictionSensitiveInference,
				Detail: fmt.Sprintf("key/value matches sensitive topic %q from a %s source; sensitive topics are only admitted from an explicit user statement, never inferred or observed", topic, item.Provenance.Kind),
			}, nil
		}
	}

	if item.Status == UserModelStatusActive && item.Confidence < policy.MinimumConfidenceForActive {
		return UserModelPrivacyDecision{
			Allowed: true, Category: category, Reason: RestrictionSpeculativeAttribute,
			Detail:               fmt.Sprintf("confidence %.2f is below the %.2f floor required to enter as Active", item.Confidence, policy.MinimumConfidenceForActive),
			DowngradeToUncertain: true,
		}, nil
	}

	return UserModelPrivacyDecision{Allowed: true, Category: category, Reason: RestrictionNone, Detail: "within allowed boundary"}, nil
}

// UserModelKnowledgeState makes "no sé" a first-class answer rather than an
// empty slice, a zero value, or the absence of an error. Every B96.34.12
// query already carries the raw ingredients (found, contested, Status) —
// this block is where those are read the same way every time, so a caller
// never has to reconstruct "does the model actually know this" from three
// separate signals on its own.
type UserModelKnowledgeState string

const (
	// UserModelKnown: exactly one live, Active belief answers the question.
	UserModelKnown UserModelKnowledgeState = "known"
	// UserModelUncertain: a live belief exists but is not yet Active —
	// either it was never promoted, or B96.34.14's own confidence floor
	// above held it back. This is "sé algo, pero no con certeza", not "no
	// sé".
	UserModelUncertain UserModelKnowledgeState = "uncertain"
	// UserModelContested: more than one live belief disagrees at this key
	// and B96.34.13 has not (yet) resolved which one stands.
	UserModelContested UserModelKnowledgeState = "contested"
	// UserModelUnknown: no live belief exists at all. This is the "no sé"
	// state the spec asks to be representable, distinct from Uncertain.
	UserModelUnknown UserModelKnowledgeState = "unknown"
)

// ClassifyUserModelKnowledgeState turns the raw signals a B96.34.12 query
// already returns (found, contested, and the live item's Status when found)
// into one explicit state. status is ignored when found is false.
func ClassifyUserModelKnowledgeState(found, contested bool, status UserModelItemStatus) UserModelKnowledgeState {
	if !found {
		return UserModelUnknown
	}
	if contested {
		return UserModelContested
	}
	if status == UserModelStatusActive {
		return UserModelKnown
	}
	return UserModelUncertain
}
