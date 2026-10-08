package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

const (
	// MinWildcardLetters is the least number of letters a word with * or ~ may have.
	MinWildcardLetters = 4
	// MaxJoinedForms caps the joined forms one term may expand to. They are the cartesian product of a
	// compound's groups; beyond this a rule is a word list in disguise and should be split.
	MaxJoinedForms = 2000
)

// Kind is how a word pattern compares against one word of a text.
type Kind uint8

const (
	Exact Kind = iota
	Prefix
	Suffix
	Infix
)

var kindNames = [...]string{"exact", "prefix", "suffix", "infix"}

func (k Kind) String() string { return kindNames[k] }

// Pattern is tested against a single word of a text's word set.
type Pattern struct {
	Kind Kind
	Text string
}

func (p Pattern) Matches(word string) bool {
	switch p.Kind {
	case Exact:
		return word == p.Text
	case Prefix:
		return strings.HasPrefix(word, p.Text)
	case Suffix:
		return strings.HasSuffix(word, p.Text)
	}
	return strings.Contains(word, p.Text)
}

func (p Pattern) String() string {
	lead, trail := "", ""
	if p.Kind == Suffix || p.Kind == Infix {
		lead = "*"
	}
	if p.Kind == Prefix || p.Kind == Infix {
		trail = "*"
	}
	return lead + p.Text + trail
}

// Form is a folded literal, or the joined form of a compound, with its wildcards.
type Form struct {
	Text  string
	Lead  bool
	Trail bool
	Tilde bool
}

func (f Form) Kind() Kind {
	switch {
	case f.Lead && f.Trail:
		return Infix
	case f.Lead:
		return Suffix
	case f.Trail:
		return Prefix
	}
	return Exact
}

// Patterns returns one pattern, or one per inflection when the form ends in ~.
func (f Form) Patterns() []Pattern {
	kind := f.Kind()
	if !f.Tilde {
		return []Pattern{{kind, f.Text}}
	}
	texts := Inflections(f.Text)
	patterns := make([]Pattern, len(texts))
	for idx, text := range texts {
		patterns[idx] = Pattern{kind, text}
	}
	return patterns
}

func (f Form) String() string {
	return star(f.Lead) + f.Text + star(f.Trail) + tildeMark(f.Tilde)
}

func star(on bool) string {
	if on {
		return "*"
	}
	return ""
}

func tildeMark(on bool) string {
	if on {
		return "~"
	}
	return ""
}

type NodeType uint8

const (
	LiteralNode NodeType = iota
	GroupNode
	CompoundNode
)

// Node is a literal, a group of alternatives (Children are the options) or a compound (Children are
// the parts).
type Node struct {
	Type NodeType
	// Raw, Lead, Trail and Tilde are the literal as written; Form is it folded.
	Raw      string
	Lead     bool
	Trail    bool
	Tilde    bool
	Form     Form
	Children []*Node
	// Forms are a compound's joined forms.
	Forms []Form
	// Patterns are a literal's patterns, or the patterns of a compound's joined forms.
	Patterns []Pattern
}

func (n *Node) literalSource() string {
	return star(n.Lead) + n.Raw + star(n.Trail) + tildeMark(n.Tilde)
}

// MatchWords reports whether the node matches a word set as returned by Words.
func (n *Node) MatchWords(words []string) bool {
	if n.Type == GroupNode {
		for _, option := range n.Children {
			if option.MatchWords(words) {
				return true
			}
		}
		return false
	}
	for _, pattern := range n.Patterns {
		for _, word := range words {
			if pattern.Matches(word) {
				return true
			}
		}
	}
	if n.Type != CompoundNode {
		return false
	}
	for _, part := range n.Children {
		if !part.MatchWords(words) {
			return false
		}
	}
	return true
}

// Term is one space separated part of a rule, or an exclude.
type Term struct {
	Source   string
	Negative bool
	Node     *Node
}

// Body is the term in normalised rule syntax, without the leading '-'.
func (t *Term) Body() string { return nodeSource(t.Node, false) }

// Rule matches a text when every positive term matches it and no negative term does.
type Rule struct {
	// Source is the rule as written, without surrounding whitespace.
	Source string
	Terms  []*Term
}

func (r *Rule) Positives() []*Term { return r.filter(false) }
func (r *Rule) Negatives() []*Term { return r.filter(true) }

func (r *Rule) filter(negative bool) []*Term {
	var terms []*Term
	for _, term := range r.Terms {
		if term.Negative == negative {
			terms = append(terms, term)
		}
	}
	return terms
}

// MatchWords reports whether the rule matches a word set as returned by Words.
func (r *Rule) MatchWords(words []string) bool {
	for _, term := range r.Terms {
		if !term.Negative && !term.Node.MatchWords(words) {
			return false
		}
	}
	for _, term := range r.Terms {
		if term.Negative && term.Node.MatchWords(words) {
			return false
		}
	}
	return true
}

func (r *Rule) Match(text string) bool { return r.MatchWords(Words(text)) }

// NormalizedSource prints the rule in normalised syntax: folded literals, single spaces between terms
// and only the parentheses that are needed. Rules that print the same match the same.
func (r *Rule) NormalizedSource() string {
	parts := make([]string, len(r.Terms))
	for idx, term := range r.Terms {
		parts[idx] = term.Body()
		if term.Negative {
			parts[idx] = "-" + parts[idx]
		}
	}
	return strings.Join(parts, " ")
}

// SyntaxError lists every problem found in one rule or exclude.
type SyntaxError struct {
	Input    string
	Messages []string
}

func (e *SyntaxError) Error() string {
	return strings.Join(e.Messages, "; ")
}

type parseError struct {
	message string
	// located messages already quote the literal they are about, so the term is not prefixed.
	located bool
}

func (e *parseError) Error() string { return e.message }

func isOperatorToken(token string) bool {
	return len(token) == 1 && strings.IndexByte("()|+*~", token[0]) >= 0
}

func tokenize(body string) []string {
	var tokens []string
	start := -1
	for idx := range len(body) {
		if strings.IndexByte("()|+*~", body[idx]) >= 0 {
			if start >= 0 {
				tokens = append(tokens, body[start:idx])
				start = -1
			}
			tokens = append(tokens, body[idx:idx+1])
		} else if start < 0 {
			start = idx
		}
	}
	if start >= 0 {
		tokens = append(tokens, body[start:])
	}
	return tokens
}

type parser struct {
	tokens []string
	pos    int
}

// peek returns "" at the end; tokens are never empty.
func (p *parser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *parser) previous() string {
	if p.pos > 0 {
		return p.tokens[p.pos-1]
	}
	return ""
}

func (p *parser) take() string {
	token := p.tokens[p.pos]
	p.pos++
	return token
}

func (p *parser) parse() (*Node, error) {
	node, err := p.alt()
	if err != nil {
		return nil, err
	}
	token := p.peek()
	if token == "(" {
		return nil, &parseError{message: "a group must be joined to its neighbour with + or |"}
	}
	if token != "" {
		return nil, &parseError{message: fmt.Sprintf("unexpected '%s'", token)}
	}
	return node, nil
}

func (p *parser) alt() (*Node, error) {
	var options []*Node
	for {
		node, err := p.seq()
		if err != nil {
			return nil, err
		}
		if node.Type == GroupNode {
			options = append(options, node.Children...)
		} else {
			options = append(options, node)
		}
		if p.peek() != "|" {
			break
		}
		p.take()
	}
	if len(options) == 1 {
		return options[0], nil
	}
	return &Node{Type: GroupNode, Children: options}, nil
}

func (p *parser) seq() (*Node, error) {
	first, err := p.unit()
	if err != nil {
		return nil, err
	}
	parts := []*Node{first}
	for p.peek() == "+" {
		p.take()
		part, err := p.unit()
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return &Node{Type: CompoundNode, Children: parts}, nil
}

func (p *parser) unit() (*Node, error) {
	if p.peek() != "(" {
		return p.literal()
	}
	p.take()
	node, err := p.alt()
	if err != nil {
		return nil, err
	}
	if p.peek() != ")" {
		return nil, &parseError{message: "missing ')'"}
	}
	p.take()
	next := p.peek()
	if next == "*" || next == "~" {
		return nil, &parseError{message: fmt.Sprintf("'%s' must be attached to a word, not to a group: write (a%s|b%s)", next, next, next)}
	}
	return node, nil
}

func (p *parser) literal() (*Node, error) {
	lead := p.peek() == "*"
	if lead {
		p.take()
	}
	token := p.peek()
	if token == "" || isOperatorToken(token) {
		return nil, &parseError{message: p.missingWord(token)}
	}
	node := &Node{Type: LiteralNode, Raw: p.take(), Lead: lead}
	node.Trail = p.peek() == "*"
	if node.Trail {
		p.take()
	}
	node.Tilde = p.peek() == "~"
	if node.Tilde {
		p.take()
	}
	source := node.literalSource()
	if node.Trail && node.Tilde {
		return nil, &parseError{message: fmt.Sprintf("'%s': a word can't end in both * and ~", source), located: true}
	}
	next := p.peek()
	if next == "*" || next == "~" {
		return nil, &parseError{message: fmt.Sprintf("'%s%s': * and ~ only once per side, and ~ must come last", source, next), located: true}
	}
	if p.peek() == "(" {
		return nil, &parseError{message: fmt.Sprintf("'%s' must be joined to the group after it with + or |", source), located: true}
	}
	return node, nil
}

func (p *parser) missingWord(token string) string {
	previous := p.previous()
	switch {
	case token == "*":
		return "'**' is not allowed"
	case previous == "*":
		if token == "(" {
			return "'*' must be attached to a word, not to a group: write (*a|*b)"
		}
		return "'*' must be followed by a word"
	case token == "~":
		return "'~' must follow a word: write word~"
	case token == ")" && previous == "(":
		return "empty group '()'"
	case token == "":
		return fmt.Sprintf("'%s' must be followed by a word or group", previous)
	case previous == "":
		return fmt.Sprintf("'%s' needs a word before it", token)
	}
	return fmt.Sprintf("missing word between '%s' and '%s'", previous, token)
}

// checkLiterals validates every literal and fills in its Form. left and right tell whether the node
// sits on the outer left or right edge of its term, where alone wildcards are allowed.
func checkLiterals(node *Node, left, right bool, errs []string) []string {
	switch node.Type {
	case GroupNode:
		for _, option := range node.Children {
			errs = checkLiterals(option, left, right, errs)
		}
		return errs
	case CompoundNode:
		last := len(node.Children) - 1
		for idx, part := range node.Children {
			errs = checkLiterals(part, left && idx == 0, right && idx == last, errs)
		}
		return errs
	}
	source := node.literalSource()
	text := Fold(node.Raw)
	var bad []string
	for _, r := range text {
		if r > unicode.MaxASCII || !isWordByte(byte(r)) {
			bad = append(bad, string(r))
		}
	}
	slices.Sort(bad)
	bad = slices.Compact(bad)
	switch {
	case text == "":
		errs = append(errs, fmt.Sprintf("'%s' has no letters or digits", source))
	case slices.Contains(bad, "-"):
		errs = append(errs, fmt.Sprintf("'%s': '-' is only allowed at the start of a term (to exclude it); write a hyphenated word as a+b or ab", source))
	case len(bad) > 0:
		quoted := make([]string, len(bad))
		for idx, ch := range bad {
			quoted[idx] = "'" + ch + "'"
		}
		errs = append(errs, fmt.Sprintf("'%s': a word may only contain letters and digits, found %s", source, strings.Join(quoted, " ")))
	case (node.Lead || node.Trail || node.Tilde) && len(text) < MinWildcardLetters:
		errs = append(errs, fmt.Sprintf("'%s': a word with * or ~ needs at least %d letters, '%s' has %d", source, MinWildcardLetters, text, len(text)))
	}
	if node.Lead && !left {
		errs = append(errs, fmt.Sprintf("'%s': a leading * is only allowed on the first part of a compound", source))
	}
	if (node.Trail || node.Tilde) && !right {
		mark := "~"
		if node.Trail {
			mark = "*"
		}
		errs = append(errs, fmt.Sprintf("'%s': a trailing %s is only allowed on the last part of a compound", source, mark))
	}
	node.Form = Form{Text: text, Lead: node.Lead, Trail: node.Trail, Tilde: node.Tilde}
	return errs
}

// Counts saturate here: they are only compared against MaxJoinedForms and never reach zero.
const countCap = 1 << 40

func countForms(node *Node) int {
	switch node.Type {
	case LiteralNode:
		return 1
	case GroupNode:
		total := 0
		for _, option := range node.Children {
			total = min(total+countForms(option), countCap)
		}
		return total
	}
	product := 1
	for _, part := range node.Children {
		factor := countForms(part)
		if product > countCap/factor {
			return countCap
		}
		product *= factor
	}
	return product
}

// countJoined counts the joined forms of every compound in the tree, nested ones included.
func countJoined(node *Node) int {
	switch node.Type {
	case LiteralNode:
		return 0
	case GroupNode:
		total := 0
		for _, option := range node.Children {
			total = min(total+countJoined(option), countCap)
		}
		return total
	}
	total := countForms(node)
	for _, part := range node.Children {
		total = min(total+countJoined(part), countCap)
	}
	return total
}

func combinations(node *Node) [][]Form {
	switch node.Type {
	case LiteralNode:
		return [][]Form{{node.Form}}
	case GroupNode:
		var combos [][]Form
		for _, option := range node.Children {
			combos = append(combos, combinations(option)...)
		}
		return combos
	}
	combos := [][]Form{nil}
	for _, part := range node.Children {
		partCombos := combinations(part)
		next := make([][]Form, 0, len(combos)*len(partCombos))
		for _, prefix := range combos {
			for _, suffix := range partCombos {
				combo := make([]Form, 0, len(prefix)+len(suffix))
				combo = append(append(combo, prefix...), suffix...)
				next = append(next, combo)
			}
		}
		combos = next
	}
	return combos
}

// prepare fills in the patterns of literals and the joined forms of compounds.
func prepare(node *Node) {
	if node.Type == LiteralNode {
		node.Patterns = node.Form.Patterns()
		return
	}
	for _, child := range node.Children {
		prepare(child)
	}
	if node.Type != CompoundNode {
		return
	}
	seenForms := map[Form]bool{}
	seenPatterns := map[Pattern]bool{}
	for _, combo := range combinations(node) {
		var text strings.Builder
		for _, form := range combo {
			text.WriteString(form.Text)
		}
		last := combo[len(combo)-1]
		form := Form{Text: text.String(), Lead: combo[0].Lead, Trail: last.Trail, Tilde: last.Tilde}
		if seenForms[form] {
			continue
		}
		seenForms[form] = true
		node.Forms = append(node.Forms, form)
		for _, pattern := range form.Patterns() {
			if !seenPatterns[pattern] {
				seenPatterns[pattern] = true
				node.Patterns = append(node.Patterns, pattern)
			}
		}
	}
}

// isSpace is unicode.IsSpace plus the ASCII information separators, as in the syntax's first
// implementation, so a rule splits into the same terms everywhere it is validated.
func isSpace(r rune) bool {
	return unicode.IsSpace(r) || (0x1c <= r && r <= 0x1f)
}

func trimSpace(text string) string {
	return strings.TrimFunc(text, isSpace)
}

func parseTerm(source string) (*Term, []string) {
	negative := strings.HasPrefix(source, "-")
	body := source
	if negative {
		body = source[1:]
	}
	if body == "" {
		return nil, []string{"'-' must be followed by a term"}
	}
	if strings.HasPrefix(body, "-") {
		return nil, []string{fmt.Sprintf("'%s': only one leading '-' is allowed", source)}
	}
	if strings.IndexByte("|+", body[0]) >= 0 || strings.IndexByte("|+", body[len(body)-1]) >= 0 {
		return nil, []string{fmt.Sprintf("'%s': | and + need a word on both sides and no spaces around them (a|b, a+b)", source)}
	}
	node, err := (&parser{tokens: tokenize(body)}).parse()
	if err != nil {
		var located *parseError
		if errors.As(err, &located) && located.located {
			return nil, []string{located.message}
		}
		return nil, []string{fmt.Sprintf("'%s': %s", source, err.Error())}
	}
	errs := checkLiterals(node, true, true, nil)
	joined := countJoined(node)
	if joined > MaxJoinedForms {
		errs = append(errs, fmt.Sprintf("'%s': expands to %d joined forms (limit %d); split it into several rules", source, joined, MaxJoinedForms))
	}
	if len(errs) > 0 {
		return nil, errs
	}
	prepare(node)
	return &Term{Source: source, Negative: negative, Node: node}, nil
}

func splitTerms(rule string) ([]string, []string) {
	depth := 0
	for _, ch := range rule {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, []string{"unbalanced parentheses: ')' without a matching '('"}
			}
		}
	}
	if depth != 0 {
		return nil, []string{"unbalanced parentheses: '(' without a matching ')'"}
	}
	var terms []string
	var current strings.Builder
	for _, ch := range rule {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		}
		switch {
		case !isSpace(ch):
			current.WriteRune(ch)
		case depth != 0:
			return nil, []string{"no spaces inside ( ): write (a|b), not (a | b)"}
		case current.Len() > 0:
			terms = append(terms, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		terms = append(terms, current.String())
	}
	return terms, nil
}

// ParseRule parses one rule. On failure the error is a *SyntaxError that lists every problem found,
// worded so it can be shown to whoever wrote the rule.
func ParseRule(rule string) (*Rule, error) {
	sources, errs := splitTerms(rule)
	if len(errs) > 0 {
		return nil, &SyntaxError{Input: rule, Messages: errs}
	}
	if len(sources) == 0 {
		return nil, &SyntaxError{Input: rule, Messages: []string{"empty rule"}}
	}
	var terms []*Term
	allNegative := true
	for _, source := range sources {
		term, termErrs := parseTerm(source)
		errs = append(errs, termErrs...)
		if term != nil {
			terms = append(terms, term)
		}
		allNegative = allNegative && strings.HasPrefix(source, "-")
	}
	if allNegative {
		errs = append(errs, "a rule needs at least one term without '-'")
	}
	if len(errs) > 0 {
		return nil, &SyntaxError{Input: rule, Messages: errs}
	}
	return &Rule{Source: trimSpace(rule), Terms: terms}, nil
}

// ParseExclude parses an exclude: one term, without spaces and without a leading '-'. On failure the
// error is a *SyntaxError.
func ParseExclude(term string) (*Term, error) {
	source := trimSpace(term)
	fail := func(messages ...string) (*Term, error) {
		return nil, &SyntaxError{Input: term, Messages: messages}
	}
	if source == "" {
		return fail("empty exclude")
	}
	if strings.IndexFunc(source, isSpace) >= 0 {
		return fail(fmt.Sprintf("'%s': an exclude is one term without spaces; join words with + (a+b)", source))
	}
	if strings.HasPrefix(source, "-") {
		return fail(fmt.Sprintf("'%s': an exclude is already negative, drop the leading '-'", source))
	}
	_, errs := splitTerms(source)
	if len(errs) > 0 {
		return fail(errs...)
	}
	parsed, errs := parseTerm(source)
	if len(errs) > 0 {
		return fail(errs...)
	}
	return parsed, nil
}

func MustParseRule(rule string) *Rule {
	parsed, err := ParseRule(rule)
	if err != nil {
		panic(err)
	}
	return parsed
}

// Messages returns the problems of a *SyntaxError, or the error's text for any other error.
func Messages(err error) []string {
	if err == nil {
		return nil
	}
	var syntax *SyntaxError
	if errors.As(err, &syntax) {
		return syntax.Messages
	}
	return []string{err.Error()}
}

// nodeSource prints the node in normalised rule syntax; + binds tighter than | so only compound parts
// need parentheses.
func nodeSource(node *Node, inCompound bool) string {
	if node.Type == LiteralNode {
		return node.Form.String()
	}
	parts := make([]string, len(node.Children))
	separator := "|"
	if node.Type == CompoundNode {
		separator = "+"
	}
	for idx, child := range node.Children {
		parts[idx] = nodeSource(child, node.Type == CompoundNode)
	}
	text := strings.Join(parts, separator)
	if inCompound {
		return "(" + text + ")"
	}
	return text
}
