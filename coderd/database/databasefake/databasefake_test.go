package databasefake_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/coder/coder/coderd/database"

	"github.com/coder/coder/coderd/database/databasefake"
)

// TestExactMethods will ensure the fake database does not hold onto excessive
// functions. The fake database is a manual implementation, so it is possible
// we forget to delete functions that we remove. This unit test just ensures
// we remove the extra methods.
func TestExactMethods(t *testing.T) {
	// extraFakeMethods contains the extra allowed methods that are not a part
	// of the database.Store interface.
	extraFakeMethods := map[string]string{
		// Example
		// "SortFakeLists": "Helper function used",
	}

	fake := reflect.TypeOf(databasefake.New())
	fakeMethods := methods(fake)

	store := reflect.TypeOf((*database.Store)(nil)).Elem()
	storeMethods := methods(store)

	// Store should be a subset
	for k := range storeMethods {
		_, ok := fakeMethods[k]
		if !ok {
			panic(fmt.Sprintf("This should never happen. FakeDB missing method %s, so doesn't fit the interface", k))
		}
		delete(storeMethods, k)
		delete(fakeMethods, k)
	}

	for k := range fakeMethods {
		_, ok := extraFakeMethods[k]
		if ok {
			continue
		}
		// If you are seeing this error, you have an extra function not required
		// for the database.Store. If you still want to keep it, add it to
		// 'extraFakeMethods' to allow it.
		t.Errorf("Fake method '%s()' is excessive and not needed to fit interface, delete it", k)
	}
}

func methods(rt reflect.Type) map[string]bool {
	methods := make(map[string]bool)
	for i := 0; i < rt.NumMethod(); i++ {
		methods[rt.Method(i).Name] = true
	}
	return methods
}
