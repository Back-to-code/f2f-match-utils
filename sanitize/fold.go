package sanitize

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// foldRune reports what one non-ASCII rune becomes when folded: word characters (mapped), nothing
// (vanish, e.g. combining marks and typographic apostrophes), or neither, which is a word separator.
func foldRune(r rune) (out string, vanish bool, mapped bool) {
	text, ok := foldMapped[r]
	if ok {
		return text, false, true
	}
	idx := sort.Search(len(foldVanishRanges), func(idx int) bool { return foldVanishRanges[idx][1] >= r })
	if idx < len(foldVanishRanges) && foldVanishRanges[idx][0] <= r {
		return "", true, false
	}
	return "", false, false
}

func isWordByte(ch byte) bool {
	return ('a' <= ch && ch <= 'z') || ('0' <= ch && ch <= '9')
}

func isDroppedASCII(ch byte) bool {
	return ch == '.' || ch == '\'' || ch == '`'
}

// Fold lowercases text, removes diacritics (é → e, ß → ss, ø → o) and drops '.' and apostrophes.
// Everything else is kept, so a folded word is all a-z0-9 exactly when each of its characters folds to
// letters or digits.
func Fold(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for idx := 0; idx < len(text); {
		ch := text[idx]
		if ch < utf8.RuneSelf {
			idx++
			if isDroppedASCII(ch) {
				continue
			}
			if 'A' <= ch && ch <= 'Z' {
				ch += 'a' - 'A'
			}
			builder.WriteByte(ch)
			continue
		}
		r, size := utf8.DecodeRuneInString(text[idx:])
		idx += size
		out, vanish, mapped := foldRune(r)
		switch {
		case mapped:
			builder.WriteString(out)
		case vanish:
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// AppendWords appends the words of text to buf and the start and end offset in buf of each word to
// spans. A word is a run of a-z0-9 after Fold: the characters Fold drops do not end one, every other
// character does.
func AppendWords(buf []byte, spans []int, text string) ([]byte, []int) {
	start := -1
	closeWord := func() {
		if start >= 0 {
			spans = append(spans, start, len(buf))
			start = -1
		}
	}
	for idx := 0; idx < len(text); {
		ch := text[idx]
		if ch < utf8.RuneSelf {
			idx++
			if 'A' <= ch && ch <= 'Z' {
				ch += 'a' - 'A'
			}
			if isWordByte(ch) {
				if start < 0 {
					start = len(buf)
				}
				buf = append(buf, ch)
				continue
			}
			if !isDroppedASCII(ch) {
				closeWord()
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(text[idx:])
		idx += size
		out, vanish, mapped := foldRune(r)
		switch {
		case mapped:
			if start < 0 {
				start = len(buf)
			}
			buf = append(buf, out...)
		case vanish:
		default:
			closeWord()
		}
	}
	closeWord()
	return buf, spans
}

// SplitWords returns the words of AppendWords in order.
func SplitWords(text string) []string {
	buf, spans := AppendWords(nil, nil, text)
	joined := string(buf)
	words := make([]string, 0, len(spans)/2)
	for idx := range len(spans) / 2 {
		words = append(words, joined[spans[2*idx]:spans[2*idx+1]])
	}
	return words
}

// WordKey returns the words of SplitWords joined by single spaces, in one pass. Texts with the same
// key have the same words, so no rule tells them apart.
func WordKey(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	inWord := false
	startWord := func() {
		if !inWord {
			if builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			inWord = true
		}
	}
	for idx := 0; idx < len(text); {
		ch := text[idx]
		if ch < utf8.RuneSelf {
			idx++
			if 'A' <= ch && ch <= 'Z' {
				ch += 'a' - 'A'
			}
			if isWordByte(ch) {
				startWord()
				builder.WriteByte(ch)
			} else if !isDroppedASCII(ch) {
				inWord = false
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(text[idx:])
		idx += size
		out, vanish, mapped := foldRune(r)
		switch {
		case mapped:
			startWord()
			builder.WriteString(out)
		case vanish:
		default:
			inWord = false
		}
	}
	return builder.String()
}
