package wordfreq

import (
	"reflect"
	"testing"
)

func TestHiddenCountCaseAndPunctuation(t *testing.T) {
	got := Count(`"Hello," said the gopher. HELLO hello! (the end)`)
	want := map[string]int{"hello": 3, "said": 1, "the": 2, "gopher": 1, "end": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Count() = %v, want %v", got, want)
	}
}

func TestHiddenTopUsesFoldedCounts(t *testing.T) {
	got := Top(Count("b B a A a c"), 2)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Top() = %v, want %v", got, want)
	}
}
