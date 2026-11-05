package selfknowledge

import (
	"strings"
	"sync"
	"time"

	"ahmelia/interaction"
	"ahmelia/personality"
)

// SelfAttribute is the self-owned counterpart of interaction.UserAttribute.
// It is deliberately separate storage: a question about Ahmelia can never
// fall through into the user's model, even when the attribute key is equal.
type SelfAttribute struct {
	Key        string                          `json:"key"`
	Value      string                          `json:"value"`
	Confidence float64                         `json:"confidence"`
	Status     interaction.UserModelItemStatus `json:"status"`
	Source     interaction.UserModelSourceRef  `json:"source"`
	UpdatedAt  time.Time                       `json:"updated_at"`
}

// SelfAttributeStore is a small, read-mostly store for stable self facts.
// Identity seeds it; later self-observation blocks may update it through an
// explicit internal boundary. User input alone never mutates this store.
type SelfAttributeStore struct {
	mu         sync.RWMutex
	attributes map[string]SelfAttribute
}

func NewSelfAttributeStore(identity *personality.Identity) *SelfAttributeStore {
	store := &SelfAttributeStore{attributes: make(map[string]SelfAttribute)}
	if identity == nil {
		return store
	}
	now := time.Now()
	seed := func(key, value string) {
		key = normalizeSelfKey(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return
		}
		store.attributes[key] = SelfAttribute{
			Key: key, Value: value, Confidence: 1,
			Status:    interaction.UserModelStatusActive,
			Source:    interaction.UserModelSourceRef{Kind: interaction.UserSourceSystem, SourceID: "personality.identity", ActorID: "ahmelia", ObservedAt: now},
			UpdatedAt: now,
		}
	}
	seed("nombre", identity.Name)
	seed("rol", identity.Role)
	seed("proposito", identity.Purpose)
	seed("creador", identity.CreatedBy)
	seed("origen", identity.Origin)
	seed("tipo de operacion", identity.OperationType)
	if identity.IsAI {
		seed("es ia", "sí")
	}
	if identity.IsHuman {
		seed("es humana", "sí")
	} else {
		seed("es humana", "no")
	}
	return store
}

func (s *SelfAttributeStore) Get(key string) (SelfAttribute, bool) {
	if s == nil {
		return SelfAttribute{}, false
	}
	key = normalizeSelfKey(key)
	if key == "" {
		return SelfAttribute{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.attributes[key]
	return item, ok
}

func (s *SelfAttributeStore) Upsert(key, value string, confidence float64, source interaction.UserModelSourceRef) bool {
	if s == nil {
		return false
	}
	key = normalizeSelfKey(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return false
	}
	if confidence <= 0 {
		confidence = .5
	}
	if confidence > 1 {
		confidence = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes[key] = SelfAttribute{Key: key, Value: value, Confidence: confidence, Status: interaction.UserModelStatusActive, Source: source, UpdatedAt: time.Now()}
	return true
}

func normalizeSelfKey(key string) string {
	key = strings.TrimSpace(key)
	return strings.Join(strings.Fields(key), " ")
}

// QuerySemantic resolves the attribute at the already-established semantic
// ownership boundary. No second lexical interpretation is required here.
func (s *SelfAttributeStore) QuerySemantic(req interaction.SemanticRequest) (SelfAttribute, bool) {
	if req.Subject != interaction.UserObservationSubjectSelf {
		return SelfAttribute{}, false
	}
	ref, ok := interaction.ResolveSemanticAttribute(req)
	if !ok || ref.HasValue || ref.Subject != interaction.UserObservationSubjectSelf {
		return SelfAttribute{}, false
	}
	return s.Get(ref.Key)
}
