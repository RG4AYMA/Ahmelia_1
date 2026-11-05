package interaction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// B96.34.15 — User Model E2E.
//
// Finalmente:
//
//	User
//	 ↓
//	Conversation
//	 ↓
//	B96.32 State
//	 ↓
//	B96.33 Cognition
//	 ↓
//	Relevant User Signal
//	 ↓
//	B96.34 User Model
//	 ↓
//	Validation
//	 ↓
//	Update / Reject
//	 ↓
//	Future Conversation
//
// This block closes the B96.34 chain with one deterministic orchestration
// entry point, the same role B96.33.12 plays for conversational cognition.
// It does not introduce a new belief domain, does not decide any domain's
// own admission policy (B96.34.2..34.8 keep that — a UserModelUpdateSignal
// arrives here only after the proposing domain's own Evaluate*Update has
// already accepted it), and does not infer privacy category or conflict
// context (both are caller-supplied, exactly as B96.34.13 and B96.34.14
// already require).
//
// The wiring is:
//
//  1. B96.34.14 (privacy boundary) — is this candidate the kind of thing
//     the User Model is allowed to hold, coming from where it came from.
//  2. B96.34.11 (update policy) — reconcile the accepted candidate against
//     whatever this domain already durably holds at the same key.
//  3. B96.34.13 (conflict resolution) — only when step 2 held a
//     contradiction rather than resolving it outright, ask whether the
//     disagreement is genuine or a context-dependent split.
//  4. B96.34.1 (core) — validate and persist through UserModelStore, or
//     reject and leave the model completely untouched.
//
// A rejection at any step never partially applies: UserModelStore.Replace
// is only ever called once, after every earlier step has already agreed to
// proceed, and every rejection path returns before reaching it.

// UserModelDomain names which B96.34.x typed collection a signal targets.
// It exists only so this block can dispatch to the right slice/map inside
// UserModel — it is not a new belief domain, and adding one here without a
// corresponding B96.34.x domain block would be exactly the kind of silent
// scope change B96.34.14's AllowedUserModelCategories() already warns
// against for categories.
type UserModelDomain string

const (
	UserModelDomainAttribute            UserModelDomain = "attribute"
	UserModelDomainPreference           UserModelDomain = "preference"
	UserModelDomainGoal                 UserModelDomain = "goal"
	UserModelDomainCapability           UserModelDomain = "capability"
	UserModelDomainKnowledge            UserModelDomain = "knowledge"
	UserModelDomainInteractionPattern   UserModelDomain = "interaction_pattern"
	UserModelDomainCommunicationProfile UserModelDomain = "communication_profile"
)

func isKnownUserModelDomain(d UserModelDomain) bool {
	switch d {
	case UserModelDomainAttribute, UserModelDomainPreference, UserModelDomainGoal,
		UserModelDomainCapability, UserModelDomainKnowledge,
		UserModelDomainInteractionPattern, UserModelDomainCommunicationProfile:
		return true
	default:
		return false
	}
}

// UserModelUpdateSignal is the "Relevant User Signal" the E2E diagram hands
// down from B96.33. Item is expected to already be an *accepted* candidate
// — produced by the domain's own Evaluate*Update plus its *UpdateItem
// adapter (B96.34.11) — not a raw utterance. Category and Context are the
// same caller-supplied values B96.34.14 and B96.34.13 already require;
// this block does not derive either from Item's own text.
//
// Context is only ever known for *this* proposal, in *this* turn. A belief
// already stored before this call has no persisted Context of its own — see
// the Known limitation note on RunUserModelUpdateE2E.
type UserModelUpdateSignal struct {
	Domain   UserModelDomain
	Category UserModelDataCategory
	Item     UserModelUpdateItem
	Context  string
}

// UserModelUpdateE2EOutcome documents every intermediate decision, for the
// same B96.34.10 audit trail every other B96.34 outcome type already
// promises. Exactly one of Applied / Rejected is true.
type UserModelUpdateE2EOutcome struct {
	Version        string
	Domain         UserModelDomain
	Privacy        UserModelPrivacyDecision
	Reconciliation UserModelUpdateOutcome
	// Conflict is non-nil only when Reconciliation.Action was
	// UserModelUpdateHoldForReconciliation and B96.34.13 ran.
	Conflict *UserModelConflictOutcome
	Applied  bool
	Rejected bool
	Reason   string
	// Model is the resulting read-only snapshot: the post-update view when
	// Applied, the unchanged pre-update view when Rejected.
	Model  UserModelView
	Valid  bool
	Errors []string
}

// RunUserModelUpdateE2E is the canonical B96.34 orchestration entry point.
// actor must be authorized for both read and modify (UserModelActorMemory
// or UserModelActorSystem, per B96.34.1's UserModelAccessPolicy) since a
// successful run does both.
//
// Known limitation: when step 3 resolves a held contradiction, only the
// incoming proposal's Context is known — a belief already durably stored
// before this call carries no persisted Context, so it is treated as
// unscoped ("") rather than reconstructed. A future block would need to
// persist Context alongside stored beliefs to change this; B96.34.15 does
// not invent one.
func RunUserModelUpdateE2E(
	store *UserModelStore,
	actor UserModelActorKind,
	signal UserModelUpdateSignal,
	privacyPolicy UserModelPrivacyPolicy,
	updatePolicy UserModelUpdatePolicy,
	conflictPolicy UserModelConflictPolicy,
	now time.Time,
) (UserModelUpdateE2EOutcome, error) {
	if store == nil {
		return UserModelUpdateE2EOutcome{}, errors.New("user model e2e: nil store")
	}
	if err := ValidateUserModelUpdateItem(signal.Item); err != nil {
		return UserModelUpdateE2EOutcome{}, err
	}
	if !isKnownUserModelDomain(signal.Domain) {
		return UserModelUpdateE2EOutcome{}, fmt.Errorf("user model e2e: unknown domain %q", signal.Domain)
	}

	readReq := UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead}
	before, err := store.Read(readReq)
	if err != nil {
		return UserModelUpdateE2EOutcome{}, err
	}

	// Step 1 (B96.34.14): is this candidate the kind of thing the User
	// Model is allowed to hold, coming from where it came from.
	decision, err := EvaluateUserModelPrivacyBoundary(signal.Item, signal.Category, privacyPolicy)
	if err != nil {
		return UserModelUpdateE2EOutcome{}, err
	}
	if !decision.Allowed {
		return rejectedUserModelUpdateE2E(signal.Domain, before, decision, UserModelUpdateOutcome{}, nil,
			"rejected by privacy boundary: "+string(decision.Reason)), nil
	}

	proposal := signal.Item
	if decision.DowngradeToUncertain && proposal.Status == UserModelStatusActive {
		proposal.Status = UserModelStatusUncertain
	}

	// Step 2 (B96.34.11): reconcile against whatever this domain already
	// durably holds at this key.
	existing, err := extractUserModelDomainItems(before.Model, signal.Domain)
	if err != nil {
		return UserModelUpdateE2EOutcome{}, err
	}
	updated, reconcileOutcome, err := ReconcileUserModelUpdate(existing, proposal, updatePolicy)
	if err != nil {
		return rejectedUserModelUpdateE2E(signal.Domain, before, decision, UserModelUpdateOutcome{}, nil,
			"rejected by update policy: "+err.Error()), nil
	}

	// Step 3 (B96.34.13): a held contradiction is not yet known to be a
	// genuine contradiction or a context-dependent split — ask, rather than
	// leaving two disagreeing live beliefs unexamined.
	var conflictOutcome *UserModelConflictOutcome
	if reconcileOutcome.Action == UserModelUpdateHoldForReconciliation {
		// ReconcileUserModelUpdate always appends the held proposal as the
		// last element, so its position is known without re-matching it by
		// value.
		proposalIdx := len(updated) - 1
		var liveIdxs []int
		for i, it := range updated {
			if it.Key == proposal.Key && isLiveUserFactStatus(it.Status) {
				liveIdxs = append(liveIdxs, i)
			}
		}
		if len(liveIdxs) >= 2 {
			candidates := make([]UserModelConflictItem, len(liveIdxs))
			for i, idx := range liveIdxs {
				ctx := ""
				if idx == proposalIdx {
					ctx = signal.Context
				}
				candidates[i] = UserModelConflictItem{UserModelUpdateItem: updated[idx], Context: ctx}
			}
			resolved, cErr := ResolveUserModelConflict(candidates, conflictPolicy, now)
			if cErr != nil {
				return rejectedUserModelUpdateE2E(signal.Domain, before, decision, reconcileOutcome, nil,
					"rejected by conflict resolution: "+cErr.Error()), nil
			}
			conflictOutcome = &resolved

			byIdentity := make(map[string]UserModelUpdateItem, len(resolved.Resolved)+len(resolved.Superseded))
			for _, r := range resolved.Resolved {
				byIdentity[userModelUpdateItemIdentity(r.UserModelUpdateItem)] = r.UserModelUpdateItem
			}
			for _, s := range resolved.Superseded {
				byIdentity[userModelUpdateItemIdentity(s.UserModelUpdateItem)] = s.UserModelUpdateItem
			}
			for _, idx := range liveIdxs {
				if repl, ok := byIdentity[userModelUpdateItemIdentity(updated[idx])]; ok {
					updated[idx] = repl
				}
			}
		}
	}

	// Step 4 (B96.34.1): project the reconciled canonical items back into
	// the model's typed storage and persist. Nothing above this line has
	// touched UserModel — every rejection so far has left before.Model
	// exactly as it was read.
	after, err := applyUserModelDomainItems(before.Model, signal.Domain, updated)
	if err != nil {
		return UserModelUpdateE2EOutcome{}, err
	}
	after.UpdatedAt = now

	if err := after.Validate(); err != nil {
		return rejectedUserModelUpdateE2E(signal.Domain, before, decision, reconcileOutcome, conflictOutcome,
			"rejected by model validation: "+err.Error()), nil
	}
	if err := store.Replace(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessModify}, after); err != nil {
		return rejectedUserModelUpdateE2E(signal.Domain, before, decision, reconcileOutcome, conflictOutcome,
			"rejected by store: "+err.Error()), nil
	}

	finalView, err := store.Read(readReq)
	if err != nil {
		return UserModelUpdateE2EOutcome{}, err
	}

	out := UserModelUpdateE2EOutcome{
		Version: "B96.34.15", Domain: signal.Domain, Privacy: decision,
		Reconciliation: reconcileOutcome, Conflict: conflictOutcome,
		Applied: true, Model: finalView,
	}
	out.Valid, out.Errors = validateUserModelUpdateE2E(out)
	return out, nil
}

func rejectedUserModelUpdateE2E(domain UserModelDomain, before UserModelView, privacy UserModelPrivacyDecision, reconciliation UserModelUpdateOutcome, conflict *UserModelConflictOutcome, reason string) UserModelUpdateE2EOutcome {
	out := UserModelUpdateE2EOutcome{
		Version: "B96.34.15", Domain: domain, Privacy: privacy, Reconciliation: reconciliation,
		Conflict: conflict, Rejected: true, Reason: reason, Model: before,
	}
	out.Valid, out.Errors = validateUserModelUpdateE2E(out)
	return out
}

func validateUserModelUpdateE2E(e UserModelUpdateE2EOutcome) (bool, []string) {
	errs := make([]string, 0, 4)
	if e.Version != "B96.34.15" {
		errs = append(errs, "version mismatch")
	}
	if e.Applied == e.Rejected {
		errs = append(errs, "outcome must be exactly one of applied or rejected")
	}
	if e.Applied && !e.Privacy.Allowed {
		errs = append(errs, "applied an update the privacy boundary did not allow")
	}
	if e.Conflict != nil && e.Reconciliation.Action != UserModelUpdateHoldForReconciliation {
		errs = append(errs, "conflict resolution ran without a held contradiction")
	}
	if e.Applied && e.Reconciliation.Action == UserModelUpdateSupersede && e.Reconciliation.Superseded == nil {
		errs = append(errs, "supersede action left no audit trail of what was superseded")
	}
	return len(errs) == 0, errs
}

// userModelUpdateItemIdentity gives a stable-enough key to relocate one
// UserModelConflictItem's origin inside the domain slice after B96.34.13
// has rewritten its Status/UpdatedAt. Value, full Provenance and CreatedAt
// are never mutated by ResolveUserModelConflict, so their combination is
// enough to identify which pre-resolution item a resolved/superseded result
// corresponds to.
func userModelUpdateItemIdentity(it UserModelUpdateItem) string {
	return strings.Join([]string{
		it.Value,
		string(it.Provenance.Kind),
		it.Provenance.SourceID,
		it.Provenance.ActorID,
		it.Provenance.EvidenceID,
		it.CreatedAt.String(),
	}, "|")
}

// extractUserModelDomainItems projects one domain's typed, persisted
// collection into the canonical B96.34.11 shape so ReconcileUserModelUpdate
// can compare the incoming proposal against it. EvidenceCount is not
// carried by any core domain type except UserInteractionPattern, so it
// defaults to zero here — reinforceUserModelUpdateItem already floors any
// weight below 1, so this does not distort reinforcement math.
func extractUserModelDomainItems(model UserModel, domain UserModelDomain) ([]UserModelUpdateItem, error) {
	switch domain {
	case UserModelDomainAttribute:
		out := make([]UserModelUpdateItem, len(model.Attributes))
		for i, a := range model.Attributes {
			out[i] = attributeToUserModelUpdateItem(a)
		}
		return out, nil
	case UserModelDomainPreference:
		out := make([]UserModelUpdateItem, len(model.Preferences))
		for i, p := range model.Preferences {
			out[i] = preferenceToUserModelUpdateItem(p)
		}
		return out, nil
	case UserModelDomainGoal:
		out := make([]UserModelUpdateItem, len(model.Goals))
		for i, g := range model.Goals {
			out[i] = goalToUserModelUpdateItem(g)
		}
		return out, nil
	case UserModelDomainCapability:
		out := make([]UserModelUpdateItem, len(model.Capabilities))
		for i, c := range model.Capabilities {
			out[i] = capabilityToUserModelUpdateItem(c)
		}
		return out, nil
	case UserModelDomainKnowledge:
		out := make([]UserModelUpdateItem, len(model.Knowledge))
		for i, k := range model.Knowledge {
			out[i] = knowledgeToUserModelUpdateItem(k)
		}
		return out, nil
	case UserModelDomainInteractionPattern:
		out := make([]UserModelUpdateItem, len(model.InteractionPatterns))
		for i, p := range model.InteractionPatterns {
			out[i] = patternToUserModelUpdateItem(p)
		}
		return out, nil
	case UserModelDomainCommunicationProfile:
		keys := make([]string, 0, len(model.InteractionProfile.Values))
		for k := range model.InteractionProfile.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic order; map iteration is not.
		out := make([]UserModelUpdateItem, 0, len(keys))
		for _, k := range keys {
			out = append(out, communicationValueToUserModelUpdateItem(k, model.InteractionProfile.Values[k]))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("user model e2e: unknown domain %q", domain)
	}
}

// applyUserModelDomainItems is extractUserModelDomainItems's inverse: it
// projects a reconciled canonical slice back into model's typed storage. It
// returns a modified copy; model itself (and therefore the caller's
// UserModelView) is never mutated in place.
func applyUserModelDomainItems(model UserModel, domain UserModelDomain, items []UserModelUpdateItem) (UserModel, error) {
	switch domain {
	case UserModelDomainAttribute:
		out := make([]UserAttribute, len(items))
		for i, it := range items {
			out[i] = userModelUpdateItemToAttribute(it)
		}
		model.Attributes = out
	case UserModelDomainPreference:
		out := make([]UserPreference, len(items))
		for i, it := range items {
			out[i] = userModelUpdateItemToPreference(it)
		}
		model.Preferences = out
	case UserModelDomainGoal:
		out := make([]UserGoal, len(items))
		for i, it := range items {
			out[i] = userModelUpdateItemToGoal(it)
		}
		model.Goals = out
	case UserModelDomainCapability:
		out := make([]UserCapability, len(items))
		for i, it := range items {
			out[i] = userModelUpdateItemToCapability(it)
		}
		model.Capabilities = out
	case UserModelDomainKnowledge:
		out := make([]UserKnowledgeItem, len(items))
		for i, it := range items {
			out[i] = userModelUpdateItemToKnowledge(it)
		}
		model.Knowledge = out
	case UserModelDomainInteractionPattern:
		out := make([]UserInteractionPattern, len(items))
		for i, it := range items {
			out[i] = userModelUpdateItemToPattern(it)
		}
		model.InteractionPatterns = out
	case UserModelDomainCommunicationProfile:
		values := make(map[string]UserInteractionProfileValue, len(items))
		for _, it := range items {
			values[it.Key] = userModelUpdateItemToCommunicationValue(it)
		}
		model.InteractionProfile.Values = values
	default:
		return UserModel{}, fmt.Errorf("user model e2e: unknown domain %q", domain)
	}
	return model, nil
}

func attributeToUserModelUpdateItem(a UserAttribute) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: a.Key, Value: a.Value, Confidence: a.Confidence,
		Provenance: a.Provenance, Status: a.Status, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}
func userModelUpdateItemToAttribute(i UserModelUpdateItem) UserAttribute {
	return UserAttribute{
		Key: i.Key, Value: i.Value, Confidence: i.Confidence,
		Provenance: i.Provenance, Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

func preferenceToUserModelUpdateItem(p UserPreference) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: p.Key, Value: p.Value, Confidence: p.Confidence,
		Provenance: p.Provenance, Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
func userModelUpdateItemToPreference(i UserModelUpdateItem) UserPreference {
	return UserPreference{
		Key: i.Key, Value: i.Value, Confidence: i.Confidence,
		Provenance: i.Provenance, Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

func goalToUserModelUpdateItem(g UserGoal) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: g.Key, Value: g.Value, Confidence: g.Confidence,
		Provenance: g.Provenance, Status: g.Status, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
	}
}
func userModelUpdateItemToGoal(i UserModelUpdateItem) UserGoal {
	return UserGoal{
		Key: i.Key, Value: i.Value, Confidence: i.Confidence,
		Provenance: i.Provenance, Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

func capabilityToUserModelUpdateItem(c UserCapability) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: c.Key, Value: c.Value, Confidence: c.Confidence,
		Provenance: c.Provenance, Status: c.Status, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
func userModelUpdateItemToCapability(i UserModelUpdateItem) UserCapability {
	return UserCapability{
		Key: i.Key, Value: i.Value, Confidence: i.Confidence,
		Provenance: i.Provenance, Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

func knowledgeToUserModelUpdateItem(k UserKnowledgeItem) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: k.Key, Value: k.Value, Confidence: k.Confidence,
		Provenance: k.Provenance, Status: k.Status, CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt,
	}
}
func userModelUpdateItemToKnowledge(i UserModelUpdateItem) UserKnowledgeItem {
	return UserKnowledgeItem{
		Key: i.Key, Value: i.Value, Confidence: i.Confidence,
		Provenance: i.Provenance, Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

// patternToUserModelUpdateItem/userModelUpdateItemToPattern are the only
// pair that round-trips EvidenceCount faithfully, since UserInteractionPattern
// is the only core type that stores it.
func patternToUserModelUpdateItem(p UserInteractionPattern) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: p.Pattern, Value: p.Pattern, Confidence: p.Confidence, EvidenceCount: p.EvidenceCount,
		Provenance: p.Provenance, Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
func userModelUpdateItemToPattern(i UserModelUpdateItem) UserInteractionPattern {
	evidence := i.EvidenceCount
	if evidence < 1 {
		evidence = 1
	}
	return UserInteractionPattern{
		Pattern: i.Key, Confidence: i.Confidence, EvidenceCount: evidence,
		Provenance: i.Provenance, Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

// communicationValueToUserModelUpdateItem/userModelUpdateItemToCommunicationValue
// bridge the B96.34.4/B96.34.12 map-keyed shape (UserInteractionProfile.Values,
// no CreatedAt field) into the canonical slice shape and back.
func communicationValueToUserModelUpdateItem(key string, v UserInteractionProfileValue) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: key, Value: v.Value, Confidence: v.Confidence,
		Provenance: v.Provenance, Status: v.Status, UpdatedAt: v.UpdatedAt,
	}
}
func userModelUpdateItemToCommunicationValue(i UserModelUpdateItem) UserInteractionProfileValue {
	return UserInteractionProfileValue{
		Value: i.Value, Confidence: i.Confidence, Provenance: i.Provenance, Status: i.Status, UpdatedAt: i.UpdatedAt,
	}
}
