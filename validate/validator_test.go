package validate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/longid"
)

func TestValidateLongID(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		type myStruct struct {
			ID string `validate:"longid"`
		}
		v := &myStruct{
			ID: longid.New().String(),
		}
		err := Validator().Struct(v)
		require.NoError(t, err, "validate")
	})

	t.Run("Invalid", func(t *testing.T) {
		type myStruct struct {
			ID string `validate:"longid"`
		}
		v := &myStruct{
			ID: longid.New().String() + "hello",
		}
		err := Validator().Struct(v)
		require.Error(t, err, "unexpectedly validated")
	})

	t.Run("WrongType", func(t *testing.T) {
		type myStruct struct {
			ID int `validate:"longid"`
		}
		v := &myStruct{
			ID: 123,
		}
		err := Validator().Struct(v)
		require.Error(t, err, "unexpectedly validated")
	})
}
