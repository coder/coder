# Routing policy: downgrade premium-tier models to a cheaper model for users
# who are not in the "premium" group.
#
# Rule queried by the route kind: data.gateway.model

model := "claude-sonnet-4-6" if {
	input.request.model == "claude-opus-4-8"
	not is_premium
}

is_premium if "premium" in object.get(input.identity, "groups", [])
