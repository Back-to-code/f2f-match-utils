package sanitize

import (
	"testing"
)

func TestNormalizeString(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		expect string
	}{
		{"normalized inputs should not change", "abc", "abc"},
		{"should be converted to lowercase", "ABC", "abc"},
		{"spaces should be trimmed", "  ABC  ", "abc"},
		{"duplicated spaces should reduced to 1 space", "A  B   C", "a b c"},
		{"new line and tab characters should be replace by a space", "A\nB\tC", "a b c"},
		{"special characters should be removed", "a+b-c(d)", "abcd"},
		{"removed characters between spaces leave one space", "a - b", "a b"},
		{"diacritics should be folded", "Coördinator Straße Æble", "coordinator strasse aeble"},
		{"apostrophes should be dropped", "it’s it's", "its its"},
		{"combining marks after a Latin letter should be dropped", "cafe\u0301", "cafe"},
		{"combining marks after other letters should be kept", "हिंदी संस्कृतम्", "हिंदी संस्कृतम्"},
		{"ligatures should be folded", "Ĳsselmonde ﬁnancieel", "ijsselmonde financieel"},
		{"non-ASCII punctuation should separate words", "a–b «c»", "a b c"},
		{"letters without an ASCII fold should be lowercased and kept", "Русский", "русский"},
		{"spacing marks should stay in their word", "खाना", "खाना"},
		{"empty string should return empty string", "", ""},
		{"copyright symbol should be stripped", "©hinese", "hinese"},
		{"registered trademark symbol should be stripped", "®egistered", "egistered"},

		// Non-breaking space (0x00A0) tests
		{"non-breaking space should be converted to regular space", "a\u00A0b", "a b"},
		{"non-breaking space should be trimmed at start", "\u00A0abc", "abc"},
		{"non-breaking space should be trimmed at end", "abc\u00A0", "abc"},
		{"multiple non-breaking spaces should be deduplicated", "a\u00A0\u00A0b", "a b"},

		// Unicode whitespace deduplication tests
		{"em space adjacent to regular space should be deduplicated", "a \u2003b", "a b"},
		{"regular space adjacent to em space should be deduplicated", "a\u2003 b", "a b"},
		{"non-breaking space adjacent to regular space should be deduplicated", "a \u00A0b", "a b"},
		{"mixed unicode spaces should be deduplicated", "a\u00A0\u2003\u2002b", "a b"},
		{"non-breaking space adjacent to tab should be deduplicated", "a\t\u00A0b", "a b"},
		{"en space adjacent to newline should be deduplicated", "a\n\u2002b", "a b"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized := NormalizeString(testCase.input)
			if normalized != testCase.expect {
				t.Errorf("NormalizeString(%q) = %q, want %q", testCase.input, normalized, testCase.expect)
			}
		})
	}
}
