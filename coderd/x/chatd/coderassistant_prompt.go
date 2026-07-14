package chatd

import (
	"strings"

	"github.com/coder/coder/v2/coderd/database"
)

// CoderAssistantSystemPrompt is the system prompt used when a Coder
// Assistant chat session is created. The Coder Assistant is the
// built-in floating assistant available to both admins and regular
// users.
const CoderAssistantSystemPrompt = `You are the Coder Assistant, a built-in assistant for the Coder platform.
Introduce yourself as the Coder Assistant when starting a conversation.

<role>
You are a helpful, concise assistant that helps users and administrators manage their Coder deployment.
Scale your capabilities to the user's role:
- For admins: help with templates, user management, deployment settings, configuration, and troubleshooting.
- For members: help with workspaces, IDE connections, dotfiles, Git setup, and Coder features.
</role>

<behavior>
Use the Coder CLI and API tools available to you to execute actions directly rather than only describing steps.
Be proactive: suggest improvements, flag potential issues, and offer logical next steps.
Stay focused on Coder-related tasks. If a request is outside the Coder domain, politely redirect.
When helping with onboarding, guide the user through choosing a template and creating their first workspace.
Reference Coder documentation when it would help the user understand a concept or workflow.
</behavior>

<communication>
Be concise and direct.
No emojis unless the user explicitly asks for them.
Prefer action over explanation: do things for the user when possible.
If you are unsure about something, say so honestly rather than guessing.
</communication>`

// coderAssistantLabelKey is the chat label key that marks a chat as a
// Coder Assistant conversation.
const coderAssistantLabelKey = "coder-assistant"

// coderAssistantPageLabelKey is the chat label key the dashboard uses to
// report the path the user is currently viewing. The value is the raw
// dashboard pathname (for example "/workspaces").
const coderAssistantPageLabelKey = "coder-assistant-page"

// CoderAssistantUserContext renders a system instruction describing the
// chat owner so the assistant can tailor its behavior. currentPage is
// the dashboard path the user is viewing; pass an empty string when it
// is unknown.
func CoderAssistantUserContext(user database.User, roles []string, orgNames []string, currentPage string) string {
	var b strings.Builder
	_, _ = b.WriteString("<user-context>\n")
	_, _ = b.WriteString("You are assisting the following Coder user:\n")
	_, _ = b.WriteString("- Username: " + user.Username + "\n")
	if name := strings.TrimSpace(user.Name); name != "" {
		_, _ = b.WriteString("- Name: " + name + "\n")
	}
	rolesLine := "member (no elevated deployment roles)"
	if len(roles) > 0 {
		rolesLine = strings.Join(roles, ", ")
	}
	_, _ = b.WriteString("- Deployment roles: " + rolesLine + "\n")
	if len(orgNames) > 0 {
		_, _ = b.WriteString("- Organizations: " + strings.Join(orgNames, ", ") + "\n")
	}
	if page := sanitizeCoderAssistantPage(currentPage); page != "" {
		_, _ = b.WriteString("They are currently viewing the " + page + " page in the Coder dashboard.\n")
	}
	_, _ = b.WriteString("Tailor guidance to their permissions: deployment admins can manage templates, users, and settings; members can manage their own workspaces.\n")
	_, _ = b.WriteString("</user-context>")
	return b.String()
}

// sanitizeCoderAssistantPage validates a client-reported dashboard path
// before it is embedded in a system instruction. It returns an empty
// string unless the value looks like a plain absolute path.
func sanitizeCoderAssistantPage(page string) string {
	page = strings.TrimSpace(page)
	if page == "" || !strings.HasPrefix(page, "/") {
		return ""
	}
	if strings.ContainsAny(page, " \t\r\n<>`\"'\\") {
		return ""
	}
	return page
}

// IsCoderAssistantChat reports whether the given chat labels indicate a
// Coder Agent conversation.
func IsCoderAssistantChat(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	return labels[coderAssistantLabelKey] == "true"
}
