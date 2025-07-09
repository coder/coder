package coderdtest

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type DynamicParameterTemplateParams struct {
	MainTF         string
	Plan           json.RawMessage
	ModulesArchive []byte

	// Uses a zip archive instead of a tar
	Zip bool

	// StaticParams is used if the provisioner daemon version does not support dynamic parameters.
	StaticParams []*proto.RichParameter

	// TemplateID is used to update an existing template instead of creating a new one.
	TemplateID uuid.UUID

	Version func(request *codersdk.CreateTemplateVersionRequest)
}

func DynamicParameterTemplate(t *testing.T, client *codersdk.Client, org uuid.UUID, args DynamicParameterTemplateParams) (codersdk.Template, codersdk.TemplateVersion) {
	t.Helper()

	files := echo.WithExtraFiles(map[string][]byte{
		"main.tf": []byte(args.MainTF),
	})
	files.ProvisionPlan = []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Plan:        args.Plan,
				ModuleFiles: args.ModulesArchive,
				Parameters:  args.StaticParams,
			},
		},
	}}

	mime := codersdk.ContentTypeTar
	if args.Zip {
		mime = codersdk.ContentTypeZip
	}
	version := CreateTemplateVersionMimeType(t, client, mime, org, files, func(request *codersdk.CreateTemplateVersionRequest) {
		if args.TemplateID != uuid.Nil {
			request.TemplateID = args.TemplateID
		}
		if args.Version != nil {
			args.Version(request)
		}
	})
	AwaitTemplateVersionJobCompleted(t, client, version.ID)

	var tpl codersdk.Template
	var err error

	if args.TemplateID == uuid.Nil {
		tpl = CreateTemplate(t, client, org, version.ID, func(request *codersdk.CreateTemplateRequest) {
			request.UseClassicParameterFlow = ptr.Ref(false)
		})
	} else {
		tpl, err = client.UpdateTemplateMeta(t.Context(), args.TemplateID, codersdk.UpdateTemplateMeta{
			UseClassicParameterFlow: ptr.Ref(false),
		})
		require.NoError(t, err)
	}

	err = client.UpdateActiveTemplateVersion(t.Context(), tpl.ID, codersdk.UpdateActiveTemplateVersion{
		ID: version.ID,
	})
	require.NoError(t, err)
	require.Equal(t, tpl.UseClassicParameterFlow, false, "template should use dynamic parameters")

	return tpl, version
}

type ParameterAsserter struct {
	Name   string
	Params []codersdk.PreviewParameter
	t      *testing.T
}

func AssertParameter(t *testing.T, name string, params []codersdk.PreviewParameter) *ParameterAsserter {
	return &ParameterAsserter{
		Name:   name,
		Params: params,
		t:      t,
	}
}

func (a *ParameterAsserter) find(name string) *codersdk.PreviewParameter {
	a.t.Helper()
	for _, p := range a.Params {
		if p.Name == name {
			return &p
		}
	}

	assert.Fail(a.t, "parameter not found", "expected parameter %q to exist", a.Name)
	return nil
}

func (a *ParameterAsserter) NotExists() *ParameterAsserter {
	a.t.Helper()

	names := slice.Convert(a.Params, func(p codersdk.PreviewParameter) string {
		return p.Name
	})

	assert.NotContains(a.t, names, a.Name)
	return a
}

func (a *ParameterAsserter) Exists() *ParameterAsserter {
	a.t.Helper()

	names := slice.Convert(a.Params, func(p codersdk.PreviewParameter) string {
		return p.Name
	})

	assert.Contains(a.t, names, a.Name)
	return a
}

func (a *ParameterAsserter) Value(expected string) *ParameterAsserter {
	a.t.Helper()

	p := a.find(a.Name)
	if p == nil {
		return a
	}

	assert.Equal(a.t, expected, p.Value.Value)
	return a
}

func (a *ParameterAsserter) Options(expected ...string) *ParameterAsserter {
	a.t.Helper()

	p := a.find(a.Name)
	if p == nil {
		return a
	}

	optValues := slice.Convert(p.Options, func(p codersdk.PreviewParameterOption) string {
		return p.Value.Value
	})
	assert.ElementsMatch(a.t, expected, optValues, "parameter %q options", a.Name)
	return a
}
