package testutil_test

import (
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
)

type simpleStruct struct {
	ID          uuid.UUID
	Name        string
	Description string
	Age         int `fake:"{number:18,60}"`
}

type nestedStruct struct {
	Person  simpleStruct
	Address string
}

func TestFake(t *testing.T) {
	t.Parallel()

	t.Run("Simple", func(t *testing.T) {
		t.Parallel()

		faker := gofakeit.New(0)
		person := testutil.Fake(t, faker, simpleStruct{
			Name: "alice",
		})
		require.Equal(t, "alice", person.Name)
		require.NotEqual(t, uuid.Nil, person.ID)
		require.NotEmpty(t, person.Description)
		require.Greater(t, person.Age, 17, "Age should be greater than 17")
		require.Less(t, person.Age, 61, "Age should be less than 61")
	})

	t.Run("Nested", func(t *testing.T) {
		t.Parallel()

		faker := gofakeit.New(0)
		person := testutil.Fake(t, faker, nestedStruct{
			Person: simpleStruct{
				Name: "alice",
			},
		})
		require.Equal(t, "alice", person.Person.Name)
		require.NotEqual(t, uuid.Nil, person.Person.ID)
		require.NotEmpty(t, person.Person.Description)
		require.Greater(t, person.Person.Age, 17, "Age should be greater than 17")
		require.NotEmpty(t, person.Address)
	})

	t.Run("DatabaseType", func(t *testing.T) {
		t.Parallel()

		faker := gofakeit.New(0)
		id := uuid.New()
		key := testutil.Fake(t, faker, database.APIKey{
			UserID:    id,
			TokenName: "keep-my-name",
		})
		require.Equal(t, id, key.UserID)
		require.NotEmpty(t, key.TokenName)
	})
}
