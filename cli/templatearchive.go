package cli

import (
	"bytes"
	"io"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/sandbox"
	"github.com/coder/coder/v2/provisionersdk"
)

func archiveTemplateDirectory(w io.Writer, logger slog.Logger, directory string, provisioner codersdk.ProvisionerType) error {
	if provisioner != codersdk.ProvisionerTypeSandbox {
		return provisionersdk.Tar(w, logger, directory, provisionersdk.TemplateArchiveLimit)
	}
	// Validate the bounded archive itself so symlinked or oversized manifests
	// cannot differ between CLI validation and what the server receives.
	var archive bytes.Buffer
	if err := provisionersdk.TarDirectory(&archive, logger, directory, provisionersdk.TemplateArchiveLimit); err != nil {
		return err
	}
	if _, err := sandbox.ReadManifestArchive(archive.Bytes()); err != nil {
		return xerrors.Errorf("invalid sandbox template: %w", err)
	}
	_, err := io.Copy(w, &archive)
	return err
}
