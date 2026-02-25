import type { WorkspaceGitEventSession } from "api/types/workspaceGitEvents";
import { AvatarData } from "components/Avatar/AvatarData";
import { Badge } from "components/Badge/Badge";
import { PaywallAIGovernance } from "components/Paywall/PaywallAIGovernance";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import type { FC } from "react";
import { relativeTime } from "utils/time";

interface SessionsPageViewProps {
	isLoading: boolean;
	isVisible: boolean;
	sessions?: readonly WorkspaceGitEventSession[];
}

export const SessionsPageView: FC<SessionsPageViewProps> = ({
	isLoading,
	isVisible,
	sessions,
}) => {
	if (!isVisible) {
		return <PaywallAIGovernance />;
	}

	return (
		<Table className="text-sm">
			<TableHeader>
				<TableRow className="text-xs">
					<TableHead>Session ID</TableHead>
					<TableHead>Developer</TableHead>
					<TableHead>Agent</TableHead>
					<TableHead>Workspace</TableHead>
					<TableHead>Repository</TableHead>
					<TableHead>Branch</TableHead>
					<TableHead>Commits</TableHead>
					<TableHead>Started</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{isLoading ? (
					<TableLoader />
				) : sessions?.length === 0 || sessions === undefined ? (
					<TableEmpty message="No sessions available" />
				) : (
					sessions.map((session) => (
						<TableRow key={session.session_id}>
							<TableCell>
								<span className="font-mono text-xs text-content-secondary">
									{session.session_id.slice(0, 8)}&hellip;
								</span>
							</TableCell>
							<TableCell>
								<AvatarData
									title={session.owner_username ?? session.owner_id}
									src={session.owner_avatar_url}
								/>
							</TableCell>
							<TableCell>{session.agent_name}</TableCell>
							<TableCell>
								{session.workspace_name ?? session.workspace_id}
							</TableCell>
							<TableCell>{session.repo_name ?? "—"}</TableCell>
							<TableCell>{session.branch ?? "—"}</TableCell>
							<TableCell>
								<Badge>{session.commit_count}</Badge>
							</TableCell>
							<TableCell>{relativeTime(session.started_at)}</TableCell>
						</TableRow>
					))
				)}
			</TableBody>
		</Table>
	);
};
