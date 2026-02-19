import type { WorkspaceConnection } from "api/typesGenerated";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import {
	connectionStatusColor,
	connectionStatusDot,
	connectionStatusLabel,
	connectionTypeLabel,
} from "modules/resources/ConnectionStatus";
import type { FC } from "react";
import { formatDateTime } from "utils/time";

interface ConnectionDetailDialogProps {
	connection: WorkspaceConnection | null;
	open: boolean;
	onClose: () => void;
}

function formatDuration(startISO: string, endISO: string): string {
	const start = new Date(startISO).getTime();
	const end = new Date(endISO).getTime();
	const diffMs = end - start;
	if (diffMs < 0) {
		return "—";
	}
	const seconds = Math.floor(diffMs / 1000);
	if (seconds < 60) {
		return `${seconds}s`;
	}
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) {
		return `${minutes}m ${seconds % 60}s`;
	}
	const hours = Math.floor(minutes / 60);
	if (hours < 24) {
		return `${hours}h ${minutes % 60}m`;
	}
	const days = Math.floor(hours / 24);
	return `${days}d ${hours % 24}h`;
}

const DetailRow: FC<{ label: string; value: React.ReactNode }> = ({
	label,
	value,
}) => (
	<div className="flex justify-between py-2 border-b border-border">
		<span className="text-content-secondary text-sm">{label}</span>
		<span className="text-sm font-medium">{value}</span>
	</div>
);

export const ConnectionDetailDialog: FC<ConnectionDetailDialogProps> = ({
	connection,
	open,
	onClose,
}) => {
	if (!connection) {
		return null;
	}

	const connectedAt = connection.connected_at ?? connection.created_at;

	return (
		<Dialog open={open} onOpenChange={(v) => !v && onClose()}>
			<DialogContent className="max-w-lg">
				<DialogHeader>
					<DialogTitle>
						{connectionTypeLabel(connection.type, connection.detail)}
					</DialogTitle>
				</DialogHeader>

				<div className="flex flex-col">
					<DetailRow
						label="Type"
						value={connectionTypeLabel(connection.type, connection.detail)}
					/>
					<DetailRow
						label="Status"
						value={
							<span className="flex items-center gap-2">
								<span
									className={`inline-block h-2 w-2 rounded-full ${connectionStatusDot(connection.status)}`}
								/>
								<span className={connectionStatusColor(connection.status)}>
									{connectionStatusLabel(connection.status)}
								</span>
							</span>
						}
					/>
					{connection.ip && (
						<DetailRow label="IP Address" value={connection.ip} />
					)}
					<DetailRow label="Connected at" value={formatDateTime(connectedAt)} />
					{connection.ended_at && (
						<DetailRow
							label="Disconnected at"
							value={formatDateTime(connection.ended_at)}
						/>
					)}
					{connection.ended_at && (
						<DetailRow
							label="Duration"
							value={formatDuration(connectedAt, connection.ended_at)}
						/>
					)}
					{connection.detail && (
						<DetailRow label="Detail" value={connection.detail} />
					)}
					{connection.client_hostname && (
						<DetailRow
							label="Client hostname"
							value={connection.client_hostname}
						/>
					)}
					{connection.short_description && (
						<DetailRow
							label="Description"
							value={connection.short_description}
						/>
					)}
					{connection.disconnect_reason && (
						<DetailRow
							label="Disconnect reason"
							value={connection.disconnect_reason}
						/>
					)}
					{connection.exit_code !== undefined && (
						<DetailRow label="Exit code" value={connection.exit_code} />
					)}
					{connection.os && (
						<DetailRow label="Operating system" value={connection.os} />
					)}
					{connection.user_agent && (
						<DetailRow label="User agent" value={connection.user_agent} />
					)}
					{connection.p2p !== undefined && (
						<DetailRow label="P2P" value={connection.p2p ? "Yes" : "No"} />
					)}
					{connection.latency_ms !== undefined && (
						<DetailRow
							label="Latency"
							value={`${connection.latency_ms.toFixed(1)} ms`}
						/>
					)}
					{connection.home_derp && (
						<DetailRow label="Home DERP" value={connection.home_derp.name} />
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
