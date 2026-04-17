package cli_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func expAgentsPtr[T any](v T) *T {
	return &v
}

func setupExpAgentsBackend(t *testing.T) (*codersdk.Client, *codersdk.ExperimentalClient, uuid.UUID) {
	t.Helper()

	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentAgents)}

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: values,
	})
	firstUser := coderdtest.CreateFirstUser(t, client)

	expClient := codersdk.NewExperimentalClient(client)
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
	})
	require.NoError(t, err)

	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: expAgentsPtr(int64(4096)),
		IsDefault:    expAgentsPtr(true),
	})
	require.NoError(t, err)

	return client, expClient, firstUser.OrganizationID
}

//nolint:revive // Test helper signature keeps t first for consistency with other helpers.
func seedChat(t *testing.T, ctx context.Context, expClient *codersdk.ExperimentalClient, orgID uuid.UUID, seed string) codersdk.Chat {
	t.Helper()

	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: seed,
			},
		},
	})
	require.NoError(t, err)
	return chat
}

type expAgentsSession struct {
	t     *testing.T
	pty   *ptytest.PTY
	errCh <-chan error
}

func (s *expAgentsSession) expect(ctx context.Context, text string) {
	s.t.Helper()
	s.pty.ExpectMatchContext(ctx, text)
}

func (s *expAgentsSession) wait(ctx context.Context) error {
	s.t.Helper()
	return testutil.RequireReceive(ctx, s.t, s.errCh)
}

//nolint:unused // Kept as a small PTY helper for future multi-character input.
func (s *expAgentsSession) write(text string) {
	s.t.Helper()
	s.pty.WriteLine(text)
}

func (s *expAgentsSession) writeRune(r rune) {
	s.t.Helper()
	_, err := s.pty.Input().Write([]byte(string(r)))
	require.NoError(s.t, err)
}

func (s *expAgentsSession) enter() {
	s.t.Helper()
	_, err := s.pty.Input().Write([]byte("\r"))
	require.NoError(s.t, err)
}

func (s *expAgentsSession) esc() {
	s.t.Helper()
	_, err := s.pty.Input().Write([]byte("\x1b"))
	require.NoError(s.t, err)
}

func (s *expAgentsSession) ctrlC() {
	s.t.Helper()
	_, err := s.pty.Input().Write([]byte{3})
	require.NoError(s.t, err)
}

func (s *expAgentsSession) quit() {
	s.t.Helper()
	s.writeRune('q')
}

//nolint:revive // Test helper signature keeps t first for consistency with other helpers.
func startExpAgentsSession(t *testing.T, ctx context.Context, client *codersdk.Client, args ...string) *expAgentsSession {
	t.Helper()

	// Reading to / writing from the PTY is flaky on non-linux systems.
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	fullArgs := append([]string{"exp", "agents"}, args...)
	inv, root := clitest.New(t, fullArgs...)
	clitest.SetupConfig(t, client, root)

	pty := ptytest.New(t)
	tty, err := os.OpenFile(pty.Name(), os.O_RDWR, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tty.Close()
	})

	inv.Stdin = tty
	inv.Stdout = tty
	inv.Stderr = tty

	errCh := make(chan error, 1)
	tGo(t, func() {
		errCh <- inv.WithContext(ctx).Run()
	})

	return &expAgentsSession{t: t, pty: pty, errCh: errCh}
}
