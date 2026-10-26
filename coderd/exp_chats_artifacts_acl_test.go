package coderd_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatArtifactSharing(t *testing.T) {
	t.Parallel()

	mAudit := audit.NewMock()
	admin, api := newChatProjectClientWithAPI(t, func(opts *coderdtest.Options) {
		opts.Auditor = mAudit
	})
	db := api.Database
	firstUser := coderdtest.CreateFirstUser(t, admin.Client)
	model := createChatModel(t, admin)
	orgID := firstUser.OrganizationID

	owner, ownerUser := newChatProjectMember(t, admin, orgID)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           ownerUser.ID,
		LastModelConfigID: model.ID,
		Title:             "Publishing chat",
	})

	// newArtifact seeds an artifact with one HTML version, as a chat of
	// the owner publishes it.
	newArtifact := func(t *testing.T, projectID uuid.NullUUID) database.ChatArtifact {
		t.Helper()
		//nolint:gocritic // The test seeds rows as chatd, which publishes artifacts.
		chatdCtx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
		artifact, err := db.InsertChatArtifact(chatdCtx, database.InsertChatArtifactParams{
			OrganizationID: orgID,
			OwnerID:        ownerUser.ID,
			ProjectID:      projectID,
			Title:          "Shared report",
		})
		require.NoError(t, err)
		_, err = db.InsertChatArtifactVersion(chatdCtx, database.InsertChatArtifactVersionParams{
			ArtifactID: artifact.ID,
			Name:       "report.html",
			Mimetype:   "text/plain",
			Data:       []byte("<h1>report</h1>"),
			CreatedBy:  uuid.NullUUID{UUID: ownerUser.ID, Valid: true},
			ChatID:     uuid.NullUUID{UUID: chat.ID, Valid: true},
		})
		require.NoError(t, err)
		return artifact
	}
	share := func(t *testing.T, client *codersdk.ExperimentalClient, artifactID uuid.UUID, req codersdk.UpdateChatACL) {
		t.Helper()
		require.NoError(t, client.UpdateChatArtifactACL(testutil.Context(t, testutil.WaitLong), artifactID, req))
	}
	read := func(id uuid.UUID) codersdk.UpdateChatACL {
		return codersdk.UpdateChatACL{UserRoles: map[string]codersdk.ChatRole{id.String(): codersdk.ChatRoleRead}}
	}
	// canPublish reports whether the user may add a version, which is what
	// the dbauthz check of a publish asks.
	canPublish := func(t *testing.T, user codersdk.User, artifactID uuid.UUID) bool {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		subject := coderdtest.AuthzUserSubjectWithDB(ctx, t, db, user)
		_, err := db.InsertChatArtifactVersion(dbauthz.As(ctx, subject), database.InsertChatArtifactVersionParams{
			ArtifactID:  artifactID,
			BaseVersion: 1,
			Name:        "report.html",
			Mimetype:    "text/plain",
			Data:        []byte("<h1>v2</h1>"),
		})
		if dbauthz.IsNotAuthorizedError(err) {
			return false
		}
		require.NoError(t, err)
		return true
	}
	requireListed := func(t *testing.T, client *codersdk.ExperimentalClient, artifactID uuid.UUID, listed bool) {
		t.Helper()
		list, err := client.ListChatArtifacts(testutil.Context(t, testutil.WaitLong))
		require.NoError(t, err)
		if listed {
			require.Contains(t, artifactIDs(list), artifactID)
		} else {
			require.NotContains(t, artifactIDs(list), artifactID)
		}
	}
	frameStatus := func(t *testing.T, client *codersdk.ExperimentalClient, artifactID uuid.UUID) int {
		t.Helper()
		res, err := client.Request(testutil.Context(t, testutil.WaitLong), http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/artifacts/%s/versions/1/frame", artifactID), nil,
			func(r *http.Request) { r.Header.Set("Sec-Fetch-Dest", "iframe") })
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}

	// requireReadable checks the artifact page, the version bytes, the frame
	// and the list for one reader.
	requireReadable := func(t *testing.T, client *codersdk.ExperimentalClient, artifactID uuid.UUID, readable bool) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, getErr := client.GetChatArtifact(ctx, artifactID)
		_, _, dataErr := client.ChatArtifactVersionData(ctx, artifactID, 1)
		if readable {
			require.NoError(t, getErr)
			require.NoError(t, dataErr)
			require.Equal(t, http.StatusOK, frameStatus(t, client, artifactID))
		} else {
			requireSDKError(t, getErr, http.StatusNotFound)
			requireSDKError(t, dataErr, http.StatusNotFound)
			require.Equal(t, http.StatusNotFound, frameStatus(t, client, artifactID))
		}
		requireListed(t, client, artifactID, readable)
	}
	storedUserACL := func(t *testing.T, artifactID uuid.UUID) database.ChatACL {
		t.Helper()
		//nolint:gocritic // The test reads the stored ACL directly.
		row, err := db.GetChatArtifactByID(dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong)), artifactID)
		require.NoError(t, err)
		return row.ChatArtifact.UserACL
	}

	t.Run("User", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		artifact := newArtifact(t, uuid.NullUUID{})
		sharee, shareeUser := newChatProjectMember(t, admin, orgID)
		other, _ := newChatProjectMember(t, admin, orgID)

		_, err := sharee.GetChatArtifact(ctx, artifact.ID)
		requireSDKError(t, err, http.StatusNotFound)

		share(t, owner, artifact.ID, read(shareeUser.ID))
		require.True(t, mAudit.Contains(t, database.AuditLog{
			Action:         database.AuditActionWrite,
			ResourceType:   database.ResourceTypeChatArtifact,
			ResourceID:     artifact.ID,
			ResourceTarget: artifact.Title,
			UserID:         ownerUser.ID,
			OrganizationID: orgID,
			StatusCode:     http.StatusNoContent,
		}))

		acl, err := owner.ChatArtifactACL(ctx, artifact.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Groups)
		require.Len(t, acl.Users, 1)
		require.Equal(t, shareeUser.ID, acl.Users[0].ID)
		require.Equal(t, codersdk.ChatRoleRead, acl.Users[0].Role)

		got, err := sharee.GetChatArtifact(ctx, artifact.ID)
		require.NoError(t, err)
		require.Len(t, got.Versions, 1)
		require.Equal(t, &chat.ID, got.Versions[0].ChatID)
		require.Empty(t, got.Versions[0].ChatTitle, "a sharee does not see the chat title")
		data, _, err := sharee.ChatArtifactVersionData(ctx, artifact.ID, 1)
		require.NoError(t, err)
		require.Equal(t, "<h1>report</h1>", string(data))
		require.Equal(t, http.StatusOK, frameStatus(t, sharee, artifact.ID))
		requireListed(t, sharee, artifact.ID, true)

		// Sharing an artifact does not share the chat that published it.
		_, err = sharee.GetChat(ctx, chat.ID)
		requireSDKError(t, err, http.StatusNotFound)
		// A sharee reads only: no publishing and no sharing.
		require.False(t, canPublish(t, shareeUser, artifact.ID))
		err = sharee.UpdateChatArtifactACL(ctx, artifact.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{uuid.NewString(): codersdk.ChatRoleRead},
		})
		requireSDKError(t, err, http.StatusForbidden)

		// Other members still get 404 everywhere.
		_, err = other.GetChatArtifact(ctx, artifact.ID)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = other.ChatArtifactACL(ctx, artifact.ID)
		requireSDKError(t, err, http.StatusNotFound)
		err = other.UpdateChatArtifactACL(ctx, artifact.ID, read(uuid.New()))
		requireSDKError(t, err, http.StatusNotFound)
		requireListed(t, other, artifact.ID, false)

		// The empty role removes the entry and revokes access.
		share(t, owner, artifact.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{shareeUser.ID.String(): codersdk.ChatRoleDeleted},
		})
		_, err = sharee.GetChatArtifact(ctx, artifact.ID)
		requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, http.StatusNotFound, frameStatus(t, sharee, artifact.ID))
		requireListed(t, sharee, artifact.ID, false)
		acl, err = owner.ChatArtifactACL(ctx, artifact.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
	})

	t.Run("GroupAndEveryone", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		groupArtifact := newArtifact(t, uuid.NullUUID{})
		orgArtifact := newArtifact(t, uuid.NullUUID{})
		groupMember, groupMemberUser := newChatProjectMember(t, admin, orgID)
		nonMember, _ := newChatProjectMember(t, admin, orgID)
		group := dbgen.Group(t, db, database.Group{OrganizationID: orgID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: groupMemberUser.ID})

		share(t, owner, groupArtifact.ID, codersdk.UpdateChatACL{
			GroupRoles: map[string]codersdk.ChatRole{group.ID.String(): codersdk.ChatRoleRead},
		})
		// The Everyone group's ID is the organization ID.
		share(t, owner, orgArtifact.ID, codersdk.UpdateChatACL{
			GroupRoles: map[string]codersdk.ChatRole{orgID.String(): codersdk.ChatRoleRead},
		})

		acl, err := owner.ChatArtifactACL(ctx, orgArtifact.ID)
		require.NoError(t, err)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, orgID, acl.Groups[0].ID)
		require.Equal(t, database.EveryoneGroup, acl.Groups[0].Name)
		require.Equal(t, codersdk.ChatRoleRead, acl.Groups[0].Role)

		requireReadable(t, groupMember, groupArtifact.ID, true)
		requireListed(t, groupMember, orgArtifact.ID, true)
		_, err = nonMember.GetChatArtifact(ctx, groupArtifact.ID)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = nonMember.GetChatArtifact(ctx, orgArtifact.ID)
		require.NoError(t, err)
		requireListed(t, nonMember, groupArtifact.ID, false)
		requireListed(t, nonMember, orgArtifact.ID, true)

		// Members of another organization do not reach the Everyone grant.
		otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
		outsiderRaw, _ := coderdtest.CreateAnotherUser(t, admin.Client, otherOrganization.ID)
		outsider := codersdk.NewExperimentalClient(outsiderRaw)
		_, err = outsider.GetChatArtifact(ctx, orgArtifact.ID)
		requireSDKError(t, err, http.StatusNotFound)
		requireListed(t, outsider, orgArtifact.ID, false)

		// Leaving the group removes the group grant.
		//nolint:gocritic // The test changes group membership directly.
		err = db.DeleteGroupMemberFromGroup(dbauthz.AsSystemRestricted(ctx), database.DeleteGroupMemberFromGroupParams{
			UserID:  groupMemberUser.ID,
			GroupID: group.ID,
		})
		require.NoError(t, err)
		requireReadable(t, groupMember, groupArtifact.ID, false)
		requireListed(t, groupMember, orgArtifact.ID, true)
	})

	t.Run("LeavingOrganizationDropsGrant", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		artifact := newArtifact(t, uuid.NullUUID{})
		sharee, shareeUser := newChatProjectMember(t, admin, orgID)
		share(t, owner, artifact.ID, read(shareeUser.ID))
		requireReadable(t, sharee, artifact.ID, true)

		//nolint:gocritic // The test changes organization membership directly.
		sysCtx := dbauthz.AsSystemRestricted(ctx)
		err := db.DeleteOrganizationMember(sysCtx, database.DeleteOrganizationMemberParams{OrganizationID: orgID, UserID: shareeUser.ID})
		require.NoError(t, err)
		require.NotContains(t, storedUserACL(t, artifact.ID), shareeUser.ID.String(), "the trigger drops the grant")

		// Joining again does not restore access, from the page or the list.
		dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: orgID, UserID: shareeUser.ID})
		requireReadable(t, sharee, artifact.ID, false)

		// A new share works again, so the 404 above came from the dropped
		// grant.
		share(t, owner, artifact.ID, read(shareeUser.ID))
		requireReadable(t, sharee, artifact.ID, true)
	})

	t.Run("CanonicalUUIDKeys", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		artifact := newArtifact(t, uuid.NullUUID{})
		sharee, shareeUser := newChatProjectMember(t, admin, orgID)
		share(t, owner, artifact.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{strings.ToUpper(shareeUser.ID.String()): codersdk.ChatRoleRead},
		})
		require.Equal(t, database.ChatACL{
			shareeUser.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		}, storedUserACL(t, artifact.ID))
		requireReadable(t, sharee, artifact.ID, true)

		// The same ID in two spellings is refused.
		err := owner.UpdateChatArtifactACL(ctx, artifact.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{
				strings.ToUpper(shareeUser.ID.String()): codersdk.ChatRoleRead,
				shareeUser.ID.String():                  codersdk.ChatRoleDeleted,
			},
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("OrgAdminShares", func(t *testing.T) {
		t.Parallel()
		artifact := newArtifact(t, uuid.NullUUID{})
		orgAdminRaw, orgAdminUser := coderdtest.CreateAnotherUser(t, admin.Client, orgID, rbac.ScopedRoleOrgAdmin(orgID))
		orgAdmin := codersdk.NewExperimentalClient(orgAdminRaw)
		sharee, shareeUser := newChatProjectMember(t, admin, orgID)

		share(t, orgAdmin, artifact.ID, read(shareeUser.ID))
		requireReadable(t, sharee, artifact.ID, true)
		require.True(t, mAudit.Contains(t, database.AuditLog{
			Action:       database.AuditActionWrite,
			ResourceType: database.ResourceTypeChatArtifact,
			ResourceID:   artifact.ID,
			UserID:       orgAdminUser.ID,
		}))
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		artifact := newArtifact(t, uuid.NullUUID{})
		otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
		_, outsiderUser := coderdtest.CreateAnotherUser(t, admin.Client, otherOrganization.ID)
		foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrganization.ID})
		_, member := newChatProjectMember(t, admin, orgID)

		for name, req := range map[string]codersdk.UpdateChatACL{
			"NotOrgMember": read(outsiderUser.ID),
			"ForeignGroup": {GroupRoles: map[string]codersdk.ChatRole{foreignGroup.ID.String(): codersdk.ChatRoleRead}},
			"Owner":        read(ownerUser.ID),
			"BadUUID":      {UserRoles: map[string]codersdk.ChatRole{"not-a-uuid": codersdk.ChatRoleRead}},
			"BadRole":      {UserRoles: map[string]codersdk.ChatRole{member.ID.String(): "admin"}},
		} {
			err := owner.UpdateChatArtifactACL(ctx, artifact.ID, req)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, name)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), name)
		}
		// An admin who shares cannot add itself.
		err := admin.UpdateChatArtifactACL(ctx, artifact.ID, read(firstUser.UserID))
		requireSDKError(t, err, http.StatusBadRequest)

		acl, err := owner.ChatArtifactACL(ctx, artifact.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
		require.Empty(t, acl.Groups)
	})

	t.Run("ProjectUnchanged", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		projectMember, projectMemberUser := newChatProjectMember(t, admin, orgID)
		sharee, shareeUser := newChatProjectMember(t, admin, orgID)
		project := createChatProject(t, owner, orgID, "Artifact sharing project")
		err := owner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{projectMemberUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		artifact := newArtifact(t, uuid.NullUUID{UUID: project.ID, Valid: true})

		share(t, owner, artifact.ID, read(shareeUser.ID))

		// The project member keeps read and publish; the sharee reads only.
		memberView, err := projectMember.GetChatArtifact(ctx, artifact.ID)
		require.NoError(t, err)
		require.Equal(t, &project.ID, memberView.ProjectID)
		require.Equal(t, project.Name, memberView.ProjectName)
		requireListed(t, projectMember, artifact.ID, true)
		require.Equal(t, &project.ID, listedArtifact(t, projectMember, artifact.ID).ProjectID)
		require.False(t, canPublish(t, shareeUser, artifact.ID))
		require.True(t, canPublish(t, projectMemberUser, artifact.ID))
		// A direct sharee outside the project does not learn the project.
		shareeView, err := sharee.GetChatArtifact(ctx, artifact.ID)
		require.NoError(t, err)
		require.Nil(t, shareeView.ProjectID)
		require.Empty(t, shareeView.ProjectName)
		listed := listedArtifact(t, sharee, artifact.ID)
		require.Nil(t, listed.ProjectID)
		require.Empty(t, listed.ProjectName)
		ownerView, err := owner.GetChatArtifact(ctx, artifact.ID)
		require.NoError(t, err)
		require.Equal(t, &project.ID, ownerView.ProjectID)
		// Sharing an artifact does not share its project, and project
		// members cannot reshare the artifact.
		_, err = sharee.GetChatProject(ctx, orgID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)
		err = projectMember.UpdateChatArtifactACL(ctx, artifact.ID, read(uuid.New()))
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("ProjectOwnerCannotShare", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		projectOwner, _ := newChatProjectMember(t, admin, orgID)
		_, shareeUser := newChatProjectMember(t, admin, orgID)
		project := createChatProject(t, projectOwner, orgID, "Someone else's project")
		err := projectOwner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{ownerUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		artifact := newArtifact(t, uuid.NullUUID{UUID: project.ID, Valid: true})

		// The project owner reads the artifact and sees its project, but
		// only the artifact owner shares it.
		got, err := projectOwner.GetChatArtifact(ctx, artifact.ID)
		require.NoError(t, err)
		require.Equal(t, &project.ID, got.ProjectID)
		err = projectOwner.UpdateChatArtifactACL(ctx, artifact.ID, read(shareeUser.ID))
		requireSDKError(t, err, http.StatusForbidden)
		require.Empty(t, storedUserACL(t, artifact.ID))
	})
}

func listedArtifact(t *testing.T, client *codersdk.ExperimentalClient, artifactID uuid.UUID) codersdk.ChatArtifact {
	t.Helper()
	list, err := client.ListChatArtifacts(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err)
	for _, a := range list {
		if a.ID == artifactID {
			return a
		}
	}
	require.FailNow(t, "artifact not listed", artifactID)
	return codersdk.ChatArtifact{}
}
