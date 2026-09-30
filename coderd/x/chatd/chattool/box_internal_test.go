package chattool

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONStringCost(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"",
		"plain ascii",
		"line\nbreaks\r\tand \"quotes\" \\",
		"<html>&amp;</html>",
		"\x00\x01\x1f\b\f",
		"\u2028\u2029 é 日本 🎉",
		"invalid \xff\xfe utf8",
	} {
		encoded, err := json.Marshal(s)
		require.NoError(t, err)
		cost := jsonStringCost(s)
		assert.GreaterOrEqual(t, cost, len(encoded)-2, "%q", s)

		for budget := 0; budget <= cost; budget++ {
			prefix := jsonStringPrefix(s, budget)
			assert.LessOrEqual(t, jsonStringCost(prefix), budget, "%q at %d", s, budget)
		}
		assert.Equal(t, s, jsonStringPrefix(s, cost))
	}
}
