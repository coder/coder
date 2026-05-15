// Package skills defines the shared model for personal and workspace skills
// used by chatd.
//
// Glossary:
//
//   - Personal skill: A user-owned skill that follows the user across Coder
//     chats and workspaces, stored by Coder rather than discovered from a
//     workspace filesystem.
//   - Workspace skill: A skill discovered from the workspace filesystem,
//     currently under .agents/skills by default.
//   - Skill source: The origin of a skill available to chatd, such as personal
//     storage or workspace filesystem discovery.
//   - Skill alias: A chat or tool lookup name for a skill. Bare aliases use the
//     skill name. Qualified aliases use personal/<name> or workspace/<name>.
//
// Decision:
//
// Personal skills are stored by Coder. For each chat turn, chatd fetches
// personal skill metadata fresh, combines it with workspace skill metadata, and
// injects the available skills into the existing skill prompt.
// When chatd needs skill content, it resolves personal skills through the
// read_skill flow instead of syncing files into workspace filesystems.
//
// If a personal skill and workspace skill share the same kebab-case name, both
// are exposed with qualified aliases: personal/<name> for the personal skill
// and workspace/<name> for the workspace skill. One source must not silently
// override the other.
//
// Site admins can read and modify personal skill content. Personal skills are
// user-authored instructions, not secret material. Audit records can include
// raw Markdown content diffs alongside the actor, target user, and relevant
// metadata.
//
// Personal skill edits affect the next chat turn. Old chat turns are not exact
// snapshots of the personal skill state that existed when they ran.
//
// The v1 design does not include CLI support, web UI support, supporting files,
// organization-scoped personal skills, syncing personal skills into workspace
// filesystems, or stable public API documentation.
//
// Consequences:
//
// Chatd can use personal and workspace skills through one prompt and one read
// path, while storage remains owned by Coder instead of individual workspace
// filesystems. Fresh metadata keeps skill changes responsive, but chat history
// is less reproducible because old turns do not capture an exact copy of
// personal skill content.
//
// Explicit collision aliases make ambiguous names visible to users and tools.
// Admin access improves operability and abuse handling, but it creates a
// privacy trade-off that must remain clear in product and support expectations.
package skills
