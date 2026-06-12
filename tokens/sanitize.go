package tokens

import (
	"iter"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// foldRanges are the Unicode blocks scanned to build foldMap: Latin-1
// Supplement, Latin Extended-A/B and Latin Extended Additional. These cover the
// accented letters that occur in European names and loanwords.
var foldRanges = [][2]rune{{0x00C0, 0x024F}, {0x1E00, 0x1EFF}}

// foldSpecials are lowercase Latin letters that do NOT decompose under Unicode
// NFD, so stripping combining marks cannot fold them. They are mapped to their
// plain base form explicitly.
var foldSpecials = map[rune]string{
	'ø': "o",
	'ł': "l",
	'đ': "d",
	'ħ': "h",
	'ß': "ss",
	'æ': "ae",
	'œ': "oe",
	'þ': "th",
	'ð': "d",
}

// foldMap maps a lowercase accented/special letter to its plain lowercase base
// form, e.g. 'è' -> "e", 'ß' -> "ss". It is built once at startup from the
// Unicode NFD decomposition so the hot tokenizing path only does a single map
// lookup per rune instead of running multiple passes over the input string.
var foldMap = buildFoldMap()

func buildFoldMap() map[rune][]rune {
	m := make(map[rune][]rune, 512)

	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	for _, rg := range foldRanges {
		for r := rg[0]; r <= rg[1]; r++ {
			if !unicode.IsLower(r) || !unicode.IsLetter(r) {
				continue
			}

			out, _, err := transform.String(t, string(r))
			if err != nil {
				continue
			}

			folded := []rune(out)
			for i := range folded {
				folded[i] = unicode.ToLower(folded[i])
			}
			if len(folded) == 1 && folded[0] == r {
				continue // No diacritic to strip.
			}
			m[r] = folded
		}
	}

	for r, base := range foldSpecials {
		m[r] = []rune(base)
	}

	return m
}

var illegalWords = map[string]struct{}{
	"aan":    {},
	"al":     {},
	"als":    {},
	"and":    {},
	"are":    {},
	"as":     {},
	"at":     {},
	"be":     {},
	"been":   {},
	"bent":   {},
	"bij":    {},
	"but":    {},
	"by":     {},
	"dan":    {},
	"de":     {},
	"den":    {},
	"der":    {},
	"door":   {},
	"een":    {},
	"en":     {},
	"for":    {},
	"from":   {},
	"haar":   {},
	"had":    {},
	"has":    {},
	"have":   {},
	"heb":    {},
	"hebben": {},
	"heeft":  {},
	"hem":    {},
	"her":    {},
	"het":    {},
	"hij":    {},
	"his":    {},
	"i":      {},
	"if":     {},
	"ik":     {},
	"in":     {},
	"is":     {},
	"it":     {},
	"je":     {},
	"jou":    {},
	"maar":   {},
	"me":     {},
	"met":    {},
	"mij":    {},
	"my":     {},
	"na":     {},
	"naar":   {},
	"nog":    {},
	"of":     {},
	"om":     {},
	"ons":    {},
	"ook":    {},
	"op":     {},
	"over":   {},
	"she":    {},
	"so":     {},
	"te":     {},
	"that":   {},
	"the":    {},
	"these":  {},
	"they":   {},
	"this":   {},
	"those":  {},
	"to":     {},
	"toch":   {},
	"tot":    {},
	"u":      {},
	"uit":    {},
	"van":    {},
	"voor":   {},
	"was":    {},
	"were":   {},
	"with":   {},
	"worden": {},
	"wordt":  {},
}

type sanitizedWordT struct {
	base     string
	addition *string
}

func sanitizedWords(in string) iter.Seq[sanitizedWordT] {
	return func(yield func(sanitizedWordT) bool) {
		var currentWord = []rune{}
		var dashCount int
		var dashAt int

		characters := []rune(in)
		for i, c := range characters {
			switch {
			case c == '-':
				if len(currentWord) > 0 {
					dashCount++
					dashAt = len(currentWord)
				}
			case c == '.' && i > 0 && i < len(characters)-1 && unicode.IsLetter(characters[i-1]) && unicode.IsLetter(characters[i+1]):
				// Do nothing, handle abbreviations
			case unicode.IsLetter(c):
				lc := unicode.ToLower(c)
				if folded, ok := foldMap[lc]; ok {
					currentWord = append(currentWord, folded...)
				} else {
					currentWord = append(currentWord, lc)
				}
			case len(currentWord) > 0:
				if dashCount == 1 && len(currentWord) > dashAt {
					base := string(currentWord[:dashAt])
					_, baseIllegal := illegalWords[base]
					additional := string(currentWord[dashAt:])
					_, additionalIllegal := illegalWords[additional]
					if baseIllegal && additionalIllegal {
						currentWord = currentWord[:0]
						dashCount = 0
						continue
					} else if !baseIllegal && !additionalIllegal {
						if !yield(sanitizedWordT{base: base, addition: &additional}) {
							return
						}
					} else if additionalIllegal {
						if !yield(sanitizedWordT{base: base}) {
							return
						}
					} else {
						if !yield(sanitizedWordT{base: additional}) {
							return
						}
					}
				} else {
					currentStr := string(currentWord)
					_, ok := illegalWords[currentStr]
					if ok {
						currentWord = currentWord[:0]
						dashCount = 0
						continue
					}

					if !yield(sanitizedWordT{base: currentStr}) {
						return
					}
				}

				currentWord = currentWord[:0]
				dashCount = 0
			}
		}

		if len(currentWord) == 0 {
			return
		}

		if dashCount == 1 && len(currentWord) > dashAt {
			base := string(currentWord[:dashAt])
			_, baseIllegal := illegalWords[base]
			additional := string(currentWord[dashAt:])
			_, additionalIllegal := illegalWords[additional]
			if baseIllegal && additionalIllegal {
				return
			} else if !baseIllegal && !additionalIllegal {
				_ = yield(sanitizedWordT{base: base, addition: &additional})
			} else if additionalIllegal {
				_ = yield(sanitizedWordT{base: base})
			} else {
				_ = yield(sanitizedWordT{base: additional})
			}

			return
		}
		currentStr := string(currentWord)
		_, ok := illegalWords[currentStr]
		if ok {
			return
		}

		_ = yield(sanitizedWordT{base: currentStr})
	}
}
