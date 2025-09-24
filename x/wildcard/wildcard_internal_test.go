package wildcard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAnyZeroIsWildcard(t *testing.T) {
	t.Parallel()
	var a Value[string]
	require.True(t, a.IsAny())
	v, ok := a.Value()
	require.False(t, ok)
	require.Equal(t, "*", a.String())
	require.Equal(t, "", v)
}

func TestAnyOfString(t *testing.T) {
	t.Parallel()
	a := Of("workspace")
	require.False(t, a.IsAny())
	v, ok := a.Value()
	require.True(t, ok)
	require.Equal(t, "workspace", v)
	// Text marshal
	b, err := a.MarshalText()
	require.NoError(t, err)
	require.Equal(t, []byte("workspace"), b)
	// Text unmarshal to wildcard
	var w Value[string]
	require.NoError(t, w.UnmarshalText([]byte("*")))
	require.True(t, w.IsAny())
}

func TestAnyUUID(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	a := Of(id)
	require.False(t, a.IsAny())
	v, ok := a.Value()
	require.True(t, ok)
	require.Equal(t, id, v)

	// String and text roundtrip
	b, err := a.MarshalText()
	require.NoError(t, err)
	require.Equal(t, id.String(), string(b))

	var w Value[uuid.UUID]
	require.NoError(t, w.UnmarshalText([]byte(id.String())))
	v2, ok2 := w.Value()
	require.True(t, ok2)
	require.Equal(t, id, v2)
}

func TestAnyScanString(t *testing.T) {
	t.Parallel()
	var a Value[string]
	// nil → wildcard
	require.NoError(t, a.Scan(nil))
	require.True(t, a.IsAny())
	// []byte
	require.NoError(t, a.Scan([]byte("template")))
	v, ok := a.Value()
	require.True(t, ok)
	require.Equal(t, "template", v)
	// string
	require.NoError(t, a.Scan("file"))
	v, ok = a.Value()
	require.True(t, ok)
	require.Equal(t, "file", v)
}

func TestAnyScanUUID(t *testing.T) {
	t.Parallel()
	var a Value[uuid.UUID]
	// wildcard via nil
	require.NoError(t, a.Scan(nil))
	require.True(t, a.IsAny())
	// valid uuid via []byte
	id := uuid.New()
	require.NoError(t, a.Scan([]byte(id.String())))
	v, ok := a.Value()
	require.True(t, ok)
	require.Equal(t, id, v)
}
