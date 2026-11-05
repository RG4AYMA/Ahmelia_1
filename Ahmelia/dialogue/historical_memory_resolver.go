package dialogue

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// B96.35 — Historical Recall is an evidence-retrieval layer over the canonical
// episodic history. It deliberately separates:
//   * historical storage (what happened),
//   * historical evidence (what can substantiate an answer),
//   * retrieval/meta responses (what Ahmelia later said about the history).
// Persisting a turn therefore never grants it historical authority.

type HistoricalRecallMode string

const (
	HistoricalRecallSummary  HistoricalRecallMode = "summary"
	HistoricalRecallDetailed HistoricalRecallMode = "detailed"
)

type HistoricalIntent string

const (
	HistoricalIntentUnknown           HistoricalIntent = "unknown"
	HistoricalIntentDecision          HistoricalIntent = "decision"
	HistoricalIntentAction            HistoricalIntent = "action"
	HistoricalIntentProblem           HistoricalIntent = "problem"
	HistoricalIntentSolution          HistoricalIntent = "solution"
	HistoricalIntentProblemSolution   HistoricalIntent = "problem_solution"
	HistoricalIntentPending           HistoricalIntent = "pending"
	HistoricalIntentPreference        HistoricalIntent = "preference"
	HistoricalIntentObjective         HistoricalIntent = "objective"
	HistoricalIntentEvent             HistoricalIntent = "event"
	HistoricalIntentDecisionEvolution HistoricalIntent = "decision_evolution"
	HistoricalIntentTimeline          HistoricalIntent = "timeline"
)

type HistoricalEvidenceKind string

const (
	EvidenceUnknown    HistoricalEvidenceKind = "unknown"
	EvidenceQuestion   HistoricalEvidenceKind = "question"
	EvidenceAnswer     HistoricalEvidenceKind = "answer"
	EvidenceDecision   HistoricalEvidenceKind = "decision"
	EvidenceAction     HistoricalEvidenceKind = "action"
	EvidenceProblem    HistoricalEvidenceKind = "problem"
	EvidenceSolution   HistoricalEvidenceKind = "solution"
	EvidencePreference HistoricalEvidenceKind = "preference"
	EvidenceObjective  HistoricalEvidenceKind = "objective"
	EvidencePending    HistoricalEvidenceKind = "pending"
	EvidenceEvent      HistoricalEvidenceKind = "event"
	EvidenceChange     HistoricalEvidenceKind = "change"
	EvidenceStatement  HistoricalEvidenceKind = "statement"
)

type HistoricalEvidenceProvenance string

const (
	EvidencePrimary     HistoricalEvidenceProvenance = "primary"
	EvidenceSecondary   HistoricalEvidenceProvenance = "secondary"
	EvidenceDerived     HistoricalEvidenceProvenance = "derived"
	EvidenceMeta        HistoricalEvidenceProvenance = "meta"
	EvidenceNonEvidence HistoricalEvidenceProvenance = "non_evidence"
)

type HistoricalRecallResult struct {
	Found      bool
	Mode       HistoricalRecallMode
	Intent     HistoricalIntent
	Evidence   []HistoricalMatch
	Response   string
	candidates []historicalEvidenceCandidate
}

type historicalEvidenceCandidate struct {
	Episode    *ConversationEpisode
	Turn       *HistoricalTurn
	Text       string
	Kind       HistoricalEvidenceKind
	Provenance HistoricalEvidenceProvenance
	Score      float64
	Index      int
}

func IsHistoricalRecallIntent(text string) bool {
	n := normalizeMemoryText(text)
	markers := []string{
		"antes", "anterior", "anteriores", "historial", "historia", "hace tiempo",
		"otra conversacion", "conversaciones anteriores", "hablamos", "habia dicho",
		"habíamos dicho", "recordas", "recuerdas", "te acuerdas", "acordamos",
		"habíamos decidido", "habia decidido", "ultima vez", "cuando hablamos",
		"lo que hablamos", "lo que dijimos", "lo que decidimos", "que hicimos",
		"qué hicimos", "que acordamos", "qué acordamos", "que problema tuvimos",
		"qué problema tuvimos", "problema encontramos", "problemas encontramos",
		"como lo solucionamos", "cómo lo solucionamos", "que solucion hicimos",
		"qué solución hicimos", "que paso", "qué paso", "que sucedio", "qué sucedió",
		"que ocurrio", "qué ocurrió", "que quedo pendiente", "qué quedó pendiente",
		"que preferia", "qué prefería", "que preferiamos", "qué preferíamos", "que preferia mantener", "qué prefería mantener",
		"cual era el objetivo", "cuál era el objetivo", "cual era la meta", "cuál era la meta",
		"por que cambiamos", "por qué cambiamos", "cambiamos la decision", "cambiamos la decisión",
	}
	for _, marker := range markers {
		if strings.Contains(n, normalizeMemoryText(marker)) {
			return true
		}
	}
	return false
}

func HistoricalRecallModeForQuery(text string) HistoricalRecallMode {
	n := normalizeMemoryText(text)
	detailed := []string{
		"todo el historial", "historial completo", "historial detallado", "todo lo que hablamos",
		"todas las conversaciones", "todas las decisiones", "muestrame todo", "muéstrame todo",
		"muestrame todos", "muéstrame todos", "detalle completo", "detallado", "detallada",
		"cronologia", "cronología", "paso a paso", "todos los eventos", "eventos relacionados",
		"todo lo relacionado", "dame todos los detalles", "dime todos los detalles", "quiero todos los detalles",
	}
	for _, marker := range detailed {
		if strings.Contains(n, normalizeMemoryText(marker)) {
			return HistoricalRecallDetailed
		}
	}
	return HistoricalRecallSummary
}

func InferHistoricalIntent(query string) HistoricalIntent {
	n := normalizeMemoryText(query)
	switch {
	case containsAny(n, "como habiamos decidido que debia responder", "cómo habíamos decidido que debía responder", "como queriamos que respondiera", "cómo queríamos que respondiera"):
		return HistoricalIntentPreference
	case containsAny(n, "por que cambiamos", "por qué cambiamos", "cambiamos la decision", "cambiamos la decisión"):
		return HistoricalIntentDecisionEvolution
	case containsAny(n, "problema", "error", "fallo") && containsAny(n, "solucion", "resolv", "arregl"):
		return HistoricalIntentProblemSolution
	case containsAny(n, "decidido", "decidimos", "acordamos", "acordado", "que vamos a hacer", "que habiamos decidido"):
		return HistoricalIntentDecision
	case containsAny(n, "problema", "error", "fallo", "que paso"):
		return HistoricalIntentProblem
	case containsAny(n, "solucion", "resolv", "arregl", "corregimos"):
		return HistoricalIntentSolution
	case containsAny(n, "pendiente", "falta", "por hacer", "proximo paso"):
		return HistoricalIntentPending
	case containsAny(n, "preferencia", "preferia", "preferiamos", "preferíamos", "prefiero", "que te gusta", "que prefiero", "como habiamos decidido que debia responder", "como habíamos decidido que debía responder"):
		return HistoricalIntentPreference
	case containsAny(n, "objetivo", "meta", "proximo objetivo"):
		return HistoricalIntentObjective
	case containsAny(n, "hicimos", "que cambiamos", "que agregamos", "accion"):
		return HistoricalIntentAction
	case HistoricalRecallModeForQuery(query) == HistoricalRecallDetailed:
		return HistoricalIntentTimeline
	case containsAny(n, "que ocurrio", "que sucedio", "que paso"):
		return HistoricalIntentEvent
	default:
		return HistoricalIntentUnknown
	}
}

// ResolveHistoricalReferenceQuery enriches a historical query with the
// referent already resolved by the live conversational layer. The referent
// is context only; it is never persisted as historical evidence. This keeps
// "¿por qué cambiamos eso?" from becoming a literal search for "eso".
func ResolveHistoricalReferenceQuery(query string, referentTopic, referentObjective string) string {
	n := normalizeMemoryText(query)
	if !containsAny(n, "eso", "esa decision", "esa decisión", "ese problema", "ese proyecto", "aquello", "lo anterior", "esa solucion", "esa solución") {
		return query
	}
	parts := []string{strings.TrimSpace(query), strings.TrimSpace(referentTopic), strings.TrimSpace(referentObjective)}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func ResolveHistoricalRecall(history *ConversationHistory, query string, now time.Time) HistoricalRecallResult {
	return ResolveHistoricalRecallExcluding(history, query, now, "")
}

func ResolveHistoricalRecallExcluding(history *ConversationHistory, query string, now time.Time, excludedEpisodeID string) HistoricalRecallResult {
	result := HistoricalRecallResult{Mode: HistoricalRecallModeForQuery(query), Intent: InferHistoricalIntent(query)}
	if history == nil || strings.TrimSpace(query) == "" {
		return result
	}
	if now.IsZero() {
		now = time.Now()
	}

	// Retrieval is deliberately broad. The next stage, not Search(), decides
	// whether a turn is evidence for the requested historical intent.
	matches := history.SearchExcluding(query, now, 32, excludedEpisodeID)
	if len(matches) == 0 {
		return result
	}

	evidence := rankHistoricalEvidence(query, matches)
	selectedEpisodes, selectedEvidence := selectHistoricalEvidence(matches, evidence, result.Intent, result.Mode)
	if len(selectedEpisodes) == 0 || len(selectedEvidence) == 0 {
		return result
	}

	result.Found = true
	result.Evidence = selectedEpisodes
	result.candidates = selectedEvidence
	if result.Mode == HistoricalRecallDetailed {
		result.Response = synthesizeDetailedHistory(query, selectedEpisodes, selectedEvidence, result.Intent)
	} else {
		result.Response = synthesizeHistoricalSummary(selectedEvidence, result.Intent)
	}
	if strings.TrimSpace(result.Response) == "" {
		result.Found = false
	}
	return result
}

func rankHistoricalEvidence(query string, matches []HistoricalMatch) []historicalEvidenceCandidate {
	qNorm, qTokens := normalizeMemoryText(query), semanticTokens(query)
	intent := InferHistoricalIntent(query)
	out := make([]historicalEvidenceCandidate, 0, len(matches)*4)
	for _, match := range matches {
		ep := match.Episode
		if ep == nil {
			continue
		}
		for i := range ep.Turns {
			turn := &ep.Turns[i]
			for _, part := range turnEvidenceParts(turn) {
				if strings.TrimSpace(part.Text) == "" || part.Provenance == EvidenceNonEvidence || part.Provenance == EvidenceMeta {
					continue
				}
				score := evidenceSemanticScore(qNorm, qTokens, ep, turn, part.Text)
				score += evidenceIntentScore(intent, part.Kind)
				if part.Kind == EvidenceQuestion {
					score -= 0.70
				}
				if part.Provenance == EvidenceSecondary {
					score -= 0.08
				}
				if score <= 0 {
					continue
				}
				out = append(out, historicalEvidenceCandidate{Episode: ep, Turn: turn, Text: strings.TrimSpace(part.Text), Kind: part.Kind, Provenance: part.Provenance, Score: score, Index: i})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			if out[i].Episode.Start.Equal(out[j].Episode.Start) {
				return out[i].Index < out[j].Index
			}
			return out[i].Episode.Start.After(out[j].Episode.Start)
		}
		return out[i].Score > out[j].Score
	})
	return out
}

type evidencePart struct {
	Text       string
	Kind       HistoricalEvidenceKind
	Provenance HistoricalEvidenceProvenance
}

func turnEvidenceParts(turn *HistoricalTurn) []evidencePart {
	if turn == nil {
		return nil
	}
	parts := make([]evidencePart, 0, 2)
	if u := strings.TrimSpace(turn.UserText); u != "" {
		parts = append(parts, evidencePart{u, classifyHistoricalText(u, turn, false), classifyHistoricalProvenance(u, false)})
	}
	if a := strings.TrimSpace(turn.Assistant); a != "" {
		parts = append(parts, evidencePart{a, classifyHistoricalText(a, turn, true), classifyHistoricalProvenance(a, true)})
	}
	return parts
}

func classifyHistoricalProvenance(text string, assistant bool) HistoricalEvidenceProvenance {
	n := normalizeMemoryText(text)
	if isHistoricalQuestion(text) {
		return EvidenceNonEvidence
	}
	if assistant && isHistoricalRetrievalMeta(n) {
		return EvidenceMeta
	}
	if isGenericHistoricalAck(text) {
		return EvidenceSecondary
	}
	if assistant {
		return EvidenceSecondary
	}
	return EvidencePrimary
}

func isHistoricalRetrievalMeta(n string) bool {
	patterns := []string{
		"recuerdo conversaciones anteriores", "no tengo suficiente detalle", "lo que puedo recuperar",
		"esto es lo que recupere", "esto es lo que recuperé", "sí lo relevante es", "si lo relevante es",
		"lo relevante es", "te escucho dime que tienes en mente",
		"dime que tienes en mente", "que parte de", "qué parte de",
	}
	for _, p := range patterns {
		if strings.Contains(n, normalizeMemoryText(p)) {
			return true
		}
	}
	return false
}

func classifyHistoricalText(text string, turn *HistoricalTurn, assistant bool) HistoricalEvidenceKind {
	n := normalizeMemoryText(text)
	if isHistoricalQuestion(text) {
		return EvidenceQuestion
	}
	// Existing structured metadata outranks lexical fallback.
	for _, value := range []string{turn.Kind, turn.SpeechAct, turn.Relation} {
		m := normalizeMemoryText(value)
		switch {
		case strings.Contains(m, "decision") || strings.Contains(m, "decis"):
			return EvidenceDecision
		case strings.Contains(m, "problem"):
			return EvidenceProblem
		case strings.Contains(m, "solution") || strings.Contains(m, "solucion"):
			return EvidenceSolution
		case strings.Contains(m, "pending") || strings.Contains(m, "pendiente"):
			return EvidencePending
		case strings.Contains(m, "preference") || strings.Contains(m, "preferencia"):
			return EvidencePreference
		case strings.Contains(m, "objective") || strings.Contains(m, "objetivo"):
			return EvidenceObjective
		case strings.Contains(m, "change") || strings.Contains(m, "cambio"):
			return EvidenceChange
		}
	}
	switch {
	case containsAny(n, "decidimos", "decidi", "acordamos", "acordado", "optamos", "elegimos", "finalmente dejamos", "se decidio", "se establecio"):
		return EvidenceDecision
	case containsAny(n, "cambiamos la decision", "cambiamos la decisión", "cambio de decision", "cambio de decisión", "decidimos cambiar"):
		return EvidenceChange
	case containsAny(n, "problema", "fallaba", "fallo", "error", "no funcionaba", "nos encontramos con"):
		return EvidenceProblem
	case containsAny(n, "solucionamos", "solucion", "resolvimos", "arreglamos", "corregimos", "quedo resuelto"):
		return EvidenceSolution
	case containsAny(n, "prefiero", "preferimos", "me gusta", "no me gusta", "quiero que"):
		return EvidencePreference
	case containsAny(n, "objetivo", "meta", "queremos lograr", "la idea es"):
		return EvidenceObjective
	case containsAny(n, "pendiente", "queda pendiente", "faltaria", "falta hacer", "por hacer"):
		return EvidencePending
	case containsAny(n, "hicimos", "hice", "creamos", "implementamos", "cambiamos", "agregamos", "eliminamos", "probamos"):
		return EvidenceAction
	}
	if assistant {
		return EvidenceAnswer
	}
	return EvidenceStatement
}

func evidenceIntentScore(intent HistoricalIntent, kind HistoricalEvidenceKind) float64 {
	switch intent {
	case HistoricalIntentDecision:
		if kind == EvidenceDecision {
			return 0.65
		}
	case HistoricalIntentAction:
		if kind == EvidenceAction {
			return 0.65
		}
	case HistoricalIntentProblem:
		if kind == EvidenceProblem {
			return 0.65
		}
	case HistoricalIntentSolution:
		if kind == EvidenceSolution {
			return 0.65
		}
	case HistoricalIntentProblemSolution:
		if kind == EvidenceProblem || kind == EvidenceSolution {
			return 0.50
		}
	case HistoricalIntentPending:
		if kind == EvidencePending {
			return 0.65
		}
	case HistoricalIntentPreference:
		if kind == EvidencePreference {
			return 0.65
		}
	case HistoricalIntentObjective:
		if kind == EvidenceObjective {
			return 0.65
		}
	case HistoricalIntentDecisionEvolution:
		if kind == EvidenceDecision || kind == EvidenceChange || kind == EvidenceProblem {
			return 0.45
		}
	case HistoricalIntentEvent:
		if kind == EvidenceEvent {
			return 0.65
		}
	case HistoricalIntentTimeline:
		if kind != EvidenceQuestion {
			return 0.20
		}
	}
	if kind == EvidenceAnswer || kind == EvidenceStatement {
		return 0.08
	}
	return 0
}

func evidenceSemanticScore(qNorm string, qTokens map[string]struct{}, ep *ConversationEpisode, turn *HistoricalTurn, text string) float64 {
	tokens := semanticTokens(text)
	if len(qTokens) == 0 || len(tokens) == 0 {
		return 0
	}
	overlap := 0
	for token := range qTokens {
		if _, ok := tokens[token]; ok {
			overlap++
		}
	}
	score := float64(overlap) / float64(len(qTokens))
	if qNorm != "" && strings.Contains(normalizeMemoryText(text), qNorm) {
		score += 0.30
	}
	if turn != nil {
		if turn.Topic != "" && relatedText(qNorm, turn.Topic) {
			score += 0.25
		}
		if turn.Objective != "" && relatedText(qNorm, turn.Objective) {
			score += 0.20
		}
	}
	if ep != nil {
		for _, topic := range ep.Topics {
			if relatedText(qNorm, topic) {
				score += 0.10
				break
			}
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

func relatedText(query, value string) bool {
	q, v := normalizeMemoryText(query), normalizeMemoryText(value)
	return q != "" && v != "" && (strings.Contains(q, v) || strings.Contains(v, q))
}

func selectHistoricalEvidence(matches []HistoricalMatch, evidence []historicalEvidenceCandidate, intent HistoricalIntent, mode HistoricalRecallMode) ([]HistoricalMatch, []historicalEvidenceCandidate) {
	threshold, maxEpisodes, maxEvidence := 0.32, 4, 4
	if mode == HistoricalRecallDetailed {
		threshold, maxEpisodes, maxEvidence = 0.22, 12, 32
	}
	if intent == HistoricalIntentUnknown {
		threshold = 0.28
	}

	bestByEpisode := map[string]historicalEvidenceCandidate{}
	for _, c := range evidence {
		if c.Episode == nil || c.Score < threshold || c.Provenance == EvidenceNonEvidence || c.Provenance == EvidenceMeta || c.Kind == EvidenceQuestion {
			continue
		}
		if intent == HistoricalIntentDecision && c.Kind != EvidenceDecision && c.Kind != EvidenceStatement && c.Kind != EvidenceAnswer {
			continue
		}
		if best, ok := bestByEpisode[c.Episode.ID]; !ok || c.Score > best.Score {
			bestByEpisode[c.Episode.ID] = c
		}
	}
	type ranked struct {
		match HistoricalMatch
		best  historicalEvidenceCandidate
	}
	rankedEpisodes := make([]ranked, 0, len(bestByEpisode))
	for _, m := range matches {
		if m.Episode == nil {
			continue
		}
		if best, ok := bestByEpisode[m.Episode.ID]; ok {
			rankedEpisodes = append(rankedEpisodes, ranked{m, best})
		}
	}
	sort.SliceStable(rankedEpisodes, func(i, j int) bool {
		if rankedEpisodes[i].best.Score == rankedEpisodes[j].best.Score {
			return rankedEpisodes[i].match.Episode.Start.After(rankedEpisodes[j].match.Episode.Start)
		}
		return rankedEpisodes[i].best.Score > rankedEpisodes[j].best.Score
	})
	episodes := make([]HistoricalMatch, 0, maxEpisodes)
	allowed := map[string]bool{}
	for i := 0; i < len(rankedEpisodes) && i < maxEpisodes; i++ {
		episodes = append(episodes, rankedEpisodes[i].match)
		allowed[rankedEpisodes[i].match.Episode.ID] = true
	}
	sort.SliceStable(episodes, func(i, j int) bool { return episodes[i].Episode.Start.Before(episodes[j].Episode.Start) })

	selected := make([]historicalEvidenceCandidate, 0, maxEvidence)
	seen := map[string]bool{}
	for _, c := range evidence {
		if c.Episode == nil || !allowed[c.Episode.ID] || c.Score < threshold || c.Provenance == EvidenceNonEvidence || c.Provenance == EvidenceMeta || c.Kind == EvidenceQuestion {
			continue
		}
		key := normalizeMemoryText(c.Text)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, c)
		if len(selected) >= maxEvidence {
			break
		}
	}
	return episodes, selected
}

func synthesizeHistoricalRecall(query string, matches []HistoricalMatch, mode HistoricalRecallMode) string {
	evidence := rankHistoricalEvidence(query, matches)
	_, selected := selectHistoricalEvidence(matches, evidence, InferHistoricalIntent(query), mode)
	if mode == HistoricalRecallDetailed {
		episodes, _ := selectHistoricalEvidence(matches, evidence, InferHistoricalIntent(query), mode)
		return synthesizeDetailedHistory(query, episodes, selected, InferHistoricalIntent(query))
	}
	return synthesizeHistoricalSummary(selected, InferHistoricalIntent(query))
}

func synthesizeHistoricalSummary(candidates []historicalEvidenceCandidate, intent HistoricalIntent) string {
	if len(candidates) == 0 {
		return ""
	}
	// For a point query, choose the strongest evidence of the requested type.
	target := HistoricalEvidenceKind("")
	switch intent {
	case HistoricalIntentDecision:
		target = EvidenceDecision
	case HistoricalIntentProblem:
		target = EvidenceProblem
	case HistoricalIntentSolution:
		target = EvidenceSolution
	case HistoricalIntentPending:
		target = EvidencePending
	case HistoricalIntentPreference:
		target = EvidencePreference
	case HistoricalIntentObjective:
		target = EvidenceObjective
	}
	best := candidates[0]
	for _, c := range candidates {
		if target == "" || c.Kind != target {
			continue
		}
		if best.Kind != target || (c.Provenance == EvidencePrimary && best.Provenance != EvidencePrimary) || (c.Provenance == best.Provenance && c.Score > best.Score) {
			best = c
		}
	}
	// Decision/evolution answers are concise but may need a small causal chain.
	if intent == HistoricalIntentDecisionEvolution || intent == HistoricalIntentProblemSolution {
		texts := make([]string, 0, 3)
		seen := map[string]bool{}
		for _, c := range candidates {
			if c.Kind == EvidenceQuestion {
				continue
			}
			k := normalizeMemoryText(c.Text)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			texts = append(texts, fmt.Sprintf("%s: %s", formatTurnTime(c.Turn.At), c.Text))
			if len(texts) == 3 {
				break
			}
		}
		if len(texts) > 0 {
			return "Sí. Lo relevante es:\n" + strings.Join(texts, "\n")
		}
	}
	return strings.TrimSpace(best.Text)
}

func synthesizeDetailedHistory(query string, episodes []HistoricalMatch, evidence []historicalEvidenceCandidate, intent HistoricalIntent) string {
	var b strings.Builder
	b.WriteString("Esto es lo que recuperé de nuestras conversaciones anteriores:\n")
	evidenceByEpisode := map[string][]historicalEvidenceCandidate{}
	for _, c := range evidence {
		if c.Episode != nil {
			evidenceByEpisode[c.Episode.ID] = append(evidenceByEpisode[c.Episode.ID], c)
		}
	}
	for i, m := range episodes {
		ep := m.Episode
		if ep == nil {
			continue
		}
		fmt.Fprintf(&b, "%d. %s", i+1, formatEpisodeTime(ep.Start))
		if len(ep.Topics) > 0 {
			fmt.Fprintf(&b, " (temas: %s)", strings.Join(ep.Topics, ", "))
		}
		anchors := evidenceByEpisode[ep.ID]
		turns := relatedHistoricalTurnsForDetail(query, ep, anchors, intent)
		for _, turn := range turns {
			if strings.TrimSpace(turn.UserText) != "" {
				fmt.Fprintf(&b, "\n   %s — Tú: %s", formatTurnTime(turn.At), strings.TrimSpace(turn.UserText))
			}
			if strings.TrimSpace(turn.Assistant) != "" {
				fmt.Fprintf(&b, "\n   %s — Ahmelia: %s", formatTurnTime(turn.At), strings.TrimSpace(turn.Assistant))
			}
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func relatedHistoricalTurnsForDetail(query string, ep *ConversationEpisode, anchors []historicalEvidenceCandidate, intent HistoricalIntent) []HistoricalTurn {
	if ep == nil {
		return nil
	}
	selected := map[uint64]bool{}
	anchorTopics, anchorObjectives := map[string]bool{}, map[string]bool{}
	for _, a := range anchors {
		if a.Turn == nil {
			continue
		}
		selected[a.Turn.ID] = true
		if a.Turn.Topic != "" {
			anchorTopics[normalizeMemoryText(a.Turn.Topic)] = true
		}
		if a.Turn.Objective != "" {
			anchorObjectives[normalizeMemoryText(a.Turn.Objective)] = true
		}
	}
	qNorm, qTokens := normalizeMemoryText(query), semanticTokens(query)
	for i := range ep.Turns {
		t := &ep.Turns[i]
		if isHistoricalQuestion(t.UserText) && intent != HistoricalIntentTimeline {
			continue
		}
		if relatedText(qNorm, t.Topic) || relatedText(qNorm, t.Objective) {
			selected[t.ID] = true
			continue
		}
		if anchorTopics[normalizeMemoryText(t.Topic)] || anchorObjectives[normalizeMemoryText(t.Objective)] {
			selected[t.ID] = true
			continue
		}
		// Keep nearby conversational context only when it is informative; this
		// is a bounded relation rule, not an unconditional ±2 turn dump.
		if turnHasInformativeEvidence(t) {
			overlap := evidenceSemanticScore(qNorm, qTokens, ep, t, t.UserText)
			if overlap >= 0.20 || hasStructuredHistoricalRelation(t) {
				selected[t.ID] = true
			}
		}
	}
	out := make([]HistoricalTurn, 0, len(selected))
	for _, t := range ep.Turns {
		if selected[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

func hasStructuredHistoricalRelation(t *HistoricalTurn) bool {
	if t == nil {
		return false
	}
	m := normalizeMemoryText(t.Kind + " " + t.SpeechAct + " " + t.Relation)
	return containsAny(m, "decision", "problem", "solution", "pending", "preference", "objective", "change")
}

func turnHasInformativeEvidence(t *HistoricalTurn) bool {
	for _, p := range turnEvidenceParts(t) {
		if p.Provenance == EvidencePrimary || (p.Provenance == EvidenceSecondary && p.Kind != EvidenceAnswer) {
			return true
		}
	}
	return false
}

func isHistoricalQuestion(text string) bool {
	n := strings.TrimSpace(text)
	return n != "" && (strings.Contains(n, "?") || strings.Contains(n, "¿"))
}

func isGenericHistoricalAck(text string) bool {
	n := normalizeMemoryText(text)
	generic := []string{"si", "correcto", "de acuerdo", "exacto", "esa fue la decision", "esa era la decision", "queda registrado", "quedo registrado"}
	for _, phrase := range generic {
		if n == normalizeMemoryText(phrase) {
			return true
		}
	}
	return false
}

func formatTurnTime(t time.Time) string { return formatEpisodeTime(t) }

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(s, normalizeMemoryText(value)) {
			return true
		}
	}
	return false
}
