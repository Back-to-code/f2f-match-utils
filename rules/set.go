package rules

import (
	"fmt"
	"strings"
)

// Set is the rules for one kind of text: a text matches when one of the includes matches it and none
// of the excludes does.
type Set struct {
	Includes []string `json:"includes"`
	Excludes []string `json:"excludes"`
}

// Clean trims every include and exclude and drops the empty ones. The lists are never nil, so a
// cleaned set marshals as [] instead of null.
func (s Set) Clean() Set {
	return Set{Includes: cleanTexts(s.Includes), Excludes: cleanTexts(s.Excludes)}
}

func cleanTexts(texts []string) []string {
	out := []string{}
	for _, text := range texts {
		text = trimSpace(text)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// Problem is an include or exclude of a Set that does not parse.
type Problem struct {
	Exclude  bool
	Index    int
	Text     string
	Messages []string
}

func (p Problem) Error() string {
	where := fmt.Sprintf("includes[%d]", p.Index)
	if p.Exclude {
		where = fmt.Sprintf("excludes[%d]", p.Index)
	}
	located := make([]string, len(p.Messages))
	for idx, message := range p.Messages {
		if strings.HasPrefix(message, "'"+trimSpace(p.Text)+"'") {
			located[idx] = where + " " + message
		} else {
			located[idx] = fmt.Sprintf("%s %q: %s", where, p.Text, message)
		}
	}
	return strings.Join(located, "; ")
}

// Compiled is a Set with its includes and excludes parsed. An invalid entry is nil, so every index
// still points into the Set's lists.
type Compiled struct {
	Set      Set
	Includes []*Rule
	Excludes []*Term
}

// Compile parses every include and exclude. The result is always usable: the problems are the
// entries left out of it, and it is up to the caller whether those make the whole set unacceptable.
func Compile(set Set) (*Compiled, []Problem) {
	compiled := &Compiled{Set: set, Includes: make([]*Rule, len(set.Includes)), Excludes: make([]*Term, len(set.Excludes))}
	var problems []Problem
	for idx, text := range set.Includes {
		rule, err := ParseRule(text)
		if err != nil {
			problems = append(problems, Problem{Index: idx, Text: text, Messages: Messages(err)})
			continue
		}
		compiled.Includes[idx] = rule
	}
	for idx, text := range set.Excludes {
		term, err := ParseExclude(text)
		if err != nil {
			problems = append(problems, Problem{Exclude: true, Index: idx, Text: text, Messages: Messages(err)})
			continue
		}
		compiled.Excludes[idx] = term
	}
	return compiled, problems
}

// MustCompile is Compile for sets known to be valid; it panics on the first problem.
func MustCompile(set Set) *Compiled {
	compiled, problems := Compile(set)
	if len(problems) > 0 {
		panic(problems[0].Error())
	}
	return compiled
}

func (c *Compiled) Excluded(words []string) bool {
	for _, term := range c.Excludes {
		if term != nil && term.Node.MatchWords(words) {
			return true
		}
	}
	return false
}

// MatchWords returns the index in Includes of the first include that matches the word set, or -1 when
// none does or an exclude matches. words is a word set as returned by Words.
//
// This evaluates every rule on its own, which is fine for a few texts and rules; for many of either,
// use the textindex or ruleindex packages.
func (c *Compiled) MatchWords(words []string) int {
	if c.Excluded(words) {
		return -1
	}
	for idx, rule := range c.Includes {
		if rule != nil && rule.MatchWords(words) {
			return idx
		}
	}
	return -1
}

// Match is MatchWords on the word set of text.
func (c *Compiled) Match(text string) int { return c.MatchWords(Words(text)) }
