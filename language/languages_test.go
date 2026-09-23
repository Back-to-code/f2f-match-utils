package language

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/language/internal/langtable"
)

func loadLanguages(t *testing.T) []langtable.Language {
	t.Helper()

	src, err := os.ReadFile(langtable.SourceFile)
	if err != nil {
		t.Fatalf("reading %s: %v", langtable.SourceFile, err)
	}

	languages, err := langtable.ParseTSV(src)
	if err != nil {
		t.Fatalf("parsing %s: %v", langtable.SourceFile, err)
	}

	return languages
}

func buildTable(t *testing.T) *langtable.Table {
	t.Helper()

	table, err := langtable.Build(loadLanguages(t))
	if err != nil {
		t.Fatalf("building the table: %v", err)
	}

	return table
}

// Build holds every rule languages.tsv has to satisfy. Running it here as well as in the
// generator means a bad edit fails the suite, not only whoever runs go generate.
func TestLanguagesFileIsValid(t *testing.T) {
	_, err := langtable.Build(loadLanguages(t))
	if err != nil {
		t.Errorf("%s is not valid: %v", langtable.SourceFile, err)
	}
}

// Two names that hash the same are two languages that cannot be told apart. The generator
// refuses to write such a table, this test says so out loud.
func TestNoHashCollisions(t *testing.T) {
	table := buildTable(t)

	seen := map[uint32]string{}
	for _, name := range table.Names {
		previous, taken := seen[name.Hash]
		if taken {
			t.Errorf("%q and %q both hash to %d", previous, name.Text, name.Hash)
		}
		seen[name.Hash] = name.Text
	}

	if len(seen) != len(table.Names) {
		t.Errorf("%d names hash to %d values", len(table.Names), len(seen))
	}
	if len(table.Names) != nameCount {
		t.Errorf("the generated table holds %d names, the source holds %d", nameCount, len(table.Names))
	}
}

// The check that the hashed table still means the same as the list it came from.
func TestEveryClaimedNameResolvesToItsLanguage(t *testing.T) {
	for _, name := range buildTable(t).Names {
		code, ok := Parse(name.Text)
		if !ok {
			t.Errorf("%q does not parse", name.Text)

			continue
		}
		if code.String() != name.Code {
			t.Errorf("%q resolves to %q, want %q", name.Text, code, name.Code)
		}
	}
}

// Every language must be reachable by its own ISO 639-3 code, which is what stored data is
// keyed on, and by its ISO 639-1 code, which is the format stored before this list moved to
// ISO 639-3 and the one older callers still send.
func TestEveryLanguageIsReachableByItsOwnCode(t *testing.T) {
	for _, lang := range loadLanguages(t) {
		code, ok := Parse(lang.Code3)
		if !ok {
			t.Errorf("%q does not parse", lang.Code3)
		} else if code.String() != lang.Code3 {
			t.Errorf("%q resolves to %q", lang.Code3, code)
		}

		if lang.Code1 == "" {
			continue
		}

		code, ok = Parse(lang.Code1)
		if !ok {
			t.Errorf("%q does not parse", lang.Code1)

			continue
		}
		if code.String() != lang.Code3 {
			t.Errorf("%q resolves to %q, want %q", lang.Code1, code, lang.Code3)
		}
	}
}

func TestEveryLanguageMapsBackToItsShortCode(t *testing.T) {
	withShortCode := 0
	for _, lang := range loadLanguages(t) {
		code := CodeOf(lang.Code3)

		if lang.Code1 == "" {
			if !code.ShortCode().IsZero() {
				t.Errorf("%q has no ISO 639-1 code but maps to %q", lang.Code3, code.ShortCode())
			}

			continue
		}

		withShortCode++
		if code.ShortCode().String() != lang.Code1 {
			t.Errorf("%q maps back to %q, want %q", lang.Code3, code.ShortCode(), lang.Code1)
		}
	}

	if withShortCode != shortCodeCount {
		t.Errorf("the generated table holds %d short codes, the source holds %d", shortCodeCount, withShortCode)
	}
}

// Only the languages carrying a Dutch name are in the supported set.
func TestSupportedLanguagesHoldsTheDutchNamedLanguages(t *testing.T) {
	offered := 0
	for _, lang := range loadLanguages(t) {
		code := CodeOf(lang.Code3)

		if !lang.Offered() {
			if _, present := SupportedLanguages[code]; present {
				t.Errorf("%q has no Dutch name but is offered", lang.Code3)
			}

			continue
		}

		offered++
		if SupportedLanguages[code] != lang.Dutch {
			t.Errorf("%q is offered as %q, want %q", lang.Code3, SupportedLanguages[code], lang.Dutch)
		}
	}

	if len(SupportedLanguages) != offered {
		t.Errorf("SupportedLanguages holds %d languages, %d carry a Dutch name", len(SupportedLanguages), offered)
	}
}

// Callers get an object keyed on the ISO 639-3 code, which only holds because Code
// implements MarshalText. That is easy to drop without noticing.
func TestSupportedLanguagesMarshalsAsCodeToName(t *testing.T) {
	marshalled, err := json.Marshal(SupportedLanguages)
	if err != nil {
		t.Fatalf("marshalling SupportedLanguages: %v", err)
	}

	byCode := map[string]string{}
	err = json.Unmarshal(marshalled, &byCode)
	if err != nil {
		t.Fatalf("unmarshalling SupportedLanguages: %v", err)
	}

	if len(byCode) != len(SupportedLanguages) {
		t.Errorf("%d languages marshalled to %d keys", len(SupportedLanguages), len(byCode))
	}
	if byCode["nld"] != "Nederlands" {
		t.Errorf("nld marshalled as %q, want Nederlands", byCode["nld"])
	}
	if byCode["eng"] != "Engels" {
		t.Errorf("eng marshalled as %q, want Engels", byCode["eng"])
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		input string
		code  Code
	}{
		// "yi" is the ISO 639-1 code of Jiddisch, not of Sichuan Yi.
		{"yi", CodeOf("yid")},
		{"jiddisch", CodeOf("yid")},
		{"yid", CodeOf("yid")},
		{"Sichuan Yi", CodeOf("iii")},
		{"nuosu", CodeOf("iii")},
		{"ii", CodeOf("iii")},

		// "mei" is the Dutch month, not a code of any language. The ISO 639-2/B
		// code for Malay is "may".
		{"may", CodeOf("msa")},
		{"maleis", CodeOf("msa")},
		{"msa", CodeOf("msa")},

		{"zha", CodeOf("zha")},
		{"zhuang", CodeOf("zha")},

		// The lookup normalizes its input.
		{"  ENGELS  ", CodeOf("eng")},
		{"Oekraïens", CodeOf("ukr")},

		// The english name and the name in the language itself resolve too.
		{"Dutch", CodeOf("nld")},
		{"Deutsch", CodeOf("deu")},
		{"français", CodeOf("fra")},
		{"русский", CodeOf("rus")},

		// Languages without an ISO 639-1 code are reachable since the list covers all
		// of ISO 639-3.
		{"pap", CodeOf("pap")},
		{"Papiamento", CodeOf("pap")},
		{"Papiamentu", CodeOf("pap")},
		{"Sranantongo", CodeOf("srn")},
		{"Sarnami", CodeOf("hns")},
		{"hindoestaans", CodeOf("hns")},
		{"Tarifit", CodeOf("rif")},
		{"Kurmanji", CodeOf("kmr")},
		{"Nederlandse Gebarentaal", CodeOf("dse")},

		// The Dutch regional languages are offered as Dutch, Frisian excepted. Their
		// codes still name their own ISO language, since a code always wins.
		{"Twents", CodeOf("nld")},
		{"Nedersaksisch", CodeOf("nld")},
		{"Limburgs", CodeOf("nld")},
		{"West-Vlaams", CodeOf("nld")},
		{"twd", CodeOf("twd")},
		{"li", CodeOf("lim")},
		{"Fries", CodeOf("fry")},

		// Both are also the name of an unoffered language further down the list.
		{"Afaan Oromoo", CodeOf("orm")},
		{"Pidgin", CodeOf("pcm")},

		// "mei" is the Dutch month but also the ISO 639-3 code of Midob, and a code
		// always wins. Language fields do hold bare codes, "nl" among them.
		{"mei", CodeOf("mei")},
	}

	for _, c := range cases {
		code, ok := Parse(c.input)
		if !ok {
			t.Errorf("%q does not parse", c.input)

			continue
		}
		if code != c.code {
			t.Errorf("%q resolves to %q, want %q", c.input, code, c.code)
		}
	}
}

// These are deliberately absent: a wrong code produces matches downstream, while an
// unparsed name only means the input names no language we can be sure of.
func TestNamesThatMustNotParse(t *testing.T) {
	names := []string{
		// Ambiguous between Moroccan Arabic and the Berber languages, both of which we
		// do list, so guessing one of them would be wrong more often than not.
		"marokkaans",
		"moroccan",

		// Syrian Arabic, or the Aramaic of Syrian Christians. Both are listed.
		"syrisch",

		"germanic languages",
		"overig",

		"",
		"   ",
		"a name no language has",
	}

	for _, name := range names {
		code, ok := Parse(name)
		if ok {
			t.Errorf("%q parses as %q", name, code)
		}
	}
}
