package render

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	gomarkdown "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"golang.org/x/xerrors"
)

// markdownLinkChars are the ASCII characters that can initiate a Markdown
// link, image, or "<url>" autolink. Escaping these neutralizes link/phishing
// injection while leaving all other punctuation, including emphasis and code
// markers (* _ ~ `), parentheses, and common name punctuation
// (- . , ' _ ! # & % @ ( )), untouched. ':' is handled separately (see
// EscapeMarkdownLinks) so that benign values such as timestamps ("15:04") are
// not modified.
//
// '(' and ')' are intentionally omitted: a link or image requires the "[...]"
// label, and a bare-URL autolink requires the scheme ':'. Both are already
// escaped, so parentheses cannot complete a link on their own, and escaping
// them would needlessly mangle common names like "Jane Doe (she/her)".
var markdownLinkChars = map[rune]struct{}{
	'[': {}, ']': {}, // inline / reference links and images
	'<': {}, '>': {}, // <url> autolinks
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// EscapeMarkdownLinks renders untrusted text so it cannot form a Markdown link,
// image, or autolink. Newlines are collapsed to spaces, and each link-forming
// ASCII character is replaced with a numeric HTML entity that the HTML and
// plaintext renderers decode back to the literal character for display.
//
// It deliberately escapes only link-forming characters, not emphasis or code
// markers: an injected link is a phishing vector, whereas injected emphasis is
// merely cosmetic. Leaving "* _ ~ ` ! # ( )" and name punctuation untouched
// keeps names and identifiers (e.g. "muddy_russell78", "My *Cool* App",
// "Jane Doe (she/her)") rendering cleanly on every delivery channel, including
// webhook Markdown consumed by Slack or Teams. (An image "![alt](url)" is still
// neutralized because its "[" and "]" are escaped.)
//
// A ':' is escaped only when it directly joins a scheme to a target, i.e. when
// it is preceded by an ASCII letter and followed by a non-whitespace character
// (e.g. "https://", "mailto:user@host"). This breaks every scheme autolink,
// current or future, without touching a ':' that cannot start one, such as in
// a timestamp ("15:04", digit before) or a label ("Note: hi", space after).
//
// Note: block-level markers such as '#' and '-' are not escaped ('>' is, since
// it doubles as the "<url>" autolink closer). Because newlines are collapsed,
// an interpolated value can only start a block element when a template places
// it at the beginning of a line. Callers must not place untrusted values at a
// line start (see the notifications line-start guard).
func EscapeMarkdownLinks(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
	runes := []rune(s)
	buf := make([]byte, 0, len(s))
	for i, r := range runes {
		escape := false
		if r < 128 {
			if _, ok := markdownLinkChars[r]; ok {
				escape = true
			} else if r == ':' && i > 0 && isASCIILetter(runes[i-1]) &&
				i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
				// Break "scheme:" autolinks (e.g. https://, mailto:) without
				// touching a ':' that cannot start one, such as a timestamp
				// ("15:04") or a label ("Note: hi").
				escape = true
			}
		}
		if escape {
			buf = append(buf, '&', '#')
			buf = strconv.AppendInt(buf, int64(r), 10)
			buf = append(buf, ';')
		} else {
			buf = utf8.AppendRune(buf, r)
		}
	}
	return string(buf)
}

var plaintextStyle = ansi.StyleConfig{
	Document: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	BlockQuote: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	Paragraph: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	List: ansi.StyleList{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{},
		},
		LevelIndent: 4,
	},
	Heading: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	H1: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	H2: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	H3: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	H4: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	H5: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	H6: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	Strikethrough:  ansi.StylePrimitive{},
	Emph:           ansi.StylePrimitive{},
	Strong:         ansi.StylePrimitive{},
	HorizontalRule: ansi.StylePrimitive{},
	Item:           ansi.StylePrimitive{},
	Enumeration: ansi.StylePrimitive{
		BlockPrefix: ". ",
	}, Task: ansi.StyleTask{},
	Link: ansi.StylePrimitive{
		Format: "({{.text}})",
	},
	LinkText: ansi.StylePrimitive{
		Format: "{{.text}}",
	},
	ImageText: ansi.StylePrimitive{
		Format: "{{.text}}",
	},
	Image: ansi.StylePrimitive{
		Format: "({{.text}})",
	},
	Code: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
	},
	CodeBlock: ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{},
	},
	Table:                 ansi.StyleTable{},
	DefinitionDescription: ansi.StylePrimitive{},
}

// PlaintextFromMarkdown function converts the description with optional Markdown tags
// to the plaintext form.
func PlaintextFromMarkdown(markdown string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("ascii"),
		glamour.WithWordWrap(0), // don't need to add spaces in the end of line
		glamour.WithStyles(plaintextStyle),
	)
	if err != nil {
		return "", xerrors.Errorf("can't initialize the Markdown renderer: %w", err)
	}

	output, err := renderer.Render(markdown)
	if err != nil {
		return "", xerrors.Errorf("can't render description to plaintext: %w", err)
	}
	defer renderer.Close()

	return strings.TrimSpace(output), nil
}

func HTMLFromMarkdown(markdown string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.HardLineBreak) // Added HardLineBreak.
	doc := p.Parse([]byte(markdown))
	renderer := html.NewRenderer(html.RendererOptions{
		// Safelink drops links with unsafe URI schemes (e.g. javascript:,
		// data:) as defense-in-depth against injected markup. It applies to
		// links only, not images.
		Flags: html.CommonFlags | html.SkipHTML | html.Safelink,
	})
	return string(bytes.TrimSpace(gomarkdown.Render(doc, renderer)))
}
