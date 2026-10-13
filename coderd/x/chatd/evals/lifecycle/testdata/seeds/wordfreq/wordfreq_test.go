package wordfreq

import (
	"reflect"
	"testing"
)

func TestCountIgnoresCase(t *testing.T) {
	got := Count("Go go GO, gopher.")
	want := map[string]int{"go": 3, "gopher": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Count() = %v, want %v", got, want)
	}
}

func TestTop(t *testing.T) {
	got := Top(map[string]int{"a": 1, "b": 3, "c": 3}, 2)
	want := []string{"b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Top() = %v, want %v", got, want)
	}
}
