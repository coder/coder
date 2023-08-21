// Package db2sdk provides common conversion routines from database types to codersdk types
package db2sdk

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/parameter"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func WorkspaceBuildParameters(params []database.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	out := make([]codersdk.WorkspaceBuildParameter, len(params))
	for i, p := range params {
		out[i] = WorkspaceBuildParameter(p)
	}
	return out
}

func WorkspaceBuildParameter(p database.WorkspaceBuildParameter) codersdk.WorkspaceBuildParameter {
	return codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: p.Value,
	}
}

func TemplateVersionParameter(param database.TemplateVersionParameter) (codersdk.TemplateVersionParameter, error) {
	options, err := templateVersionParameterOptions(param.Options)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}

	descriptionPlaintext, err := parameter.Plaintext(param.Description)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}

	var validationMin *int32
	if param.ValidationMin.Valid {
		validationMin = &param.ValidationMin.Int32
	}

	var validationMax *int32
	if param.ValidationMax.Valid {
		validationMax = &param.ValidationMax.Int32
	}

	return codersdk.TemplateVersionParameter{
		Name:                 param.Name,
		DisplayName:          param.DisplayName,
		Description:          param.Description,
		DescriptionPlaintext: descriptionPlaintext,
		Type:                 param.Type,
		Mutable:              param.Mutable,
		DefaultValue:         param.DefaultValue,
		Icon:                 param.Icon,
		Options:              options,
		ValidationRegex:      param.ValidationRegex,
		ValidationMin:        validationMin,
		ValidationMax:        validationMax,
		ValidationError:      param.ValidationError,
		ValidationMonotonic:  codersdk.ValidationMonotonicOrder(param.ValidationMonotonic),
		Required:             param.Required,
		Ephemeral:            param.Ephemeral,
	}, nil
}

func ProvisionerJobStatus(provisionerJob database.ProvisionerJob) codersdk.ProvisionerJobStatus {
	// The case where jobs are hung is handled by the unhang package. We can't
	// just return Failed here when it's hung because that doesn't reflect in
	// the database.
	switch {
	case provisionerJob.CanceledAt.Valid:
		if !provisionerJob.CompletedAt.Valid {
			return codersdk.ProvisionerJobCanceling
		}
		if provisionerJob.Error.String == "" {
			return codersdk.ProvisionerJobCanceled
		}
		return codersdk.ProvisionerJobFailed
	case !provisionerJob.StartedAt.Valid:
		return codersdk.ProvisionerJobPending
	case provisionerJob.CompletedAt.Valid:
		if provisionerJob.Error.String == "" {
			return codersdk.ProvisionerJobSucceeded
		}
		return codersdk.ProvisionerJobFailed
	default:
		return codersdk.ProvisionerJobRunning
	}
}

func User(user database.User, organizationIDs []uuid.UUID) codersdk.User {
	convertedUser := codersdk.User{
		ID:              user.ID,
		Email:           user.Email,
		CreatedAt:       user.CreatedAt,
		LastSeenAt:      user.LastSeenAt,
		Username:        user.Username,
		Status:          codersdk.UserStatus(user.Status),
		OrganizationIDs: organizationIDs,
		Roles:           make([]codersdk.Role, 0, len(user.RBACRoles)),
		AvatarURL:       user.AvatarURL.String,
		LoginType:       codersdk.LoginType(user.LoginType),
	}

	for _, roleName := range user.RBACRoles {
		rbacRole, _ := rbac.RoleByName(roleName)
		convertedUser.Roles = append(convertedUser.Roles, Role(rbacRole))
	}

	return convertedUser
}

func Role(role rbac.Role) codersdk.Role {
	return codersdk.Role{
		DisplayName: role.DisplayName,
		Name:        role.Name,
	}
}

func TemplateInsightsParameters(parameterRows []database.GetTemplateParameterInsightsRow) ([]codersdk.TemplateParameterUsage, error) {
	parametersByNum := make(map[int64]*codersdk.TemplateParameterUsage)
	for _, param := range parameterRows {
		if _, ok := parametersByNum[param.Num]; !ok {
			var opts []codersdk.TemplateVersionParameterOption
			err := json.Unmarshal(param.Options, &opts)
			if err != nil {
				return nil, err
			}

			plaintextDescription, err := parameter.Plaintext(param.Description)
			if err != nil {
				return nil, err
			}

			parametersByNum[param.Num] = &codersdk.TemplateParameterUsage{
				TemplateIDs: param.TemplateIDs,
				Name:        param.Name,
				Type:        param.Type,
				DisplayName: param.DisplayName,
				Description: plaintextDescription,
				Options:     opts,
			}
		}
		parametersByNum[param.Num].Values = append(parametersByNum[param.Num].Values, codersdk.TemplateParameterValue{
			Value: param.Value,
			Count: param.Count,
		})
	}
	parametersUsage := []codersdk.TemplateParameterUsage{}
	for _, param := range parametersByNum {
		parametersUsage = append(parametersUsage, *param)
	}

	sort.Slice(parametersUsage, func(i, j int) bool {
		return parametersUsage[i].Name < parametersUsage[j].Name
	})
	return parametersUsage, nil
}

func templateVersionParameterOptions(rawOptions json.RawMessage) ([]codersdk.TemplateVersionParameterOption, error) {
	var protoOptions []*proto.RichParameterOption
	err := json.Unmarshal(rawOptions, &protoOptions)
	if err != nil {
		return nil, err
	}
	var options []codersdk.TemplateVersionParameterOption
	for _, option := range protoOptions {
		options = append(options, codersdk.TemplateVersionParameterOption{
			Name:        option.Name,
			Description: option.Description,
			Value:       option.Value,
			Icon:        option.Icon,
		})
	}
	return options, nil
}
