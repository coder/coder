package render_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"

	"github.com/coder/coder/v2/coderd/notifications/types"
)

func TestGoTemplate(t *testing.T) {
	t.Parallel()

	const userEmail = "bob@xyz.com"

	tests := []struct {
		name           string
		in             string
		payload        types.MessagePayload
		expectedOutput string
		expectedErr    error
	}{
		{
			name:           "top-level variables are accessible and substituted",
			in:             "{{ .UserEmail }}",
			payload:        types.MessagePayload{UserEmail: userEmail},
			expectedOutput: userEmail,
			expectedErr:    nil,
		},
		{
			name: "input labels are accessible and substituted",
			in:   "{{ .Labels.user_email }}",
			payload: types.MessagePayload{Labels: map[string]string{
				"user_email": userEmail,
			}},
			expectedOutput: userEmail,
			expectedErr:    nil,
		},
		{
			name: "render workspace URL",
			in: `[{
				"label": "View workspace",
				"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}"
			}]`,
			payload: types.MessagePayload{
				UserName:     "John Doe",
				UserUsername: "johndoe",
				Labels: map[string]string{
					"name": "my-workspace",
				},
			},
			expectedOutput: `[{
				"label": "View workspace",
				"url": "https://mocked-server-address/@johndoe/my-workspace"
			}]`,
		},
		{
			name: "render notification template ID",
			in:   `{{ .NotificationTemplateID }}`,
			payload: types.MessagePayload{
				NotificationTemplateID: "4e19c0ac-94e1-4532-9515-d1801aa283b2",
			},
			expectedOutput: "4e19c0ac-94e1-4532-9515-d1801aa283b2",
			expectedErr:    nil,
		},
	}

	for _, tc := range tests {
		tc := tc // unnecessary as of go1.22 but the linter is outdated

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := render.GoTemplate(tc.in, tc.payload, map[string]any{
				"base_url": func() string { return "https://mocked-server-address" },
			})
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.expectedErr)
			}

			require.Equal(t, tc.expectedOutput, out)
		})
	}
}
