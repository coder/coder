import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
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
	const workspaceNames = [...new Set(workspaces.map((ws) => ws.name))];

	const sessions = workspaces
		.flatMap((ws) => ws.sessions)
		.sort(
			(a, b) =>
				new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
		);

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
				<TableHeader>
					<TableRow>
						<TableHead className="p-0">
							<div className="flex items-center gap-3 px-8 py-2 text-xs">
								<span className="w-2 shrink-0" />
								<span className="w-36 shrink-0">Type</span>
								<span className="min-w-0 flex-1">Source</span>
								<span className="w-24 shrink-0">Host</span>
								<span className="w-36 shrink-0">Workspace</span>
								<span className="w-20 shrink-0 text-right">Duration</span>
								<span className="w-12 shrink-0 text-right">Time</span>
								<span className="w-36 shrink-0 text-right">Status</span>
								<span className="w-4 shrink-0" />
							</div>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{sessions.length === 0 ? (
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState message="No sessions in this time window" />
							</TableCell>
						</TableRow>
					) : (
						<Timeline
							items={sessions}
							getDate={(s) => new Date(s.started_at)}
							row={(session) => (
								<SessionRow
									key={`${session.id}-${session.started_at}`}
									session={session}
								/>
							)}
						/>
					)}
				</TableBody>
			</Table>
		</div>
	);
};
