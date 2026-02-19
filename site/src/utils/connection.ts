import type { ConnectionType } from "api/typesGenerated";
import { connectionTypeLabel } from "modules/resources/ConnectionStatus";

export const connectionTypeToFriendlyName = (type: ConnectionType): string => {
	return connectionTypeLabel(type);
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
		case "vscode":
		case "system": {
			return false;
		}
	}
};
