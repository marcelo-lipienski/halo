//go:build !windows

package doctor

import "testing"

func TestParseMeminfoBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
		wantErr  bool
	}{
		{
			name:     "valid meminfo",
			input:    "MemTotal:       16384000 kB\nMemFree:         8000000 kB\n",
			expected: 16384000 * 1024,
			wantErr:  false,
		},
		{
			name:     "invalid number",
			input:    "MemTotal:       invalid kB\n",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "missing MemTotal",
			input:    "MemFree:         8000000 kB\n",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "malformed MemTotal line",
			input:    "MemTotal:\n",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMeminfoBytes([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Errorf("parseMeminfoBytes() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if got != tc.expected {
				t.Errorf("parseMeminfoBytes() = %d, expected %d", got, tc.expected)
			}
		})
	}
}
