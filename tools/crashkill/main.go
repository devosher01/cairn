package main

import (
	"flag"
	"fmt"
	"os"
)

const _defaultIterations = 10

func main() {
	child := flag.Bool("child", false, "run as the writer killed by the parent")
	dir := flag.String("dir", "", "database directory for a child, campaign root for the parent")
	iterations := flag.Int("iterations", _defaultIterations, "number of kill iterations")
	flag.Parse()

	if err := run(*child, *dir, *iterations); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(child bool, dir string, iterations int) error {
	if child {
		return runChild(dir)
	}

	return runParent(dir, iterations)
}
