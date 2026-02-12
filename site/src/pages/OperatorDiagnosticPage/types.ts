import type {
	ConnectionType,
	WorkspaceConnectionStatus,
} from "api/typesGenerated";

// Top-level response returned by the diagnostic endpoint.
export interface UserDiagnosticResponse {
	user: DiagnosticUser;
	generated_at: string;
	time_window: DiagnosticTimeWindow;
	summary: DiagnosticSummary;
	current_connections: DiagnosticConnection[];
	workspaces: DiagnosticWorkspace[];
	patterns: DiagnosticPattern[];
}

export interface DiagnosticUser {
	id: string;
	username: string;
	name: string;
	avatar_url: string;
	email: string;
	roles: string[];
	last_seen_at: string;
	created_at: string;
}

export interface DiagnosticTimeWindow {
	start: string;
	end: string;
	hours: number;
}

export interface DiagnosticSummary {
	total_sessions: number;
	total_connections: number;
	active_connections: number;
	by_status: {
		ongoing: number;
		clean: number;
		lost: number;
		workspace_stopped: number;
		workspace_deleted: number;
	};
	by_type: Record<string, number>;
	network: {
		p2p_connections: number;
		derp_connections: number;
		avg_latency_ms: number | null;
		p95_latency_ms: number | null;
		primary_derp_region: string | null;
	};
	headline: string;
}

export interface DiagnosticConnection {
	id: string;
	workspace_id: string;
	workspace_name: string;
	agent_id: string;
	agent_name: string;
	ip: string;
	client_hostname: string;
	short_description: string;
	type: ConnectionType;
	detail: string;
	status: WorkspaceConnectionStatus;
	started_at: string;
	p2p: boolean | null;
	latency_ms: number | null;
	home_derp: { id: number; name: string } | null;
	explanation: string;
}

export interface DiagnosticWorkspace {
	id: string;
	name: string;
	owner_username: string;
	status: string;
	template_name: string;
	template_display_name: string;
	health: "healthy" | "degraded" | "unhealthy" | "inactive";
	health_reason: string;
	sessions: DiagnosticSession[];
}

export interface DiagnosticSession {
	id: string;
	workspace_id: string;
	workspace_name: string;
	agent_name: string;
	ip: string;
	client_hostname: string;
	short_description: string;
	started_at: string;
	ended_at: string | null;
	duration_seconds: number | null;
	status: WorkspaceConnectionStatus;
	disconnect_reason: string;
	explanation: string;
	network: {
		p2p: boolean | null;
		avg_latency_ms: number | null;
		home_derp: string | null;
	};
	connections: DiagnosticSessionConnection[];
	timeline: DiagnosticTimelineEvent[];
}

export interface DiagnosticSessionConnection {
	id: string;
	type: ConnectionType;
	detail: string;
	connected_at: string;
	disconnected_at: string | null;
	status: WorkspaceConnectionStatus;
	exit_code: number | null;
	explanation: string;
}

export type DiagnosticTimelineEventKind =
	| "tunnel_created"
	| "tunnel_removed"
	| "node_update"
	| "peer_lost"
	| "peer_recovered"
	| "connection_opened"
	| "connection_closed"
	| "derp_fallback"
	| "p2p_established"
	| "latency_spike"
	| "workspace_state_change";

export interface DiagnosticTimelineEvent {
	timestamp: string;
	kind: DiagnosticTimelineEventKind;
	description: string;
	metadata: Record<string, string | number | boolean>;
	severity: "info" | "warning" | "error";
}

export type DiagnosticPatternType =
	| "device_sleep"
	| "workspace_autostart"
	| "network_policy"
	| "agent_crash"
	| "latency_degradation"
	| "derp_fallback"
	| "clean_usage"
	| "unknown_drops";

export interface DiagnosticPattern {
	id: string;
	type: DiagnosticPatternType;
	severity: "info" | "warning" | "critical";
	affected_sessions: number;
	total_sessions: number;
	title: string;
	description: string;
	commonalities: {
		connection_types: string[];
		client_descriptions: string[];
		duration_range: { min_seconds: number; max_seconds: number } | null;
		disconnect_reasons: string[];
		time_of_day_range: string | null;
	};
	recommendation: string;
}
