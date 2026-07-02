package database_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestChatMessageSearchText(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	cases := []struct {
		name    string
		content sql.NullString // JSONB input; invalid means SQL NULL.
		want    sql.NullString // expected text; invalid means SQL NULL.
	}{
		{
			name:    "SingleTextPart",
			content: sql.NullString{String: `[{"type":"text","text":"hello world"}]`, Valid: true},
			want:    sql.NullString{String: "hello world", Valid: true},
		},
		{
			name: "TextInterleavedWithNonText",
			content: sql.NullString{String: `[
				{"type":"text","text":"first"},
				{"type":"reasoning","text":"thinking"},
				{"type":"tool-call","toolName":"execute"},
				{"type":"text","text":"second"}
			]`, Valid: true},
			want: sql.NullString{String: "first second", Valid: true},
		},
		{
			name:    "ScalarContent",
			content: sql.NullString{String: `"hello"`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "EmptyArray",
			content: sql.NullString{String: `[]`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "NullInput",
			content: sql.NullString{},
			want:    sql.NullString{},
		},
		{
			name:    "ElementsMissingTypeOrText",
			content: sql.NullString{String: `[{"text":"no type"},{"type":"text"},{"type":"text","text":"kept"}]`, Valid: true},
			want:    sql.NullString{String: "kept", Valid: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			var got sql.NullString
			err := sqlDB.QueryRowContext(ctx,
				`SELECT chat_message_search_text($1::jsonb)`, tc.content,
			).Scan(&got)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
