package interaction

import (
	"errors"
	"strings"
	"time"
)

// B96.34.12 — User Model Query.
//
// Ahora otros sistemas necesitan consultarlo. Pero no quiero:
//
//	GetEverythingAboutUser()
//
// Eso sería una mala arquitectura. Quiero consultas específicas:
//
//	GetRelevantPreferences(context)
//	GetCommunicationProfile(context)
//	GetRelevantGoals(context)
//	GetCapabilityLevel(domain)
//	GetRelevantUserKnowledge(topic)
//
// Es decir: el User Model no se vuelca entero sobre cada conversación.
// B96.33 decide qué información es relevante.
//
// Concretely: this block does not rank, score, or guess relevance itself —
// that judgment already happened upstream, in B96.33's cognition pipeline.
// What B96.33 hands down here is UserModelQueryContext.Topics, the set of
// keys/topics it has already decided matter for the current turn. This
// block's only job is to turn that decision into a narrow, read-only,
// access-checked slice of UserModel — through UserModelStore.Read
// (B96.34.1), so the read/modify boundary still applies to every query
// below — and to never silently resolve an unresolved contradiction left by
// B96.34.11 into a single confident answer. Where B96.34.11 left two live,
// disagreeing beliefs at the same key, that ambiguity is real information:
// this block surfaces it rather than picking one, consistent with B96.34.14
// — Ahmelia must be able to represent "no sé" as much as "sé".
//
// There is deliberately no GetEverythingAboutUser() here, and there never
// will be in this file.

// UserModelQueryContext carries whatever B96.33 has already decided is
// relevant for the current turn. It is not a search query this block
// evaluates for relevance — it is the outcome of that evaluation, done
// elsewhere.
type UserModelQueryContext struct {
	// Topics is the set of keys/topics/dimensions B96.33 has already
	// identified as relevant. Required and non-empty for
	// GetRelevantPreferences and GetRelevantGoals — those collections can
	// grow unbounded, so an empty context there is refused rather than
	// silently treated as "return everything". Optional for
	// GetCommunicationProfile, whose backing set is the fixed, small
	// B96.34.4 dimension list (formality, directness, ...): with no
	// topics given, the whole (still status-filtered) profile is returned,
	// since returning eight bounded dimensions is not the
	// GetEverythingAboutUser() problem this block exists to prevent.
	Topics []string
	// IncludeUncertain allows Uncertain-status beliefs into the result
	// alongside Active ones. Default false: a caller asking a narrow,
	// specific question should get confident beliefs unless it explicitly
	// says it can handle (and wants to see) unresolved ones.
	IncludeUncertain bool
}

func (c UserModelQueryContext) matches(key string) bool {
	if len(c.Topics) == 0 {
		return false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, topic := range c.Topics {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic == "" {
			continue
		}
		if key == topic || strings.Contains(key, topic) || strings.Contains(topic, key) {
			return true
		}
	}
	return false
}

func isQueryableStatus(status UserModelItemStatus, includeUncertain bool) bool {
	switch status {
	case UserModelStatusActive:
		return true
	case UserModelStatusUncertain:
		return includeUncertain
	default:
		return false
	}
}

// CommunicationProfileValueToInteractionProfile projects one accepted
// B96.34.4 communication-profile value into the B96.34.1 core storage slot,
// UserModel.InteractionProfile.Values, keyed by dimension name. No earlier
// block wired this bridge: B96.34.4 defined its own richer container
// (UserCommunicationProfile) without ever targeting a UserModel field, and
// UserModel.InteractionProfile has existed since B96.34.1 with nothing
// writing to it. Without this, GetCommunicationProfile below would have
// nothing durable to read. This is a one-way projection, like every other
// domain's *ToCore adapter — it does not decide admission, B96.34.4 already
// did.
// UserProfileCompatibilityView projects the canonical B96.34 UserModel into
// the legacy UserProfile shape for consumers that have not yet migrated their
// signatures. It is deliberately read-only with respect to the store: the
// returned profile is detached and must never be persisted back as a source
// of current beliefs. PrimaryAuthority is supplied by the compatibility
// caller because authority/configuration remains outside B96.34 belief data.
func UserProfileCompatibilityView(store *UserModelStore, authority AuthorityIdentity) (*UserProfile, error) {
	if store == nil {
		return nil, errors.New("user model compatibility: nil store")
	}
	view, err := store.Read(UserModelAccessRequest{Actor: UserModelActorCognition, Mode: UserModelAccessRead})
	if err != nil {
		return nil, err
	}
	p := NewUserProfile()
	p.ensureMaps()
	p.PrimaryAuthority = authority
	toFact := func(value string, confidence float64, provenance UserModelSourceRef, status UserModelItemStatus, updatedAt time.Time) ProfileFact {
		return ProfileFact{Value: value, Confidence: confidence, Evidence: 1, Source: provenance.SourceID, Status: string(status), LastSeen: updatedAt, FirstSeen: updatedAt}
	}
	for _, item := range view.Model.Preferences {
		if isQueryableStatus(item.Status, false) {
			p.Preferences[strings.ToLower(strings.TrimSpace(item.Key))] = toFact(item.Value, item.Confidence, item.Provenance, item.Status, item.UpdatedAt)
		}
	}
	for _, item := range view.Model.Goals {
		if isQueryableStatus(item.Status, false) {
			p.Goals[strings.ToLower(strings.TrimSpace(item.Key))] = toFact(item.Value, item.Confidence, item.Provenance, item.Status, item.UpdatedAt)
		}
	}
	for key, item := range view.Model.InteractionProfile.Values {
		if !isQueryableStatus(item.Status, false) {
			continue
		}
		p.Communication[strings.ToLower(strings.TrimSpace(key))] = toFact(item.Value, item.Confidence, item.Provenance, item.Status, item.UpdatedAt)
	}
	return p, nil
}

func CommunicationProfileValueToInteractionProfile(r UserCommunicationProfileValue) UserInteractionProfileValue {
	return UserInteractionProfileValue{
		Value:      r.Value,
		Confidence: r.Confidence,
		Provenance: r.Source,
		Status:     r.Status,
		UpdatedAt:  r.UpdatedAt,
	}
}

func GetRelevantAttributes(store *UserModelStore, actor UserModelActorKind, ctx UserModelQueryContext) ([]UserAttribute, error) {
	if len(ctx.Topics) == 0 {
		return nil, errors.New("user model query: GetRelevantAttributes requires at least one topic")
	}
	view, err := store.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return nil, err
	}
	out := make([]UserAttribute, 0)
	for _, attribute := range view.Model.Attributes {
		if !isQueryableStatus(attribute.Status, ctx.IncludeUncertain) {
			continue
		}
		if ctx.matches(attribute.Key) {
			out = append(out, attribute)
		}
	}
	return out, nil
}

// GetRelevantPreferences returns the live preferences matching context's
// topics. context.Topics must be non-empty: preferences are an unbounded,
// growing collection, and an empty-topics "give me the relevant ones" would
// quietly become GetEverythingAboutUser(). If B96.34.11 left more than one
// live, disagreeing preference at the same key, all of them are returned —
// picking a winner here would hide a contradiction B96.34.13 has not yet
// resolved.
func GetRelevantPreferences(store *UserModelStore, actor UserModelActorKind, ctx UserModelQueryContext) ([]UserPreference, error) {
	if len(ctx.Topics) == 0 {
		return nil, errors.New("user model query: GetRelevantPreferences requires at least one topic")
	}
	view, err := store.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return nil, err
	}
	var out []UserPreference
	for _, p := range view.Model.Preferences {
		if !isQueryableStatus(p.Status, ctx.IncludeUncertain) {
			continue
		}
		if ctx.matches(p.Key) {
			out = append(out, p)
		}
	}
	return out, nil
}

// GetRelevantGoals returns the live goals matching context's topics. Same
// non-empty-topics requirement and same no-silent-winner behavior as
// GetRelevantPreferences, for the same reasons.
func GetRelevantGoals(store *UserModelStore, actor UserModelActorKind, ctx UserModelQueryContext) ([]UserGoal, error) {
	if len(ctx.Topics) == 0 {
		return nil, errors.New("user model query: GetRelevantGoals requires at least one topic")
	}
	view, err := store.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return nil, err
	}
	var out []UserGoal
	for _, g := range view.Model.Goals {
		if !isQueryableStatus(g.Status, ctx.IncludeUncertain) {
			continue
		}
		if ctx.matches(g.Key) {
			out = append(out, g)
		}
	}
	return out, nil
}

// GetRelevantUserKnowledge returns the live knowledge items matching one
// topic. Unlike preferences/goals, B96.34.5 keys knowledge one record per
// topic, so this takes a single topic rather than a context set — matching
// the spec's own GetRelevantUserKnowledge(topic) shape, not
// GetRelevantX(context).
func GetRelevantUserKnowledge(store *UserModelStore, actor UserModelActorKind, topic string) ([]UserKnowledgeItem, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, errors.New("user model query: topic is required")
	}
	view, err := store.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return nil, err
	}
	ctx := UserModelQueryContext{Topics: []string{topic}}
	var out []UserKnowledgeItem
	for _, k := range view.Model.Knowledge {
		if !isQueryableStatus(k.Status, false) {
			continue
		}
		if ctx.matches(k.Key) {
			out = append(out, k)
		}
	}
	return out, nil
}

// GetCapabilityLevel answers one specific, single-domain question, not a
// browsable set — matching the spec's GetCapabilityLevel(domain) shape. It
// only considers Active beliefs: a capability judgment shapes a concrete
// behavioral choice ("¿Quieres que te explique cómo hacerlo o directamente
// preparo la implementación?"), so it should not rest on an unconfirmed
// guess. found reports whether any Active capability exists for domain;
// contested reports whether B96.34.11 left more than one live (Active or
// Uncertain) belief at that key with disagreeing values — in which case the
// single Level returned is the most recently updated one, but the caller
// should treat it as unsettled rather than confident, and this is exactly
// the case B96.34.13 exists to resolve.
func GetCapabilityLevel(store *UserModelStore, actor UserModelActorKind, domain string) (level UserCapability, found bool, contested bool, err error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return UserCapability{}, false, false, errors.New("user model query: domain is required")
	}
	view, err := store.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return UserCapability{}, false, false, err
	}

	var liveValues []string
	for _, c := range view.Model.Capabilities {
		if !strings.EqualFold(strings.TrimSpace(c.Key), domain) {
			continue
		}
		if isQueryableStatus(c.Status, true) {
			liveValues = appendUnique(liveValues, c.Value)
		}
		if c.Status == UserModelStatusActive && (!found || c.UpdatedAt.After(level.UpdatedAt)) {
			level = c
			found = true
		}
	}
	contested = len(liveValues) > 1
	return level, found, contested, nil
}

// GetCommunicationProfile returns the live dimensions of the B96.34.4
// communication profile, filtered to context.Topics when given. Unlike
// GetRelevantPreferences/GetRelevantGoals, an empty context is allowed: the
// backing set is the fixed, small B96.34.4 dimension list, not an unbounded
// collection, so returning all of it is not the GetEverythingAboutUser()
// problem this block guards against.
func GetCommunicationProfile(store *UserModelStore, actor UserModelActorKind, ctx UserModelQueryContext) (map[string]UserInteractionProfileValue, error) {
	view, err := store.Read(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead})
	if err != nil {
		return nil, err
	}
	out := make(map[string]UserInteractionProfileValue)
	for key, value := range view.Model.InteractionProfile.Values {
		if !isQueryableStatus(value.Status, ctx.IncludeUncertain) {
			continue
		}
		if len(ctx.Topics) > 0 && !ctx.matches(key) {
			continue
		}
		out[key] = value
	}
	return out, nil
}

// UserModelQuery describes one narrowly-scoped request for durable user
// information. B96.33 decides whether the turn is actually a request for
// information; this helper only maps an already-question-shaped request to a
// supported User Model key.
type UserModelQuery struct {
	Domain string
	Topics []string
}

func DetectUserModelQuery(req SemanticRequest) (UserModelQuery, bool) {
	acts := CanonicalActs(req)
	if req.Subject != UserObservationSubjectUser {
		return UserModelQuery{}, false
	}
	if !acts.Has(ActQuestion) && !acts.Has(ActOpinionRequest) {
		return UserModelQuery{}, false
	}

	// Domain-specific B96.34 queries are resolved before the generic scalar
	// attribute bridge. These domains (goal, knowledge, capability,
	// communication profile) have their own canonical storage contracts; the
	// selector is based on the semantic domain/topic, never on a list of
	// individual attributes.
	n := normalizeForClassification(req.Normalized)
	switch {
	case strings.Contains(n, "objetivo") || strings.Contains(n, "objetivos") || strings.Contains(n, "goal") || strings.Contains(n, "goals"):
		topic := extractQuestionTopic(n, []string{"sobre", "con", "para"})
		if topic == "" && strings.TrimSpace(req.Topic) != "" {
			topic = strings.TrimSpace(req.Topic)
		}
		if topic == "" {
			return UserModelQuery{}, false
		}
		return UserModelQuery{Domain: string(UserModelDomainGoal), Topics: []string{topic}}, true
	case strings.Contains(n, "qué sé sobre"), strings.Contains(n, "que se sobre"), strings.Contains(n, "que sé de"), strings.Contains(n, "que se de"),
		strings.Contains(n, "what do i know about"):
		topic := extractQuestionTopic(n, []string{"sobre", "de", "about"})
		if topic == "" && strings.TrimSpace(req.Topic) != "" {
			topic = strings.TrimSpace(req.Topic)
		}
		if topic == "" {
			return UserModelQuery{}, false
		}
		return UserModelQuery{Domain: string(UserModelDomainKnowledge), Topics: []string{topic}}, true
	case strings.Contains(n, "nivel tengo"), strings.Contains(n, "qué nivel tengo"), strings.Contains(n, "que nivel tengo"),
		strings.Contains(n, "mi nivel en"), strings.Contains(n, "my level in"):
		domain := extractQuestionTopic(n, []string{"en", "sobre", "de", "in", "about"})
		if domain == "" && strings.TrimSpace(req.ExplicitDomain) != "" {
			domain = strings.TrimSpace(req.ExplicitDomain)
		}
		if domain == "" {
			return UserModelQuery{}, false
		}
		return UserModelQuery{Domain: string(UserModelDomainCapability), Topics: []string{domain}}, true
	case strings.Contains(n, "cómo prefiero"), strings.Contains(n, "como prefiero"),
		strings.Contains(n, "cómo quieres que respondas"), strings.Contains(n, "como quieres que respondas"),
		strings.Contains(n, "how should you respond"):
		return UserModelQuery{Domain: string(UserModelDomainCommunicationProfile)}, true
	}

	// Generic person-attribute queries are resolved from the same semantic
	// reference used by the observation/write path. The key is extracted from
	// the user's language; no list of attribute names lives here.
	if attribute, ok := ResolveSemanticAttribute(req); ok && !attribute.HasValue && attribute.Subject == UserObservationSubjectUser {
		domain := UserModelDomainAttribute
		if SemanticAttributeDomainForKey(attribute.Key) == SemanticAttributeDomainPreference {
			domain = UserModelDomainPreference
		}
		return UserModelQuery{Domain: string(domain), Topics: []string{attribute.Key}}, true
	}

	return UserModelQuery{}, false
}

func extractQuestionTopic(text string, markers []string) string {
	text = strings.TrimSpace(text)
	for _, marker := range markers {
		marker = strings.TrimSpace(strings.ToLower(marker))
		if marker == "" {
			continue
		}
		// Match a word/phrase boundary, not an arbitrary substring. This is
		// especially important for capability queries: the marker "en" must
		// not match the "en" inside "tengo".
		needle := " " + marker + " "
		if idx := strings.Index(" "+text+" ", needle); idx >= 0 {
			start := idx + len(needle)
			value := strings.TrimSpace((" " + text + " ")[start:])
			value = strings.Trim(value, " ?¿.!,:;")
			if value != "" {
				return value
			}
		}
	}
	return ""
}
