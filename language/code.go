// Package language resolves a language name, alias, or ISO 639-1 or ISO 639-2 code to an
// ISO 639-3 code. The list lives in languages.tsv and the lookup tables are generated from
// it; README.md explains how and why.
package language

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"

	"github.com/Back-to-code/f2f-match-utils/language/internal/langcode"
	"github.com/Back-to-code/f2f-match-utils/sanitize"
)

// Parse returns the Code for the given language name, ISO 639-1 2-letter, or ISO 639-2/639-3
// 3-letter code. The lookup is case-insensitive and normalizes the input.
func Parse(s string) (Code, bool) {
	return lookupName(sanitize.NormalizeString(s))
}

// lookupName resolves an already normalized name against the generated table.
func lookupName(name string) (Code, bool) {
	if name == "" {
		return 0, false
	}

	index, found := slices.BinarySearch(nameHashes[:], langcode.FNV32a(name))
	if !found {
		return 0, false
	}

	return Code(nameCodes[index]), true
}

// parseCode is Parse limited to the ISO codes, so a language name is not accepted
// where a code is expected. Both the ISO 639-1 and the ISO 639-2 codes resolve to
// the ISO 639-3 code of that language.
func parseCode(s string) (Code, bool) {
	if len(s) != 2 && len(s) != 3 {
		return 0, false
	}
	return Parse(s)
}

// Code is an ISO 639-3 language code, held as its three letters packed into a base 26
// number. Only Parse and CodeOf produce one, so a non-zero Code always spells a code.
type Code uint16

// LanguageCode is an alias for Code.
type LanguageCode = Code

// CodeOf returns the Code for a literal ISO 639-3 code, and the zero Code for anything
// that is not three letters. It checks the shape only: use Parse to also check that a
// language by that code exists.
func CodeOf(s string) Code {
	packed, err := langcode.Pack(s)
	if err != nil {
		return 0
	}

	return Code(packed)
}

func (lc Code) IsZero() bool {
	return lc == 0
}

func (lc Code) letters() ([3]byte, bool) {
	return langcode.Unpack(uint16(lc))
}

// TypescriptType tells a TypeScript type generator that a Code goes over the wire as its
// ISO 639-3 code, not as the number the Go type is.
func (lc Code) TypescriptType() any {
	return ""
}

func (lc Code) MarshalJSON() ([]byte, error) {
	letters, ok := lc.letters()
	if !ok {
		return []byte("null"), nil
	}
	return []byte{'"', letters[0], letters[1], letters[2], '"'}, nil
}

// UnmarshalJSON reads a code it cannot resolve as the zero value rather than as an error,
// so one unknown language does not reject an otherwise valid payload.
func (lc *Code) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*lc = 0
		return nil
	}

	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return errors.New("invalid language code")
	}

	code, ok := parseCode(string(b[1 : len(b)-1]))
	if !ok {
		*lc = 0
		return nil
	}

	*lc = code

	return nil
}

// MarshalText is what encoding/json uses for a map key, where MarshalJSON is not
// consulted. SupportedLanguages is keyed on Code and is meant to be served as JSON.
func (lc Code) MarshalText() ([]byte, error) {
	letters, ok := lc.letters()
	if !ok {
		return []byte{}, nil
	}
	return []byte{letters[0], letters[1], letters[2]}, nil
}

// UnmarshalText mirrors MarshalText, and like UnmarshalJSON reads an unknown code as no
// language rather than as an error.
func (lc *Code) UnmarshalText(b []byte) error {
	code, _ := parseCode(string(b))
	*lc = code

	return nil
}

func (lc Code) String() string {
	letters, ok := lc.letters()
	if !ok {
		return ""
	}
	return string(letters[:])
}

// ShortCode returns the ISO 639-1 code of this language, or the zero ShortCode for the
// vast majority of ISO 639-3 languages that have none.
func (lc Code) ShortCode() ShortCode {
	index, found := slices.BinarySearch(shortCodeKeys[:], uint16(lc))
	if !found {
		return ShortCode{}
	}

	packed := shortCodeValues[index]

	return ShortCode{byte(packed >> 8), byte(packed)}
}

func (lc Code) Value() (driver.Value, error) {
	if lc.IsZero() {
		return nil, nil
	}
	return lc.String(), nil
}

// Scan converts rows still holding an ISO 639-1 code to ISO 639-3. Unlike UnmarshalJSON it
// fails on a code it cannot resolve, since that is a broken row and not user input.
func (lc *Code) Scan(src any) error {
	var s sql.NullString
	err := s.Scan(src)
	if err != nil {
		return err
	}
	if !s.Valid {
		*lc = 0
		return nil
	}
	code, ok := parseCode(s.String)
	if !ok {
		return fmt.Errorf("language.Code: unknown code %q", s.String)
	}
	*lc = code
	return nil
}
