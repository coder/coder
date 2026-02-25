package chatd

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	coderHomeInstructionDir  = ".coder"
	coderHomeInstructionFile = "AGENTS.md"
	maxInstructionFileBytes  = 64 * 1024
)

var markdownCommentPattern = regexp.MustCompile(`<!--[\s\S]*?-->`)

func readHomeInstructionFile(
	ctx context.Context,
	conn workspacesdk.AgentConn,
) (content string, sourcePath string, truncated bool, err error) {
	if conn == nil {
		return "", "", false, nil
	}

	coderDir, err := conn.LS(ctx, "", workspacesdk.LSRequest{
		Path:       []string{coderHomeInstructionDir},
		Relativity: workspacesdk.LSRelativityHome,
	})
	if err != nil {
		if isCodersdkStatusCode(err, http.StatusNotFound) {
			return "", "", false, nil
		}
		return "", "", false, xerrors.Errorf("list home instruction directory: %w", err)
	}

	var filePath string
	for _, entry := range coderDir.Contents {
		if entry.IsDir {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(entry.Name), coderHomeInstructionFile) {
			filePath = strings.TrimSpace(entry.AbsolutePathString)
			break
		}
	}
	if filePath == "" {
		return "", "", false, nil
	}

	reader, _, err := conn.ReadFile(
		ctx,
		filePath,
		0,
		maxInstructionFileBytes+1,
	)
	if err != nil {
		if isCodersdkStatusCode(err, http.StatusNotFound) {
			return "", "", false, nil
		}
		return "", "", false, xerrors.Errorf("read home instruction file: %w", err)
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", "", false, xerrors.Errorf("read home instruction bytes: %w", err)
	}

	truncated = int64(len(raw)) > maxInstructionFileBytes
	if truncated {
		raw = raw[:maxInstructionFileBytes]
	}

	content = sanitizeInstructionMarkdown(string(raw))
	if content == "" {
		return "", "", truncated, nil
	}

	return content, filePath, truncated, nil
}

func sanitizeInstructionMarkdown(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = markdownCommentPattern.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

//nolint:revive // Boolean indicates content was truncated.
func formatHomeInstruction(content string, sourcePath string, truncated bool) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		sourcePath = "~/.coder/AGENTS.md"
	}

	var b strings.Builder
	_, _ = b.WriteString("<coder-home-instructions>\n")
	_, _ = b.WriteString("Source: ")
	_, _ = b.WriteString(sourcePath)
	if truncated {
		_, _ = b.WriteString(" (truncated to 64KiB)")
	}
	_, _ = b.WriteString("\n\n")
	_, _ = b.WriteString(content)
	_, _ = b.WriteString("\n</coder-home-instructions>")
	return b.String()
}

func isCodersdkStatusCode(err error, statusCode int) bool {
	var sdkErr *codersdk.Error
	if !xerrors.As(err, &sdkErr) {
		return false
	}
	return sdkErr.StatusCode() == statusCode
}
