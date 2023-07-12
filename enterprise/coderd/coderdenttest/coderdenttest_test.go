package coderdenttest_test

import (
	"testing"

	"github.com/coder/coder/enterprise/coderd/coderdenttest"
)

func TestNew(t *testing.T) {
	t.Parallel()
	_, _ = coderdenttest.New(t, nil)
}
