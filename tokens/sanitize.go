package tokens

import (
	"iter"
	"unicode"
)

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

func sanitizedWords(in string) iter.Seq[string] {
	return func(yield func(string) bool) {
		var currentWord = []rune{}

		characters := []rune(in)
		for i, c := range characters {
			switch {
			case c == '.' && i > 0 && i < len(characters)-1 && unicode.IsLetter(characters[i-1]) && unicode.IsLetter(characters[i+1]):
				// Do nothing, handle abbreviations
			case unicode.IsLetter(c):
				if unicode.IsUpper(c) {
					currentWord = append(currentWord, unicode.ToLower(c))
				} else {
					currentWord = append(currentWord, c)
				}
			case len(currentWord) > 0:
				currentStr := string(currentWord)
				_, ok := illegalWords[currentStr]
				if !ok && !yield(currentStr) {
					return
				}
				currentWord = currentWord[:0]
			}
		}

		if len(currentWord) > 0 {
			currentStr := string(currentWord)
			_, ok := illegalWords[currentStr]
			if !ok && !yield(currentStr) {
				return
			}
		}
	}
}
