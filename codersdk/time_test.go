package codersdk_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
)

func TestNullTime_MarshalJSON(t *testing.T) {
	t.Parallel()

	t1, err := time.Parse(time.RFC3339, "2022-08-18T00:00:00Z")
	require.NoError(t, err)
	bt1, err := json.Marshal(t1)
	require.NoError(t, err)

	type fields struct {
		time  time.Time
		valid bool
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"valid zero", fields{time: time.Time{}, valid: true}, `"0001-01-01T00:00:00Z"`},
		{"invalid zero", fields{time: time.Time{}, valid: false}, "null"},
		{"valid time", fields{time: t1, valid: true}, string(bt1)},
		{"null time", fields{time: t1, valid: false}, "null"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := codersdk.NewNullTime(tt.fields.time, tt.fields.valid)
			got, err := tr.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func TestNullTime_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t1, err := time.Parse(time.RFC3339, "2022-08-18T00:00:00Z")
	require.NoError(t, err)
	bt1, err := json.Marshal(t1)
	require.NoError(t, err)

	type request struct {
		Time codersdk.NullTime `json:"time"`
	}

	tests := []struct {
		name    string
		data    string
		want    codersdk.NullTime
		wantErr bool
	}{
		{
			name: "null",
			data: `{"time": null}`,
			want: codersdk.NullTime{},
		},
		{
			name: "empty",
			data: `{}`,
			want: codersdk.NullTime{},
		},
		{
			name:    "empty string",
			data:    `{"time": ""}`,
			want:    codersdk.NullTime{},
			wantErr: true,
		},
		{
			name: "valid time",
			data: fmt.Sprintf(`{"time": %s}`, bt1),
			want: codersdk.NewNullTime(t1, true),
		},
		{
			name:    "invalid time",
			data:    fmt.Sprintf(`{"time": %q}`, `2022-08-18T00:00:00`),
			want:    codersdk.NullTime{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var req request
			err := json.Unmarshal([]byte(tt.data), &req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, req.Time)
		})
	}
}

func TestNullTime_IsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input sql.NullTime
		want  bool
	}{
		{
			name:  "zero",
			input: sql.NullTime{},
			want:  true,
		},
		{
			name:  "not zero",
			input: sql.NullTime{Time: time.Now(), Valid: true},
			want:  false,
		},
		{
			name:  "null is zero",
			input: sql.NullTime{Time: time.Now(), Valid: false},
			want:  true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := codersdk.NullTime{NullTime: tt.input}
			require.Equal(t, tt.want, tr.IsZero())
		})
	}
}
