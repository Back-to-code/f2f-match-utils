package rules_test

import (
	"slices"
	"testing"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
)

var pruneTexts = []string{
	"bouwtechniek", "techniek bouw", "medewerker techniek", "installatie techniek", "installatietechniek",
	"techniekinstallatie", "techniekbouw", "techniek bouwkundig", "bouwkundig tekenaar", "tekenaars", "architect",
	"architecten", "elektromonteurs", "elektro monteur", "automonteur", "monteur", "magazijnmedewerkster",
	"magazijn medewerker", "kok", "chef", "bouwmarkt medewerker", "",
}

func TestPruneIncludes(t *testing.T) {
	tests := []struct {
		name     string
		includes []string
		want     []string
		changed  int
	}{
		{
			name: "an infix covers every include whose words all contain it",
			includes: []string{
				"*bouw*", "bouw+techniek", "bouwkundig~", "(bouw|installatie)+techniek", "bouwkundig|architect~",
				"techniek+(bouw|installatie)", "*monteur -auto*",
			},
			want: []string{
				"*bouw*", "", "", "installatie+techniek", "architect~", "techniek+installatie", "*monteur -auto*",
			},
			changed: 5,
		},
		{
			name:     "a prefix does not cover a joined form that starts with another part",
			includes: []string{"bouw*", "techniek+(bouw|installatie)", "(bouw|installatie)+techniek"},
			want:     []string{"bouw*", "techniek+(bouw|installatie)", "installatie+techniek"},
			changed:  1,
		},
		{
			name:     "a suffix does not cover inflections that end differently",
			includes: []string{"*monteur", "elektro+monteur~", "elektro+monteur"},
			want:     []string{"*monteur", "elektro+monteur~", ""},
			changed:  1,
		},
		{
			name:     "of duplicates the first stays and a compound covers its joined form",
			includes: []string{"magazijn+medewerker~", "magazijnmedewerker", "magazijn+medewerker~", "magazijn+medewerker~|kok"},
			want:     []string{"magazijn+medewerker~", "", "", "kok"},
			changed:  3,
		},
		{
			name:     "of includes covering each other only one loses the shared alternative",
			includes: []string{"kok|kokkin", "kok|chef"},
			want:     []string{"kok|kokkin", "chef"},
			changed:  1,
		},
		{
			name:     "an include of several terms covers nothing but can be covered",
			includes: []string{"bouw* -bouwmarkt*", "bouwkundig", "tekenaar~", "tekenaar~ bouw*"},
			want:     []string{"bouw* -bouwmarkt*", "bouwkundig", "tekenaar~", ""},
			changed:  1,
		},
		{
			name:     "blank and invalid includes are passed through",
			includes: []string{"", "*er", "*bouw*", "bouw"},
			want:     []string{"", "*er", "*bouw*", ""},
			changed:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := rules.Set{Includes: test.includes, Excludes: []string{"*manager"}}
			pruned, changed := set.PruneIncludes()

			if !slices.Equal(pruned.Includes, test.want) || changed != test.changed {
				t.Fatalf("got %q with %d changed, want %q with %d", pruned.Includes, changed, test.want, test.changed)
			}
			if !slices.Equal(pruned.Excludes, set.Excludes) {
				t.Errorf("excludes: got %q", pruned.Excludes)
			}
			assertSameMatches(t, set, pruned, pruneTexts)
		})
	}
}

func TestPruneIncludesKeepsWhatSyntheticSetsMatch(t *testing.T) {
	gen := synth.New(17, 0)
	texts := make([]string, 1_000)
	for idx := range texts {
		texts[idx] = gen.Text()
	}

	changed := 0
	for range 50 {
		set := gen.Set(40, 2)
		pruned, setChanged := set.PruneIncludes()
		changed += setChanged

		for idx, include := range pruned.Includes {
			if include == "" || include == set.Includes[idx] {
				continue
			}
			rule, err := rules.ParseRule(include)
			if err != nil || rule.NormalizedSource() != include {
				t.Fatalf("%q was rewritten to %q, which does not parse to itself: %v", set.Includes[idx], include, err)
			}
		}
		assertSameMatches(t, set, pruned, texts)
	}
	if changed == 0 {
		t.Fatal("no set had an include to prune, so nothing was checked")
	}
}

func assertSameMatches(t *testing.T, before, after rules.Set, texts []string) {
	t.Helper()
	compiledBefore, _ := rules.Compile(before)
	compiledAfter, _ := rules.Compile(after)
	for _, text := range texts {
		words := rules.Words(text)
		if (compiledBefore.MatchWords(words) >= 0) != (compiledAfter.MatchWords(words) >= 0) {
			t.Fatalf("%q matches differently after pruning %q to %q", text, before.Includes, after.Includes)
		}
	}
}

func BenchmarkPruneIncludes(b *testing.B) {
	set := synth.New(19, 0).Set(150, 0)
	_, changed := set.PruneIncludes()
	b.Logf("%d of %d includes pruned", changed, len(set.Includes))
	for b.Loop() {
		set.PruneIncludes()
	}
}
