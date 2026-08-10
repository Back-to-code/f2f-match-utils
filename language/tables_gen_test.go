package language

import (
	"bytes"
	"os"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/language/internal/langtable"
)

// tables_gen.go is committed, so it can go stale the moment someone edits languages.tsv
// without running go generate. Regenerating it here and comparing catches that.
func TestGeneratedTablesAreUpToDate(t *testing.T) {
	src, err := os.ReadFile(langtable.SourceFile)
	if err != nil {
		t.Fatalf("reading %s: %v", langtable.SourceFile, err)
	}

	expected, err := langtable.Generate(src)
	if err != nil {
		t.Fatalf("generating the table: %v", err)
	}

	actual, err := os.ReadFile(langtable.OutputFile)
	if err != nil {
		t.Fatalf("reading %s: %v", langtable.OutputFile, err)
	}

	// The file is a few hundred kilobytes, so what differs is left out on purpose.
	if !bytes.Equal(expected, actual) {
		t.Errorf("%s is out of date, run: go generate ./language/...", langtable.OutputFile)
	}
}

func BenchmarkParse(b *testing.B) {
	for b.Loop() {
		Parse("English")
	}
}

func BenchmarkParseMiss(b *testing.B) {
	for b.Loop() {
		Parse("not a language at all")
	}
}
