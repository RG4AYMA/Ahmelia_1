package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserCommunicationDimension identifies a stable communication preference.
type UserCommunicationDimension string

const (
	CommunicationFormality             UserCommunicationDimension = "formality"
	CommunicationDirectness            UserCommunicationDimension = "directness"
	CommunicationDetailTolerance       UserCommunicationDimension = "detail_tolerance"
	CommunicationTechnicalDepth        UserCommunicationDimension = "technical_depth"
	CommunicationHumorTolerance        UserCommunicationDimension = "humor_tolerance"
	CommunicationCorrectionPreference  UserCommunicationDimension = "correction_preference"
	CommunicationExplanationPreference UserCommunicationDimension = "explanation_preference"
	CommunicationInitiativePreference  UserCommunicationDimension = "initiative_preference"
)

// UserCommunicationEvidence records the observable or explicit basis for a
// communication-profile belief.
type UserCommunicationEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserCommunicationProfileValue is the richer B96.34.4 semantic form of one
// communication dimension. It remains auditable and does not imply certainty
// merely because a value exists.
type UserCommunicationProfileValue struct {
	Dimension  UserCommunicationDimension  `json:"dimension"`
	Value      string                      `json:"value"`
	Confidence float64                     `json:"confidence"`
	Source     UserModelSourceRef          `json:"source"`
	Evidence   []UserCommunicationEvidence `json:"evidence,omitempty"`
	Status     UserModelItemStatus         `json:"status"`
	CreatedAt  time.Time                   `json:"created_at"`
	UpdatedAt  time.Time                   `json:"updated_at"`
}

// UserCommunicationProfile is a scoped collection of learned communication
// dimensions. It is intentionally distinct from UserState and from generic
// preferences: these values describe how communication should work.
type UserCommunicationProfile struct {
	Values map[UserCommunicationDimension]UserCommunicationProfileValue `json:"values,omitempty"`
}

// UserCommunicationProfileUpdate is a proposal; it does not mutate UserModel.
type UserCommunicationProfileUpdate struct {
	Dimension  UserCommunicationDimension
	Value      string
	Confidence float64
	Source     UserModelSourceRef
	Evidence   []UserCommunicationEvidence
	Now        time.Time
}

// UserCommunicationProfilePolicy prevents weak observations from becoming
// durable communication traits. Full reconciliation remains B96.34.11.
type UserCommunicationProfilePolicy struct {
	MinInferredConfidence float64
	MinObservedConfidence float64
	MinEvidenceCount      int
}

func DefaultUserCommunicationProfilePolicy() UserCommunicationProfilePolicy {
	return UserCommunicationProfilePolicy{
		MinInferredConfidence: 0.85,
		MinObservedConfidence: 0.90,
		MinEvidenceCount:      2,
	}
}

func validCommunicationDimension(d UserCommunicationDimension) bool {
	switch d {
	case CommunicationFormality, CommunicationDirectness,
		CommunicationDetailTolerance, CommunicationTechnicalDepth,
		CommunicationHumorTolerance, CommunicationCorrectionPreference,
		CommunicationExplanationPreference, CommunicationInitiativePreference:
		return true
	default:
		return false
	}
}

// EvaluateUserCommunicationProfileUpdate classifies a proposed trait. An
// inferred value can be accepted as an uncertain candidate, but never as an
// active fact solely because it crossed a local inference threshold.
func EvaluateUserCommunicationProfileUpdate(update UserCommunicationProfileUpdate, policy UserCommunicationProfilePolicy) (UserCommunicationProfileValue, bool, error) {
	if !validCommunicationDimension(update.Dimension) {
		return UserCommunicationProfileValue{}, false, errors.New("communication profile: invalid dimension")
	}
	if strings.TrimSpace(update.Value) == "" {
		return UserCommunicationProfileValue{}, false, errors.New("communication profile: value is required")
	}
	if !validConfidence(update.Confidence) {
		return UserCommunicationProfileValue{}, false, errors.New("communication profile: confidence must be between 0 and 1")
	}
	if !validSourceKind(update.Source.Kind) {
		return UserCommunicationProfileValue{}, false, errors.New("communication profile: invalid source kind")
	}
	if policy.MinEvidenceCount < 1 {
		return UserCommunicationProfileValue{}, false, errors.New("communication profile: minimum evidence count must be positive")
	}

	value := UserCommunicationProfileValue{
		Dimension:  update.Dimension,
		Value:      strings.TrimSpace(update.Value),
		Confidence: update.Confidence,
		Source:     update.Source,
		Evidence:   append([]UserCommunicationEvidence(nil), update.Evidence...),
		Status:     UserModelStatusUncertain,
		CreatedAt:  update.Now,
		UpdatedAt:  update.Now,
	}

	switch update.Source.Kind {
	case UserSourceExplicitUser:
		value.Status = UserModelStatusActive
		return value, true, nil
	case UserSourceObserved:
		if update.Confidence >= policy.MinObservedConfidence && len(update.Evidence) >= policy.MinEvidenceCount {
			value.Status = UserModelStatusActive
			return value, true, nil
		}
	case UserSourceInferred:
		if update.Confidence >= policy.MinInferredConfidence && len(update.Evidence) >= policy.MinEvidenceCount {
			return value, true, nil
		}
	case UserSourceSystem, UserSourceImported:
		if update.Confidence >= 1.0 && len(update.Evidence) >= 1 {
			value.Status = UserModelStatusActive
			return value, true, nil
		}
	}

	return value, false, nil
}

// ValidateUserCommunicationProfileValue validates structure, not truth.
func ValidateUserCommunicationProfileValue(value UserCommunicationProfileValue) error {
	if !validCommunicationDimension(value.Dimension) {
		return errors.New("communication profile: invalid dimension")
	}
	if strings.TrimSpace(value.Value) == "" {
		return fmt.Errorf("communication profile %q: value is required", value.Dimension)
	}
	if !validConfidence(value.Confidence) {
		return fmt.Errorf("communication profile %q: confidence must be between 0 and 1", value.Dimension)
	}
	if !validSourceKind(value.Source.Kind) {
		return fmt.Errorf("communication profile %q: invalid source kind", value.Dimension)
	}
	if !validStatus(value.Status) {
		return fmt.Errorf("communication profile %q: invalid status", value.Dimension)
	}
	seen := map[string]struct{}{}
	for i, evidence := range value.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("communication profile %q: evidence %d has empty id", value.Dimension, i)
		}
		if _, ok := seen[evidence.ID]; ok {
			return fmt.Errorf("communication profile %q: duplicate evidence id %q", value.Dimension, evidence.ID)
		}
		seen[evidence.ID] = struct{}{}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("communication profile %q: evidence %q has invalid source", value.Dimension, evidence.ID)
		}
	}
	return nil
}

// CommunicationProfileToCore projects only the canonical core map. The rich
// value remains the authoritative B96.34.4 representation; no second route
// for interpretation or update is introduced here.
func (p UserCommunicationProfile) CommunicationProfileToCore() UserInteractionProfile {
	result := UserInteractionProfile{Values: map[string]UserInteractionProfileValue{}}
	for dimension, value := range p.Values {
		result.Values[string(dimension)] = UserInteractionProfileValue{
			Value:      value.Value,
			Confidence: value.Confidence,
			Provenance: value.Source,
			Status:     value.Status,
			UpdatedAt:  value.UpdatedAt,
		}
	}
	return result
}

// NewUserCommunicationProfile creates an empty profile without inferring any
// communication trait.
func NewUserCommunicationProfile() UserCommunicationProfile {
	return UserCommunicationProfile{Values: map[UserCommunicationDimension]UserCommunicationProfileValue{}}
}
