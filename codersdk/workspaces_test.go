package codersdk_test

import (
	"reflect"
	"testing"

	"github.com/coder/coder/codersdk"
)

func TestOrphanTerraformState(t *testing.T) {
	t.Parallel()

	type args struct {
		state []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{"invalid json", args{[]byte("---")}, nil, true},
		{"no resources", args{[]byte(`{"a":4}`)}, nil, true},
		{"some resources", args{[]byte(`{"a":4, "resources":[1, 2, 3]}`)}, []byte(`{"a":4,"resources":[]}`), false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := codersdk.OrphanTerraformState(tt.args.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("OrphanTerraformState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OrphanTerraformState() = %s, want %s", got, tt.want)
			}
		})
	}
}
