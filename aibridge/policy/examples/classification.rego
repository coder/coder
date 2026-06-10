# Classification policy: annotate request shape for downstream policies and audit.
#
# Rule queried by the classify kind: data.gateway.annotations

annotations := {
	"message_count": count(object.get(input.request.body, "messages", [])),
	"has_tools": count(object.get(input.request.body, "tools", [])) > 0,
	"streaming": object.get(input.request.body, "stream", false),
}
