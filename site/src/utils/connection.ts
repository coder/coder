import type { ConnectionType } from "#/api/typesGenerated";

// Labels the filter menu. A log entry uses its own `type_display_name`,
// since an agent can report any app.
export const connectionTypeToFriendlyName = (type: ConnectionType): string => {
	switch (type) {
		case "jetbrains":
			return "JetBrains";
		case "reconnecting_pty":
			return "Web Terminal";
		case "ssh":
			return "SSH";
		case "vscode":
			return "VS Code";
		case "port_forwarding":
			return "Port Forwarding";
		case "workspace_app":
			return "Workspace App";
		case "tunnel":
			return "Tunnel";
	}
};

// Types coderd records from an HTTP request. They carry `web_info` (user,
// IP, user agent, HTTP status code) rather than agent-reported `ssh_info`,
// and are not necessarily browser connections: tunnels are typically
// established by the CLI or an IDE extension.
const WEB_CONNECTION_TYPES = new Set<string>([
	"port_forwarding",
	"workspace_app",
	"tunnel",
]);

// A log entry's type is open, so this takes any string.
export const connectionTypeIsWeb = (type: string): boolean =>
	WEB_CONNECTION_TYPES.has(type);
