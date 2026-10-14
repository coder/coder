import type { ConnectionLogMethod } from "#/api/typesGenerated";

export const connectionLogMethodLabels: Record<ConnectionLogMethod, string> = {
	ssh: "SSH",
	reconnecting_pty: "Web terminal",
	workspace_app: "Workspace app",
	port_forwarding: "Port forwarding",
	tunnel: "Tunnel",
};
