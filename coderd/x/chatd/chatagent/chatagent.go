package chatagent

import "strings"

// Suffix marks chat-designated agents during the current PoC. This naming
// convention is an implementation detail, not a stable contract.
const Suffix = "-coderd-chat"

// IsChatAgent reports whether name uses the chat-agent suffix convention.
func IsChatAgent(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), Suffix)
}
