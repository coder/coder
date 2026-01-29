import type { ConnectionType } from "api/typesGenerated";

export const connectionTypeToFriendlyName = (type: ConnectionType): string => {
	switch (type) {
		case "jetbrains":
			return "JetBrains";
		case "reconnecting_pty":
			return "Web terminal";
		case "ssh":
			return "SSH";
		case "vscode":
			return "VS Code";
		case "port_forwarding":
			return "Port forwarding";
		case "workspace_app":
			return "Workspace app";
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
