// Command chart-flags prints the agent flags a Helm chart template can pass, sorted, one per line. It parses a define
// and reads the flags from the literal text of every branch, without evaluating the template, so the result doesn't
// depend on the chart values.
//
// Usage: chart-flags <template-file> <define>
package main

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"text/template/parse"
)

var flagRE = regexp.MustCompile(`--([a-z0-9][a-z0-9-]*)`)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: chart-flags <template-file> <define>")
		os.Exit(2)
	}

	flags, err := chartFlags(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "chart-flags: %s\n", err)
		os.Exit(1)
	}

	for _, f := range flags {
		fmt.Println(f)
	}
}

func chartFlags(filename, define string) ([]string, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// Helm and Sprig functions, such as include, aren't defined here.
	tree := parse.New(filename)
	tree.Mode = parse.SkipFuncCheck
	trees := make(map[string]*parse.Tree)

	if _, err := tree.Parse(string(src), "", "", trees); err != nil {
		return nil, err
	}

	t, ok := trees[define]
	if !ok {
		return nil, fmt.Errorf("%s: define %q not found", filename, define)
	}

	flags := make(map[string]struct{})
	if err := collect(t.Root, flags); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}

	return slices.Sorted(maps.Keys(flags)), nil
}

// collect adds the flags in the literal text under node to flags, in every branch.
func collect(node parse.Node, flags map[string]struct{}) error {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return nil
		}

		for _, c := range n.Nodes {
			if err := collect(c, flags); err != nil {
				return err
			}
		}

	case *parse.TextNode:
		for _, m := range flagRE.FindAllSubmatch(n.Text, -1) {
			flags[string(m[1])] = struct{}{}
		}

	case *parse.IfNode:
		return collectBranch(&n.BranchNode, flags)

	case *parse.WithNode:
		return collectBranch(&n.BranchNode, flags)

	case *parse.RangeNode:
		return collectBranch(&n.BranchNode, flags)

	case *parse.ActionNode:
		// A flag built in an action, such as with printf, can't be read without evaluating the template.
		if strings.Contains(n.String(), "--") {
			return fmt.Errorf("%s builds a flag in an action: write flags as literal text", n)
		}
	}

	return nil
}

func collectBranch(b *parse.BranchNode, flags map[string]struct{}) error {
	if err := collect(b.List, flags); err != nil {
		return err
	}

	return collect(b.ElseList, flags)
}
