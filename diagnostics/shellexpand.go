package diagnostics

import (
	"errors"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// shellEnvRef describes a shell parameter reference.
type shellEnvRef struct {
	name       string // Variable name.
	hasDefault bool   // True if expansion has default fallback.
	required   bool   // True if expansion is required.
}

// parseShellWord parses shell-word to AST. Returns nil if empty.
func parseShellWord(s string) *syntax.Word {
	p := syntax.NewParser()
	for w, _ := range p.WordsSeq(strings.NewReader(s)) {
		return w
	}
	return nil
}

// extractShellEnvRefs parses parameter references from s. See ADR-0003.
func extractShellEnvRefs(s string) []shellEnvRef {
	word := parseShellWord(s)
	if word == nil {
		return nil
	}

	var refs []shellEnvRef
	seen := make(map[string]bool)

	syntax.Walk(word, func(node syntax.Node) bool {
		pe, ok := node.(*syntax.ParamExp)
		if !ok {
			return true
		}
		name := pe.Param.Value
		if name == "" || seen[name] {
			return true
		}

		// Skip special parameters.
		if len(name) == 1 && !isAlphaNum(name[0]) {
			return true
		}

		seen[name] = true

		ref := shellEnvRef{name: name}
		if pe.Exp != nil {
			switch pe.Exp.Op {
			case syntax.DefaultUnset, syntax.DefaultUnsetOrNull:
				ref.hasDefault = true
			case syntax.ErrorUnset, syntax.ErrorUnsetOrNull:
				ref.required = true
			}
		}
		refs = append(refs, ref)
		return true
	})

	return refs
}

func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// resolveShellExpr expands parameter expressions in s using env and host env. See ADR-0003.
func resolveShellExpr(s string, env map[string]string) string {
	if !strings.Contains(s, "$") {
		return s
	}

	word := parseShellWord(s)
	if word == nil {
		return s
	}

	cfg := &expand.Config{
		Env: expand.FuncEnviron(func(name string) string {
			// System env takes precedence over .env map. See ADR-0003.
			if v, ok := os.LookupEnv(name); ok {
				return v
			}
			return env[name]
		}),
	}

	result, err := expand.Literal(cfg, word)
	if err != nil {
		// Return empty string on unset/empty required variable. See ADR-0003.
		var unsetErr expand.UnsetParameterError
		if errors.As(err, &unsetErr) {
			return ""
		}
		return s
	}

	return result
}

// ResolveShellExpr wraps resolveShellExpr.
func ResolveShellExpr(s string, env map[string]string) string {
	return resolveShellExpr(s, env)
}

