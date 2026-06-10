# Pre-tool decision policy: block the "bash" tool, case-insensitively.
#
# Evaluated once per assembled, client-bound tool call at the pre-tool hook.
# Rule queried by the decide kind: data.gateway.verdict
#
# input.tool_call carries {id, name, arguments, index} for the call being gated.

default verdict := "ALLOW"

verdict := "BLOCK" if lower(input.tool_call.name) == "bash"
