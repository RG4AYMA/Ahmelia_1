package capability

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"ahmelia/interaction"
)

// DiscoveryResult describes how strongly a registered Capability matches a
// natural-language objective. Discovery is advisory: it never authorizes or
// executes a Capability.
type DiscoveryResult struct {
	Capability Capability
	Score      int
	Matched    []string
}

// Discovery provides deterministic discovery over the Capability Registry.
// It deliberately uses metadata already present in Capability rather than a
// second hardcoded list of goals. A future semantic/LLM discoverer can feed
// the same Registry without changing the Capability contract.
type Discovery struct {
	registry *Registry
}

// NewDiscovery creates a discovery service over a Capability Registry.
func NewDiscovery(registry *Registry) *Discovery {
	return &Discovery{registry: registry}
}

// Discover returns registered capabilities relevant to query, ranked by a
// deterministic metadata score. Empty queries fail closed rather than
// returning every capability as a false positive.
func (d *Discovery) Discover(query string) ([]DiscoveryResult, error) {
	if d == nil || d.registry == nil {
		return nil, fmt.Errorf("capability discovery: registry nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("capability discovery: query vacío")
	}

	return d.discoverTokens(expandSemanticTokens(tokenize(query)))
}

// DiscoverSemantic matches capabilities using the structured semantic
// contract. It deliberately excludes Raw/Normalized and therefore cannot
// become a second language classifier. The registry metadata remains the only
// candidate-matching surface at this layer.
func (d *Discovery) DiscoverSemantic(req interaction.SemanticRequest) ([]DiscoveryResult, error) {
	if d == nil || d.registry == nil {
		return nil, fmt.Errorf("capability discovery: registry nil")
	}
	tokens := make([]string, 0, 16)
	appendTokens := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		tokens = append(tokens, tokenize(value)...)
	}
	appendTokens(req.Action.Name)
	appendTokens(req.Topic)
	appendTokens(req.ExplicitDomain)
	appendTokens(req.RequestedAction)
	appendTokens(string(req.Kind))
	for _, entity := range req.Entities {
		appendTokens(entity)
	}
	for _, rel := range req.Relations {
		appendTokens(rel.Predicate)
		appendTokens(rel.Target)
		appendTokens(rel.Value)
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("capability discovery: semantic request sin términos")
	}
	tokens = expandSemanticTokens(uniqueTokens(tokens))
	return d.discoverTokens(tokens)
}

func uniqueTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func (d *Discovery) discoverTokens(queryTokens []string) ([]DiscoveryResult, error) {
	if d == nil || d.registry == nil {
		return nil, fmt.Errorf("capability discovery: registry nil")
	}
	if len(queryTokens) == 0 {
		return nil, fmt.Errorf("capability discovery: query sin términos")
	}
	results := make([]DiscoveryResult, 0)
	for _, c := range d.registry.List() {
		fields := []string{string(c.ID), string(c.Domain), c.Action, c.Description}
		for _, p := range c.Inputs {
			fields = append(fields, p.Name, p.Description)
		}
		for _, p := range c.Outputs {
			fields = append(fields, p.Name, p.Description)
		}
		fieldSet := make(map[string]struct{})
		for _, token := range tokenize(strings.Join(fields, " ")) {
			fieldSet[token] = struct{}{}
		}
		score := 0
		matched := make([]string, 0)
		seen := make(map[string]struct{})
		for _, token := range queryTokens {
			if _, ok := fieldSet[token]; !ok {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			matched = append(matched, token)
			score++
		}
		if score == 0 {
			continue
		}
		results = append(results, DiscoveryResult{Capability: c, Score: score, Matched: matched})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Capability.ID < results[j].Capability.ID
	})
	return results, nil
}

// Best returns the highest-ranked discovered Capability. Ties are resolved
// by Capability ID, so discovery never depends on map iteration order.
func (d *Discovery) Best(query string) (DiscoveryResult, bool, error) {
	results, err := d.Discover(query)
	if err != nil {
		return DiscoveryResult{}, false, err
	}
	if len(results) == 0 {
		return DiscoveryResult{}, false, nil
	}
	return results[0], true, nil
}

func expandSemanticTokens(tokens []string) []string {
	out := append([]string(nil), tokens...)
	equivalents := map[string]string{
		// Stable semantic/action vocabulary. These are token aliases for
		// candidate matching only; they never select the execution lane.
		"create": "crear", "crea": "crear", "creating": "crear",
		"modify": "modificar", "modify_code": "modificar", "modifica": "modificar",
		"build": "compilar", "compile": "compilar",
		"run": "ejecutar", "execute": "ejecutar",
		"debug":     "corregir",
		"search":    "buscar",
		"construct": "construir", "construye": "construir",
		"generate": "generar", "genera": "generar",
		"implement": "implementar", "implementa": "implementar",
		"prepare": "preparar", "prepara": "preparar",
		"corrige": "corregir", "termina": "terminar", "avanza": "avanzar",
		"avances": "avanzar", "haz": "hacer", "anade": "añadir",
	}
	seen := make(map[string]bool, len(out))
	for _, t := range out {
		seen[t] = true
	}
	for _, t := range tokens {
		if eq, ok := equivalents[t]; ok && !seen[eq] {
			out = append(out, eq)
			seen[eq] = true
		}
	}
	return out
}

func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	})
}
