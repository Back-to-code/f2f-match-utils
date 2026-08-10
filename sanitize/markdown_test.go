package sanitize

import (
	"testing"
)

func TestEscapeMarkdown(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty string", "", ""},
		{"plain text", "hello world", "hello world"},
		{"bold asterisks", "**bold**", "\\*\\*bold\\*\\*"},
		{"italic underscores", "_italic_", "\\_italic\\_"},
		{"link syntax", "[text](url)", "\\[text\\]\\(url\\)"},
		{"heading", "# heading", "\\# heading"},
		{"blockquote", "> quote", "\\> quote"},
		{"backslash", `path\to\file`, `path\\to\\file`},
		{"backtick", "`code`", "\\`code\\`"},
		{"pipe", "col1 | col2", "col1 \\| col2"},
		{"exclamation image", "![alt](img)", "\\!\\[alt\\]\\(img\\)"},
		{"html angle brackets", "<div>", "\\<div\\>"},
		{"mixed special chars", "**Error**: `Owner.DepartmentId` does not exist", "\\*\\*Error\\*\\*: \\`Owner.DepartmentId\\` does not exist"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			escaped := EscapeMarkdown(testCase.input)
			if escaped != testCase.expect {
				t.Errorf("EscapeMarkdown(%q) = %q, want %q", testCase.input, escaped, testCase.expect)
			}
		})
	}
}
