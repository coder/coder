package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

const maxInterceptionsLimit = 1000

func (r *RootCmd) aibridge() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "aibridge",
		Short: "Manage AI Bridge.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.aibridgeInterceptions(),
		},
	}
	return cmd
}

func (r *RootCmd) aibridgeInterceptions() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "interceptions",
		Short: "Manage AI Bridge interceptions.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.aibridgeInterceptionsList(),
		},
	}
	return cmd
}

func (r *RootCmd) aibridgeInterceptionsList() *serpent.Command {
	var (
		initiator        string
		startedBeforeRaw string
		startedAfterRaw  string
		provider         string
		model            string
		afterIDRaw       string
		limit            int64
	)

	return &serpent.Command{
		Use:   "list",
		Short: "List AI Bridge interceptions as JSON.",
		Options: serpent.OptionSet{
			{
				Flag:        "initiator",
				Description: `Only return interceptions initiated by this user. Accepts a user ID, username, or "me".`,
				Default:     "",
				Value:       serpent.StringOf(&initiator),
			},
			{
				Flag:        "started-before",
				Description: fmt.Sprintf("Only return interceptions started before this time. Must be after 'started-after' if set. Accepts a time in the RFC 3339 format, e.g. %q.", time.RFC3339),
				Default:     "",
				Value:       serpent.StringOf(&startedBeforeRaw),
			},
			{
				Flag:        "started-after",
				Description: fmt.Sprintf("Only return interceptions started after this time. Must be before 'started-before' if set. Accepts a time in the RFC 3339 format, e.g. %q.", time.RFC3339),
				Default:     "",
				Value:       serpent.StringOf(&startedAfterRaw),
			},
			{
				Flag:        "provider",
				Description: `Only return interceptions from this provider.`,
				Default:     "",
				Value:       serpent.StringOf(&provider),
			},
			{
				Flag:        "model",
				Description: `Only return interceptions from this model.`,
				Default:     "",
				Value:       serpent.StringOf(&model),
			},
			{
				Flag:        "after-id",
				Description: "The ID of the last result on the previous page to use as a pagination cursor.",
				Default:     "",
				Value:       serpent.StringOf(&afterIDRaw),
			},
			{
				Flag:        "limit",
				Description: fmt.Sprintf(`The limit of results to return. Must be between 1 and %d.`, maxInterceptionsLimit),
				Default:     "100",
				Value:       serpent.Int64Of(&limit),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			startedBefore := time.Time{}
			if startedBeforeRaw != "" {
				startedBefore, err = time.Parse(time.RFC3339, startedBeforeRaw)
				if err != nil {
					return xerrors.Errorf("parse started before filter value %q: %w", startedBeforeRaw, err)
				}
			}

			startedAfter := time.Time{}
			if startedAfterRaw != "" {
				startedAfter, err = time.Parse(time.RFC3339, startedAfterRaw)
				if err != nil {
					return xerrors.Errorf("parse started after filter value %q: %w", startedAfterRaw, err)
				}
			}

			afterID := uuid.Nil
			if afterIDRaw != "" {
				afterID, err = uuid.Parse(afterIDRaw)
				if err != nil {
					return xerrors.Errorf("parse after_id filter value %q: %w", afterIDRaw, err)
				}
			}

			if limit < 1 || limit > maxInterceptionsLimit {
				return xerrors.Errorf("limit value must be between 1 and %d", maxInterceptionsLimit)
			}

			resp, err := client.AIBridgeListInterceptions(inv.Context(), codersdk.AIBridgeListInterceptionsFilter{
				Pagination: codersdk.Pagination{
					AfterID: afterID,
					// #nosec G115 - Checked above.
					Limit: int(limit),
				},
				Initiator:     initiator,
				StartedBefore: startedBefore,
				StartedAfter:  startedAfter,
				Provider:      provider,
				Model:         model,
			})
			if err != nil {
				return xerrors.Errorf("list interceptions: %w", err)
			}

			// We currently only support JSON output, so we don't use a
			// formatter.
			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			err = enc.Encode(resp.Results)
			if err != nil {
				return err
			}

			return err
		},
	}
}
