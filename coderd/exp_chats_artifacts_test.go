package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatArtifact(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	admin, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, admin.Client)
	model := createChatModel(t, admin)
	orgID := firstUser.OrganizationID

	projectOwner, _ := newChatProjectMember(t, admin, orgID)
	owner, ownerUser := newChatProjectMember(t, admin, orgID)
	member, memberUser := newChatProjectMember(t, admin, orgID)
	stranger, _ := newChatProjectMember(t, admin, orgID)

	project := createChatProject(t, projectOwner, orgID, "Artifacts")
	err := projectOwner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			ownerUser.ID.String():  codersdk.ChatProjectRoleUse,
			memberUser.ID.String(): codersdk.ChatProjectRoleUse,
		},
	})
	require.NoError(t, err)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           ownerUser.ID,
		LastModelConfigID: model.ID,
		Title:             "Funnel chat",
	})
	//nolint:gocritic // The test seeds rows as chatd, which publishes artifacts.
	sysCtx := dbauthz.AsChatd(ctx)
	insertArtifact := func(projectID uuid.NullUUID, versions ...[2]string) database.ChatArtifact {
		artifact, err := db.InsertChatArtifact(sysCtx, database.InsertChatArtifactParams{
			OrganizationID: orgID,
			OwnerID:        ownerUser.ID,
			ProjectID:      projectID,
			Title:          "Signup funnel",
		})
		require.NoError(t, err)
		var base int32
		for _, v := range versions {
			_, err := db.InsertChatArtifactVersion(sysCtx, database.InsertChatArtifactVersionParams{
				ArtifactID:  artifact.ID,
				BaseVersion: base,
				Name:        v[0],
				Mimetype:    v[1],
				Data:        []byte("<h1>" + v[0] + "</h1>"),
				CreatedBy:   uuid.NullUUID{UUID: ownerUser.ID, Valid: true},
				ChatID:      uuid.NullUUID{UUID: chat.ID, Valid: true},
			})
			require.NoError(t, err)
			base++
		}
		return artifact
	}
	shared := insertArtifact(uuid.NullUUID{UUID: project.ID, Valid: true},
		[2]string{"funnel.html", "text/plain"},
		[2]string{"funnel.md", "text/markdown"},
	)
	personal := insertArtifact(uuid.NullUUID{}, [2]string{"notes.md", "text/markdown"})

	type frameResponse struct {
		StatusCode int
		Header     http.Header
		Body       string
	}
	frame := func(ctx context.Context, client *codersdk.ExperimentalClient, artifactID uuid.UUID, version int, dest string) frameResponse {
		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/artifacts/%s/versions/%d/frame", artifactID, version), nil,
			func(r *http.Request) {
				if dest != "" {
					r.Header.Set("Sec-Fetch-Dest", dest)
				}
			})
		require.NoError(t, err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return frameResponse{StatusCode: res.StatusCode, Header: res.Header, Body: string(body)}
	}

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		got, err := owner.GetChatArtifact(ctx, shared.ID)
		require.NoError(t, err)
		require.Equal(t, int32(2), got.LatestVersion)
		require.Equal(t, "funnel.md", got.LatestName)
		require.Equal(t, "text/markdown", got.LatestMediaType)
		require.Equal(t, ownerUser.Username, got.OwnerUsername)
		require.Equal(t, &project.ID, got.ProjectID)
		require.Equal(t, project.Name, got.ProjectName)
		require.Len(t, got.Versions, 2)
		require.Equal(t, int32(2), got.Versions[0].Version, "newest first")
		require.Equal(t, "funnel.md", got.Versions[0].Name)
		require.Equal(t, int64(len("<h1>funnel.md</h1>")), got.Versions[0].Size)
		require.Equal(t, ownerUser.Username, got.Versions[0].CreatedByUsername)
		require.Equal(t, &chat.ID, got.Versions[0].ChatID)
		require.Equal(t, "Funnel chat", got.Versions[0].ChatTitle, "the chat owner sees the chat title")

		list, err := owner.ListChatArtifacts(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{shared.ID, personal.ID}, artifactIDs(list))
		for _, a := range list {
			require.Empty(t, a.Versions, "the list has no versions")
			if a.ID == shared.ID {
				require.Equal(t, "funnel.md", a.LatestName, "the list names the latest version's file")
				require.Equal(t, "text/markdown", a.LatestMediaType)
			}
		}
	})

	t.Run("ProjectMember", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		got, err := member.GetChatArtifact(ctx, shared.ID)
		require.NoError(t, err)
		require.Len(t, got.Versions, 2)
		require.Equal(t, &chat.ID, got.Versions[0].ChatID)
		require.Empty(t, got.Versions[0].ChatTitle, "only the chat owner sees the chat title")
		data, _, err := member.ChatArtifactVersionData(ctx, shared.ID, 2)
		require.NoError(t, err)
		require.Equal(t, "<h1>funnel.md</h1>", string(data))

		list, err := member.ListChatArtifacts(ctx)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{shared.ID}, artifactIDs(list))

		_, err = member.GetChatArtifact(ctx, personal.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ProjectOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		got, err := projectOwner.GetChatArtifact(ctx, shared.ID)
		require.NoError(t, err)
		require.Equal(t, shared.ID, got.ID)
		list, err := projectOwner.ListChatArtifacts(ctx)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{shared.ID}, artifactIDs(list))
	})

	t.Run("Stranger", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		for _, id := range []uuid.UUID{shared.ID, personal.ID} {
			_, err := stranger.GetChatArtifact(ctx, id)
			requireSDKError(t, err, http.StatusNotFound)
			_, _, err = stranger.ChatArtifactVersionData(ctx, id, 1)
			requireSDKError(t, err, http.StatusNotFound)
			require.Equal(t, http.StatusNotFound, frame(ctx, stranger, id, 1, "iframe").StatusCode)
		}
		list, err := stranger.ListChatArtifacts(ctx)
		require.NoError(t, err)
		require.Empty(t, list)
	})

	t.Run("FrameHeaders", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res := frame(ctx, member, shared.ID, 1, "iframe")
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
		csp := res.Header.Get("Content-Security-Policy")
		require.Contains(t, csp, "sandbox allow-scripts;")
		require.Contains(t, csp, "default-src 'none'")
		require.Contains(t, csp, "connect-src 'none'")
		require.Contains(t, csp, "frame-ancestors 'self'")
		require.NotContains(t, csp, "allow-same-origin")
		require.Equal(t, "private, no-store", res.Header.Get("Cache-Control"))
		require.Equal(t, "no-referrer", res.Header.Get("Referrer-Policy"))
		require.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
		require.Equal(t, "<h1>funnel.html</h1>", res.Body)
	})

	t.Run("FrameTopLevel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		require.Equal(t, http.StatusNotFound, frame(ctx, owner, shared.ID, 1, "").StatusCode)
		require.Equal(t, http.StatusNotFound, frame(ctx, owner, shared.ID, 1, "document").StatusCode)
	})

	t.Run("FrameNotHTML", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		require.Equal(t, http.StatusNotFound, frame(ctx, owner, shared.ID, 2, "iframe").StatusCode)
		require.Equal(t, http.StatusNotFound, frame(ctx, owner, shared.ID, 9, "iframe").StatusCode)
	})

	t.Run("RawHTMLDownloads", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := owner.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/artifacts/%s/versions/1", shared.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, `attachment; filename=funnel.html`, res.Header.Get("Content-Disposition"))
		require.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
		require.Empty(t, res.Header.Get("Content-Security-Policy"))

		_, contentType, err := owner.ChatArtifactVersionData(ctx, shared.ID, 2)
		require.NoError(t, err)
		require.Equal(t, "text/markdown", contentType)
	})

	t.Run("CoderKitIsInlined", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// A user of its own keeps this artifact out of the lists that the
		// other subtests check.
		kitOwner, kitUser := newChatProjectMember(t, admin, orgID)
		artifact, err := db.InsertChatArtifact(sysCtx, database.InsertChatArtifactParams{
			OrganizationID: orgID,
			OwnerID:        kitUser.ID,
			Title:          "Kit page",
		})
		require.NoError(t, err)
		page := `<link rel="stylesheet" href="coder-kit.css"><body class="ck-page">Kit</body>`
		_, err = db.InsertChatArtifactVersion(sysCtx, database.InsertChatArtifactVersionParams{
			ArtifactID: artifact.ID,
			Name:       "kit.html",
			Mimetype:   "text/plain",
			Data:       []byte(page),
			CreatedBy:  uuid.NullUUID{UUID: kitUser.ID, Valid: true},
		})
		require.NoError(t, err)

		get := func(path string, dest string) string {
			res, err := kitOwner.Request(ctx, http.MethodGet, path, nil, func(r *http.Request) {
				if dest != "" {
					r.Header.Set("Sec-Fetch-Dest", dest)
				}
			})
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			return string(body)
		}
		versionPath := fmt.Sprintf("/api/experimental/chats/artifacts/%s/versions/1", artifact.ID)

		framed := get(versionPath+"/frame?theme=dark", "iframe")
		require.Contains(t, framed, "<style data-coder-kit>")
		require.Contains(t, framed, `r.setAttribute("data-theme","dark")`)
		require.NotContains(t, framed, `href="coder-kit.css"`)
		require.True(t, strings.HasSuffix(framed, `<body class="ck-page">Kit</body>`))

		// The download is self-contained too, and leaves the theme to the OS.
		downloaded := get(versionPath, "")
		require.Contains(t, downloaded, "<style data-coder-kit>")
		require.NotContains(t, downloaded, `r.setAttribute("data-theme"`)

		// Agents edit the stored page, which keeps the link.
		stored, err := db.GetChatArtifactVersion(sysCtx, database.GetChatArtifactVersionParams{ArtifactID: artifact.ID, Version: 1})
		require.NoError(t, err)
		require.Equal(t, page, string(stored.Data))
	})
}

func artifactIDs(artifacts []codersdk.ChatArtifact) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(artifacts))
	for _, a := range artifacts {
		ids = append(ids, a.ID)
	}
	return ids
}
