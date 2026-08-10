package langcode

import (
	"hash/fnv"
	"testing"
)

// A packing that is not exactly reversible hands out the wrong language. All 17,576 codes
// the encoding can hold are cheap enough to just check.
func TestPackRoundTripsEveryCode(t *testing.T) {
	for first := byte('a'); first <= 'z'; first++ {
		for second := byte('a'); second <= 'z'; second++ {
			for third := byte('a'); third <= 'z'; third++ {
				code := [3]byte{first, second, third}
				text := string(code[:])

				packed, ok := PackBytes(code)
				if !ok {
					t.Fatalf("%s does not pack", text)
				}
				if packed == 0 {
					t.Fatalf("%s packs to the zero value, which means no language", text)
				}

				unpacked, ok := Unpack(packed)
				if !ok {
					t.Fatalf("%s does not unpack", text)
				}
				if unpacked != code {
					t.Fatalf("%s does not survive the round trip, it came back as %s", text, string(unpacked[:]))
				}
			}
		}
	}
}

// The zero Code always means no language, and a cast of a stray number never spells one.
func TestUnpackRejectsWhatIsNotACode(t *testing.T) {
	for _, packed := range []uint16{0, Max + 1, 30000, 65535} {
		_, ok := Unpack(packed)
		if ok {
			t.Errorf("%d unpacked", packed)
		}
	}
}

// Case is not part of a code, so it is normalized away rather than rejected.
func TestPackIsCaseInsensitive(t *testing.T) {
	lower, ok := PackBytes([3]byte{'n', 'l', 'd'})
	if !ok {
		t.Fatal("nld does not pack")
	}

	upper, ok := PackBytes([3]byte{'N', 'L', 'D'})
	if !ok {
		t.Fatal("NLD does not pack")
	}
	if upper != lower {
		t.Errorf("NLD packed as %d, nld as %d", upper, lower)
	}

	fromText, err := Pack("NlD")
	if err != nil {
		t.Fatalf("NlD does not pack: %v", err)
	}
	if fromText != lower {
		t.Errorf("NlD packed as %d, nld as %d", fromText, lower)
	}
}

// Skipping is only ever allowed to reach three letters. Padding or truncating would turn
// one code into another.
func TestPackSkipsNonLettersButStillNeedsThree(t *testing.T) {
	nld, err := Pack("nld")
	if err != nil {
		t.Fatalf("nld does not pack: %v", err)
	}

	for _, code := range []string{"n.l.d", "N-L-D", " nld ", "n l d"} {
		packed, err := Pack(code)
		if err != nil {
			t.Errorf("%q did not pack: %v", code, err)

			continue
		}
		if packed != nld {
			t.Errorf("%q packed as another code", code)
		}
	}

	for _, code := range []string{"", "ab", "a b", "n/a", "abcd", "a.b.c.d", "1234"} {
		_, err := Pack(code)
		if err == nil {
			t.Errorf("%q packed", code)
		}
	}
}

func TestPackBytesRejectsWhatIsNotALetter(t *testing.T) {
	for _, code := range [][3]byte{{'a', 'b', '1'}, {}, {'a', 'b', ' '}, {'a', 'b', '-'}} {
		_, ok := PackBytes(code)
		if ok {
			t.Errorf("%v packed", code)
		}
	}
}

// The generated table is a list of hashes and nothing else, so the hash is part of its
// meaning. Changing it silently would turn every entry into a name nothing resolves to.
func TestFNV32aIsStable(t *testing.T) {
	cases := map[string]uint32{
		"":                        2166136261,
		"nld":                     196030137,
		"engels":                  3366934055,
		"nederlandse gebarentaal": 3162973534,
	}

	for input, expected := range cases {
		hash := FNV32a(input)
		if hash != expected {
			t.Errorf("the hash of %q changed, it is %d and was %d", input, hash, expected)
		}

		reference := fnv.New32a()
		reference.Write([]byte(input))
		if hash != reference.Sum32() {
			t.Errorf("%q no longer matches hash/fnv, it hashes to %d and hash/fnv gives %d", input, hash, reference.Sum32())
		}
	}
}
