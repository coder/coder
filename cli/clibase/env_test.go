package clibase_test

import (
	"reflect"
	"testing"

	"github.com/coder/coder/cli/clibase"
)

func TestFilterNamePrefix(t *testing.T) {
	t.Parallel()
	type args struct {
		environ []string
		prefix  string
	}
	tests := []struct {
		name string
		args args
		want clibase.Environ
	}{
		{"empty", args{[]string{}, "SHIRE"}, nil},
		{
			"ONE",
			args{
				[]string{
					"SHIRE_BRANDYBUCK=hmm",
				},
				"SHIRE_",
			},
			[]clibase.EnvVar{
				{Name: "BRANDYBUCK", Value: "hmm"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := clibase.ParseEnviron(tt.args.environ, tt.args.prefix); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterNamePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}
