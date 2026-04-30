package dynamicparameters

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

type parameterValueSource int

const (
	sourceDefault parameterValueSource = iota
	sourcePrevious
	sourceBuild
	sourcePreset
)

const (
	secretRequirementKindEnv  = "env"
	secretRequirementKindFile = "file"
)

type parameterValue struct {
	Value  string
	Source parameterValueSource
}

// ResolveOption configures optional behavior for ResolveParameters.
type ResolveOption func(*resolveOptions)

type resolveOptions struct {
	skipSecretRequirements bool
}

// SkipSecretRequirements skips structured secret-requirement validation and
// enforcement. Callers must pass this for non-start transitions so an
// unsatisfied coder_secret, or an admin who can't read the owner's secrets,
// doesn't block stop or delete.
func SkipSecretRequirements() ResolveOption {
	return func(o *resolveOptions) {
		o.skipSecretRequirements = true
	}
}

//nolint:revive // firstbuild is a control flag to turn on immutable validation
func ResolveParameters(
	ctx context.Context,
	ownerID uuid.UUID,
	renderer Renderer,
	firstBuild bool,
	previousValues []database.WorkspaceBuildParameter,
	buildValues []codersdk.WorkspaceBuildParameter,
	presetValues []database.TemplateVersionPresetParameter,
	opts ...ResolveOption,
) (map[string]string, error) {
	o := resolveOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	previousValuesMap := slice.ToMapFunc(previousValues, func(p database.WorkspaceBuildParameter) (string, string) {
		return p.Name, p.Value
	})

	// Start with previous
	values := parameterValueMap(slice.ToMapFunc(previousValues, func(p database.WorkspaceBuildParameter) (string, parameterValue) {
		return p.Name, parameterValue{Source: sourcePrevious, Value: p.Value}
	}))

	// Add build values (overwrite previous values if they exist)
	for _, buildValue := range buildValues {
		values[buildValue.Name] = parameterValue{Source: sourceBuild, Value: buildValue.Value}
	}

	// Add preset values (overwrite previous and build values if they exist)
	for _, preset := range presetValues {
		values[preset.Name] = parameterValue{Source: sourcePreset, Value: preset.Value}
	}

	// originalInputValues is going to be used to detect if a user tried to change
	// an immutable parameter after the first build.
	// The actual input values are mutated based on attributes like mutability
	// and ephemerality.
	originalInputValues := make(map[string]parameterValue, len(values))
	for name, value := range values {
		// Store the original values for later use.
		originalInputValues[name] = value
	}

	// Render the parameters using the values that were supplied to the previous build.
	//
	// This is how the form should look to the user on their workspace settings page.
	// This is the original form truth that our validations should initially be based on.
	result, diags := renderer.Render(ctx, ownerID, previousValuesMap)
	if diags.HasErrors() {
		// Top level diagnostics should break the build. Previous values (and new) should
		// always be valid. If there is a case where this is not true, then this has to
		// be changed to allow the build to continue with a different set of values.

		return nil, parameterValidationError(diags)
	}
	output := result.Output

	// The user's input now needs to be validated against the parameters.
	// Mutability & Ephemeral parameters depend on sequential workspace builds.
	//
	// To enforce these, the user's input values are trimmed based on the
	// mutability and ephemeral parameters defined in the template version.
	for _, parameter := range output.Parameters {
		// Ephemeral parameters should not be taken from the previous build.
		// They must always be explicitly set in every build.
		// So remove their values if they are sourced from the previous build.
		if parameter.Ephemeral {
			v := values[parameter.Name]
			if v.Source == sourcePrevious {
				delete(values, parameter.Name)
			}
		}
	}

	// This is the final set of values that will be used. Any errors at this stage
	// are fatal. Additional validation for immutability has to be done manually.
	var renderOpts []RenderOption
	if !o.skipSecretRequirements {
		renderOpts = append(renderOpts, IncludeSecretRequirements())
	}
	result, diags = renderer.Render(ctx, ownerID, values.ValuesMap(), renderOpts...)
	if !o.skipSecretRequirements && !diags.HasErrors() {
		var missing []codersdk.SecretRequirementStatus
		for _, req := range result.SecretRequirements {
			if !req.Satisfied {
				missing = append(missing, req)
			}
		}
		if len(missing) > 0 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Missing required secrets",
				Detail:   formatMissingSecrets(missing),
				Extra: previewtypes.DiagnosticExtra{
					Code: DiagCodeMissingSecret,
				},
			})
		}
	}
	if diags.HasErrors() {
		return nil, parameterValidationError(diags)
	}
	output = result.Output

	// parameterNames is going to be used to remove any excess values left
	// around without a parameter.
	parameterNames := make(map[string]struct{}, len(output.Parameters))
	parameterError := parameterValidationError(nil)
	for _, parameter := range output.Parameters {
		parameterNames[parameter.Name] = struct{}{}

		// Validate mutability constraints.
		if !firstBuild && !parameter.Mutable {
			// previousValuesMap should be used over the first render output
			// for the previous state of parameters. The previous build
			// should emit all values, so the previousValuesMap should be
			// complete with all parameter values (user specified and defaults)
			originalValue, ok := previousValuesMap[parameter.Name]

			// Immutable parameters should not be changed after the first build.
			// If the value matches the previous input value, that is fine.
			//
			// If the previous value is not set, that means this is a new parameter. New
			// immutable parameters are allowed. This is an opinionated choice to prevent
			// workspaces failing to update or delete. Ideally we would block this, as
			// immutable parameters should only be able to be set at creation time.
			if ok && parameter.Value.AsString() != originalValue {
				var src *hcl.Range
				if parameter.Source != nil {
					src = &parameter.Source.HCLBlock().TypeRange
				}

				// An immutable parameter was changed, which is not allowed.
				// Add a failed diagnostic to the output.
				parameterError.Extend(parameter.Name, hcl.Diagnostics{
					&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Immutable parameter changed",
						Detail:   fmt.Sprintf("Parameter %q is not mutable, so it can't be updated after creating a workspace.", parameter.Name),
						Subject:  src,
					},
				})
			}
		}

		// Validate monotonic constraints. Monotonic parameters
		// require the value to only increase or only decrease
		// relative to the previous build.
		if !firstBuild {
			prevStr, hasPrev := previousValuesMap[parameter.Name]
			// Only validate on currently valid parameters. Do not load extra diagnostics if
			// the parameter is already invalid.
			if hasPrev && parameter.Value.Valid() {
			MonotonicValidationLoop:
				for _, v := range parameter.Validations {
					if v.Monotonic == nil || *v.Monotonic == "" {
						continue
					}

					validation := &provider.Validation{
						Monotonic:   *v.Monotonic,
						MinDisabled: true,
						MaxDisabled: true,
					}
					prev := prevStr
					if err := validation.Valid(provider.OptionType(parameter.Type), parameter.Value.AsString(), &prev); err != nil {
						parameterError.Extend(parameter.Name, hcl.Diagnostics{
							&hcl.Diagnostic{
								Severity: hcl.DiagError,
								Summary:  fmt.Sprintf("Parameter %q monotonicity", parameter.Name),
								Detail:   err.Error(),
							},
						})
						break MonotonicValidationLoop
					}
				}
			}
		}

		// TODO: Fix the `hcl.Diagnostics(...)` type casting. It should not be needed.
		if hcl.Diagnostics(parameter.Diagnostics).HasErrors() {
			// All validation errors are raised here for each parameter.
			parameterError.Extend(parameter.Name, hcl.Diagnostics(parameter.Diagnostics))
		}

		// If the parameter has a value, but it was not set explicitly by the user at any
		// build, then save the default value. An example where this is important is if a
		// template has a default value of 'region = us-west-2', but the user never sets
		// it. If the default value changes to 'region = us-east-1', we want to preserve
		// the original value of 'us-west-2' for the existing workspaces.
		//
		// parameter.Value will be populated from the default at this point. So grab it
		// from there.
		if _, ok := values[parameter.Name]; !ok && parameter.Value.IsKnown() && parameter.Value.Valid() {
			values[parameter.Name] = parameterValue{
				Value:  parameter.Value.AsString(),
				Source: sourceDefault,
			}
		}
	}

	// Delete any values that do not belong to a parameter. This is to not save
	// parameter values that have no effect. These leaky parameter values can cause
	// problems in the future, as it makes it challenging to remove values from the
	// database
	for k := range values {
		if _, ok := parameterNames[k]; !ok {
			delete(values, k)
		}
	}

	if parameterError.HasError() {
		// If there are any errors, return them.
		return nil, parameterError
	}

	// Return the values to be saved for the build.
	return values.ValuesMap(), nil
}

type parameterValueMap map[string]parameterValue

func (p parameterValueMap) ValuesMap() map[string]string {
	values := make(map[string]string, len(p))
	for name, paramValue := range p {
		values[name] = paramValue.Value
	}
	return values
}

func secretRequirementKind(env, file string) string {
	switch {
	case env != "" && file == "":
		return secretRequirementKindEnv
	case file != "" && env == "":
		return secretRequirementKindFile
	default:
		return ""
	}
}

func formatMissingSecrets(reqs []codersdk.SecretRequirementStatus) string {
	var b strings.Builder
	for i, req := range reqs {
		if i > 0 {
			_, _ = b.WriteString("\n")
		}
		switch secretRequirementKind(req.Env, req.File) {
		case secretRequirementKindEnv:
			_, _ = fmt.Fprintf(&b, "%s %s", secretRequirementKindEnv, req.Env)
		case secretRequirementKindFile:
			_, _ = fmt.Fprintf(&b, "%s %s", secretRequirementKindFile, req.File)
		default:
			// checkSecretRequirements filters malformed requirements produced
			// by preview before they reach the resolver.
			_, _ = b.WriteString("malformed secret requirement")
		}
		if req.HelpMessage != "" {
			_, _ = fmt.Fprintf(&b, ": %s", req.HelpMessage)
		}
	}
	return b.String()
}
