package sanitize

import (
	"slices"
	"strings"
	"testing"
)

func TestFold(t *testing.T) {
	cases := map[string]string{
		"Magazijn-Medewerker~": "magazijn-medewerker~",
		"*Coördinator":         "*coordinator",
		"Straße":               "strasse",
		"Æble Œuvre":           "aeble oeuvre",
		"Þór Đorđe":            "thor dorde",
		"Ĳssel ﬁnancieel":      "ijssel financieel",
		"o.a. it’s":            "oa its",
		"Русский":              "Русский",
	}
	for text, expected := range cases {
		got := Fold(text)
		if got != expected {
			t.Errorf("Fold(%q) = %q, want %q", text, got, expected)
		}
	}
}

func TestSplitWords(t *testing.T) {
	cases := map[string][]string{
		"R&D / QA (senior), 2e lijns":    {"r", "d", "qa", "senior", "2e", "lijns"},
		"Magazijn-medewerker":            {"magazijn", "medewerker"},
		"orderpicker/magazijnmedewerker": {"orderpicker", "magazijnmedewerker"},
		"medewerker o.a. planning":       {"medewerker", "oa", "planning"},
		"Café́":                          {"cafe"},
		"":                               {},
		" - / ":                          {},
	}
	for text, expected := range cases {
		got := SplitWords(text)
		if !slices.Equal(got, expected) {
			t.Errorf("SplitWords(%q) = %q, want %q", text, got, expected)
		}
	}
}

func TestWordKey(t *testing.T) {
	cases := map[string]string{
		"  Magazijn-Medewerker  ":        "magazijn medewerker",
		"magazijn\tmedewerker":           "magazijn medewerker",
		"orderpicker/magazijnmedewerker": "orderpicker magazijnmedewerker",
		"E&I Monteur":                    "e i monteur",
		"Straße":                         "strasse",
		"":                               "",
		" - / ":                          "",
	}
	for text, expected := range cases {
		got := WordKey(text)
		if got != expected {
			t.Errorf("WordKey(%q) = %q, want %q", text, got, expected)
		}
	}
}

func FuzzWordKeyJoinsSplitWords(f *testing.F) {
	for _, seed := range []string{"Magazijn-medewerker (o.a. heftruck)", "it’s Straße", " - / ", "Ж ж áb"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		key := WordKey(text)
		joined := strings.Join(SplitWords(text), " ")
		if key != joined {
			t.Errorf("WordKey(%q) = %q, SplitWords joined gives %q", text, key, joined)
		}
	})
}

var benchmarkTitle = "Senior Orderpicker / Magazijn-Medewerker (Reachtruck)  m/v"

func BenchmarkFold(b *testing.B) {
	for b.Loop() {
		Fold(benchmarkTitle)
	}
}

func BenchmarkWordKey(b *testing.B) {
	for b.Loop() {
		WordKey(benchmarkTitle)
	}
}

func BenchmarkNormalizeString(b *testing.B) {
	for b.Loop() {
		NormalizeString(benchmarkTitle)
	}
}
