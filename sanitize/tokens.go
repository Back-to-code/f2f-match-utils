package sanitize

import (
	"strings"
	"unicode"
)

// WordAndNumberTokens splits a string into tokens containing only words and numbers.
// For example "foo123@bar.com" will be split into ["foo123", "bar", "com"]
func WordAndNumberTokens(input string) []string {
	return strings.FieldsFunc(input, func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	})
}
