package programming

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// B96.11 — Error Analysis
//
// Converts compiler/test/runtime output into structured programming-domain
// diagnostics. It does not patch code, retry execution, or decide recovery;
// those authorities remain in Production/B95.

type ErrorAnalysisRequest struct {
	ProjectID string
	Language  string
	Output    string
}

func (r ErrorAnalysisRequest) Validate() error {
	if strings.TrimSpace(r.ProjectID) == "" {
		return fmt.Errorf("programming error analysis: ProjectID vacío")
	}
	if strings.TrimSpace(r.Output) == "" {
		return fmt.Errorf("programming error analysis: Output vacío")
	}
	return nil
}

type ErrorAnalyzer interface {
	Analyze(ErrorAnalysisRequest) []Diagnostic
}

type ErrorAnalysis struct {
	ProjectID   string
	Diagnostics []Diagnostic
}

// AnalyzeErrors is the domain entry point. The parser is selected by language,
// with a conservative generic parser as fallback.
func AnalyzeErrors(req ErrorAnalysisRequest) (ErrorAnalysis, error) {
	if err := req.Validate(); err != nil {
		return ErrorAnalysis{}, err
	}
	var diagnostics []Diagnostic
	if strings.EqualFold(req.Language, "go") || strings.EqualFold(req.Language, "golang") {
		diagnostics = parseGoDiagnostics(req)
	} else {
		diagnostics = parseGenericDiagnostics(req)
	}
	return ErrorAnalysis{ProjectID: req.ProjectID, Diagnostics: diagnostics}, nil
}

var locationPattern = regexp.MustCompile(`^(.*?):([0-9]+):([0-9]+):\s*(.+)$`)
var locationNoColumnPattern = regexp.MustCompile(`^(.*?):([0-9]+):\s*(.+)$`)

func parseGoDiagnostics(req ErrorAnalysisRequest) []Diagnostic {
	return parseDiagnostics(req, true)
}

func parseGenericDiagnostics(req ErrorAnalysisRequest) []Diagnostic {
	return parseDiagnostics(req, false)
}

func parseDiagnostics(req ErrorAnalysisRequest, goStyle bool) []Diagnostic {
	var out []Diagnostic
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(strings.NewReader(req.Output))
	index := 0
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		file, line, col, msg, ok := parseLocation(raw, goStyle)
		if !ok {
			continue
		}
		if file == "" || msg == "" {
			continue
		}
		key := file + "|" + strconv.Itoa(line) + "|" + strconv.Itoa(col) + "|" + msg
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		index++
		severity := SeverityError
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "warning") || strings.Contains(lower, "advertencia") {
			severity = SeverityWarning
		}
		out = append(out, Diagnostic{
			ID:        fmt.Sprintf("diagnostic-%03d", index),
			ProjectID: req.ProjectID,
			File:      filepath.Clean(file),
			Line:      line,
			Column:    col,
			ErrorType: classifyErrorType(msg),
			Message:   msg,
			Severity:  severity,
		})
	}
	return out
}

func parseLocation(line string, goStyle bool) (string, int, int, string, bool) {
	if m := locationPattern.FindStringSubmatch(line); len(m) == 5 {
		ln, err1 := strconv.Atoi(m[2])
		col, err2 := strconv.Atoi(m[3])
		if err1 == nil && err2 == nil && ln > 0 && col > 0 {
			return m[1], ln, col, strings.TrimSpace(m[4]), true
		}
	}
	if goStyle {
		if m := locationNoColumnPattern.FindStringSubmatch(line); len(m) == 4 {
			ln, err := strconv.Atoi(m[2])
			if err == nil && ln > 0 {
				return m[1], ln, 0, strings.TrimSpace(m[3]), true
			}
		}
	}
	return "", 0, 0, "", false
}

func classifyErrorType(message string) string {
	m := strings.ToLower(message)
	switch {
	case strings.Contains(m, "imported and not used"), strings.Contains(m, "imported but not used"):
		// This is a source-level compile error, not a missing dependency. Keep it
		// distinct so recovery can remove/use the import instead of changing module metadata.
		return "unused_import"
	case strings.Contains(m, "undefined"), strings.Contains(m, "not defined"):
		return "undefined_symbol"
	case strings.Contains(m, "syntax error"), strings.Contains(m, "expected"):
		return "syntax_error"
	case strings.Contains(m, "cannot use"), strings.Contains(m, "type mismatch"), strings.Contains(m, "incompatible types"):
		return "type_error"
	case strings.Contains(m, "no required module provides package"), strings.Contains(m, "missing go.sum entry"), strings.Contains(m, "cannot find module"), strings.Contains(m, "module lookup disabled"), strings.Contains(m, "package not in std"), strings.Contains(m, "is not in std"):
		return "dependency_error"
	case strings.Contains(m, "permission"), strings.Contains(m, "access denied"):
		return "permission_error"
	case strings.Contains(m, "panic"), strings.Contains(m, "runtime error"):
		return "runtime_error"
	default:
		return "compiler_error"
	}
}
