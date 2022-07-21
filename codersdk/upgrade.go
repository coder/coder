package codersdk

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/xerrors"
)

func (c *Client) FetchCLI(ctx context.Context, os, arch string) (io.ReadCloser, error) {
	binName := fmt.Sprintf("coder-%s-%s", os, arch)
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/bin/%s", binName), nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, readBodyAsError(res)
	}

	return res.Body, nil
}

type CLIChecksums map[string]string

func (c CLIChecksums) Checksum(os, arch string) string {
	key := fmt.Sprintf("coder-%s-%s", os, arch)
	if os == "windows" {
		key += ".exe"
	}

	return c[key]
}

func (c *Client) CLIChecksums(ctx context.Context) (CLIChecksums, error) {
	res, err := c.Request(ctx, http.MethodGet, "/bin/coder.sha1", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	return parseChecksumFile(res.Body)
}

func parseChecksumFile(rd io.Reader) (CLIChecksums, error) {
	var (
		scan = bufio.NewScanner(rd)
		sums = map[string]string{}
	)

	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 2 {
			return nil, xerrors.Errorf("malformed checksum file")
		}

		var (
			binary   = fields[1]
			checksum = fields[0]
		)

		sums[binary] = checksum
	}

	return sums, nil
}
