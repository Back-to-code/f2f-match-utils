package rules

import (
	"strings"

	"github.com/Back-to-code/f2f-match-utils/sanitize"
)

func isWordByte(ch byte) bool {
	return ('a' <= ch && ch <= 'z') || ('0' <= ch && ch <= '9')
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

// Words returns the word set a rule is matched against: the words of sanitize.SplitWords plus every two
// adjacent words joined into one, without duplicates, real words first.
func Words(text string) []string {
	var normalizer Normalizer
	return append([]string(nil), normalizer.Words(text)...)
}

// Words is the package level Words without the copy: the returned slice is only valid until the next
// call, the strings in it stay valid.
func (n *Normalizer) Words(text string) []string {
	n.buf, n.spans = sanitize.AppendWords(n.buf[:0], n.spans[:0], text)
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

func isVowel(ch byte) bool {
	return strings.IndexByte(vowels, ch) >= 0
}

func isConsonant(ch byte) bool {
	return 'a' <= ch && ch <= 'z' && !isVowel(ch)
}

// sterStem respells the stem of an -er word for its -ster form, as the verb stem it comes from:
// vuller → vul, beheerder → beheer, verkoper → verkoop. Begeleider keeps its d, the d of beheerder only
// sits between r and er.
func sterStem(stem string) string {
	size := len(stem)
	if size < 3 {
		return stem
	}
	last, beforeLast := stem[size-1], stem[size-2]
	switch {
	case last == 'd' && beforeLast == 'r':
		return stem[:size-1]
	case last == beforeLast && isConsonant(last):
		return stem[:size-1]
	case isConsonant(last) && strings.IndexByte("aeou", beforeLast) >= 0 && isConsonant(stem[size-3]):
		return stem[:size-1] + stem[size-2:]
	}
	return stem
}

// Inflections returns the forms `word~` stands for, word itself first.
func Inflections(word string) []string {
	forms := []string{word, word + "s", word + "e", word + "en", word + "es", word + "n"}
	if strings.HasSuffix(word, "er") {
		stem := sterStem(word[:len(word)-2])
		forms = append(forms, stem+"ster", stem+"sters")
	}
	if strings.HasSuffix(word, "eur") {
		forms = append(forms, word[:len(word)-3]+"euse")
	}
	size := len(word)
	if size >= 3 && word[size-3] == word[size-2] && isVowel(word[size-2]) && isConsonant(word[size-1]) {
		single := word[:size-2] + word[size-1:]
		forms = append(forms, single+"e", single+"en")
	}
	unique := forms[:0]
	for _, form := range forms {
		unique = appendUnique(unique, form)
	}
	return unique
}
