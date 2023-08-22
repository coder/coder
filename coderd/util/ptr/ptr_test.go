package ptr_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/util/ptr"
)

func Test_Ref_Deref(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		val := "test"
		p := ptr.Ref(val)
		assert.Equal(t, &val, p)
	})

	t.Run("Bool", func(t *testing.T) {
		t.Parallel()
		val := true
		p := ptr.Ref(val)
		assert.Equal(t, &val, p)
	})

	t.Run("Int64", func(t *testing.T) {
		t.Parallel()
		val := int64(42)
		p := ptr.Ref(val)
		assert.Equal(t, &val, p)
	})

	t.Run("Float64", func(t *testing.T) {
		t.Parallel()
		val := float64(3.14159)
		p := ptr.Ref(val)
		assert.Equal(t, &val, p)
	})
}

func Test_NilOrEmpty(t *testing.T) {
	t.Parallel()
	nilString := (*string)(nil)
	emptyString := ""
	nonEmptyString := "hi"

	assert.True(t, ptr.NilOrEmpty(nilString))
	assert.True(t, ptr.NilOrEmpty(&emptyString))
	assert.False(t, ptr.NilOrEmpty(&nonEmptyString))
}

func Test_NilToEmpty(t *testing.T) {
	t.Parallel()

	assert.False(t, ptr.NilToEmpty((*bool)(nil)))
	assert.Empty(t, ptr.NilToEmpty((*int64)(nil)))
	assert.Empty(t, ptr.NilToEmpty((*string)(nil)))
	assert.Equal(t, true, ptr.NilToEmpty(ptr.Ref(true)))
}

func Test_NilToDefault(t *testing.T) {
	t.Parallel()

	assert.True(t, ptr.NilToDefault(ptr.Ref(true), false))
	assert.True(t, ptr.NilToDefault((*bool)(nil), true))

	assert.Equal(t, int64(4), ptr.NilToDefault(ptr.Ref[int64](4), 5))
	assert.Equal(t, int64(5), ptr.NilToDefault((*int64)(nil), 5))

	assert.Equal(t, "hi", ptr.NilToDefault((*string)(nil), "hi"))
	assert.Equal(t, "hello", ptr.NilToDefault(ptr.Ref("hello"), "hi"))
}

func Test_NilOrZero(t *testing.T) {
	t.Parallel()

	nilInt64 := (*int64)(nil)
	nilFloat64 := (*float64)(nil)
	nilDuration := (*time.Duration)(nil)

	zeroInt64 := int64(0)
	zeroFloat64 := float64(0.0)
	zeroDuration := time.Duration(0)

	nonZeroInt64 := int64(1)
	nonZeroFloat64 := float64(3.14159)
	nonZeroDuration := time.Hour

	assert.True(t, ptr.NilOrZero(nilInt64))
	assert.True(t, ptr.NilOrZero(nilFloat64))
	assert.True(t, ptr.NilOrZero(nilDuration))

	assert.True(t, ptr.NilOrZero(&zeroInt64))
	assert.True(t, ptr.NilOrZero(&zeroFloat64))
	assert.True(t, ptr.NilOrZero(&zeroDuration))

	assert.False(t, ptr.NilOrZero(&nonZeroInt64))
	assert.False(t, ptr.NilOrZero(&nonZeroFloat64))
	assert.False(t, ptr.NilOrZero(&nonZeroDuration))
}
