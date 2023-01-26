package rbac

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// rbacTraceAttributes are the attributes that are added to all spans created by
// the rbac package. These attributes should help to debug slow spans.
func rbacTraceAttributes(actor Subject, action Action, objectType string, extra ...attribute.KeyValue) trace.SpanStartOption {
	return trace.WithAttributes(
		append(extra,
			attribute.StringSlice("subject_roles", actor.SafeRoleNames()),
			attribute.Int("num_subject_roles", len(actor.SafeRoleNames())),
			attribute.Int("num_groups", len(actor.Groups)),
			attribute.String("scope", actor.SafeScopeName()),
			attribute.String("action", string(action)),
			attribute.String("object_type", objectType),
		)...)
}
