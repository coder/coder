import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Table, TableBody, TableCell, TableRow } from "components/Table/Table";
import { Timeline } from "components/Timeline/Timeline";
import type { FC } from "react";
import { SessionRow } from "./SessionRow";
import type { DiagnosticWorkspace } from "./types";

interface SessionListProps {
	workspaces: DiagnosticWorkspace[];
	statusFilter: string;
	workspaceFilter: string;
	onStatusFilterChange: (status: string) => void;
	onWorkspaceFilterChange: (workspace: string) => void;
}

const STATUS_OPTIONS = [
	{ value: "all", label: "All" },
	{ value: "ongoing", label: "Connected" },
	{ value: "disconnected", label: "Disconnected" },
	{ value: "workspace_stopped", label: "Workspace Stopped" },
] as const;

export const SessionList: FC<SessionListProps> = ({
	workspaces,
	statusFilter,
	workspaceFilter,
	onStatusFilterChange,
	onWorkspaceFilterChange,
}) => {
	const workspaceNames = [
		...new Set(workspaces.flatMap((ws) => ws.sessions.map(() => ws.name))),
	];

	let filtered = workspaces
		.flatMap((ws) => ws.sessions)
		.sort(
			(a, b) =>
				new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
		);

	if (statusFilter === "ongoing") {
		filtered = filtered.filter((s) => s.status === "ongoing");
	} else if (statusFilter === "disconnected") {
		filtered = filtered.filter(
			(s) =>
				s.status !== "ongoing" &&
				!s.disconnect_reason.toLowerCase().includes("workspace stopped"),
		);
	} else if (statusFilter === "workspace_stopped") {
		filtered = filtered.filter((s) =>
			s.disconnect_reason.toLowerCase().includes("workspace stopped"),
		);
	}
	if (workspaceFilter !== "all") {
		filtered = filtered.filter((s) => s.workspace_name === workspaceFilter);
	}

	return (
		<div>
			<div className="flex items-center gap-4 mb-4">
				<div className="flex items-center gap-1">
					{STATUS_OPTIONS.map((opt) => (
						<Button
							key={opt.value}
							size="sm"
							variant={statusFilter === opt.value ? "default" : "outline"}
							onClick={() => onStatusFilterChange(opt.value)}
						>
							{opt.label}
						</Button>
					))}
				</div>
				{workspaceNames.length > 1 && (
					<select
						value={workspaceFilter}
						onChange={(e) => onWorkspaceFilterChange(e.target.value)}
						className="h-8 rounded-md border border-border bg-transparent px-3 text-xs text-content-primary"
					>
						<option value="all">All workspaces</option>
						{workspaceNames.map((name) => (
							<option key={name} value={name}>
								{name}
							</option>
						))}
					</select>
				)}
			</div>
			<Table>
				<TableBody>
					{filtered.length === 0 ? (
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState message="No sessions in this time window" />
							</TableCell>
						</TableRow>
					) : (
						<Timeline
							items={filtered}
							getDate={(s) => new Date(s.started_at)}
							row={(session) => (
								<SessionRow key={session.id} session={session} />
							)}
						/>
					)}
				</TableBody>
			</Table>
		</div>
	);
};
