import type { ConnectionType } from "api/typesGenerated";

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
	}
};

export const connectionTypeIsWeb = (type: ConnectionType): boolean => {
	switch (type) {
		case "port_forwarding":
		case "workspace_app": {
			return true;
		}
		case "reconnecting_pty":
		case "ssh":
		case "jetbrains":
		case "vscode": {
			return false;
		}
	}
};
