package bigcli_test

import (
	"reflect"
	"testing"

	"github.com/coder/coder/cli/bigcli"
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
		want []bigcli.EnvVar
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
			[]bigcli.EnvVar{
				{Name: "BRANDYBUCK", Value: "hmm"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := bigcli.EnvsWithPrefix(tt.args.environ, tt.args.prefix); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EnvsWithPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}
