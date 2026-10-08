package textindex

import (
	"math/bits"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

// A pattern with this many postings that the set uses more than once is kept as a bitmap for the
// rest of the search: OR-ing a bitmap is cheaper than setting its bits again.
const heavyPostings = 512

type cachedPattern struct {
	uses     int
	resolved bool
	words    []uint32
	// postings is the number of texts the words have together; 0 means the pattern matches nothing.
	postings int
	heavy    bitmap
}

// searcher is the scratch state of one search over a corpus of size text ids. Its pooled bitmaps
// are always empty, so taking one costs nothing: whoever is done with a bitmap empties it, mostly in
// the same pass that reads it last.
type searcher struct {
	size     int
	pool     []bitmap
	patterns map[rules.Pattern]int
	cache    []cachedPattern
	// words holds the word lists of the cached patterns.
	words []uint32
	// marks is an empty bitmap over word ids for corpus.wordsFor.
	marks bitmap
	// ruleOf[text] is the include that matched the text; only valid for matched texts.
	ruleOf []uint32
	// slots marks the slots with a matched text while the hits are collected.
	slots bitmap
}

func newSearcher(size int) *searcher {
	return &searcher{size: size, patterns: map[rules.Pattern]int{}, ruleOf: make([]uint32, size)}
}

func (s *searcher) get() bitmap {
	if len(s.pool) == 0 {
		return newBitmap(s.size)
	}
	b := s.pool[len(s.pool)-1]
	s.pool = s.pool[:len(s.pool)-1]
	return b
}

// put takes back an empty bitmap.
func (s *searcher) put(b bitmap) { s.pool = append(s.pool, b) }

func (s *searcher) orWords(c *corpus, words []uint32, dst bitmap) {
	for _, word := range words {
		for _, textID := range c.postings[word] {
			dst[textID>>6] |= 1 << (textID & 63)
		}
	}
}

// count notes how often the set uses each pattern, before anything is looked up.
func (s *searcher) count(node *rules.Node) {
	for _, pattern := range node.Patterns {
		idx, ok := s.patterns[pattern]
		if !ok {
			idx = len(s.cache)
			s.patterns[pattern] = idx
			s.cache = append(s.cache, cachedPattern{})
		}
		s.cache[idx].uses++
	}
	for _, child := range node.Children {
		s.count(child)
	}
}

// patternInto ORs the texts the pattern matches into dst and reports whether there were any.
func (s *searcher) patternInto(c *corpus, pattern rules.Pattern, dst bitmap) bool {
	cached := &s.cache[s.patterns[pattern]]
	if !cached.resolved {
		cached.resolved = true
		start := len(s.words)
		s.words = c.wordsFor(pattern, s.words, s.marks)
		cached.words = s.words[start:len(s.words):len(s.words)]
		for _, word := range cached.words {
			cached.postings += len(c.postings[word])
		}
		if cached.postings >= heavyPostings && cached.uses > 1 {
			cached.heavy = s.get()
			s.orWords(c, cached.words, cached.heavy)
		}
	}
	switch {
	case cached.heavy != nil:
		dst.or(cached.heavy)
	case cached.postings > 0:
		s.orWords(c, cached.words, dst)
	}
	return cached.postings > 0
}

// nodeInto ORs the texts the node matches into dst and reports whether there were any.
func (s *searcher) nodeInto(c *corpus, node *rules.Node, dst bitmap) bool {
	found := false
	for _, pattern := range node.Patterns {
		found = s.patternInto(c, pattern, dst) || found
	}
	switch node.Type {
	case rules.GroupNode:
		for _, option := range node.Children {
			found = s.nodeInto(c, option, dst) || found
		}
	case rules.CompoundNode:
		acc := s.get()
		apart := s.nodeInto(c, node.Children[0], acc)
		if apart {
			part := s.get()
			for _, child := range node.Children[1:] {
				if !s.nodeInto(c, child, part) {
					clear(acc)
					apart = false
					break
				}
				if !acc.andClear(part) {
					apart = false
					break
				}
			}
			s.put(part)
		}
		if apart {
			dst.orClear(acc)
		}
		s.put(acc)
		found = found || apart
	}
	return found
}

// ruleInto sets the texts the rule matches in dst, which must be empty, and reports whether any did.
// When it reports none, dst is empty again.
func (s *searcher) ruleInto(c *corpus, rule *rules.Rule, dst bitmap) bool {
	first := true
	var part bitmap
	defer func() {
		if part != nil {
			s.put(part)
		}
	}()
	for _, term := range rule.Terms {
		if term.Negative {
			continue
		}
		if first {
			if !s.nodeInto(c, term.Node, dst) {
				return false
			}
			first = false
			continue
		}
		if part == nil {
			part = s.get()
		}
		if !s.nodeInto(c, term.Node, part) {
			clear(dst)
			return false
		}
		if !dst.andClear(part) {
			return false
		}
	}
	for _, term := range rule.Terms {
		if !term.Negative {
			continue
		}
		if part == nil {
			part = s.get()
		}
		if s.nodeInto(c, term.Node, part) && !dst.andNotClear(part) {
			return false
		}
	}
	return true
}

// claim gives the texts in matched that no exclude and no earlier include took to include, and
// empties matched.
func (s *searcher) claim(matched, excluded, claimed bitmap, include uint32) {
	for idx, word := range matched {
		if word == 0 {
			continue
		}
		matched[idx] = 0
		word &^= excluded[idx] | claimed[idx]
		claimed[idx] |= word
		for word != 0 {
			s.ruleOf[idx*64+bits.TrailingZeros64(word)] = include
			word &= word - 1
		}
	}
}

// match returns the texts the set matches, with ruleOf filled in for them. The caller empties the
// bitmap and puts it back.
func (s *searcher) match(c *corpus, set *rules.Compiled) bitmap {
	if len(s.marks) < (len(c.words)+63)/64 {
		s.marks = newBitmap(len(c.words))
	}
	for _, rule := range set.Includes {
		if rule != nil {
			for _, term := range rule.Terms {
				s.count(term.Node)
			}
		}
	}
	for _, term := range set.Excludes {
		if term != nil {
			s.count(term.Node)
		}
	}
	excluded := s.get()
	for _, term := range set.Excludes {
		if term != nil {
			s.nodeInto(c, term.Node, excluded)
		}
	}
	claimed := s.get()
	matched := s.get()
	for include, rule := range set.Includes {
		if rule != nil && s.ruleInto(c, rule, matched) {
			s.claim(matched, excluded, claimed, uint32(include))
		}
	}
	s.put(matched)
	clear(excluded)
	s.put(excluded)
	for idx := range s.cache {
		if heavy := s.cache[idx].heavy; heavy != nil {
			clear(heavy)
			s.put(heavy)
		}
	}
	clear(s.patterns)
	s.cache = s.cache[:0]
	s.words = s.words[:0]
	return claimed
}
