import type {
	DiagnosticConnection,
	DiagnosticPattern,
	DiagnosticSession,
	DiagnosticTimelineEvent,
	DiagnosticWorkspace,
	UserDiagnosticResponse,
} from "./types";

function hoursAgo(h: number): string {
	return new Date(Date.now() - h * 60 * 60 * 1000).toISOString();
}

function minutesAgo(m: number): string {
	return new Date(Date.now() - m * 60 * 1000).toISOString();
}

// ---------------------------------------------------------------------------
// SCENARIO 1: Workspace Auto-Stop
// User john-ops has 5 sessions over 5 days, all ended by workspace auto-stop
// at midnight. No current connections.
// ---------------------------------------------------------------------------

const wsStopWorkspaceId = "b1a2c3d4-e5f6-7890-abcd-000000000001";

function makeWsStopSession(
	dayOffset: number,
	durationHours: number,
): DiagnosticSession {
	const id = `b1a2c3d4-e5f6-7890-abcd-1000000000${String(dayOffset).padStart(2, "0")}`;
	const startH = dayOffset * 24 + (24 - durationHours);
	const endH = dayOffset * 24;
	const connId = `b1a2c3d4-e5f6-7890-abcd-2000000000${String(dayOffset).padStart(2, "0")}`;

	const timeline: DiagnosticTimelineEvent[] = [
		{
			timestamp: hoursAgo(startH),
			kind: "tunnel_created",
			description: "Tailnet tunnel established with P2P connection.",
			metadata: { p2p: true, latency_ms: 8 },
			severity: "info",
		},
		{
			timestamp: hoursAgo(startH - 0.1),
			kind: "connection_opened",
			description: "VS Code connected via SSH tunnel.",
			metadata: { type: "vscode", detail: "VS Code Desktop 1.96" },
			severity: "info",
		},
		{
			timestamp: hoursAgo(endH + 0.01),
			kind: "workspace_state_change",
			description: "Workspace transitioned to stopped by auto-stop schedule.",
			metadata: { from: "running", to: "stopped", trigger: "autostop" },
			severity: "warning",
		},
		{
			timestamp: hoursAgo(endH),
			kind: "connection_closed",
			description: "Connection closed: workspace stopped.",
			metadata: { reason: "workspace stopped" },
			severity: "info",
		},
	];

	return {
		id,
		workspace_id: wsStopWorkspaceId,
		workspace_name: "web-backend",
		agent_name: "main",
		ip: "fd7a:115c:a1e0:49d6::1001",
		client_hostname: "john-laptop.local",
		short_description: "VS Code Desktop (macOS, arm64)",
		started_at: hoursAgo(startH),
		ended_at: hoursAgo(endH),
		duration_seconds: durationHours * 3600,
		status: "clean_disconnected",
		disconnect_reason: "workspace stopped",
		explanation:
			"Session ended cleanly when the workspace was stopped by the auto-stop schedule.",
		network: {
			p2p: true,
			avg_latency_ms: 8,
			home_derp: null,
		},
		connections: [
			{
				id: connId,
				type: "vscode",
				detail: "VS Code Desktop 1.96",
				connected_at: hoursAgo(startH),
				disconnected_at: hoursAgo(endH),
				status: "clean_disconnected",
				exit_code: 0,
				explanation: "Disconnected when workspace auto-stopped.",
			},
		],
		timeline,
	};
}

const wsStopSessions: DiagnosticSession[] = [
	makeWsStopSession(1, 16),
	makeWsStopSession(2, 14),
	makeWsStopSession(3, 12),
	makeWsStopSession(4, 15),
	makeWsStopSession(5, 8),
];

const wsStopWorkspace: DiagnosticWorkspace = {
	id: wsStopWorkspaceId,
	name: "web-backend",
	owner_username: "john-ops",
	status: "stopped",
	template_name: "docker-dev",
	template_display_name: "Docker Development",
	health: "inactive",
	health_reason: "Workspace is stopped.",
	sessions: wsStopSessions,
};

const wsStopPattern: DiagnosticPattern = {
	id: "b1a2c3d4-e5f6-7890-abcd-300000000001",
	type: "workspace_autostart",
	severity: "warning",
	affected_sessions: 5,
	total_sessions: 5,
	title: "All disconnects caused by workspace auto-stop",
	description:
		"Every session in the last 5 days was terminated by the workspace auto-stop schedule at midnight UTC. No unexpected disconnects detected.",
	commonalities: {
		connection_types: ["vscode"],
		client_descriptions: ["VS Code Desktop (macOS, arm64)"],
		duration_range: { min_seconds: 28800, max_seconds: 57600 },
		disconnect_reasons: ["workspace stopped"],
		time_of_day_range: "23:55-00:05 UTC",
	},
	recommendation:
		"If the user needs longer sessions, consider adjusting the workspace auto-stop schedule or increasing the TTL.",
};

export const SCENARIO_WORKSPACE_STOP: UserDiagnosticResponse = {
	user: {
		id: "b1a2c3d4-e5f6-7890-abcd-000000000010",
		username: "john-ops",
		name: "John Operators",
		avatar_url: "",
		email: "john@example.com",
		roles: ["member"],
		last_seen_at: hoursAgo(24),
		created_at: hoursAgo(24 * 90),
	},
	generated_at: new Date().toISOString(),
	time_window: {
		start: hoursAgo(120),
		end: new Date().toISOString(),
		hours: 120,
	},
	summary: {
		total_sessions: 5,
		total_connections: 5,
		active_connections: 0,
		by_status: {
			ongoing: 0,
			clean: 5,
			lost: 0,
			workspace_stopped: 5,
			workspace_deleted: 0,
		},
		by_type: { vscode: 5 },
		network: {
			p2p_connections: 5,
			derp_connections: 0,
			avg_latency_ms: 8,
			p95_latency_ms: 12,
			primary_derp_region: null,
		},
		headline:
			"All 5 disconnects in the last 5 days were caused by workspace auto-stop at midnight.",
	},
	current_connections: [],
	workspaces: [wsStopWorkspace],
	patterns: [wsStopPattern],
};

// ---------------------------------------------------------------------------
// SCENARIO 2: Device Sleep
// User sarah-chen has 7 sessions in 24h: 4 clean (8h+), 3 lost (5-8 min).
// 1 current VS Code connection, P2P.
// ---------------------------------------------------------------------------

const sleepWorkspaceId = "c2b3d4e5-f6a7-8901-bcde-000000000001";
const sleepAgentId = "c2b3d4e5-f6a7-8901-bcde-000000000002";

function makeSleepCleanSession(
	offsetHours: number,
	durationHours: number,
	index: number,
): DiagnosticSession {
	const id = `c2b3d4e5-f6a7-8901-bcde-1100000000${String(index).padStart(2, "0")}`;
	const connId = `c2b3d4e5-f6a7-8901-bcde-2100000000${String(index).padStart(2, "0")}`;

	const timeline: DiagnosticTimelineEvent[] = [
		{
			timestamp: hoursAgo(offsetHours),
			kind: "tunnel_created",
			description: "Tailnet tunnel established with P2P connection.",
			metadata: { p2p: true, latency_ms: 11 },
			severity: "info",
		},
		{
			timestamp: hoursAgo(offsetHours - 0.05),
			kind: "p2p_established",
			description: "Direct peer-to-peer connection established.",
			metadata: { latency_ms: 11 },
			severity: "info",
		},
		{
			timestamp: hoursAgo(offsetHours - 0.1),
			kind: "connection_opened",
			description: "VS Code connected via SSH tunnel.",
			metadata: { type: "vscode", detail: "VS Code Desktop 1.96" },
			severity: "info",
		},
		{
			timestamp: hoursAgo(offsetHours - durationHours),
			kind: "connection_closed",
			description: "Connection closed cleanly by client.",
			metadata: { exit_code: 0 },
			severity: "info",
		},
	];

	return {
		id,
		workspace_id: sleepWorkspaceId,
		workspace_name: "frontend-app",
		agent_name: "dev",
		ip: "fd7a:115c:a1e0:49d6::2001",
		client_hostname: "sarah-macbook.local",
		short_description: "VS Code Desktop (macOS, arm64)",
		started_at: hoursAgo(offsetHours),
		ended_at: hoursAgo(offsetHours - durationHours),
		duration_seconds: durationHours * 3600,
		status: "clean_disconnected",
		disconnect_reason: "client disconnected",
		explanation: "",
		network: {
			p2p: true,
			avg_latency_ms: 11,
			home_derp: null,
		},
		connections: [
			{
				id: connId,
				type: "vscode",
				detail: "VS Code Desktop 1.96",
				connected_at: hoursAgo(offsetHours),
				disconnected_at: hoursAgo(offsetHours - durationHours),
				status: "clean_disconnected",
				exit_code: 0,
				explanation: "",
			},
		],
		timeline,
	};
}

function makeSleepLostSession(
	offsetMinutes: number,
	durationMinutes: number,
	index: number,
): DiagnosticSession {
	const id = `c2b3d4e5-f6a7-8901-bcde-1200000000${String(index).padStart(2, "0")}`;
	const connId = `c2b3d4e5-f6a7-8901-bcde-2200000000${String(index).padStart(2, "0")}`;
	const startMin = offsetMinutes;
	const endMin = offsetMinutes - durationMinutes;

	const timeline: DiagnosticTimelineEvent[] = [
		{
			timestamp: minutesAgo(startMin),
			kind: "tunnel_created",
			description: "Tailnet tunnel established with P2P connection.",
			metadata: { p2p: true, latency_ms: 12 },
			severity: "info",
		},
		{
			timestamp: minutesAgo(startMin - 1),
			kind: "connection_opened",
			description: "VS Code connected via SSH tunnel.",
			metadata: { type: "vscode", detail: "VS Code Desktop 1.96" },
			severity: "info",
		},
		{
			timestamp: minutesAgo(endMin + 2),
			kind: "latency_spike",
			description: "Latency spike detected before connection loss.",
			metadata: { latency_ms: 4500 },
			severity: "warning",
		},
		{
			timestamp: minutesAgo(endMin + 1),
			kind: "peer_lost",
			description:
				"Peer unreachable. Consistent with device entering sleep mode.",
			metadata: { last_handshake_age_s: 120 },
			severity: "error",
		},
		{
			timestamp: minutesAgo(endMin),
			kind: "connection_closed",
			description: "Connection lost: control channel timeout.",
			metadata: { reason: "control_lost" },
			severity: "error",
		},
	];

	return {
		id,
		workspace_id: sleepWorkspaceId,
		workspace_name: "frontend-app",
		agent_name: "dev",
		ip: "fd7a:115c:a1e0:49d6::2001",
		client_hostname: "sarah-macbook.local",
		short_description: "VS Code Desktop (macOS, arm64)",
		started_at: minutesAgo(startMin),
		ended_at: minutesAgo(endMin),
		duration_seconds: durationMinutes * 60,
		status: "control_lost",
		disconnect_reason: "",
		explanation:
			"Session lost after a short period, consistent with the client device entering sleep mode.",
		network: {
			p2p: true,
			avg_latency_ms: 12,
			home_derp: null,
		},
		connections: [
			{
				id: connId,
				type: "vscode",
				detail: "VS Code Desktop 1.96",
				connected_at: minutesAgo(startMin),
				disconnected_at: minutesAgo(endMin),
				status: "control_lost",
				exit_code: null,
				explanation:
					"Connection lost without graceful shutdown. Duration (~5-8 min) suggests device sleep.",
			},
		],
		timeline,
	};
}

const sleepSessions: DiagnosticSession[] = [
	makeSleepCleanSession(22, 8, 1),
	makeSleepLostSession(840, 7, 1),
	makeSleepCleanSession(16, 9, 2),
	makeSleepLostSession(600, 5, 2),
	makeSleepCleanSession(10, 8.5, 3),
	makeSleepLostSession(360, 8, 3),
	makeSleepCleanSession(4, 3, 4),
];

const sleepCurrentConnection: DiagnosticConnection = {
	id: "c2b3d4e5-f6a7-8901-bcde-300000000001",
	workspace_id: sleepWorkspaceId,
	workspace_name: "frontend-app",
	agent_id: sleepAgentId,
	agent_name: "dev",
	ip: "fd7a:115c:a1e0:49d6::2001",
	client_hostname: "sarah-macbook.local",
	short_description: "VS Code Desktop (macOS, arm64)",
	type: "vscode",
	detail: "VS Code Desktop 1.96",
	status: "ongoing",
	started_at: minutesAgo(45),
	p2p: true,
	latency_ms: 12,
	home_derp: null,
	explanation: "",
};

const sleepWorkspace: DiagnosticWorkspace = {
	id: sleepWorkspaceId,
	name: "frontend-app",
	owner_username: "sarah-chen",
	status: "running",
	template_name: "react-dev",
	template_display_name: "React Development",
	health: "healthy",
	health_reason: "",
	sessions: sleepSessions,
};

const sleepPattern: DiagnosticPattern = {
	id: "c2b3d4e5-f6a7-8901-bcde-400000000001",
	type: "device_sleep",
	severity: "warning",
	affected_sessions: 3,
	total_sessions: 7,
	title: "Short sessions lost to suspected device sleep",
	description:
		"3 of 7 sessions were lost after only 5-8 minutes. All losses occurred on the same VS Code + macOS client with no graceful disconnect, consistent with a laptop lid close or device sleep.",
	commonalities: {
		connection_types: ["vscode"],
		client_descriptions: ["VS Code Desktop (macOS, arm64)"],
		duration_range: { min_seconds: 300, max_seconds: 480 },
		disconnect_reasons: [],
		time_of_day_range: null,
	},
	recommendation:
		"Check the user's device power settings. macOS aggressive sleep can drop tailnet tunnels before the keep-alive fires.",
};

export const SCENARIO_DEVICE_SLEEP: UserDiagnosticResponse = {
	user: {
		id: "c2b3d4e5-f6a7-8901-bcde-000000000010",
		username: "sarah-chen",
		name: "Sarah Chen",
		avatar_url: "",
		email: "sarah@example.com",
		roles: ["member"],
		last_seen_at: minutesAgo(2),
		created_at: hoursAgo(24 * 180),
	},
	generated_at: new Date().toISOString(),
	time_window: {
		start: hoursAgo(24),
		end: new Date().toISOString(),
		hours: 24,
	},
	summary: {
		total_sessions: 7,
		total_connections: 8,
		active_connections: 1,
		by_status: {
			ongoing: 1,
			clean: 4,
			lost: 3,
			workspace_stopped: 0,
			workspace_deleted: 0,
		},
		by_type: { vscode: 8 },
		network: {
			p2p_connections: 8,
			derp_connections: 0,
			avg_latency_ms: 12,
			p95_latency_ms: 18,
			primary_derp_region: null,
		},
		headline:
			"3 of 7 sessions lost in 24h. All losses are VS Code on macOS, 5-8 min duration, consistent with device sleep.",
	},
	current_connections: [sleepCurrentConnection],
	workspaces: [sleepWorkspace],
	patterns: [sleepPattern],
};

// ---------------------------------------------------------------------------
// SCENARIO 3: DERP Fallback
// User alex-dev has 4 sessions in 48h, all through DERP relay. High latency.
// 2 current connections via DERP.
// ---------------------------------------------------------------------------

const derpWorkspaceId = "d3c4e5f6-a7b8-9012-cdef-000000000001";
const derpAgentId = "d3c4e5f6-a7b8-9012-cdef-000000000002";

function makeDerpSession(
	offsetHours: number,
	durationHours: number,
	avgLatency: number,
	index: number,
): DiagnosticSession {
	const id = `d3c4e5f6-a7b8-9012-cdef-1100000000${String(index).padStart(2, "0")}`;
	const sshConnId = `d3c4e5f6-a7b8-9012-cdef-2100000000${String(index).padStart(2, "0")}`;
	const vscodeConnId = `d3c4e5f6-a7b8-9012-cdef-2200000000${String(index).padStart(2, "0")}`;

	const timeline: DiagnosticTimelineEvent[] = [
		{
			timestamp: hoursAgo(offsetHours),
			kind: "tunnel_created",
			description: "Tailnet tunnel established via DERP relay us-east-1.",
			metadata: {
				p2p: false,
				derp_region: "us-east-1",
				latency_ms: avgLatency,
			},
			severity: "info",
		},
		{
			timestamp: hoursAgo(offsetHours - 0.05),
			kind: "derp_fallback",
			description:
				"P2P connection failed. Falling back to DERP relay us-east-1.",
			metadata: {
				reason: "no direct path",
				derp_region: "us-east-1",
				latency_ms: avgLatency,
			},
			severity: "warning",
		},
		{
			timestamp: hoursAgo(offsetHours - durationHours),
			kind: "connection_closed",
			description: "Connection closed cleanly by client.",
			metadata: { exit_code: 0 },
			severity: "info",
		},
	];

	return {
		id,
		workspace_id: derpWorkspaceId,
		workspace_name: "data-pipeline",
		agent_name: "main",
		ip: "fd7a:115c:a1e0:49d6::3001",
		client_hostname: "alex-workstation.corp.internal",
		short_description: "SSH Client (Linux, amd64)",
		started_at: hoursAgo(offsetHours),
		ended_at: hoursAgo(offsetHours - durationHours),
		duration_seconds: durationHours * 3600,
		status: "clean_disconnected",
		disconnect_reason: "client disconnected",
		explanation:
			"Session completed normally but was relayed via DERP the entire time, resulting in elevated latency.",
		network: {
			p2p: false,
			avg_latency_ms: avgLatency,
			home_derp: "us-east-1",
		},
		connections: [
			{
				id: sshConnId,
				type: "ssh",
				detail: "OpenSSH 9.6",
				connected_at: hoursAgo(offsetHours),
				disconnected_at: hoursAgo(offsetHours - durationHours),
				status: "clean_disconnected",
				exit_code: 0,
				explanation: "",
			},
			{
				id: vscodeConnId,
				type: "vscode",
				detail: "VS Code Remote SSH 1.96",
				connected_at: hoursAgo(offsetHours - 0.1),
				disconnected_at: hoursAgo(offsetHours - durationHours),
				status: "clean_disconnected",
				exit_code: 0,
				explanation: "",
			},
		],
		timeline,
	};
}

const derpSessions: DiagnosticSession[] = [
	makeDerpSession(44, 10, 280, 1),
	makeDerpSession(32, 8, 310, 2),
	makeDerpSession(20, 6, 260, 3),
	makeDerpSession(8, 5, 350, 4),
];

const derpCurrentConnections: DiagnosticConnection[] = [
	{
		id: "d3c4e5f6-a7b8-9012-cdef-300000000001",
		workspace_id: derpWorkspaceId,
		workspace_name: "data-pipeline",
		agent_id: derpAgentId,
		agent_name: "main",
		ip: "fd7a:115c:a1e0:49d6::3001",
		client_hostname: "alex-workstation.corp.internal",
		short_description: "SSH Client (Linux, amd64)",
		type: "ssh",
		detail: "OpenSSH 9.6",
		status: "ongoing",
		started_at: minutesAgo(90),
		p2p: false,
		latency_ms: 280,
		home_derp: { id: 1, name: "us-east-1" },
		explanation: "",
	},
	{
		id: "d3c4e5f6-a7b8-9012-cdef-300000000002",
		workspace_id: derpWorkspaceId,
		workspace_name: "data-pipeline",
		agent_id: derpAgentId,
		agent_name: "main",
		ip: "fd7a:115c:a1e0:49d6::3001",
		client_hostname: "alex-workstation.corp.internal",
		short_description: "VS Code Remote SSH (Linux, amd64)",
		type: "vscode",
		detail: "VS Code Remote SSH 1.96",
		status: "ongoing",
		started_at: minutesAgo(85),
		p2p: false,
		latency_ms: 310,
		home_derp: { id: 1, name: "us-east-1" },
		explanation: "",
	},
];

const derpWorkspace: DiagnosticWorkspace = {
	id: derpWorkspaceId,
	name: "data-pipeline",
	owner_username: "alex-dev",
	status: "running",
	template_name: "k8s-data",
	template_display_name: "Kubernetes Data Pipeline",
	health: "healthy",
	health_reason: "",
	sessions: derpSessions,
};

const derpPattern: DiagnosticPattern = {
	id: "d3c4e5f6-a7b8-9012-cdef-400000000001",
	type: "network_policy",
	severity: "warning",
	affected_sessions: 4,
	total_sessions: 4,
	title: "All connections relayed via DERP",
	description:
		"No direct P2P connections detected across 4 sessions. All traffic is routed through DERP relay us-east-1 with average latency of 290ms. This typically indicates a firewall or NAT policy blocking UDP hole-punching.",
	commonalities: {
		connection_types: ["ssh", "vscode"],
		client_descriptions: [
			"SSH Client (Linux, amd64)",
			"VS Code Remote SSH (Linux, amd64)",
		],
		duration_range: null,
		disconnect_reasons: ["client disconnected"],
		time_of_day_range: null,
	},
	recommendation:
		"Check the corporate firewall rules for UDP traffic on STUN ports (3478). Enabling UDP hole-punching would allow direct P2P connections with significantly lower latency.",
};

export const SCENARIO_DERP_FALLBACK: UserDiagnosticResponse = {
	user: {
		id: "d3c4e5f6-a7b8-9012-cdef-000000000010",
		username: "alex-dev",
		name: "Alex Developer",
		avatar_url: "",
		email: "alex@example.com",
		roles: ["member"],
		last_seen_at: minutesAgo(5),
		created_at: hoursAgo(24 * 60),
	},
	generated_at: new Date().toISOString(),
	time_window: {
		start: hoursAgo(48),
		end: new Date().toISOString(),
		hours: 48,
	},
	summary: {
		total_sessions: 4,
		total_connections: 10,
		active_connections: 2,
		by_status: {
			ongoing: 2,
			clean: 4,
			lost: 0,
			workspace_stopped: 0,
			workspace_deleted: 0,
		},
		by_type: { ssh: 5, vscode: 5 },
		network: {
			p2p_connections: 0,
			derp_connections: 10,
			avg_latency_ms: 290,
			p95_latency_ms: 350,
			primary_derp_region: "us-east-1",
		},
		headline:
			"All connections relayed via DERP us-east-1. No direct P2P connections detected. Average latency 290ms.",
	},
	current_connections: derpCurrentConnections,
	workspaces: [derpWorkspace],
	patterns: [derpPattern],
};

// ---------------------------------------------------------------------------
// SCENARIO 4: Agent Crash
// User priya-ml has 8 sessions across 3 workspaces: 6 lost on "ml-training",
// 2 clean on others. 2 current connections on healthy workspaces.
// ---------------------------------------------------------------------------

const crashMlWorkspaceId = "e4d5f6a7-b8c9-0123-defa-000000000001";
const crashMlAgentId = "e4d5f6a7-b8c9-0123-defa-000000000002";
const crashApiWorkspaceId = "e4d5f6a7-b8c9-0123-defa-000000000003";
const crashApiAgentId = "e4d5f6a7-b8c9-0123-defa-000000000004";
const crashNotebookWorkspaceId = "e4d5f6a7-b8c9-0123-defa-000000000005";
const crashNotebookAgentId = "e4d5f6a7-b8c9-0123-defa-000000000006";

function makeCrashLostSession(
	offsetMinutes: number,
	durationMinutes: number,
	index: number,
): DiagnosticSession {
	const id = `e4d5f6a7-b8c9-0123-defa-1100000000${String(index).padStart(2, "0")}`;
	const connId = `e4d5f6a7-b8c9-0123-defa-2100000000${String(index).padStart(2, "0")}`;
	const portFwdId = `e4d5f6a7-b8c9-0123-defa-2200000000${String(index).padStart(2, "0")}`;

	const timeline: DiagnosticTimelineEvent[] = [
		{
			timestamp: minutesAgo(offsetMinutes),
			kind: "tunnel_created",
			description: "Tailnet tunnel established with P2P connection.",
			metadata: { p2p: true, latency_ms: 15 },
			severity: "info",
		},
		{
			timestamp: minutesAgo(offsetMinutes - 1),
			kind: "connection_opened",
			description: "SSH session opened for ML training job.",
			metadata: { type: "ssh", detail: "OpenSSH 9.6" },
			severity: "info",
		},
		{
			timestamp: minutesAgo(offsetMinutes - 2),
			kind: "connection_opened",
			description: "Port forwarding opened for TensorBoard.",
			metadata: { type: "port_forwarding", detail: "localhost:6006" },
			severity: "info",
		},
		{
			timestamp: minutesAgo(offsetMinutes - durationMinutes + 1),
			kind: "peer_lost",
			description: "Agent became unreachable. No handshake response.",
			metadata: { last_handshake_age_s: 300, agent_id: crashMlAgentId },
			severity: "error",
		},
		{
			timestamp: minutesAgo(offsetMinutes - durationMinutes),
			kind: "connection_closed",
			description:
				"All connections lost: agent timeout after 5 minutes without response.",
			metadata: { reason: "agent timeout" },
			severity: "error",
		},
	];

	return {
		id,
		workspace_id: crashMlWorkspaceId,
		workspace_name: "ml-training",
		agent_name: "gpu",
		ip: "fd7a:115c:a1e0:49d6::4001",
		client_hostname: "priya-laptop.local",
		short_description: "SSH Client (macOS, arm64)",
		started_at: minutesAgo(offsetMinutes),
		ended_at: minutesAgo(offsetMinutes - durationMinutes),
		duration_seconds: durationMinutes * 60,
		status: "control_lost",
		disconnect_reason: "agent timeout",
		explanation:
			"Session lost when the workspace agent stopped responding, likely due to an agent crash or OOM kill on the ML training workspace.",
		network: {
			p2p: true,
			avg_latency_ms: 15,
			home_derp: null,
		},
		connections: [
			{
				id: connId,
				type: "ssh",
				detail: "OpenSSH 9.6",
				connected_at: minutesAgo(offsetMinutes),
				disconnected_at: minutesAgo(offsetMinutes - durationMinutes),
				status: "control_lost",
				exit_code: null,
				explanation: "SSH connection lost when agent timed out.",
			},
			{
				id: portFwdId,
				type: "port_forwarding",
				detail: "localhost:6006",
				connected_at: minutesAgo(offsetMinutes - 2),
				disconnected_at: minutesAgo(offsetMinutes - durationMinutes),
				status: "control_lost",
				exit_code: null,
				explanation: "Port forward dropped when agent timed out.",
			},
		],
		timeline,
	};
}

function makeCrashCleanSession(
	workspaceId: string,
	workspaceName: string,
	agentName: string,
	offsetHours: number,
	durationHours: number,
	index: number,
): DiagnosticSession {
	const id = `e4d5f6a7-b8c9-0123-defa-1200000000${String(index).padStart(2, "0")}`;
	const connId = `e4d5f6a7-b8c9-0123-defa-2300000000${String(index).padStart(2, "0")}`;

	const timeline: DiagnosticTimelineEvent[] = [
		{
			timestamp: hoursAgo(offsetHours),
			kind: "tunnel_created",
			description: "Tailnet tunnel established with P2P connection.",
			metadata: { p2p: true, latency_ms: 10 },
			severity: "info",
		},
		{
			timestamp: hoursAgo(offsetHours - 0.1),
			kind: "connection_opened",
			description: "VS Code connected via SSH tunnel.",
			metadata: { type: "vscode", detail: "VS Code Desktop 1.96" },
			severity: "info",
		},
		{
			timestamp: hoursAgo(offsetHours - durationHours),
			kind: "connection_closed",
			description: "Connection closed cleanly by client.",
			metadata: { exit_code: 0 },
			severity: "info",
		},
	];

	return {
		id,
		workspace_id: workspaceId,
		workspace_name: workspaceName,
		agent_name: agentName,
		ip: "fd7a:115c:a1e0:49d6::4002",
		client_hostname: "priya-laptop.local",
		short_description: "VS Code Desktop (macOS, arm64)",
		started_at: hoursAgo(offsetHours),
		ended_at: hoursAgo(offsetHours - durationHours),
		duration_seconds: durationHours * 3600,
		status: "clean_disconnected",
		disconnect_reason: "client disconnected",
		explanation: "",
		network: {
			p2p: true,
			avg_latency_ms: 10,
			home_derp: null,
		},
		connections: [
			{
				id: connId,
				type: "vscode",
				detail: "VS Code Desktop 1.96",
				connected_at: hoursAgo(offsetHours),
				disconnected_at: hoursAgo(offsetHours - durationHours),
				status: "clean_disconnected",
				exit_code: 0,
				explanation: "",
			},
		],
		timeline,
	};
}

const crashMlSessions: DiagnosticSession[] = [
	makeCrashLostSession(1400, 45, 1),
	makeCrashLostSession(1200, 30, 2),
	makeCrashLostSession(960, 60, 3),
	makeCrashLostSession(720, 20, 4),
	makeCrashLostSession(480, 55, 5),
	makeCrashLostSession(240, 35, 6),
];

const crashApiSession = makeCrashCleanSession(
	crashApiWorkspaceId,
	"api-server",
	"main",
	18,
	8,
	1,
);

const crashNotebookSession = makeCrashCleanSession(
	crashNotebookWorkspaceId,
	"notebook",
	"jupyter",
	6,
	4,
	2,
);

const crashMlWorkspace: DiagnosticWorkspace = {
	id: crashMlWorkspaceId,
	name: "ml-training",
	owner_username: "priya-ml",
	status: "running",
	template_name: "gpu-workstation",
	template_display_name: "GPU Workstation",
	health: "unhealthy",
	health_reason:
		"Agent has not responded in 10 minutes. Last 6 sessions ended with agent timeout.",
	sessions: crashMlSessions,
};

const crashApiWorkspace: DiagnosticWorkspace = {
	id: crashApiWorkspaceId,
	name: "api-server",
	owner_username: "priya-ml",
	status: "running",
	template_name: "docker-dev",
	template_display_name: "Docker Development",
	health: "healthy",
	health_reason: "",
	sessions: [crashApiSession],
};

const crashNotebookWorkspace: DiagnosticWorkspace = {
	id: crashNotebookWorkspaceId,
	name: "notebook",
	owner_username: "priya-ml",
	status: "running",
	template_name: "jupyter-lab",
	template_display_name: "Jupyter Lab",
	health: "healthy",
	health_reason: "",
	sessions: [crashNotebookSession],
};

const crashCurrentConnections: DiagnosticConnection[] = [
	{
		id: "e4d5f6a7-b8c9-0123-defa-300000000001",
		workspace_id: crashApiWorkspaceId,
		workspace_name: "api-server",
		agent_id: crashApiAgentId,
		agent_name: "main",
		ip: "fd7a:115c:a1e0:49d6::4002",
		client_hostname: "priya-laptop.local",
		short_description: "VS Code Desktop (macOS, arm64)",
		type: "vscode",
		detail: "VS Code Desktop 1.96",
		status: "ongoing",
		started_at: minutesAgo(120),
		p2p: true,
		latency_ms: 9,
		home_derp: null,
		explanation: "",
	},
	{
		id: "e4d5f6a7-b8c9-0123-defa-300000000002",
		workspace_id: crashNotebookWorkspaceId,
		workspace_name: "notebook",
		agent_id: crashNotebookAgentId,
		agent_name: "jupyter",
		ip: "fd7a:115c:a1e0:49d6::4003",
		client_hostname: "priya-laptop.local",
		short_description: "VS Code Desktop (macOS, arm64)",
		type: "vscode",
		detail: "VS Code Desktop 1.96",
		status: "ongoing",
		started_at: minutesAgo(60),
		p2p: true,
		latency_ms: 11,
		home_derp: null,
		explanation: "",
	},
];

const crashPattern: DiagnosticPattern = {
	id: "e4d5f6a7-b8c9-0123-defa-400000000001",
	type: "agent_crash",
	severity: "critical",
	affected_sessions: 6,
	total_sessions: 8,
	title: "Repeated agent crashes on ml-training workspace",
	description:
		"6 of 8 sessions were lost, all on the ml-training workspace. The agent repeatedly becomes unreachable after 20-60 minutes. Other workspaces (api-server, notebook) are unaffected, ruling out client-side issues.",
	commonalities: {
		connection_types: ["ssh", "port_forwarding"],
		client_descriptions: ["SSH Client (macOS, arm64)"],
		duration_range: { min_seconds: 1200, max_seconds: 3600 },
		disconnect_reasons: ["agent timeout"],
		time_of_day_range: null,
	},
	recommendation:
		"Check the ml-training workspace agent logs and system dmesg for OOM kills. The GPU workstation template may need higher memory limits.",
};

export const SCENARIO_AGENT_CRASH: UserDiagnosticResponse = {
	user: {
		id: "e4d5f6a7-b8c9-0123-defa-000000000010",
		username: "priya-ml",
		name: "Priya ML Engineer",
		avatar_url: "",
		email: "priya@example.com",
		roles: ["member"],
		last_seen_at: minutesAgo(1),
		created_at: hoursAgo(24 * 120),
	},
	generated_at: new Date().toISOString(),
	time_window: {
		start: hoursAgo(24),
		end: new Date().toISOString(),
		hours: 24,
	},
	summary: {
		total_sessions: 8,
		total_connections: 16,
		active_connections: 2,
		by_status: {
			ongoing: 2,
			clean: 2,
			lost: 6,
			workspace_stopped: 0,
			workspace_deleted: 0,
		},
		by_type: { ssh: 7, vscode: 4, port_forwarding: 7 },
		network: {
			p2p_connections: 16,
			derp_connections: 0,
			avg_latency_ms: 12,
			p95_latency_ms: 18,
			primary_derp_region: null,
		},
		headline:
			"6 connections lost in 24h, all to workspace ml-training. Other workspaces are healthy.",
	},
	current_connections: crashCurrentConnections,
	workspaces: [crashMlWorkspace, crashApiWorkspace, crashNotebookWorkspace],
	patterns: [crashPattern],
};
