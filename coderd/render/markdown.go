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

// innerTextMarkdown converts Markdown to HTML for InnerTextFromMarkdown. Table
// renders cells as text (not pipe-delimited lines); WithUnsafe lets embedded raw
// HTML through so its inner text survives. Safe to share: goldmark inits the
// parser once via sync.Once, then only reads it.
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

// InnerTextFromMarkdown renders Markdown (including embedded raw HTML) to HTML
// and returns its visible text ("innerText"). Block, code-line, and table-cell
// boundaries become newlines and intra-line whitespace is collapsed; link text
// is kept but URLs, images, and badges are dropped.
//
// Input is untrusted: a parser panic is recovered and returned as an error.
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
	// script and style are raw-text elements: their body is the single text token
	// after the start tag. Skip just that token (not a running depth) so a stray
	// </script> or unterminated tag can't swallow the rest of the document.
	skipNextText := false
	for {
		if z.Next() == xhtml.ErrorToken {
			break // includes io.EOF
		}
		switch tok := z.Token(); tok.Type {
		case xhtml.StartTagToken:
			skipNextText = tok.Data == "script" || tok.Data == "style"
		case xhtml.TextToken:
			if skipNextText {
				skipNextText = false
				continue
			}
			_, _ = b.WriteString(tok.Data)
		default:
			skipNextText = false
		}
	}

	// Collapse intra-line whitespace but keep newlines so code lines, table
	// cells, and block boundaries stay on separate lines; drop blank lines.
	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if f := strings.Join(strings.Fields(line), " "); f != "" {
			lines = append(lines, f)
		}
	}
	return strings.Join(lines, "\n"), nil
}
