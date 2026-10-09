package sanitize

import (
	"slices"
	"testing"
)

func TestWordAndNumberTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single word",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "single number",
			input:    "123",
			expected: []string{"123"},
		},
		{
			name:     "word and number combined",
			input:    "hello123",
			expected: []string{"hello123"},
		},
		{
			name:     "email address",
			input:    "foo123@bar.com",
			expected: []string{"foo123", "bar", "com"},
		},
		{
			name:     "multiple words separated by spaces",
			input:    "hello world 123",
			expected: []string{"hello", "world", "123"},
		},
		{
			name:     "words separated by special characters",
			input:    "hello-world_test",
			expected: []string{"hello", "world", "test"},
		},
		{
			name:     "mixed separators",
			input:    "user@domain.com:8080",
			expected: []string{"user", "domain", "com", "8080"},
		},
		{
			name:     "only special characters",
			input:    "@#$%^&*()",
			expected: []string{},
		},
		{
			name:     "leading special characters",
			input:    "!!!hello",
			expected: []string{"hello"},
		},
		{
			name:     "trailing special characters",
			input:    "hello!!!",
			expected: []string{"hello"},
		},
		{
			name:     "multiple consecutive special characters",
			input:    "hello@@@world",
			expected: []string{"hello", "world"},
		},
		{
			name:     "unicode letters",
			input:    "café123",
			expected: []string{"café123"},
		},
		{
			name:     "mixed case with numbers",
			input:    "Hello123World456",
			expected: []string{"Hello123World456"},
		},
		{
			name:     "path-like string",
			input:    "/path/to/file123.txt",
			expected: []string{"path", "to", "file123", "txt"},
		},
		{
			name:     "URL-like string",
			input:    "https://example.com/page?id=123",
			expected: []string{"https", "example", "com", "page", "id", "123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WordAndNumberTokens(tt.input)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("WordAndNumberTokens(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
