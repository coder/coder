import type {
	ConnectionType,
	WorkspaceConnectionStatus,
} from "api/typesGenerated";

export function connectionStatusLabel(
	status: WorkspaceConnectionStatus,
): string {
	switch (status) {
		case "ongoing":
			return "Connected";
		case "control_lost":
			return "Control Lost";
		case "client_disconnected":
			return "Disconnected";
		case "clean_disconnected":
			return "Disconnected";
		default:
			return status;
	}
}

export function connectionStatusColor(
	status: WorkspaceConnectionStatus,
): string {
	switch (status) {
		case "ongoing":
			return "text-content-success";
		case "control_lost":
			return "text-content-warning";
		case "client_disconnected":
		case "clean_disconnected":
			return "text-content-secondary";
		default:
			return "text-content-secondary";
	}
}

export function connectionStatusDot(status: WorkspaceConnectionStatus): string {
	switch (status) {
		case "ongoing":
			return "bg-content-success";
		case "control_lost":
			return "bg-content-warning";
		case "client_disconnected":
		case "clean_disconnected":
			return "bg-content-secondary";
		default:
			return "bg-content-secondary";
	}
}

export function connectionTypeLabel(
	type_: ConnectionType,
	detail?: string,
): string {
	switch (type_) {
		case "ssh":
			return "SSH";
		case "reconnecting_pty":
			return "Web Terminal";
		case "vscode":
			return "VS Code";
		case "jetbrains":
			return "JetBrains";
		case "workspace_app":
			return detail ? `App: ${detail}` : "Workspace App";
		case "port_forwarding":
			return detail ? `Port ${detail}` : "Port Forwarding";
		case "system":
			return "System";
		default:
			return type_;
	}
}
