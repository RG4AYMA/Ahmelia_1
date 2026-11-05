package initiative

import (
	"errors"
	"strings"
	"time"
)

// B96.37.11 — Privacy & Information Boundaries.
// This layer evaluates disclosure boundaries and enriches an opportunity. It
// never authorizes an initiative, chooses timing, or generates content.

type PrivacyLevel string

const (
	PrivacyPublic     PrivacyLevel = "PUBLIC"
	PrivacyInternal   PrivacyLevel = "INTERNAL"
	PrivacyRestricted PrivacyLevel = "RESTRICTED"
	PrivacyPrivate    PrivacyLevel = "PRIVATE"
	PrivacySensitive  PrivacyLevel = "SENSITIVE"
	PrivacyProhibited PrivacyLevel = "PROHIBITED"
)

type InformationClassification string

const (
	ClassificationPublic       InformationClassification = "PUBLIC"
	ClassificationInternal     InformationClassification = "INTERNAL"
	ClassificationConfidential InformationClassification = "CONFIDENTIAL"
	ClassificationPrivate      InformationClassification = "PRIVATE"
	ClassificationSensitive    InformationClassification = "SENSITIVE"
	ClassificationProhibited   InformationClassification = "PROHIBITED"
)

type SourceSensitivity string

const (
	SourceSensitivityUnknown  SourceSensitivity = "UNKNOWN"
	SourceSensitivityLow      SourceSensitivity = "LOW"
	SourceSensitivityMedium   SourceSensitivity = "MEDIUM"
	SourceSensitivityHigh     SourceSensitivity = "HIGH"
	SourceSensitivityCritical SourceSensitivity = "CRITICAL"
)

type AudienceEnvironment string

const (
	AudienceUserOnly         AudienceEnvironment = "USER_ONLY"
	AudienceUserAndAssistant AudienceEnvironment = "USER_AND_ASSISTANT"
	AudiencePrivateWorkspace AudienceEnvironment = "PRIVATE_WORKSPACE"
	AudienceSharedWorkspace  AudienceEnvironment = "SHARED_WORKSPACE"
	AudienceExternal         AudienceEnvironment = "EXTERNAL"
	AudienceUnknown          AudienceEnvironment = "UNKNOWN"
)

type PermissionResult string

const (
	PermissionAllowed    PermissionResult = "ALLOWED"
	PermissionRestricted PermissionResult = "RESTRICTED"
	PermissionDenied     PermissionResult = "DENIED"
	PermissionUnknown    PermissionResult = "UNKNOWN"
)

type PrivacyConflict string

const (
	PrivacyConflictNone           PrivacyConflict = "NONE"
	PrivacyConflictClassification PrivacyConflict = "CLASSIFICATION_CONFLICT"
	PrivacyConflictSource                         = "SOURCE_SENSITIVITY_CONFLICT"
	PrivacyConflictAudience                       = "AUDIENCE_CONFLICT"
	PrivacyConflictChannel                        = "CHANNEL_CONFLICT"
	PrivacyConflictPermission                     = "PERMISSION_CONFLICT"
)

type PrivacyContext struct {
	Classification      InformationClassification
	SourceSensitivity   SourceSensitivity
	Audience            AudienceEnvironment
	Channel             string
	Permission          PermissionResult
	UserPrivate         bool
	HistoricalMemory    bool
	UserModel           bool
	WorkProject         bool
	ExplicitDisclosure  bool
	RequestedDisclosure bool
	Now                 time.Time
}

type DisclosurePolicy struct {
	Version                               string
	AllowExternalDisclosure               bool
	AllowSharedWorkspace                  bool
	AllowHistoricalMemoryDisclosure       bool
	AllowUserModelDisclosure              bool
	AllowWorkProjectDisclosure            bool
	RequireExplicitPermissionForPrivate   bool
	RequireExplicitPermissionForSensitive bool
}

func DefaultDisclosurePolicy() DisclosurePolicy {
	return DisclosurePolicy{Version: "B96.37.11", AllowExternalDisclosure: false, AllowSharedWorkspace: false,
		AllowHistoricalMemoryDisclosure: true, AllowUserModelDisclosure: true, AllowWorkProjectDisclosure: true,
		RequireExplicitPermissionForPrivate: true, RequireExplicitPermissionForSensitive: true}
}

type PrivacyAssessment struct {
	Level                   PrivacyLevel              `json:"level"`
	Classification          InformationClassification `json:"classification"`
	SourceSensitivity       SourceSensitivity         `json:"source_sensitivity"`
	Permission              PermissionResult          `json:"permission"`
	DisclosureAllowed       bool                      `json:"disclosure_allowed"`
	UserPrivate             bool                      `json:"user_private"`
	HistoricalMemoryAllowed bool                      `json:"historical_memory_allowed"`
	UserModelAllowed        bool                      `json:"user_model_allowed"`
	WorkProjectAllowed      bool                      `json:"work_project_allowed"`
	ChannelAllowed          bool                      `json:"channel_allowed"`
	AudienceAllowed         bool                      `json:"audience_allowed"`
	Conflict                PrivacyConflict           `json:"conflict"`
	Reason                  string                    `json:"reason"`
	EvaluatedAt             time.Time                 `json:"evaluated_at"`
}

func (a PrivacyAssessment) Validate() error {
	if a.EvaluatedAt.IsZero() {
		return errors.New("B96.37.11 assessment requires evaluation time")
	}
	if !validPrivacyLevel(a.Level) {
		return errors.New("invalid B96.37.11 privacy level")
	}
	if !validClassification(a.Classification) {
		return errors.New("invalid B96.37.11 classification")
	}
	if a.DisclosureAllowed && (!a.ChannelAllowed || !a.AudienceAllowed) {
		return errors.New("allowed disclosure requires channel and audience permission")
	}
	return nil
}

func validPrivacyLevel(v PrivacyLevel) bool {
	switch v {
	case PrivacyPublic, PrivacyInternal, PrivacyRestricted, PrivacyPrivate, PrivacySensitive, PrivacyProhibited:
		return true
	}
	return false
}
func validClassification(v InformationClassification) bool {
	switch v {
	case ClassificationPublic, ClassificationInternal, ClassificationConfidential, ClassificationPrivate, ClassificationSensitive, ClassificationProhibited:
		return true
	}
	return false
}

func classificationToLevel(c InformationClassification) PrivacyLevel {
	switch c {
	case ClassificationPublic:
		return PrivacyPublic
	case ClassificationInternal:
		return PrivacyInternal
	case ClassificationConfidential:
		return PrivacyRestricted
	case ClassificationPrivate:
		return PrivacyPrivate
	case ClassificationSensitive:
		return PrivacySensitive
	case ClassificationProhibited:
		return PrivacyProhibited
	}
	return PrivacyRestricted
}

func sourceSensitivityFor(c PrivacyContext) SourceSensitivity {
	if c.SourceSensitivity != SourceSensitivityUnknown {
		return c.SourceSensitivity
	}
	if c.UserPrivate || c.UserModel || c.HistoricalMemory {
		return SourceSensitivityHigh
	}
	if c.WorkProject {
		return SourceSensitivityMedium
	}
	return SourceSensitivityLow
}

func EvaluatePrivacy(ctx PrivacyContext, policy DisclosurePolicy) PrivacyAssessment {
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}
	level := classificationToLevel(ctx.Classification)
	source := sourceSensitivityFor(ctx)
	a := PrivacyAssessment{Level: level, Classification: ctx.Classification, SourceSensitivity: source, Permission: ctx.Permission,
		UserPrivate: ctx.UserPrivate, EvaluatedAt: now, HistoricalMemoryAllowed: policy.AllowHistoricalMemoryDisclosure,
		UserModelAllowed: policy.AllowUserModelDisclosure, WorkProjectAllowed: policy.AllowWorkProjectDisclosure,
		ChannelAllowed: true, AudienceAllowed: true}
	if ctx.Permission == PermissionDenied {
		a.DisclosureAllowed = false
		a.Conflict = PrivacyConflictPermission
		a.Reason = "explicit_permission_denied"
		return a
	}
	if ctx.Audience == AudienceExternal && !policy.AllowExternalDisclosure {
		a.AudienceAllowed = false
		a.Conflict = PrivacyConflictAudience
	}
	if ctx.Audience == AudienceSharedWorkspace && !policy.AllowSharedWorkspace {
		a.AudienceAllowed = false
		if a.Conflict == PrivacyConflictNone {
			a.Conflict = PrivacyConflictAudience
		}
	}
	ch := strings.ToLower(strings.TrimSpace(ctx.Channel))
	if ch == "" || ch == "unknown" {
		a.ChannelAllowed = true
	} else if ch == "external" || ch == "public" {
		a.ChannelAllowed = policy.AllowExternalDisclosure
		if !a.ChannelAllowed && a.Conflict == PrivacyConflictNone {
			a.Conflict = PrivacyConflictChannel
		}
	}
	if ctx.HistoricalMemory && !policy.AllowHistoricalMemoryDisclosure {
		a.ChannelAllowed = false
		if a.Conflict == PrivacyConflictNone {
			a.Conflict = PrivacyConflictPermission
		}
	}
	if ctx.UserModel && !policy.AllowUserModelDisclosure {
		a.ChannelAllowed = false
		if a.Conflict == PrivacyConflictNone {
			a.Conflict = PrivacyConflictPermission
		}
	}
	if ctx.WorkProject && !policy.AllowWorkProjectDisclosure {
		a.ChannelAllowed = false
		if a.Conflict == PrivacyConflictNone {
			a.Conflict = PrivacyConflictPermission
		}
	}
	if level == PrivacyPrivate && policy.RequireExplicitPermissionForPrivate && !ctx.ExplicitDisclosure {
		a.DisclosureAllowed = false
		a.Conflict = PrivacyConflictPermission
		a.Reason = "private_information_requires_explicit_permission"
		return a
	}
	if level == PrivacySensitive && policy.RequireExplicitPermissionForSensitive && !ctx.ExplicitDisclosure {
		a.DisclosureAllowed = false
		a.Conflict = PrivacyConflictClassification
		a.Reason = "sensitive_information_requires_explicit_disclosure"
		return a
	}
	if level == PrivacyProhibited {
		a.DisclosureAllowed = false
		a.Conflict = PrivacyConflictClassification
		a.Reason = "prohibited_information"
		return a
	}
	if source == SourceSensitivityCritical && !ctx.ExplicitDisclosure {
		a.DisclosureAllowed = false
		a.Conflict = PrivacyConflictSource
		a.Reason = "critical_source_requires_explicit_disclosure"
		return a
	}
	if !a.AudienceAllowed {
		a.DisclosureAllowed = false
		a.Reason = "audience_not_authorized"
		return a
	}
	if !a.ChannelAllowed {
		a.DisclosureAllowed = false
		a.Reason = "channel_not_authorized"
		return a
	}
	a.DisclosureAllowed = true
	a.Permission = PermissionAllowed
	a.Reason = "within_privacy_boundary"
	return a
}

// EvaluateDisclosurePolicy is the public policy entry point.
func EvaluateDisclosurePolicy(ctx PrivacyContext, policy DisclosurePolicy) (PrivacyAssessment, error) {
	if policy.Version == "" {
		policy.Version = "B96.37.11"
	}
	a := EvaluatePrivacy(ctx, policy)
	return a, a.Validate()
}

func InformationClassificationFromLegacy(p InformationPolicy) InformationClassification {
	switch p {
	case InformationAllowed:
		return ClassificationPublic
	case InformationRestricted:
		return ClassificationConfidential
	case InformationPrivate:
		return ClassificationPrivate
	case InformationSensitive:
		return ClassificationSensitive
	case InformationProhibited:
		return ClassificationProhibited
	default:
		return ClassificationConfidential
	}
}
func LegacyInformationPolicy(c InformationClassification) InformationPolicy {
	switch c {
	case ClassificationPublic:
		return InformationAllowed
	case ClassificationInternal, ClassificationConfidential:
		return InformationRestricted
	case ClassificationPrivate:
		return InformationPrivate
	case ClassificationSensitive:
		return InformationSensitive
	case ClassificationProhibited:
		return InformationProhibited
	default:
		return InformationRestricted
	}
}

type PrivacyOpportunityAssessment struct {
	Privacy           PrivacyAssessment `json:"privacy"`
	DisclosureAllowed bool              `json:"disclosure_allowed"`
	Enriched          bool              `json:"enriched"`
}

// EnrichPrivacyOpportunity modifies only the opportunity's privacy metadata.
// It cannot construct or return an InitiativeDecision.
func EnrichPrivacyOpportunity(op InitiativeOpportunity, ctx PrivacyContext, policy DisclosurePolicy, now time.Time) (InitiativeOpportunity, PrivacyOpportunityAssessment, error) {
	if now.IsZero() {
		now = time.Now()
	}
	ctx.Now = now
	if ctx.Classification == "" {
		ctx.Classification = InformationClassificationFromLegacy(op.Privacy)
	}
	if ctx.Channel == "" {
		ctx.Channel = op.Channel
	}
	a, err := EvaluateDisclosurePolicy(ctx, policy)
	if err != nil {
		return op, PrivacyOpportunityAssessment{Privacy: a}, err
	}
	op.Privacy = LegacyInformationPolicy(a.Classification)
	if !a.DisclosureAllowed {
		op.Privacy = InformationPrivate
	}
	op.Sensitivity = string(a.Level)
	if !a.DisclosureAllowed {
		op.Signal.Relevant = false
	}
	return op, PrivacyOpportunityAssessment{Privacy: a, DisclosureAllowed: a.DisclosureAllowed, Enriched: true}, nil
}

// LegacyPrivacyMigration normalizes the historical InformationPolicy contract.
func LegacyPrivacyMigration(p InformationPolicy) (InformationPolicy, bool) {
	switch p {
	case InformationAllowed, InformationRestricted, InformationPrivate, InformationSensitive, InformationProhibited:
		return p, false
	default:
		return InformationRestricted, true
	}
}

// ClassifyInformation is the explicit classification boundary for 11.2.
func ClassifyInformation(ctx PrivacyContext) InformationClassification {
	if ctx.Classification != "" {
		return ctx.Classification
	}
	if ctx.UserPrivate || ctx.HistoricalMemory || ctx.UserModel {
		return ClassificationPrivate
	}
	if ctx.WorkProject {
		return ClassificationConfidential
	}
	return ClassificationPublic
}

// SourceSensitivityFor returns source sensitivity without changing disclosure authority.
func SourceSensitivityFor(ctx PrivacyContext) SourceSensitivity { return sourceSensitivityFor(ctx) }

// EvaluatePermission is the narrow permission boundary for 11.11.
func EvaluatePermission(ctx PrivacyContext, policy DisclosurePolicy) PermissionResult {
	if ctx.Permission == PermissionDenied {
		return PermissionDenied
	}
	a := EvaluatePrivacy(ctx, policy)
	if a.DisclosureAllowed {
		return PermissionAllowed
	}
	if a.Conflict == PrivacyConflictNone {
		return PermissionUnknown
	}
	return PermissionRestricted
}

// ResolvePrivacyConflict deterministically selects the strictest applicable
// boundary. Conflict resolution never becomes initiative authorization.
func ResolvePrivacyConflict(ctx PrivacyContext, policy DisclosurePolicy) PrivacyConflict {
	a := EvaluatePrivacy(ctx, policy)
	return a.Conflict
}

// ChannelDisclosureAllowed answers only whether the channel can carry the
// classified information under policy. It does not answer whether Ahmelia
// should speak.
func ChannelDisclosureAllowed(channel string, audience AudienceEnvironment, policy DisclosurePolicy) bool {
	c := PrivacyContext{Classification: ClassificationPublic, Channel: channel, Audience: audience, ExplicitDisclosure: true}
	return EvaluatePrivacy(c, policy).ChannelAllowed && EvaluatePrivacy(c, policy).AudienceAllowed
}
