package sanitize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeString keeps the letters and digits of text, lowercased and with diacritics folded like
// Fold, with words separated by a single space and no spaces around the result:
//   - ASCII whitespace and the non-ASCII characters that are no letter or digit separate words
//   - other ASCII characters are removed, so "e-mail" becomes "email"
//   - letters and digits without an ASCII fold, e.g. Cyrillic or Devanagari, are lowercased and kept
//     with their marks; only a combining mark after a Latin letter is folded away
func NormalizeString(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	pendingSpace := false
	// Outside Latin text a combining mark is often a vowel sign, dropping it would make different words equal.
	afterUnfolded := false
	writeSpace := func() {
		if pendingSpace {
			builder.WriteByte(' ')
			pendingSpace = false
		}
	}
	for idx := 0; idx < len(text); {
		ch := text[idx]
		if ch < utf8.RuneSelf {
			idx++
			afterUnfolded = false
			if 'A' <= ch && ch <= 'Z' {
				ch += 'a' - 'A'
			}
			switch {
			case isWordByte(ch):
				writeSpace()
				builder.WriteByte(ch)
			case ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t':
				pendingSpace = builder.Len() > 0
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(text[idx:])
		idx += size
		out, vanish, mapped := foldRune(r)
		switch {
		case mapped:
			writeSpace()
			builder.WriteString(out)
			afterUnfolded = false
		case vanish:
			if afterUnfolded && unicode.Is(unicode.Mn, r) {
				builder.WriteRune(r)
			}
		case unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			writeSpace()
			builder.WriteRune(unicode.ToLower(r))
			afterUnfolded = true
		default:
			pendingSpace = builder.Len() > 0
			afterUnfolded = false
		}
	}
	return builder.String()
}
