package diagnostics

import (
	"reflect"
	"testing"
)

func TestResolveShellExpr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		env      map[string]string
		expected string
	}{
		{
			name:     "no variables",
			input:    "/some/path/without/vars",
			env:      map[string]string{},
			expected: "/some/path/without/vars",
		},
		{
			name:     "single variable",
			input:    "/path/to/${VAR}",
			env:      map[string]string{"VAR": "foo"},
			expected: "/path/to/foo",
		},
		{
			name:     "multiple variables with spaces",
			input:    "/path with spaces/${VAR1} and ${VAR2}",
			env:      map[string]string{"VAR1": "foo", "VAR2": "bar"},
			expected: "/path with spaces/foo and bar",
		},
		{
			name:     "variable with default fallback",
			input:    "/path/${VAR:-default}",
			env:      map[string]string{},
			expected: "/path/default",
		},
		{
			name:     "required variable present",
			input:    "/path/${VAR:?error}",
			env:      map[string]string{"VAR": "val"},
			expected: "/path/val",
		},
		{
			name:     "required variable empty/unset",
			input:    "/path/${VAR:?error}",
			env:      map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveShellExpr(tt.input, tt.env)
			if got != tt.expected {
				t.Errorf("ResolveShellExpr() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestExtractShellEnvRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []shellEnvRef
	}{
		{
			name:     "no variables",
			input:    "python -u app.py",
			expected: nil,
		},
		{
			name:  "single variable in multi-word",
			input: "python -u ${SCRIPT_NAME}",
			expected: []shellEnvRef{
				{name: "SCRIPT_NAME"},
			},
		},
		{
			name:  "multiple variables in multi-word command",
			input: "${ENV_BIN} -m ${ENV_MODULE} --port ${PORT:?error}",
			expected: []shellEnvRef{
				{name: "ENV_BIN"},
				{name: "ENV_MODULE"},
				{name: "PORT", required: true},
			},
		},
		{
			name:  "variables with default values",
			input: "sh -c echo ${MESSAGE:-hello} from ${SENDER:-admin}",
			expected: []shellEnvRef{
				{name: "MESSAGE", hasDefault: true},
				{name: "SENDER", hasDefault: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractShellEnvRefs(tt.input)
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("extractShellEnvRefs() = %+v, expected %+v", got, tt.expected)
			}
		})
	}
}

func BenchmarkResolveShellExpr(b *testing.B) {
	env := map[string]string{"VAR1": "foo", "VAR2": "bar"}
	input := "/path with spaces/${VAR1} and ${VAR2}"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ResolveShellExpr(input, env)
	}
}
