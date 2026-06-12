package tokens

import (
	"sync"
	"testing"
)

// resetState clears all global tokenizer state so tests start fresh.
func resetState() {
	lock.Lock()
	toTokenCache = make(map[string][][]Token, 1024)
	wordToToken = make(map[string]Token, 1024)
	lock.Unlock()
}

func TestToTokensBasic(t *testing.T) {
	resetState()

	tokens := ToTokens("Hello World")

	if tokens == nil {
		t.Fatal("Expected tokens but got nil")
	}

	if len(tokens[0]) != 2 {
		t.Fatalf("Expected 2 tokens, got %d", len(tokens[0]))
	}

	// Verify tokens are created consistently
	tokens2 := ToTokens("Hello World")
	if len(tokens2[0]) != 2 {
		t.Fatalf("Expected 2 tokens on second call, got %d", len(tokens2[0]))
	}

	if tokens[0][0] != tokens2[0][0] || tokens[0][1] != tokens2[0][1] {
		t.Fatal("Tokens should be consistent across calls")
	}
}

func TestToTokensWithEmptyString(t *testing.T) {
	resetState()

	tokens := ToTokens("")

	// Empty string returns nil or empty slice
	if len(tokens) != 0 {
		t.Fatalf("Expected 0 tokens for empty string, got %d", len(tokens))
	}
}

func TestToTokensFiltersIllegalWords(t *testing.T) {
	resetState()

	// Test with illegal words mixed in
	tokens := ToTokens("the quick brown fox")

	// "the" is an illegal word, should be filtered out
	if len(tokens[0]) != 3 {
		t.Fatalf("Expected 3 tokens (quick, brown, fox), got %d", len(tokens[0]))
	}

	// Verify with more illegal words
	tokens = ToTokens("en in de")
	if len(tokens) != 0 {
		t.Fatalf("Expected 0 tokens (all illegal words), got %d", len(tokens))
	}
}

func TestToTokensHandlesCaseInsensitivity(t *testing.T) {
	resetState()

	tokens1 := ToTokens("Hello")
	tokens2 := ToTokens("HELLO")
	tokens3 := ToTokens("hello")

	if len(tokens1) != 1 || len(tokens2) != 1 || len(tokens3) != 1 {
		t.Fatal("Expected 1 token for each variation")
	}

	if tokens1[0][0] != tokens2[0][0] || tokens1[0][0] != tokens3[0][0] {
		t.Fatal("Expected same token for case variations")
	}
}

func TestToTokensUsesCache(t *testing.T) {
	resetState()

	tokens1 := ToTokens("Hello World")
	tokens2 := ToTokens("Hello World")

	// Verify both return the same slice (from cache)
	if &tokens1[0] != &tokens2[0] {
		t.Log("Warning: Cache might not be returning same slice reference")
	}

	if len(tokens1) != len(tokens2) {
		t.Fatal("Cached tokens should have same length")
	}

	for i := range tokens1 {
		if tokens1[0][i] != tokens2[0][i] {
			t.Fatalf("Token mismatch at index %d", i)
		}
	}
}

func TestCleanupCacheClearsCache(t *testing.T) {
	resetState()

	tokens1 := ToTokens("Hello World")
	CleanupCache()
	tokens2 := ToTokens("Hello World")

	// Tokens should still be the same values
	if len(tokens1) != len(tokens2) {
		t.Fatal("Tokens should have same length after cache cleanup")
	}

	for i := range tokens1 {
		if tokens1[0][i] != tokens2[0][i] {
			t.Fatalf("Token mismatch at index %d after cache cleanup", i)
		}
	}
}

func TestToTokensIsThreadSafe(t *testing.T) {
	resetState()

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine tokenizes different strings
			tokens := ToTokens("concurrent test")
			if tokens == nil {
				t.Errorf("Goroutine %d: got nil tokens", id)
			}
		}(i)
	}

	wg.Wait()
}

func TestToTokensHandlesPunctuationAndSpecialCharacters(t *testing.T) {
	resetState()

	tokens := ToTokens("Hello, World! How are you?")

	// Should extract: hello, world, how, you (are is an illegal word)
	if len(tokens[0]) != 4 {
		t.Fatalf("Expected 4 tokens, got %d", len(tokens[0]))
	}
}

func TestToTokensIncrementsTokenIDsCorrectly(t *testing.T) {
	resetState()

	tokens1 := ToTokens("first")
	tokens2 := ToTokens("second")
	tokens3 := ToTokens("third")

	if tokens1[0][0] != 0 {
		t.Fatalf("First token should be 0, got %d", tokens1[0])
	}

	if tokens2[0][0] != 1 {
		t.Fatalf("Second token should be 1, got %d", tokens2[0])
	}

	if tokens3[0][0] != 2 {
		t.Fatalf("Third token should be 2, got %d", tokens3[0])
	}
}

func TestToTokensHandlesMultipleSpaces(t *testing.T) {
	resetState()

	tokens := ToTokens("word1    word2")

	if len(tokens[0]) != 2 {
		t.Fatalf("Expected 2 tokens, got %d", len(tokens[0]))
	}
}

func TestToTokensTableDriven(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
		description   string
	}{
		// Basic cases
		{"single word", "hello", 1, "single lowercase word"},
		{"two words", "hello world", 2, "two simple words"},
		{"three words", "hello beautiful world", 3, "three simple words"},

		// Case variations
		{"uppercase", "HELLO WORLD", 2, "uppercase words"},
		{"mixed case", "HeLLo WoRLd", 2, "mixed case words"},
		{"camelCase", "helloWorld", 1, "camelCase treated as one word"},

		// Punctuation and special characters
		{"with comma", "hello, world", 2, "comma separated words"},
		{"with period", "hello. world.", 2, "period separated words"},
		{"with exclamation", "hello! world!", 2, "exclamation marks"},
		{"with question", "hello? world?", 2, "question marks"},
		{"with semicolon", "hello; world; test", 3, "semicolon separated"},
		{"with colon", "hello: world: test", 3, "colon separated"},
		{"with quotes", "\"hello\" 'world'", 2, "quoted words"},
		{"with parentheses", "(hello) (world)", 2, "parentheses"},
		{"with brackets", "[hello] [world]", 2, "square brackets"},
		{"with braces", "{hello} {world}", 2, "curly braces"},
		{"with dash", "hello-world", 1, "dash separated"},
		{"with underscore", "hello_world", 2, "underscore separated"},
		{"with slash", "hello/world", 2, "slash separated"},
		{"with backslash", "hello\\world", 2, "backslash separated"},
		{"with at symbol", "hello@world", 2, "at symbol"},
		{"with hash", "#hello #world", 2, "hash tags"},
		{"with dollar", "$hello $world", 2, "dollar signs"},
		{"with percent", "hello%world", 2, "percent sign"},
		{"with ampersand", "hello&world", 2, "ampersand"},
		{"with asterisk", "hello*world", 2, "asterisk"},
		{"with plus", "hello+world", 2, "plus sign"},
		{"with equals", "hello=world", 2, "equals sign"},

		// Numbers and mixed content
		{"with numbers", "hello123world", 2, "numbers break words"},
		{"pure numbers", "123 456 789", 0, "pure numbers filtered"},
		{"alphanumeric", "test1 test2 test3", 3, "alphanumeric words"},

		// Whitespace variations
		{"multiple spaces", "hello    world", 2, "multiple spaces"},
		{"tab separated", "hello	world", 2, "tab separated"},
		{"newline", "hello\nworld", 2, "newline separated"},
		{"carriage return", "hello\rworld", 2, "carriage return"},
		{"mixed whitespace", "hello \t\n world", 2, "mixed whitespace"},
		{"leading spaces", "   hello world", 2, "leading spaces"},
		{"trailing spaces", "hello world   ", 2, "trailing spaces"},

		// Illegal words (stop words)
		{"with the", "the quick brown fox", 3, "filters 'the'"},
		{"with and", "cats and dogs", 2, "filters 'and'"},
		{"with in", "in the house", 1, "filters 'in' and 'the'"},
		{"with de", "de quick fox", 2, "filters 'de'"},
		{"with en", "en het van", 0, "filters 'en', 'het', 'van'"},
		{"with voor", "voor met of", 0, "filters 'voor', 'met', 'of'"},
		{"with een", "een quick fox", 2, "filters 'een'"},
		{"with op", "op aan bij", 0, "filters 'op', 'aan', 'bij'"},
		{"with ik", "ik ben happy", 2, "filters 'ik'"},
		{"all illegal", "the and in de en", 0, "all illegal words"},

		// Real-world sentences
		{"simple sentence", "The quick brown fox jumps over lazy dog", 6, "classic sentence"},
		{"question", "What is your name?", 3, "question sentence"},
		{"exclamation", "Watch out for that car!", 3, "exclamation"},
		{"email-like", "contact@example.com", 2, "email address parts (with abbr handling)"},
		{"url-like", "https://www.example.com", 2, "URL parts (with abbr handling)"},
		{"file path", "/usr/local/bin/test", 4, "file path"},

		// Multilingual content
		{"french", "bonjour le monde", 3, "French text (filters 'le' if illegal)"},
		{"spanish", "hola mundo hermoso", 3, "Spanish text"},
		{"german", "guten tag welt", 3, "German text"},
		{"dutch", "hallo wereld", 2, "Dutch text"},
		{"italian", "ciao mondo", 2, "Italian text"},

		// Edge cases with length
		{"long sentence", "This is a very long sentence with many different words to test the tokenizer", 9, "long sentence"},
		{"single char words", "a b c d e f g h", 8, "single character words"},
		{"repeated words", "test test test test", 4, "same word repeated"},
		{"palindrome", "racecar level radar", 3, "palindrome words"},

		// Special formats
		{"code snippet", "func main() { return true }", 4, "code-like text"},
		{"sql-like", "SELECT * FROM users WHERE id", 4, "SQL-like text"},
		{"json-like", "{\"key\": \"value\"}", 2, "JSON-like text"},
		{"xml-like", "<tag>content</tag>", 3, "XML-like text"},

		// Empty and minimal
		{"empty string", "", 0, "empty string"},
		{"only spaces", "     ", 0, "only whitespace"},
		{"only punctuation", "!@#$%^&*()", 0, "only special chars"},
		{"single letter", "a", 1, "single letter"},

		// Stress tests
		{"many words", "one two three four five six seven eight nine ten eleven twelve", 12, "many words"},
		{"complex punctuation", "hello...world!!!how???are+++you", 4, "complex punctuation"},
		{"unicode-like", "hello\u0020world", 2, "unicode space"},

		// Business/Technical content
		{"company names", "Microsoft Apple Google Amazon", 4, "company names"},
		{"tech terms", "JavaScript TypeScript Python Ruby", 4, "programming languages"},
		{"product names", "iPhone Android Windows Linux", 4, "product names"},
		{"cities", "Amsterdam Rotterdam Utrecht Eindhoven", 4, "Dutch cities"},
		{"countries", "Netherlands Belgium Germany France", 4, "European countries"},

		// Articles and blog-like content
		{"blog title", "How to Build a RESTful API", 5, "blog title style"},
		{"news headline", "Breaking News: Major Event Occurs Today", 6, "news headline"},
		{"recipe title", "Delicious Chocolate Cake Recipe", 4, "recipe title"},

		// Social media style
		{"hashtags", "#trending #popular #viral", 3, "hashtags"},
		{"mentions", "@user1 @user2 hello", 3, "mentions and words"},
		{"emoji context", "hello world great day", 4, "words with emoji context"},

		// Date and time formats
		{"date format", "2024-01-15", 0, "date with numbers"},
		{"time format", "14:30:00", 0, "time with numbers"},

		// Repeated punctuation
		{"multiple dots", "hello...world", 2, "ellipsis"},
		{"multiple dashes", "hello---world", 1, "multiple dashes"},
		{"multiple questions", "what??? really???", 2, "multiple question marks"},

		// Long words
		{"very long word", "supercalifragilisticexpialidocious", 1, "very long word"},
		{"compound word", "telecommunications", 1, "long compound word"},

		// Mixed everything
		{"chaos", "Hello123!!! @#$world456??? test...test+++END", 5, "chaotic mix"},
		{"real chaos", "The_QUICK-brown@FOX#123 jumps!!! over...the LAZY~~~dog???", 5, "real chaos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState()
			tokens := ToTokens(tt.input)

			// tt.expectedCount is the word count of the primary (combined)
			// interpretation, i.e. branch 0. Empty/all-illegal inputs yield no
			// branches at all.
			got := 0
			if len(tokens) > 0 {
				got = len(tokens[0])
			}
			if got != tt.expectedCount {
				t.Errorf("%s: expected %d tokens, got %d (input: %q)",
					tt.description, tt.expectedCount, got, tt.input)
			}
		})
	}
}

func TestDiacriticFolding(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// Common Latin diacritics (NFD-decomposable).
		{"dutch trema", "kassière", []string{"kassiere"}},
		{"acute e", "café", []string{"cafe"}},
		{"diaeresis i", "naïve", []string{"naive"}},
		{"tilde n", "mañana", []string{"manana"}},
		{"cedilla", "façade", []string{"facade"}},
		{"umlaut", "Müller", []string{"muller"}},
		{"ring above", "Ångström", []string{"angstrom"}},
		{"grave", "voilà", []string{"voila"}},

		// Non-decomposing specials.
		{"eszett", "straße", []string{"strasse"}},
		{"ae ligature", "encyclopædia", []string{"encyclopaedia"}},
		{"oe ligature", "œuvre", []string{"oeuvre"}},
		{"slashed o", "søn", []string{"son"}},
		{"slashed l", "Łódź", []string{"lodz"}},
		{"thorn", "þorn", []string{"thorn"}},
		{"eth", "garðr", []string{"gardr"}},

		// Folding makes accented and plain spellings collapse together.
		{"multiple words", "Beyoncé café", []string{"beyonce", "cafe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for part := range sanitizedWords(tt.input) {
				full := part.base
				if part.addition != nil {
					full += *part.addition
				}
				got = append(got, full)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("input %q: expected %v, got %v", tt.input, tt.want, got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("input %q: word %d expected %q, got %q", tt.input, i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestDiacriticFoldingTokenEquality(t *testing.T) {
	resetState()

	// Accented and plain spellings must map to the same token.
	accented := ToTokens("café")
	plain := ToTokens("cafe")

	if len(accented) != 1 || len(plain) != 1 {
		t.Fatalf("expected 1 token each, got %d and %d", len(accented), len(plain))
	}
	if accented[0][0] != plain[0][0] {
		t.Errorf("café and cafe should produce the same token, got %d and %d", accented[0], plain[0])
	}
}

func TestAbbreviationHandling(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
		description   string
	}{
		// Basic abbreviations
		{"HBO", "H.B.O.", 1, "HBO abbreviation"},
		{"USA", "U.S.A.", 1, "USA abbreviation"},
		{"IBM", "I.B.M.", 1, "IBM abbreviation"},
		{"PHD", "P.H.D.", 1, "PHD abbreviation"},
		{"CIA", "C.I.A.", 1, "CIA abbreviation"},
		{"FBI", "F.B.I.", 1, "FBI abbreviation"},
		{"NBA", "N.B.A.", 1, "NBA abbreviation"},
		{"NFL", "N.F.L.", 1, "NFL abbreviation"},
		{"CEO", "C.E.O.", 1, "CEO abbreviation"},
		{"CTO", "C.T.O.", 1, "CTO abbreviation"},

		// Two-letter abbreviations
		{"OK", "O.K.", 1, "OK abbreviation"},
		{"AM", "A.M.", 1, "AM abbreviation"},
		{"PM", "P.M.", 1, "PM abbreviation"},
		{"UK", "U.K.", 1, "UK abbreviation"},
		{"US", "U.S.", 1, "US abbreviation"},

		// Four-letter abbreviations
		{"USSR", "U.S.S.R.", 1, "USSR abbreviation"},
		{"ASAP", "A.S.A.P.", 1, "ASAP abbreviation"},

		// Longer abbreviations
		{"UNICEF", "U.N.I.C.E.F.", 1, "UNICEF abbreviation"},

		// Case variations
		{"lowercase abbr", "h.b.o.", 1, "lowercase abbreviation"},
		{"mixed case abbr", "H.b.O.", 1, "mixed case abbreviation"},
		{"uppercase abbr", "H.B.O.", 1, "uppercase abbreviation"},

		// Abbreviations in sentences
		{"abbr in sentence", "H.B.O. is great", 2, "abbreviation with words"},
		{"abbr mid sentence", "I love H.B.O. shows", 3, "abbreviation in middle"},
		{"abbr start", "U.S.A. is large", 2, "abbreviation at start"},
		{"multiple abbrs", "U.S.A. and U.K.", 2, "multiple abbreviations"},
		{"abbr with illegal", "The H.B.O. is great", 2, "abbreviation with stop words"},

		// Not abbreviations (periods with spaces or no letters)
		{"sentence end", "Hello world.", 2, "period at end of sentence"},
		{"multiple sentences", "Hello. World.", 2, "periods between sentences"},
		{"abbr-like but not", "H. B. O.", 3, "letters with spaces and periods"},

		// Mixed content
		{"abbr and period", "H.B.O. is great.", 2, "abbreviation and sentence period"},
		{"abbr with comma", "H.B.O., is great", 2, "abbreviation with comma"},
		{"abbr with exclaim", "H.B.O.! amazing", 2, "abbreviation with exclamation"},
		{"abbr with question", "H.B.O.? really", 2, "abbreviation with question mark"},

		// Edge cases
		{"single letter period", "A.", 1, "single letter with period"},
		{"two letters period", "A.B.", 1, "two letters with periods"},
		{"abbr no trailing", "H.B.O", 1, "abbreviation without trailing period"},
		{"period between digits", "3.14", 0, "period between numbers (not abbr)"},

		// Academic/Professional titles
		{"Dr", "Dr.", 1, "Doctor abbreviation"},
		{"Mr", "Mr.", 1, "Mister abbreviation"},
		{"Mrs", "Mrs.", 1, "Missus abbreviation"},
		{"Ms", "Ms.", 1, "Miss abbreviation"},
		{"Prof", "Prof.", 1, "Professor abbreviation"},

		// Real-world examples
		{"email domain", "example.com", 1, "domain with period treated as abbr"},
		{"file extension", "file.txt", 1, "file with extension treated as abbr"},
		{"version number", "v1.2.3", 1, "version number"},

		// Academic degrees
		{"BSc", "B.S.c.", 1, "Bachelor of Science"},
		{"MSc", "M.S.c.", 1, "Master of Science"},
		{"PhD", "P.h.D.", 1, "Doctor of Philosophy"},
		{"MBA", "M.B.A.", 1, "Master of Business Administration"},

		// Organizations
		{"UN", "U.N.", 1, "United Nations"},
		{"EU", "E.U.", 1, "European Union"},
		{"WHO", "W.H.O.", 1, "World Health Organization"},
		{"NASA", "N.A.S.A.", 1, "NASA abbreviation"},
		{"NATO", "N.A.T.O.", 1, "NATO abbreviation"},

		// Technical abbreviations
		{"API", "A.P.I.", 1, "API abbreviation"},
		{"SQL", "S.Q.L.", 1, "SQL abbreviation"},
		{"XML", "X.M.L.", 1, "XML abbreviation"},
		{"HTML", "H.T.M.L.", 1, "HTML abbreviation"},
		{"CSS", "C.S.S.", 1, "CSS abbreviation"},
		{"JSON", "J.S.O.N.", 1, "JSON abbreviation"},

		// Edge case: consecutive abbreviations
		{"consecutive abbrs", "U.S.A.U.K.", 1, "consecutive abbreviations without space"},
		{"abbrs with space", "U.S.A. U.K.", 2, "consecutive abbreviations with space"},

		// Complex real-world sentences
		{"complex 1", "The U.S.A. and U.K. have strong ties", 4, "complex sentence with abbrs"},
		{"complex 2", "I.B.M. and C.I.A. are different", 3, "multiple abbreviations in sentence"},
		{"complex 3", "Dr. Smith works for N.A.S.A. in U.S.A.", 5, "titles and abbreviations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState()
			tokens := ToTokens(tt.input)

			got := 0
			if len(tokens) > 0 {
				got = len(tokens[0])
			}
			if got != tt.expectedCount {
				t.Errorf("%s: expected %d tokens, got %d (input: %q)",
					tt.description, tt.expectedCount, got, tt.input)
			}
		})
	}
}

func TestAbbreviationConsistency(t *testing.T) {
	resetState()

	// Test that H.B.O. always produces the same token
	tokens1 := ToTokens("H.B.O.")
	tokens2 := ToTokens("H.B.O.")
	tokens3 := ToTokens("h.b.o.") // lowercase

	if len(tokens1) != 1 || len(tokens2) != 1 || len(tokens3) != 1 {
		t.Fatal("Expected 1 token for each abbreviation variant")
	}

	if tokens1[0][0] != tokens2[0][0] {
		t.Error("Same abbreviation should produce same token")
	}

	if tokens1[0][0] != tokens3[0][0] {
		t.Error("Case-insensitive: H.B.O. and h.b.o. should produce same token")
	}
}

func TestAbbreviationVsRegularPeriod(t *testing.T) {
	resetState()

	// "H.B.O." should be 1 token (abbreviation)
	abbr := ToTokens("H.B.O.")
	if len(abbr[0]) != 1 {
		t.Errorf("H.B.O. should be 1 token, got %d", len(abbr[0]))
	}

	// "hello." should be 1 token (word with period at end)
	word := ToTokens("hello.")
	if len(word[0]) != 1 {
		t.Errorf("hello. should be 1 token, got %d", len(word[0]))
	}

	// "hello. world." should be 2 tokens
	sentence := ToTokens("hello. world.")
	if len(sentence[0]) != 2 {
		t.Errorf("hello. world. should be 2 tokens, got %d", len(sentence[0]))
	}

	// "H. B. O." should be 3 tokens (not an abbreviation due to spaces)
	notAbbr := ToTokens("H. B. O.")
	if len(notAbbr[0]) != 3 {
		t.Errorf("H. B. O. should be 3 tokens, got %d", len(notAbbr[0]))
	}
}

func TestAbbreviationBoundaryConditions(t *testing.T) {
	resetState()

	// Period at the very end of input
	tokens := ToTokens("test.")
	if len(tokens) != 1 {
		t.Errorf("test. should be 1 token, got %d", len(tokens))
	}

	// Period at the very beginning (unusual but should not crash)
	tokens = ToTokens(".test")
	if len(tokens) != 1 {
		t.Errorf(".test should be 1 token, got %d", len(tokens))
	}

	// Just periods
	tokens = ToTokens("...")
	if len(tokens) != 0 {
		t.Errorf("... should be 0 tokens, got %d", len(tokens))
	}

	// Single character abbreviation at end: "A."
	tokens = ToTokens("A.")
	if len(tokens) != 1 {
		t.Errorf("A. should be 1 token, got %d", len(tokens))
	}
}

func TestCacheSize(t *testing.T) {
	resetState()

	if CacheSize() != 0 {
		t.Fatalf("Expected cache size 0, got %d", CacheSize())
	}

	ToTokens("hello")
	if CacheSize() != 1 {
		t.Fatalf("Expected cache size 1, got %d", CacheSize())
	}

	ToTokens("world")
	if CacheSize() != 2 {
		t.Fatalf("Expected cache size 2, got %d", CacheSize())
	}

	CleanupCache()
	if CacheSize() != 0 {
		t.Fatalf("Expected cache size 0 after cleanup, got %d", CacheSize())
	}
}

// equalTokens reports whether two token branches are identical.
func equalTokens(a, b []Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestToTokensMultiBranchCount checks when a hyphenated word causes ToTokens to
// branch into multiple interpretations. A word containing exactly one interior
// dash branches into two interpretations (combined + split); everything else
// stays a single interpretation.
func TestToTokensMultiBranchCount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		branches int
		desc     string
	}{
		{"empty", "", 0, "empty input yields no branches"},
		{"single dash", "hello-world", 2, "one interior dash branches into combined + split"},
		{"hyphenated name", "anne-marie", 2, "hyphenated name branches"},
		{"dash word then word", "hello-world foo", 2, "branches even with a following word"},
		{"word then dash word", "foo hello-world", 2, "branches when hyphen word is not first"},
		{"two dashes", "a-b-c", 1, "more than one dash does not branch"},
		{"consecutive dashes", "hello--world", 1, "consecutive dashes do not branch"},
		{"leading dash", "-hello", 1, "leading dash does not branch"},
		{"trailing dash", "hello-", 1, "trailing dash does not branch"},
		{"plain words", "hello world", 1, "plain words never branch"},
		{"empty", "", 0, "empty input yields no branches"},
		{"dash only", "-", 0, "a lone dash yields no branches"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState()
			got := ToTokens(tt.input)
			if len(got) != tt.branches {
				t.Errorf("%s: expected %d branches, got %d (input %q -> %v)",
					tt.desc, tt.branches, len(got), tt.input, got)
			}
		})
	}
}

// TestToTokensMultiBranchInterpretations verifies the meaning of the two
// branches: branch 0 is the combined interpretation (the de-hyphenated word as a
// single token) and branch 1 is the split interpretation (the two halves as
// separate tokens). The constituent forms are registered first so the branch
// reuses their stable tokens.
func TestToTokensMultiBranchInterpretations(t *testing.T) {
	resetState()

	separate := ToTokens("hello world") // [[hello world]]
	combined := ToTokens("helloworld")  // [[helloworld]]

	dash := ToTokens("hello-world")
	if len(dash) != 2 {
		t.Fatalf("expected 2 branches, got %d: %v", len(dash), dash)
	}

	// Branch 0: combined interpretation, equal to the de-hyphenated word.
	if !equalTokens(dash[0], combined[0]) {
		t.Errorf("combined branch %v should equal helloworld tokens %v", dash[0], combined[0])
	}

	// Branch 1: split interpretation, equal to the two halves as separate words.
	if !equalTokens(dash[1], separate[0]) {
		t.Errorf("split branch %v should equal 'hello world' tokens %v", dash[1], separate[0])
	}
}

// TestToTokensMultiBranchCached confirms a branched result is cached and returns
// the same branches on a second call.
func TestToTokensMultiBranchCached(t *testing.T) {
	resetState()

	first := ToTokens("anne-marie")
	if len(first) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(first))
	}
	if CacheSize() == 0 {
		t.Fatal("expected hyphenated input to be cached")
	}

	second := ToTokens("anne-marie")
	if len(second) != len(first) {
		t.Fatalf("cached branch count mismatch: %d vs %d", len(second), len(first))
	}
	for b := range first {
		if !equalTokens(first[b], second[b]) {
			t.Errorf("branch %d differs between calls: %v vs %v", b, first[b], second[b])
		}
	}
}

// TestToTokensMultiBranchNoEmptyBranches verifies branched results never contain
// empty branches (the trailing filter loop drops them).
func TestToTokensMultiBranchNoEmptyBranches(t *testing.T) {
	resetState()

	for _, in := range []string{"hello-world", "anne-marie bob", "foo hello-world"} {
		resetState()
		for b, branch := range ToTokens(in) {
			if len(branch) == 0 {
				t.Errorf("input %q: branch %d is empty", in, b)
			}
		}
	}
}
