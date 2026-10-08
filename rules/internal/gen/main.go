// Command gen regenerates fold_table.go. Run it through "go generate ./rules/..." so it gets the rules
// package directory as its working directory.
package main

import (
	"fmt"
	"os"

	"github.com/Back-to-code/f2f-match-utils/rules/internal/foldtable"
)

func main() {
	out, err := foldtable.Generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//nolint:gosec // G703: the path is a constant, only the generated content comes from a file
	err = os.WriteFile(foldtable.OutputFile, out, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s (%d bytes)\n", foldtable.OutputFile, len(out))
}
