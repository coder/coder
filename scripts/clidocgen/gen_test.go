package main

import (
	_ "embed"
	"testing"
)

func Test_parseEnv(t *testing.T) {
	t.Parallel()

	type args struct {
		flagUsage string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"no env",
			args{"Perform a trial run with no changes made, showing a diff at the end."},
			"",
		},
		{
			"env",
			args{`Specifies the path to an SSH config.
			Consumes $CODER_SSH_CONFIG_FILE`},
			"$CODER_SSH_CONFIG_FILE",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseEnv(tt.args.flagUsage); got != tt.want {
				t.Errorf("parseEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
