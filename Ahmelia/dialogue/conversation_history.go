package dialogue

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// B96.35 — Conversational Memory 2.0.
//
// ConversationalMemory (B62) remains the compatibility index for topic state
// (active/pending/closed). ConversationHistory is the canonical historical
// record: complete sessions/episodes, ranked retrieval, reconstruction and
// retention. B62 data is migrated once into synthetic historical episodes.

const ConversationHistoryPath = "memory/conversation_history.json"
const ConversationHistorySchemaVersion = 1

// RetentionPolicy is explicit and configurable. It bounds storage without
// silently discarding a recent or currently active session.
type RetentionPolicy struct {
	MaxEpisodes int           `json:"max_episodes"`
	MaxAge      time.Duration `json:"max_age"`
	MaxTurns    int           `json:"max_turns_per_episode"`
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{MaxEpisodes: 500, MaxAge: 180 * 24 * time.Hour, MaxTurns: 0}
}

type HistoricalTurn struct {
	ID           uint64    `json:"id"`
	At           time.Time `json:"at"`
	UserText     string    `json:"user_text"`
	Assistant    string    `json:"assistant,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	SpeechAct    string    `json:"speech_act,omitempty"`
	Language     string    `json:"language,omitempty"`
	Topic        string    `json:"topic,omitempty"`
	Objective    string    `json:"objective,omitempty"`
	Relation     string    `json:"relation,omitempty"`
	ParentTurnID uint64    `json:"parent_turn_id,omitempty"`
}

type ConversationEpisode struct {
	ID          string           `json:"id"`
	SessionID   string           `json:"session_id"`
	Start       time.Time        `json:"start"`
	End         time.Time        `json:"end,omitempty"`
	Status      string           `json:"status"` // active | completed | interrupted
	Turns       []HistoricalTurn `json:"turns"`
	Topics      []string         `json:"topics,omitempty"`
	Objectives  []string         `json:"objectives,omitempty"`
	Summary     string           `json:"summary,omitempty"`
	MigratedB62 bool             `json:"migrated_b62,omitempty"`
}

type ConversationHistory struct {
	mu                sync.RWMutex
	SchemaVersion     int                    `json:"schema_version"`
	Episodes          []*ConversationEpisode `json:"episodes"`
	Retention         RetentionPolicy        `json:"retention"`
	LegacyB62Migrated bool                   `json:"legacy_b62_migrated"`
}

type HistoricalMatch struct {
	Episode       *ConversationEpisode
	Score         float64
	SemanticScore float64
	RecencyScore  float64
}

type HistoricalContext struct {
	Query         string
	Matches       []HistoricalMatch
	Reconstructed string
}

func NewConversationHistory() *ConversationHistory {
	return &ConversationHistory{SchemaVersion: ConversationHistorySchemaVersion, Episodes: make([]*ConversationEpisode, 0, 32), Retention: DefaultRetentionPolicy()}
}

func LoadConversationHistory(path string, legacy *ConversationalMemory) *ConversationHistory {
	h := NewConversationHistory()
	if data, err := os.ReadFile(path); err == nil {
		var loaded ConversationHistory
		if json.Unmarshal(data, &loaded) == nil {
			if loaded.Episodes == nil {
				loaded.Episodes = make([]*ConversationEpisode, 0, 32)
			}
			if loaded.Retention.MaxEpisodes == 0 {
				loaded.Retention = DefaultRetentionPolicy()
			}
			if loaded.SchemaVersion == 0 {
				loaded.SchemaVersion = ConversationHistorySchemaVersion
			}
			h = &loaded
		}
	}
	if legacy != nil && !h.LegacyB62Migrated {
		h.migrateB62Locked(legacy)
		h.LegacyB62Migrated = true
		_ = h.Save(path)
	}
	return h
}

func (h *ConversationHistory) Save(path string) error {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	data, err := json.MarshalIndent(h, "", "  ")
	h.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// BeginSession creates an episode immediately. Turns are persisted as they
// arrive, so a crash still leaves an incomplete historical episode.
func (h *ConversationHistory) BeginSession(sessionID string, at time.Time) string {
	if h == nil {
		return ""
	}
	if at.IsZero() {
		at = time.Now()
	}
	id := fmt.Sprintf("episode_%s_%d", strings.TrimSpace(sessionID), at.UnixNano())
	h.mu.Lock()
	h.Episodes = append(h.Episodes, &ConversationEpisode{ID: id, SessionID: sessionID, Start: at, Status: "active", Turns: make([]HistoricalTurn, 0, 32)})
	h.mu.Unlock()
	return id
}

func (h *ConversationHistory) RecordTurn(episodeID string, turn HistoricalTurn) bool {
	if h == nil || strings.TrimSpace(episodeID) == "" || strings.TrimSpace(turn.UserText) == "" {
		return false
	}
	if turn.At.IsZero() {
		turn.At = time.Now()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ep := h.episodeLocked(episodeID)
	if ep == nil {
		return false
	}
	if h.Retention.MaxTurns > 0 && len(ep.Turns) >= h.Retention.MaxTurns {
		ep.Turns = ep.Turns[1:]
	}
	ep.Turns = append(ep.Turns, turn)
	addUnique(&ep.Topics, turn.Topic)
	addUnique(&ep.Objectives, turn.Objective)
	if turn.At.Before(ep.Start) {
		ep.Start = turn.At
	}
	return true
}

func (h *ConversationHistory) CompleteSession(episodeID, summary string, at time.Time) bool {
	if h == nil {
		return false
	}
	if at.IsZero() {
		at = time.Now()
	}
	h.mu.Lock()
	ep := h.episodeLocked(episodeID)
	if ep == nil {
		h.mu.Unlock()
		return false
	}
	ep.End, ep.Status, ep.Summary = at, "completed", strings.TrimSpace(summary)
	h.applyRetentionLocked(at)
	h.mu.Unlock()
	return true
}

func (h *ConversationHistory) InterruptSession(episodeID string, at time.Time) bool {
	if h == nil {
		return false
	}
	if at.IsZero() {
		at = time.Now()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ep := h.episodeLocked(episodeID)
	if ep == nil {
		return false
	}
	ep.End, ep.Status = at, "interrupted"
	return true
}

func (h *ConversationHistory) Search(query string, now time.Time, limit int) []HistoricalMatch {
	return h.SearchExcluding(query, now, limit, "")
}

// SearchExcluding retrieves historical episodes while excluding the current
// episode when the caller is resolving a historical request from inside an
// active session. This prevents the recall query itself (and nearby turns in
// the same live conversation) from becoming its own historical evidence.
// It does not exclude all active episodes: an active episode may represent a
// previous process that ended unexpectedly and is therefore valid historical
// evidence.
func (h *ConversationHistory) SearchExcluding(query string, now time.Time, limit int, excludedEpisodeID string) []HistoricalMatch {
	if h == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	qTokens := semanticTokens(query)
	qNorm := normalizeMemoryText(query)
	anchorTokens := historicalAnchorTokens(qTokens, query)
	h.mu.RLock()
	matches := make([]HistoricalMatch, 0)
	for _, ep := range h.Episodes {
		if excludedEpisodeID != "" && ep.ID == excludedEpisodeID {
			continue
		}
		// A query with a concrete subject must not fall back to an unrelated
		// historical episode merely because generic intent words such as
		// "decidimos" overlap. This is the evidence-layer equivalent of
		// entity grounding: if the user asks about Aurora, an episode that
		// never mentions Aurora (or a semantically matching topic/objective)
		// is not eligible evidence. Broad retrieval remains allowed for
		// subject-less questions such as "¿qué habíamos decidido?".
		if len(anchorTokens) > 0 && !episodeHasHistoricalAnchors(ep, anchorTokens) {
			continue
		}
		sem := semanticScore(qNorm, qTokens, ep)
		if sem <= 0 {
			continue
		}
		rec := recencyScore(ep.Start, now)
		score := sem*0.70 + rec*0.30
		matches = append(matches, HistoricalMatch{Episode: cloneEpisode(ep), Score: score, SemanticScore: sem, RecencyScore: rec})
	}
	h.mu.RUnlock()
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Episode.Start.After(matches[j].Episode.Start)
		}
		return matches[i].Score > matches[j].Score
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// Latest returns historical episodes ordered by recency. It is used when
// the user asks for an open-ended historical recall without naming a topic.
func (h *ConversationHistory) Latest(now time.Time, limit int, olderThan time.Duration) []HistoricalMatch {
	if h == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	h.mu.RLock()
	matches := make([]HistoricalMatch, 0)
	for _, ep := range h.Episodes {
		if ep.Start.IsZero() {
			continue
		}
		age := now.Sub(ep.Start)
		if age < 0 || (olderThan > 0 && age < olderThan) {
			continue
		}
		rec := recencyScore(ep.Start, now)
		matches = append(matches, HistoricalMatch{Episode: cloneEpisode(ep), Score: rec, RecencyScore: rec})
	}
	h.mu.RUnlock()
	sort.Slice(matches, func(i, j int) bool { return matches[i].Episode.Start.After(matches[j].Episode.Start) })
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func (h *ConversationHistory) Reconstruct(query string, now time.Time, limit int) HistoricalContext {
	matches := h.Search(query, now, limit)
	var b strings.Builder
	if len(matches) == 0 {
		return HistoricalContext{Query: query, Matches: matches}
	}
	b.WriteString("Contexto histórico relevante:\n")
	for i, m := range matches {
		ep := m.Episode
		fmt.Fprintf(&b, "%d. %s — %s", i+1, formatEpisodeTime(ep.Start), episodeLabel(ep))
		if ep.Summary != "" {
			fmt.Fprintf(&b, ". %s", ep.Summary)
		}
		if len(ep.Topics) > 0 {
			fmt.Fprintf(&b, ". Temas: %s", strings.Join(ep.Topics, ", "))
		}
		if len(ep.Turns) > 0 {
			// Include only the most relevant recent turns, keeping reconstruction bounded.
			shown := 0
			for j := len(ep.Turns) - 1; j >= 0 && shown < 3; j-- {
				t := ep.Turns[j]
				if semanticScore(normalizeMemoryText(query), semanticTokens(query), ep) > 0 {
					if strings.TrimSpace(t.UserText) != "" {
						fmt.Fprintf(&b, " Turno: %s", t.UserText)
						shown++
					}
				}
			}
		}
		b.WriteByte('\n')
	}
	return HistoricalContext{Query: query, Matches: matches, Reconstructed: strings.TrimSpace(b.String())}
}

func (h *ConversationHistory) PendingMigration() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return !h.LegacyB62Migrated
}

func (h *ConversationHistory) applyRetentionLocked(now time.Time) {
	p := h.Retention
	kept := h.Episodes[:0]
	cutoff := now.Add(-p.MaxAge)
	for _, ep := range h.Episodes {
		if ep.Status == "active" || p.MaxAge <= 0 || ep.Start.After(cutoff) {
			kept = append(kept, ep)
		}
	}
	h.Episodes = kept
	if p.MaxEpisodes > 0 && len(h.Episodes) > p.MaxEpisodes {
		// Never delete active episodes solely because of the count bound.
		sort.SliceStable(h.Episodes, func(i, j int) bool { return h.Episodes[i].Start.Before(h.Episodes[j].Start) })
		excess := len(h.Episodes) - p.MaxEpisodes
		for excess > 0 {
			idx := -1
			for i, ep := range h.Episodes {
				if ep.Status != "active" {
					idx = i
					break
				}
			}
			if idx < 0 {
				break
			}
			h.Episodes = append(h.Episodes[:idx], h.Episodes[idx+1:]...)
			excess--
		}
	}
}

func (h *ConversationHistory) episodeLocked(id string) *ConversationEpisode {
	for _, ep := range h.Episodes {
		if ep.ID == id {
			return ep
		}
	}
	return nil
}

func (h *ConversationHistory) migrateB62Locked(legacy *ConversationalMemory) {
	if legacy == nil {
		return
	}
	now := time.Now()
	for _, rec := range legacy.Topics {
		if rec == nil || strings.TrimSpace(rec.Name) == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, rec.LastTouched)
		if err != nil {
			at = now
		}
		first, err := time.Parse(time.RFC3339, rec.FirstTouched)
		if err != nil {
			first = at
		}
		id := fmt.Sprintf("legacy_b62_%s_%d", strings.ToLower(strings.ReplaceAll(rec.Name, " ", "_")), at.UnixNano())
		ep := &ConversationEpisode{ID: id, SessionID: "legacy-b62", Start: first, End: at, Status: "completed", Topics: []string{rec.Name}, Summary: rec.LastNote, MigratedB62: true, Turns: []HistoricalTurn{{ID: uint64(max(1, rec.TurnCount)), At: at, UserText: rec.Name, Topic: rec.Name}}}
		h.Episodes = append(h.Episodes, ep)
	}
}

func semanticScore(qNorm string, qTokens map[string]struct{}, ep *ConversationEpisode) float64 {
	if ep == nil {
		return 0
	}
	parts := append(append([]string{}, ep.Topics...), ep.Objectives...)
	parts = append(parts, episodeTurnText(ep)...)
	text := strings.Join(parts, " ")
	tokens := semanticTokens(text)
	if len(qTokens) == 0 || len(tokens) == 0 {
		return 0
	}
	overlap := 0
	for t := range qTokens {
		if _, ok := tokens[t]; ok {
			overlap++
		}
	}
	score := float64(overlap) / float64(len(qTokens))
	if strings.Contains(normalizeMemoryText(text), qNorm) {
		score += 0.35
	}
	// Topic/objective matches are stronger than arbitrary turn-word overlap.
	for _, topic := range ep.Topics {
		if strings.Contains(normalizeMemoryText(topic), qNorm) || strings.Contains(qNorm, normalizeMemoryText(topic)) {
			score += 0.25
			break
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

func recencyScore(start, now time.Time) float64 {
	age := now.Sub(start)
	if age < 0 {
		age = 0
	}
	days := age.Hours() / 24
	return math.Exp(-days / 30.0)
}
func episodeTurnText(ep *ConversationEpisode) []string {
	out := make([]string, 0, len(ep.Turns)*2)
	for _, t := range ep.Turns {
		out = append(out, t.UserText, t.Assistant)
	}
	return out
}

// historicalAnchorTokens returns subject-bearing query tokens after removing
// generic historical intent vocabulary. These anchors prevent a concrete
// question from retrieving a semantically unrelated episode.
func historicalAnchorTokens(tokens map[string]struct{}, query string) map[string]struct{} {
	generic := map[string]bool{
		"decid": true, "acord": true, "prefer": true, "solucion": true,
		"resolv": true, "pendient": true, "objetiv": true, "problem": true,
		"cambi": true, "implement": true, "hicimos": true, "hiciste": true,
		"dijimos": true, "dicho": true, "hablamos": true, "record": true,
		"paso": true, "sucedio": true, "ocurrio": true, "antes": true,
		"anterior": true, "historial": true, "historia": true,
		"responder": true, "respondera": true, "habiamos": true, "habia": true, "debia": true, "deberia": true,
	}
	out := make(map[string]struct{})
	for token := range tokens {
		if !generic[token] {
			out[token] = struct{}{}
		}
	}
	_ = query // reserved for future linguistic anchor extraction
	return out
}

func episodeHasHistoricalAnchors(ep *ConversationEpisode, anchors map[string]struct{}) bool {
	if ep == nil || len(anchors) == 0 {
		return false
	}
	texts := make([]string, 0, len(ep.Turns)*2+len(ep.Topics)+len(ep.Objectives))
	texts = append(texts, ep.Topics...)
	texts = append(texts, ep.Objectives...)
	texts = append(texts, episodeTurnText(ep)...)
	for _, text := range texts {
		for token := range semanticTokens(text) {
			if _, ok := anchors[token]; ok {
				return true
			}
		}
	}
	return false
}

func semanticTokens(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.Fields(normalizeMemoryText(s)) {
		if len(w) < 3 || memoryStopWords[w] {
			continue
		}
		out[w] = struct{}{}
		// Lightweight concept normalization keeps historical retrieval
		// formulation-agnostic without making the evidence layer topic-specific.
		for _, prefix := range []string{"decid", "acord", "prefer", "solucion", "resolv", "pendient", "objetiv", "problem", "cambi", "implement", "respuest", "respond"} {
			if strings.HasPrefix(w, prefix) {
				out[prefix] = struct{}{}
				if prefix == "respond" {
					out["respuest"] = struct{}{}
				}
				if prefix == "respuest" {
					out["respond"] = struct{}{}
				}
				break
			}
		}
	}
	return out
}
func normalizeMemoryText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == 'á' || r == 'é' || r == 'í' || r == 'ó' || r == 'ú' || r == 'ñ' || r == 'ü' {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
func addUnique(dst *[]string, v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	for _, x := range *dst {
		if strings.EqualFold(x, v) {
			return
		}
	}
	*dst = append(*dst, v)
}
func cloneEpisode(ep *ConversationEpisode) *ConversationEpisode {
	if ep == nil {
		return nil
	}
	c := *ep
	c.Turns = append([]HistoricalTurn(nil), ep.Turns...)
	c.Topics = append([]string(nil), ep.Topics...)
	c.Objectives = append([]string(nil), ep.Objectives...)
	return &c
}
func formatEpisodeTime(t time.Time) string {
	if t.IsZero() {
		return "fecha desconocida"
	}
	months := [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	hour := t.Hour()
	period := "a. m."
	displayHour := hour
	if hour >= 12 {
		period = "p. m."
	}
	if displayHour == 0 {
		displayHour = 12
	} else if displayHour > 12 {
		displayHour -= 12
	}
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	tz := fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
	return fmt.Sprintf("%d de %s de %d, %d:%02d %s (%s)", t.Day(), months[t.Month()-1], t.Year(), displayHour, t.Minute(), period, tz)
}
func episodeLabel(ep *ConversationEpisode) string {
	if ep.Status == "interrupted" {
		return "sesión interrumpida"
	}
	if ep.MigratedB62 {
		return "registro migrado de B62"
	}
	return "sesión conversacional"
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var memoryStopWords = map[string]bool{"que": true, "qué": true, "como": true, "cómo": true, "con": true, "para": true, "por": true, "una": true, "uno": true, "las": true, "los": true, "del": true, "de": true, "el": true, "la": true, "y": true, "en": true, "un": true, "sobre": true, "hace": true, "donde": true, "dónde": true, "nos": true, "lo": true, "dejamos": true, "quedamos": true, "hablamos": true, "quiero": true, "tengo": true, "mi": true, "mis": true, "me": true, "tu": true, "tus": true}
