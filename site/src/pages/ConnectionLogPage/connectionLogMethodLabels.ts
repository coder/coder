import type { ConnectionLogMethod } from "#/api/typesGenerated";

export const connectionLogMethodLabels: Record<ConnectionLogMethod, string> = {
	ssh: "SSH",
	reconnecting_pty: "Reconnecting PTY",
	workspace_app: "Workspace App",
	port_forwarding: "Port Forwarding",
	tunnel: "Tunnel",
};
