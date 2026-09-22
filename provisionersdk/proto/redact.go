package proto

// RedactedValue replaces sensitive template variable and parameter values
// anywhere they are logged or returned to a caller.
const RedactedValue = "*redacted*"

// RedactVariableValues returns a copy of the values with sensitive entries
// replaced by RedactedValue. Safe for logging.
func RedactVariableValues(values []*VariableValue) []*VariableValue {
	redacted := make([]*VariableValue, 0, len(values))
	for _, v := range values {
		if v.Sensitive {
			redacted = append(redacted, &VariableValue{
				Name:      v.Name,
				Value:     RedactedValue,
				Sensitive: true,
			})
			continue
		}
		redacted = append(redacted, v)
	}
	return redacted
}

// RedactRichParameterValues returns a copy of the values with sensitive
// entries replaced by RedactedValue. Safe for logging.
func RedactRichParameterValues(values []*RichParameterValue) []*RichParameterValue {
	redacted := make([]*RichParameterValue, 0, len(values))
	for _, v := range values {
		if v.Sensitive {
			redacted = append(redacted, &RichParameterValue{
				Name:      v.Name,
				Value:     RedactedValue,
				Sensitive: true,
			})
			continue
		}
		redacted = append(redacted, v)
	}
	return redacted
}
