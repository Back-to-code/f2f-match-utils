package sanitize

import "strings"

var markdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	`*`, `\*`,
	`_`, `\_`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`>`, `\>`,
	`#`, `\#`,
	`|`, `\|`,
	`!`, `\!`,
	`<`, `\<`,
	"`", "\\`",
)

// EscapeMarkdown escapes markdown special characters in a string
// so it renders as literal text when processed by a markdown parser.
func EscapeMarkdown(s string) string {
	return markdownEscaper.Replace(s)
}
