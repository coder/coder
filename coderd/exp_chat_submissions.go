package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

func chatSubmissionOptions(ctx context.Context, requestID *uuid.UUID, options chatd.SubmissionOptions, input any) (*chatd.SubmissionOptions, error) {
	if requestID == nil {
		if options.ExactSettings {
			return nil, xerrors.New("exact_settings requires request_id")
		}
		return nil, nil //nolint:nilnil // An omitted identity deliberately selects the existing non-journaled path.
	}
	if *requestID == uuid.Nil {
		return nil, xerrors.New("request_id must be a nonzero UUID")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	options.ActorContext = ctx
	options.RequestID = *requestID
	options.InputDigest = digest[:]
	return &options, nil
}

func writeChatSubmissionError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	if errors.Is(err, workspaceexec.ErrAdmissionClosed) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: err.Error()})
		return true
	}
	if errors.Is(err, chatd.ErrInvalidExactSettings) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: err.Error()})
		return true
	}
	var submissionErr *chatd.SubmissionError
	if !errors.As(err, &submissionErr) {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, struct {
		codersdk.Response
		Submission codersdk.ChatSubmissionReceipt `json:"submission"`
	}{
		Response:   codersdk.Response{Message: submissionErr.Error()},
		Submission: submissionErr.Receipt,
	})
	return true
}

// replayChatSubmission is called only after current actor/resource authorization.
// It never revalidates mutable new-admission settings or starts additional work.
func (api *API) replayChatSubmission(ctx context.Context, rw http.ResponseWriter, opts *chatd.SubmissionOptions, organizationID uuid.UUID, kind string) bool {
	replayed, err := api.chatDaemon.ReplaySubmission(ctx, opts, organizationID, kind)
	if err != nil {
		if !writeChatSubmissionError(ctx, rw, err) {
			if dbauthz.IsNotAuthorizedError(err) {
				httpapi.ResourceNotFound(rw)
			} else {
				httpapi.InternalServerError(rw, err)
			}
		}
		return true
	}
	if !replayed {
		return false
	}
	chat, err := api.Database.GetChatByID(ctx, opts.Receipt.ChatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			httpapi.ResourceNotFound(rw)
		} else {
			httpapi.InternalServerError(rw, err)
		}
		return true
	}
	if kind == "create" {
		response := db2sdk.Chat(chat, nil, api.fetchChatFileMetadata(ctx, chat.ID))
		response.Submission = opts.Receipt
		httpapi.Write(ctx, rw, http.StatusCreated, response)
	} else {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.CreateChatMessageResponse{Queued: opts.Receipt.QueuedMessageID != 0, Submission: opts.Receipt})
	}
	return true
}
