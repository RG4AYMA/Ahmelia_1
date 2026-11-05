package interaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserModel is the durable, auditable representation of relatively stable
// information about a user. It is intentionally separate from UserState:
// UserState describes the current interaction; UserModel describes beliefs
// that may persist across interactions.
//
// B96.34.1 establishes the contract only. Later B96.34 blocks populate and
// update the domain-specific collections through validated boundaries.
type UserModel struct {
	UserID              string                   `json:"user_id"`
	Attributes          []UserAttribute          `json:"attributes,omitempty"`
	Preferences         []UserPreference         `json:"preferences,omitempty"`
	Goals               []UserGoal               `json:"goals,omitempty"`
	Capabilities        []UserCapability         `json:"capabilities,omitempty"`
	Knowledge           []UserKnowledgeItem      `json:"knowledge,omitempty"`
	InteractionPatterns []UserInteractionPattern `json:"interaction_patterns,omitempty"`
	InteractionProfile  UserInteractionProfile   `json:"interaction_profile"`
	Confidence          float64                  `json:"confidence"`
	Sources             []UserModelSource        `json:"sources,omitempty"`
	UpdatedAt           time.Time                `json:"updated_at"`
}

// UserAttribute, UserPreference, UserGoal, UserCapability and
// UserKnowledgeItem are intentionally minimal contracts in this block. Their
// richer semantics are defined by subsequent B96.34 blocks.
type UserAttribute struct {
	Key        string              `json:"key"`
	Value      string              `json:"value"`
	Confidence float64             `json:"confidence"`
	Provenance UserModelSourceRef  `json:"provenance"`
	Status     UserModelItemStatus `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type UserPreference struct {
	Key        string              `json:"key"`
	Value      string              `json:"value"`
	Confidence float64             `json:"confidence"`
	Provenance UserModelSourceRef  `json:"provenance"`
	Status     UserModelItemStatus `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type UserGoal struct {
	Key        string              `json:"key"`
	Value      string              `json:"value"`
	Confidence float64             `json:"confidence"`
	Provenance UserModelSourceRef  `json:"provenance"`
	Status     UserModelItemStatus `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type UserCapability struct {
	Key        string              `json:"key"`
	Value      string              `json:"value"`
	Confidence float64             `json:"confidence"`
	Provenance UserModelSourceRef  `json:"provenance"`
	Status     UserModelItemStatus `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type UserKnowledgeItem struct {
	Key        string              `json:"key"`
	Value      string              `json:"value"`
	Confidence float64             `json:"confidence"`
	Provenance UserModelSourceRef  `json:"provenance"`
	Status     UserModelItemStatus `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// UserInteractionPattern is the minimal B96.34.1-style contract for a
// probabilistic behavioral pattern. Unlike the other minimal domain items,
// a pattern is not evaluated once: it accumulates EvidenceCount across many
// observations, and Confidence is expected to move as a running estimate
// rather than settle from a single proposal. B96.34.8 owns the richer
// semantics and the accumulation logic; this is only the durable projection.
type UserInteractionPattern struct {
	Pattern       string              `json:"pattern"`
	Confidence    float64             `json:"confidence"`
	EvidenceCount int                 `json:"evidence_count"`
	Provenance    UserModelSourceRef  `json:"provenance"`
	Status        UserModelItemStatus `json:"status"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// UserInteractionProfile is a stable interaction contract. It starts empty;
// later blocks decide which dimensions are learned and how they are inferred.
type UserInteractionProfile struct {
	Values map[string]UserInteractionProfileValue `json:"values,omitempty"`
}

type UserInteractionProfileValue struct {
	Value      string              `json:"value"`
	Confidence float64             `json:"confidence"`
	Provenance UserModelSourceRef  `json:"provenance"`
	Status     UserModelItemStatus `json:"status"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// UserModelSourceKind identifies how a model item entered the system.
type UserModelSourceKind string

const (
	UserSourceExplicitUser UserModelSourceKind = "explicit_user"
	UserSourceObserved     UserModelSourceKind = "observed"
	UserSourceInferred     UserModelSourceKind = "inferred"
	UserSourceSystem       UserModelSourceKind = "system"
	UserSourceImported     UserModelSourceKind = "imported"
)

// UserModelSourceRef is attached to individual model items so provenance is
// not lost when the model is queried or copied.
type UserModelSourceRef struct {
	Kind       UserModelSourceKind `json:"kind"`
	SourceID   string              `json:"source_id,omitempty"`
	ActorID    string              `json:"actor_id,omitempty"`
	EvidenceID string              `json:"evidence_id,omitempty"`
	ObservedAt time.Time           `json:"observed_at,omitempty"`
}

// UserModelSource records provenance at model level. It can point to a turn,
// memory entry, import, or system event without forcing a specific storage
// implementation in B96.34.1.
type UserModelSource struct {
	ID          string              `json:"id"`
	Kind        UserModelSourceKind `json:"kind"`
	ActorID     string              `json:"actor_id,omitempty"`
	Description string              `json:"description,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
}

type UserModelItemStatus string

const (
	UserModelStatusActive     UserModelItemStatus = "active"
	UserModelStatusUncertain  UserModelItemStatus = "uncertain"
	UserModelStatusSuperseded UserModelItemStatus = "superseded"
	UserModelStatusRejected   UserModelItemStatus = "rejected"
)

// UserModelActorKind describes which architectural role is requesting access.
type UserModelActorKind string

const (
	UserModelActorUser        UserModelActorKind = "user"
	UserModelActorInteraction UserModelActorKind = "interaction"
	UserModelActorCognition   UserModelActorKind = "cognition"
	UserModelActorMemory      UserModelActorKind = "memory"
	UserModelActorSystem      UserModelActorKind = "system"
	UserModelActorExternal    UserModelActorKind = "external"
)

type UserModelAccessMode string

const (
	UserModelAccessRead   UserModelAccessMode = "read"
	UserModelAccessModify UserModelAccessMode = "modify"
)

// UserModelAccessRequest makes the read/write boundary explicit. The model is
// never exposed as a mutable internal object to a caller through this API.
type UserModelAccessRequest struct {
	Actor UserModelActorKind
	Mode  UserModelAccessMode
}

// UserModelAccessPolicy is deliberately conservative. Cognition and
// interaction may read through scoped interfaces; only the dedicated model
// update boundary may modify persisted beliefs. External callers have no
// implicit access.
type UserModelAccessPolicy struct{}

func (UserModelAccessPolicy) Allow(req UserModelAccessRequest) bool {
	switch req.Mode {
	case UserModelAccessRead:
		return req.Actor == UserModelActorInteraction ||
			req.Actor == UserModelActorCognition ||
			req.Actor == UserModelActorMemory ||
			req.Actor == UserModelActorSystem
	case UserModelAccessModify:
		return req.Actor == UserModelActorMemory || req.Actor == UserModelActorSystem
	default:
		return false
	}
}

// UserModelView is a read-only value snapshot. It is safe to pass across
// subsystem boundaries because slices and maps are copied by the constructor.
type UserModelView struct {
	Model UserModel `json:"model"`
}

// NewUserModel creates an empty formal model. No user attributes, preferences,
// goals, capabilities, knowledge, or interaction traits are inferred here.
func NewUserModel(userID string) UserModel {
	return UserModel{
		UserID:              strings.TrimSpace(userID),
		Attributes:          []UserAttribute{},
		Preferences:         []UserPreference{},
		Goals:               []UserGoal{},
		Capabilities:        []UserCapability{},
		Knowledge:           []UserKnowledgeItem{},
		InteractionPatterns: []UserInteractionPattern{},
		InteractionProfile:  UserInteractionProfile{Values: map[string]UserInteractionProfileValue{}},
		Sources:             []UserModelSource{},
		Confidence:          0,
	}
}

// Validate checks structural invariants only. It does not decide whether a
// belief is true; provenance, confidence and status remain explicit.
func (m UserModel) Validate() error {
	if strings.TrimSpace(m.UserID) == "" {
		return errors.New("user model: user_id is required")
	}
	if !validConfidence(m.Confidence) {
		return errors.New("user model: confidence must be between 0 and 1")
	}
	if err := validateAttributes(m.Attributes); err != nil {
		return err
	}
	if err := validatePreferences(m.Preferences); err != nil {
		return err
	}
	if err := validateGoals(m.Goals); err != nil {
		return err
	}
	if err := validateCapabilities(m.Capabilities); err != nil {
		return err
	}
	if err := validateKnowledge(m.Knowledge); err != nil {
		return err
	}
	if err := validateInteractionPatterns(m.InteractionPatterns); err != nil {
		return err
	}
	for key, value := range m.InteractionProfile.Values {
		if strings.TrimSpace(key) == "" {
			return errors.New("user model: interaction profile contains an empty key")
		}
		if !validConfidence(value.Confidence) {
			return fmt.Errorf("user model: interaction profile %q has invalid confidence", key)
		}
	}
	for i, source := range m.Sources {
		if strings.TrimSpace(source.ID) == "" {
			return fmt.Errorf("user model: source %d has empty id", i)
		}
		if !validSourceKind(source.Kind) {
			return fmt.Errorf("user model: source %q has invalid kind", source.ID)
		}
	}
	return nil
}

func validateAttributes(items []UserAttribute) error {
	for i, item := range items {
		if err := validateModelItem("attribute", i, item.Key, item.Confidence, item.Status, item.Provenance); err != nil {
			return err
		}
	}
	return nil
}
func validatePreferences(items []UserPreference) error {
	for i, item := range items {
		if err := validateModelItem("preference", i, item.Key, item.Confidence, item.Status, item.Provenance); err != nil {
			return err
		}
	}
	return nil
}
func validateGoals(items []UserGoal) error {
	for i, item := range items {
		if err := validateModelItem("goal", i, item.Key, item.Confidence, item.Status, item.Provenance); err != nil {
			return err
		}
	}
	return nil
}
func validateCapabilities(items []UserCapability) error {
	for i, item := range items {
		if err := validateModelItem("capability", i, item.Key, item.Confidence, item.Status, item.Provenance); err != nil {
			return err
		}
	}
	return nil
}
func validateKnowledge(items []UserKnowledgeItem) error {
	for i, item := range items {
		if err := validateModelItem("knowledge", i, item.Key, item.Confidence, item.Status, item.Provenance); err != nil {
			return err
		}
	}
	return nil
}
func validateInteractionPatterns(items []UserInteractionPattern) error {
	for i, item := range items {
		if err := validateModelItem("interaction_pattern", i, item.Pattern, item.Confidence, item.Status, item.Provenance); err != nil {
			return err
		}
		if item.EvidenceCount < 1 {
			return fmt.Errorf("user model: interaction_pattern %d (%q) must have evidence_count >= 1", i, item.Pattern)
		}
	}
	return nil
}

func validateModelItem(kind string, index int, key string, confidence float64, status UserModelItemStatus, provenance UserModelSourceRef) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("user model: %s %d has empty key", kind, index)
	}
	if !validConfidence(confidence) {
		return fmt.Errorf("user model: %s %q has invalid confidence", kind, key)
	}
	if !validStatus(status) {
		return fmt.Errorf("user model: %s %q has invalid status", kind, key)
	}
	if !validSourceKind(provenance.Kind) {
		return fmt.Errorf("user model: %s %q has invalid provenance", kind, key)
	}
	return nil
}

func validConfidence(v float64) bool { return v >= 0 && v <= 1 }

func validSourceKind(kind UserModelSourceKind) bool {
	switch kind {
	case UserSourceExplicitUser, UserSourceObserved, UserSourceInferred, UserSourceSystem, UserSourceImported:
		return true
	default:
		return false
	}
}

func validStatus(status UserModelItemStatus) bool {
	switch status {
	case UserModelStatusActive, UserModelStatusUncertain, UserModelStatusSuperseded, UserModelStatusRejected:
		return true
	default:
		return false
	}
}

// Snapshot returns a detached read view. Mutation of the returned value cannot
// alter the original model.
func (m UserModel) Snapshot() UserModelView {
	out := m
	out.Attributes = append([]UserAttribute(nil), m.Attributes...)
	out.Preferences = append([]UserPreference(nil), m.Preferences...)
	out.Goals = append([]UserGoal(nil), m.Goals...)
	out.Capabilities = append([]UserCapability(nil), m.Capabilities...)
	out.Knowledge = append([]UserKnowledgeItem(nil), m.Knowledge...)
	out.InteractionPatterns = append([]UserInteractionPattern(nil), m.InteractionPatterns...)
	out.Sources = append([]UserModelSource(nil), m.Sources...)
	out.InteractionProfile.Values = make(map[string]UserInteractionProfileValue, len(m.InteractionProfile.Values))
	for k, v := range m.InteractionProfile.Values {
		out.InteractionProfile.Values[k] = v
	}
	return UserModelView{Model: out}
}

// UserModelStore is the ownership boundary for a live model. Callers do not
// receive the internal value; reads return a detached snapshot and writes are
// accepted only from an authorized architectural actor after validation.
type UserModelStore struct {
	model  UserModel
	policy UserModelAccessPolicy
}

const UserModelPath = "memory/user_model.json"

// LoadUserModelStore loads the durable B96.34 User Model. A missing file is
// treated as a first-run empty model; malformed or invalid persisted data is
// rejected rather than silently replaced.
func LoadUserModelStore(path, userID string) (*UserModelStore, error) {
	if strings.TrimSpace(path) == "" {
		path = UserModelPath
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewUserModelStore(NewUserModel(userID))
	}
	if err != nil {
		return nil, err
	}
	var model UserModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("user model: decode %s: %w", path, err)
	}
	if strings.TrimSpace(model.UserID) == "" {
		model.UserID = strings.TrimSpace(userID)
	}
	return NewUserModelStore(model)
}

// Save persists a detached read view of the model. The caller must be an
// authorized read actor; persistence itself never exposes the mutable model.
func (s *UserModelStore) Save(path string, actor UserModelActorKind) error {
	if strings.TrimSpace(path) == "" {
		path = UserModelPath
	}
	view, err := s.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(view.Model, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func NewUserModelStore(model UserModel) (*UserModelStore, error) {
	if err := model.Validate(); err != nil {
		return nil, err
	}
	return &UserModelStore{model: model, policy: UserModelAccessPolicy{}}, nil
}

func (s *UserModelStore) Read(req UserModelAccessRequest) (UserModelView, error) {
	if s == nil {
		return UserModelView{}, errors.New("user model store: nil store")
	}
	if req.Mode != UserModelAccessRead || !s.policy.Allow(req) {
		return UserModelView{}, errors.New("user model store: read access denied")
	}
	return s.model.Snapshot(), nil
}

func (s *UserModelStore) Replace(req UserModelAccessRequest, model UserModel) error {
	if s == nil {
		return errors.New("user model store: nil store")
	}
	if req.Mode != UserModelAccessModify || !s.policy.Allow(req) {
		return errors.New("user model store: modify access denied")
	}
	if err := model.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(model.UserID) != strings.TrimSpace(s.model.UserID) {
		return errors.New("user model store: user_id cannot change through replace")
	}
	s.model = model
	return nil
}
