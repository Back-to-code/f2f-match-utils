package rules

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// foldRune reports what one non-ASCII rune becomes when folded: word characters (mapped), nothing
// (vanish, e.g. combining marks and typographic apostrophes), or a word separator.
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
// Everything else is kept, so a folded word is valid in a rule exactly when it is all a-z0-9.
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

// Normalizer turns texts into word sets. It reuses its buffers between calls, so it is not safe for
// concurrent use; the zero value is ready to use.
type Normalizer struct {
	buf   []byte
	spans []int
	words []string
	// RealWords is how many of the last Words result are real words; the joined pairs follow them.
	RealWords int
}

// SplitWords returns the folded words of text in order: runs of a-z0-9 after Fold.
func SplitWords(text string) []string {
	var normalizer Normalizer
	normalizer.split(text)
	joined := string(normalizer.buf)
	words := make([]string, 0, len(normalizer.spans)/2)
	for idx := range len(normalizer.spans) / 2 {
		words = append(words, joined[normalizer.spans[2*idx]:normalizer.spans[2*idx+1]])
	}
	return words
}

// Words returns the word set a rule is matched against: the words of SplitWords plus every two
// adjacent words joined into one, without duplicates, real words first.
func Words(text string) []string {
	var normalizer Normalizer
	return append([]string(nil), normalizer.Words(text)...)
}

func (n *Normalizer) split(text string) {
	n.buf = n.buf[:0]
	n.spans = n.spans[:0]
	start := -1
	closeWord := func() {
		if start >= 0 {
			n.spans = append(n.spans, start, len(n.buf))
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
					start = len(n.buf)
				}
				n.buf = append(n.buf, ch)
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
				start = len(n.buf)
			}
			n.buf = append(n.buf, out...)
		case vanish:
		default:
			closeWord()
		}
	}
	closeWord()
}

// Words is the package level Words without the copy: the returned slice is only valid until the next
// call, the strings in it stay valid.
func (n *Normalizer) Words(text string) []string {
	n.split(text)
	realEnd := len(n.buf)
	pairs := len(n.spans)/2 - 1
	for idx := range max(pairs, 0) {
		left, right := 2*idx, 2*idx+2
		n.buf = append(n.buf, n.buf[n.spans[left]:n.spans[left+1]]...)
		n.buf = append(n.buf, n.buf[n.spans[right]:n.spans[right+1]]...)
	}
	joined := string(n.buf)
	n.words = n.words[:0]
	for idx := range len(n.spans) / 2 {
		n.words = appendUnique(n.words, joined[n.spans[2*idx]:n.spans[2*idx+1]])
	}
	n.RealWords = len(n.words)
	offset := realEnd
	for idx := range max(pairs, 0) {
		size := n.spans[2*idx+1] - n.spans[2*idx] + n.spans[2*idx+3] - n.spans[2*idx+2]
		n.words = appendUnique(n.words, joined[offset:offset+size])
		offset += size
	}
	return n.words
}

func appendUnique(words []string, word string) []string {
	for _, existing := range words {
		if existing == word {
			return words
		}
	}
	return append(words, word)
}

const vowels = "aeiou"

func isConsonant(ch byte) bool {
	return 'a' <= ch && ch <= 'z' && strings.IndexByte(vowels, ch) < 0
}

// Inflections returns the forms `word~` stands for, word itself first.
func Inflections(word string) []string {
	forms := []string{word, word + "s", word + "e", word + "en", word + "es", word + "n"}
	if strings.HasSuffix(word, "er") {
		stem := word[:len(word)-2]
		forms = append(forms, stem+"ster", stem+"sters")
	}
	if strings.HasSuffix(word, "eur") {
		forms = append(forms, word[:len(word)-3]+"euse")
	}
	size := len(word)
	if size >= 3 && word[size-3] == word[size-2] && strings.IndexByte(vowels, word[size-2]) >= 0 && isConsonant(word[size-1]) {
		forms = append(forms, word[:size-2]+word[size-1:]+"e")
	}
	unique := forms[:0]
	for _, form := range forms {
		unique = appendUnique(unique, form)
	}
	return unique
}
