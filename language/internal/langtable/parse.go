// Package langtable turns languages.tsv into the lookup tables the language package
// compiles in. Everything that can be checked is checked here, at generate time, so the
// runtime package is nothing but a binary search over static data.
package langtable

import (
	"fmt"
	"strings"
)

// The two files this package sits between, named relative to the language package.
const (
	SourceFile = "languages.tsv"
	OutputFile = "tables_gen.go"
)

// Language is one row of languages.tsv.
type Language struct {
	Code3   string
	Code1   string
	Dutch   string
	English string
	Native  string
	Aliases []string

	// Line is the 1-based line in the source file, for error messages.
	Line int
}

// Offered reports whether the language is part of the supported set, which is the ones
// we have a Dutch name for.
func (l Language) Offered() bool {
	return l.Dutch != ""
}

const columnCount = 6

// ParseTSV reads languages.tsv. Blank lines and # comments are skipped, the order of the
// remaining rows is the order names are claimed in.
func ParseTSV(src []byte) ([]Language, error) {
	languages := []Language{}

	for i, line := range strings.Split(string(src), "\n") {
		lineNumber := i + 1
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		columns := strings.Split(line, "\t")
		if len(columns) > columnCount {
			return nil, fmt.Errorf("line %d: expected at most %d tab separated columns, got %d", lineNumber, columnCount, len(columns))
		}

		// Editors and formatters strip trailing tabs, so a row whose last columns are
		// empty arrives short. Build still rejects a row missing a code or English name.
		for len(columns) < columnCount {
			columns = append(columns, "")
		}

		aliases := []string{}
		if columns[5] != "" {
			aliases = strings.Split(columns[5], ",")
		}

		languages = append(languages, Language{
			Code3:   columns[0],
			Code1:   columns[1],
			Dutch:   columns[2],
			English: columns[3],
			Native:  columns[4],
			Aliases: aliases,
			Line:    lineNumber,
		})
	}

	if len(languages) == 0 {
		return nil, fmt.Errorf("no languages in the source file")
	}

	return languages, nil
}
