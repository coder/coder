package coderd_test

import (
	"archive/tar"
	"bytes"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateVersionDocs(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for name, content := range map[string]string{
		"README.md":       "# Welcome",
		"./ACCESS.md":     "## Remote Desktop Access",
		"NOTES.MD":        "# Notes",
		"docs/private.md": "not root",
		"../outside.md":   "not root",
		"main.tf":         "source must stay restricted",
	} {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}))
		_, err := writer.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "LINK.md", Typeflag: tar.TypeSymlink, Linkname: "main.tf"}))
	require.NoError(t, writer.Close())
	file, err := client.Upload(ctx, codersdk.ContentTypeTar, &archive)
	require.NoError(t, err)
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil, func(req *codersdk.CreateTemplateVersionRequest) { req.FileID = file.ID })
	coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	docs, err := member.TemplateVersionDocs(ctx, version.ID)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"README.md": "# Welcome", "ACCESS.md": "## Remote Desktop Access", "NOTES.MD": "# Notes"}, docs.Files)
	response, err := member.Request(ctx, http.MethodGet, "/api/v2/files/"+file.ID.String(), nil)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusNotFound, response.StatusCode)
	_, err = member.TemplateVersionDocs(ctx, uuid.New())
	require.Error(t, err)
	anonymous := codersdk.New(client.URL)
	_, err = anonymous.TemplateVersionDocs(ctx, version.ID)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
}
