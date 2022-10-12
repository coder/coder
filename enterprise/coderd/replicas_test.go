package coderd_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
)

func TestReplicas(t *testing.T) {
	t.Parallel()
	db, pubsub := dbtestutil.NewDB(t)
	firstClient := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		},
	})
	_ = coderdtest.CreateFirstUser(t, firstClient)

	secondClient := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		},
	})
	secondClient.SessionToken = firstClient.SessionToken

	user, err := secondClient.User(context.Background(), codersdk.Me)
	require.NoError(t, err)
	fmt.Printf("%+v\n", user)
}
