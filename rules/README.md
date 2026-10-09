# rules

A small pattern language for matching short texts such as job titles, and two indexes that match it
at scale. A **rule** matches one text; a **set** of rules decides whether a text belongs to something.

```go
import (
	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/ruleindex"
	"github.com/Back-to-code/f2f-match-utils/rules/textindex"
)
```

| package | for |
| --- | --- |
| `rules` | parsing, validation with readable errors, `Explain`, normalisation, matching one set against one text |
| `rules/textindex` | a few dozen rules at a time against up to millions of stored texts |
| `rules/ruleindex` | a few texts at a time against up to millions of stored rules |

Both indexes give exactly the results of evaluating every rule on every text with `rules.Compiled`;
they only get there faster.

## The syntax

A set has **includes** (rules) and **excludes** (terms). A text matches a set when one of its includes
matches the text and none of its excludes does.

```json
{
  "includes": ["(magazijn|warehouse)+medewerker~", "orderpicker~", "logistiek* -manager~"],
  "excludes": ["*manager", "stage*"]
}
```

### Normalisation

Texts and the words in rules are normalised the same way, by the fold of the [`sanitize`](../sanitize)
package:

1. Lowercase and remove diacritics: `é` → `e`, `ë` → `e`, `ß` → `ss`, `ø` → `o`, `æ` → `ae`, `œ` → `oe`, `ł` → `l`,
   `đ` → `d`, `ð` → `d`, `ħ` → `h`, `þ` → `th`.
2. Drop `.` and apostrophes (`'`, `’`, `‘`, `` ` ``, `´`): `o.a.` → `oa`, `it's` → `its`.
3. Every other character that is not `a-z0-9` separates words.
4. **Joined pairs**: every two adjacent words are also added as one word. `magazijn medewerker` and
   `magazijn-medewerker` both give the words `magazijn`, `medewerker` and `magazijnmedewerker`.

A text is then a *set of words*. Word order only matters through the joined pairs. `rules.Words` returns
this set.

### Grammar

```
rule     := term (' ' term)*          every positive term must match, no '-' term may match
term     := ['-'] alt
alt      := seq ('|' seq)*            any alternative
seq      := unit ('+' unit)*          compound: one word, or its parts as separate words
unit     := '(' alt ')' | literal
literal  := ['*'] letters ['*'] | ['*'] letters '~'
```

| syntax | matches a word that | example |
| --- | --- | --- |
| `x` | equals `x` | `kok` |
| `x*` | starts with `x` | `logistiek*`: logistiek, logistieke, logistiekmedewerker |
| `*x` | ends with `x` | `*monteur`: monteur, automonteur |
| `*x*` | contains `x` (allowed, rarely what you want) | `*logist*` |
| `x~` | is `x` or an inflection of it | `medewerker~`: medewerkers, medewerkster, … |
| `a\|b` | matches either | `verkoper~\|verkoopster` |
| `a+b` | is the joined form `ab`, **or** each part matches some word, in any order | `magazijn+medewerker`: magazijnmedewerker, medewerker magazijn |
| `-term` | (in a rule) must not match any word | `medewerker~ -hoofd*` |

- `~` stands for the word plus `s`, `e`, `en`, `es` and `n`; a word ending in `er` also takes `ster` and
  `sters` (medewerker → medewerkster), one ending in `eur` also `euse` (chauffeur → chauffeuse), one
  ending in a doubled vowel and a consonant also the single vowel plus `e` (commercieel → commerciele).
  `rules.Inflections` lists them. Irregular forms are written as alternatives.
- `+` binds tighter than `|`: `a|b+c` is `a` or `b+c`. Parts may be groups: `(magazijn|warehouse)+medewerker~`.
- Wildcards sit on the outer ends of a term: a leading `*` on the first part of a compound, a trailing
  `*` or `~` on the last part. They attach to words, never to groups.
- Terms are separated by spaces; there are no spaces inside `( )` or around `|` and `+`.
- An exclude is a single term without spaces and without a leading `-`.

### Validation

`rules.ParseRule` and `rules.ParseExclude` return a `*rules.SyntaxError` listing every problem, worded
for whoever wrote the rule:

- a word may only contain letters and digits after normalisation (`e-commerce` must be `e+commerce`
  or `ecommerce`);
- a word with `*` or `~` needs at least 4 letters (`*er` is rejected);
- a rule needs at least one term without `-`;
- one term may expand to at most 2000 joined forms.

`rules.Explain` prints what a rule covers, which helps when writing rules by hand or with a language
model.

## Using it

### One set, one text

```go
set, problems := rules.Compile(rules.Set{
	Includes: []string{"(magazijn|warehouse)+medewerker~", "*er"},
	Excludes: []string{"*manager"},
})
// problems[0]: includes[1] '*er': a word with * or ~ needs at least 4 letters, 'er' has 2

set.Match("Magazijn-medewerkers") // 0: the index in Includes of the first include that matches
set.Match("magazijnmanager")      // -1
```

`Compile` always returns a usable set: an invalid entry is left out, and every index keeps pointing
into the original lists, so stored attributions stay right.

### Many texts: `textindex`

Texts are stored under an ID of any comparable type, a few per ID if that fits (one ID per person, with
a text per position held). Texts with the same words are stored once.

```go
texts := textindex.New[int64]()
texts.Set(42, "Magazijnmedewerker", "Heftruckchauffeur")
texts.Append(42, "Orderpicker")
texts.Delete(7)

for _, hit := range texts.Match(set) {
	// hit.ID: 42, hit.Position: 0, hit.Text: "Magazijnmedewerker",
	// hit.Include: 0, hit.Rule: "(magazijn|warehouse)+medewerker~"
}
```

Hits are grouped by ID, in the order of the texts' positions. Writes take a write lock; a match holds a
read lock while it runs.

### Many rules: `ruleindex`

Sets are stored under an ID of any comparable type.

```go
sets := ruleindex.New[string]()
problems := sets.Set("set-1", rules.Set{Includes: []string{"chauffeur~", "heftruck*"}})
sets.SetAll(allSets) // iter.Seq2[ID, rules.Set], to fill the index in one go
sets.Delete("set-2")

for _, match := range sets.Match("Heftruckchauffeur") {
	// match.ID: "set-1", match.Include: 0, match.Rule: "chauffeur~"
}
```

Matches come in the order the sets were last written. Matching takes no lock and sees the sets as
they were when it started.

## Speed

Apple M5, synthetic data from `internal/synth`: made-up titles whose words follow a power law, and
sets of random rules that use the whole syntax. 0.7% of the sets match a text, and a set matches 0.7%
of the texts.

| | size | per call | index memory | build |
| --- | --- | --- | --- | --- |
| `textindex.Match`, a set of 20 includes and 3 excludes | 1M texts (635k distinct) | 0.6 ms | 275 MB | 1.7 s |
| `textindex.Set`, replace the 3 texts of an ID | 1M texts | 24 µs | | |
| `ruleindex.Match`, one text | 1M rules (20k sets) | 22 µs | 390 MB | 2.3 s |
| `ruleindex.Set`, replace a set of 50 includes | 1M rules | 0.7 ms | | |

```sh
go test -run '^$' -bench . -benchmem ./rules/textindex ./rules/ruleindex
```

## How the indexes work

The syntax is shaped so both directions can be indexed: every term compiles to word patterns of four
kinds (exact, prefix, suffix, infix), and a pattern never spans a word boundary, since the joined pairs
take care of compounds written apart.

**textindex** keeps a vocabulary of every word with the texts that have it. Exact patterns are a hash
lookup. Prefix and suffix patterns are a range in the words sorted forwards or backwards, and infix
patterns read the words that have the pattern's two rarest four-letter grams, which works because
wildcard words have at least four letters. A joined pair is left out of a prefix range when its left
word is at least as long as the prefix: the text then also has that word, which starts with the prefix
too. Each rule is evaluated with bitmaps over the distinct texts, and the excludes and first-match
attribution are bitmap operations too. Of the 0.6 ms above, about 0.2 ms is the matching and the rest
is collecting the 7k hits, so a set that matches less is faster. Words and postings are updated in
place on every write.

**ruleindex** puts every pattern of every rule in four hash maps. For each word of a text it probes the
whole word, its prefixes, its suffixes and its substrings, with a 4-letter bit filter skipping most
probes. The result is the set of patterns the text hits. Each rule is listed under the patterns of its
most selective term only, so only rules with a hit are evaluated, and evaluating one is bit tests on
pattern ids. Identical rules and subterms are stored once across all sets. Pass
`ruleindex.WithEstimator(texts.Estimate)` to rank the terms by how common they are in your own texts
instead of by a heuristic.

The rules live in immutable segments. A write builds a small segment for the sets it adds and marks
their old versions dead. Segments are merged as they pile up, and merges beyond 50k rules run in the
background, so neither writes nor matches wait for them. `Compact` merges everything into one segment.

## The fold table

The fold table lives in the [`sanitize`](../sanitize#the-fold-table) package.
