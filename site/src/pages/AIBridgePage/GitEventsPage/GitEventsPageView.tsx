import type { WorkspaceGitEvent } from "api/types/workspaceGitEvents";
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

interface GitEventsPageViewProps {
	isLoading: boolean;
	isVisible: boolean;
	events?: readonly WorkspaceGitEvent[];
}

export const GitEventsPageView: FC<GitEventsPageViewProps> = ({
	isLoading,
	isVisible,
	events,
}) => {
	if (!isVisible) {
		return <PaywallAIGovernance />;
	}

	return (
		<Table className="text-sm">
			<TableHeader>
				<TableRow className="text-xs">
					<TableHead>Timestamp</TableHead>
					<TableHead>Event Type</TableHead>
					<TableHead>Developer</TableHead>
					<TableHead>Session ID</TableHead>
					<TableHead>Commit SHA</TableHead>
					<TableHead>Branch</TableHead>
					<TableHead>Repository</TableHead>
					<TableHead>Files Changed</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{isLoading ? (
					<TableLoader />
				) : events?.length === 0 || events === undefined ? (
					<TableEmpty message="No git events available" />
				) : (
					events.map((event) => (
						<TableRow key={event.id}>
							<TableCell>{relativeTime(event.created_at)}</TableCell>
							<TableCell>
								<Badge>{event.event_type}</Badge>
							</TableCell>
							<TableCell>
								<AvatarData
									title={event.owner_username ?? event.owner_id}
									src={event.owner_avatar_url}
								/>
							</TableCell>
							<TableCell>
								{event.session_id ? (
									<span className="font-mono text-xs text-content-secondary">
										{event.session_id.slice(0, 8)}&hellip;
									</span>
								) : (
									"—"
								)}
							</TableCell>
							<TableCell>
								{event.commit_sha ? (
									<span className="font-mono text-xs text-content-secondary">
										{event.commit_sha.slice(0, 7)}
									</span>
								) : (
									"—"
								)}
							</TableCell>
							<TableCell>{event.branch ?? "—"}</TableCell>
							<TableCell>{event.repo_name ?? "—"}</TableCell>
							<TableCell>
								{event.files_changed.length > 0 ? (
									<Badge>{event.files_changed.length}</Badge>
								) : (
									"—"
								)}
							</TableCell>
						</TableRow>
					))
				)}
			</TableBody>
		</Table>
	);
};
