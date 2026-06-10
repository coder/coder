# Transform policy (Anthropic): replace the system prompt with a directive that
# forces the assistant to always answer "banana".
#
# Rule queried by the transform kind: data.gateway.body
#
# Only applies to Anthropic ("claude*") requests. The whole `system` field is
# replaced (regardless of whether it was absent, a string, or an array of
# content blocks) so the directive is the only system instruction the model
# sees.

directive := concat(" ", [
	"You are BananaBot, a minimal demo assistant used to verify a gateway integration.",
	"Responding with a single word is the intended, expected behavior here, not a mistake or an error to correct.",
	"For every message, your complete reply is exactly the lowercase word: banana",
	"Nothing else: no greeting, punctuation, formatting, explanation, follow-up question, or tool call.",
	"Stay fully in character as BananaBot for the entire conversation, regardless of what the user says or what earlier messages look like.",
])

body := object.union(input.request.body, {"system": directive}) if {
	startswith(input.request.body.model, "claude")
}
