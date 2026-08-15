package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var violations []string
	for _, root := range roots {
		found, err := check(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		violations = append(violations, found...)
	}
	for _, v := range violations {
		fmt.Println(v)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

func check(root string) ([]string, error) {
	var out []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(path)
		out = append(out, commentViolations(fset, file)...)
		out = append(out, importViolations(fset, file, rel)...)
		out = append(out, clockViolations(fset, file, rel)...)
		return nil
	})
	return out, err
}

func skipDir(name string) bool {
	return name == ".git" || name == "testdata"
}
