package xjson_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/xjson"
)

func TestParseUUIDList(t *testing.T) {
	t.Parallel()

	a := uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5")
	b := uuid.MustParse("8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818")

	tests := []struct {
		name    string
		input   string
		want    []uuid.UUID
		wantErr string
	}{
		{
			name:  "EmptyString",
			input: "",
			want:  []uuid.UUID{},
		},
		{
			name:  "JSONNull",
			input: "null",
			want:  []uuid.UUID{},
		},
		{
			name:  "WhitespaceOnly",
			input: "  \n\t ",
			want:  []uuid.UUID{},
		},
		{
			name:  "ValidUUIDs",
			input: `["c7c6686d-a93c-4df2-bef9-5f837e9a33d5","8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818"]`,
			want:  []uuid.UUID{a, b},
		},
		{
			name:    "InvalidJSON",
			input:   "not json at all",
			wantErr: "unmarshal uuid list",
		},
		{
			name:    "InvalidUUID",
			input:   `["not-a-uuid"]`,
			wantErr: "parse uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := xjson.ParseUUIDList(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.want, got)
		})
	}
}
