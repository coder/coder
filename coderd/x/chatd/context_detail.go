package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// maxContextChangeContentBytes caps each side of an instruction-file change so
// the single-chat GET response stays bounded. The agent push admits resource
// bodies up to 256KiB; for the on-read diff we surface at most this many bytes
// per side, truncated on a rune boundary.
const maxContextChangeContentBytes = 64 * 1024

// ContextDetail computes the chat's pinned context resource list and, when the
// chat has drifted, the per-source change set against the agent's latest
// pushed snapshot. It is read-only and intended for the single-chat GET
// handler; list and watch payloads omit this detail to stay lightweight.
//
// resources mirrors the prompt-injection rules so it equals the context the
// model actually sees: OK instruction files with non-empty content and OK
// skills with a name. changes is nil unless the chat is dirty (and has a
// resolvable agent), so the second read is only paid for when it can differ.
func (server *Server) ContextDetail(
	ctx context.Context,
	chat database.Chat,
) (resources []codersdk.ChatContextResource, changes []codersdk.ChatContextResourceChange, err error) {
	pinned, err := server.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return nil, nil, xerrors.Errorf("list chat context resources: %w", err)
	}
	resources = pinnedContextResources(pinned)

	if !chat.ContextDirtySince.Valid || !chat.AgentID.Valid {
		return resources, nil, nil
	}
	snapshot, err := server.db.ListWorkspaceAgentContextResources(ctx, chat.AgentID.UUID)
	if err != nil {
		return nil, nil, xerrors.Errorf("list workspace agent context resources: %w", err)
	}
	changes = diffContextResources(pinned, snapshot)
	server.logger.Debug(ctx, "computed chat context detail",
		slog.F("chat_id", chat.ID),
		slog.F("resource_count", len(resources)),
		slog.F("change_count", len(changes)),
	)
	return resources, changes, nil
}

// contextResourceSide is the subset of a context resource row needed to diff
// one source across the pinned copy and the agent snapshot.
type contextResourceSide struct {
	kind        database.WorkspaceAgentContextBodyKind
	body        json.RawMessage
	contentHash []byte
}

// diffContextResources compares a chat's pinned context against the agent's
// latest snapshot, by source, and returns the changes among prompt kinds
// (instruction files and skills). A source present on both sides with an equal
// content hash is unchanged and omitted; a differing hash is modified;
// pinned-only is removed; snapshot-only is added. Output is ordered by source.
func diffContextResources(
	pinned []database.ChatContextResource,
	snapshot []database.WorkspaceAgentContextResource,
) []codersdk.ChatContextResourceChange {
	pinnedBySource := make(map[string]contextResourceSide, len(pinned))
	for _, r := range pinned {
		pinnedBySource[r.Source] = contextResourceSide{kind: r.BodyKind, body: r.Body, contentHash: r.ContentHash}
	}
	snapshotBySource := make(map[string]contextResourceSide, len(snapshot))
	sources := make([]string, 0, len(pinned)+len(snapshot))
	for _, r := range pinned {
		sources = append(sources, r.Source)
	}
	for _, r := range snapshot {
		if _, ok := pinnedBySource[r.Source]; !ok {
			sources = append(sources, r.Source)
		}
		snapshotBySource[r.Source] = contextResourceSide{kind: r.BodyKind, body: r.Body, contentHash: r.ContentHash}
	}
	sort.Strings(sources)

	var changes []codersdk.ChatContextResourceChange
	for _, source := range sources {
		pinnedSide, hasPinned := pinnedBySource[source]
		snapshotSide, hasSnapshot := snapshotBySource[source]
		switch {
		case hasPinned && hasSnapshot:
			if bytes.Equal(pinnedSide.contentHash, snapshotSide.contentHash) {
				continue
			}
			if change, ok := buildResourceChange(source, codersdk.ChatContextResourceChangeStatusModified, &pinnedSide, &snapshotSide); ok {
				changes = append(changes, change)
			}
		case hasPinned:
			if change, ok := buildResourceChange(source, codersdk.ChatContextResourceChangeStatusRemoved, &pinnedSide, nil); ok {
				changes = append(changes, change)
			}
		case hasSnapshot:
			if change, ok := buildResourceChange(source, codersdk.ChatContextResourceChangeStatusAdded, nil, &snapshotSide); ok {
				changes = append(changes, change)
			}
		}
	}
	return changes
}

// buildResourceChange assembles a change entry for one source. The reported
// kind comes from the side that exists now (snapshot for added/modified,
// pinned for removed); ok is false only for kinds chatd does not track. An
// instruction-file change carries the sanitized, capped bodies of whichever
// sides are present; a skill change carries the identifying name and
// description; MCP config/server changes carry only source, kind, and status.
func buildResourceChange(
	source string,
	status codersdk.ChatContextResourceChangeStatus,
	pinned, snapshot *contextResourceSide,
) (codersdk.ChatContextResourceChange, bool) {
	current := snapshot
	if current == nil {
		current = pinned
	}
	kind, ok := contextResourceKind(current.kind)
	if !ok {
		return codersdk.ChatContextResourceChange{}, false
	}

	change := codersdk.ChatContextResourceChange{
		Source: source,
		Kind:   kind,
		Status: status,
	}
	switch kind {
	case codersdk.ChatContextResourceKindInstructionFile:
		if pinned != nil {
			change.OldContent = cappedInstructionContent(pinned.body)
		}
		if snapshot != nil {
			change.NewContent = cappedInstructionContent(snapshot.body)
		}
	case codersdk.ChatContextResourceKindSkill:
		// Removed skills exist only on the pinned side; otherwise the snapshot
		// identifies what a refresh would adopt.
		identity := snapshot
		if identity == nil {
			identity = pinned
		}
		if body, decoded := decodeSkillMetaBody(identity.body); decoded {
			change.SkillName = body.GetName()
			change.SkillDescription = body.GetDescription()
		}
	}
	return change, true
}

// cappedInstructionContent decodes, sanitizes, and length-caps an instruction
// file body for display in a change diff. It returns "" when the body is not a
// decodable instruction file (e.g. a non-OK snapshot with an empty body).
func cappedInstructionContent(body json.RawMessage) string {
	decoded, ok := decodeInstructionFileBody(body)
	if !ok {
		return ""
	}
	return truncateUTF8(SanitizePromptText(string(decoded.GetContent())), maxContextChangeContentBytes)
}

// truncateUTF8 returns s truncated to at most n bytes without splitting a
// multi-byte rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
