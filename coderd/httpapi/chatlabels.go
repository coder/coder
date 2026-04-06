package httpapi

import (
	"fmt"
	"regexp"

	"github.com/coder/coder/v2/codersdk"
)

const (
	// maxLabelsPerChat is the maximum number of labels allowed on a
	// single chat.
	maxLabelsPerChat = 50
	// maxLabelKeyLength is the maximum length of a label key in bytes.
	maxLabelKeyLength = 64
	// maxLabelValueLength is the maximum length of a label value in
	// bytes.
	maxLabelValueLength = 256
)

// labelKeyRegex validates that a label key starts with an alphanumeric
// character and is followed by alphanumeric characters, dots, hyphens,
// underscores, or forward slashes.
var labelKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// ValidateChatLabels checks that the provided labels map conforms to the
// labeling constraints for chats. It returns a list of validation
// errors, one per violated constraint.
func ValidateChatLabels(labels map[string]string) []codersdk.ValidationError {
	var errs []codersdk.ValidationError

	if len(labels) > maxLabelsPerChat {
		errs = append(errs, codersdk.ValidationError{
			Field:  "labels",
			Detail: fmt.Sprintf("too many labels (%d); maximum is %d", len(labels), maxLabelsPerChat),
		})
	}

	for k, v := range labels {
		if k == "" {
			errs = append(errs, codersdk.ValidationError{
				Field:  "labels",
				Detail: "label key must not be empty",
			})
			continue
		}

		if len(k) > maxLabelKeyLength {
			errs = append(errs, codersdk.ValidationError{
				Field:  "labels",
				Detail: fmt.Sprintf("label key %q exceeds maximum length of %d bytes", k, maxLabelKeyLength),
			})
		}

		if !labelKeyRegex.MatchString(k) {
			errs = append(errs, codersdk.ValidationError{
				Field:  "labels",
				Detail: fmt.Sprintf("label key %q contains invalid characters; must match %s", k, labelKeyRegex.String()),
			})
		}

		if v == "" {
			errs = append(errs, codersdk.ValidationError{
				Field:  "labels",
				Detail: fmt.Sprintf("label value for key %q must not be empty", k),
			})
		}

		if len(v) > maxLabelValueLength {
			errs = append(errs, codersdk.ValidationError{
				Field:  "labels",
				Detail: fmt.Sprintf("label value for key %q exceeds maximum length of %d bytes", k, maxLabelValueLength),
			})
		}
	}

	return errs
}
