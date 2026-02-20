#!/usr/bin/env bash
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

echo "[check] validating adapter dependency contract"
mod_files=()
for mod in tools/artifactctl/go.mod; do
  if [[ -f "${mod}" ]]; then
    mod_files+=("${mod}")
  fi
done
if (( ${#mod_files[@]} > 0 )); then
  if search_files '^\s*(replace\s+)?github\.com/(poruru|poruru-code)/(edge-serverless-box|esb)/pkg/[[:alnum:]_.-]+(\s+v[^[:space:]]+)?\s+=>\s*' \
    "${mod_files[@]}"; then
    echo "[error] do not add pkg/* replace directives to adapter go.mod files" >&2
    exit 1
  fi

  echo "[check] validating adapter pkg/* v0.0.0 freeze"
  adapter_v0_allowlist_file="tools/ci/adapter_pkg_v0_allowlist.txt"
  if [[ ! -f "${adapter_v0_allowlist_file}" ]]; then
    echo "[error] missing allowlist file: ${adapter_v0_allowlist_file}" >&2
    exit 1
  fi

  actual_adapter_v0="$(
    awk '
      function emit(line) {
        gsub(/^[[:space:]]*require[[:space:]]+/, "", line)
        sub(/^[[:space:]]+/, "", line)
        split(line, parts, /[[:space:]]+/)
        if (parts[1] != "" && parts[2] == "v0.0.0") {
          print FILENAME " " parts[1]
        }
      }

      /^[[:space:]]*require[[:space:]]+github.com\/(poruru|poruru-code)\/(edge-serverless-box|esb)\/pkg\/[A-Za-z0-9_.-]+[[:space:]]+v0\.0\.0([[:space:]]|$)/ {
        emit($0)
        next
      }

      /^[[:space:]]*github.com\/(poruru|poruru-code)\/(edge-serverless-box|esb)\/pkg\/[A-Za-z0-9_.-]+[[:space:]]+v0\.0\.0([[:space:]]|$)/ {
        emit($0)
        next
      }
    ' "${mod_files[@]}" | sed '/^[[:space:]]*$/d' | sort -u
  )"
  expected_adapter_v0="$(sed '/^[[:space:]]*$/d' "${adapter_v0_allowlist_file}" | sort -u)"
  if ! diff -u \
    <(printf "%s\n" "${expected_adapter_v0}" | sed '/^[[:space:]]*$/d') \
    <(printf "%s\n" "${actual_adapter_v0}" | sed '/^[[:space:]]*$/d'); then
    echo "[error] adapter pkg/* v0.0.0 set changed; update allowlist with separation rationale" >&2
    exit 1
  fi
fi

echo "[check] validating runtime/tooling dependency direction"
if search_go_tree \
  '"github\.com/(poruru|poruru-code)/(edge-serverless-box|esb)/(tools/|pkg/artifactcore|pkg/composeprovision)' \
  services; then
  echo "[error] services must not import tools/* or pkg/artifactcore|pkg/composeprovision" >&2
  exit 1
fi

echo "[check] validating pure-core package restrictions"
if search_go_tree_non_test '"os/exec"|exec\.Command\(' \
  pkg/artifactcore pkg/yamlshape; then
  echo "[error] pkg/artifactcore and pkg/yamlshape must not execute external commands" >&2
  exit 1
fi

if search_go_tree_non_test 'CONTAINER_REGISTRY|HOST_REGISTRY_ADDR' \
  pkg/artifactcore pkg/yamlshape; then
  echo "[error] pkg/artifactcore and pkg/yamlshape must not depend on runtime registry env vars" >&2
  exit 1
fi

echo "[check] validating artifactcore public API surface"
allowlist_file="tools/ci/artifactcore_exports_allowlist.txt"
if [[ ! -f "${allowlist_file}" ]]; then
  echo "[error] missing allowlist file: ${allowlist_file}" >&2
  exit 1
fi
actual_exports="$(artifactcore_exports pkg/artifactcore)"
expected_exports="$(sort -u "${allowlist_file}")"
if ! diff -u <(printf "%s\n" "${expected_exports}") <(printf "%s\n" "${actual_exports}"); then
  echo "[error] artifactcore public API changed; update allowlist with design rationale" >&2
  exit 1
fi

echo "[check] validating artifactcore exported helper naming guard"
if printf "%s\n" "${actual_exports}" | grep -Eq '(^|\.)(Infer|Parse|Preferred|Has)[A-Z]'; then
  echo "[error] artifactcore must not export helper-style APIs (Infer/Parse/Preferred/Has...)." >&2
  echo "        Move helper logic to caller/internal packages and keep artifactcore API contract-oriented." >&2
  exit 1
fi

echo "[check] boundary checks passed"
