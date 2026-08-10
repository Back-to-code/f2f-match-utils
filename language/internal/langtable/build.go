package langtable

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Back-to-code/f2f-match-utils/language/internal/langcode"
	"github.com/Back-to-code/f2f-match-utils/sanitize"
)

// Name is a lookup name reduced to the hash it is matched on and the code it answers with.
type Name struct {
	Hash uint32
	Code string

	// Text is the name this hash was made of. It reaches the collision message and the
	// tests, never the generated table.
	Text string
}

// ShortCode pairs a language with its ISO 639-1 code. Only the ~3% that have one are here.
type ShortCode struct {
	Code3 string
	Code1 string
}

// Dutch pairs a language with the Dutch name we offer it under.
type Dutch struct {
	Code3 string
	Name  string
}

// Table is everything the generated file holds.
type Table struct {
	// Names is sorted by hash, which is what makes the runtime lookup a binary search.
	Names      []Name
	ShortCodes []ShortCode
	Dutch      []Dutch
}

// Build validates the languages and turns them into the lookup table, so a bad edit to
// languages.tsv fails the generator rather than shipping.
func Build(languages []Language) (*Table, error) {
	err := validateFields(languages)
	if err != nil {
		return nil, err
	}

	table, byHash, err := claimNames(languages)
	if err != nil {
		return nil, err
	}

	err = validateOfferedLanguages(languages, byHash)
	if err != nil {
		return nil, err
	}

	return table, nil
}

func validateFields(languages []Language) error {
	problems := []error{}
	seenCode3 := map[string]int{}
	seenCode1 := map[string]int{}
	seenDutch := map[string]int{}

	for _, lang := range languages {
		if len(lang.Code3) != 3 || !isLowerAlpha(lang.Code3) {
			problems = append(problems, fmt.Errorf("line %d: %q is not a 3 letter lowercase ISO 639-3 code", lang.Line, lang.Code3))
		}
		if lang.Code1 != "" && (len(lang.Code1) != 2 || !isLowerAlpha(lang.Code1)) {
			problems = append(problems, fmt.Errorf("line %d: %q is not a 2 letter lowercase ISO 639-1 code", lang.Line, lang.Code1))
		}
		if lang.English == "" {
			problems = append(problems, fmt.Errorf("line %d: %q has no English name", lang.Line, lang.Code3))
		}

		problems = appendIfDuplicate(problems, seenCode3, lang.Code3, lang.Line, "ISO 639-3 code")
		if lang.Code1 != "" {
			problems = appendIfDuplicate(problems, seenCode1, lang.Code1, lang.Line, "ISO 639-1 code")
		}
		if lang.Dutch != "" {
			problems = appendIfDuplicate(problems, seenDutch, lang.Dutch, lang.Line, "Dutch name")
		}

		problems = append(problems, validateAliases(lang)...)
	}

	return errors.Join(problems...)
}

// An alias that does not survive NormalizeString is unreachable, and one that repeats a
// name or code is dead weight, since those are claimed from their own column.
func validateAliases(lang Language) []error {
	problems := []error{}
	claimed := map[string]struct{}{
		lang.Code3:                             {},
		lang.Code1:                             {},
		sanitize.NormalizeString(lang.Dutch):   {},
		sanitize.NormalizeString(lang.English): {},
		sanitize.NormalizeString(lang.Native):  {},
	}

	seen := map[string]struct{}{}
	for _, alias := range lang.Aliases {
		switch {
		case alias == "":
			problems = append(problems, fmt.Errorf("line %d: %q has an empty alias", lang.Line, lang.Code3))
			continue
		case alias != sanitize.NormalizeString(alias):
			problems = append(problems, fmt.Errorf("line %d: alias %q of %q is not normalized, it should be %q", lang.Line, alias, lang.Code3, sanitize.NormalizeString(alias)))
			continue
		}

		_, repeated := claimed[alias]
		if repeated {
			problems = append(problems, fmt.Errorf("line %d: alias %q repeats a name or code of %q", lang.Line, alias, lang.Code3))
		}

		_, duplicate := seen[alias]
		if duplicate {
			problems = append(problems, fmt.Errorf("line %d: alias %q of %q is listed twice", lang.Line, alias, lang.Code3))
		}
		seen[alias] = struct{}{}
	}

	return problems
}

// claimNames registers every lookup name, first row wins. Codes are claimed before any
// name, so a code always beats a name. The README covers why the order matters.
func claimNames(languages []Language) (*Table, map[uint32]Name, error) {
	table := &Table{}
	byHash := map[uint32]Name{}

	claim := func(text, code string) error {
		if text == "" {
			return nil
		}

		hash := langcode.FNV32a(text)
		existing, taken := byHash[hash]
		switch {
		case taken && existing.Text == text:
			return nil
		case taken:
			return fmt.Errorf("hash collision: %q and %q both hash to %d, rename one of them or widen the hash", existing.Text, text, hash)
		}

		byHash[hash] = Name{Hash: hash, Code: code, Text: text}
		table.Names = append(table.Names, byHash[hash])

		return nil
	}

	for _, lang := range languages {
		err := errors.Join(claim(lang.Code3, lang.Code3), claim(lang.Code1, lang.Code3))
		if err != nil {
			return nil, nil, err
		}

		if lang.Code1 != "" {
			table.ShortCodes = append(table.ShortCodes, ShortCode{Code3: lang.Code3, Code1: lang.Code1})
		}
		if lang.Offered() {
			table.Dutch = append(table.Dutch, Dutch{Code3: lang.Code3, Name: lang.Dutch})
		}
	}

	for _, lang := range languages {
		names := append(lang.normalizedNames(), lang.Aliases...)
		for _, name := range names {
			err := claim(name, lang.Code3)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	slices.SortFunc(table.Names, func(a, b Name) int { return int(a.Hash) - int(b.Hash) })
	slices.SortFunc(table.ShortCodes, func(a, b ShortCode) int { return strings.Compare(a.Code3, b.Code3) })
	slices.SortFunc(table.Dutch, func(a, b Dutch) int { return strings.Compare(a.Code3, b.Code3) })

	return table, byHash, nil
}

// normalizedNames returns the language's three names as they are looked up.
func (l Language) normalizedNames() []string {
	return []string{
		sanitize.NormalizeString(l.Dutch),
		sanitize.NormalizeString(l.English),
		sanitize.NormalizeString(l.Native),
	}
}

// Sharing a name is unavoidable for the reference names and autonyms the ISO register
// hands to a macrolanguage and its varieties, but the Dutch names and aliases are ours: a
// language we offer must never lose one, and must stay reachable by one of its names.
func validateOfferedLanguages(languages []Language, byHash map[uint32]Name) error {
	problems := []error{}

	codes := map[string]struct{}{}
	for _, lang := range languages {
		codes[lang.Code3] = struct{}{}
		if lang.Code1 != "" {
			codes[lang.Code1] = struct{}{}
		}
	}

	owner := map[string]string{}
	for _, lang := range languages {
		if !lang.Offered() {
			continue
		}

		ourNames := append([]string{sanitize.NormalizeString(lang.Dutch)}, lang.Aliases...)
		for _, name := range ourNames {
			previous, taken := owner[name]
			if taken {
				problems = append(problems, fmt.Errorf("line %d: %q is a name of both %q and %q", lang.Line, name, previous, lang.Code3))
			}
			owner[name] = lang.Code3

			_, isCode := codes[name]
			if isCode && name != lang.Code3 {
				continue
			}

			resolved, ok := byHash[langcode.FNV32a(name)]
			if !ok || resolved.Code != lang.Code3 {
				problems = append(problems, fmt.Errorf("line %d: %q resolves to %q instead of %q", lang.Line, name, resolved.Code, lang.Code3))
			}
		}

		if !reachable(lang, byHash) {
			problems = append(problems, fmt.Errorf("line %d: %q is not reachable by any of its names", lang.Line, lang.Code3))
		}
	}

	return errors.Join(problems...)
}

func reachable(lang Language, byHash map[uint32]Name) bool {
	for _, name := range append(lang.normalizedNames(), lang.Aliases...) {
		if name == "" {
			continue
		}

		resolved, ok := byHash[langcode.FNV32a(name)]
		if ok && resolved.Code == lang.Code3 {
			return true
		}
	}

	return false
}

func appendIfDuplicate(problems []error, seen map[string]int, value string, line int, what string) []error {
	previous, duplicate := seen[value]
	if duplicate {
		return append(problems, fmt.Errorf("line %d: duplicate %s %q, also on line %d", line, what, value, previous))
	}

	seen[value] = line

	return problems
}

func isLowerAlpha(s string) bool {
	for i := range len(s) {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}
