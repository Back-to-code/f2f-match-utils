package rules

import (
	"fmt"
	"slices"
	"strings"
)

func describeForm(form Form) string {
	if form.Kind() == Exact {
		return form.String()
	}
	return fmt.Sprintf("%s (%s)", form, form.Kind())
}

func describePart(node *Node, top bool) string {
	switch node.Type {
	case LiteralNode:
		return describeForm(node.Form)
	case GroupNode:
		parts := make([]string, len(node.Children))
		for idx, option := range node.Children {
			parts[idx] = describePart(option, false)
		}
		return "(" + strings.Join(parts, " | ") + ")"
	}
	return nodeSource(node, top)
}

func tildeLiterals(node *Node, found []Form) []Form {
	if node.Type == LiteralNode {
		if node.Form.Tilde && !slices.Contains(found, node.Form) {
			found = append(found, node.Form)
		}
		return found
	}
	for _, child := range node.Children {
		found = tildeLiterals(child, found)
	}
	return found
}

const maxListedForms = 24

// Explain describes in plain text what a rule covers: per term its alternatives, the joined and apart
// forms of compounds and the words every ~ stands for. For an invalid rule it lists the problems.
func Explain(rule string) string {
	parsed, err := ParseRule(rule)
	if err != nil {
		lines := []string{"invalid rule: " + rule}
		for _, message := range Messages(err) {
			lines = append(lines, "  "+message)
		}
		return strings.Join(lines, "\n")
	}
	sources := make([]string, len(parsed.Terms))
	for idx, term := range parsed.Terms {
		sources[idx] = term.Source
	}
	lines := []string{"rule: " + strings.Join(sources, " ")}
	var tildeForms []Form
	joinedTilde := false
	for number, term := range parsed.Terms {
		options := []*Node{term.Node}
		if term.Node.Type == GroupNode {
			options = term.Node.Children
		}
		header := fmt.Sprintf("term %d must match", number+1)
		if term.Negative {
			header = fmt.Sprintf("term %d must NOT match", number+1)
		}
		if len(options) > 1 {
			lines = append(lines, fmt.Sprintf("%s, any of %d alternatives: %s", header, len(options), term.Body()))
		}
		for idx, option := range options {
			bullet := ""
			switch {
			case len(options) > 1:
				bullet = fmt.Sprintf("  %d. ", idx+1)
				if option.Type == LiteralNode {
					lines = append(lines, bullet+describeForm(option.Form))
				} else {
					lines = append(lines, bullet+nodeSource(option, false))
				}
			case option.Type == LiteralNode:
				lines = append(lines, fmt.Sprintf("%s: %s", header, describeForm(option.Form)))
			default:
				bullet = "  "
				lines = append(lines, fmt.Sprintf("%s: %s", header, nodeSource(option, false)))
			}
			if option.Type == LiteralNode {
				continue
			}
			indent := strings.Repeat(" ", len(bullet))
			var listed []string
			for _, form := range option.Forms[:min(len(option.Forms), maxListedForms)] {
				listed = append(listed, describeForm(form))
			}
			if len(option.Forms) > maxListedForms {
				listed = append(listed, fmt.Sprintf("... %d more", len(option.Forms)-maxListedForms))
			}
			for _, form := range option.Forms {
				joinedTilde = joinedTilde || form.Tilde
			}
			parts := make([]string, len(option.Children))
			for partIdx, part := range option.Children {
				parts[partIdx] = describePart(part, true)
			}
			lines = append(lines, indent+"one word: "+strings.Join(listed, " | "))
			lines = append(lines, indent+"or apart, each part some word, any order: "+strings.Join(parts, " & "))
		}
		tildeForms = tildeLiterals(term.Node, tildeForms)
	}
	if len(tildeForms) > 0 {
		note := ""
		if joinedTilde {
			note = " (a joined form ending in a ~ word takes the same endings)"
		}
		lines = append(lines, "~ forms"+note+":")
		for _, form := range tildeForms {
			ends := ""
			if form.Lead {
				ends = "ends with "
			}
			lines = append(lines, fmt.Sprintf("  %s = %s%s", form, ends, strings.Join(Inflections(form.Text), ", ")))
		}
	}
	return strings.Join(lines, "\n")
}
