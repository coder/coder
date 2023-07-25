package dashboard

import (
	"context"
	"io"
	"math/rand"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/scaletest/harness"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	client.Trace = cfg.Trace
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

func (r *Runner) Run(ctx context.Context, _ string, _ io.Writer) error {
	me, err := r.client.User(ctx, codersdk.Me)
	if err != nil {
		return err
	}
	if len(me.OrganizationIDs) == 0 {
		return xerrors.Errorf("user has no organizations")
	}
	orgID := me.OrganizationIDs[0]
	rolls := make(chan int)
	// Roll a die
	go func() {
		t := time.NewTicker(r.randWait())
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rolls <- rand.Intn(6)
				t.Reset(r.randWait())
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-rolls:
			switch n {
			case 0:
				go r.do(ctx, "fetch workspaces", func(client *codersdk.Client) error {
					_, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
					return err
				})
			case 1:
				go r.do(ctx, "fetch users", func(client *codersdk.Client) error {
					_, err := client.Users(ctx, codersdk.UsersRequest{})
					return err
				})
			case 2:
				go r.do(ctx, "fetch templates", func(client *codersdk.Client) error {
					_, err = client.TemplatesByOrganization(ctx, orgID)
					return err
				})
			case 3:
				go r.do(ctx, "auth check - owner", func(client *codersdk.Client) error {
					_, err := client.AuthCheck(ctx, randAuthReq(
						ownedBy(me.ID),
						withAction(randAction()),
						withObjType(randObjectType()),
						inOrg(orgID),
					))
					return err
				})
			case 4:
				go r.do(ctx, "auth check - not owner", func(client *codersdk.Client) error {
					_, err := client.AuthCheck(ctx, randAuthReq(
						ownedBy(uuid.New()),
						withAction(randAction()),
						withObjType(randObjectType()),
						inOrg(orgID),
					))
					return err
				})
			default:
				r.cfg.Logger.Warn(ctx, "no action for roll - skipping my turn", slog.F("roll", n))
			}
		}
	}
}

func (*Runner) Cleanup(_ context.Context, _ string) error {
	return nil
}

func (r *Runner) do(ctx context.Context, label string, fn func(client *codersdk.Client) error) {
	select {
	case <-ctx.Done():
		r.cfg.Logger.Info(ctx, "context done, stopping")
		return
	default:
		start := time.Now()
		err := fn(r.client)
		elapsed := time.Since(start)
		if err != nil {
			r.cfg.Logger.Error(
				ctx, "function returned error",
				slog.Error(err),
				slog.F("fn", label),
				slog.F("elapsed", elapsed),
			)
		} else {
			r.cfg.Logger.Info(ctx, "completed successfully",
				slog.F("fn", label),
				slog.F("elapsed", elapsed),
			)
		}
	}
}

func (r *Runner) randWait() time.Duration {
	// nolint:gosec // This is not for cryptographic purposes. Chill, gosec. Chill.
	wait := time.Duration(rand.Intn(int(r.cfg.MaxWait) - int(r.cfg.MinWait)))
	return r.cfg.MinWait + wait
}

// nolint: gosec
func randAuthReq(mut ...func(*codersdk.AuthorizationCheck)) codersdk.AuthorizationRequest {
	var check codersdk.AuthorizationCheck
	for _, m := range mut {
		m(&check)
	}
	return codersdk.AuthorizationRequest{
		Checks: map[string]codersdk.AuthorizationCheck{
			"check": check,
		},
	}
}

func ownedBy(myID uuid.UUID) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.OwnerID = myID.String()
	}
}

func inOrg(orgID uuid.UUID) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.OrganizationID = orgID.String()
	}
}

func withResourceID(id uuid.UUID) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.ResourceID = id.String()
	}
}

func withObjType(objType codersdk.RBACResource) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.ResourceType = objType
	}
}

func withAction(action string) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Action = action
	}
}

func randAction() string {
	// nolint:gosec
	return codersdk.AllRBACActions[rand.Intn(len(codersdk.AllRBACActions))]
}

func randObjectType() codersdk.RBACResource {
	// nolint:gosec
	return codersdk.AllRBACResources[rand.Intn(len(codersdk.AllRBACResources))]
}
