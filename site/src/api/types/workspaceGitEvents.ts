// TODO: These types are placeholders. Run `make gen` once the backend
// API is implemented to generate proper types from the Go SDK.

export interface WorkspaceGitEvent {
	id: string;
	workspace_id: string;
	agent_id: string;
	owner_id: string;
	organization_id: string;
	event_type: "session_start" | "commit" | "push" | "session_end";
	session_id: string | null;
	commit_sha: string | null;
	commit_message: string | null;
	branch: string | null;
	repo_name: string | null;
	files_changed: string[];
	agent_name: string;
	ai_bridge_interception_id: string | null;
	created_at: string;
	// Joined fields from API
	owner_username?: string;
	owner_avatar_url?: string;
	workspace_name?: string;
}

export interface WorkspaceGitEventSession {
	session_id: string;
	owner_id: string;
	workspace_id: string;
	agent_name: string;
	repo_name: string | null;
	branch: string | null;
	started_at: string;
	ended_at: string | null;
	commit_count: number;
	push_count: number;
	// Joined fields from API
	owner_username?: string;
	owner_avatar_url?: string;
	workspace_name?: string;
}
