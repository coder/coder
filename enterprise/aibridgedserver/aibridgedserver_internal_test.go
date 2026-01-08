package aibridgedserver

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
)

func TestMarshalMetadata(t *testing.T) {
	t.Parallel()

	t.Run("NilData", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		out := marshalMetadata(context.Background(), logger, nil)
		require.JSONEq(t, "{}", string(out))
	})

	t.Run("WithData", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		list := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("a"),
			structpb.NewNumberValue(1),
			structpb.NewBoolValue(false),
		}})
		obj := structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
			"a": structpb.NewStringValue("b"),
			"n": structpb.NewNumberValue(3),
		}})

		nonValue := mustMarshalAny(t, &structpb.Struct{Fields: map[string]*structpb.Value{
			"ignored": structpb.NewStringValue("yes"),
		}})
		invalid := &anypb.Any{TypeUrl: "type.googleapis.com/google.protobuf.Value", Value: []byte{0xff, 0x00}}

		in := map[string]*anypb.Any{
			"null": mustMarshalAny(t, structpb.NewNullValue()),
			// Scalars
			"string": mustMarshalAny(t, structpb.NewStringValue("hello")),
			"bool":   mustMarshalAny(t, structpb.NewBoolValue(true)),
			"number": mustMarshalAny(t, structpb.NewNumberValue(42)),
			// Complex types
			"list":   mustMarshalAny(t, list),
			"object": mustMarshalAny(t, obj),
			// Extra valid entries
			"ok":  mustMarshalAny(t, structpb.NewStringValue("present")),
			"nan": mustMarshalAny(t, structpb.NewNumberValue(math.NaN())),
			// Entries that should be ignored
			"invalid":   invalid,
			"non_value": nonValue,
		}

		out := marshalMetadata(context.Background(), logger, in)
		require.NotNil(t, out)
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got))

		expected := map[string]any{
			"string": "hello",
			"bool":   true,
			"number": float64(42),
			"null":   nil,
			"list":   []any{"a", float64(1), false},
			"object": map[string]any{"a": "b", "n": float64(3)},
			"ok":     "present",
			"nan":    "NaN",
		}
		require.Equal(t, expected, got)
	})
}

func mustMarshalAny(t testing.TB, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	require.NoError(t, err)
	return a
}
