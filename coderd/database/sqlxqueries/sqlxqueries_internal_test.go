package sqlxqueries

import "testing"

func Test_loadQueries(t *testing.T) {
	t.Parallel()
	// If this panics, the test will fail.
	loadQueries()
}
