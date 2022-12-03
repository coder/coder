package coderd_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

func TestServiceBanners(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	client := coderdenttest.New(t, &coderdenttest.Options{})

	user := coderdtest.CreateFirstUser(t, client)
	coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
		ServiceBanners: true,
	})
}
