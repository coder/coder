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
	"sort"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/flog"
)

// routeTitles is the manifest breadcrumb to this page's route. The page
// mirrors that route's metadata into its front matter, so the manifest stays
// the single source of the title and description.
var routeTitles = []string{"Reference", "API key scopes"}

const intro = `A scope limits what an API key can do.
A scoped key never exceeds the permissions of the user who created it: Coder checks the scope and the user's roles on every request, so a scope narrows access and never widens it.

Pass ` + "`--scope`" + ` once per scope when you create a token:

` + "```shell" + `
coder tokens create --scope coder:workspaces.access --scope template:read
` + "```" + `

A token created without an explicit scope uses ` + "`coder:all`" + `, which grants the full permissions of its owner.
To create and revoke tokens, refer to [Sessions & API Tokens](../admin/users/sessions-tokens.md).

This page lists every scope a token can request.
Coder rejects any other scope name as internal.

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

The ` + "`resource:*`" + ` form grants every action listed for that resource.

`

const deprecatedSection = `## Deprecated scope names

Coder still accepts the following names and stores each one as its canonical equivalent.
Use the canonical name.

| Deprecated name | Canonical name |
|-----------------|----------------|
| ` + "`all`" + ` | ` + "`coder:all`" + ` |
| ` + "`application_connect`" + ` | ` + "`coder:application_connect`" + ` |
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
	_, _ = b.WriteString(intro)

	_, _ = b.WriteString(builtinSection)
	if err := renderBuiltin(&b, builtin); err != nil {
		return "", err
	}

	_, _ = b.WriteString(compositeSection)
	if err := renderComposite(&b, composite); err != nil {
		return "", err
	}

	_, _ = b.WriteString(lowLevelSection)
	renderLowLevel(&b, lowLevel)

	_, _ = b.WriteString(deprecatedSection)

	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

// partitionScopes splits the public scope names into the built-in scopes, the
// composite coder:* scopes, and the low-level resource:action scopes.
func partitionScopes(names []string) (builtin, composite, lowLevel []string) {
	for _, name := range names {
		switch {
		case name == string(rbac.ScopeAll), name == string(rbac.ScopeApplicationConnect):
			builtin = append(builtin, name)
		case strings.HasPrefix(name, "coder:"):
			composite = append(composite, name)
		default:
			lowLevel = append(lowLevel, name)
		}
	}
	sort.Strings(builtin)
	sort.Strings(composite)
	sort.Strings(lowLevel)
	return builtin, composite, lowLevel
}

func renderBuiltin(b *strings.Builder, names []string) error {
	_, _ = b.WriteString("| Scope | Grants |\n|-------|--------|\n")
	for _, name := range names {
		scope, err := rbac.ExpandScope(rbac.ScopeName(name))
		if err != nil {
			return xerrors.Errorf("expand builtin scope %q: %w", name, err)
		}
		_, _ = fmt.Fprintf(b, "| `%s` | %s |\n", name, sentence(scope.DisplayName))
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
		for _, resource := range sortedKeys(byResource) {
			actions := byResource[resource]
			sort.Strings(actions)
			_, _ = fmt.Fprintf(b, "| `%s` | %s |\n", resource, codeList(actions))
		}
		_, _ = b.WriteString("\n")
	}
	return nil
}

func renderLowLevel(b *strings.Builder, names []string) {
	byResource := map[string][]string{}
	for _, name := range names {
		resource, _, ok := rbac.ParseResourceAction(name)
		if !ok {
			continue
		}
		byResource[resource] = append(byResource[resource], name)
	}

	for _, resource := range sortedKeys(byResource) {
		_, _ = fmt.Fprintf(b, "### `%s`\n\n", resource)
		_, _ = b.WriteString("| Scope | Allows |\n|-------|--------|\n")

		scopes := byResource[resource]
		sort.Strings(scopes)
		for _, scope := range scopes {
			_, action, _ := rbac.ParseResourceAction(scope)
			_, _ = fmt.Fprintf(b, "| `%s` | %s |\n", scope, actionDescription(resource, action))
		}
		_, _ = b.WriteString("\n")
	}
}

// actionDescription returns the description the policy declares for an action
// on a resource. The wildcard action has no policy entry of its own, so it is
// described in terms of the resource it covers.
func actionDescription(resource, action string) string {
	if action == policy.WildcardSymbol {
		return fmt.Sprintf("Every action listed for `%s`.", resource)
	}
	def, ok := policy.RBACPermissions[resource]
	if !ok {
		return ""
	}
	desc, ok := def.Actions[policy.Action(action)]
	if !ok {
		return ""
	}
	return sentence(string(desc))
}

// acronyms restores the capitalization of terms that the policy descriptions
// spell in lower case. Only the first word of a description is rewritten,
// because that is the word sentence capitalizes.
var acronyms = map[string]string{
	"api":  "API",
	"cli":  "CLI",
	"idp":  "IdP",
	"oidc": "OIDC",
	"ssh":  "SSH",
	"ui":   "UI",
	"url":  "URL",
}

// sentence capitalizes the first word of a policy description and gives it
// terminal punctuation, so table cells read as sentences.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	first, rest, _ := strings.Cut(s, " ")
	if upper, ok := acronyms[strings.ToLower(first)]; ok {
		first = upper
	} else {
		first = strings.ToUpper(first[:1]) + first[1:]
	}
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

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
