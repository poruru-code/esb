#!/usr/bin/env bash
# Where: .github/checks/check_tooling_boundaries.sh
# What: Guard import/module/public-API boundaries for shared packages.
# Why: Keep architecture contracts enforceable in CI.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

has_rg() {
  command -v rg >/dev/null 2>&1
}

search_files() {
  local pattern="$1"
  shift
  if has_rg; then
    rg -n --glob 'go.mod' "${pattern}" "$@"
  else
    grep -nE "${pattern}" "$@"
  fi
}

search_go_tree() {
  local pattern="$1"
  shift
  if has_rg; then
    rg -n --glob '**/*.go' "${pattern}" "$@"
  else
    grep -RInE --include='*.go' "${pattern}" "$@"
  fi
}

search_go_tree_non_test() {
  local pattern="$1"
  shift
  if has_rg; then
    rg -n --glob '**/*.go' --glob '!**/*_test.go' "${pattern}" "$@"
  else
    find "$@" -type f -name '*.go' ! -name '*_test.go' -print0 | xargs -0 -r grep -nE "${pattern}"
  fi
}

artifactcore_exports() {
  local dir="$1"
  local tmp_go
  tmp_go="$(mktemp "${TMPDIR:-/tmp}/artifactcore-exports-XXXXXX.go")"
  cat >"${tmp_go}" <<'EOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func receiverName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		if id, ok := v.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: export-list <dir>")
		os.Exit(2)
	}

	root := os.Args[1]
	fset := token.NewFileSet()
	exports := map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range node.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							exports[s.Name.Name] = struct{}{}
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if ast.IsExported(name.Name) {
								exports[name.Name] = struct{}{}
							}
						}
					}
				}
			case *ast.FuncDecl:
				if node.Name == nil || !ast.IsExported(node.Name.Name) {
					continue
				}
				if node.Recv == nil || len(node.Recv.List) == 0 {
					exports[node.Name.Name] = struct{}{}
					continue
				}
				recv := receiverName(node.Recv.List[0].Type)
				if recv == "" {
					exports[node.Name.Name] = struct{}{}
					continue
				}
				exports[recv+"."+node.Name.Name] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	names := make([]string, 0, len(exports))
	for name := range exports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Println(name)
	}
}
EOF

  set +e
  local out
  out="$(go run "${tmp_go}" "${dir}")"
  local status=$?
  set -e
  rm -f "${tmp_go}"
  if [[ ${status} -ne 0 ]]; then
    return "${status}"
  fi
  printf "%s\n" "${out}"
}

echo "[check] validating runtime/tooling dependency direction"
if search_go_tree \
  '"github\.com/(poruru|poruru-code)/(edge-serverless-box|esb)/tools/' \
  services; then
  echo "[error] services must not import tools/* modules" >&2
  exit 1
fi

echo "[check] boundary checks passed"
