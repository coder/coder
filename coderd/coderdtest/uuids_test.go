package coderdtest_test

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/coderdtest"
)

func ExampleNewDeterministicUUIDGenerator() {
	det := coderdtest.NewDeterministicUUIDGenerator()
	testCases := []struct {
		CreateUsers []uuid.UUID
		ExpectedIDs []uuid.UUID
	}{
		{
			CreateUsers: []uuid.UUID{
				det.ID("player1"),
				det.ID("player2"),
			},
			ExpectedIDs: []uuid.UUID{
				det.ID("player1"),
				det.ID("player2"),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		_ = tc
		// Do the test with CreateUsers as the setup, and the expected IDs
		// will match.
	}
}
