package envparse_test

import (
	"reflect"
	"testing"

	"github.com/coder/coder/cli/envparse"
)

func TestParse(t *testing.T) {
	t.Parallel()
	type args struct {
		line string
	}
	tests := []struct {
		name string
		args args
		want envparse.Var
	}{
		{"empty", args{""}, envparse.Var{}},
		{"onlykey", args{"GANDALF"}, envparse.Var{
			Name: "GANDALF",
		}},
		{"onlyval", args{"=WIZARD"}, envparse.Var{Value: "WIZARD"}},
		{"both", args{"GANDALF=WIZARD"}, envparse.Var{
			Name: "GANDALF", Value: "WIZARD",
		}},
		{"nameAlwaysUpper", args{"gandalf=WIZARD"}, envparse.Var{
			Name: "GANDALF", Value: "WIZARD",
		}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := envparse.Parse(tt.args.line); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterNamePrefix(t *testing.T) {
	t.Parallel()
	type args struct {
		environ []string
		prefix  string
	}
	tests := []struct {
		name string
		args args
		want []envparse.Var
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
			[]envparse.Var{
				{Name: "BRANDYBUCK", Value: "hmm"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := envparse.FilterNamePrefix(tt.args.environ, tt.args.prefix); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterNamePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}
