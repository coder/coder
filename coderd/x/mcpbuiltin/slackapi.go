package mcpbuiltin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

const (
	defaultSlackAPIBaseURL = "https://slack.com/api"
	// maxSlackResponseBytes caps one Slack Web API response body.
	maxSlackResponseBytes = 4 << 20
	// maxSlackMessageRunes is Slack's limit for one section block's text.
	maxSlackMessageRunes = 3000
)

// slackAPI calls the Slack Web API with one caller's token.
type slackAPI struct {
	baseURL *url.URL
	// token is the caller's Slack token. It must never be logged.
	token  string
	client *http.Client
}

// slackAPIError is an error the Slack Web API reported.
type slackAPIError struct {
	method     string
	code       string
	needed     string
	retryAfter string
	status     int
}

func (e *slackAPIError) Error() string {
	switch {
	case e.status == http.StatusTooManyRequests:
		msg := fmt.Sprintf("Slack rate limited %s", e.method)
		if e.retryAfter != "" {
			msg += fmt.Sprintf("; retry after %s seconds", e.retryAfter)
		}
		return msg + "."
	case e.status != 0:
		return fmt.Sprintf("Slack returned HTTP %d for %s.", e.status, e.method)
	}
	msg := fmt.Sprintf("Slack rejected %s: %s.", e.method, e.code)
	switch e.code {
	case "missing_scope":
		if e.needed != "" {
			msg += fmt.Sprintf(" The Slack app needs the %s scope.", e.needed)
		}
	case "not_in_channel":
		msg += " Invite the Slack app to the channel first."
	case "channel_not_found", "thread_not_found", "message_not_found":
		msg += " Check the channel ID and message timestamp."
	case "invalid_auth", "not_authed", "token_revoked", "token_expired", "account_inactive":
		msg += " The Slack token is invalid; reconnect the Slack MCP server with a valid token."
	case "not_allowed_token_type":
		msg += " This method needs a different token type, for example a user token for search."
	}
	return msg
}

// call posts a form-encoded Web API request, the encoding the official
// Slack SDKs use. Non-string params must already be JSON encoded. When
// out is non-nil the full response is decoded into it.
func (s slackAPI) call(ctx context.Context, method string, params url.Values, out any) error {
	endpoint := s.baseURL.JoinPath(method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(params.Encode()))
	if err != nil {
		return xerrors.Errorf("build Slack request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return xerrors.Errorf("call Slack %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return &slackAPIError{method: method, status: resp.StatusCode, retryAfter: resp.Header.Get("Retry-After")}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &slackAPIError{method: method, status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSlackResponseBytes+1))
	if err != nil {
		return xerrors.Errorf("read Slack %s response: %w", method, err)
	}
	if len(body) > maxSlackResponseBytes {
		return xerrors.Errorf("slack %s response exceeds %d bytes", method, maxSlackResponseBytes)
	}
	var envelope struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Needed string `json:"needed"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return xerrors.Errorf("decode Slack %s response: %w", method, err)
	}
	if !envelope.OK {
		code := envelope.Error
		if code == "" {
			code = "unknown_error"
		}
		return &slackAPIError{method: method, code: code, needed: envelope.Needed}
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return xerrors.Errorf("decode Slack %s response: %w", method, err)
		}
	}
	return nil
}

func jsonParam(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Only called with plain maps and slices of strings.
		panic(fmt.Sprintf("developer error: marshal Slack param: %v", err))
	}
	return string(b)
}

var (
	slackCodePattern      = regexp.MustCompile("```[\\s\\S]*?```|`[^`]+`")
	slackLinkPattern      = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	slackBoldPattern      = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	slackMentionPattern   = regexp.MustCompile(`<@([A-Za-z0-9._-]+)>`)
	slackUserIDPattern    = regexp.MustCompile(`^[UW][A-Z0-9]{8,}$`)
	slackPlaceholderStart = "\x00CODE"
	// slackBroadcastPattern matches special mentions that notify a whole
	// channel, workspace, or user group.
	slackBroadcastPattern = regexp.MustCompile(`<!(channel|here|everyone|subteam\^[A-Za-z0-9]+)(\|([^>]*))?>`)
)

// formatSlackMessage converts common Markdown to Slack mrkdwn and caps the
// text at Slack's section limit. Code spans are left untouched.
func formatSlackMessage(text string) (formatted string, truncated bool, originalLength int) {
	runes := []rune(text)
	originalLength = len(runes)
	if len(runes) > maxSlackMessageRunes {
		text = string(runes[:maxSlackMessageRunes])
		truncated = true
	}

	var preserved []string
	text = slackCodePattern.ReplaceAllStringFunc(text, func(match string) string {
		preserved = append(preserved, match)
		return slackPlaceholderStart + strconv.Itoa(len(preserved)-1) + "\x00"
	})
	text = slackLinkPattern.ReplaceAllString(text, "<$2|$1>")
	text = slackBoldPattern.ReplaceAllString(text, "*$1*")
	text = slackMentionPattern.ReplaceAllStringFunc(text, func(match string) string {
		handle := slackMentionPattern.FindStringSubmatch(match)[1]
		if slackUserIDPattern.MatchString(handle) {
			return match
		}
		return "@" + handle
	})
	for i, code := range preserved {
		text = strings.Replace(text, slackPlaceholderStart+strconv.Itoa(i)+"\x00", code, 1)
	}
	return text, truncated, originalLength
}

// neutralizeSlackBroadcasts turns @channel, @here, @everyone, and user
// group mentions into plain text, including inside code spans, so model
// output cannot notify everyone unless asked to.
func neutralizeSlackBroadcasts(text string) string {
	return slackBroadcastPattern.ReplaceAllStringFunc(text, func(match string) string {
		groups := slackBroadcastPattern.FindStringSubmatch(match)
		if label := strings.TrimPrefix(groups[3], "@"); label != "" {
			return "@" + label
		}
		if strings.HasPrefix(groups[1], "subteam^") {
			return "@group"
		}
		return "@" + groups[1]
	})
}
