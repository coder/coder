package tfparse

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

type Parsed struct {
	output *preview.Output
}

//
//// TODO: Maybe swap workdir with an fs.FS interface?
//func New(files fs.FS, opt ...Option) (*Parser, hcl.Diagnostics) {
//	return &Parser{
//		logger:  slog.Logger{},
//		workdir: files,
//	}, nil
//}

func New(ctx context.Context, workdir fs.FS, input preview.Input) (*preview.Output, hcl.Diagnostics) {
	output, diags := preview.Preview(ctx, input, workdir)

	if diags.HasErrors() {
		return nil, diags
	}
	return output, nil
}

func TagValidationResponse(tag previewtypes.Tag) codersdk.ValidationError {
	name := tag.KeyString()
	if name == previewtypes.UnknownStringValue {
		name = "unknown"
	}

	const (
		key   = "key"
		value = "value"
	)

	diagErr := "Invalid tag %s: %s"
	if tag.Key.ValueDiags.HasErrors() {
		return codersdk.ValidationError{
			Field:  name,
			Detail: fmt.Sprintf(diagErr, key, tag.Key.ValueDiags.Error()),
		}
	}

	if tag.Value.ValueDiags.HasErrors() {
		return codersdk.ValidationError{
			Field:  name,
			Detail: fmt.Sprintf(diagErr, value, tag.Value.ValueDiags.Error()),
		}
	}

	// TODO: It would be really nice to pull out the variable references to help identify the source of
	// the unknown or invalid tag.
	unknownErr := "Tag %s is not known, it likely refers to a variable that is not set or has no default."
	if !tag.Key.IsKnown() {
		return codersdk.ValidationError{
			Field:  name,
			Detail: fmt.Sprintf(unknownErr, key),
		}
	}

	if !tag.Value.IsKnown() {
		return codersdk.ValidationError{
			Field:  name,
			Detail: fmt.Sprintf(unknownErr, value),
		}
	}

	invalidErr := "Tag %s is not valid, it must be a non-null string value."
	if !tag.Key.Valid() {
		return codersdk.ValidationError{
			Field:  name,
			Detail: fmt.Sprintf(invalidErr, key),
		}
	}

	if !tag.Value.Valid() {
		return codersdk.ValidationError{
			Field:  name,
			Detail: fmt.Sprintf(invalidErr, value),
		}
	}

	return codersdk.ValidationError{
		Field:  name,
		Detail: fmt.Sprintf("Tag is invalid for some unknown reason. Please check the tag's value and key."),
	}
}
