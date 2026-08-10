package render

import (
	"strings"
	"text/template"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/notifications/types"
)

// NoValue is used when a template variable is not found.
// This string is not exported as a const from the text/template.
const NoValue = "<no value>"

// markdownReplacer escapes characters that have special meaning in Markdown.
// This prevents user-controlled values (display names, labels) from being
// interpreted as Markdown syntax when interpolated into notification body
// templates that are subsequently rendered as HTML.
var markdownReplacer = strings.NewReplacer(
	// Escape the backslash first (conceptually): strings.NewReplacer performs a
	// single non-overlapping left-to-right pass and never re-processes its own
	// output, so a user-supplied "\" becomes a literal "\\" without touching the
	// backslashes we introduce below.
	"\\", "\\\\",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"#", "\\#",
	"!", "\\!",
	"*", "\\*",
	// Escape both angle brackets. ">" alone left "<...>" CommonMark autolinks
	// exploitable (angle-bracket autolinks are core inline syntax, not the
	// parser.Autolink extension, so disabling that extension does not stop
	// them); escaping "<" prevents such an autolink from ever opening.
	"<", "\\<",
	">", "\\>",
	"~", "\\~",
	"`", "\\`",
	"|", "\\|",
	"_", "\\_",
	"\n", " ",
	"\r", "",
)

// SanitizeMarkdown escapes Markdown metacharacters in a string and
// collapses newlines so that the value is rendered as literal text
// when embedded in a Markdown document.
func SanitizeMarkdown(s string) string {
	return markdownReplacer.Replace(s)
}

// SanitizedPayload returns a copy of p with Markdown metacharacters escaped in
// the user-controlled fields (UserName and Labels). Use the returned copy only
// to render the title and body templates, whose output is subsequently passed
// through a Markdown renderer (HTMLFromMarkdownSafe for the SMTP HTML email, and
// react-markdown for the in-product inbox). Escaping there prevents display
// names and label values from injecting Markdown such as links or headings.
//
// The original payload is left unmodified. Non-Markdown consumers must receive
// the verbatim values: the webhook delivers payload.Labels and payload.UserName
// as raw JSON, the SMTP greeting interpolates UserName as HTML (escaped by the
// template's `| html`, not by Markdown escaping), and the plaintext email part
// is not Markdown. Escaping those in place left literal backslashes in every one
// of them.
//
// Label keys listed in skipLabels are left verbatim. These are machine-set enum
// values used in Go template control flow (e.g. {{if eq .Labels.x "y"}}) rather
// than rendered as text; escaping them (for example turning "user_override" into
// "user\_override") would break the comparison. Every skipped key must be
// assigned from a code or DB enum and must never carry user input.
func SanitizedPayload(p types.MessagePayload, skipLabels ...string) types.MessagePayload {
	skip := make(map[string]struct{}, len(skipLabels))
	for _, k := range skipLabels {
		skip[k] = struct{}{}
	}

	sanitized := p
	sanitized.UserName = SanitizeMarkdown(p.UserName)
	if p.Labels != nil {
		// Copy the map so the caller's payload keeps its raw label values.
		sanitized.Labels = make(map[string]string, len(p.Labels))
		for k, v := range p.Labels {
			if _, ok := skip[k]; ok {
				sanitized.Labels[k] = v
				continue
			}
			sanitized.Labels[k] = SanitizeMarkdown(v)
		}
	}
	return sanitized
}

// GoTemplate attempts to substitute the given payload into the given template using Go's templating syntax.
// TODO: memoize templates for memory efficiency?
func GoTemplate(in string, payload types.MessagePayload, extraFuncs template.FuncMap) (string, error) {
	tmpl, err := template.New("text").
		Funcs(extraFuncs).
		// text/template substitutes a missing label with "<no value>".
		// NOTE: html/template does not, for obvious reasons.
		Option("missingkey=invalid").
		Parse(in)
	if err != nil {
		return "", xerrors.Errorf("template parse: %w", err)
	}

	var out strings.Builder
	if err = tmpl.Execute(&out, payload); err != nil {
		return "", xerrors.Errorf("template execute: %w", err)
	}

	return out.String(), nil
}
