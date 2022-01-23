package databasefake

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/database"
)

// New returns an in-memory fake of the database.
func New() database.Store {
	return &fakeQuerier{
		apiKeys:             make([]database.APIKey, 0),
		organizations:       make([]database.Organization, 0),
		organizationMembers: make([]database.OrganizationMember, 0),
		users:               make([]database.User, 0),

		project:          make([]database.Project, 0),
		projectHistory:   make([]database.ProjectHistory, 0),
		projectParameter: make([]database.ProjectParameter, 0),
	}
}

// fakeQuerier replicates database functionality to enable quick testing.
type fakeQuerier struct {
	// Legacy tables
	apiKeys             []database.APIKey
	organizations       []database.Organization
	organizationMembers []database.OrganizationMember
	users               []database.User

	// New tables
	project          []database.Project
	projectHistory   []database.ProjectHistory
	projectParameter []database.ProjectParameter
}

// InTx doesn't rollback data properly for in-memory yet.
func (q *fakeQuerier) InTx(fn func(database.Store) error) error {
	return fn(q)
}

func (q *fakeQuerier) GetAPIKeyByID(_ context.Context, id string) (database.APIKey, error) {
	for _, apiKey := range q.apiKeys {
		if apiKey.ID == id {
			return apiKey, nil
		}
	}
	return database.APIKey{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByEmailOrUsername(_ context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	for _, user := range q.users {
		if user.Email == arg.Email || user.Username == arg.Username {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByID(_ context.Context, id string) (database.User, error) {
	for _, user := range q.users {
		if user.ID == id {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserCount(_ context.Context) (int64, error) {
	return int64(len(q.users)), nil
}

func (q *fakeQuerier) GetOrganizationByName(_ context.Context, name string) (database.Organization, error) {
	for _, organization := range q.organizations {
		if organization.Name == name {
			return organization, nil
		}
	}
	return database.Organization{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetOrganizationsByUserID(_ context.Context, userID string) ([]database.Organization, error) {
	organizations := make([]database.Organization, 0)
	for _, organizationMember := range q.organizationMembers {
		if organizationMember.UserID != userID {
			continue
		}
		for _, organization := range q.organizations {
			if organization.ID != organizationMember.OrganizationID {
				continue
			}
			organizations = append(organizations, organization)
		}
	}
	if len(organizations) == 0 {
		return nil, sql.ErrNoRows
	}
	return organizations, nil
}

func (q *fakeQuerier) GetProjectByOrganizationAndName(_ context.Context, arg database.GetProjectByOrganizationAndNameParams) (database.Project, error) {
	for _, project := range q.project {
		if project.OrganizationID != arg.OrganizationID {
			continue
		}
		if !strings.EqualFold(project.Name, arg.Name) {
			continue
		}
		return project, nil
	}
	return database.Project{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProjectHistoryByProjectID(_ context.Context, projectID uuid.UUID) ([]database.ProjectHistory, error) {
	history := make([]database.ProjectHistory, 0)
	for _, projectHistory := range q.projectHistory {
		if projectHistory.ProjectID.String() != projectID.String() {
			continue
		}
		history = append(history, projectHistory)
	}
	if len(history) == 0 {
		return nil, sql.ErrNoRows
	}
	return history, nil
}

func (q *fakeQuerier) GetProjectsByOrganizationIDs(_ context.Context, ids []string) ([]database.Project, error) {
	projects := make([]database.Project, 0)
	for _, project := range q.project {
		for _, id := range ids {
			if project.OrganizationID == id {
				projects = append(projects, project)
				break
			}
		}
	}
	if len(projects) == 0 {
		return nil, sql.ErrNoRows
	}
	return projects, nil
}

func (q *fakeQuerier) GetOrganizationMemberByUserID(_ context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	for _, organizationMember := range q.organizationMembers {
		if organizationMember.OrganizationID != arg.OrganizationID {
			continue
		}
		if organizationMember.UserID != arg.UserID {
			continue
		}
		return organizationMember, nil
	}
	return database.OrganizationMember{}, sql.ErrNoRows
}

func (q *fakeQuerier) InsertAPIKey(_ context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	//nolint:gosimple
	key := database.APIKey{
		ID:               arg.ID,
		HashedSecret:     arg.HashedSecret,
		UserID:           arg.UserID,
		Application:      arg.Application,
		Name:             arg.Name,
		LastUsed:         arg.LastUsed,
		ExpiresAt:        arg.ExpiresAt,
		CreatedAt:        arg.CreatedAt,
		UpdatedAt:        arg.UpdatedAt,
		LoginType:        arg.LoginType,
		OIDCAccessToken:  arg.OIDCAccessToken,
		OIDCRefreshToken: arg.OIDCRefreshToken,
		OIDCIDToken:      arg.OIDCIDToken,
		OIDCExpiry:       arg.OIDCExpiry,
		DevurlToken:      arg.DevurlToken,
	}
	q.apiKeys = append(q.apiKeys, key)
	return key, nil
}

func (q *fakeQuerier) InsertOrganization(_ context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	organization := database.Organization{
		ID:        arg.ID,
		Name:      arg.Name,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
	}
	q.organizations = append(q.organizations, organization)
	return organization, nil
}

func (q *fakeQuerier) InsertOrganizationMember(_ context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	//nolint:gosimple
	organizationMember := database.OrganizationMember{
		OrganizationID: arg.OrganizationID,
		UserID:         arg.UserID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Roles:          arg.Roles,
	}
	q.organizationMembers = append(q.organizationMembers, organizationMember)
	return organizationMember, nil
}

func (q *fakeQuerier) InsertProject(_ context.Context, arg database.InsertProjectParams) (database.Project, error) {
	project := database.Project{
		ID:             arg.ID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		OrganizationID: arg.OrganizationID,
		Name:           arg.Name,
		Provisioner:    arg.Provisioner,
	}
	q.project = append(q.project, project)
	return project, nil
}

func (q *fakeQuerier) InsertProjectHistory(_ context.Context, arg database.InsertProjectHistoryParams) (database.ProjectHistory, error) {
	//nolint:gosimple
	history := database.ProjectHistory{
		ID:            arg.ID,
		ProjectID:     arg.ProjectID,
		CreatedAt:     arg.CreatedAt,
		UpdatedAt:     arg.UpdatedAt,
		Name:          arg.Name,
		Description:   arg.Description,
		StorageMethod: arg.StorageMethod,
		StorageSource: arg.StorageSource,
		ImportJobID:   arg.ImportJobID,
	}
	q.projectHistory = append(q.projectHistory, history)
	return history, nil
}

func (q *fakeQuerier) InsertProjectParameter(_ context.Context, arg database.InsertProjectParameterParams) (database.ProjectParameter, error) {
	//nolint:gosimple
	param := database.ProjectParameter{
		ID:                       arg.ID,
		CreatedAt:                arg.CreatedAt,
		ProjectHistoryID:         arg.ProjectHistoryID,
		Name:                     arg.Name,
		Description:              arg.Description,
		DefaultSource:            arg.DefaultSource,
		AllowOverrideSource:      arg.AllowOverrideSource,
		DefaultDestination:       arg.DefaultDestination,
		AllowOverrideDestination: arg.AllowOverrideDestination,
		DefaultRefresh:           arg.DefaultRefresh,
		RedisplayValue:           arg.RedisplayValue,
		ValidationError:          arg.ValidationError,
		ValidationCondition:      arg.ValidationCondition,
		ValidationTypeSystem:     arg.ValidationTypeSystem,
		ValidationValueType:      arg.ValidationValueType,
	}
	q.projectParameter = append(q.projectParameter, param)
	return param, nil
}

func (q *fakeQuerier) InsertUser(_ context.Context, arg database.InsertUserParams) (database.User, error) {
	user := database.User{
		ID:             arg.ID,
		Email:          arg.Email,
		Name:           arg.Name,
		LoginType:      arg.LoginType,
		HashedPassword: arg.HashedPassword,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Username:       arg.Username,
	}
	q.users = append(q.users, user)
	return user, nil
}

func (q *fakeQuerier) UpdateAPIKeyByID(_ context.Context, arg database.UpdateAPIKeyByIDParams) error {
	for index, apiKey := range q.apiKeys {
		if apiKey.ID != arg.ID {
			continue
		}
		apiKey.LastUsed = arg.LastUsed
		apiKey.ExpiresAt = arg.ExpiresAt
		apiKey.OIDCAccessToken = arg.OIDCAccessToken
		apiKey.OIDCRefreshToken = arg.OIDCRefreshToken
		apiKey.OIDCExpiry = arg.OIDCExpiry
		q.apiKeys[index] = apiKey
		return nil
	}
	return sql.ErrNoRows
}
