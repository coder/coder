package cli_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli"
)

func writeTarArchiveFileEntry(tw *tar.Writer, filename string, content []byte) error {
	hdr := &tar.Header{
		Name: filename,
		Mode: 0600,
		Size: int64(len(content)),
	}

	err := tw.WriteHeader(hdr)
	if err != nil {
		return err
	}
	_, err = tw.Write([]byte(content))
	if err != nil {
		return err
	}
	return nil
}

func TestTemplateCheckoutExtractArchive(t *testing.T) {
	t.Parallel()

	t.Run("TestTemplateCheckoutExtractArchive", func(t *testing.T) {
		subdirName := "subtle"
		expectedNames := []string{
			"rat-one", "rat-two", fmt.Sprintf("%s/trouble", subdirName),
		}
		expectedContents := []string{
			"{ 'tar' : 'but is it art?' }\n", "{ 'zap' : 'brannigan' }\n", "{ 'with' : 'a T' }\n",
		}

		t.Parallel()

		var bb bytes.Buffer
		w := bufio.NewWriter(&bb)
		tw := tar.NewWriter(w)

		hdr := &tar.Header{
			Name:     subdirName,
			Mode:     0700,
			Typeflag: tar.TypeDir,
		}
		err := tw.WriteHeader(hdr)
		if err != nil {
			t.Fatalf(err.Error())
		}

		for i := 0; i < len(expectedNames); i++ {
			err = writeTarArchiveFileEntry(tw, expectedNames[i], []byte(expectedContents[i]))
			if err != nil {
				t.Fatalf(err.Error())
			}
		}

		tw.Close()

		dirname, err := ioutil.TempDir("", "template-checkout-test")
		if err != nil {
			t.Fatalf(err.Error())
		}

		cli.TarBytesToTree(dirname, bb.Bytes())

		for i := 0; i < len(expectedNames); i++ {
			filename := fmt.Sprintf("%s/%s", dirname, expectedNames[i])
			actualContents, err := ioutil.ReadFile(filename)

			if err != nil {
				t.Fatalf(err.Error())
			}

			require.Equal(t, expectedContents[i], string(actualContents))
		}
	})
}
