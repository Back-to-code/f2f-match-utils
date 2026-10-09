package sanitize

import (
	"bytes"
	"os"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/sanitize/internal/foldtable"
)

func TestFoldTableIsUpToDate(t *testing.T) {
	version, err := foldtable.Version()
	if err != nil {
		t.Skip(err)
	}
	// Each toolchain carries its own Unicode tables; only the one the table came from can regenerate it.
	if version != foldUnicodeVersion {
		t.Skipf("%s holds Unicode %s, this toolchain has %s", foldtable.OutputFile, foldUnicodeVersion, version)
	}
	expected, err := foldtable.Generate()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(foldtable.OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, actual) {
		t.Errorf("%s is out of date, run: go generate ./sanitize/...", foldtable.OutputFile)
	}
}

func TestFoldAgreesWithReference(t *testing.T) {
	version, err := foldtable.Version()
	if err != nil || version != foldUnicodeVersion {
		t.Skip("the reference fold uses another Unicode version than the table")
	}
	for r := rune(0x80); r < 0x3000; r++ {
		want := foldtable.Fold(r)
		out, vanish, mapped := foldRune(r)
		switch {
		case vanish && want != "":
			t.Errorf("U+%04X vanishes, reference gives %q", r, want)
		case mapped && out != want:
			t.Errorf("U+%04X maps to %q, reference gives %q", r, out, want)
		}
	}
}
