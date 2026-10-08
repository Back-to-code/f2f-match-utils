package ruleindex_test

import (
	"fmt"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/ruleindex"
)

func ExampleIndex_Match() {
	sets := ruleindex.New[string]()
	sets.Set("drivers", rules.Set{Includes: []string{"chauffeur~", "heftruck*"}})
	sets.Set("warehouse", rules.Set{Includes: []string{"magazijn+medewerker~", "*truck+chauffeur~"}, Excludes: []string{"*manager"}})
	problems := sets.Set("broken", rules.Set{Includes: []string{"*er"}})
	fmt.Println(problems[0])

	for _, match := range sets.Match("Heftruckchauffeur") {
		fmt.Printf("%s: include %d %q\n", match.ID, match.Include, match.Rule)
	}
	// Output:
	// includes[0] '*er': a word with * or ~ needs at least 4 letters, 'er' has 2
	// drivers: include 1 "heftruck*"
	// warehouse: include 1 "*truck+chauffeur~"
}
