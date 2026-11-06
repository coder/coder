package chatlist

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

const (
	// LabelKey and LabelValue mark chats created by this scaletest so later
	// runs can reuse them instead of creating more.
	LabelKey   = "scaletest"
	LabelValue = "chat-list"

	createConcurrency = 10
	countPageSize     = 100
)

// EnsureUser returns the named user, creating it in organizationID if it does
// not exist. The user is granted agents-access so it can create and list
// chats regardless of the organization's default member roles.
func EnsureUser(ctx context.Context, client *codersdk.Client, organizationID uuid.UUID, username string) (codersdk.User, error) {
	user, err := client.User(ctx, username)
	if err != nil {
		var sdkErr *codersdk.Error
		if !errors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusNotFound {
			return codersdk.User{}, xerrors.Errorf("get user %q: %w", username, err)
		}
		password, err := cryptorand.String(16)
		if err != nil {
			return codersdk.User{}, xerrors.Errorf("generate password: %w", err)
		}
		user, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{organizationID},
			Username:        username,
			Email:           username + "@scaletest.local",
			Password:        password,
		})
		if err != nil {
			return codersdk.User{}, xerrors.Errorf("create user %q: %w", username, err)
		}
	}

	_, err = client.UpdateOrganizationMemberRoles(ctx, organizationID, user.ID.String(), codersdk.UpdateRoles{
		Roles: []string{codersdk.RoleAgentsAccess},
	})
	if err != nil {
		return codersdk.User{}, xerrors.Errorf("grant %s to %q: %w", codersdk.RoleAgentsAccess, username, err)
	}
	return user, nil
}

// EnsureChats makes sure the client's user owns at least count unarchived
// scaletest chats, creating the shortfall. Chats are created idle with no
// initial message, so no LLM work is started. It returns the number of chats
// that already existed and the number created.
func EnsureChats(ctx context.Context, client *codersdk.Client, logger slog.Logger, organizationID, modelConfigID uuid.UUID, count int) (existing int, created int, err error) {
	existing, err = countScaletestChats(ctx, client)
	if err != nil {
		return 0, 0, err
	}
	missing := count - existing
	logger.Info(ctx, "ensuring scaletest chats exist", slog.F("existing", existing), slog.F("wanted", count), slog.F("creating", max(missing, 0)))
	if missing <= 0 {
		return existing, 0, nil
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(createConcurrency)
	for i := 0; i < missing; i++ {
		title := fmt.Sprintf("scaletest chat-list %d", existing+i)
		eg.Go(func() error {
			_, err := client.CreateChat(egCtx, codersdk.CreateChatRequest{
				OrganizationID: organizationID,
				ModelConfigID:  &modelConfigID,
				Title:          &title,
				Labels:         map[string]string{LabelKey: LabelValue},
			})
			if err != nil {
				return xerrors.Errorf("create chat %q: %w", title, err)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return existing, 0, err
	}
	return existing, missing, nil
}

func countScaletestChats(ctx context.Context, client *codersdk.Client) (int, error) {
	total := 0
	for {
		chats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Labels:     map[string]string{LabelKey: LabelValue},
			Pagination: codersdk.Pagination{Limit: countPageSize, Offset: total},
		})
		if err != nil {
			return 0, xerrors.Errorf("list scaletest chats: %w", err)
		}
		total += len(chats)
		if len(chats) < countPageSize {
			return total, nil
		}
	}
}
