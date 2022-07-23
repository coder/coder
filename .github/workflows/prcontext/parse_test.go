package main

import (
	"reflect"
	"testing"
)

func Test_parseBody_basic(t *testing.T) {
	parseBody(`
This is a test PR.	

[ci-skip postgres windows]
	`)
}

func Test_parseBody(t *testing.T) {
	type args struct {
		body string
	}
	tests := []struct {
		name      string
		args      args
		wantSkips []string
	}{
		{"no directive", args{"test pr 123\n\n"}, nil},
		{"single dir single skip", args{"test pr [ci-skip dog] 123\n\n"}, []string{"dog"}},
		{"double dir double skip", args{"test pr [ci-skip dog] [ci-skip cat] 123\n\n"}, []string{"dog", "cat"}},
		{"single dir double skip", args{"test pr [ci-skip dog cat] 123\n\n"}, []string{"dog", "cat"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotSkips := parseBody(tt.args.body); !reflect.DeepEqual(gotSkips, tt.wantSkips) {
				t.Errorf("parseBody() = %v, want %v", gotSkips, tt.wantSkips)
			}
		})
	}
}
