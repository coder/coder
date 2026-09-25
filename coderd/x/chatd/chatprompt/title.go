package chatprompt

import (
	"strings"

	"github.com/google/uuid"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
)

// syntheticPasteTitleBudget caps, in runes, how much of a pasted-text
// attachment feeds title generation. It is far smaller than
// syntheticPasteInlineBudget because it only seeds a short title, not
// the model prompt.
const syntheticPasteTitleBudget = 16 * 1024

// TitlePasteBytePrefix caps, in bytes, how much of a pasted-text blob
// feeds title derivation: four bytes per rune (the UTF-8 maximum)
// covers syntheticPasteTitleBudget runes. Database callers pass it to
// GetChatFileDataPrefixesByIDs to bound the fetch itself.
const TitlePasteBytePrefix = 4 * syntheticPasteTitleBudget

// TitlePasteText converts pasted-text content to TitleText input,
// copying at most TitlePasteBytePrefix bytes. Every caller that
// builds a pasteText map must use it so all derivation paths feed
// TitleText identical strings.
func TitlePasteText(data []byte) string {
	return string(data[:min(len(data), TitlePasteBytePrefix)])
}

// TitleText derives title-generation input from message parts. Text
// and file-reference parts are joined in part order. When they yield
// nothing, the content of synthetic pasted-text attachments is used
// instead, looked up in pasteText by file ID and truncated to
// syntheticPasteTitleBudget runes per file.
//
// The chat-creation fallback title and both title-generation paths
// must derive their input through this function: auto-titling only
// proceeds when the current title equals FallbackTitle of this exact
// string, so a drift between derivations silently disables it.
func TitleText(parts []codersdk.ChatMessagePart, pasteText map[uuid.UUID]string) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeText:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			texts = append(texts, text)
		case codersdk.ChatMessagePartTypeFileReference:
			texts = append(texts, fileReferencePartToText(part))
		}
	}
	if joined := strings.TrimSpace(strings.Join(texts, " ")); joined != "" {
		return joined
	}

	pastes := make([]string, 0, len(pasteText))
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeFile || !part.FileID.Valid {
			continue
		}
		content := strings.TrimSpace(pasteText[part.FileID.UUID])
		if content == "" {
			continue
		}
		pastes = append(pastes, stringutil.Truncate(content, syntheticPasteTitleBudget))
	}
	return strings.TrimSpace(strings.Join(pastes, "\n\n"))
}

// SyntheticPasteFileIDs returns the file IDs of file parts that are
// synthetic pasted-text attachments created by the chat UI. Callers
// resolve these to file content and pass the result to TitleText.
func SyntheticPasteFileIDs(parts []codersdk.ChatMessagePart) []uuid.UUID {
	var ids []uuid.UUID
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeFile || !part.FileID.Valid {
			continue
		}
		if !IsSyntheticPaste(part.Name, part.MediaType) {
			continue
		}
		ids = append(ids, part.FileID.UUID)
	}
	return ids
}

// fallbackTitleMaxRunes caps stored fallback titles; clients truncate
// visually to fit their layout.
const fallbackTitleMaxRunes = 80

// FallbackTitle derives a deterministic chat title from title text:
// the whole text with whitespace collapsed, capped at 80 runes and
// ellipsized only when that cap cuts it. Empty input yields "New Chat".
func FallbackTitle(message string) string {
	title := strings.Join(strings.Fields(message), " ")
	if title == "" {
		return "New Chat"
	}
	if len([]rune(title)) > fallbackTitleMaxRunes {
		return stringutil.Truncate(title, fallbackTitleMaxRunes-1) + "…"
	}
	return title
}

// IsFallbackTitle reports whether title is the fallback title for
// message. It also accepts the previous six-word format so chats
// created by older replicas during a rolling deploy stay eligible for
// auto-titling.
func IsFallbackTitle(title, message string) bool {
	return title == FallbackTitle(message) || title == legacyFallbackTitle(message)
}

// legacyFallbackTitle is the fallback title format used before titles
// kept the full message: the first six words, ellipsized when cut.
func legacyFallbackTitle(message string) string {
	const maxWords = 6

	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}
	if len(words) <= maxWords {
		return stringutil.Truncate(strings.Join(words, " "), fallbackTitleMaxRunes)
	}
	return stringutil.Truncate(strings.Join(words[:maxWords], " "), fallbackTitleMaxRunes-1) + "…"
}
