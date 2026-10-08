package rules_test

import (
	"fmt"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

func ExampleCompile() {
	set, problems := rules.Compile(rules.Set{
		Includes: []string{"(magazijn|warehouse)+medewerker~", "*er"},
		Excludes: []string{"*manager"},
	})
	for _, problem := range problems {
		fmt.Println(problem)
	}
	fmt.Println(set.Match("Magazijn-medewerkers"))
	fmt.Println(set.Match("magazijnmanager"))
	// Output:
	// includes[1] '*er': a word with * or ~ needs at least 4 letters, 'er' has 2
	// 0
	// -1
}

func ExampleWords() {
	fmt.Println(rules.Words("Magazijn-medewerker (o.a. heftruck)"))
	// Output:
	// [magazijn medewerker oa heftruck magazijnmedewerker medewerkeroa oaheftruck]
}

func ExampleExplain() {
	fmt.Println(rules.Explain("magazijn+medewerker~ -hoofd*"))
	// Output:
	// rule: magazijn+medewerker~ -hoofd*
	// term 1 must match: magazijn+medewerker~
	//   one word: magazijnmedewerker~
	//   or apart, each part some word, any order: magazijn & medewerker~
	// term 2 must NOT match: hoofd* (prefix)
	// ~ forms (a joined form ending in a ~ word takes the same endings):
	//   medewerker~ = medewerker, medewerkers, medewerkere, medewerkeren, medewerkeres, medewerkern, medewerkster, medewerksters
}
