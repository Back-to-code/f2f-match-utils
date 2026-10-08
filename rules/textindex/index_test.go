package textindex

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
)

// reference stores the texts per ID and evaluates the set on each, the semantics the index must
// reproduce. IDs are kept in insertion order so it can list hits in a stable order.
type reference struct {
	ids   []int
	texts map[int][]string
}

func newReference() *reference { return &reference{texts: map[int][]string{}} }

func (r *reference) set(id int, texts []string) {
	if _, ok := r.texts[id]; !ok {
		r.ids = append(r.ids, id)
	}
	r.texts[id] = texts
	if len(texts) == 0 {
		r.delete(id)
	}
}

func (r *reference) delete(id int) {
	delete(r.texts, id)
	r.ids = slices.DeleteFunc(r.ids, func(other int) bool { return other == id })
}

type found struct {
	id, position, include int
	text                  string
}

func (r *reference) match(set *rules.Compiled) map[found]bool {
	out := map[found]bool{}
	for _, id := range r.ids {
		for position, text := range r.texts[id] {
			if include := set.Match(text); include >= 0 {
				out[found{id, position, include, text}] = true
			}
		}
	}
	return out
}

func check(t *testing.T, ix *Index[int], ref *reference, sets []*rules.Compiled) int {
	t.Helper()
	total := 0
	for _, set := range sets {
		want := ref.match(set)
		hits := ix.Match(set)
		got := map[found]bool{}
		for idx, hit := range hits {
			key := found{hit.ID, hit.Position, hit.Include, hit.Text}
			if got[key] {
				t.Fatalf("%v: hit %+v twice", set.Set, hit)
			}
			got[key] = true
			if hit.Rule != set.Set.Includes[hit.Include] {
				t.Fatalf("hit %+v names the wrong rule", hit)
			}
			if idx > 0 && hits[idx-1].ID == hit.ID && hits[idx-1].Position >= hit.Position {
				t.Fatalf("hits of one ID are out of position order: %+v, %+v", hits[idx-1], hit)
			}
		}
		seen := map[int]bool{}
		for idx, hit := range hits {
			if seen[hit.ID] && hits[idx-1].ID != hit.ID {
				t.Fatalf("hits of ID %d are not grouped", hit.ID)
			}
			seen[hit.ID] = true
		}
		if len(got) != len(want) {
			t.Fatalf("%v: %d hits, want %d", set.Set, len(got), len(want))
		}
		for key := range want {
			if !got[key] {
				t.Fatalf("%v: missing %+v", set.Set, key)
			}
		}
		total += len(hits)
	}
	return total
}

func sets(gen *synth.Generator, count int) []*rules.Compiled {
	out := make([]*rules.Compiled, count)
	for idx := range out {
		out[idx] = rules.MustCompile(gen.Set(1+idx%25, idx%4))
	}
	return out
}

func texts(gen *synth.Generator, count int) []string {
	out := make([]string, count)
	for idx := range out {
		out[idx] = gen.Text()
	}
	return out
}

func TestMatchesLikeTheSets(t *testing.T) {
	gen := synth.New(1, 300)
	ix := New[int]()
	ref := newReference()
	for id := range 2500 {
		stored := texts(gen, 1+id%4)
		ix.Set(id, stored...)
		ref.set(id, stored)
	}
	if total := check(t, ix, ref, sets(gen, 120)); total < 1000 {
		t.Fatalf("only %d hits: the test tests little", total)
	}
}

func TestWrites(t *testing.T) {
	gen := synth.New(2, 150)
	rng := rand.New(rand.NewPCG(2, 2))
	ix := New[int]()
	ref := newReference()
	probes := sets(gen, 40)
	for step := range 6000 {
		id := rng.IntN(400)
		switch roll := rng.IntN(10); {
		case roll == 0:
			ix.Delete(id)
			ref.delete(id)
		case roll == 1:
			extra := texts(gen, 1+rng.IntN(2))
			ix.Append(id, extra...)
			ref.set(id, append(slices.Clone(ref.texts[id]), extra...))
		case roll == 2:
			// Rewriting an ID with the texts it has must leave everything as it was.
			kept := ix.Get(id)
			ix.Set(id, kept...)
			ref.set(id, kept)
		default:
			stored := texts(gen, rng.IntN(4))
			ix.Set(id, stored...)
			ref.set(id, stored)
		}
		if step%1000 == 999 {
			check(t, ix, ref, probes)
			if err := verify(&ix.corpus); err != nil {
				t.Fatal(err)
			}
		}
	}
	if ix.Len() != len(ref.texts) {
		t.Fatalf("len %d, want %d", ix.Len(), len(ref.texts))
	}
	for id, stored := range ref.texts {
		if !slices.Equal(ix.Get(id), stored) {
			t.Fatalf("get %d: %v, want %v", id, ix.Get(id), stored)
		}
	}
	if err := verify(&ix.corpus); err != nil {
		t.Fatal(err)
	}
	for id := range 400 {
		ix.Delete(id)
	}
	if stats := ix.Stats(); stats != (Stats{}) {
		t.Fatalf("an emptied index holds %+v", stats)
	}
	if err := verify(&ix.corpus); err != nil {
		t.Fatal(err)
	}
}

// verify checks that postings, buckets and the counts that keep them cheap to change agree.
func verify(c *corpus) error {
	for key, textID := range c.textOf {
		stored := c.texts[textID]
		if stored.key != key {
			return fmt.Errorf("text %d %q has key %q", textID, key, stored.key)
		}
		sequence := strings.Split(key, " ")
		for idx, word := range sequence {
			id := c.wordOf[word]
			if c.lefts[id] != 0 || c.rights[id] != 0 {
				return fmt.Errorf("%q is a word of %q but counts as a pair", word, key)
			}
			if idx > 0 {
				pair := c.wordOf[sequence[idx-1]+word]
				if int(c.lefts[pair]) > len(sequence[idx-1]) || int(c.rights[pair]) > len(word) {
					return fmt.Errorf("pair %q of %q has splits %d, %d", sequence[idx-1]+word, key, c.lefts[pair], c.rights[pair])
				}
			}
		}
		for _, wordID := range stored.words {
			if count := countOf(c.postings[wordID], textID); count != 1 {
				return fmt.Errorf("text %q is %d times in the postings of %q", key, count, c.words[wordID])
			}
		}
	}
	listed := make([]uint32, len(c.words))
	for gram, listing := range c.infixes {
		retired := uint32(0)
		for _, id := range listing.words {
			listed[id]++
			if c.words[id] == "" {
				retired++
			}
		}
		if retired != listing.retired || retired*2 > uint32(len(listing.words)) || len(listing.words) == 0 {
			return fmt.Errorf("infix bucket %d: %d of %d retired, counted %d", gram, retired, len(listing.words), listing.retired)
		}
	}
	long := 0
	for _, word := range c.words {
		if len(word) >= 4 {
			long++
		}
	}
	for _, order := range []*orderedWords{&c.forward, &c.backward} {
		var previous string
		count := 0
		for idx, ids := range order.chunks {
			if len(ids) == 0 || len(ids) > chunkSize || cap(ids) != chunkSize || (len(order.chunks) > 1 && len(ids) < chunkSize/8) {
				return fmt.Errorf("chunk %d of %d holds %d ids, room for %d", idx, len(order.chunks), len(ids), cap(ids))
			}
			if len(order.fences) != len(order.chunks) || order.fences[idx] != order.key8(c.words[ids[len(ids)-1]]) {
				return fmt.Errorf("chunk %d has a stale fence", idx)
			}
			for _, id := range ids {
				word := c.words[id]
				if len(word) < 4 || c.wordOf[word] != id || (count > 0 && order.compare(previous, word) >= 0) {
					return fmt.Errorf("ordered words: %q after %q", word, previous)
				}
				previous = word
				count++
			}
		}
		if count != long {
			return fmt.Errorf("ordered words hold %d words, want %d", count, long)
		}
	}
	for id, word := range c.words {
		if listed[id] != c.listed[id] {
			return fmt.Errorf("word %d %q is in %d buckets, counted %d", id, word, listed[id], c.listed[id])
		}
		if word != "" && (c.wordOf[word] != uint32(id) || len(c.postings[id]) == 0) {
			return fmt.Errorf("word %d %q: maps to %d, %d postings", id, word, c.wordOf[word], len(c.postings[id]))
		}
	}
	for _, id := range c.freeWords {
		if c.words[id] != "" || c.listed[id] != 0 {
			return fmt.Errorf("free word %d is %q, in %d buckets", id, c.words[id], c.listed[id])
		}
	}
	return nil
}

func countOf(values []uint32, value uint32) int {
	count := 0
	for _, other := range values {
		if other == value {
			count++
		}
	}
	return count
}

func TestHits(t *testing.T) {
	ix := New[string]()
	ix.Set("a", "Kok", "Magazijn-medewerker", "", "Hoofd magazijn")
	ix.Set("b", "magazijnmedewerker")
	ix.Set("c", "Medewerker magazijn", "MAGAZIJNMEDEWERKER")
	set := rules.MustCompile(rules.Set{Includes: []string{"kok", "magazijn+medewerker~"}, Excludes: []string{"hoofd*"}})
	got := ix.Match(set)
	want := []Hit[string]{
		{ID: "a", Position: 0, Text: "Kok", Include: 0, Rule: "kok"},
		{ID: "a", Position: 1, Text: "Magazijn-medewerker", Include: 1, Rule: "magazijn+medewerker~"},
		{ID: "b", Position: 0, Text: "magazijnmedewerker", Include: 1, Rule: "magazijn+medewerker~"},
		{ID: "c", Position: 0, Text: "Medewerker magazijn", Include: 1, Rule: "magazijn+medewerker~"},
		{ID: "c", Position: 1, Text: "MAGAZIJNMEDEWERKER", Include: 1, Rule: "magazijn+medewerker~"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v", got)
	}
	if stats := ix.Stats(); stats.Texts != 7 || stats.DistinctTexts != 5 {
		t.Fatalf("stats %+v", stats)
	}
	if estimate := ix.Estimate(rules.Pattern{Kind: rules.Prefix, Text: "magazijn"}); estimate != 5 {
		t.Fatalf("estimate %v", estimate)
	}
}

func TestInvalidEntriesKeepIndexes(t *testing.T) {
	ix := New[int]()
	ix.Set(1, "chauffeur", "hoofd chauffeur")
	set, problems := rules.Compile(rules.Set{Includes: []string{"*er", "chauffeur~"}, Excludes: []string{"-hoofd*"}})
	if len(problems) != 2 {
		t.Fatalf("problems %v", problems)
	}
	got := ix.Match(set)
	if len(got) != 2 || got[0].Include != 1 || got[1].Include != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	gen := synth.New(3, 100)
	ix := New[int]()
	pool := texts(gen, 500)
	probes := sets(gen, 10)
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Go(func() {
			for step := range 400 {
				ix.Set((step*4+worker)%200, pool[(step*7+worker)%len(pool)], pool[(step*3)%len(pool)])
			}
		})
		wg.Go(func() {
			for step := range 40 {
				ix.Match(probes[(step+worker)%len(probes)])
			}
		})
	}
	wg.Wait()
	ref := newReference()
	for id := range 200 {
		if stored := ix.Get(id); stored != nil {
			ref.set(id, stored)
		}
	}
	check(t, ix, ref, probes)
}
