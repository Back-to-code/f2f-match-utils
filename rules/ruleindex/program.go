package ruleindex

import (
	"slices"
	"strings"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

// patternTable interns word patterns: every distinct (kind, text) gets one dense id.
type patternTable struct {
	ids   [4]map[string]uint32
	kinds []rules.Kind
	texts []string
}

func newPatternTable() *patternTable {
	table := &patternTable{}
	for kind := range table.ids {
		table.ids[kind] = map[string]uint32{}
	}
	return table
}

func (t *patternTable) intern(pattern rules.Pattern) uint32 {
	id, ok := t.ids[pattern.Kind][pattern.Text]
	if ok {
		return id
	}
	id = uint32(len(t.kinds))
	// Cloned so the table owns its memory and does not pin the syntax tree the text came from.
	text := strings.Clone(pattern.Text)
	t.ids[pattern.Kind][text] = id
	t.kinds = append(t.kinds, pattern.Kind)
	t.texts = append(t.texts, text)
	return id
}

func (t *patternTable) len() int { return len(t.kinds) }

type progNode struct {
	typ          rules.NodeType
	patLo, patHi uint32
	kidLo, kidHi uint32
}

type progRule struct {
	// terms[termLo:posHi] are the positive terms, terms[posHi:termHi] the negative ones.
	termLo, posHi, termHi uint32
}

type progSet struct {
	// setRules[ruleLo:ruleHi] are the set's valid includes in order.
	ruleLo, ruleHi uint32
	exclLo, exclHi uint32
}

// program is the flat, pointer-free form of many sets: nodes refer to pattern ids of a patternTable.
// Nodes and rules are hash-consed: an identical subtree (say medewerker~, or a whole rule several sets
// share) is stored once, so it costs memory once and, with a per-text memo, is evaluated once per text.
type program struct {
	nodes []progNode
	pats  []uint32
	kids  []uint32
	terms []uint32
	rules []progRule
	// setRules holds rule ids, setIncludes the index of each in its set's Includes.
	setRules    []uint32
	setIncludes []uint32
	sets        []progSet
	excl        []uint32

	nodeKeys map[string]uint32
	ruleKeys map[string]uint32
	keyBuf   []byte
}

func appendKey(key []byte, values []uint32) []byte {
	for _, value := range values {
		key = append(key, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
	}
	return key
}

func (pr *program) addNode(node *rules.Node, table *patternTable) uint32 {
	kidIDs := make([]uint32, 0, len(node.Children))
	for _, child := range node.Children {
		kidIDs = append(kidIDs, pr.addNode(child, table))
	}
	patIDs := make([]uint32, 0, len(node.Patterns))
	for _, pattern := range node.Patterns {
		patIDs = append(patIDs, table.intern(pattern))
	}
	if pr.nodeKeys == nil {
		pr.nodeKeys = map[string]uint32{}
		pr.ruleKeys = map[string]uint32{}
	}
	key := appendKey(append(pr.keyBuf[:0], byte(node.Type)), []uint32{uint32(len(patIDs))})
	key = appendKey(appendKey(key, patIDs), kidIDs)
	pr.keyBuf = key
	id, ok := pr.nodeKeys[string(key)]
	if ok {
		return id
	}
	compiled := progNode{typ: node.Type, patLo: uint32(len(pr.pats)), kidLo: uint32(len(pr.kids))}
	pr.pats = append(pr.pats, patIDs...)
	pr.kids = append(pr.kids, kidIDs...)
	compiled.patHi = uint32(len(pr.pats))
	compiled.kidHi = uint32(len(pr.kids))
	id = uint32(len(pr.nodes))
	pr.nodes = append(pr.nodes, compiled)
	pr.nodeKeys[string(key)] = id
	return id
}

func (pr *program) addRule(rule *rules.Rule, table *patternTable) uint32 {
	var positives, negatives []uint32
	for _, term := range rule.Terms {
		nodeID := pr.addNode(term.Node, table)
		if term.Negative {
			negatives = append(negatives, nodeID)
		} else {
			positives = append(positives, nodeID)
		}
	}
	// Term order does not change what a rule matches, so `a b` and `b a` share one rule.
	slices.Sort(positives)
	slices.Sort(negatives)
	key := appendKey(pr.keyBuf[:0], []uint32{uint32(len(positives))})
	key = appendKey(appendKey(key, positives), negatives)
	pr.keyBuf = key
	id, ok := pr.ruleKeys[string(key)]
	if ok {
		return id
	}
	compiled := progRule{termLo: uint32(len(pr.terms))}
	pr.terms = append(pr.terms, positives...)
	compiled.posHi = uint32(len(pr.terms))
	pr.terms = append(pr.terms, negatives...)
	compiled.termHi = uint32(len(pr.terms))
	id = uint32(len(pr.rules))
	pr.rules = append(pr.rules, compiled)
	pr.ruleKeys[string(key)] = id
	return id
}

func (pr *program) addSet(set *rules.Compiled, table *patternTable) uint32 {
	id := uint32(len(pr.sets))
	compiled := progSet{ruleLo: uint32(len(pr.setRules))}
	for include, rule := range set.Includes {
		if rule == nil {
			continue
		}
		pr.setRules = append(pr.setRules, pr.addRule(rule, table))
		pr.setIncludes = append(pr.setIncludes, uint32(include))
	}
	compiled.ruleHi = uint32(len(pr.setRules))
	compiled.exclLo = uint32(len(pr.excl))
	for _, term := range set.Excludes {
		if term != nil {
			pr.excl = append(pr.excl, pr.addNode(term.Node, table))
		}
	}
	compiled.exclHi = uint32(len(pr.excl))
	pr.sets = append(pr.sets, compiled)
	return id
}

func exact[T any](values []T) []T { return append(make([]T, 0, len(values)), values...) }

// compact drops the build-only maps and the spare capacity of the growing slices; with many sets that
// is a large share of the heap.
func (pr *program) compact() {
	pr.nodeKeys = nil
	pr.ruleKeys = nil
	pr.keyBuf = nil
	pr.nodes = exact(pr.nodes)
	pr.pats = exact(pr.pats)
	pr.kids = exact(pr.kids)
	pr.terms = exact(pr.terms)
	pr.rules = exact(pr.rules)
	pr.setRules = exact(pr.setRules)
	pr.setIncludes = exact(pr.setIncludes)
	pr.sets = exact(pr.sets)
	pr.excl = exact(pr.excl)
}

func (t *patternTable) compact() {
	t.kinds = exact(t.kinds)
	t.texts = exact(t.texts)
}
