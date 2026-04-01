package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// defaultDumpPath is the repo-relative path to the generated schema dump.
const defaultDumpPath = "coderd/database/dump.sql"

var dumpPathFlag = flag.String("dump", defaultDumpPath, "path to dump.sql (defaults to coderd/database/dump.sql)")

func main() {
	flag.Parse()

	want := expectedFromRBAC()
	have, err := enumValuesFromDump(*dumpPathFlag)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "check-scopes: error reading dump: %v\n", err)
		os.Exit(2)
	}

	// Compute missing: want - have
	var missing []string
	for k := range want {
		if _, ok := have[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)

	if len(missing) == 0 {
		_, _ = fmt.Println("check-scopes: OK — all RBAC <resource>:<action> values exist in api_key_scope enum")
		return
	}

	_, _ = fmt.Fprintln(os.Stderr, "check-scopes: missing enum values:")
	for _, m := range missing {
		_, _ = fmt.Fprintf(os.Stderr, "  - %s\n", m)
	}
	_, _ = fmt.Fprintln(os.Stderr)
	_, _ = fmt.Fprintln(os.Stderr, "To fix: add a DB migration extending the enum, e.g.:")
	for _, m := range missing {
		_, _ = fmt.Fprintf(os.Stderr, "  ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS '%s';\n", m)
	}
	_, _ = fmt.Fprintln(os.Stderr)
	_, _ = fmt.Fprintln(os.Stderr, "Also decide if each new scope is external (exposed in the `externalLowLevel` in coderd/rbac/scopes_catalog.go) or internal-only.")
	os.Exit(1)
}

// expectedFromRBAC returns the set of scope names the DB enum must support.
func expectedFromRBAC() map[string]struct{} {
	want := make(map[string]struct{})
	add := func(name string) {
		if name == "" {
			return
		}
		want[name] = struct{}{}
	}
	// Low-level <resource>:<action> and synthesized <resource>:* wildcards
	for resource, def := range policy.RBACPermissions {
		if resource == policy.WildcardSymbol {
			// Ignore wildcard entry; it has no concrete <resource>:<action> pairs.
			continue
		}
		add(resource + ":" + policy.WildcardSymbol)
		for action := range def.Actions {
			add(resource + ":" + string(action))
		}
	}
	// Composite coder:* names
	for _, n := range rbac.CompositeScopeNames() {
		add(n)
	}
	// Built-in coder-prefixed scopes such as coder:all
	for _, n := range rbac.BuiltinScopeNames() {
		s := string(n)
		if !strings.Contains(s, ":") {
			continue
		}
		add(s)
	}
	return want
}

// enumValuesFromDump parses dump.sql and extracts all literals from the
// `CREATE TYPE api_key_scope AS ENUM (...)` block.
func enumValuesFromDump(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const enumHead = "CREATE TYPE api_key_scope AS ENUM ("
	litRe := regexp.MustCompile(`'([^']+)'`)

	values := make(map[string]struct{})
	inEnum := false
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !inEnum {
			if strings.Contains(line, enumHead) {
				inEnum = true
			}
			continue
		}
		if strings.HasPrefix(line, ");") {
			// End of enum block
			return values, nil
		}
		// Collect single-quoted literals on this line.
		for _, m := range litRe.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 {
				values[m[1]] = struct{}{}
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if !inEnum {
		return nil, xerrors.New("api_key_scope enum block not found in dump")
	}
	return values, nil
}
