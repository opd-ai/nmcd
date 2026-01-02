package server

import (
	"testing"
)

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single value",
			input:    "peer1:8334",
			expected: []string{"peer1:8334"},
		},
		{
			name:     "multiple values",
			input:    "peer1:8334,peer2:8334,peer3:8334",
			expected: []string{"peer1:8334", "peer2:8334", "peer3:8334"},
		},
		{
			name:     "values with spaces",
			input:    "peer1:8334, peer2:8334 , peer3:8334",
			expected: []string{"peer1:8334", "peer2:8334", "peer3:8334"},
		},
		{
			name:     "empty value filtered",
			input:    "peer1:8334,,peer2:8334",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
		{
			name:     "only whitespace filtered",
			input:    "peer1:8334,   ,peer2:8334",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "trailing comma",
			input:    "peer1:8334,peer2:8334,",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
		{
			name:     "leading comma",
			input:    ",peer1:8334,peer2:8334",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitAndTrim(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d elements, got %d", len(tt.expected), len(result))
				return
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("element %d: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}
