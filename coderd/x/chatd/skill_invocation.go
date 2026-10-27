package chatd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

// MaxSkillInvocationsPerMessage bounds the skill-invocation parts in one
// message, and with them the skill bodies a message stores.
const MaxSkillInvocationsPerMessage = 8

// ErrTooManySkillInvocations reports a message with more than
// MaxSkillInvocationsPerMessage skill-invocation parts.
var ErrTooManySkillInvocations = xerrors.Errorf("a message can invoke at most %d skills", MaxSkillInvocationsPerMessage)

// SkillInvocationUnavailableError reports a skill-invocation part whose
// skill the chat owner cannot use: it is missing, disabled, or not readable.
type SkillInvocationUnavailableError struct {
	Source codersdk.ChatSkillSource
	Name   string
}

func (e *SkillInvocationUnavailableError) Error() string {
	return fmt.Sprintf("Skill %s/%s is not available.", e.Source, e.Name)
}

// resolveChatSkillInvocations loads the chat and resolves the
// skill-invocation parts of a message sent to it. The chat is only read
// when the message invokes a skill.
func (p *Server) resolveChatSkillInvocations(
	ctx context.Context,
	chatID uuid.UUID,
	parts []codersdk.ChatMessagePart,
) ([]codersdk.ChatMessagePart, error) {
	if !slices.ContainsFunc(parts, isSkillInvocation) {
		return parts, nil
	}
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return nil, xerrors.Errorf("load chat for skill invocations: %w", err)
	}
	return p.resolveSkillInvocations(ctx, chat, parts)
}

// resolveSkillInvocations stores the current body of every invoked skill on
// its part, read with the chat owner's permissions through the same loaders
// as read_skill. Skills load by source and name, so an invocation does not
// depend on the skill being listed in the turn's skill index, and skills
// that are not model-invocable still load. Workspace
// skills come from the chat's pinned context snapshot, so a chat without
// one cannot invoke them. An invoked workspace skill also stores its
// directory: read_skill refuses a skill hidden from the model, so the
// invocation is the only place the model is told where its scripts are.
// The directory is stored with the chat's workspace, because the path
// means nothing once the chat is bound to another workspace.
func (p *Server) resolveSkillInvocations(
	ctx context.Context,
	chat database.Chat,
	parts []codersdk.ChatMessagePart,
) ([]codersdk.ChatMessagePart, error) {
	invocations := countSkillInvocations(parts)
	if invocations == 0 {
		return parts, nil
	}
	if invocations > MaxSkillInvocationsPerMessage {
		return nil, ErrTooManySkillInvocations
	}
	loadOwner := sync.OnceValues(func() (rbac.Subject, error) {
		return p.chatOwnerSubject(ctx, chat.OwnerID)
	})
	loadWorkspaceSkills := sync.OnceValues(func() ([]chattool.SkillMeta, error) {
		_, skills, err := p.resolveTurnWorkspaceContext(ctx, chat, database.WorkspaceAgent{})
		return skills, err
	})
	type invokedSkill struct {
		body        string
		dir         string
		workspaceID uuid.UUID
	}
	loadSkill := func(part codersdk.ChatMessagePart) (invokedSkill, error) {
		switch part.SkillSource {
		case codersdk.ChatSkillSourcePersonal:
			parsed, err := p.loadPersonalSkillBody(ctx, chat.OwnerID, part.SkillName, skillLoaderUser)
			return invokedSkill{body: parsed.Body}, err
		case codersdk.ChatSkillSourceOrganization:
			owner, err := loadOwner()
			if err != nil {
				return invokedSkill{}, err
			}
			parsed, err := p.loadOrganizationSkillBody(ctx, owner, chat.OrganizationID, part.SkillName, skillLoaderUser)
			return invokedSkill{body: parsed.Body}, err
		case codersdk.ChatSkillSourceProject:
			// Read as chatd behind the chat owner's project usability check,
			// like project skill loading for turns.
			//nolint:gocritic // Access is gated on ChatProjectUsableBy for the chat owner.
			chatdCtx := dbauthz.AsChatd(ctx)
			projectID := p.resolveUsableChatProjectID(chatdCtx, p.logger, chat)
			if !projectID.Valid {
				return invokedSkill{}, skillspkg.ErrSkillNotFound
			}
			parsed, err := p.loadProjectSkillBody(chatdCtx, projectID.UUID, part.SkillName, skillLoaderUser)
			return invokedSkill{body: parsed.Body}, err
		case codersdk.ChatSkillSourceWorkspace:
			skills, err := loadWorkspaceSkills()
			if err != nil {
				return invokedSkill{}, err
			}
			for _, skill := range skills {
				if skill.Name == part.SkillName {
					body, err := chattool.PinnedWorkspaceSkillBody(skill)
					return invokedSkill{body: body, dir: skill.Dir, workspaceID: chat.WorkspaceID.UUID}, err
				}
			}
		}
		return invokedSkill{}, skillspkg.ErrSkillNotFound
	}

	// A message that repeats a skill loads it once.
	type skillKey struct {
		source codersdk.ChatSkillSource
		name   string
	}
	loaded := make(map[skillKey]invokedSkill, invocations)
	resolved := slices.Clone(parts)
	for i, part := range resolved {
		if !isSkillInvocation(part) {
			continue
		}
		key := skillKey{source: part.SkillSource, name: part.SkillName}
		skill, ok := loaded[key]
		if !ok {
			var err error
			skill, err = loadSkill(part)
			if errors.Is(err, skillspkg.ErrSkillNotFound) {
				return nil, &SkillInvocationUnavailableError{Source: part.SkillSource, Name: part.SkillName}
			}
			if err != nil {
				return nil, xerrors.Errorf("load invoked skill %s/%s: %w", part.SkillSource, part.SkillName, err)
			}
			loaded[key] = skill
		}
		resolved[i].SkillBody = skill.body
		resolved[i].SkillDir = skill.dir
		resolved[i].SkillWorkspaceID = skill.workspaceID
	}
	return resolved, nil
}

func countSkillInvocations(parts []codersdk.ChatMessagePart) int {
	count := 0
	for _, part := range parts {
		if isSkillInvocation(part) {
			count++
		}
	}
	return count
}

func isSkillInvocation(part codersdk.ChatMessagePart) bool {
	return part.Type == codersdk.ChatMessagePartTypeSkillInvocation
}
