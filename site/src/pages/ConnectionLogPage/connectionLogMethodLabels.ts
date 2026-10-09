import type { ConnectionLogMethod } from "#/api/typesGenerated";

export const connectionLogMethodLabels: Record<ConnectionLogMethod, string> = {
	ssh: "SSH",
	reconnecting_pty: "Web Terminal",
	workspace_app: "Workspace App",
	port_forwarding: "Port Forwarding",
	tunnel: "Tunnel",
};
