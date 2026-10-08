package textindex_test

import (
	"fmt"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/textindex"
)

func ExampleIndex_Match() {
	texts := textindex.New[int64]()
	texts.Set(42, "Magazijnmedewerker", "Heftruckchauffeur")
	texts.Append(42, "Orderpicker")
	texts.Set(7, "Magazijnmanager")

	set := rules.MustCompile(rules.Set{
		Includes: []string{"(magazijn|warehouse)+medewerker~", "orderpicker~"},
		Excludes: []string{"*manager"},
	})
	for _, hit := range texts.Match(set) {
		fmt.Printf("%d: text %d %q, include %d %q\n", hit.ID, hit.Position, hit.Text, hit.Include, hit.Rule)
	}
	// Output:
	// 42: text 0 "Magazijnmedewerker", include 0 "(magazijn|warehouse)+medewerker~"
	// 42: text 2 "Orderpicker", include 1 "orderpicker~"
}
