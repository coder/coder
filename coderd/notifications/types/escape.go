package types

import "github.com/coder/coder/v2/coderd/render"

// EscapedForMarkdown returns a copy of the payload whose string values have
// Markdown structure neutralized, for rendering the title and body templates.
//
// The receiver is left untouched. The stored payload keeps the values as they
// were enqueued, which is what the webhook dispatcher surfaces to consumers, and
// what the SMTP dispatcher escapes at its own HTML sinks.
func (p MessagePayload) EscapedForMarkdown() MessagePayload {
	out := p
	out.UserName = render.EscapeMarkdown(p.UserName)

	if p.Labels != nil {
		labels := make(map[string]string, len(p.Labels))
		for k, v := range p.Labels {
			labels[k] = render.EscapeMarkdown(v)
		}
		out.Labels = labels
	}

	if p.Data != nil {
		data := make(map[string]any, len(p.Data))
		for k, v := range p.Data {
			data[k] = escapeValue(v)
		}
		out.Data = data
	}
	return out
}

// escapeValue walks a decoded JSON value and escapes its string leaves. Numbers,
// booleans and nulls pass through unchanged so that template comparisons such as
// `{{if gt $version.failed_count 1}}` keep working.
func escapeValue(v any) any {
	switch t := v.(type) {
	case string:
		return render.EscapeMarkdown(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = escapeValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = escapeValue(vv)
		}
		return out
	default:
		return v
	}
}
