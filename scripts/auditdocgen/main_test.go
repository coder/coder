package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestUpdateAuditDoc(t *testing.T) {
	t.Parallel()

	doc := []byte("# Title\n\n" + string(generatorPrefix) + "\n\nstale content\n\n" + string(generatorSuffix) + "\n\n## Next\n")
	resources := AuditableResourcesMap{
		"Template": {"name": true, "created_at": false},
	}

	got, err := updateAuditDoc(doc, resources)
	require.NoError(t, err)
	require.NotContains(t, string(got), "stale content")
	require.Contains(t, string(got), "### Template\n")
	require.True(t, bytes.HasPrefix(got, []byte("# Title\n\n"+string(generatorPrefix)+"\n\n")))
	require.True(t, bytes.HasSuffix(got, []byte("\n\n"+string(generatorSuffix)+"\n\n## Next\n")))

	again, err := updateAuditDoc(got, resources)
	require.NoError(t, err)
	require.Equal(t, string(got), string(again), "regenerating must be idempotent")
}

func TestUpdateAuditDocMissingMarkers(t *testing.T) {
	t.Parallel()

	_, err := updateAuditDoc([]byte("no markers"), AuditableResourcesMap{})
	require.Error(t, err)

	_, err = updateAuditDoc(append([]byte{}, generatorPrefix...), AuditableResourcesMap{})
	require.Error(t, err)
}

func TestWriteResourceSections(t *testing.T) {
	t.Parallel()

	resources := AuditableResourcesMap{
		"Zeta":           {"b": true, "a": false},
		"AuditableGroup": {"name": true},
	}
	actions := map[string][]codersdk.AuditAction{
		"Group": {codersdk.AuditActionCreate, codersdk.AuditActionDelete},
	}

	var buf bytes.Buffer
	writeResourceSections(&buf, resources, actions)

	want := "### Group\n\n" +
		"Actions: `create`, `delete`\n\n" +
		fieldsTableHeader +
		"<tr><td><code>name</code></td><td>Yes</td></tr>\n" +
		"</tbody>\n</table>\n\n" +
		"### Zeta\n\n" +
		fieldsTableHeader +
		"<tr><td><code>a</code></td><td>No</td></tr>\n" +
		"<tr><td><code>b</code></td><td>Yes</td></tr>\n" +
		"</tbody>\n</table>\n\n"
	require.Contains(t, fieldsTableHeader, `<th width="75%">Field</th><th width="25%">Tracked</th>`)
	require.Equal(t, want, buf.String())
}
