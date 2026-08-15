package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

var _clockFuncs = map[string]struct{}{
	"Now":       {},
	"Since":     {},
	"Until":     {},
	"After":     {},
	"AfterFunc": {},
	"Sleep":     {},
	"NewTicker": {},
	"NewTimer":  {},
	"Tick":      {},
}

func clockViolations(fset *token.FileSet, file *ast.File, rel string) []string {
	if strings.Contains(rel, "internal/env/osenv/") || strings.Contains(rel, "tools/") {
		return nil
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "time" {
			return true
		}
		if _, banned := _clockFuncs[sel.Sel.Name]; !banned {
			return true
		}
		pos := fset.Position(sel.Pos())
		out = append(out, fmt.Sprintf("%s:%d: real clock access time.%s", pos.Filename, pos.Line, sel.Sel.Name))
		return true
	})
	return out
}
