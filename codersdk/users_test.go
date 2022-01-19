package codersdk_test

import (
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestUsers(t *testing.T) {
	t.Run("Personal", func(t *testing.T) {
		_ = coderdtest.New(t)

	})
}
