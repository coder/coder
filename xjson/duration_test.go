package xjson

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuration(t *testing.T) {
	t.Run("MarshalUnmarshalJSON", func(t *testing.T) {
		var dur = Duration(time.Hour)
		b, err := json.Marshal(dur)
		require.NoError(t, err, "marshal duration")

		var unmarshalDur Duration
		err = json.Unmarshal(b, &unmarshalDur)
		require.NoError(t, err, "unmarshal duration")
		require.Equal(t, dur, unmarshalDur, "Did not parse to milliseconds")
	})
}
