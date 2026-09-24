// Package experimentstest provides test doubles for the experiments
// package.
package experimentstest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/codersdk"
)

// Store is an in-memory experiments.Store. The zero value is a store with
// no rules and no users.
type Store struct {
	// StoredRules is returned by Rules.
	StoredRules map[codersdk.Experiment]experiments.StoredRule
	// RulesErr, when set, is returned by Rules.
	RulesErr error
	// Users is read by UserAttributes. Missing users return an error.
	Users map[uuid.UUID]experiments.User
	// UserErr, when set, is returned by UserAttributes.
	UserErr error
}

var _ experiments.Store = Store{}

// Rules implements experiments.Store.
func (s Store) Rules(context.Context) (map[codersdk.Experiment]experiments.StoredRule, error) {
	if s.RulesErr != nil {
		return nil, s.RulesErr
	}
	return s.StoredRules, nil
}

// UserAttributes implements experiments.Store.
func (s Store) UserAttributes(_ context.Context, userID uuid.UUID) (experiments.User, error) {
	if s.UserErr != nil {
		return experiments.User{}, s.UserErr
	}
	user, ok := s.Users[userID]
	if !ok {
		return experiments.User{}, xerrors.Errorf("user %s not found", userID)
	}
	return user, nil
}

// StoredRule encodes rule as it would be stored.
func StoredRule(t testing.TB, rule experiments.Rule) experiments.StoredRule {
	t.Helper()
	value, err := json.Marshal(rule)
	require.NoError(t, err)
	return experiments.StoredRule{Value: value}
}
