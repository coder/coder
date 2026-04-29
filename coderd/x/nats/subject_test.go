package nats_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/nats"
)

func TestLegacyEventSubject(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			event string
			want  nats.Subject
		}{
			{"Simple", "workspace", "coder.v1.pubsub.workspace"},
			{"WithUUID", "workspace_owner:11111111-1111-1111-1111-111111111111", "coder.v1.pubsub.workspace_owner.11111111-1111-1111-1111-111111111111"},
			{"MultiColon", "inbox_notification:owner:22222222-2222-2222-2222-222222222222", "coder.v1.pubsub.inbox_notification.owner.22222222-2222-2222-2222-222222222222"},
			{"Hyphen", "agent-logs:33333333-3333-3333-3333-333333333333", "coder.v1.pubsub.agent-logs.33333333-3333-3333-3333-333333333333"},
			{"ChatStream", "chat:stream:44444444-4444-4444-4444-444444444444", "coder.v1.pubsub.chat.stream.44444444-4444-4444-4444-444444444444"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got, err := nats.LegacyEventSubject(tc.event)
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
				// The output must also pass ValidateSubject.
				assert.NoError(t, nats.ValidateSubject(string(got)))
			})
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			event string
		}{
			{"Empty", ""},
			{"LeadingColon", ":foo"},
			{"TrailingColon", "foo:"},
			{"DoubleColon", "foo::bar"},
			{"Space", "foo bar"},
			{"Tab", "foo\tbar"},
			{"Newline", "foo\nbar"},
			{"Star", "foo:*"},
			{"Gt", "foo:>"},
			{"Dot", "foo.bar"},
			{"NonASCII", "café"},
			{"Slash", "foo/bar"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := nats.LegacyEventSubject(tc.event)
				require.Error(t, err)
			})
		}
	})

	t.Run("EmptyIsEmptySubject", func(t *testing.T) {
		t.Parallel()
		_, err := nats.LegacyEventSubject("")
		require.ErrorIs(t, err, nats.ErrEmptySubject)
	})

	t.Run("InvalidIsInvalidToken", func(t *testing.T) {
		t.Parallel()
		_, err := nats.LegacyEventSubject("foo::bar")
		require.ErrorIs(t, err, nats.ErrInvalidToken)
	})
}

func TestBuildSubject(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		got, err := nats.BuildSubject("tailnet", "peer", "abc")
		require.NoError(t, err)
		assert.Equal(t, nats.Subject("coder.v1.tailnet.peer.abc"), got)
		assert.NoError(t, nats.ValidateSubject(string(got)))
	})

	t.Run("SingleToken", func(t *testing.T) {
		t.Parallel()
		got, err := nats.BuildSubject("workspace", "abc-123")
		require.NoError(t, err)
		assert.Equal(t, nats.Subject("coder.v1.workspace.abc-123"), got)
	})

	t.Run("EmptyDomain", func(t *testing.T) {
		t.Parallel()
		_, err := nats.BuildSubject("", "tok")
		require.Error(t, err)
		require.ErrorIs(t, err, nats.ErrInvalidToken)
	})

	t.Run("NoTokens", func(t *testing.T) {
		t.Parallel()
		_, err := nats.BuildSubject("tailnet")
		require.Error(t, err)
		require.ErrorIs(t, err, nats.ErrEmptySubject)
	})

	t.Run("EmptyToken", func(t *testing.T) {
		t.Parallel()
		_, err := nats.BuildSubject("tailnet", "peer", "")
		require.Error(t, err)
		require.ErrorIs(t, err, nats.ErrInvalidToken)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()
		cases := []string{"foo*", "foo>", "foo.bar", "foo bar", "café"}
		for _, c := range cases {
			c := c
			t.Run(c, func(t *testing.T) {
				t.Parallel()
				_, err := nats.BuildSubject("tailnet", c)
				require.Error(t, err)
				require.ErrorIs(t, err, nats.ErrInvalidToken)
			})
		}
	})

	t.Run("InvalidDomain", func(t *testing.T) {
		t.Parallel()
		_, err := nats.BuildSubject("tail*net", "peer")
		require.Error(t, err)
		require.ErrorIs(t, err, nats.ErrInvalidToken)
	})
}

func TestValidateSubject(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		built, err := nats.BuildSubject("tailnet", "peer", "abc")
		require.NoError(t, err)
		require.NoError(t, nats.ValidateSubject(string(built)))

		require.NoError(t, nats.ValidateSubject("coder.v1.foo"))
		require.NoError(t, nats.ValidateSubject("coder.v1.foo.bar.baz"))
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name    string
			subject string
		}{
			{"Empty", ""},
			{"MissingPrefix", "other.v1.foo"},
			{"PrefixOnly", "coder.v1"},
			{"PrefixOnlyWithDot", "coder.v1."},
			{"WildcardStar", "coder.v1.foo.*"},
			{"WildcardGt", "coder.v1.foo.>"},
			{"EmptyTokenBetweenDots", "coder.v1.foo..bar"},
			{"NonASCII", "coder.v1.café"},
			{"Whitespace", "coder.v1.foo bar"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				err := nats.ValidateSubject(tc.subject)
				require.Error(t, err)
				// Should be one of our sentinels.
				assert.True(t,
					errors.Is(err, nats.ErrInvalidSubject) ||
						errors.Is(err, nats.ErrInvalidToken) ||
						errors.Is(err, nats.ErrEmptySubject),
					"expected sentinel error, got %v", err)
			})
		}
	})
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		cases := []string{
			"foo",
			"FOO",
			"foo123",
			"foo_bar",
			"foo-bar",
			"a",
			"A1_b-2",
			"11111111-1111-1111-1111-111111111111",
			"workspace_owner",
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc, func(t *testing.T) {
				t.Parallel()
				assert.NoError(t, nats.ValidateToken(tc))
			})
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			token string
		}{
			{"Empty", ""},
			{"Space", "foo bar"},
			{"LeadingSpace", " foo"},
			{"TrailingSpace", "foo "},
			{"Tab", "foo\tbar"},
			{"Newline", "foo\nbar"},
			{"Star", "*"},
			{"StarSuffix", "foo*"},
			{"Gt", ">"},
			{"GtSuffix", "foo>"},
			{"Dot", "foo.bar"},
			{"NonASCII", "café"},
			{"Emoji", "foo🎉"},
			{"Slash", "foo/bar"},
			{"Colon", "foo:bar"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				err := nats.ValidateToken(tc.token)
				require.Error(t, err)
				require.ErrorIs(t, err, nats.ErrInvalidToken)
			})
		}
	})
}
