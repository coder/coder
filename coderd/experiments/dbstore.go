package experiments

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// NewDBStore returns a Store that reads rules and subject attributes from
// db. Reads run as the system: the Evaluator decides for any subject, not
// for the caller.
func NewDBStore(db database.Store) Store {
	return dbStore{db: db}
}

type dbStore struct {
	db database.Store
}

// Rules implements Store. Rows for unknown experiments are returned keyed
// by their name, and values are returned undecoded; the Evaluator ignores
// the former and decides off for undecodable values.
func (s dbStore) Rules(ctx context.Context) (map[codersdk.Experiment]StoredRule, error) {
	//nolint:gocritic // The Evaluator reads rules for every subject.
	rows, err := s.db.GetExperimentRules(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return nil, xerrors.Errorf("get experiment rules: %w", err)
	}
	rules := make(map[codersdk.Experiment]StoredRule, len(rows))
	for _, row := range rows {
		rules[codersdk.Experiment(row.Experiment)] = StoredRule{Value: []byte(row.Value)}
	}
	return rules, nil
}

// UserAttributes implements Store.
func (s dbStore) UserAttributes(ctx context.Context, userID uuid.UUID) (User, error) {
	//nolint:gocritic // Conditions read the subject's attributes, not the caller's.
	ctx = dbauthz.AsSystemRestricted(ctx)
	user, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, xerrors.Errorf("get user: %w", err)
	}
	orgs, err := s.db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  userID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return User{}, xerrors.Errorf("get user organizations: %w", err)
	}
	groups, err := s.db.GetGroups(ctx, database.GetGroupsParams{HasMemberID: userID})
	if err != nil {
		return User{}, xerrors.Errorf("get user groups: %w", err)
	}

	attrs := User{
		ID:            user.ID.String(),
		Username:      user.Username,
		Email:         user.Email,
		Roles:         append([]string{}, user.RBACRoles...),
		Organizations: make([]string, 0, len(orgs)),
		Groups:        make([]string, 0, len(groups)),
	}
	for _, org := range orgs {
		attrs.Organizations = append(attrs.Organizations, org.Name)
	}
	for _, group := range groups {
		attrs.Groups = append(attrs.Groups, group.OrganizationName+"/"+group.Group.Name)
	}
	return attrs, nil
}
