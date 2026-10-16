package main

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Elements and attributes that docs pages have no use for. Each one carries
// accessibility requirements of its own (an accessible name, a valid role, a
// sensible focus order, a valid language tag) that no docs check verifies, so
// keeping them out of docs Markdown keeps those requirements out too. Content
// that seems to need one belongs in Markdown or in the docs site's components.
const (
	kindDisallowedElement   findingKind = "disallowed-element"
	kindDisallowedAttribute findingKind = "disallowed-attribute"
)

// disallowedElements maps each element to why it doesn't belong in a page.
// The elements stay in allowedElements, so the dedicated message here is what
// an author sees rather than the generic unknown-element one.
var disallowedElements = map[string]string{
	"iframe":   "embedded frames need a title and are opaque to search; link to the content instead",
	"object":   "embedded objects need a text alternative; link to the content or use an image with alt text",
	"area":     "image maps need alt text on every area; use an image with alt text and ordinary links",
	"form":     "docs pages don't submit forms",
	"input":    "form controls need labels and do nothing on a docs page",
	"select":   "form controls need labels and do nothing on a docs page",
	"textarea": "form controls need labels and do nothing on a docs page",
	"button":   "buttons need an accessible name and do nothing on a docs page; use a link, or bold text for a UI label",
	"meta":     "page metadata belongs to the docs site, not to page content",
}

// disallowedAttributes maps each attribute to why it doesn't belong in a page.
// Any aria-* attribute is also disallowed; see disallowedAttribute.
var disallowedAttributes = map[string]string{
	"role":     "roles override the element's native semantics",
	"tabindex": "tabindex changes the keyboard focus order",
	"lang":     "the docs site sets the page language",
}

// disallowedAttribute returns why the attribute is disallowed, or "" when it
// is allowed. The tokenizer lowercases attribute names.
func disallowedAttribute(name string) string {
	if strings.HasPrefix(name, "aria-") {
		return "ARIA attributes override what the element and its text already convey"
	}
	return disallowedAttributes[name]
}

// guardTag reports a disallowed element, and each disallowed attribute, on one
// start tag. name is the lowercased tag name.
func guardTag(z *html.Tokenizer, name string, line int) []finding {
	var out []finding
	if why, ok := disallowedElements[name]; ok {
		out = append(out, finding{
			line: line,
			kind: kindDisallowedElement,
			tag:  name,
			msg:  fmt.Sprintf("<%s> is not allowed in docs pages: %s", name, why),
		})
	}
	for {
		key, _, more := z.TagAttr()
		if why := disallowedAttribute(string(key)); why != "" {
			out = append(out, finding{
				line: line,
				kind: kindDisallowedAttribute,
				tag:  name,
				msg:  fmt.Sprintf("%s on <%s> is not allowed in docs pages: %s", key, name, why),
			})
		}
		if !more {
			return out
		}
	}
}
