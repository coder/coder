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

func TarBytesToTree(destination string, raw []byte) error {
	err := os.Mkdir(destination, 0700)

	archiveReader := tar.NewReader(bytes.NewReader(raw))
	hdr, err := archiveReader.Next()
	for err != io.EOF {
		if hdr == nil { //	some blog post indicated this could happen sometimes
			continue
		}
		filename := filepath.FromSlash(fmt.Sprintf("%s/%s", destination, hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.Mkdir(filename, 0700)
			if err != nil {
				return xerrors.Errorf("unable to check out template directory: %w", err)
			}
		case tar.TypeReg:
			f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				return xerrors.Errorf("unable to create template file: %w", err)
			}

			_, err = io.Copy(f, archiveReader)
			if err != nil {
				f.Close() // is this necessary?
				return xerrors.Errorf("error writing template file: %w", err)
			}
			f.Close()
		}

		hdr, err = archiveReader.Next()
	}
	return nil
}

func templateCheckout() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkout <template name> [destination]",
		Short: "Download the named template's contents into a subdirectory.",
		Long:  "Download the named template's contents and extract them into a subdirectory named according to the destination or <template name> if no destination is specified.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateName := args[0]
			var destination string
			if len(args) > 1 {
				destination = args[1]
			} else {
				destination = templateName
			}

			raw, err := fetchTemplateArchiveBytes(cmd, templateName)
			if err != nil {
				return err
			}

			// Stat the destination to ensure nothing exists already.
			stat, err := os.Stat(destination)
			if stat != nil {
				return xerrors.Errorf("template file/directory already exists: %s", destination)
			}

			return TarBytesToTree(destination, raw)
		},
	}

	cliui.AllowSkipPrompt(cmd)

	return cmd
}
