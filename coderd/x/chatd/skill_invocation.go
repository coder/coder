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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

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
// depend on the skill being listed in the turn's skill index. Workspace
// skills come from the chat's pinned context snapshot, so a chat without
// one cannot invoke them.
func (p *Server) resolveSkillInvocations(
	ctx context.Context,
	chat database.Chat,
	parts []codersdk.ChatMessagePart,
) ([]codersdk.ChatMessagePart, error) {
	if !slices.ContainsFunc(parts, isSkillInvocation) {
		return parts, nil
	}
	loadOwner := sync.OnceValues(func() (rbac.Subject, error) {
		return p.chatOwnerSubject(ctx, chat.OwnerID)
	})
	loadWorkspaceSkills := sync.OnceValues(func() ([]chattool.SkillMeta, error) {
		_, skills, err := p.resolveTurnWorkspaceContext(ctx, chat, database.WorkspaceAgent{})
		return skills, err
	})
	loadBody := func(part codersdk.ChatMessagePart) (string, error) {
		switch part.SkillSource {
		case codersdk.ChatSkillSourcePersonal:
			parsed, err := p.loadPersonalSkillBody(ctx, chat.OwnerID, part.SkillName)
			return parsed.Body, err
		case codersdk.ChatSkillSourceOrganization:
			owner, err := loadOwner()
			if err != nil {
				return "", err
			}
			parsed, err := p.loadOrganizationSkillBody(ctx, owner, chat.OrganizationID, part.SkillName)
			return parsed.Body, err
		case codersdk.ChatSkillSourceProject:
			projectID := p.resolveChatProjectID(ctx, p.logger, chat)
			if !projectID.Valid {
				return "", skillspkg.ErrSkillNotFound
			}
			owner, err := loadOwner()
			if err != nil {
				return "", err
			}
			parsed, err := p.loadProjectSkillBody(ctx, owner, projectID.UUID, part.SkillName)
			return parsed.Body, err
		case codersdk.ChatSkillSourceWorkspace:
			skills, err := loadWorkspaceSkills()
			if err != nil {
				return "", err
			}
			for _, skill := range skills {
				if skill.Name == part.SkillName {
					return chattool.PinnedWorkspaceSkillBody(skill)
				}
			}
		}
		return "", skillspkg.ErrSkillNotFound
	}

	resolved := slices.Clone(parts)
	for i, part := range resolved {
		if !isSkillInvocation(part) {
			continue
		}
		body, err := loadBody(part)
		if errors.Is(err, skillspkg.ErrSkillNotFound) {
			return nil, &SkillInvocationUnavailableError{Source: part.SkillSource, Name: part.SkillName}
		}
		if err != nil {
			return nil, xerrors.Errorf("load invoked skill %s/%s: %w", part.SkillSource, part.SkillName, err)
		}
		resolved[i].SkillBody = body
	}
	return resolved, nil
}

func isSkillInvocation(part codersdk.ChatMessagePart) bool {
	return part.Type == codersdk.ChatMessagePartTypeSkillInvocation
}
