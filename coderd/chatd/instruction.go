package chatd

import (
	"context"
	"io"
	"net/http"
	"path"
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

// readHomeInstructionFile reads the ~/.coder/AGENTS.md file from the
// workspace agent's home directory.
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

	return readInstructionFile(ctx, conn, filePath)
}

// readInstructionFile reads and sanitizes an instruction file at the
// given absolute path.
func readInstructionFile(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	filePath string,
) (content string, sourcePath string, truncated bool, err error) {
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
		return "", "", false, xerrors.Errorf("read instruction file: %w", err)
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", "", false, xerrors.Errorf("read instruction bytes: %w", err)
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

// formatSystemInstructions builds the <workspace-context> block from
// agent metadata and zero or more instruction file sections.
func formatSystemInstructions(
	operatingSystem, directory string,
	sections []instructionFileSection,
) string {
	hasSections := false
	for _, s := range sections {
		if s.content != "" {
			hasSections = true
			break
		}
	}
	if !hasSections && operatingSystem == "" && directory == "" {
		return ""
	}

	var b strings.Builder
	_, _ = b.WriteString("<workspace-context>\n")
	if operatingSystem != "" {
		_, _ = b.WriteString("Operating System: ")
		_, _ = b.WriteString(operatingSystem)
		_, _ = b.WriteString("\n")
	}
	if directory != "" {
		_, _ = b.WriteString("Working Directory: ")
		_, _ = b.WriteString(directory)
		_, _ = b.WriteString("\n")
	}
	for _, s := range sections {
		if s.content == "" {
			continue
		}
		_, _ = b.WriteString("\nSource: ")
		_, _ = b.WriteString(s.source)
		if s.truncated {
			_, _ = b.WriteString(" (truncated to 64KiB)")
		}
		_, _ = b.WriteString("\n")
		_, _ = b.WriteString(s.content)
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString("</workspace-context>")
	return b.String()
}

// instructionFileSection is a single instruction file's content and
// source path for rendering inside <workspace-context>.
type instructionFileSection struct {
	content   string
	source    string
	truncated bool
}

// pwdInstructionFilePath returns the absolute path to the AGENTS.md
// file in the given working directory, or empty if directory is empty.
func pwdInstructionFilePath(directory string) string {
	if directory == "" {
		return ""
	}
	return path.Join(directory, coderHomeInstructionFile)
}

func isCodersdkStatusCode(err error, statusCode int) bool {
	var sdkErr *codersdk.Error
	if !xerrors.As(err, &sdkErr) {
		return false
	}
	return sdkErr.StatusCode() == statusCode
}
