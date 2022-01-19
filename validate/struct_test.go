package validate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFieldsValidation(t *testing.T) {
	t.Parallel()

	t.Run("AllFieldsValidated", func(t *testing.T) {
		type s struct {
			Field string `validate:"min=3"`
		}
		var v s
		fs, err := FieldsMissingValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
		fs, err = FieldsWithValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 1, len(fs), "num fields")
	})

	t.Run("Pointer", func(t *testing.T) {
		type s struct {
			Field string `validate:"min=3"`
		}
		var v s
		fs, err := FieldsMissingValidation(&v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
		fs, err = FieldsWithValidation(&v)
		require.NoError(t, err, "err")
		require.Equal(t, 1, len(fs), "num fields")
	})

	t.Run("MissingValidations", func(t *testing.T) {
		type s struct {
			Field1 string
			Field2 string
		}
		var v s
		fs, err := FieldsMissingValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 2, len(fs), "num fields")
		fs, err = FieldsWithValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
	})

	t.Run("UnexportedFields", func(t *testing.T) {
		type s struct {
			field string
		}
		var v = s{field: "string"}
		fs, err := FieldsMissingValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
		fs, err = FieldsWithValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
	})

	t.Run("Bools", func(t *testing.T) {
		type s struct {
			Field1 *bool
			Field2 bool
		}
		var v s
		fs, err := FieldsMissingValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
		fs, err = FieldsWithValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
	})

	t.Run("Nested", func(t *testing.T) {
		type nested struct {
			Field string `validate:"min=3"`
		}
		type s struct {
			Nested *nested `validate:"required"`
		}
		var v s
		fs, err := FieldsMissingValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
		fs, err = FieldsWithValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 2, len(fs), "num fields")
	})

	t.Run("NestedUnexported", func(t *testing.T) {
		// Specifically using time since it has known unexported fields, and is
		// from a different package.
		type s struct {
			Time time.Time `validate:"required"`
		}
		var v s
		fs, err := FieldsMissingValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 0, len(fs), "num fields")
		fs, err = FieldsWithValidation(v)
		require.NoError(t, err, "err")
		require.Equal(t, 1, len(fs), "num fields")
	})
}

func TestValidateField(t *testing.T) {
	t.Parallel()

	t.Run("NoMatch", func(t *testing.T) {
		type s struct {
			Field string `validate:"min=3"`
		}
		err := Field(s{}, JSONTagValueFieldSelector("hello"))
		require.NoError(t, err, "validate")
	})

	t.Run("MatchValid", func(t *testing.T) {
		type s struct {
			Field string `json:"hello" validate:"min=3"`
		}
		err := Field(s{Field: "world"}, JSONTagValueFieldSelector("hello"))
		require.NoError(t, err, "validate")
	})

	t.Run("MatchInvalid", func(t *testing.T) {
		type s struct {
			Field string `json:"hello" validate:"min=3"`
		}
		err := Field(s{Field: "hi"}, JSONTagValueFieldSelector("hello"))
		require.Error(t, err, "validate")
	})
}

func TestSelectFields(t *testing.T) {
	t.Parallel()

	t.Run("Some", func(t *testing.T) {
		type s struct {
			Field1 string `bogus:"bogus"`
			Field2 string
		}
		fs := TagKeyFieldSelector("bogus")
		fields, err := SelectFields(s{}, fs, nil)
		require.NoError(t, err, "select")
		require.Equal(t, 1, len(fields), "num fields")
	})

	t.Run("Nested", func(t *testing.T) {
		type nested struct {
			Field1 string `bogus:"bogus"`
		}
		type s struct {
			Field1 string `bogus:"bogus"`
			Nested *nested
		}
		fs := TagKeyFieldSelector("bogus")
		fields, err := SelectFields(s{}, fs, nil)
		require.NoError(t, err, "select")
		require.Equal(t, 2, len(fields), "num fields")
	})

	t.Run("Embedded", func(t *testing.T) {
		type embedded struct {
			Field1 string `bogus:"bogus"`
		}
		type s struct {
			embedded
		}
		fs := TagKeyFieldSelector("bogus")
		fields, err := SelectFields(s{}, fs, nil)
		require.NoError(t, err, "select")
		require.Equal(t, 1, len(fields), "num fields")
	})

	t.Run("InfiniteRecursion", func(t *testing.T) {
		type s struct {
			Field *s `bogus:"bogus"`
		}
		fs := TagKeyFieldSelector("bogus")
		fields, err := SelectFields(s{}, fs, nil)
		require.NoError(t, err, "select")
		require.Equal(t, 1, len(fields), "num fields")
	})
}

func TestValidateStruct(t *testing.T) {
	t.Run("Anonymous", func(t *testing.T) {
		// Test validate on anonymous fields
		type s struct {
			json.RawMessage `validate:"min=4"`
		}

		// Not enough bytes
		v := s{RawMessage: []byte("{}}")}

		err := Validator().Struct(v)
		require.Error(t, err, "validate")
	})
}
