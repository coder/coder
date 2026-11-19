package main

import (
	"bytes"
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsrelated"
)

// relatedDataSectionName is the API section that documents include_related.
const relatedDataSectionName = "Workspaces"

const relatedDataIntro = `## Related data

The [Get workspace metadata by ID](#get-workspace-metadata-by-id) and [Get workspace metadata by user and workspace name](#get-workspace-metadata-by-user-and-workspace-name) endpoints accept an ` + "`include_related`" + ` query parameter that selects which related data the server loads.
Loading less related data makes the request cheaper.

The value is a comma-separated list of paths, such as ` + "`template,latest_build.resources.agents`" + `:

- Omit ` + "`include_related`" + ` to return all related data.
- Pass an empty value, ` + "`include_related=`" + `, to return only the fields that are always returned.
- Selecting a path also selects its ancestors. For example, ` + "`latest_build.job`" + ` also selects ` + "`latest_build`" + `.
- End a path with ` + "`.*`" + ` to select it and every path below it, or pass ` + "`*`" + ` to select everything.

Fields whose related data is not selected don't hold meaningful values, so don't read them.
Most are returned with zero values, such as ` + "`null`" + `, an empty string, ` + "`0`" + `, ` + "`false`" + `, or a zero ID.
Fields computed from related data, such as ` + "`health`" + `, ` + "`outdated`" + `, and ` + "`latest_build.status`" + `, can hold values computed from the missing data.

The following table lists the response fields that each path returns.
A field that holds an object or array returns everything in it.
A row that names more than one path lists fields that need all of those paths.

`

// relatedDataSection renders the Markdown section that documents which
// include_related paths return which codersdk.Workspace fields.
func relatedDataSection() ([]byte, error) {
	fields := wsrelated.Fields(reflect.TypeFor[codersdk.Workspace]())

	pathIndex := map[string]int{}
	for i, p := range wsrelated.Paths() {
		pathIndex[p] = i
	}

	// Fields of an untagged struct are covered by the field that holds the
	// struct, so only fields with their own tag are listed.
	var tagged []wsrelated.Field
	for _, f := range fields {
		if !f.Inherited {
			tagged = append(tagged, f)
		}
	}

	type row struct {
		order  []int
		label  string
		fields []string
	}
	rows := map[string]*row{}
	var notReturned []string
	seen := map[string]bool{}
	for _, f := range tagged {
		// A field whose struct has tagged fields is described by those fields.
		isContainer := slices.ContainsFunc(tagged, func(g wsrelated.Field) bool {
			return strings.HasPrefix(g.JSONPath, f.JSONPath+".") || strings.HasPrefix(g.JSONPath, f.JSONPath+"[].")
		})
		if !isContainer {
			r, ok := rows[f.Tag]
			if !ok {
				r = &row{label: "Always returned"}
				var labels []string
				if f.Tag != "" {
					for _, p := range strings.Split(f.Tag, ",") {
						i, ok := pathIndex[p]
						if !ok {
							return nil, xerrors.Errorf("field %s has unknown %s tag path %q", f.JSONPath, wsrelated.TagKey, p)
						}
						r.order = append(r.order, i)
						labels = append(labels, "`"+p+"`")
					}
					r.label = strings.Join(labels, " and ")
				}
				rows[f.Tag] = r
			}
			r.fields = append(r.fields, "`"+f.JSONPath+"`")
		}

		// Untagged fields beside f in the same struct are not returned.
		prefix := ""
		if i := strings.LastIndex(f.JSONPath, "."); i >= 0 {
			prefix = f.JSONPath[:i+1]
		}
		for i := range f.Struct.NumField() {
			sf := f.Struct.Field(i)
			if _, ok := sf.Tag.Lookup(wsrelated.TagKey); ok || !sf.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = sf.Name
			}
			if p := prefix + name; !seen[p] {
				seen[p] = true
				notReturned = append(notReturned, "`"+p+"`")
			}
		}
	}

	sorted := make([]*row, 0, len(rows))
	for _, r := range rows {
		sorted = append(sorted, r)
	}
	slices.SortFunc(sorted, func(a, b *row) int {
		return cmp.Or(slices.Compare(a.order, b.order), cmp.Compare(a.label, b.label))
	})

	var buf bytes.Buffer
	_, _ = buf.WriteString(relatedDataIntro)
	_, _ = buf.WriteString("| `include_related` | Response fields |\n")
	_, _ = buf.WriteString("|---|---|\n")
	for _, r := range sorted {
		_, _ = fmt.Fprintf(&buf, "| %s | %s |\n", r.label, strings.Join(r.fields, ", "))
	}
	if len(notReturned) > 0 {
		_, _ = buf.WriteString("\nThe following fields are not returned by these endpoints, whatever the value of `include_related`: ")
		_, _ = buf.WriteString(strings.Join(notReturned, ", "))
		_, _ = buf.WriteString(".\n")
	}
	_, _ = buf.WriteString("\n")
	return buf.Bytes(), nil
}

// insertAfterHeading inserts content after the first line of section, its
// "# {name}" heading.
func insertAfterHeading(section, content []byte) []byte {
	heading, rest, _ := bytes.Cut(section, []byte{'\n'})
	out := make([]byte, 0, len(section)+len(content)+2)
	out = append(out, heading...)
	out = append(out, "\n\n"...)
	out = append(out, content...)
	out = append(out, bytes.TrimLeft(rest, "\r\n")...)
	return out
}
