package rules

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

func matches(t *testing.T, rule, text string) bool {
	t.Helper()
	parsed, err := ParseRule(rule)
	if err != nil {
		t.Fatalf("%q: %v", rule, err)
	}
	return parsed.Match(text)
}

func sorted(words []string) []string {
	out := append([]string(nil), words...)
	sort.Strings(out)
	return out
}

func TestWords(t *testing.T) {
	cases := map[string][]string{
		"magazijn medewerker":              {"magazijn", "medewerker", "magazijnmedewerker"},
		"Magazijn-Medewerker":              {"magazijn", "medewerker", "magazijnmedewerker"},
		"medewerker logistiek en magazijn": {"medewerker", "logistiek", "en", "magazijn", "medewerkerlogistiek", "logistieken", "enmagazijn"},
		"Coördinator":                      {"coordinator"},
		"café é ë":                         {"cafe", "e", "cafee", "ee"},
		"commerciële":                      {"commerciele"},
		"Straße":                           {"strasse"},
		"Søren":                            {"soren"},
		"Æble Œuvre":                       {"aeble", "oeuvre", "aebleoeuvre"},
		"Łódź":                             {"lodz"},
		"o.a.":                             {"oa"},
		"it's":                             {"its"},
		"it’s":                             {"its"},
		"medewerker o.a. planning":         {"medewerker", "oa", "planning", "medewerkeroa", "oaplanning"},
		"":                                 {},
		" - / ":                            {},
		"a a":                              {"a", "aa"},
	}
	for text, expected := range cases {
		got := sorted(Words(text))
		if !slices.Equal(got, sorted(expected)) {
			t.Errorf("%q: got %v, want %v", text, got, sorted(expected))
		}
	}
	got := SplitWords("R&D / QA (senior), 2e lijns")
	if !slices.Equal(got, []string{"r", "d", "qa", "senior", "2e", "lijns"}) {
		t.Errorf("split: %v", got)
	}
}

func TestNormalizerRealWords(t *testing.T) {
	var normalizer Normalizer
	words := normalizer.Words("magazijn medewerker magazijn")
	if !slices.Equal(words, []string{"magazijn", "medewerker", "magazijnmedewerker", "medewerkermagazijn"}) || normalizer.RealWords != 2 {
		t.Errorf("got %v with %d real words", words, normalizer.RealWords)
	}
}

func TestInflections(t *testing.T) {
	got := Inflections("monteur")[:6]
	if !slices.Equal(got, []string{"monteur", "monteurs", "monteure", "monteuren", "monteures", "monteurn"}) {
		t.Errorf("monteur: %v", got)
	}
	forms := Inflections("medewerker")
	if !slices.Contains(forms, "medewerkster") || !slices.Contains(forms, "medewerksters") || len(forms) != 8 {
		t.Errorf("medewerker: %v", forms)
	}
	if !slices.Contains(Inflections("chauffeur"), "chauffeuse") {
		t.Error("chauffeuse")
	}
	if !slices.Contains(Inflections("commercieel"), "commerciele") || !slices.Contains(Inflections("groot"), "grote") {
		t.Error("doubled vowel")
	}
}

func TestLiterals(t *testing.T) {
	yes := [][2]string{
		{"magazijnmedewerker", "Magazijnmedewerker"},
		{"logistiek*", "logistieke medewerker"}, {"logistiek*", "medewerker logistiek"},
		{"*monteur", "automonteur"}, {"*monteur", "service monteur"}, {"monteur~", "monteurs"},
		{"*logist*", "medewerker intralogistieke dienst"},
		{"medewerker~", "medewerksters"}, {"medewerker~", "medewerkeren"},
		{"chauffeur~", "chauffeuse"}, {"commercieel~", "commerciële binnendienst"},
		{"verkoper~|verkoopster", "verkoopster"}, {"verkoper~|verkoopster", "verkopers"},
		{"*monteur~", "automonteurs"}, {"*monteur~", "monteur"},
		{"Coördinator", "coordinator"}, {"coordinator", "Coördinator"}, {"o.a.", "medewerker o.a. planning"},
		{"it's", "It’s"},
	}
	no := [][2]string{
		{"magazijnmedewerker", "magazijnmedewerkers"}, {"logistiek*", "supply chain"}, {"*monteur", "monteurs"},
		{"*logist*", "magazijn"}, {"medewerker~", "medewerk"}, {"medewerker~", "magazijnmedewerker"},
		{"*monteur~", "monteurtje"},
	}
	for _, pair := range yes {
		if !matches(t, pair[0], pair[1]) {
			t.Errorf("%q should match %q", pair[0], pair[1])
		}
	}
	for _, pair := range no {
		if matches(t, pair[0], pair[1]) {
			t.Errorf("%q should not match %q", pair[0], pair[1])
		}
	}
}

func formStrings(forms []Form) []string {
	out := make([]string, len(forms))
	for idx, form := range forms {
		out[idx] = form.String()
	}
	return out
}

func TestCompounds(t *testing.T) {
	for _, text := range []string{"magazijnmedewerker", "magazijn medewerker", "magazijn-medewerker", "medewerker magazijn", "medewerker logistiek en magazijn"} {
		if !matches(t, "magazijn+medewerker", text) {
			t.Errorf("magazijn+medewerker should match %q", text)
		}
	}
	for _, text := range []string{"magazijnchef", "medewerker", "magazijn"} {
		if matches(t, "magazijn+medewerker", text) {
			t.Errorf("magazijn+medewerker should not match %q", text)
		}
	}
	rule := MustParseRule("*truck+chauffeur~")
	if got := formStrings(rule.Terms[0].Node.Forms); !slices.Equal(got, []string{"*truckchauffeur~"}) {
		t.Errorf("forms: %v", got)
	}
	for _, text := range []string{"heftruckchauffeuse", "heftruck chauffeuse", "chauffeurs heftruck", "vrachtwagen truckchauffeur"} {
		if !rule.Match(text) {
			t.Errorf("*truck+chauffeur~ should match %q", text)
		}
	}
	if rule.Match("truckersvrouw chauffeur") || rule.Match("heftruck bestuurder") {
		t.Error("*truck+chauffeur~ false positive")
	}
	nested := MustParseRule("(magazijn|(order|voorraad)+picker)+medewerker").Terms[0].Node
	if got := formStrings(nested.Forms); !slices.Equal(got, []string{"magazijnmedewerker", "orderpickermedewerker", "voorraadpickermedewerker"}) {
		t.Errorf("nested forms %v", got)
	}
	if got := formStrings(MustParseRule("(*abcd|b)+(c|efgh*)").Terms[0].Node.Forms); !slices.Equal(got, []string{"*abcdc", "*abcdefgh*", "bc", "befgh*"}) {
		t.Errorf("wildcard forms %v", got)
	}
	if !matches(t, "kok|keuken+hulp", "hulp in de keuken") || matches(t, "kok|keuken+hulp", "keuken") {
		t.Error("kok|keuken+hulp")
	}
	if !matches(t, "medewerker+klant+service", "service medewerker klant") || matches(t, "medewerker+klant+service", "klantservice") {
		t.Error("three parts")
	}
}

func TestNegativeTerms(t *testing.T) {
	if !matches(t, "medewerker~ -hoofd*", "medewerker magazijn") || matches(t, "medewerker~ -hoofd*", "hoofd medewerker") {
		t.Error("negative term")
	}
}

func TestValidation(t *testing.T) {
	valid := []string{
		"logistiek*", "*monteur", "*logist*", "medewerker~", "*monteur~", "magazijn+medewerker",
		"(magazijn|warehouse)+medewerker~", "logistiek*|magazijn*", "c+(abcd*|b)", "(*abcd|b)+c",
		"*truck+chauffeur~", "medewerker~ -hoofd* -*chef", "Coördinator", "o.a.", "  magazijn   medewerker  ",
		"(a+b)+c", "a|(b|c)", "abcd*",
	}
	for _, rule := range valid {
		if _, err := ParseRule(rule); err != nil {
			t.Errorf("%q should be valid: %v", rule, err)
		}
	}
	group := "(" + strings.Join(strings.Split("abcdefghij", ""), "|") + ")"
	invalid := map[string]string{
		"*er": "at least 4 letters", "abc~": "at least 4 letters", "*abc*": "at least 4 letters",
		"a*+b": "trailing * is only allowed on the last part", "abcd~+b": "trailing ~ is only allowed on the last part",
		"b+*abcd": "leading * is only allowed on the first part", "(abcd*|b)+c": "trailing * is only allowed on the last part",
		"x+(*abcd|b)": "leading * is only allowed on the first part", "~*": "'~' must follow a word",
		"medewerker~*": "~ must come last", "medewerker*~": "both * and ~", "(a|b)~": "not to a group",
		"*(abcd|b)": "not to a group", "**abcd": "'**'", "": "empty rule", "   ": "empty rule", "()": "empty group",
		"a||b": "missing word", "a+": "word on both sides", "|a": "word on both sides", "-": "followed by a term",
		"-medewerker": "at least one term without '-'", "-medewerker -magazijn*": "at least one term without '-'",
		"--medewerker": "only one leading '-'", "(magazijn|warehouse+medewerker": "unbalanced", "magazijn)": "unbalanced",
		"(a))(b": "unbalanced", "e-commerce": "'-' is only allowed at the start", "c#": "letters and digits",
		"(a | b)": "no spaces inside", "a | b": "no spaces around", "a(b)": "joined to the group", "(a)(b)": "joined to its neighbour",
		strings.Join([]string{group, group, group, group}, "+"): "limit",
	}
	for rule, fragment := range invalid {
		_, err := ParseRule(rule)
		if err == nil || !strings.Contains(err.Error(), fragment) {
			t.Errorf("%q: want %q, got %v", rule, fragment, err)
		}
	}
	if _, err := ParseRule(strings.Join([]string{group, group, group}, "+")); err != nil {
		t.Errorf("1110 forms should be valid: %v", err)
	}
	_, err := ParseRule("*er a*+b")
	if got := Messages(err); len(got) != 3 {
		t.Errorf("one message per problem, got %v", got)
	}
	for _, term := range []string{"*manager", "(team|afdeling)+leider~"} {
		if _, err := ParseExclude(term); err != nil {
			t.Errorf("exclude %q should be valid: %v", term, err)
		}
	}
	for _, term := range []string{"-*manager", "hoofd magazijn", "", "*er"} {
		if _, err := ParseExclude(term); err == nil {
			t.Errorf("exclude %q should be invalid", term)
		}
	}
}

func TestNormalizedSource(t *testing.T) {
	rule := MustParseRule("  (Magazijn|warehouse)+Medewerker~   -hoofd*  ")
	if rule.Source != "(Magazijn|warehouse)+Medewerker~   -hoofd*" {
		t.Errorf("source %q", rule.Source)
	}
	if got := rule.NormalizedSource(); got != "(magazijn|warehouse)+medewerker~ -hoofd*" {
		t.Errorf("normalized %q", got)
	}
}

func TestExplain(t *testing.T) {
	text := Explain("(magazijn|warehouse)+medewerker~ -hoofd*")
	for _, fragment := range []string{"one word: magazijnmedewerker~ | warehousemedewerker~", "(magazijn | warehouse) & medewerker~", "must NOT match: hoofd* (prefix)", "medewerkster"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("explain misses %q:\n%s", fragment, text)
		}
	}
	if text := Explain("*er"); !strings.HasPrefix(text, "invalid rule: *er\n  ") {
		t.Errorf("invalid explain:\n%s", text)
	}
}

func TestCompile(t *testing.T) {
	set := MustCompile(Set{Includes: []string{"(magazijn|warehouse)+medewerker~"}, Excludes: []string{"*manager"}})
	if set.Match("warehouse medewerker") != 0 || set.Match("magazijnmedewerker / assistent-manager") != -1 {
		t.Error("excludes")
	}
	first := MustCompile(Set{Includes: []string{"chauffeur~", "heftruck*"}})
	if first.Match("heftruck chauffeur") != 0 || first.Match("heftruckchauffeur") != 1 || first.Match("kraanmachinist") != -1 {
		t.Error("first matching include")
	}

	compiled, problems := Compile(Set{Includes: []string{"magazijn*", "*er", "chauffeur~"}, Excludes: []string{"-*manager"}})
	if len(problems) != 2 || problems[0].Exclude || problems[0].Index != 1 || !problems[1].Exclude || problems[1].Index != 0 {
		t.Fatalf("problems: %+v", problems)
	}
	if compiled.Includes[1] != nil || compiled.Excludes[0] != nil {
		t.Error("invalid entries must be nil")
	}
	if compiled.Match("chauffeur") != 2 {
		t.Error("an include after an invalid one keeps its index")
	}
	if got := problems[0].Error(); got != "includes[1] '*er': a word with * or ~ needs at least 4 letters, 'er' has 2" {
		t.Errorf("located message %q", got)
	}
	if got := problems[1].Error(); got != "excludes[0] '-*manager': an exclude is already negative, drop the leading '-'" {
		t.Errorf("located message %q", got)
	}
	_, problems = Compile(Set{Includes: []string{"a | b"}})
	if got := problems[0].Error(); got != `includes[0] "a | b": '|': | and + need a word on both sides and no spaces around them (a|b, a+b)` {
		t.Errorf("unlocated message %q", got)
	}
}

func TestClean(t *testing.T) {
	cleaned := Set{Includes: []string{"  magazijn*  ", "", " \t"}}.Clean()
	if !slices.Equal(cleaned.Includes, []string{"magazijn*"}) || cleaned.Excludes == nil || len(cleaned.Excludes) != 0 {
		t.Errorf("cleaned %#v", cleaned)
	}
}
