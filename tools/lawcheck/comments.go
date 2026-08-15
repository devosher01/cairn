package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

func commentViolations(fset *token.FileSet, file *ast.File) []string {
	var out []string
	for _, group := range file.Comments {
		if isOutputBlock(group) {
			continue
		}
		for _, c := range group.List {
			if isDirective(c.Text) {
				continue
			}
			pos := fset.Position(c.Pos())
			out = append(out, fmt.Sprintf("%s:%d: comment", pos.Filename, pos.Line))
		}
	}
	return out
}

func isDirective(text string) bool {
	return strings.HasPrefix(text, "//go:")
}

func isOutputBlock(group *ast.CommentGroup) bool {
	first := group.List[0].Text
	return strings.HasPrefix(first, "// Output:") ||
		strings.HasPrefix(first, "// Unordered output:")
}
