package cli

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
)

func tarBytesToTree(templateName string, raw []byte) error {
	err := os.Mkdir(templateName, 0700)

	archiveReader := tar.NewReader(bytes.NewReader(raw))
	hdr, err := archiveReader.Next()
	for err != io.EOF {
		if hdr == nil { //	some blog post indicated this could happen sometimes
			continue
		}
		filename := filepath.FromSlash(fmt.Sprintf("%s/%s", templateName, hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.Mkdir(filename, 0700)
			if err != nil {
				return xerrors.Errorf("exporting archived directory: %w", err)
			}
		case tar.TypeReg:
			f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				return xerrors.Errorf("unable to create archived file: %w", err)
			}

			_, err = io.Copy(f, archiveReader)
			if err != nil {
				f.Close() // is this necessary?
				return xerrors.Errorf("error writing archive file: %w", err)
			}
			f.Close()
		}

		hdr, err = archiveReader.Next()
	}
	return nil
}

func templateExport() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <template name>",
		Short: "Create download the named template's contents and extract them into a subdirectory named <template name>.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				templateName = args[0]
			)

			raw, err := fetchTemplateArchiveBytes(cmd, templateName)
			if err != nil {
				return err
			}

			// Stat the destination to ensure nothing exists already.
			stat, err := os.Stat(templateName)
			if stat != nil {
				return xerrors.Errorf("template file/directory already exists: %s", err)
			}

			return tarBytesToTree(templateName, raw)
		},
	}

	cliui.AllowSkipPrompt(cmd)

	return cmd
}
