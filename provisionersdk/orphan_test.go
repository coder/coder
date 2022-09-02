package provisionersdk_test

import (
	"reflect"
	"testing"

	"github.com/coder/coder/provisionersdk"
)

func TestOrphanState(t *testing.T) {
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

			got, err := provisionersdk.OrphanState(tt.args.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("OrphanState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OrphanState() = %s, want %s", got, tt.want)
			}
		})
	}
}
