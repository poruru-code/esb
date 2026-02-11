// Where: cli/internal/architecture/dependency_contracts_test.go
// What: Contract checks for anti-pattern dependency usage across internal layers.
// Why: Prevent regressions where command/usecase instantiate concrete infra dependencies directly.
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type dependencyContract struct {
	forbiddenImports      map[string]struct{}
	forbiddenCalls        map[string]map[string]struct{}
	forbiddenTypeLiterals map[string]map[string]struct{}
}

var dependencyContracts = map[string]dependencyContract{
	"command": {
		forbiddenImports: map[string]struct{}{
			internalImportPrefix + "infra/env": {},
		},
		forbiddenCalls: map[string]map[string]struct{}{
			internalImportPrefix + "infra/ui": {
				"NewDeployUI": {},
			},
			internalImportPrefix + "infra/compose": {
				"NewDockerClient": {},
			},
		},
		forbiddenTypeLiterals: map[string]map[string]struct{}{
			internalImportPrefix + "infra/compose": {
				"ExecRunner": {},
			},
		},
	},
	"usecase/deploy": {
		forbiddenImports: map[string]struct{}{
			"github.com/docker/docker/client": {},
		},
		forbiddenCalls: map[string]map[string]struct{}{
			internalImportPrefix + "infra/deploy": {
				"NewComposeProvisioner": {},
			},
			internalImportPrefix + "infra/compose": {
				"NewDockerClient": {},
			},
		},
	},
	"infra/runtime": {
		forbiddenImports: map[string]struct{}{
			"github.com/docker/docker/client": {},
		},
	},
	"infra/deploy": {
		forbiddenCalls: map[string]map[string]struct{}{
			internalImportPrefix + "infra/compose": {
				"NewDockerClient": {},
			},
		},
	},
	"infra/sam": {
		forbiddenCalls: map[string]map[string]struct{}{
			"fmt": {
				"Print":   {},
				"Printf":  {},
				"Println": {},
			},
		},
	},
	"infra/templategen": {
		forbiddenCalls: map[string]map[string]struct{}{
			"fmt": {
				"Print":   {},
				"Printf":  {},
				"Println": {},
			},
		},
	},
}

func TestDependencyContracts(t *testing.T) {
	t.Parallel()

	internalRoot := resolveInternalRoot(t)
	fset := token.NewFileSet()
	violations := []string{}

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
		sourcePkg := filepath.ToSlash(filepath.Dir(rel))
		contract, ok := dependencyContractForPackage(sourcePkg)
		if !ok {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		importAliases := resolveImportAliases(file)
		fileViolations := detectDependencyContractViolations(fset, rel, file, importAliases, contract)
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal packages: %v", err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("dependency contract violations:\n%s", strings.Join(violations, "\n"))
	}
}

func dependencyContractForPackage(sourcePkg string) (dependencyContract, bool) {
	pkg := strings.TrimSpace(sourcePkg)
	if pkg == "" {
		return dependencyContract{}, false
	}
	for prefix, contract := range dependencyContracts {
		if pkg == prefix || strings.HasPrefix(pkg, prefix+"/") {
			return contract, true
		}
	}
	return dependencyContract{}, false
}

func resolveImportAliases(file *ast.File) map[string]string {
	aliases := map[string]string{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		if importPath == "" {
			continue
		}
		alias := pathpkg.Base(importPath)
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				continue
			}
			alias = imp.Name.Name
		}
		aliases[alias] = importPath
	}
	return aliases
}

func detectDependencyContractViolations(
	fset *token.FileSet,
	relPath string,
	file *ast.File,
	importAliases map[string]string,
	contract dependencyContract,
) []string {
	violations := []string{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		if _, ok := contract.forbiddenImports[importPath]; !ok {
			continue
		}
		line := fset.Position(imp.Pos()).Line
		violations = append(violations, relPath+":"+strconv.Itoa(line)+" -> import "+importPath)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			importPath, symbol, ok := resolveSelector(n.Fun, importAliases)
			if !ok {
				return true
			}
			if isForbiddenSymbol(contract.forbiddenCalls, importPath, symbol) {
				line := fset.Position(n.Pos()).Line
				violations = append(
					violations,
					relPath+":"+strconv.Itoa(line)+" -> call "+importPath+"."+symbol,
				)
			}
		case *ast.CompositeLit:
			importPath, symbol, ok := resolveSelector(n.Type, importAliases)
			if !ok {
				return true
			}
			if isForbiddenSymbol(contract.forbiddenTypeLiterals, importPath, symbol) {
				line := fset.Position(n.Pos()).Line
				violations = append(
					violations,
					relPath+":"+strconv.Itoa(line)+" -> literal "+importPath+"."+symbol,
				)
			}
		}
		return true
	})
	return violations
}

func resolveSelector(expr ast.Expr, importAliases map[string]string) (importPath string, symbol string, ok bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	importPath, ok = importAliases[ident.Name]
	if !ok {
		return "", "", false
	}
	return importPath, selector.Sel.Name, true
}

func isForbiddenSymbol(forbidden map[string]map[string]struct{}, importPath, symbol string) bool {
	symbols, ok := forbidden[importPath]
	if !ok {
		return false
	}
	_, found := symbols[symbol]
	return found
}
