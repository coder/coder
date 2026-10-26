// Package coderkit ships the Coder Kit, the stylesheet that gives HTML
// artifacts the Coder product look, and the built-in coder-kit skill that
// teaches chat agents to use it.
//
// A page links the kit by name, as the coder-brand skill does:
//
//	<link rel="stylesheet" href="coder-kit.css">
//	<script src="coder-kit.js" defer></script>
//
// Pack replaces both tags with the embedded kit when coderd serves the
// page, so the stored artifact stays small and the sandboxed frame needs no
// network. kit/ is a verbatim copy of skills/coder-brand/kit in
// coder/fux-plugins; NOTICE.md names the commit.
package coderkit

import (
	"bytes"
	"embed"
	"encoding/base64"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

//go:embed kit
var kitFS embed.FS

//go:embed SKILL.md
var skillMarkdown string

// These match the tags that coder-brand's pack.mjs replaces: any path that
// ends in the kit's file name.
var (
	kitLinkRe    = regexp.MustCompile(`(?i)<link\b[^>]*href=["'][^"']*coder-kit\.css["'][^>]*>`)
	kitScriptRe  = regexp.MustCompile(`(?i)<script\b[^>]*src=["'][^"']*coder-kit\.js["'][^>]*>\s*</script>`)
	fontURLRe    = regexp.MustCompile(`url\("([^":]+\.woff2)"\)`)
	importRe     = regexp.MustCompile(`(?m)^@import url\("([^":]+\.css)"\);[ \t]*$`)
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	kitClassRe   = regexp.MustCompile(`\.(ck-[a-z0-9-]+)`)
	scriptBodyRe = regexp.MustCompile(`(?is)(<script\b[^>]*>).*?(</script>)`)
	classAttrRe  = regexp.MustCompile(`(?i)\bclass=["']([^"']*)["']`)
)

// Theme is the color theme that the viewer of a page uses.
type Theme string

const (
	// ThemeNone leaves the theme to the page and the OS preference.
	ThemeNone Theme = ""
	// ThemeLight forces the light theme on the page root.
	ThemeLight Theme = "light"
	// ThemeDark forces the dark theme on the page root.
	ThemeDark Theme = "dark"
)

// ParseTheme returns the theme for a query value. Unknown values return
// ThemeNone.
func ParseTheme(value string) Theme {
	switch Theme(value) {
	case ThemeLight, ThemeDark:
		return Theme(value)
	default:
		return ThemeNone
	}
}

// stylesheet is coder-kit.css with its imports and fonts inlined.
var stylesheet = sync.OnceValue(func() string {
	css, err := inlineCSS("kit/coder-kit.css", map[string]bool{})
	if err != nil {
		// The kit is embedded, so this is a build defect that tests catch.
		panic(xerrors.Errorf("inline the Coder Kit: %w", err))
	}
	return css
})

var script = sync.OnceValue(func() string {
	js, err := kitFS.ReadFile("kit/coder-kit.js")
	if err != nil {
		panic(xerrors.Errorf("read coder-kit.js: %w", err))
	}
	return string(js)
})

// classes is every ck- class that the kit defines.
var classes = sync.OnceValue(func() map[string]bool {
	known := map[string]bool{}
	for _, name := range []string{"kit/components.css", "kit/icons.css"} {
		css, err := kitFS.ReadFile(name)
		if err != nil {
			panic(xerrors.Errorf("read %s: %w", name, err))
		}
		for _, m := range kitClassRe.FindAllStringSubmatch(cssCommentRe.ReplaceAllString(string(css), ""), -1) {
			known[m[1]] = true
		}
	}
	return known
})

func inlineCSS(name string, seen map[string]bool) (string, error) {
	if seen[name] {
		return "", xerrors.Errorf("%s imports itself", name)
	}
	seen[name] = true
	raw, err := kitFS.ReadFile(name)
	if err != nil {
		return "", err
	}
	dir := path.Dir(name)
	var inlineErr error
	css := fontURLRe.ReplaceAllStringFunc(string(raw), func(match string) string {
		font, err := kitFS.ReadFile(path.Join(dir, fontURLRe.FindStringSubmatch(match)[1]))
		if err != nil {
			inlineErr = err
			return match
		}
		return `url("data:font/woff2;base64,` + base64.StdEncoding.EncodeToString(font) + `")`
	})
	css = importRe.ReplaceAllStringFunc(css, func(match string) string {
		imported, err := inlineCSS(path.Join(dir, importRe.FindStringSubmatch(match)[1]), seen)
		if err != nil {
			inlineErr = err
			return match
		}
		return imported
	})
	if inlineErr != nil {
		return "", inlineErr
	}
	return css, nil
}

// LinksKit reports whether an HTML page links coder-kit.css.
func LinksKit(html []byte) bool {
	return kitLinkRe.Match(html)
}

// Pack returns the page with the first coder-kit.css link replaced by the
// inlined kit, and the first coder-kit.js script replaced by the inlined
// script. A theme other than ThemeNone sets data-theme on the page root,
// unless the page sets it. A page that does not link the kit is returned
// as is.
func Pack(html []byte, theme Theme) []byte {
	loc := kitLinkRe.FindIndex(html)
	if loc == nil {
		return html
	}
	var style strings.Builder
	if theme != ThemeNone {
		_, _ = style.WriteString(`<script data-coder-kit>(function(){var r=document.documentElement;if(!r.hasAttribute("data-theme"))r.setAttribute("data-theme","` + string(theme) + `")})()</script>`)
	}
	_, _ = style.WriteString("<style data-coder-kit>\n")
	_, _ = style.WriteString(stylesheet())
	_, _ = style.WriteString("</style>")

	out := make([]byte, 0, len(html)+style.Len()+len(script()))
	out = append(out, html[:loc[0]]...)
	out = append(out, style.String()...)
	rest := html[loc[1]:]
	if js := kitScriptRe.FindIndex(rest); js != nil {
		out = append(out, rest[:js[0]]...)
		out = append(out, "<script data-coder-kit>\n"...)
		out = append(out, script()...)
		out = append(out, "</script>"...)
		rest = rest[js[1]:]
	}
	return append(out, rest...)
}

// UnknownClasses returns the ck- classes in the page's class attributes
// that the kit does not define, sorted and without duplicates. Classes
// that scripts build at run time are not checked.
func UnknownClasses(html []byte) []string {
	markup := scriptBodyRe.ReplaceAll(html, []byte("$1$2"))
	known := classes()
	var unknown []string
	for _, m := range classAttrRe.FindAllSubmatch(markup, -1) {
		for _, class := range bytes.Fields(m[1]) {
			name := string(class)
			if strings.HasPrefix(name, "ck-") && !known[name] && !slices.Contains(unknown, name) {
				unknown = append(unknown, name)
			}
		}
	}
	slices.Sort(unknown)
	return unknown
}

// skill is the parsed built-in coder-kit skill.
var skill = sync.OnceValue(func() skills.ParsedSkill {
	name, description, body, err := workspacesdk.ParseSkillFrontmatter(skillMarkdown)
	if err != nil {
		panic(xerrors.Errorf("parse the coder-kit skill: %w", err))
	}
	return skills.ParsedSkill{
		Skill: skills.Skill{Name: name, Description: description, Source: skills.SourceBuiltin},
		Body:  body,
	}
})

// Skill returns the built-in coder-kit skill.
func Skill() skills.ParsedSkill {
	return skill()
}
