package diagnostics

import (
	"errors"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// shellEnvRef describes a shell parameter reference found in a string expression.
type shellEnvRef struct {
	// name is the variable name (e.g. "DB_PASSWORD" in ${DB_PASSWORD:-secret}).
	name string
	// hasDefault is true when the expansion carries a fallback value
	// (${VAR:-default} or ${VAR-default}) so a missing or empty variable is
	// not necessarily fatal.
	hasDefault bool
	// required is true when the expansion uses the :? operator
	// (${VAR:?error}) meaning the variable must be set and non-empty.
	required bool
}

// parseShellWord parses a single shell-word string into a *syntax.Word.
// Returns nil if the string contains no parseable words.
func parseShellWord(s string) *syntax.Word {
	p := syntax.NewParser()
	var result *syntax.Word
	_ = p.Words(strings.NewReader(s), func(w *syntax.Word) bool {
		result = w
		return false // stop after first word
	})
	return result
}

// extractShellEnvRefs parses a shell word string and returns all parameter
// expansion references found inside it. It recognises ${VAR}, $VAR,
// ${VAR:-default}, and ${VAR:?error} forms using mvdan.cc/sh/v3/syntax.
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

		// Skip special shell parameters ($0, $#, $?, etc.)
		if len(name) == 1 && !isAlphaNum(name[0]) {
			return true
		}

		seen[name] = true

		ref := shellEnvRef{name: name}
		if pe.Exp != nil {
			switch pe.Exp.Op {
			case syntax.DefaultUnset, syntax.DefaultUnsetOrNull:
				// ${VAR-default} or ${VAR:-default}: has a fallback
				ref.hasDefault = true
			case syntax.ErrorUnset, syntax.ErrorUnsetOrNull:
				// ${VAR?error} or ${VAR:?error}: required, no default
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

// resolveShellExpr expands shell parameter expressions in s using the provided
// env map and the host OS environment. It supports ${VAR}, $VAR, ${VAR:-default},
// and ${VAR:?error} via mvdan.cc/sh/v3/expand. On any parse or expansion error
// the original string s is returned unchanged.
//
// Note: expand.FuncEnviron treats empty-string returns as "unset", which means
// ${VAR:-default} correctly falls back when VAR is empty — matching the shell
// :- (unset-or-empty) semantics.
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
			// System environment takes precedence over the .env map.
			// expand.FuncEnviron treats "" as "unset", so variables defined
			// but empty in os.Environ will correctly trigger :- fallbacks.
			if v, ok := os.LookupEnv(name); ok {
				return v
			}
			return env[name]
		}),
	}

	result, err := expand.Literal(cfg, word)
	if err != nil {
		// ${VAR:?message} with an unset/empty var raises UnsetParameterError —
		// return empty string so callers treat it as unresolved.
		var unsetErr expand.UnsetParameterError
		if errors.As(err, &unsetErr) {
			return ""
		}
		return s
	}

	return result
}
