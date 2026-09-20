package chatd

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// Limits applied to agent-to-agent messages in the chat tree.
const (
	// chatTreeMessageMaxRunes caps the text of one message.
	chatTreeMessageMaxRunes = 32768
	// chatTreeMessageMaxHops caps consecutive agent-to-agent deliveries since
	// the last human-initiated turn anywhere in the chain.
	chatTreeMessageMaxHops = 12
	// chatTreeHumanTurnSendLimit caps send_chat_message calls in a turn whose
	// starting prompt was typed by a human.
	chatTreeHumanTurnSendLimit = 20
	// chatTreeRelayedTurnSendLimit caps send_chat_message calls in a turn
	// whose starting prompt was delivered by another chat's agent.
	chatTreeRelayedTurnSendLimit = 3
)

const (
	sendChatMessageToolName = "send_chat_message"
	listChatTreeToolName    = "list_chat_tree"
	// chatTreeParentTarget is the chat_id literal that addresses the parent.
	chatTreeParentTarget = "parent"
)

// chatTreeDelivery is the send_chat_message delivery argument.
type chatTreeDelivery string

const (
	chatTreeDeliveryQueue     chatTreeDelivery = "queue"
	chatTreeDeliveryInterrupt chatTreeDelivery = "interrupt"
)

// chatTreeDeliveryOutcome is the delivery field of a successful
// send_chat_message result.
type chatTreeDeliveryOutcome string

const (
	chatTreeDeliveryOutcomeStarted      chatTreeDeliveryOutcome = "started"
	chatTreeDeliveryOutcomeQueued       chatTreeDeliveryOutcome = "queued"
	chatTreeDeliveryOutcomeInterrupting chatTreeDeliveryOutcome = "interrupting"
	chatTreeDeliveryOutcomeRejected     chatTreeDeliveryOutcome = "rejected"
)

var (
	// ErrChatTreeNotNeighbour indicates the target is not the sender's parent
	// or one of its direct named children. It also covers missing chats and
	// chats the owner cannot read so the error is not an existence oracle.
	ErrChatTreeNotNeighbour = xerrors.New("chat is not the parent or a direct child of this chat")
	// ErrChatTreeNoParent indicates the sender has no parent chat.
	ErrChatTreeNoParent = xerrors.New("this chat has no parent chat")
	// ErrChatTreeMessageEmpty indicates the message has no text after trimming.
	ErrChatTreeMessageEmpty = xerrors.New("message must not be empty")
	// ErrChatTreeMessageTooLong indicates the message exceeds
	// chatTreeMessageMaxRunes.
	ErrChatTreeMessageTooLong = xerrors.Errorf("message exceeds %d characters", chatTreeMessageMaxRunes)
	// ErrChatTreeMessageRelayLimit indicates the outgoing relay hop would
	// exceed chatTreeMessageMaxHops.
	ErrChatTreeMessageRelayLimit = xerrors.New("relay depth limit reached; wait for a human message before messaging again")
	// ErrChatTreeMessageTurnLimit indicates the per-turn send budget is spent.
	// Every send_chat_message call in the turn counts, including rejected
	// ones.
	ErrChatTreeMessageTurnLimit = xerrors.New("per-turn message limit reached (rejected attempts count too)")
	// ErrChatTreeInvalidDelivery indicates delivery is neither queue nor
	// interrupt.
	ErrChatTreeInvalidDelivery = xerrors.New(`delivery must be "queue" or "interrupt"`)
	// ErrChatTreeInvalidChatID indicates chat_id is neither a UUID nor the
	// parent literal.
	ErrChatTreeInvalidChatID = xerrors.New(`chat_id must be a valid UUID or "parent"`)
	// ErrChatTreeSenderNotEligible indicates the sending chat is a subagent.
	ErrChatTreeSenderNotEligible = xerrors.New("subagent chats cannot send chat tree messages")
	// ErrChatOwnerInactive indicates the chat owner's account is deleted or
	// not active.
	ErrChatOwnerInactive = xerrors.New("chat owner is not active")
	// errChatTreeHistoryUnavailable indicates the sender's history has no
	// user prompt, so the relay hop cannot be derived.
	errChatTreeHistoryUnavailable = xerrors.New("chat history unavailable; cannot determine relay depth")
	// errChatTreeInternal replaces unexpected store errors in tool results;
	// the detail is logged.
	errChatTreeInternal = xerrors.New("internal error sending chat tree message")
)

type sendChatMessageArgs struct {
	ChatID   string `json:"chat_id"`
	Message  string `json:"message"`
	Delivery string `json:"delivery,omitempty"`
}

type listChatTreeArgs struct{}

// chatOwnerContext returns ctx acting as the chat owner with full scope.
// It fails when the owner's account is deleted or not active.
func chatOwnerContext(ctx context.Context, store database.Store, ownerID uuid.UUID) (context.Context, error) {
	// GetAuthorizationUserRoles does not filter deleted users, so the row
	// is loaded first. Tool callbacks run on the chatd worker context,
	// which cannot read user rows, so this single read is system scoped.
	//nolint:gocritic // Background chatd work has no owner actor yet; every later call runs as the owner.
	owner, err := store.GetUserByID(dbauthz.AsSystemRestricted(ctx), ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrChatOwnerInactive
		}
		return nil, xerrors.Errorf("load chat owner: %w", err)
	}
	if owner.Deleted || owner.Status != database.UserStatusActive {
		return nil, ErrChatOwnerInactive
	}
	actor, status, err := httpmw.UserRBACSubject(ctx, store, ownerID, rbac.ScopeAll)
	if err != nil {
		return nil, xerrors.Errorf("load chat owner authorization: %w", err)
	}
	if status != database.UserStatusActive {
		return nil, ErrChatOwnerInactive
	}
	//nolint:gocritic // Chat tree messages run as the chat owner so dbauthz enforces the owner's chat permissions.
	return dbauthz.As(ctx, actor), nil
}

// chatTreeNeighbour is a resolved send_chat_message target. relation is the
// target's position relative to the sender.
type chatTreeNeighbour struct {
	chat     database.Chat
	relation codersdk.ChatSenderChatRelation
}

// senderRelation is the sender's position relative to the target, which is
// what the delivered sender-chat part records.
func (n chatTreeNeighbour) senderRelation() codersdk.ChatSenderChatRelation {
	if n.relation == codersdk.ChatSenderChatRelationParent {
		return codersdk.ChatSenderChatRelationChild
	}
	return codersdk.ChatSenderChatRelationParent
}

// resolveChatTreeMessageTarget loads the target addressed by rawChatID and
// checks that it is the sender's parent or one of the sender's direct named
// children with the same owner and organization. ctx must already act as
// the owner.
func resolveChatTreeMessageTarget(
	ctx context.Context,
	store database.Store,
	sender database.Chat,
	rawChatID string,
) (chatTreeNeighbour, error) {
	rawChatID = strings.TrimSpace(rawChatID)
	var targetID uuid.UUID
	if strings.EqualFold(rawChatID, chatTreeParentTarget) {
		if !sender.ParentChatID.Valid {
			return chatTreeNeighbour{}, ErrChatTreeNoParent
		}
		targetID = sender.ParentChatID.UUID
	} else {
		parsed, err := uuid.Parse(rawChatID)
		if err != nil {
			return chatTreeNeighbour{}, ErrChatTreeInvalidChatID
		}
		targetID = parsed
	}
	if targetID == sender.ID {
		return chatTreeNeighbour{}, ErrChatTreeNotNeighbour
	}

	target, err := store.GetChatByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return chatTreeNeighbour{}, ErrChatTreeNotNeighbour
		}
		return chatTreeNeighbour{}, xerrors.Errorf("load target chat: %w", err)
	}
	if target.OwnerID != sender.OwnerID || target.OrganizationID != sender.OrganizationID {
		return chatTreeNeighbour{}, ErrChatTreeNotNeighbour
	}

	var relation codersdk.ChatSenderChatRelation
	switch {
	case sender.ParentChatID.Valid && sender.ParentChatID.UUID == target.ID &&
		(target.Kind == database.ChatKindRoot || target.Kind == database.ChatKindChat):
		relation = codersdk.ChatSenderChatRelationParent
	case target.ParentChatID.Valid && target.ParentChatID.UUID == sender.ID &&
		target.Kind == database.ChatKindChat:
		relation = codersdk.ChatSenderChatRelationChild
	default:
		return chatTreeNeighbour{}, ErrChatTreeNotNeighbour
	}
	if target.Archived {
		return chatTreeNeighbour{}, ErrChatArchived
	}
	return chatTreeNeighbour{chat: target, relation: relation}, nil
}

// chatTreeTurnState summarizes the sender's current turn for the relay and
// budget checks.
type chatTreeTurnState struct {
	// relayed is true when a prompt that started the turn carries a
	// sender-chat part.
	relayed bool
	// maxHop is the highest relay_hop among the prompts that started the
	// turn; 0 when none was relayed.
	maxHop int
	// resolvedSends counts send_chat_message calls in this turn that already
	// have a tool result.
	resolvedSends int
	// unresolvedSendCallIDs lists send_chat_message calls in this turn
	// without a tool result, in assistant order.
	unresolvedSendCallIDs []string
}

// chatTreeTurnStateFromHistory derives the turn state from the sender's
// user-visible history. The prompts that started the turn are the last run
// of user-role rows; system rows (hook notices) inside that run are skipped
// and assistant or tool rows end it. Deleted rows are ignored. Compressed
// rows are kept so a mid-turn compaction cannot hide the starting prompt.
func chatTreeTurnStateFromHistory(messages []database.ChatMessage) (chatTreeTurnState, error) {
	var state chatTreeTurnState
	live := make([]database.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Deleted {
			continue
		}
		live = append(live, msg)
	}

	segmentEnd := -1
	for i := len(live) - 1; i >= 0; i-- {
		if live[i].Role == database.ChatMessageRoleUser {
			segmentEnd = i
			break
		}
	}
	if segmentEnd == -1 {
		return chatTreeTurnState{}, errChatTreeHistoryUnavailable
	}
	segmentStart := segmentEnd
	for i := segmentEnd - 1; i >= 0; i-- {
		switch live[i].Role {
		case database.ChatMessageRoleUser, database.ChatMessageRoleSystem:
			segmentStart = i
			continue
		}
		break
	}

	for _, msg := range live[segmentStart : segmentEnd+1] {
		if msg.Role != database.ChatMessageRoleUser {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return chatTreeTurnState{}, xerrors.Errorf("parse user message %d: %w", msg.ID, err)
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeSenderChat {
				continue
			}
			state.relayed = true
			state.maxHop = max(state.maxHop, part.RelayHop)
		}
	}

	var sendCallIDs []string
	handled := make(map[string]bool)
	for _, msg := range live[segmentEnd+1:] {
		switch msg.Role {
		case database.ChatMessageRoleAssistant, database.ChatMessageRoleTool:
		default:
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return chatTreeTurnState{}, xerrors.Errorf("parse message %d: %w", msg.ID, err)
		}
		for _, part := range parts {
			switch {
			case msg.Role == database.ChatMessageRoleAssistant &&
				part.Type == codersdk.ChatMessagePartTypeToolCall &&
				part.ToolName == sendChatMessageToolName:
				sendCallIDs = append(sendCallIDs, part.ToolCallID)
			case msg.Role == database.ChatMessageRoleTool &&
				part.Type == codersdk.ChatMessagePartTypeToolResult &&
				part.ToolCallID != "":
				handled[part.ToolCallID] = true
			}
		}
	}
	for _, id := range sendCallIDs {
		if handled[id] {
			state.resolvedSends++
			continue
		}
		state.unresolvedSendCallIDs = append(state.unresolvedSendCallIDs, id)
	}
	return state, nil
}

// sendLimit is the per-turn send budget for this turn.
func (s chatTreeTurnState) sendLimit() int {
	if s.relayed {
		return chatTreeRelayedTurnSendLimit
	}
	return chatTreeHumanTurnSendLimit
}

// nextHop is the relay_hop the outgoing message carries.
func (s chatTreeTurnState) nextHop() int {
	return s.maxHop + 1
}

// allowSend reports whether the call identified by toolCallID fits the
// per-turn budget. Resolved calls count first; the current call counts by
// its position among the unresolved calls, so calls executed together and
// calls re-executed after a replica takeover get the same decision. A
// duplicated call id occupies the slot of its last occurrence, and an id
// missing from history is treated as following every unresolved call.
func (s chatTreeTurnState) allowSend(toolCallID string) bool {
	position := len(s.unresolvedSendCallIDs)
	for i := len(s.unresolvedSendCallIDs) - 1; i >= 0; i-- {
		if s.unresolvedSendCallIDs[i] == toolCallID {
			position = i
			break
		}
	}
	return s.resolvedSends+position < s.sendLimit()
}

func parseChatTreeDelivery(raw string) (chatTreeDelivery, error) {
	switch chatTreeDelivery(strings.ToLower(strings.TrimSpace(raw))) {
	case "", chatTreeDeliveryQueue:
		return chatTreeDeliveryQueue, nil
	case chatTreeDeliveryInterrupt:
		return chatTreeDeliveryInterrupt, nil
	default:
		return "", ErrChatTreeInvalidDelivery
	}
}

func chatTreeDeliveryToBusyBehavior(delivery chatTreeDelivery) SendMessageBusyBehavior {
	if delivery == chatTreeDeliveryInterrupt {
		return SendMessageBusyBehaviorInterrupt
	}
	return SendMessageBusyBehaviorQueue
}

// chatTreeTools returns the tree messaging tools for a root or named chat.
// currentChat is the per-step chat snapshot and messages the per-step
// user-visible history the generation loop loaded, which already contains
// the assistant row whose tool calls are executing.
func (p *Server) chatTreeTools(
	currentChat func() database.Chat,
	messages func() []database.ChatMessage,
) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			sendChatMessageToolName,
			"Send a fire-and-forget message to this chat's parent chat or to "+
				"one of its direct child chats in the chat tree. chat_id is the "+
				"UUID of a direct child chat, or the literal \"parent\" for the "+
				"parent chat; call list_chat_tree to discover them. The message "+
				"is delivered to the target as a user prompt marked as sent by "+
				"this chat. delivery \"queue\" (default) lets the target finish "+
				"its current work first; \"interrupt\" stops the target's current "+
				"work and runs the message next, after any messages already "+
				"queued there. An interrupt is downgraded to queue when the "+
				"target is waiting for a human approval. There is no reply "+
				"channel: the target may answer with its own send_chat_message. "+
				"The result's relation field is the target's position relative "+
				"to this chat (parent or child). Sends are limited per turn, "+
				"rejected attempts count against that limit, and relay chains "+
				"between agents are capped.",
			func(ctx context.Context, args sendChatMessageArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return p.runSendChatMessage(ctx, currentChat(), messages(), args, call)
			},
		),
		fantasy.NewAgentTool(
			listChatTreeToolName,
			"List this chat's neighbors in the chat tree: its parent chat, "+
				"if any, and its unarchived direct child chats, with ids, "+
				"titles, and statuses, for use with send_chat_message.",
			func(ctx context.Context, _ listChatTreeArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return p.runListChatTree(ctx, currentChat())
			},
		),
	}
}

// loadChatTreeSender re-reads the sending chat as its owner and checks it
// may send tree messages.
func (p *Server) loadChatTreeSender(ctx context.Context, snapshot database.Chat) (context.Context, database.Chat, error) {
	ownerCtx, err := chatOwnerContext(ctx, p.db, snapshot.OwnerID)
	if err != nil {
		return nil, database.Chat{}, err
	}
	sender, err := p.db.GetChatByID(ownerCtx, snapshot.ID)
	if err != nil {
		return nil, database.Chat{}, xerrors.Errorf("load sending chat: %w", err)
	}
	if sender.Kind == database.ChatKindSubagent {
		return nil, database.Chat{}, ErrChatTreeSenderNotEligible
	}
	return ownerCtx, sender, nil
}

func (p *Server) runSendChatMessage(
	ctx context.Context,
	snapshot database.Chat,
	history []database.ChatMessage,
	args sendChatMessageArgs,
	call fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
	delivery, err := parseChatTreeDelivery(args.Delivery)
	if err != nil {
		return toolJSONErrorResponse(map[string]any{"error": err.Error()}), nil
	}
	message := strings.TrimSpace(args.Message)
	if message == "" {
		return toolJSONErrorResponse(map[string]any{"error": ErrChatTreeMessageEmpty.Error()}), nil
	}
	if utf8.RuneCountInString(message) > chatTreeMessageMaxRunes {
		return toolJSONErrorResponse(map[string]any{"error": ErrChatTreeMessageTooLong.Error()}), nil
	}

	ownerCtx, sender, err := p.loadChatTreeSender(ctx, snapshot)
	if err != nil {
		return toolJSONErrorResponse(map[string]any{"error": p.chatTreeErrorText(ctx, err, snapshot.ID, uuid.Nil)}), nil
	}
	target, err := resolveChatTreeMessageTarget(ownerCtx, p.db, sender, args.ChatID)
	if err != nil {
		return toolJSONErrorResponse(map[string]any{"error": p.chatTreeErrorText(ctx, err, sender.ID, uuid.Nil)}), nil
	}

	turn, err := chatTreeTurnStateFromHistory(history)
	if err != nil {
		return toolJSONErrorResponse(map[string]any{"error": p.chatTreeErrorText(ctx, err, sender.ID, target.chat.ID)}), nil
	}
	relation := string(target.relation)
	// Rejections before the send record the requested delivery; the
	// effective delivery is only known under the target's lock.
	if turn.nextHop() > chatTreeMessageMaxHops {
		p.metrics.RecordChatTreeMessage(relation, string(delivery), string(chatTreeDeliveryOutcomeRejected))
		return chatTreeErrorResponse(ErrChatTreeMessageRelayLimit, target.chat), nil
	}
	if !turn.allowSend(call.ID) {
		p.metrics.RecordChatTreeMessage(relation, string(delivery), string(chatTreeDeliveryOutcomeRejected))
		return chatTreeErrorResponse(ErrChatTreeMessageTurnLimit, target.chat), nil
	}

	result, err := p.SendMessage(ownerCtx, SendMessageOptions{
		ChatID:    target.chat.ID,
		CreatedBy: sender.OwnerID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageSenderChat(sender.ID, sender.Title, target.senderRelation(), turn.nextHop()),
			codersdk.ChatMessageText(message),
		},
		BusyBehavior: chatTreeDeliveryToBusyBehavior(delivery),
		// A pending human approval on the target must not be canceled by
		// another agent; the status is checked under the target's lock.
		QueueOnRequiresAction: true,
	})
	if err != nil {
		p.metrics.RecordChatTreeMessage(relation, string(delivery), string(chatTreeDeliveryOutcomeRejected))
		// A failed hook dispatch must fail the turn instead of degrading
		// into a tool error the model can ignore.
		if _, ok := errors.AsType[*dispatch.Error](err); ok {
			return fantasy.ToolResponse{}, err
		}
		return toolJSONErrorResponse(map[string]any{
			"error":   p.chatTreeErrorText(ctx, err, sender.ID, target.chat.ID),
			"chat_id": target.chat.ID.String(),
			"title":   target.chat.Title,
		}), nil
	}

	effectiveDelivery := delivery
	if result.Downgraded {
		effectiveDelivery = chatTreeDeliveryQueue
	}
	outcome := chatTreeDeliveryOutcomeQueued
	switch {
	case !result.Queued:
		outcome = chatTreeDeliveryOutcomeStarted
	case effectiveDelivery == chatTreeDeliveryInterrupt && result.PreviousStatus == database.ChatStatusRunning:
		outcome = chatTreeDeliveryOutcomeInterrupting
	}
	p.metrics.RecordChatTreeMessage(relation, string(effectiveDelivery), string(outcome))

	response := map[string]any{
		"chat_id":         target.chat.ID.String(),
		"title":           target.chat.Title,
		"relation":        relation,
		"previous_status": string(result.PreviousStatus),
		"status":          string(result.Chat.Status),
		"delivery":        string(outcome),
		"relay_hop":       turn.nextHop(),
	}
	if result.Downgraded {
		response["downgraded_from"] = string(chatTreeDeliveryInterrupt)
	}
	if result.Queued && result.QueuedMessage != nil {
		response["queued_message_id"] = result.QueuedMessage.ID
	} else if !result.Queued {
		response["message_id"] = result.Message.ID
	}
	return toolJSONResponse(response), nil
}

func chatTreeErrorResponse(err error, target database.Chat) fantasy.ToolResponse {
	return toolJSONErrorResponse(map[string]any{
		"error":   err.Error(),
		"chat_id": target.ID.String(),
		"title":   target.Title,
	})
}

// chatTreeErrorText returns the error text placed in a tool result. Typed
// and sentinel errors are reported by their own message without wrapping
// context; anything else is logged with the chat ids and replaced by a
// fixed message.
func (p *Server) chatTreeErrorText(ctx context.Context, err error, senderID, targetID uuid.UUID) string {
	if reportable := chatTreeReportableError(err); reportable != nil {
		return reportable.Error()
	}
	p.logger.Warn(ctx, "chat tree message failed",
		slog.F("sender_chat_id", senderID),
		slog.F("target_chat_id", targetID),
		slog.Error(err),
	)
	return errChatTreeInternal.Error()
}

// chatTreeReportableError returns the typed or sentinel error in err's chain
// whose message may be shown to the model, or nil when there is none.
func chatTreeReportableError(err error) error {
	for _, sentinel := range []error{
		ErrChatArchived,
		ErrNoDefaultChatModelConfig,
		ErrChatTreeNotNeighbour,
		ErrChatTreeNoParent,
		ErrChatTreeMessageEmpty,
		ErrChatTreeMessageTooLong,
		ErrChatTreeMessageRelayLimit,
		ErrChatTreeMessageTurnLimit,
		ErrChatTreeInvalidDelivery,
		ErrChatTreeInvalidChatID,
		ErrChatTreeSenderNotEligible,
		ErrChatOwnerInactive,
		errChatTreeHistoryUnavailable,
	} {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	if queueFull, ok := errors.AsType[*chatstate.MessageQueueFullError](err); ok {
		return queueFull
	}
	if denied, ok := errors.AsType[*chathooks.UserPromptDeniedError](err); ok {
		return denied
	}
	return nil
}

// sortChatTreeChildren orders children newest first, with the chat id as
// the tiebreaker so equal timestamps produce a stable order.
func sortChatTreeChildren(rows []database.GetChildChatsByParentIDsRow) {
	slices.SortStableFunc(rows, func(a, b database.GetChildChatsByParentIDsRow) int {
		if c := b.Chat.UpdatedAt.Compare(a.Chat.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.Chat.ID.String(), b.Chat.ID.String())
	})
}

func (p *Server) runListChatTree(ctx context.Context, snapshot database.Chat) (fantasy.ToolResponse, error) {
	ownerCtx, sender, err := p.loadChatTreeSender(ctx, snapshot)
	if err != nil {
		return toolJSONErrorResponse(map[string]any{"error": p.chatTreeErrorText(ctx, err, snapshot.ID, uuid.Nil)}), nil
	}

	response := map[string]any{
		"self": map[string]any{
			"chat_id": sender.ID.String(),
			"title":   sender.Title,
			"kind":    string(sender.Kind),
		},
		"parent": nil,
	}
	if sender.ParentChatID.Valid {
		parent, err := p.db.GetChatByID(ownerCtx, sender.ParentChatID.UUID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) && !dbauthz.IsNotAuthorizedError(err) {
			return toolJSONErrorResponse(map[string]any{"error": p.chatTreeErrorText(ctx, xerrors.Errorf("load parent chat: %w", err), sender.ID, uuid.Nil)}), nil
		}
		if err == nil && parent.Kind != database.ChatKindSubagent {
			response["parent"] = map[string]any{
				"chat_id": parent.ID.String(),
				"title":   parent.Title,
				"kind":    string(parent.Kind),
				"status":  string(parent.Status),
			}
		}
	}

	rows, err := p.db.GetChildChatsByParentIDs(ownerCtx, database.GetChildChatsByParentIDsParams{
		ParentIds: []uuid.UUID{sender.ID},
		Kinds:     []database.ChatKind{database.ChatKindChat},
		Archived:  sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return toolJSONErrorResponse(map[string]any{"error": p.chatTreeErrorText(ctx, xerrors.Errorf("list child chats: %w", err), sender.ID, uuid.Nil)}), nil
	}
	sortChatTreeChildren(rows)
	children := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		children = append(children, map[string]any{
			"chat_id":    row.Chat.ID.String(),
			"title":      row.Chat.Title,
			"status":     string(row.Chat.Status),
			"updated_at": row.Chat.UpdatedAt,
		})
	}
	response["children"] = children
	return toolJSONResponse(response), nil
}
