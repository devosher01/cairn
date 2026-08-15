package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

type importRule struct {
	path         string
	allowed      []string
	allowInTests bool
}

var _importRules = []importRule{
	{path: "os", allowed: []string{"internal/env/osenv/", "tools/"}, allowInTests: true},
	{path: "math/rand", allowed: []string{"internal/env/osenv/"}, allowInTests: true},
	{path: "math/rand/v2", allowed: []string{"internal/env/osenv/", "internal/env/simenv/"}, allowInTests: true},
	{path: "syscall", allowed: []string{"internal/env/osenv/"}},
}

func importViolations(fset *token.FileSet, file *ast.File, rel string) []string {
	var out []string
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		rule, ok := ruleFor(path)
		if !ok || allowedAt(rule, rel) {
			continue
		}
		if rule.allowInTests && strings.HasSuffix(rel, "_test.go") {
			continue
		}
		pos := fset.Position(imp.Pos())
		out = append(out, fmt.Sprintf("%s:%d: forbidden import %q", pos.Filename, pos.Line, path))
	}
	return out
}

func ruleFor(path string) (importRule, bool) {
	for _, r := range _importRules {
		if r.path == path {
			return r, true
		}
	}
	return importRule{}, false
}

func allowedAt(r importRule, rel string) bool {
	for _, prefix := range r.allowed {
		if strings.Contains(rel, prefix) {
			return true
		}
	}
	return false
}
