package interaction

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// SolveQuestion handles deterministic question types that Ahmelia can answer
// without delegating cognition to the linguistic model.
func SolveQuestion(r SemanticRequest) (string, bool) {
	if answer, ok := solveArithmetic(r.Raw); ok {
		return answer, true
	}
	return "", false
}

func solveArithmetic(raw string) (string, bool) {
	s := strings.TrimSpace(strings.ToLower(raw))
	for _, p := range []string{"¿cuánto es", "cuánto es", "cuanto es", "calcula", "calcular", "resuelve", "resolver"} {
		s = strings.TrimSpace(strings.TrimPrefix(s, p))
	}
	s = strings.Trim(s, " ?¿.")
	if s == "" || !containsArithmeticOperator(s) {
		return "", false
	}
	// Only arithmetic characters are accepted. This is deliberately strict:
	// a linguistic sentence must never be evaluated as an expression.
	for _, r := range s {
		if !(unicode.IsDigit(r) || strings.ContainsRune("+-*/(). ", r)) {
			return "", false
		}
	}
	v, pos, err := parseExpr(s, 0)
	if err != nil || strings.TrimSpace(s[pos:]) != "" {
		return "", false
	}
	return formatNumber(v), true
}

func containsArithmeticOperator(s string) bool {
	return strings.ContainsAny(s, "+-*/")
}

func parseExpr(s string, pos int) (float64, int, error) {
	left, pos, err := parseTerm(s, pos)
	if err != nil {
		return 0, pos, err
	}
	for {
		pos = skipSpaces(s, pos)
		if pos >= len(s) || (s[pos] != '+' && s[pos] != '-') {
			return left, pos, nil
		}
		op := s[pos]
		pos++
		right, next, err := parseTerm(s, pos)
		if err != nil {
			return 0, next, err
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
		pos = next
	}
}

func parseTerm(s string, pos int) (float64, int, error) {
	left, pos, err := parseFactor(s, pos)
	if err != nil {
		return 0, pos, err
	}
	for {
		pos = skipSpaces(s, pos)
		if pos >= len(s) || (s[pos] != '*' && s[pos] != '/') {
			return left, pos, nil
		}
		op := s[pos]
		pos++
		right, next, err := parseFactor(s, pos)
		if err != nil {
			return 0, next, err
		}
		if op == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, next, fmt.Errorf("division by zero")
			}
			left /= right
		}
		pos = next
	}
}

func parseFactor(s string, pos int) (float64, int, error) {
	pos = skipSpaces(s, pos)
	if pos >= len(s) {
		return 0, pos, fmt.Errorf("missing operand")
	}
	if s[pos] == '+' || s[pos] == '-' {
		sign := 1.0
		if s[pos] == '-' {
			sign = -1
		}
		v, next, err := parseFactor(s, pos+1)
		return sign * v, next, err
	}
	if s[pos] == '(' {
		v, next, err := parseExpr(s, pos+1)
		if err != nil {
			return 0, next, err
		}
		next = skipSpaces(s, next)
		if next >= len(s) || s[next] != ')' {
			return 0, next, fmt.Errorf("missing closing parenthesis")
		}
		return v, next + 1, nil
	}
	start := pos
	dots := 0
	for pos < len(s) && (unicode.IsDigit(rune(s[pos])) || s[pos] == '.') {
		if s[pos] == '.' {
			dots++
		}
		if dots > 1 {
			return 0, pos, fmt.Errorf("invalid number")
		}
		pos++
	}
	if start == pos {
		return 0, pos, fmt.Errorf("expected number")
	}
	v, err := strconv.ParseFloat(s[start:pos], 64)
	if err != nil {
		return 0, pos, err
	}
	return v, pos, nil
}

func skipSpaces(s string, pos int) int {
	for pos < len(s) && unicode.IsSpace(rune(s[pos])) {
		pos++
	}
	return pos
}

func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
