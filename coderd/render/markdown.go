package render

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	gomarkdown "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
	"golang.org/x/xerrors"
)

// innerTextMarkdown renders Markdown to HTML for InnerTextFromMarkdown. The
// Table extension renders table cells as text rather than pipe-delimited lines,
// and WithUnsafe passes embedded raw HTML through so its inner text survives.
// Read-only after construction.
var innerTextMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.Table),
	goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
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
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.HardLineBreak) // Added HardLineBreak.
	doc := p.Parse([]byte(markdown))
	renderer := html.NewRenderer(html.RendererOptions{
		Flags: html.CommonFlags | html.SkipHTML,
	})
	return string(bytes.TrimSpace(gomarkdown.Render(doc, renderer)))
}

// InnerTextFromMarkdown renders Markdown (including any embedded raw HTML) to
// HTML and returns the visible text content (its "innerText"): markdown
// formatting, images, and link URLs collapse to their visible text, while code
// blocks and table cells are preserved as text. Whitespace is collapsed to
// single spaces.
//
// The input is treated as untrusted: a panic from the markdown or HTML parser is
// recovered and returned as an error.
func InnerTextFromMarkdown(markdown string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = "", xerrors.Errorf("render markdown to innertext: %v", r)
		}
	}()

	var rendered bytes.Buffer
	if convErr := innerTextMarkdown.Convert([]byte(markdown), &rendered); convErr != nil {
		return "", xerrors.Errorf("convert markdown to html: %w", convErr)
	}

	z := xhtml.NewTokenizer(&rendered)
	var b strings.Builder
	skipDepth := 0
	for {
		if z.Next() == xhtml.ErrorToken {
			break // includes io.EOF
		}
		switch tok := z.Token(); tok.Type {
		case xhtml.StartTagToken:
			if tok.Data == "script" || tok.Data == "style" {
				skipDepth++
			}
		case xhtml.EndTagToken:
			if (tok.Data == "script" || tok.Data == "style") && skipDepth > 0 {
				skipDepth--
			}
		case xhtml.TextToken:
			if skipDepth == 0 {
				// Write text as-is; goldmark's inter-block newlines preserve
				// block separation without splitting inline punctuation.
				_, _ = b.WriteString(tok.Data)
			}
		}
	}
	// Collapse all whitespace (newlines, block boundaries) to single spaces.
	return strings.Join(strings.Fields(b.String()), " "), nil
}
