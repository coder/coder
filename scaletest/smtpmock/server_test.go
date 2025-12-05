package smtpmock_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/scaletest/smtpmock"
	"github.com/coder/coder/v2/testutil"
)

func TestServer_StartStop(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(smtpmock.Server)
	err := srv.Start(ctx, smtpmock.Config{
		HostAddress: "127.0.0.1",
		SMTPPort:    0,
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	require.NotEmpty(t, srv.SMTPAddress())
	require.NotEmpty(t, srv.APIAddress())

	err = srv.Stop()
	require.NoError(t, err)
}

func TestServer_SendAndReceiveEmail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(smtpmock.Server)
	err := srv.Start(ctx, smtpmock.Config{
		HostAddress: "127.0.0.1",
		SMTPPort:    0,
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	err = sendTestEmail(srv.SMTPAddress(), "test@example.com", "Test Subject", "Test Body")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return srv.MessageCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	url := fmt.Sprintf("%s/messages", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var summaries []smtpmock.EmailSummary
	err = json.NewDecoder(resp.Body).Decode(&summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, "Test Subject", summaries[0].Subject)
}

func TestServer_FilterByEmail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(smtpmock.Server)
	err := srv.Start(ctx, smtpmock.Config{
		HostAddress: "127.0.0.1",
		SMTPPort:    0,
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	err = sendTestEmail(srv.SMTPAddress(), "admin@coder.com", "Email for admin", "Body 1")
	require.NoError(t, err)

	err = sendTestEmail(srv.SMTPAddress(), "test-user@coder.com", "Email for test-user", "Body 2")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return srv.MessageCount() == 2
	}, testutil.WaitShort, testutil.IntervalMedium)

	url := fmt.Sprintf("%s/messages?email=admin@coder.com", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var summaries []smtpmock.EmailSummary
	err = json.NewDecoder(resp.Body).Decode(&summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, "Email for admin", summaries[0].Subject)
}

func TestServer_AlertTemplateID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(smtpmock.Server)
	err := srv.Start(ctx, smtpmock.Config{
		HostAddress: "127.0.0.1",
		SMTPPort:    0,
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	notificationID := uuid.New()
	body := fmt.Sprintf(`<p><a href=3D"http://127.0.0.1:3000/settings/notifications?disabled=3D%s">Unsubscribe</a></p>`, notificationID.String())

	err = sendTestEmail(srv.SMTPAddress(), "test-user@coder.com", "Notification", body)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return srv.MessageCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	url := fmt.Sprintf("%s/messages", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var summaries []smtpmock.EmailSummary
	err = json.NewDecoder(resp.Body).Decode(&summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, notificationID, summaries[0].AlertTemplateID)
}

func TestServer_Purge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(smtpmock.Server)
	err := srv.Start(ctx, smtpmock.Config{
		HostAddress: "127.0.0.1",
		SMTPPort:    0,
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	err = sendTestEmail(srv.SMTPAddress(), "test-user@coder.com", "Test", "Body")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return srv.MessageCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	url := fmt.Sprintf("%s/purge", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, 0, srv.MessageCount())
}

func sendTestEmail(smtpAddr, to, subject, body string) error {
	from := "noreply@coder.com"
	now := time.Now().Format(time.RFC1123Z)

	msg := strings.Builder{}
	_, _ = msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	_, _ = msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	_, _ = msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	_, _ = msg.WriteString(fmt.Sprintf("Date: %s\r\n", now))
	_, _ = msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	_, _ = msg.WriteString("\r\n")
	_, _ = msg.WriteString(body)

	return smtp.SendMail(smtpAddr, nil, from, []string{to}, []byte(msg.String()))
}
