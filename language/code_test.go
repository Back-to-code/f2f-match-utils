package language

import (
	"encoding/json"
	"testing"
)

var (
	nld = CodeOf("nld")
	eng = CodeOf("eng")
)

func TestUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		code  Code
	}{
		{"iso 639-3 code", `"nld"`, nld},
		{"iso 639-1 code is upgraded", `"nl"`, nld},
		{"iso 639-2 bibliographic code is upgraded", `"dut"`, nld},
		{"uppercase", `"EN"`, eng},
		{"null", `null`, Code(0)},

		// A payload can carry a code per language, and a single unknown one must not
		// reject the whole payload. The zero value stores as NULL, the same as no code
		// at all.
		{"unknown code", `"xx"`, Code(0)},
		{"empty string", `""`, Code(0)},
		{"language name is not a code", `"nederlands"`, Code(0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var code Code
			err := json.Unmarshal([]byte(c.input), &code)
			if err != nil {
				t.Fatalf("unmarshalling %s: %v", c.input, err)
			}
			if code != c.code {
				t.Errorf("%s unmarshalled as %q, want %q", c.input, code, c.code)
			}
		})
	}
}

func TestUnmarshalJSONRejectsNonStrings(t *testing.T) {
	for _, input := range []string{`1`, `true`, `{}`, `[]`} {
		var code Code
		err := json.Unmarshal([]byte(input), &code)
		if err == nil {
			t.Errorf("%s was accepted", input)
		}
	}
}

func TestMarshalJSON(t *testing.T) {
	marshalled, err := json.Marshal(nld)
	if err != nil {
		t.Fatalf("marshalling nld: %v", err)
	}
	if string(marshalled) != `"nld"` {
		t.Errorf("nld marshalled as %s, want %q", marshalled, `"nld"`)
	}

	marshalled, err = json.Marshal(Code(0))
	if err != nil {
		t.Fatalf("marshalling the zero Code: %v", err)
	}
	if string(marshalled) != `null` {
		t.Errorf("the zero Code marshalled as %s, want null", marshalled)
	}
}

func TestScan(t *testing.T) {
	cases := []struct {
		name  string
		input any
		code  Code
	}{
		{"iso 639-3 code", "nld", nld},
		{"iso 639-1 code is upgraded", "nl", nld},
		{"iso 639-2 bibliographic code is upgraded", "dut", nld},
		{"bytes", []byte("eng"), eng},
		{"null", nil, Code(0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var code Code
			err := code.Scan(c.input)
			if err != nil {
				t.Fatalf("scanning %v: %v", c.input, err)
			}
			if code != c.code {
				t.Errorf("%v scanned as %q, want %q", c.input, code, c.code)
			}
		})
	}
}

// A stored value we cannot resolve is a broken row rather than untrusted input, so
// unlike UnmarshalJSON it fails loudly instead of reading as no language.
func TestScanRejectsUnknownCodes(t *testing.T) {
	for _, input := range []string{"xx", "", "nederlands"} {
		var code Code
		err := code.Scan(input)
		if err == nil {
			t.Errorf("%q was accepted", input)
		}
	}
}

func TestValue(t *testing.T) {
	value, err := nld.Value()
	if err != nil {
		t.Fatalf("the value of nld: %v", err)
	}
	if value != "nld" {
		t.Errorf("nld has value %v, want nld", value)
	}

	value, err = Code(0).Value()
	if err != nil {
		t.Fatalf("the value of the zero Code: %v", err)
	}
	if value != nil {
		t.Errorf("the zero Code has value %v, want nil", value)
	}
}

// CodeOf normalizes what is only a spelling difference and rejects the rest. Anything it
// let through would be a Code that String and MarshalJSON cannot spell.
func TestCodeOf(t *testing.T) {
	if CodeOf("nld") != nld {
		t.Errorf("CodeOf(\"nld\") is %q, want nld", CodeOf("nld"))
	}
	if CodeOf("nld").String() != "nld" {
		t.Errorf("CodeOf(\"nld\") spells %q, want nld", CodeOf("nld").String())
	}

	// A code no language has still packs: CodeOf checks the shape, Parse checks the list.
	if CodeOf("xxx").IsZero() {
		t.Error("CodeOf(\"xxx\") produced no Code")
	}
	if _, known := Parse("xxx"); known {
		t.Error("xxx parses as a language")
	}

	// Case and punctuation are not part of a code, so they are normalized away.
	for _, input := range []string{"NLD", "n.l.d"} {
		if CodeOf(input) != nld {
			t.Errorf("CodeOf(%q) is %q, want nld", input, CodeOf(input))
		}
	}

	for _, input := range []string{"", "nl", "nldx", "nl1", "nl ", "n/a"} {
		if !CodeOf(input).IsZero() {
			t.Errorf("%q produced a Code", input)
		}
	}
}

// A Code that never came out of CodeOf or Parse spells nothing.
func TestOutOfRangeCodeIsNotALanguage(t *testing.T) {
	stray := Code(60000)

	if stray.String() != "" {
		t.Errorf("the stray Code spells %q", stray.String())
	}
	if !stray.ShortCode().IsZero() {
		t.Errorf("the stray Code has short code %q", stray.ShortCode())
	}

	marshalled, err := json.Marshal(stray)
	if err != nil {
		t.Fatalf("marshalling the stray Code: %v", err)
	}
	if string(marshalled) != "null" {
		t.Errorf("the stray Code marshalled as %s, want null", marshalled)
	}
}

func TestMarshalText(t *testing.T) {
	marshalled, err := nld.MarshalText()
	if err != nil {
		t.Fatalf("marshalling nld as text: %v", err)
	}
	if string(marshalled) != "nld" {
		t.Errorf("nld marshalled as %q, want nld", marshalled)
	}

	marshalled, err = Code(0).MarshalText()
	if err != nil {
		t.Fatalf("marshalling the zero Code as text: %v", err)
	}
	if len(marshalled) != 0 {
		t.Errorf("the zero Code marshalled as %q, want nothing", marshalled)
	}
}

func TestUnmarshalText(t *testing.T) {
	cases := map[string]Code{
		"nld":        nld,
		"nl":         nld,
		"dut":        nld,
		"EN":         eng,
		"xx":         0,
		"":           0,
		"nederlands": 0,
	}

	for input, expected := range cases {
		var code Code
		err := code.UnmarshalText([]byte(input))
		if err != nil {
			t.Fatalf("unmarshalling %q as text: %v", input, err)
		}
		if code != expected {
			t.Errorf("%q unmarshalled as %q, want %q", input, code, expected)
		}
	}
}

func TestShortCode(t *testing.T) {
	if nld.ShortCode() != (ShortCode{'n', 'l'}) {
		t.Errorf("nld has short code %q, want nl", nld.ShortCode())
	}
	if eng.ShortCode() != (ShortCode{'e', 'n'}) {
		t.Errorf("eng has short code %q, want en", eng.ShortCode())
	}
	if !Code(0).ShortCode().IsZero() {
		t.Errorf("the zero Code has short code %q", Code(0).ShortCode())
	}
	if (ShortCode{}).String() != "" {
		t.Errorf("the zero ShortCode spells %q", ShortCode{}.String())
	}
}
