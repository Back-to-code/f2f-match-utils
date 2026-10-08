package ruleindex

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
)

// reference matches every set on its own, the semantics the index must reproduce.
type reference map[int]*rules.Compiled

func (ref reference) match(text string) map[int]int {
	words := rules.Words(text)
	out := map[int]int{}
	for id, set := range ref {
		if include := set.MatchWords(words); include >= 0 {
			out[id] = include
		}
	}
	return out
}

func check(t *testing.T, ix *Index[int], ref reference, texts []string) {
	t.Helper()
	for _, text := range texts {
		want := ref.match(text)
		got := map[int]int{}
		for _, match := range ix.Match(text) {
			if _, dup := got[match.ID]; dup {
				t.Fatalf("%q: set %d matched twice", text, match.ID)
			}
			got[match.ID] = match.Include
			set := ref[match.ID]
			if set == nil || match.Rule != set.Set.Includes[match.Include] {
				t.Fatalf("%q: set %d rule %q", text, match.ID, match.Rule)
			}
		}
		if !maps.Equal(got, want) {
			t.Fatalf("%q: got %v, want %v", text, got, want)
		}
	}
}

func sample(gen *synth.Generator, count int) []string {
	texts := make([]string, count)
	for idx := range texts {
		texts[idx] = gen.Text()
	}
	return texts
}

func TestMatchesLikeTheSets(t *testing.T) {
	gen := synth.New(1, 300)
	texts := sample(gen, 3000)
	ix := New[int]()
	ref := reference{}
	all := map[int]rules.Set{}
	for id := range 400 {
		set := gen.Set(1+id%30, id%4)
		all[id] = set
		ref[id] = rules.MustCompile(set)
	}
	if problems := ix.SetAll(maps.All(all)); len(problems) > 0 {
		t.Fatal(problems)
	}
	check(t, ix, ref, texts)
	matched := 0
	for _, text := range texts {
		matched += len(ix.Match(text))
	}
	if matched < len(texts) {
		t.Fatalf("only %d matches over %d texts: the test tests little", matched, len(texts))
	}
}

func TestWritesAndMerges(t *testing.T) {
	gen := synth.New(2, 200)
	texts := sample(gen, 600)
	ix := New[int]()
	ix.opts.syncMergeWeight = 200
	ref := reference{}
	rng := rand.New(rand.NewPCG(2, 2))
	for step := range 1500 {
		key := rng.IntN(120)
		switch {
		case step%11 == 0:
			ix.Delete(key)
			delete(ref, key)
		default:
			set := gen.Set(1+step%9, step%3)
			ix.Set(key, set)
			ref[key] = rules.MustCompile(set)
		}
		if step%100 == 0 {
			check(t, ix, ref, texts)
		}
	}
	ix.mu.Lock()
	ix.waitIdle()
	ix.mu.Unlock()
	check(t, ix, ref, texts)
	if ix.Len() != len(ref) {
		t.Fatalf("len %d, want %d", ix.Len(), len(ref))
	}
	ix.Compact()
	if stats := ix.Stats(); stats.Segments != 1 || stats.DeadSets != 0 {
		t.Fatalf("after compact: %+v", stats)
	}
	check(t, ix, ref, texts)
}

func TestFewSegments(t *testing.T) {
	gen := synth.New(3, 100)
	ix := New[int]()
	for id := range 3000 {
		ix.Set(id, gen.Set(5, 1))
	}
	ix.mu.Lock()
	ix.waitIdle()
	ix.mu.Unlock()
	if stats := ix.Stats(); stats.Segments > 12 {
		t.Fatalf("%d segments for 3000 single writes", stats.Segments)
	}
}

func TestInvalidEntriesKeepIndexes(t *testing.T) {
	ix := New[string]()
	problems := ix.Set("a", rules.Set{Includes: []string{"*er", "magazijn+medewerker~"}, Excludes: []string{"-hoofd*", "*manager"}})
	if len(problems) != 2 || problems[0].Index != 0 || !problems[1].Exclude {
		t.Fatalf("problems %+v", problems)
	}
	got := ix.Match("Magazijn medewerkers")
	if len(got) != 1 || got[0] != (Match[string]{ID: "a", Include: 1, Rule: "magazijn+medewerker~"}) {
		t.Fatalf("got %+v", got)
	}
	if len(ix.Match("magazijnmedewerker en manager")) != 0 {
		t.Fatal("the valid exclude must still apply")
	}
	if len(ix.Match("hoofd magazijnmedewerker")) != 1 {
		t.Fatal("the invalid exclude must not apply")
	}
}

func TestOrderAndReplace(t *testing.T) {
	ix := New[string]()
	ix.Set("first", rules.Set{Includes: []string{"chauffeur~"}})
	ix.Set("second", rules.Set{Includes: []string{"kok", "*chauffeur"}})
	ix.Set("third", rules.Set{Includes: []string{"heftruck+chauffeur"}})
	ids := func(text string) []string {
		var out []string
		for _, match := range ix.Match(text) {
			out = append(out, fmt.Sprintf("%s/%d", match.ID, match.Include))
		}
		return out
	}
	if got := ids("Heftruckchauffeur"); !slices.Equal(got, []string{"second/1", "third/0"}) {
		t.Fatalf("got %v", got)
	}
	if got := ids("heftruck chauffeur"); !slices.Equal(got, []string{"first/0", "second/1", "third/0"}) {
		t.Fatalf("got %v", got)
	}
	ix.Set("first", rules.Set{Includes: []string{"kok", "heftruck*"}})
	if got := ids("heftruck chauffeur"); !slices.Equal(got, []string{"second/1", "third/0", "first/1"}) {
		t.Fatalf("a replaced set moves to the end: %v", got)
	}
	if !ix.Delete("second") || ix.Delete("second") {
		t.Fatal("delete")
	}
	if got := ids("heftruck chauffeur"); !slices.Equal(got, []string{"third/0", "first/1"}) {
		t.Fatalf("got %v", got)
	}
	set, ok := ix.Get("first")
	if !ok || !slices.Equal(set.Includes, []string{"kok", "heftruck*"}) {
		t.Fatalf("get %v %v", set, ok)
	}
}

func TestSetAllLastWins(t *testing.T) {
	ix := New[int]()
	sets := func(yield func(int, rules.Set) bool) {
		_ = yield(1, rules.Set{Includes: []string{"kok"}}) &&
			yield(2, rules.Set{Includes: []string{"*er"}}) &&
			yield(1, rules.Set{Includes: []string{"chauffeur"}})
	}
	problems := ix.SetAll(sets)
	if len(problems) != 1 || len(problems[2]) != 1 {
		t.Fatalf("problems %v", problems)
	}
	if len(ix.Match("kok")) != 0 || len(ix.Match("chauffeur")) != 1 || ix.Len() != 2 {
		t.Fatal("the last set of an ID wins")
	}
}

func TestConcurrentUse(t *testing.T) {
	gen := synth.New(4, 100)
	texts := sample(gen, 200)
	ix := New[int]()
	ix.opts.syncMergeWeight = 100
	sets := make([]rules.Set, 400)
	for idx := range sets {
		sets[idx] = gen.Set(4, 1)
	}
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Go(func() {
			for idx := worker; idx < len(sets); idx += 4 {
				ix.Set(idx%50, sets[idx])
				if idx%9 == 0 {
					ix.Delete(idx % 50)
				}
			}
		})
		wg.Go(func() {
			for round := range 300 {
				ix.Match(texts[(round*7+worker)%len(texts)])
			}
		})
	}
	wg.Wait()
	ix.Compact()
	ref := reference{}
	for id := range 50 {
		if set, ok := ix.Get(id); ok {
			ref[id] = rules.MustCompile(set)
		}
	}
	check(t, ix, ref, texts)
}
