# Decision policy: block any request whose prompt mentions the word "banana".
#
# Rule queried by the decide kind: data.gateway.verdict
# Optional rule surfaced to the user on a block: data.gateway.message

default verdict := "ALLOW"

verdict := "BLOCK" if prompt_has_banana

# Optional: override the generic block message shown to the user.
message := "This request was blocked because it mentioned bananas." if prompt_has_banana

# User message with content sent as a plain string.
prompt_has_banana if {
	some msg in input.request.body.messages
	msg.role == "user"
	is_string(msg.content)
	contains(lower(msg.content), "banana")
}

# User message with content sent as an array of typed content blocks.
prompt_has_banana if {
	some msg in input.request.body.messages
	msg.role == "user"
	is_array(msg.content)
	some block in msg.content
	is_string(block.text)
	contains(lower(block.text), "banana")
}
