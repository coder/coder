package chatautomation

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// MaxAutomationsPerOwner is how many automations one user can own across
	// all organizations.
	MaxAutomationsPerOwner = 50
	// minScheduleInterval is the shortest time between two runs of a
	// schedule.
	minScheduleInterval = 5 * time.Minute
	maxNameChars        = 128
	// maxPromptBytes matches the default limit of the other chat prompts.
	maxPromptBytes = 128 << 10
)

// violations collects the rejected fields of a request. No detail quotes the
// prompt.
type violations []codersdk.ValidationError

func (v *violations) add(field, detail string) {
	*v = append(*v, codersdk.ValidationError{Field: field, Detail: detail})
}

// validateRequest checks every field of req that needs no database read. It
// returns the schedule when the cron expression and the time zone are valid.
func validateRequest(req codersdk.CreateChatAutomationRequest) (*cron.Schedule, violations) {
	var v violations
	switch name := strings.TrimSpace(req.Name); {
	case name == "":
		v.add("name", "Required.")
	case utf8.RuneCountInString(name) > maxNameChars:
		v.add("name", fmt.Sprintf("Must be at most %d characters.", maxNameChars))
	case strings.ContainsRune(name, 0):
		v.add("name", "Must not contain NUL characters.")
	}
	switch {
	case strings.TrimSpace(req.Prompt) == "":
		v.add("prompt", "Required.")
	case len(req.Prompt) > maxPromptBytes:
		v.add("prompt", fmt.Sprintf("Must be at most %d bytes.", maxPromptBytes))
	case strings.ContainsRune(req.Prompt, 0):
		v.add("prompt", "Must not contain NUL characters.")
	}

	switch req.TargetMode {
	case codersdk.ChatAutomationTargetModeExistingChat:
		if req.TargetChatID == nil {
			v.add("target_chat_id", "Required when target_mode is existing_chat.")
		}
		// An existing chat keeps its model and project.
		if req.NewChatModelConfigID != nil {
			v.add("new_chat_model_config_id", "Not allowed when target_mode is existing_chat.")
		}
		if req.ProjectID != nil {
			v.add("project_id", "Not allowed when target_mode is existing_chat.")
		}
	case codersdk.ChatAutomationTargetModeNewChat:
		if req.NewChatModelConfigID == nil {
			v.add("new_chat_model_config_id", "Required when target_mode is new_chat.")
		}
		if req.TargetChatID != nil {
			v.add("target_chat_id", "Not allowed when target_mode is new_chat.")
		}
	default:
		v.add("target_mode", "Must be existing_chat or new_chat.")
	}

	// A valid expression never fails because of the zone, so checking the
	// zone first names the field that is wrong.
	if _, err := cron.Standard("* * * * *", req.ScheduleTimeZone); err != nil {
		v.add("schedule_time_zone", err.Error())
		return nil, v
	}
	sched, err := cron.Standard(req.ScheduleCron, req.ScheduleTimeZone)
	switch {
	case err != nil:
		v.add("schedule_cron", err.Error())
		return nil, v
	case sched.MinInterval() < minScheduleInterval:
		v.add("schedule_cron", fmt.Sprintf("Must not run more often than every %d minutes.", int(minScheduleInterval/time.Minute)))
		return nil, v
	}
	return sched, v
}
