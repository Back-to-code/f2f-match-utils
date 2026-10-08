package rules_test

import (
	"math/rand/v2"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
	"github.com/Back-to-code/f2f-match-utils/rules/ruleindex"
	"github.com/Back-to-code/f2f-match-utils/rules/textindex"
)

// match is one set matching one text, with the include that matched it first.
type match struct {
	set, text, include int
}

func viaTexts(texts *textindex.Index[int], sets map[int]rules.Set) map[match]bool {
	out := map[match]bool{}
	for id, set := range sets {
		compiled, _ := rules.Compile(set)
		for _, hit := range texts.Match(compiled) {
			out[match{id, hit.ID, hit.Include}] = true
		}
	}
	return out
}

func viaRules(sets *ruleindex.Index[int], texts map[int]string) map[match]bool {
	out := map[match]bool{}
	for id, text := range texts {
		for _, found := range sets.Match(text) {
			out[match{found.ID, id, found.Include}] = true
		}
	}
	return out
}

func compareDirections(t *testing.T, textIx *textindex.Index[int], ruleIx *ruleindex.Index[int], sets map[int]rules.Set, texts map[int]string) int {
	t.Helper()
	byTexts, byRules := viaTexts(textIx, sets), viaRules(ruleIx, texts)
	for found := range byTexts {
		if !byRules[found] {
			t.Fatalf("only the text index finds set %d on %q (include %d)", found.set, texts[found.text], found.include)
		}
	}
	for found := range byRules {
		if !byTexts[found] {
			t.Fatalf("only the rule index finds set %d on %q (include %d)", found.set, texts[found.text], found.include)
		}
	}
	t.Logf("%d matches, the same both ways", len(byTexts))
	return len(byTexts)
}

// TestDirectionsAgree matches the same sets and texts both ways, through writes to both indexes, and
// expects the same set, text and include every time.
func TestDirectionsAgree(t *testing.T) {
	gen := synth.New(11, 250)
	rng := rand.New(rand.NewPCG(11, 11))
	set := func() rules.Set {
		set := gen.Set(1+rng.IntN(25), rng.IntN(4))
		if rng.IntN(5) == 0 {
			set.Includes[rng.IntN(len(set.Includes))] = "*er"
		}
		return set
	}

	textIx, texts := textindex.New[int](), map[int]string{}
	for id := range 3000 {
		texts[id] = gen.Text()
		textIx.Set(id, texts[id])
	}
	ruleIx, sets := ruleindex.New[int](), map[int]rules.Set{}
	for id := range 200 {
		sets[id] = set()
	}
	ruleIx.SetAll(func(yield func(int, rules.Set) bool) {
		for id, set := range sets {
			if !yield(id, set) {
				return
			}
		}
	})
	if found := compareDirections(t, textIx, ruleIx, sets, texts); found < 1000 {
		t.Fatalf("only %d matches: the test tests little", found)
	}

	for round := range 3 {
		for range 300 {
			id := rng.IntN(3500)
			if rng.IntN(4) == 0 {
				textIx.Delete(id)
				delete(texts, id)
				continue
			}
			texts[id] = gen.Text()
			textIx.Set(id, texts[id])
		}
		for range 60 {
			id := rng.IntN(250)
			if rng.IntN(4) == 0 {
				ruleIx.Delete(id)
				delete(sets, id)
				continue
			}
			sets[id] = set()
			ruleIx.Set(id, sets[id])
		}
		if round == 2 {
			ruleIx.Compact()
		}
		compareDirections(t, textIx, ruleIx, sets, texts)
	}
}
