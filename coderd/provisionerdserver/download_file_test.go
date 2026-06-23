package provisionerdserver_test

import (
	"context"
	crand "crypto/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	proto "github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// mockDownloadStream is an in-memory implementation of
// proto.DRPCProvisionerDaemon_DownloadFileStream that records every message
// sent by the handler so tests can assert on them.
type mockDownloadStream struct {
	ctx      context.Context
	messages []*sdkproto.FileUpload
}

func (m *mockDownloadStream) Send(f *sdkproto.FileUpload) error {
	m.messages = append(m.messages, f)
	return nil
}
func (m *mockDownloadStream) Context() context.Context                { return m.ctx }
func (*mockDownloadStream) CloseSend() error                          { return nil }
func (*mockDownloadStream) Close() error                              { return nil }
func (*mockDownloadStream) MsgSend(drpc.Message, drpc.Encoding) error { return nil }
func (*mockDownloadStream) MsgRecv(drpc.Message, drpc.Encoding) error { return nil }

// failure returns the error message streamed by the handler, if any.
func (m *mockDownloadStream) failure() (string, bool) {
	for _, msg := range m.messages {
		if e, ok := msg.Type.(*sdkproto.FileUpload_Error); ok {
			return e.Error.Error, true
		}
	}
	return "", false
}

// downloadedBytes reassembles the file streamed by the handler from its
// DataUpload + ChunkPiece messages.
func (m *mockDownloadStream) downloadedBytes(t *testing.T) []byte {
	t.Helper()
	var upload *sdkproto.DataUpload
	chunks := map[int32][]byte{}
	for _, msg := range m.messages {
		switch v := msg.Type.(type) {
		case *sdkproto.FileUpload_DataUpload:
			require.Nil(t, upload, "received more than one DataUpload")
			upload = v.DataUpload
		case *sdkproto.FileUpload_ChunkPiece:
			chunks[v.ChunkPiece.PieceIndex] = v.ChunkPiece.Data
		case *sdkproto.FileUpload_Error:
			t.Fatalf("unexpected error message in stream: %s", v.Error.Error)
		}
	}
	require.NotNil(t, upload, "no DataUpload message was streamed")
	var data []byte
	for i := int32(0); i < upload.Chunks; i++ {
		data = append(data, chunks[i]...)
	}
	return data
}

// insertModuleFile inserts a system-created (CreatedBy=uuid.Nil) tar file and
// links it as the cached module files of a template version in the given
// organization, returning the file.
func insertModuleFile(t *testing.T, db database.Store, orgID uuid.UUID, data []byte) database.File {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)

	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: orgID,
		CreatedBy:      user.ID,
	})
	jobID := uuid.New()
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: orgID,
		CreatedBy:      user.ID,
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		JobID:          jobID,
	})
	// Insert the file directly rather than via dbgen.File: the helper treats a
	// zero CreatedBy as "unset" and replaces it with a random UUID, but module
	// files must be system-created (CreatedBy=uuid.Nil) to match the handler's
	// metadata check.
	file, err := db.InsertFile(ctx, database.InsertFileParams{
		ID:        uuid.New(),
		Hash:      uuid.NewString(),
		CreatedAt: dbtime.Now(),
		CreatedBy: uuid.Nil,
		Mimetype:  "application/x-tar",
		Data:      data,
	})
	require.NoError(t, err)
	err = db.InsertTemplateVersionTerraformValuesByJobID(ctx, database.InsertTemplateVersionTerraformValuesByJobIDParams{
		JobID:             version.JobID,
		CachedPlan:        []byte("{}"),
		CachedModuleFiles: uuid.NullUUID{UUID: file.ID, Valid: true},
		UpdatedAt:         dbtime.Now(),
	})
	require.NoError(t, err)
	return file
}

// TestDownloadFileModuleFileTenantIsolation verifies that a provisioner daemon
// cannot download cached module archives belonging to other organizations
// (ANT-2026-22440), while still being able to download module files from its
// own organization.
func TestDownloadFileModuleFileTenantIsolation(t *testing.T) {
	t.Parallel()

	t.Run("RejectsOtherOrgModuleFile", func(t *testing.T) {
		t.Parallel()

		// The server is scoped to the default organization (org A).
		server, db, _, daemon := setup(t, false, &overrides{
			externalAuthConfigs: []*externalauth.Config{{}},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Create a module file belonging to a different organization (org B).
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		require.NotEqual(t, daemon.OrganizationID, otherOrg.ID)

		moduleData := make([]byte, sdkproto.ChunkSize*2)
		_, err := crand.Read(moduleData)
		require.NoError(t, err)
		file := insertModuleFile(t, db, otherOrg.ID, moduleData)

		stream := &mockDownloadStream{ctx: ctx}
		err = server.DownloadFile(&proto.FileRequest{
			FileId:     file.ID.String(),
			UploadType: sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES,
		}, stream)
		require.Error(t, err)
		require.ErrorContains(t, err, "is not a modules file")

		// The handler must not have streamed any of the file's contents.
		msg, ok := stream.failure()
		require.True(t, ok, "expected an error message on the stream")
		require.Contains(t, msg, "is not a modules file")
		for _, m := range stream.messages {
			switch m.Type.(type) {
			case *sdkproto.FileUpload_DataUpload, *sdkproto.FileUpload_ChunkPiece:
				t.Fatal("handler leaked file contents for another org's module file")
			}
		}
	})

	t.Run("AllowsSameOrgModuleFile", func(t *testing.T) {
		t.Parallel()

		// The server is scoped to the default organization (org A).
		server, db, _, daemon := setup(t, false, &overrides{
			externalAuthConfigs: []*externalauth.Config{{}},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		moduleData := make([]byte, sdkproto.ChunkSize*2+512)
		_, err := crand.Read(moduleData)
		require.NoError(t, err)
		file := insertModuleFile(t, db, daemon.OrganizationID, moduleData)

		stream := &mockDownloadStream{ctx: ctx}
		err = server.DownloadFile(&proto.FileRequest{
			FileId:     file.ID.String(),
			UploadType: sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES,
		}, stream)
		require.NoError(t, err)

		if msg, ok := stream.failure(); ok {
			t.Fatalf("unexpected error on stream: %s", msg)
		}
		require.Equal(t, moduleData, stream.downloadedBytes(t))
	})
}
