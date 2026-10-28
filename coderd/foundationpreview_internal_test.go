package coderd

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestFoundationSourceReferences(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		files      map[string]string
		want       []string
		unresolved bool
	}{
		{
			name: "direct_and_cross_file_alias",
			files: map[string]string{
				"main.tf": `locals { foundation = data.coder_foundation.current.outputs }`,
				"consumer.tf": `resource "example" "consumer" {
  network = data.coder_foundation.current.outputs.network
  token = local.foundation.token
  unaffected = local.foundation.unaffected
  literal = "data.coder_foundation.current.outputs.not_a_reference"
  secret = "NEVER-EXPOSE-SOURCE-SECRET"
}`,
			},
			want: []string{"network", "token"},
		},
		{
			name: "public_and_sensitive_references",
			files: map[string]string{
				"main.tf": `locals {
  public = data.coder_foundation.current.outputs
  private = data.coder_foundation.current.sensitive_outputs
}
resource "example" "consumer" {
  network = local.public.network
  token = local.private.token
  direct = data.coder_foundation.current.sensitive_outputs.token
  literal = "data.coder_foundation.current.sensitive_outputs.not_a_reference"
}`,
			},
			want: []string{"network", "token"},
		},
		{
			name: "sensitive_dynamic_reference",
			files: map[string]string{"main.tf": `locals {
  private = data.coder_foundation.current.sensitive_outputs
  value = local.private[var.output_name]
}`},
			want: []string{}, unresolved: true,
		},
		{
			name:  "sensitive_whole_object",
			files: map[string]string{"main.tf": `output "all" { value = data.coder_foundation.current.sensitive_outputs }`},
			want:  []string{}, unresolved: true,
		},
		{
			name: "dynamic_reference",
			files: map[string]string{"main.tf": `locals {
  foundation = data.coder_foundation.current.outputs
  value = local.foundation[var.output_name]
}`},
			want: []string{}, unresolved: true,
		},
		{
			name:  "whole_object",
			files: map[string]string{"main.tf": `output "all" { value = data.coder_foundation.current.outputs }`},
			want:  []string{}, unresolved: true,
		},
		{
			name:  "alias_not_proven",
			files: map[string]string{"main.tf": `output "network" { value = local.foundation.network }`},
			want:  []string{}, unresolved: true,
		},
		{
			name: "unrelated_literal_and_comment",
			files: map[string]string{"main.tf": `# data.coder_foundation.current.outputs.token
locals { unrelated = "data.coder_foundation.current.outputs.network" }`},
			want: []string{},
		},
		{
			name: "module_analysis_is_incomplete",
			files: map[string]string{"main.tf": `module "consumer" {
  source = "./module"
  network = data.coder_foundation.current.outputs.network
}`, "module/main.tf": `variable "network" { type = string }`},
			want: []string{"network"}, unresolved: true,
		},
		{
			name:  "json_configuration_unresolved",
			files: map[string]string{"main.tf.json": `{"locals":{"network":"${data.coder_foundation.current.outputs.network}"}}`},
			want:  []string{}, unresolved: true,
		},
		{
			name:       "invalid_source_unresolved",
			files:      map[string]string{"main.tf": `NEVER-EXPOSE-SOURCE-SECRET = {`},
			unresolved: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			archive := foundationPreviewTestArchive(t, test.files)
			names, unresolved := foundationSourceReferences(archive, map[string]bool{"network": true, "token": true, "not_a_reference": true})
			require.Equal(t, test.want, names)
			require.Equal(t, test.unresolved, unresolved)
			data, err := json.Marshal(names)
			require.NoError(t, err)
			require.NotContains(t, string(data), "NEVER-EXPOSE")
		})
	}
}

func TestFoundationPreviewUnsafeArchive(t *testing.T) {
	t.Parallel()
	archive := foundationPreviewTestArchive(t, map[string]string{"../main.tf": `output "token" { value = data.coder_foundation.current.outputs.token }`})
	names, unresolved := foundationSourceReferences(archive, map[string]bool{"token": true})
	require.Empty(t, names)
	require.True(t, unresolved)
}

func TestDecodeFoundationPlanPreview(t *testing.T) {
	t.Parallel()
	require.Nil(t, decodeFoundationPlanPreview(nil))
	unavailable := decodeFoundationPlanPreview([]byte(`not JSON NEVER-EXPOSE`))
	require.False(t, unavailable.Available)
	preview := decodeFoundationPlanPreview([]byte(`{"available":true,"output_changes":[],"advisory":"NEVER-EXPOSE","raw_plan":"NEVER-EXPOSE","consumer_references":[{"output_names":["NEVER-EXPOSE"]}]}`))
	require.True(t, preview.Available)
	require.Empty(t, preview.ConsumerReferences)
	data, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(data), "NEVER-EXPOSE")
}

func TestEnrichFoundationPreviews(t *testing.T) {
	t.Parallel()
	db := dbmock.NewMockStore(gomock.NewController(t))
	foundationID, versionID, missingVersionID := uuid.New(), uuid.New(), uuid.New()
	fileID, missingFileID := uuid.New(), uuid.New()
	db.EXPECT().GetFoundationConsumerTemplateVersions(gomock.Any(), foundationID).Return([]database.GetFoundationConsumerTemplateVersionsRow{
		{ID: versionID, FileID: fileID}, {ID: missingVersionID, FileID: missingFileID},
	}, nil)
	db.EXPECT().GetFileByID(gomock.Any(), fileID).Return(database.File{Data: foundationPreviewTestArchive(t, map[string]string{
		"main.tf": `output "consumer" { value = data.coder_foundation.current.outputs.network }
output "secret_consumer" {
  value = data.coder_foundation.current.sensitive_outputs.token
  sensitive = true
}`,
	})}, nil)
	db.EXPECT().GetFileByID(gomock.Any(), missingFileID).Return(database.File{}, errors.New("NEVER-EXPOSE-SOURCE-ERROR"))
	preview := provisionersdk.FoundationPlanPreview([]byte(`{"format_version":"1.2","output_changes":{"network":{"actions":["delete"]},"token":{"actions":["delete"],"before_sensitive":true}}}`))
	other := provisionersdk.FoundationPlanPreview([]byte(`{"format_version":"1.2","output_changes":{"unconsumed":{"actions":["create"]}}}`))
	runs := []codersdk.TemplateResourceRun{{Preview: &preview}, {Preview: &other}, {}}
	require.NoError(t, enrichFoundationPreviews(t.Context(), db, foundationID, runs))
	require.Equal(t, []codersdk.FoundationConsumerReference{
		{TemplateVersionID: versionID, OutputNames: []string{"network", "token"}},
		{TemplateVersionID: missingVersionID, OutputNames: []string{}, Unresolved: true},
	}, preview.ConsumerReferences)
	require.Equal(t, []codersdk.FoundationConsumerReference{
		{TemplateVersionID: missingVersionID, OutputNames: []string{}, Unresolved: true},
	}, other.ConsumerReferences)
	data, err := json.Marshal(runs)
	require.NoError(t, err)
	require.NotContains(t, string(data), "NEVER-EXPOSE")
	// No discovery or source reads are needed when no output changes are known.
	require.NoError(t, enrichFoundationPreviews(t.Context(), db, foundationID, []codersdk.TemplateResourceRun{{}}))
}

func foundationPreviewTestArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	for name, content := range files {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(content))}))
		_, err := writer.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buf.Bytes()
}
