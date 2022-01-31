package coderd_test

import (
	"testing"
	"time"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisionerd/provisionerdtest"
)

func TestProvisionerd(t *testing.T) {
	t.Parallel()
	t.Run("Listen", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		_ = provisionerdtest.New(t, server.Client)

		time.Sleep(time.Second)
	})
}
