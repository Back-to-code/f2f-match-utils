package rules

import (
	"slices"
	"strings"
)

// PruneIncludes returns the set with every include blanked whose matches the other includes already
// have, and with the alternatives left out of an include that only bring such matches, so it matches
// the same texts. The int is how many includes it blanked or rewrote.
//
// Every index keeps pointing into the original lists, as with Compile; Clean drops the blanks. A
// rewritten include comes back in normalised syntax, the others as written. Invalid includes and the
// excludes are passed through.
//
// It reads the rules alone, never texts, so it only prunes what it can prove: an include stays when it
// takes several others together to cover it.
func (s Set) PruneIncludes() (Set, int) {
	parsed := make([]*Rule, len(s.Includes))
	index := coverIndex{patterns: map[Pattern]int{}, infixes: map[string]int{}, compounds: map[string]int{}}
	for idx, include := range s.Includes {
		rule, err := ParseRule(include)
		if err != nil {
			continue
		}
		parsed[idx] = rule
		index.addRule(rule, 1)
	}

	includes := slices.Clone(s.Includes)
	changed := 0
	// Backwards, so of includes that match the same texts the first stays and keeps their attribution
	for idx, rule := range slices.Backward(parsed) {
		if rule == nil {
			continue
		}

		index.addRule(rule, -1)
		kept := index.pruneRule(rule)
		if kept == rule {
			index.addRule(rule, 1)
			continue
		}

		changed++
		includes[idx] = ""
		if kept != nil {
			includes[idx] = kept.Source
			index.addRule(kept, 1)
		}
	}
	return Set{Includes: includes, Excludes: slices.Clone(s.Excludes)}, changed
}

// coverIndex holds the patterns of the includes that match a text through one pattern alone.
type coverIndex struct {
	patterns map[Pattern]int
	// No map finds the infixes inside a text, so they are scanned; a set has few.
	infixes map[string]int
	// compounds are kept in normalised syntax, as no pattern covers their parts matching apart.
	compounds map[string]int
}

// addRule skips includes of several terms: no single pattern makes those match.
func (c *coverIndex) addRule(rule *Rule, delta int) {
	if len(rule.Terms) == 1 {
		c.addNode(rule.Terms[0].Node, delta)
	}
}

func (c *coverIndex) addNode(node *Node, delta int) {
	switch node.Type {
	case GroupNode:
		for _, option := range node.Children {
			c.addNode(option, delta)
		}
		return
	case CompoundNode:
		addCount(c.compounds, nodeSource(node, false), delta)
	}
	for _, pattern := range node.Patterns {
		if pattern.Kind == Infix {
			addCount(c.infixes, pattern.Text, delta)
		} else {
			addCount(c.patterns, pattern, delta)
		}
	}
}

func addCount[K comparable](counts map[K]int, key K, delta int) {
	counts[key] += delta
	if counts[key] <= 0 {
		delete(counts, key)
	}
}

// covers reports whether every word pattern matches is matched by an indexed pattern.
func (c *coverIndex) covers(pattern Pattern) bool {
	text := pattern.Text
	if pattern.Kind == Exact && c.has(Exact, text) {
		return true
	}
	if pattern.Kind == Exact || pattern.Kind == Prefix {
		for end := range len(text) {
			if c.has(Prefix, text[:end+1]) {
				return true
			}
		}
	}
	if pattern.Kind == Exact || pattern.Kind == Suffix {
		for start := range len(text) {
			if c.has(Suffix, text[start:]) {
				return true
			}
		}
	}
	for infix := range c.infixes {
		if strings.Contains(text, infix) {
			return true
		}
	}
	return false
}

func (c *coverIndex) has(kind Kind, text string) bool {
	return c.patterns[Pattern{Kind: kind, Text: text}] > 0
}

func (c *coverIndex) coversAll(patterns []Pattern) bool {
	for _, pattern := range patterns {
		if !c.covers(pattern) {
			return false
		}
	}
	return true
}

// coversNode reports whether every word set node matches is matched by the index.
func (c *coverIndex) coversNode(node *Node) bool {
	switch node.Type {
	case GroupNode:
		for _, option := range node.Children {
			if !c.coversNode(option) {
				return false
			}
		}
		return true
	case CompoundNode:
		if c.compounds[nodeSource(node, false)] > 0 {
			return true
		}
		// Its parts only match together, so one covered part covers them
		return slices.ContainsFunc(node.Children, c.coversNode) && c.coversAll(node.Patterns)
	}
	return c.coversAll(node.Patterns)
}

// pruneRule returns nil when the index covers rule, a new rule when it left alternatives out, or rule
// itself.
func (c *coverIndex) pruneRule(rule *Rule) *Rule {
	// The positive terms must all match, so one covered term covers the rule
	for _, term := range rule.Positives() {
		if c.coversNode(term.Node) {
			return nil
		}
	}

	terms := slices.Clone(rule.Terms)
	changed := false
	for idx, term := range terms {
		if term.Negative {
			continue
		}
		node, nodeChanged := c.prune(term.Node)
		if nodeChanged {
			terms[idx] = &Term{Source: nodeSource(node, false), Node: node}
			changed = true
		}
	}
	if !changed {
		return rule
	}

	pruned := &Rule{Terms: terms}
	pruned.Source = pruned.NormalizedSource()
	return pruned
}

// prune returns node without the covered options of its groups, sharing the nodes it keeps, and reports
// whether it left any out. node is never changed.
func (c *coverIndex) prune(node *Node) (*Node, bool) {
	switch node.Type {
	case GroupNode:
		return c.pruneGroup(node)
	case CompoundNode:
		return c.pruneCompound(node)
	}
	return node, false
}

func (c *coverIndex) pruneGroup(group *Node) (*Node, bool) {
	var options []*Node
	changed := false
	for _, option := range group.Children {
		if c.coversNode(option) {
			changed = true
			continue
		}
		pruned, prunedChanged := c.prune(option)
		options = append(options, pruned)
		changed = changed || prunedChanged
	}
	if !changed {
		return group, false
	}
	return groupOf(options), true
}

// pruneCompound only checks the groups that are a part of compound, not deeper ones: an option there
// also changes the compound's joined forms. The group of a part never empties, as the caller already
// found that the whole compound is not covered.
func (c *coverIndex) pruneCompound(compound *Node) (*Node, bool) {
	parts := slices.Clone(compound.Children)
	changed := false
	for idx, part := range compound.Children {
		if part.Type != GroupNode {
			continue
		}
		var options []*Node
		for _, option := range part.Children {
			if c.coversVariant(compound, idx, option) {
				changed = true
				continue
			}
			options = append(options, option)
		}
		parts[idx] = groupOf(options)
	}
	if !changed {
		return compound, false
	}

	pruned := &Node{Type: CompoundNode, Children: parts}
	pruned.Forms, pruned.Patterns = joinedForms(pruned)
	return pruned, true
}

// coversVariant reports whether the index covers compound with option in place of the group at part.
// Most variants fail on their parts, so few get their joined forms built.
func (c *coverIndex) coversVariant(compound *Node, part int, option *Node) bool {
	parts := slices.Clone(compound.Children)
	parts[part] = option
	if !slices.ContainsFunc(parts, c.coversNode) {
		return false
	}

	_, patterns := joinedForms(&Node{Type: CompoundNode, Children: parts})
	return c.coversAll(patterns)
}

func groupOf(options []*Node) *Node {
	if len(options) == 1 {
		return options[0]
	}
	return &Node{Type: GroupNode, Children: options}
}
