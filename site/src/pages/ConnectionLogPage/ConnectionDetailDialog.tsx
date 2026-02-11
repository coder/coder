import type { WorkspaceConnection } from "api/typesGenerated";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { X } from "lucide-react";
import type { FC } from "react";
import { connectionTypeToFriendlyName } from "utils/connection";

interface ConnectionDetailDialogProps {
	connection: WorkspaceConnection | null;
	onClose: () => void;
}

export const ConnectionDetailDialog: FC<ConnectionDetailDialogProps> = ({
	connection,
	onClose,
}) => {
	return (
		<Dialog open={connection !== null} onOpenChange={onClose}>
			<DialogContent className="max-w-md">
				<DialogHeader>
					<DialogTitle>Connection Details</DialogTitle>
					<DialogClose asChild>
						<button
							type="button"
							className="absolute right-4 top-4 text-content-secondary hover:text-content-primary"
							aria-label="Close"
						>
							<X className="h-4 w-4" />
						</button>
					</DialogClose>
				</DialogHeader>
				{connection && <ConnectionDetails connection={connection} />}
			</DialogContent>
		</Dialog>
	);
};

const ConnectionDetails: FC<{ connection: WorkspaceConnection }> = ({
	connection,
}) => {
	const formatTime = (time?: string) => {
		if (!time) return "—";
		return new Date(time).toLocaleString();
	};

	const duration = () => {
		if (!connection.connected_at) return "—";
		const start = new Date(connection.connected_at).getTime();
		const end = connection.ended_at
			? new Date(connection.ended_at).getTime()
			: Date.now();
		const diffMs = end - start;
		const seconds = Math.floor(diffMs / 1000);
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
		const hours = Math.floor(minutes / 60);
		return `${hours}h ${minutes % 60}m`;
	};

	const rows: [string, string | undefined][] = [
		["Type", connectionTypeToFriendlyName(connection.type)],
		["Status", connection.status],
		["IP", connection.ip],
		["Client Hostname", connection.client_hostname],
		["Description", connection.short_description],
		["Connected At", formatTime(connection.connected_at)],
		["Disconnected At", formatTime(connection.ended_at)],
		["Duration", duration()],
		["Detail", connection.detail],
		["Disconnect Reason", connection.disconnect_reason],
		[
			"Exit Code",
			connection.exit_code !== undefined
				? String(connection.exit_code)
				: undefined,
		],
		["User Agent", connection.user_agent],
		["P2P", connection.p2p !== undefined ? String(connection.p2p) : undefined],
		[
			"Latency",
			connection.latency_ms !== undefined
				? `${connection.latency_ms}ms`
				: undefined,
		],
		["Home DERP", connection.home_derp?.display_name],
	];

	return (
		<dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
			{rows.map(
				([label, value]) =>
					value !== undefined && (
						<div key={label} className="contents">
							<dt className="text-content-secondary font-medium">{label}</dt>
							<dd className="text-content-primary break-all">{value}</dd>
						</div>
					),
			)}
		</dl>
	);
};
