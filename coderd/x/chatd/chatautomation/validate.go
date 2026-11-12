package chatautomation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
	"github.com/coder/coder/v2/codersdk"
)

const (
	maxNameChars = 128
	// The cron and zone bounds apply before cron.Standard runs, because the
	// text of its parse errors quotes the input.
	maxCronBytes = 256
	maxZoneBytes = 64
)

// violations collects the fields of a request that Create rejects. Every
// detail is static text or a limit and never quotes the prompt or the secret.
type violations []codersdk.ValidationError

func (v *violations) add(field, detail string) {
	*v = append(*v, codersdk.ValidationError{Field: field, Detail: detail})
}

// oneOf rejects a value that is empty or not valid. options lists the valid
// values for the detail.
func (v *violations) oneOf(field, value string, valid bool, options string) {
	switch {
	case value == "":
		v.add(field, "Required. Use one of: "+options+".")
	case !valid:
		v.add(field, "Must be one of: "+options+".")
	}
}

type setField struct {
	name string
	set  bool
}

// forbid rejects each field that is set but does not apply. why completes the
// sentence "Not allowed ...".
func (v *violations) forbid(why string, fields ...setField) {
	for _, f := range fields {
		if f.set {
			v.add(f.name, "Not allowed "+why+".")
		}
	}
}

// normalized holds what Create needs from a request that passed validation.
type normalized struct {
	name            string
	enabled         bool
	whenBusy        database.NullChatAutomationWhenBusy
	reasoningEffort database.NullChatReasoningEffort
	// verifier is set for a webhook.
	verifier chatwebhook.WebhookVerifier
	// secret is the trimmed secret that the sender created. It is set for
	// standard_webhooks only.
	secret string
	// schedule is set for a schedule.
	schedule *cron.Schedule
}

// textProblem returns why text cannot be stored, or "" when it can.
func textProblem(text string) string {
	switch {
	case !utf8.ValidString(text):
		return "Must be valid UTF-8."
	case strings.ContainsRune(text, 0):
		return "Must not contain NUL characters."
	}
	return ""
}

// validateRequest checks every field of req without a database read. It
// returns every violation. An error means a bug.
func validateRequest(req codersdk.CreateChatAutomationRequest, limits Limits) (normalized, []codersdk.ValidationError, error) {
	var v violations
	n := normalized{
		name:    strings.TrimSpace(req.Name),
		enabled: req.Enabled == nil || *req.Enabled,
	}

	switch problem := textProblem(n.name); {
	case problem != "":
		v.add("name", problem)
	case n.name == "":
		v.add("name", "Required.")
	case utf8.RuneCountInString(n.name) > maxNameChars:
		v.add("name", fmt.Sprintf("Must be at most %d characters.", maxNameChars))
	}
	switch problem := textProblem(req.Prompt); {
	case problem != "":
		v.add("prompt", problem)
	case strings.TrimSpace(req.Prompt) == "":
		v.add("prompt", "Required.")
	case len(req.Prompt) > limits.MaxPromptBytes:
		v.add("prompt", fmt.Sprintf("Must be at most %d bytes.", limits.MaxPromptBytes))
	}

	v.oneOf("kind", string(req.Kind), database.ChatAutomationKind(req.Kind).Valid(), "webhook, schedule")
	v.oneOf("target_mode", string(req.TargetMode), database.ChatAutomationTargetMode(req.TargetMode).Valid(), "existing_chat, new_chat")

	switch req.TargetMode {
	case codersdk.ChatAutomationTargetModeExistingChat:
		if req.TargetChatID == nil {
			v.add("target_chat_id", "Required when target_mode is existing_chat.")
		}
		whenBusy := database.ChatAutomationWhenBusy(req.WhenBusy)
		if whenBusy == "" {
			whenBusy = database.ChatAutomationWhenBusyQueue
			if req.Kind == codersdk.ChatAutomationKindSchedule {
				whenBusy = database.ChatAutomationWhenBusySkip
			}
		}
		v.oneOf("when_busy", string(whenBusy), whenBusy.Valid(), "queue, skip")
		n.whenBusy = database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: whenBusy, Valid: true}
		v.forbid("when target_mode is existing_chat, because an existing chat keeps its model and reasoning effort",
			setField{"new_chat_model_config_id", req.NewChatModelConfigID != nil},
			setField{"project_id", req.ProjectID != nil},
			setField{"reasoning_effort", req.ReasoningEffort != ""},
		)
	case codersdk.ChatAutomationTargetModeNewChat:
		if req.NewChatModelConfigID == nil {
			v.add("new_chat_model_config_id", "Required when target_mode is new_chat.")
		}
		if effort := database.ChatReasoningEffort(req.ReasoningEffort); effort != "" {
			v.oneOf("reasoning_effort", string(effort), effort.Valid(), "none, minimal, low, medium, high, xhigh, max")
			n.reasoningEffort = database.NullChatReasoningEffort{ChatReasoningEffort: effort, Valid: true}
		}
		v.forbid("when target_mode is new_chat",
			setField{"target_chat_id", req.TargetChatID != nil},
			setField{"when_busy", req.WhenBusy != ""},
		)
	}

	switch req.Kind {
	case codersdk.ChatAutomationKindWebhook:
		auth := database.ChatAutomationWebhookAuth(req.WebhookAuth)
		v.oneOf("webhook_auth", string(auth), auth.Valid(), "bearer, github, standard_webhooks")
		v.forbid("when kind is webhook",
			setField{"schedule_cron", req.ScheduleCron != ""},
			setField{"schedule_time_zone", req.ScheduleTimeZone != ""},
		)
		if !auth.Valid() {
			break
		}
		verifier, err := chatwebhook.NewVerifier(chatwebhook.Scheme(auth))
		if err != nil {
			return normalized{}, nil, xerrors.Errorf("get webhook verifier: %w", err)
		}
		n.verifier = verifier
		if auth != database.ChatAutomationWebhookAuthStandardWebhooks {
			v.forbid("for this webhook_auth, because Coder creates the secret",
				setField{"webhook_secret", req.WebhookSecret != ""})
			break
		}
		n.secret = strings.TrimSpace(req.WebhookSecret)
		switch {
		case n.secret == "":
			v.add("webhook_secret", "Required when webhook_auth is standard_webhooks.")
		case verifier.ValidateSecret(n.secret) != nil:
			v.add("webhook_secret", "Must be a Standard Webhooks secret: whsec_ followed by base64.")
		}
	case codersdk.ChatAutomationKindSchedule:
		v.forbid("when kind is schedule",
			setField{"webhook_auth", req.WebhookAuth != ""},
			setField{"webhook_secret", req.WebhookSecret != ""},
		)
		n.schedule = validateSchedule(&v, req, limits)
	}
	return n, v, nil
}

// validateSchedule checks the cron expression and the time zone, and returns
// the schedule when both are valid.
func validateSchedule(v *violations, req codersdk.CreateChatAutomationRequest, limits Limits) *cron.Schedule {
	complete := true
	for _, f := range []struct {
		field, value string
		maxBytes     int
	}{
		{"schedule_cron", req.ScheduleCron, maxCronBytes},
		{"schedule_time_zone", req.ScheduleTimeZone, maxZoneBytes},
	} {
		switch {
		case f.value == "":
			v.add(f.field, "Required when kind is schedule.")
			complete = false
		case len(f.value) > f.maxBytes:
			v.add(f.field, fmt.Sprintf("Must be at most %d bytes.", f.maxBytes))
			complete = false
		}
	}
	if !complete {
		return nil
	}
	// A valid spec never fails because of the zone, so an error here names
	// the field that caused it.
	if _, err := cron.Standard("* * * * *", req.ScheduleTimeZone); err != nil {
		v.add("schedule_time_zone", err.Error())
		return nil
	}
	sched, err := cron.Standard(req.ScheduleCron, req.ScheduleTimeZone)
	if err != nil {
		v.add("schedule_cron", err.Error())
		return nil
	}
	if sched.MinInterval() < limits.MinInterval {
		v.add("schedule_cron", fmt.Sprintf("Must not run more often than every %s.", limits.MinInterval))
		return nil
	}
	return sched
}
