package testutil

import (
	"reflect"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
)

// Fake will populate any zero fields in the provided struct with fake data.
// Non-zero fields will remain unchanged.
// Usage:
//
//	key := Fake(t, faker, database.APIKey{
//	  TokenName: "keep-my-name",
//	})
func Fake[T any](t *testing.T, faker *gofakeit.Faker, seed T) T {
	t.Helper()

	var tmp T
	err := faker.Struct(&tmp)
	require.NoError(t, err, "failed to generate fake data for type %T", tmp)

	mergeZero(&seed, tmp)
	return seed
}

// mergeZero merges the fields of src into dst, but only if the field in dst is
// currently the zero value.
// Make sure `dst` is a pointer to a struct, otherwise the fields are not assignable.
func mergeZero(dst any, src any) {
	srcv := reflect.ValueOf(src)
	if srcv.Kind() == reflect.Ptr {
		srcv = srcv.Elem()
	}
	remain := [][2]reflect.Value{
		{reflect.ValueOf(dst).Elem(), srcv},
	}

	// Traverse the struct fields and set them only if they are currently zero.
	// This is a breadth-first traversal of the struct fields. Struct definitions
	// Should not be that deep, so we should not hit any stack overflow issues.
	for {
		if len(remain) == 0 {
			return
		}
		dv, sv := remain[0][0], remain[0][1]
		remain = remain[1:] //
		for i := 0; i < dv.NumField(); i++ {
			df := dv.Field(i)
			sf := sv.Field(i)
			if !df.CanSet() {
				continue
			}
			if df.IsZero() { // only write if currently zero
				df.Set(sf)
				continue
			}

			if dv.Field(i).Kind() == reflect.Struct {
				// If the field is a struct, we need to traverse it as well.
				remain = append(remain, [2]reflect.Value{df, sf})
			}
		}
	}
}
