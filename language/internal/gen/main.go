// Command gen regenerates tables_gen.go from languages.tsv. Run it through
// "go generate ./language/..." so it gets the language package directory as
// its working directory.
package main

import (
	"fmt"
	"os"

	"github.com/Back-to-code/f2f-match-utils/language/internal/langtable"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", langtable.SourceFile, err)
		os.Exit(1)
	}
}

func run() error {
	src, err := os.ReadFile(langtable.SourceFile)
	if err != nil {
		return err
	}

	out, err := langtable.Generate(src)
	if err != nil {
		return err
	}

	//nolint:gosec // G703: the path is a constant, only the generated content comes from a file
	err = os.WriteFile(langtable.OutputFile, out, 0o644)
	if err != nil {
		return err
	}

	fmt.Printf("wrote %s (%d bytes)\n", langtable.OutputFile, len(out))

	return nil
}
