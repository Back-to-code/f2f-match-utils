package textindex

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
	"github.com/Back-to-code/f2f-match-utils/sanitize"
)

// reference stores the texts per ID and evaluates the set on each, the semantics the index must
// reproduce. IDs are kept in insertion order so it can list hits in a stable order.
type reference struct {
	ids   []int
	texts map[int][]string
	tags  map[int][]uint32
}

func newReference() *reference {
	return &reference{texts: map[int][]string{}, tags: map[int][]uint32{}}
}

func (r *reference) set(id int, texts []string) {
	if _, ok := r.texts[id]; !ok {
		r.ids = append(r.ids, id)
	}
	r.texts[id] = texts
	r.tags[id] = make([]uint32, len(texts))
	if len(texts) == 0 {
		r.delete(id)
	}
}

func (r *reference) append(id int, tag uint32, texts ...string) {
	tags := r.tags[id]
	r.set(id, append(slices.Clone(r.texts[id]), texts...))
	for range texts {
		tags = append(tags, tag)
	}
	r.tags[id] = tags
}

func (r *reference) delete(id int) {
	delete(r.texts, id)
	delete(r.tags, id)
	r.ids = slices.DeleteFunc(r.ids, func(other int) bool { return other == id })
}

type found struct {
	id, position, include int
	tag                   uint32
	text                  string
}

func (r *reference) match(set *rules.Compiled) map[found]bool {
	out := map[found]bool{}
	for _, id := range r.ids {
		for position, text := range r.texts[id] {
			if include := set.Match(text); include >= 0 {
				out[found{id, position, include, r.tags[id][position], text}] = true
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
			key := found{hit.ID, hit.Position, hit.Include, hit.Tag, hit.Text}
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

func TestWritesWithCollidingKeys(t *testing.T) {
	defer func(original func(string) uint64) { keyHash = original }(keyHash)
	keyHash = func(key string) uint64 { return uint64(len(key) % 5) }
	TestWrites(t)
}

func TestWrites(t *testing.T) {
	gen := synth.New(2, 150)
	rng := rand.New(rand.NewPCG(2, 2))
	ix := New[int]()
	ref := newReference()
	probes := sets(gen, 40)
	for step := range 6000 {
		id := rng.IntN(400)
		switch rng.IntN(10) {
		case 0:
			ix.Delete(id)
			ref.delete(id)
		case 1:
			extra := texts(gen, 1+rng.IntN(2))
			ix.Append(id, extra...)
			ref.append(id, 0, extra...)
		case 2:
			// Rewriting an ID with the texts it has must leave everything as it was.
			kept := ix.Get(id)
			ix.Set(id, kept...)
			ref.set(id, kept)
		case 3:
			text, tag := texts(gen, 1)[0], rng.Uint32()
			ix.AppendTagged(id, tag, text)
			ref.append(id, tag, text)
		default:
			stored := texts(gen, rng.IntN(4))
			ix.Set(id, stored...)
			ref.set(id, stored)
		}
		if step%700 == 0 {
			ix.Trim()
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

// TestShortWordsAndJoins writes texts of short words over three letters: most pairs are never a word
// on their own, many infix patterns only match across a join, and short words come and go.
func TestShortWordsAndJoins(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 4))
	word := func(size int) string {
		letters := make([]byte, size)
		for idx := range letters {
			letters[idx] = "abc"[rng.IntN(3)]
		}
		return string(letters)
	}
	text := func() string {
		words := make([]string, 1+rng.IntN(4))
		for idx := range words {
			words[idx] = word(1 + rng.IntN(5))
		}
		return strings.Join(words, " ")
	}
	var infixes []string
	pattern := func() string {
		switch rng.IntN(4) {
		case 0:
			return word(1 + rng.IntN(3))
		case 1:
			return word(4) + "*"
		case 2:
			return "*" + word(4)
		}
		infix := word(4 + rng.IntN(6))
		infixes = append(infixes, infix)
		return "*" + infix + "*"
	}
	probes := make([]*rules.Compiled, 60)
	for idx := range probes {
		set := rules.Set{Includes: []string{pattern(), pattern()}}
		if idx%3 == 0 {
			set.Excludes = []string{pattern()}
		}
		probes[idx] = rules.MustCompile(set)
	}
	ix := New[int]()
	ref := newReference()
	total, acrossJoins := 0, 0
	for step := range 3000 {
		id := rng.IntN(200)
		switch rng.IntN(4) {
		case 0:
			ix.Delete(id)
			ref.delete(id)
		case 1:
			extra := text()
			ix.Append(id, extra)
			ref.append(id, 0, extra)
		default:
			stored := []string{text(), text()}[:rng.IntN(3)]
			ix.Set(id, stored...)
			ref.set(id, stored)
		}
		if step%300 == 299 {
			total += check(t, ix, ref, probes)
			if err := verify(&ix.corpus); err != nil {
				t.Fatal(err)
			}
			for _, infix := range infixes {
				acrossJoins += len(ix.corpus.appendJunctionWords(nil, rules.Pattern{Kind: rules.Infix, Text: infix}))
			}
		}
	}
	if total < 1000 || acrossJoins < 100 {
		t.Fatalf("%d hits, %d pairs matched across their join: the test tests little", total, acrossJoins)
	}
	for id := range 200 {
		ix.Delete(id)
	}
	if err := verify(&ix.corpus); err != nil {
		t.Fatal(err)
	}
	if stats := ix.Stats(); stats != (Stats{}) || len(ix.corpus.freeWords) != len(ix.corpus.words) {
		t.Fatalf("an emptied index holds %+v, %d of %d word ids free", stats, len(ix.corpus.freeWords), len(ix.corpus.words))
	}
}

func TestPairsListedAtTheirJoin(t *testing.T) {
	ix := New[int]()
	c := &ix.corpus
	ref := newReference()
	probes := []*rules.Compiled{
		rules.MustCompile(rules.Set{Includes: []string{"*bcdef*"}}),
		rules.MustCompile(rules.Set{Includes: []string{"*defg*"}}),
		rules.MustCompile(rules.Set{Includes: []string{"*qrst*"}}),
		rules.MustCompile(rules.Set{Includes: []string{"*bcdefghi*"}}),
	}
	set := func(id int, texts ...string) {
		t.Helper()
		ix.Set(id, texts...)
		ref.set(id, texts)
		check(t, ix, ref, probes)
		if err := verify(c); err != nil {
			t.Fatal(err)
		}
	}
	gramsAt := func(word string) uint8 {
		t.Helper()
		id, ok := c.lookupWord(word)
		if !ok {
			t.Fatalf("%q is not a word", word)
		}
		return c.gramsAt[id]
	}

	set(1, "ab cdefgh")
	set(2, "xb cdefgh")
	if gramsAt("abcdefgh") != 2 || gramsAt("xbcdefgh") != 2 || len(c.junctions) != 3 {
		t.Fatalf("pairs at %d and %d in %d junction buckets", gramsAt("abcdefgh"), gramsAt("xbcdefgh"), len(c.junctions))
	}
	set(1)
	set(2)
	if len(c.junctions) != 0 || len(c.freeWords) != len(c.words) {
		t.Fatalf("%d junction buckets and %d of %d word ids free after the pairs went", len(c.junctions), len(c.freeWords), len(c.words))
	}

	set(3, "ab cdefgh")
	set(4, "abcdefgh")
	if gramsAt("abcdefgh") != allGrams || len(c.junctions) != 0 {
		t.Fatalf("a pair that is also a word on its own is at %d, %d junction buckets", gramsAt("abcdefgh"), len(c.junctions))
	}

	set(5, "pq rstu")
	set(6, "pqr stu")
	if gramsAt("pqrstu") != allGrams || len(c.junctions) != 0 {
		t.Fatalf("a pair joined at two places is at %d, %d junction buckets", gramsAt("pqrstu"), len(c.junctions))
	}

	set(7, "abcde fghij")
	if got := ix.Match(probes[3]); len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("a pattern that crosses the join four letters in found %+v", got)
	}
}

// verify checks that postings, buckets and the counts that keep them cheap to change agree.
func verify(c *corpus) error {
	keyed := map[string]uint32{}
	for _, textID := range c.textOf.held {
		keyed[sanitize.WordKey(c.texts[textID].raw)] = textID
	}
	for key, textID := range c.textOf.collided {
		keyed[key] = textID
	}
	for key, textID := range keyed {
		stored := c.texts[textID]
		if found, ok := c.lookup(key); !ok || found != textID {
			return fmt.Errorf("text %d %q is not found by its key", textID, key)
		}
		sequence := strings.Split(key, " ")
		for idx, word := range sequence {
			id, _ := c.lookupWord(word)
			if c.lefts[id] != 0 || c.rights[id] != 0 {
				return fmt.Errorf("%q is a word of %q but counts as a pair", word, key)
			}
			if idx > 0 {
				pair, _ := c.lookupWord(sequence[idx-1] + word)
				if int(c.lefts[pair]) > len(sequence[idx-1]) || int(c.rights[pair]) > len(word) {
					return fmt.Errorf("pair %q of %q has splits %d, %d", sequence[idx-1]+word, key, c.lefts[pair], c.rights[pair])
				}
				if at := c.gramsAt[pair]; at != allGrams && int(at) != len(sequence[idx-1]) {
					return fmt.Errorf("pair %q of %q is listed at join %d", sequence[idx-1]+word, key, at)
				}
			}
		}
		for _, wordID := range stored.words {
			if count := countOf(c.postings[wordID], textID); count != 1 {
				return fmt.Errorf("text %q is %d times in the postings of %q", key, count, c.words[wordID])
			}
		}
	}
	if live := len(c.texts) - len(c.freeTexts); live != len(keyed) {
		return fmt.Errorf("%d texts in use, %d found by key", live, len(keyed))
	}
	type membership struct {
		kind     string
		gram, id uint32
	}
	members := map[membership]bool{}
	listed := make([]uint32, len(c.words))
	for kind, buckets := range map[string]map[uint32]bucket{"infix": c.infixes, "junction": c.junctions} {
		for gram, listing := range buckets {
			retired := uint32(0)
			for _, id := range listing.words {
				listed[id]++
				members[membership{kind, gram, id}] = true
				if c.words[id] == "" {
					retired++
				}
			}
			if retired != listing.retired || retired*2 > uint32(len(listing.words)) || len(listing.words) == 0 {
				return fmt.Errorf("%s bucket %d: %d of %d retired, counted %d", kind, gram, retired, len(listing.words), listing.retired)
			}
		}
	}
	live := 0
	for id, word := range c.words {
		if word == "" {
			continue
		}
		live++
		at := c.gramsAt[id]
		if at == noGrams || (c.lefts[id] == 0 && at != allGrams) {
			return fmt.Errorf("word %q, a pair %v, is listed at %d", word, c.lefts[id] != 0, at)
		}
		kind, grams := "infix", 0
		var missing []uint32
		note := func(gram uint32) {
			grams++
			if !members[membership{kind, gram, uint32(id)}] {
				missing = append(missing, gram)
			}
		}
		if at == allGrams {
			c.forEachGram(word, note)
		} else {
			kind = "junction"
			forEachJunctionGram(word, int(at), note)
		}
		if len(missing) > 0 || int(listed[id]) != grams {
			return fmt.Errorf("word %q is in %d buckets, has %d %s grams, misses %v", word, listed[id], grams, kind, missing)
		}
	}
	if live != c.wordOf.len() {
		return fmt.Errorf("%d live words, %d found by hash", live, c.wordOf.len())
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
				found, _ := c.lookupWord(word)
				if len(word) < 4 || found != id || (count > 0 && order.compare(previous, word) >= 0) {
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
		found, _ := c.lookupWord(word)
		if word != "" && (found != uint32(id) || len(c.postings[id]) == 0) {
			return fmt.Errorf("word %d %q: maps to %d, %d postings", id, word, found, len(c.postings[id]))
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

func TestEqualTextsShareTheirString(t *testing.T) {
	ix := New[int]()
	ix.Set(1, strings.Clone("Kok"))
	ix.Append(2, strings.Clone("Kok"))
	ix.AppendTagged(3, 7, strings.Clone("Kok"))
	ix.Set(4, strings.Clone("KOK"))
	first := unsafe.StringData(ix.Get(1)[0])
	for _, id := range []int{2, 3} {
		if unsafe.StringData(ix.Get(id)[0]) != first {
			t.Fatalf("text of %d is a copy", id)
		}
	}
	if unsafe.StringData(ix.Get(4)[0]) == first || ix.Get(4)[0] != "KOK" {
		t.Fatalf("a text that differs in case got %q", ix.Get(4)[0])
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
