// Package synth generates made-up texts and rule sets for tests and benchmarks. The words are generic
// job title words plus generated pseudo-words; word and text frequencies follow a power law, so a large
// sample repeats texts and words the way real titles do.
package synth

import (
	"math/rand/v2"
	"strings"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

var modifiers = []string{
	"senior", "junior", "medior", "allround", "ervaren", "assistent", "hoofd", "leerling", "stagiair",
	"eerste", "tweede", "zelfstandig", "fulltime", "parttime", "lead", "chief", "trainee", "interim",
}

var sectors = []string{
	"magazijn", "logistiek", "productie", "verkoop", "klantenservice", "administratie", "financiele",
	"commerciele", "technische", "zorg", "bouw", "horeca", "keuken", "schoonmaak", "onderhoud", "service",
	"installatie", "elektro", "werkplaats", "transport", "inkoop", "marketing", "personeel", "ict", "data",
	"software", "kwaliteit", "beveiliging", "receptie", "facilitair", "planning", "project", "order",
	"voorraad", "warehouse", "customer", "sales", "office", "field", "retail", "winkel", "filiaal",
}

var nouns = []string{
	"medewerker", "monteur", "chauffeur", "verkoper", "adviseur", "manager", "planner", "coordinator",
	"engineer", "developer", "assistent", "leider", "specialist", "consultant", "operator", "analist",
	"kok", "schoonmaker", "verpleegkundige", "begeleider", "docent", "accountant", "controller",
	"timmerman", "loodgieter", "elektricien", "lasser", "schilder", "orderpicker", "heftruckchauffeur",
	"vrachtwagenchauffeur", "receptionist", "secretaresse", "boekhouder", "inkoper", "programmeur",
	"tester", "architect", "ontwerper", "tekenaar", "bediende", "kassamedewerker", "vakkenvuller",
	"teamleider", "supervisor", "directeur", "officer", "agent", "representative", "clerk", "worker",
}

var connectors = []string{"bij", "in", "en", "van", "voor", "de", "/", "-", "&"}

var syllables = []string{
	"ka", "ro", "vin", "ter", "mos", "del", "an", "bru", "sel", "pi", "lo", "mar", "ven", "tis", "or",
	"gal", "ne", "zu", "fer", "dam", "kel", "rin", "op", "sta", "mi", "bor", "len", "tu", "has", "e",
}

var roleEndings = []string{"er", "eur", "ist", "man", "aar", "ier", "ant", "ent", "or", "ster"}

// Generator is not safe for concurrent use.
type Generator struct {
	rng      *rand.Rand
	nouns    []string
	sectors  []string
	nounZipf *rand.Zipf
	sectZipf *rand.Zipf
}

// New makes a generator whose vocabulary grows with pseudoWords made-up nouns and sectors on top of the
// fixed lists; more of them means more distinct patterns and texts at scale.
func New(seed uint64, pseudoWords int) *Generator {
	rng := rand.New(rand.NewPCG(seed, 0x5eed))
	gen := &Generator{rng: rng, nouns: append([]string(nil), nouns...), sectors: append([]string(nil), sectors...)}
	seen := map[string]bool{}
	for _, word := range append(append([]string(nil), nouns...), sectors...) {
		seen[word] = true
	}
	for len(gen.nouns)+len(gen.sectors) < len(nouns)+len(sectors)+pseudoWords {
		var word strings.Builder
		for range 2 + rng.IntN(3) {
			word.WriteString(syllables[rng.IntN(len(syllables))])
		}
		if rng.IntN(2) == 0 {
			word.WriteString(roleEndings[rng.IntN(len(roleEndings))])
			if !seen[word.String()] {
				gen.nouns = append(gen.nouns, word.String())
			}
		} else if !seen[word.String()] {
			gen.sectors = append(gen.sectors, word.String())
		}
		seen[word.String()] = true
	}
	gen.nounZipf = rand.NewZipf(rng, 1.15, 2, uint64(len(gen.nouns)-1))
	gen.sectZipf = rand.NewZipf(rng, 1.15, 2, uint64(len(gen.sectors)-1))
	return gen
}

func (g *Generator) noun() string   { return g.nouns[g.nounZipf.Uint64()] }
func (g *Generator) sector() string { return g.sectors[g.sectZipf.Uint64()] }

// Rules are about specific work, so their words are drawn evenly rather than by how common they are.
func (g *Generator) anyNoun() string   { return g.pick(g.nouns) }
func (g *Generator) anySector() string { return g.pick(g.sectors) }

func (g *Generator) pick(words []string) string { return words[g.rng.IntN(len(words))] }

func (g *Generator) modifier() string { return g.pick(modifiers) }

func (g *Generator) inflect(word string) string {
	switch g.rng.IntN(10) {
	case 0:
		return word + "s"
	case 1:
		if strings.HasSuffix(word, "er") {
			return word[:len(word)-2] + "ster"
		}
		return word + "e"
	case 2:
		return word + "en"
	}
	return word
}

func (g *Generator) style(text string) string {
	switch g.rng.IntN(8) {
	case 0:
		return strings.ToUpper(text[:1]) + text[1:]
	case 1:
		return strings.ToUpper(text)
	case 2:
		return strings.ReplaceAll(text, "e", "é")
	}
	return text
}

// Text returns a title-like text of one to six words.
func (g *Generator) Text() string {
	var parts []string
	if g.rng.IntN(4) == 0 {
		parts = append(parts, g.modifier())
	}
	noun := g.inflect(g.noun())
	switch g.rng.IntN(6) {
	case 0:
		parts = append(parts, g.sector()+noun)
	case 1:
		parts = append(parts, g.sector()+"-"+noun)
	case 2:
		parts = append(parts, g.sector(), noun)
	case 3:
		parts = append(parts, noun, g.pick(connectors), g.sector())
	default:
		parts = append(parts, noun)
	}
	if g.rng.IntN(5) == 0 {
		parts = append(parts, g.pick(connectors), g.sector())
	}
	return g.style(strings.Join(parts, " "))
}

func (g *Generator) stem(word string, size int) string {
	return word[:min(len(word), max(rules.MinWildcardLetters, size+g.rng.IntN(3)-1))]
}

func (g *Generator) ending(word string, size int) string {
	return word[len(word)-min(len(word), max(rules.MinWildcardLetters, size+g.rng.IntN(3)-1)):]
}

// long returns a word with at least the letters a wildcard needs.
func (g *Generator) long(next func() string) string {
	for {
		word := next()
		if len(word) >= rules.MinWildcardLetters {
			return word
		}
	}
}

// Rule returns a valid rule that uses most of the syntax.
func (g *Generator) Rule() string {
	noun, sector := g.long(g.anyNoun), g.long(g.anySector)
	switch roll := g.rng.Float64(); {
	case roll < 0.12:
		return noun
	case roll < 0.30:
		return noun + "~"
	case roll < 0.40:
		return g.stem(sector, 6) + "*"
	case roll < 0.50:
		return "*" + g.ending(noun, 7)
	case roll < 0.65:
		return sector + "+" + noun + "~"
	case roll < 0.75:
		return "(" + sector + "|" + g.long(g.anySector) + ")+" + noun + "~"
	case roll < 0.82:
		return noun + "~ " + g.stem(sector, 5) + "*"
	case roll < 0.89:
		return sector + "+" + noun + " -" + g.stem(g.long(g.modifier), 5) + "*"
	case roll < 0.94:
		return noun + "|" + g.long(g.anyNoun) + "~|*" + g.ending(g.long(g.anyNoun), 6)
	case roll < 0.97:
		return "*" + g.stem(noun, 6) + "*"
	}
	return "*" + sector + "+" + noun + "~"
}

func (g *Generator) Exclude() string {
	switch g.rng.IntN(4) {
	case 0:
		return g.stem(g.long(g.modifier), 5) + "*"
	case 1:
		return "*" + g.ending(g.long(g.anyNoun), 6)
	case 2:
		return g.long(g.anySector) + "+" + g.long(g.anyNoun) + "~"
	}
	return g.modifier()
}

func (g *Generator) Set(includes, excludes int) rules.Set {
	set := rules.Set{Includes: make([]string, includes), Excludes: make([]string, excludes)}
	for idx := range set.Includes {
		set.Includes[idx] = g.Rule()
	}
	for idx := range set.Excludes {
		set.Excludes[idx] = g.Exclude()
	}
	return set
}
