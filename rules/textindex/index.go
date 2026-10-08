// Package textindex matches rule sets against many texts at once: the texts are indexed up front, the
// rule sets are not. Use it when there are many texts (up to millions) and a set of a few dozen rules
// at a time; for the opposite, see the ruleindex package.
package textindex

import (
	"math/bits"
	"sync"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

type Hit[ID comparable] struct {
	ID ID
	// Position is the index of the text among the texts stored under ID.
	Position int
	// Text is the text as it was stored.
	Text string
	// Include is the index in the set's Includes of the first include that matches the text.
	Include int
	// Rule is that include as it was written.
	Rule string
}

type entry struct {
	text uint32
	raw  string
}

type slot[ID comparable] struct {
	id      ID
	entries []entry
}

// Index holds texts by ID and matches rule sets against all of them. It is safe for concurrent use;
// writes wait for running matches and the other way around.
type Index[ID comparable] struct {
	mu sync.RWMutex

	slots     []slot[ID]
	slotOf    map[ID]uint32
	freeSlots []uint32
	entries   int

	corpus    corpus
	searchers sync.Pool
}

func New[ID comparable]() *Index[ID] {
	return &Index[ID]{slotOf: map[ID]uint32{}, corpus: newCorpus()}
}

// Set stores texts under id, replacing what was stored there before. Without texts it is Delete.
func (ix *Index[ID]) Set(id ID, texts ...string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if len(texts) == 0 {
		ix.delete(id)
		return
	}
	slotID, ok := ix.slotOf[id]
	if !ok {
		slotID = ix.newSlot(id)
	}
	old := ix.slots[slotID].entries
	entries := make([]entry, len(texts))
	// The new texts are attached before the old ones are detached, so a text the id keeps is not
	// unindexed and indexed again.
	for position, raw := range texts {
		entries[position] = entry{ix.corpus.attach(raw, owner{slotID, uint32(position)}), raw}
	}
	for position, previous := range old {
		ix.corpus.detach(previous.text, owner{slotID, uint32(position)})
	}
	ix.slots[slotID].entries = entries
	ix.entries += len(entries) - len(old)
}

// Append adds texts after the texts already stored under id.
func (ix *Index[ID]) Append(id ID, texts ...string) {
	if len(texts) == 0 {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	slotID, ok := ix.slotOf[id]
	if !ok {
		slotID = ix.newSlot(id)
	}
	stored := &ix.slots[slotID]
	for _, raw := range texts {
		stored.entries = append(stored.entries, entry{ix.corpus.attach(raw, owner{slotID, uint32(len(stored.entries))}), raw})
	}
	ix.entries += len(texts)
}

// Delete removes the texts stored under id and reports whether there were any.
func (ix *Index[ID]) Delete(id ID) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.delete(id)
}

func (ix *Index[ID]) delete(id ID) bool {
	slotID, ok := ix.slotOf[id]
	if !ok {
		return false
	}
	for position, stored := range ix.slots[slotID].entries {
		ix.corpus.detach(stored.text, owner{slotID, uint32(position)})
	}
	ix.entries -= len(ix.slots[slotID].entries)
	ix.slots[slotID] = slot[ID]{}
	delete(ix.slotOf, id)
	ix.freeSlots = append(ix.freeSlots, slotID)
	return true
}

func (ix *Index[ID]) newSlot(id ID) uint32 {
	var slotID uint32
	if count := len(ix.freeSlots); count > 0 {
		slotID = ix.freeSlots[count-1]
		ix.freeSlots = ix.freeSlots[:count-1]
		ix.slots[slotID] = slot[ID]{id: id}
	} else {
		slotID = uint32(len(ix.slots))
		ix.slots = append(ix.slots, slot[ID]{id: id})
	}
	ix.slotOf[id] = slotID
	return slotID
}

func (ix *Index[ID]) Get(id ID) []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	slotID, ok := ix.slotOf[id]
	if !ok {
		return nil
	}
	texts := make([]string, len(ix.slots[slotID].entries))
	for idx, stored := range ix.slots[slotID].entries {
		texts[idx] = stored.raw
	}
	return texts
}

// Len returns the number of IDs.
func (ix *Index[ID]) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.slotOf)
}

type Stats struct {
	IDs   int
	Texts int
	// DistinctTexts counts texts with different words; texts that only differ in case, accents or
	// punctuation are stored and matched once.
	DistinctTexts int
	// Words counts the distinct words, joined pairs included.
	Words int
}

func (ix *Index[ID]) Stats() Stats {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return Stats{IDs: len(ix.slotOf), Texts: ix.entries, DistinctTexts: len(ix.corpus.textOf), Words: len(ix.corpus.wordOf)}
}

// Estimate returns how many stored texts the pattern matches, about. It fits ruleindex.Estimator, so
// a rule index can pick its anchors by the texts it will see.
func (ix *Index[ID]) Estimate(pattern rules.Pattern) float64 {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.corpus.estimate(pattern)
}

// Match returns every stored text the set matches, grouped by ID and in the order of their positions.
func (ix *Index[ID]) Match(set *rules.Compiled) []Hit[ID] {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	s, _ := ix.searchers.Get().(*searcher)
	if s == nil || s.size != len(ix.corpus.texts) {
		s = newSearcher(len(ix.corpus.texts))
	}
	defer ix.searchers.Put(s)
	if len(s.slots) < (len(ix.slots)+63)/64 {
		s.slots = newBitmap(len(ix.slots))
	}
	matched := s.match(&ix.corpus, set)

	count := 0
	matched.forEach(func(textID uint32) {
		stored := &ix.corpus.texts[textID]
		s.slots.set(stored.first.slot)
		for _, by := range stored.more {
			s.slots.set(by.slot)
		}
		count += 1 + len(stored.more)
	})
	// Going through the slots in order, rather than through the texts, groups the hits by ID.
	hits := make([]Hit[ID], 0, count)
	for idx, word := range s.slots {
		if word == 0 {
			continue
		}
		s.slots[idx] = 0
		for ; word != 0; word &= word - 1 {
			stored := &ix.slots[idx*64+bits.TrailingZeros64(word)]
			for position, entry := range stored.entries {
				if entry.text == noText || !matched.has(entry.text) {
					continue
				}
				include := s.ruleOf[entry.text]
				hits = append(hits, Hit[ID]{
					ID:       stored.id,
					Position: position,
					Text:     entry.raw,
					Include:  int(include),
					Rule:     set.Set.Includes[include],
				})
			}
		}
	}
	clear(matched)
	s.put(matched)
	return hits
}
