import { EmptyState } from "components/EmptyState/EmptyState";
import { Table, TableBody, TableCell, TableRow } from "components/Table/Table";
import { Timeline } from "components/Timeline/Timeline";
import type { FC } from "react";
import { SessionRow } from "./SessionRow";
import type { DiagnosticWorkspace } from "./types";

interface SessionListProps {
	workspaces: DiagnosticWorkspace[];
}

export const SessionList: FC<SessionListProps> = ({ workspaces }) => {
	const sessions = workspaces
		.flatMap((ws) => ws.sessions)
		.sort(
			(a, b) =>
				new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
		);

	return (
		<Table>
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
						row={(session) => <SessionRow key={session.id} session={session} />}
					/>
				)}
			</TableBody>
		</Table>
	);
};
