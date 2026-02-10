// Where: cli/internal/architecture/layering_cycles_test.go
// What: Import cycle guard for CLI internal packages.
// Why: Detect cyclic coupling early and keep package boundaries maintainable.
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestNoInternalImportCycles(t *testing.T) {
	t.Parallel()

	internalRoot := resolveInternalRoot(t)
	fset := token.NewFileSet()
	graph := map[string]map[string]struct{}{}

	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		relDir := filepath.ToSlash(filepath.Dir(rel))
		if relDir == "." {
			return nil
		}
		sourcePkg := internalImportPrefix + relDir
		if _, ok := graph[sourcePkg]; !ok {
			graph[sourcePkg] = map[string]struct{}{}
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			if !strings.HasPrefix(importPath, internalImportPrefix) {
				continue
			}
			graph[sourcePkg][importPath] = struct{}{}
			if _, ok := graph[importPath]; !ok {
				graph[importPath] = map[string]struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal packages: %v", err)
	}

	cycles := detectCycles(graph)
	if len(cycles) > 0 {
		sort.Strings(cycles)
		t.Fatalf("internal import cycles detected:\n%s", strings.Join(cycles, "\n"))
	}
}

func detectCycles(graph map[string]map[string]struct{}) []string {
	const (
		stateUnvisited = 0
		stateVisiting  = 1
		stateDone      = 2
	)

	state := map[string]int{}
	stack := []string{}
	seenCycles := map[string]struct{}{}
	cycles := []string{}

	var walk func(string)
	walk = func(node string) {
		state[node] = stateVisiting
		stack = append(stack, node)

		neighbors := make([]string, 0, len(graph[node]))
		for next := range graph[node] {
			neighbors = append(neighbors, next)
		}
		sort.Strings(neighbors)

		for _, next := range neighbors {
			switch state[next] {
			case stateUnvisited:
				walk(next)
			case stateVisiting:
				start := -1
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == next {
						start = i
						break
					}
				}
				if start >= 0 {
					path := append(append([]string{}, stack[start:]...), next)
					cycle := strings.Join(path, " -> ")
					if _, ok := seenCycles[cycle]; !ok {
						seenCycles[cycle] = struct{}{}
						cycles = append(cycles, cycle)
					}
				}
			}
		}

		stack = stack[:len(stack)-1]
		state[node] = stateDone
	}

	nodes := make([]string, 0, len(graph))
	for node := range graph {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if state[node] == stateUnvisited {
			walk(node)
		}
	}

	return cycles
}
