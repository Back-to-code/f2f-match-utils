package ruleindex

import (
	"slices"
	"sync"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

// Estimator ranks patterns by how many texts they are expected to match; lower is more selective.
// A rule is only evaluated for texts that hit a pattern of its most selective positive term, so a good
// estimate makes matching cheaper but never changes its result.
type Estimator func(pattern rules.Pattern) float64

// HeuristicEstimate is the Estimator used without text statistics.
func HeuristicEstimate(pattern rules.Pattern) float64 {
	switch pattern.Kind {
	case rules.Exact:
		return 1
	case rules.Prefix, rules.Suffix:
		return 400 / float64(len(pattern.Text))
	}
	return 4000 / float64(len(pattern.Text))
}

var gramCode = func() [256]uint32 {
	var codes [256]uint32
	for ch := 'a'; ch <= 'z'; ch++ {
		codes[ch] = uint32(ch - 'a')
	}
	for ch := '0'; ch <= '9'; ch++ {
		codes[ch] = uint32(26 + ch - '0')
	}
	return codes
}()

func gram4(text string) uint32 {
	return ((gramCode[text[0]]*36+gramCode[text[1]])*36+gramCode[text[2]])*36 + gramCode[text[3]]
}

// gramFilter has a bit for every hashed 4-letter start (prefix, infix) or end (suffix) of one kind of
// pattern, so most prefix, suffix and substring probes are skipped with one bit test. It is sized to
// the patterns, so a segment with a single set does not carry a filter over every possible gram.
type gramFilter struct {
	bits  []uint64
	shift uint32
}

func newGramFilter(patterns int) gramFilter {
	logSize := uint32(6)
	for 1<<logSize < patterns*16 && logSize < 22 {
		logSize++
	}
	return gramFilter{bits: make([]uint64, 1<<(logSize-6)), shift: 32 - logSize}
}

func (f gramFilter) slot(text string) uint32 { return (gram4(text) * 0x9E3779B1) >> f.shift }

func (f gramFilter) add(text string) {
	slot := f.slot(text)
	f.bits[slot>>6] |= 1 << (slot & 63)
}

func (f gramFilter) has(text string) bool {
	slot := f.slot(text)
	return f.bits[slot>>6]&(1<<(slot&63)) != 0
}

// hit is one set a text matched, with the index in the set's Includes of its first matching include.
type hit struct {
	set     uint32
	include uint32
}

// core matches one text against all sets of a segment at once. Every pattern of every rule and exclude
// sits in one of four hash maps; a text's words are looked up (whole word, prefixes, suffixes,
// substrings) to get the set of hit patterns, and only rules anchored on a hit pattern are evaluated.
type core struct {
	table *patternTable
	prog  *program

	anchorOff   []uint32
	anchorRules []uint32
	// ownerOff/owners list, per rule, every (set, include) that uses it.
	ownerOff []uint32
	owners   []hit
	// Distinct exclude terms: their node, pattern -> exclude anchors, and exclude -> sets.
	exclNodes     []uint32
	exclAnchorOff []uint32
	exclAnchors   []uint32
	exclOwnerOff  []uint32
	exclOwners    []uint32
	grams         [4]gramFilter
	lenMask       [4]uint64
	maxLen        [4]int
	hasShort      bool

	matchers sync.Pool
}

// coreBuilder compiles sets one at a time, so their syntax trees can be dropped while building.
type coreBuilder struct {
	core *core
}

func newCoreBuilder() *coreBuilder {
	return &coreBuilder{core: &core{table: newPatternTable(), prog: &program{}}}
}

func (b *coreBuilder) add(set *rules.Compiled) uint32 {
	return b.core.prog.addSet(set, b.core.table)
}

func (b *coreBuilder) finish(estimate Estimator) *core {
	c := b.core
	b.core = nil
	c.prog.compact()
	c.table.compact()
	c.buildLookups()
	c.buildAnchors(estimate)
	c.buildOwners()
	c.matchers.New = func() any { return c.newMatcher() }
	return c
}

func (c *core) buildOwners() {
	prog := c.prog
	counts := make([]uint32, len(prog.rules)+1)
	for _, ruleID := range prog.setRules {
		counts[ruleID+1]++
	}
	for idx := range len(prog.rules) {
		counts[idx+1] += counts[idx]
	}
	c.ownerOff = counts
	c.owners = make([]hit, len(prog.setRules))
	fill := append([]uint32(nil), counts[:len(prog.rules)]...)
	for setID, set := range prog.sets {
		for position := set.ruleLo; position < set.ruleHi; position++ {
			ruleID := prog.setRules[position]
			c.owners[fill[ruleID]] = hit{uint32(setID), prog.setIncludes[position]}
			fill[ruleID]++
		}
	}
}

func (c *core) buildLookups() {
	var counts [4]int
	for _, kind := range c.table.kinds {
		counts[kind]++
	}
	for kind := rules.Prefix; kind <= rules.Infix; kind++ {
		c.grams[kind] = newGramFilter(counts[kind])
	}
	for id, kind := range c.table.kinds {
		text := c.table.texts[id]
		size := len(text)
		c.maxLen[kind] = max(c.maxLen[kind], size)
		c.lenMask[kind] |= 1 << min(size, 63)
		if kind == rules.Exact {
			continue
		}
		if size < 4 {
			c.hasShort = true
			continue
		}
		if kind == rules.Suffix {
			c.grams[kind].add(text[size-4:])
		} else {
			c.grams[kind].add(text)
		}
	}
}

// trigger appends patterns such that the node can only match when one of them hits: a literal's own
// patterns, all options of a group, and for a compound its joined forms plus the cheapest single part
// (the apart form needs every part, so any one part is a sufficient trigger).
func (c *core) trigger(nodeID uint32, costs []float64, dst []uint32) ([]uint32, float64) {
	prog := c.prog
	node := &prog.nodes[nodeID]
	cost := 0.0
	for _, pattern := range prog.pats[node.patLo:node.patHi] {
		dst = append(dst, pattern)
		cost += costs[pattern]
	}
	kids := prog.kids[node.kidLo:node.kidHi]
	switch node.typ {
	case rules.GroupNode:
		for _, kid := range kids {
			var kidCost float64
			dst, kidCost = c.trigger(kid, costs, dst)
			cost += kidCost
		}
	case rules.CompoundNode:
		var best []uint32
		bestCost := -1.0
		for _, kid := range kids {
			candidate, kidCost := c.trigger(kid, costs, nil)
			if bestCost < 0 || kidCost < bestCost {
				best, bestCost = candidate, kidCost
			}
		}
		dst = append(dst, best...)
		cost += bestCost
	}
	return dst, cost
}

// csr turns per-item lists of keys into key -> items lists (compressed sparse rows over size keys).
func csr(size int, lists [][]uint32) ([]uint32, []uint32) {
	offsets := make([]uint32, size+1)
	for _, keys := range lists {
		for _, key := range keys {
			offsets[key+1]++
		}
	}
	for idx := range size {
		offsets[idx+1] += offsets[idx]
	}
	values := make([]uint32, offsets[size])
	fill := append([]uint32(nil), offsets[:size]...)
	for item, keys := range lists {
		for _, key := range keys {
			values[fill[key]] = uint32(item)
			fill[key]++
		}
	}
	return offsets, values
}

func (c *core) buildAnchors(estimate Estimator) {
	costs := make([]float64, c.table.len())
	for id := range costs {
		// The +1 keeps patterns that match no text from looking free: they still cost lookups.
		costs[id] = estimate(rules.Pattern{Kind: c.table.kinds[id], Text: c.table.texts[id]}) + 1
	}
	prog := c.prog
	anchors := make([][]uint32, len(prog.rules))
	for ruleID := range prog.rules {
		rule := &prog.rules[ruleID]
		var best []uint32
		bestCost := -1.0
		for _, term := range prog.terms[rule.termLo:rule.posHi] {
			candidate, cost := c.trigger(term, costs, nil)
			if bestCost < 0 || cost < bestCost {
				best, bestCost = candidate, cost
			}
		}
		slices.Sort(best)
		anchors[ruleID] = slices.Compact(best)
	}
	c.anchorOff, c.anchorRules = csr(c.table.len(), anchors)

	// Excludes are inverted the same way: per text only the excludes a hit pattern triggers are evaluated,
	// and a matching one marks its sets, instead of checking every exclude of every matched set.
	dense := map[uint32]uint32{}
	var owners [][]uint32
	for setID, set := range prog.sets {
		for _, nodeID := range prog.excl[set.exclLo:set.exclHi] {
			id, ok := dense[nodeID]
			if !ok {
				id = uint32(len(c.exclNodes))
				dense[nodeID] = id
				c.exclNodes = append(c.exclNodes, nodeID)
				owners = append(owners, nil)
			}
			owners[id] = append(owners[id], uint32(setID))
		}
	}
	triggers := make([][]uint32, len(c.exclNodes))
	for id, nodeID := range c.exclNodes {
		patterns, _ := c.trigger(nodeID, costs, nil)
		slices.Sort(patterns)
		triggers[id] = slices.Compact(patterns)
	}
	c.exclAnchorOff, c.exclAnchors = csr(c.table.len(), triggers)
	c.exclOwnerOff = make([]uint32, len(owners)+1)
	for id, sets := range owners {
		c.exclOwnerOff[id+1] = c.exclOwnerOff[id] + uint32(len(sets))
		c.exclOwners = append(c.exclOwners, sets...)
	}
}

// matcher holds the per-goroutine scratch state for a core.
type matcher struct {
	core     *core
	hit      []uint64
	hitList  []uint32
	stamp    uint32
	ruleSeen []uint32
	// memo[node] is stamp<<1 | result for nodes already evaluated on this text (one cache line, not two).
	memo     []uint32
	setSeen  []uint32
	setBest  []uint32
	exclSeen []uint32
	setExcl  []uint32
	touched  []uint32
	out      []hit
}

func (c *core) newMatcher() *matcher {
	return &matcher{
		core:     c,
		hit:      make([]uint64, c.table.len()/64+1),
		ruleSeen: make([]uint32, len(c.prog.rules)),
		memo:     make([]uint32, len(c.prog.nodes)),
		setSeen:  make([]uint32, len(c.prog.sets)),
		setBest:  make([]uint32, len(c.prog.sets)),
		exclSeen: make([]uint32, len(c.exclNodes)),
		setExcl:  make([]uint32, len(c.prog.sets)),
	}
}

// eval tells whether a node matches the current text. The hit set is complete for every pattern of
// the program, so no string is touched here; shared nodes are memoised per text.
func (m *matcher) eval(nodeID uint32) bool {
	memo := m.memo[nodeID]
	if memo>>1 == m.stamp {
		return memo&1 == 1
	}
	prog := m.core.prog
	node := &prog.nodes[nodeID]
	result := false
	for _, pattern := range prog.pats[node.patLo:node.patHi] {
		if hasBit(m.hit, pattern) {
			result = true
			break
		}
	}
	if !result {
		kids := prog.kids[node.kidLo:node.kidHi]
		switch node.typ {
		case rules.GroupNode:
			for _, kid := range kids {
				if m.eval(kid) {
					result = true
					break
				}
			}
		case rules.CompoundNode:
			result = true
			for _, kid := range kids {
				if !m.eval(kid) {
					result = false
					break
				}
			}
		}
	}
	m.memo[nodeID] = m.stamp << 1
	if result {
		m.memo[nodeID] |= 1
	}
	return result
}

func (m *matcher) ruleMatches(ruleID uint32) bool {
	prog := m.core.prog
	rule := &prog.rules[ruleID]
	for _, term := range prog.terms[rule.termLo:rule.posHi] {
		if !m.eval(term) {
			return false
		}
	}
	for _, term := range prog.terms[rule.posHi:rule.termHi] {
		if m.eval(term) {
			return false
		}
	}
	return true
}

func (m *matcher) mark(id uint32) {
	word, bit := id>>6, uint64(1)<<(id&63)
	if m.hit[word]&bit == 0 {
		m.hit[word] |= bit
		m.hitList = append(m.hitList, id)
	}
}

func (m *matcher) lookup(kind rules.Kind, text string) {
	id, ok := m.core.table.ids[kind][text]
	if ok {
		m.mark(id)
	}
}

func (c *core) hasLen(kind rules.Kind, size int) bool {
	return c.lenMask[kind]&(1<<min(size, 63)) != 0
}

func (m *matcher) collect(word string) {
	c := m.core
	size := len(word)
	m.lookup(rules.Exact, word)
	if c.hasShort {
		m.collectShort(word)
	}
	if size < 4 {
		return
	}
	if c.grams[rules.Prefix].has(word) {
		for length := 4; length <= min(size, c.maxLen[rules.Prefix]); length++ {
			if c.hasLen(rules.Prefix, length) {
				m.lookup(rules.Prefix, word[:length])
			}
		}
	}
	if c.grams[rules.Suffix].has(word[size-4:]) {
		for length := 4; length <= min(size, c.maxLen[rules.Suffix]); length++ {
			if c.hasLen(rules.Suffix, length) {
				m.lookup(rules.Suffix, word[size-length:])
			}
		}
	}
	for start := 0; start+4 <= size; start++ {
		if !c.grams[rules.Infix].has(word[start:]) {
			continue
		}
		for length := 4; length <= min(size-start, c.maxLen[rules.Infix]); length++ {
			if c.hasLen(rules.Infix, length) {
				m.lookup(rules.Infix, word[start:start+length])
			}
		}
	}
}

// collectShort handles wildcard patterns under 4 letters, which only a syntax tree built without the
// parser can hold.
func (m *matcher) collectShort(word string) {
	size := len(word)
	for length := 1; length <= min(size, 3); length++ {
		m.lookup(rules.Prefix, word[:length])
		m.lookup(rules.Suffix, word[size-length:])
		for start := 0; start+length <= size; start++ {
			m.lookup(rules.Infix, word[start:start+length])
		}
	}
}

// matchWords returns the matching sets for a word set; the slice is reused by the next call.
func (m *matcher) matchWords(words []string) []hit {
	c := m.core
	m.stamp++
	if m.stamp == 1<<31 {
		clear(m.ruleSeen)
		clear(m.memo)
		clear(m.setSeen)
		clear(m.exclSeen)
		clear(m.setExcl)
		m.stamp = 1
	}
	stamp := m.stamp
	m.hitList = m.hitList[:0]
	for _, word := range words {
		m.collect(word)
	}
	m.touched = m.touched[:0]
	for _, pattern := range m.hitList {
		for _, ruleID := range c.anchorRules[c.anchorOff[pattern]:c.anchorOff[pattern+1]] {
			if m.ruleSeen[ruleID] == stamp {
				continue
			}
			m.ruleSeen[ruleID] = stamp
			if !m.ruleMatches(ruleID) {
				continue
			}
			for _, owner := range c.owners[c.ownerOff[ruleID]:c.ownerOff[ruleID+1]] {
				set := owner.set
				if m.setSeen[set] != stamp {
					m.setSeen[set] = stamp
					m.setBest[set] = owner.include
					m.touched = append(m.touched, set)
				} else if owner.include < m.setBest[set] {
					m.setBest[set] = owner.include
				}
			}
		}
	}
	if len(m.touched) > 0 {
		for _, pattern := range m.hitList {
			for _, exclude := range c.exclAnchors[c.exclAnchorOff[pattern]:c.exclAnchorOff[pattern+1]] {
				if m.exclSeen[exclude] == stamp {
					continue
				}
				m.exclSeen[exclude] = stamp
				if !m.eval(c.exclNodes[exclude]) {
					continue
				}
				for _, set := range c.exclOwners[c.exclOwnerOff[exclude]:c.exclOwnerOff[exclude+1]] {
					m.setExcl[set] = stamp
				}
			}
		}
	}
	m.out = m.out[:0]
	for _, set := range m.touched {
		if m.setExcl[set] != stamp {
			m.out = append(m.out, hit{set, m.setBest[set]})
		}
	}
	for _, pattern := range m.hitList {
		m.hit[pattern>>6] = 0
	}
	return m.out
}

func hasBit(bits []uint64, idx uint32) bool { return bits[idx>>6]&(1<<(idx&63)) != 0 }
