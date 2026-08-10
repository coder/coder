package render

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	gomarkdown "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"golang.org/x/xerrors"
)

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
	return renderHTMLFromMarkdown(markdown, parser.CommonExtensions|parser.HardLineBreak, html.CommonFlags|html.SkipHTML)
}

// HTMLFromMarkdownSafe renders Markdown to HTML with additional security
// hardening for content that may include user-controlled values (e.g.
// notification emails): autolinks are disabled so that only explicit Markdown
// link syntax produces <a> tags, and Safelink drops links whose scheme is not
// http/https/ftp/mailto.
//
// The hardening is scoped to this function. HTMLFromMarkdown renders
// admin-authored deployment text (OIDCConfig.SignupsDisabledText) and keeps the
// standard flags so links with custom schemes (e.g. slack://) still render.
func HTMLFromMarkdownSafe(markdown string) string {
	extensions := parser.CommonExtensions | parser.HardLineBreak
	extensions &^= parser.Autolink
	return renderHTMLFromMarkdown(markdown, extensions, html.CommonFlags|html.SkipHTML|html.Safelink)
}

func renderHTMLFromMarkdown(markdown string, extensions parser.Extensions, flags html.Flags) string {
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(markdown))
	renderer := html.NewRenderer(html.RendererOptions{
		Flags: flags,
	})
	return string(bytes.TrimSpace(gomarkdown.Render(doc, renderer)))
}
