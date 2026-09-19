// Command scopesdocgen generates the API key scopes reference at
// docs/reference/api-key-scopes.md from the RBAC scope catalog. It lists the
// scopes a token may request: the built-in scopes, the composite coder:*
// scopes with the permissions each one expands to, and the low-level
// resource:action scopes. Because the source is rbac.ExternalScopeNames and
// the policy definitions behind it, the page stays in sync as scopes change.
package main

import (
	"flag"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/flog"
)

// routeTitles is the manifest breadcrumb to this page's route. The page
// mirrors that route's metadata into its front matter, so the manifest stays
// the single source of the title and description.
var routeTitles = []string{"Reference", "API key scopes"}

var exampleScopes = []rbac.ScopeName{
	"coder:workspaces.access",
	"template:read",
}

const intro = `A scope limits what an API key can do.
A scoped key never exceeds the permissions of the user who created it: Coder checks the scope and the user's roles on every request, so a scope narrows access and never widens it.

Pass ` + "`--scope`" + ` once per scope when you create a token:

%s

A token created without an explicit scope uses ` + "`coder:all`" + `, which grants the full permissions of its owner.
To create and revoke tokens, refer to [Sessions & API Tokens](../admin/users/sessions-tokens.md).

This page lists every canonical scope a token can request and the deprecated names Coder accepts for backward compatibility.
Coder rejects any scope name not listed on this page with a ` + "`400`" + ` response.

`

const builtinSection = `## Built-in scopes

Built-in scopes cover the two broadest cases: the full permissions of the owner, and connections to workspace applications.

`

const compositeSection = `## Composite scopes

A composite scope groups the permissions that one task needs, so you can scope a token to that task without listing each permission.
Each scope below grants the permissions in its table.

`

const lowLevelSection = `## Low-level scopes

A low-level scope grants one action on one resource, written as ` + "`resource:action`" + `.
Combine low-level scopes when no composite scope matches the task.

The ` + "`resource:*`" + ` form grants every action on that resource, including actions not listed on this page.

`

const deprecatedSection = `## Deprecated scope names

Coder still accepts the following names and stores each one as its canonical equivalent.
Use the canonical name.

`

func main() {
	manifestPath := flag.String("manifest", "docs/manifest.json", "path to the docs manifest that supplies the page metadata")
	out := flag.String("out", "docs/reference/api-key-scopes.md", "path to write the generated reference page")
	flag.Parse()

	manifest, err := docgenenv.LoadManifest(*manifestPath)
	if err != nil {
		flog.Fatalf("%v", err)
	}
	route := manifest.FindRoute(routeTitles...)
	if route == nil {
		flog.Fatalf("manifest %q has no route %q", *manifestPath, strings.Join(routeTitles, " > "))
	}

	content, err := render(*route)
	if err != nil {
		flog.Fatalf("render scopes reference: %v", err)
	}
	if err := atomicwrite.File(*out, []byte(content)); err != nil {
		flog.Fatalf("write %s: %v", *out, err)
	}
	flog.Successf("wrote %s", *out)
}

func render(route docgenenv.Route) (string, error) {
	builtin, composite, lowLevel := partitionScopes(rbac.ExternalScopeNames())

	var b strings.Builder
	// The front matter title renders as the page heading, so the body starts
	// at the intro and its sections begin at level two.
	_, _ = b.WriteString(docgenenv.GeneratedHeader(route))
	_, _ = fmt.Fprintf(&b, intro, exampleCommand())

	_, _ = b.WriteString(builtinSection)
	if err := renderBuiltin(&b, builtin); err != nil {
		return "", err
	}

	_, _ = b.WriteString(compositeSection)
	if err := renderComposite(&b, composite); err != nil {
		return "", err
	}

	_, _ = b.WriteString(lowLevelSection)
	if err := renderLowLevel(&b, lowLevel); err != nil {
		return "", err
	}

	_, _ = b.WriteString(deprecatedSection)
	if err := renderDeprecatedAliases(&b, rbac.ScopeAliases()); err != nil {
		return "", err
	}

	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

// partitionScopes splits the public scope names into the built-in scopes,
// composite scopes, and low-level resource:action scopes.
func partitionScopes(names []string) (builtin, composite, lowLevel []string) {
	for _, name := range names {
		scope := rbac.ScopeName(name)
		switch scope {
		case rbac.ScopeAll, rbac.ScopeApplicationConnect:
			builtin = append(builtin, name)
		default:
			if _, ok := rbac.CompositeSitePermissions(scope); ok {
				composite = append(composite, name)
			} else {
				lowLevel = append(lowLevel, name)
			}
		}
	}
	slices.Sort(builtin)
	slices.Sort(composite)
	slices.Sort(lowLevel)
	return builtin, composite, lowLevel
}

func renderBuiltin(b *strings.Builder, names []string) error {
	_, _ = b.WriteString("| Scope | Grants |\n|-------|--------|\n")
	for _, name := range names {
		scope, err := rbac.ExpandScope(rbac.ScopeName(name))
		if err != nil {
			return xerrors.Errorf("expand builtin scope %q: %w", name, err)
		}
		description := sentence(scope.DisplayName)
		if err := validateTableCells(name, description); err != nil {
			return xerrors.Errorf("render builtin scope %q: %w", name, err)
		}
		_, _ = fmt.Fprintf(b, "| `%s` | %s |\n", name, description)
	}
	_, _ = b.WriteString("\n")
	return nil
}

func renderComposite(b *strings.Builder, names []string) error {
	for _, name := range names {
		perms, ok := rbac.CompositeSitePermissions(rbac.ScopeName(name))
		if !ok {
			return xerrors.Errorf("composite scope %q has no permissions", name)
		}

		byResource := map[string][]string{}
		for _, perm := range perms {
			byResource[perm.ResourceType] = append(byResource[perm.ResourceType], string(perm.Action))
		}

		_, _ = fmt.Fprintf(b, "### `%s`\n\n", name)
		_, _ = b.WriteString("| Resource | Actions |\n|----------|---------|\n")
		for _, resource := range slices.Sorted(maps.Keys(byResource)) {
			actions := byResource[resource]
			slices.Sort(actions)
			actionList := codeList(actions)
			if err := validateTableCells(resource, actionList); err != nil {
				return xerrors.Errorf("render composite scope %q: %w", name, err)
			}
			_, _ = fmt.Fprintf(b, "| `%s` | %s |\n", resource, actionList)
		}
		_, _ = b.WriteString("\n")
	}
	return nil
}

func renderLowLevel(b *strings.Builder, names []string) error {
	byResource := map[string][]string{}
	for _, name := range names {
		resource, _, ok := rbac.ParseResourceAction(name)
		if !ok {
			return xerrors.Errorf("low-level scope %q is not a resource:action pair", name)
		}
		byResource[resource] = append(byResource[resource], name)
	}

	for _, resource := range slices.Sorted(maps.Keys(byResource)) {
		_, _ = fmt.Fprintf(b, "### `%s`\n\n", resource)
		_, _ = b.WriteString("| Scope | Description |\n|-------|-------------|\n")

		scopes := byResource[resource]
		slices.Sort(scopes)
		for _, scope := range scopes {
			_, action, _ := rbac.ParseResourceAction(scope)
			description, err := actionDescription(resource, action)
			if err != nil {
				return xerrors.Errorf("describe low-level scope %q: %w", scope, err)
			}
			if err := validateTableCells(scope, description); err != nil {
				return xerrors.Errorf("render low-level scope %q: %w", scope, err)
			}
			_, _ = fmt.Fprintf(b, "| `%s` | %s |\n", scope, description)
		}
		_, _ = b.WriteString("\n")
	}
	return nil
}

// actionDescription returns the description the policy declares for an action
// on a resource. The wildcard action has no policy entry of its own, so it is
// described in terms of the resource it covers.
func actionDescription(resource, action string) (string, error) {
	if action == policy.WildcardSymbol {
		return fmt.Sprintf("Every action on `%s`, including actions not listed on this page.", resource), nil
	}
	def, ok := policy.RBACPermissions[resource]
	if !ok {
		return "", xerrors.Errorf("resource %q has no policy definition", resource)
	}
	desc, ok := def.Actions[policy.Action(action)]
	if !ok {
		return "", xerrors.Errorf("resource %q has no description for action %q", resource, action)
	}
	return sentence(string(desc)), nil
}

// acronyms restores the capitalization of terms that the policy descriptions
// spell in lower case.
var acronyms = map[string]string{
	"api":  "API",
	"cli":  "CLI",
	"idp":  "IdP",
	"oidc": "OIDC",
	"ssh":  "SSH",
	"ui":   "UI",
	"url":  "URL",
}

var acronymPattern = regexp.MustCompile(`(?i)\b(` + strings.Join(slices.Sorted(maps.Keys(acronyms)), "|") + `)\b`)

// sentence capitalizes the first word of a policy description and gives it
// terminal punctuation, so table cells read as sentences.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	s = acronymPattern.ReplaceAllStringFunc(s, func(word string) string {
		return acronyms[strings.ToLower(word)]
	})

	first, rest, _ := strings.Cut(s, " ")
	first = stringutil.Capitalize(first)
	s = strings.TrimSpace(first + " " + rest)

	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// codeList renders values as a comma-separated list of inline code spans.
func codeList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "`" + v + "`"
	}
	return strings.Join(quoted, ", ")
}

func exampleCommand() string {
	args := make([]string, len(exampleScopes))
	for i, scope := range exampleScopes {
		args[i] = "--scope " + string(scope)
	}
	return "```shell\ncoder tokens create " + strings.Join(args, " ") + "\n```"
}

func renderDeprecatedAliases(b *strings.Builder, aliases map[rbac.ScopeName]rbac.ScopeName) error {
	_, _ = b.WriteString("| Deprecated name | Canonical name |\n|-----------------|----------------|\n")
	for _, alias := range slices.Sorted(maps.Keys(aliases)) {
		if err := validateTableCells(string(alias), string(aliases[alias])); err != nil {
			return xerrors.Errorf("render deprecated scope %q: %w", alias, err)
		}
		_, _ = fmt.Fprintf(b, "| `%s` | `%s` |\n", alias, aliases[alias])
	}
	return nil
}

func validateTableCells(values ...string) error {
	for _, value := range values {
		if strings.Contains(value, "|") {
			return xerrors.Errorf("table cell contains a pipe: %q", value)
		}
	}
	return nil
}
