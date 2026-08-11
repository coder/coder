import type { FC } from "react";
import { useInfiniteQuery, useQuery } from "react-query";
import {
	infiniteAISandboxSessionNetworkEvents,
	workspaceAISandboxSessions,
} from "#/api/queries/workspaces";
import type { AISandboxSession } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { formatDate } from "#/utils/time";

const enforcementVariant = {
	forced: "default",
	advisory: "warning",
	none: "destructive",
} as const;

const enforcementHelp = {
	forced: "All egress is routed through the platform proxy.",
	advisory: "Egress is routed cooperatively and can be bypassed.",
	none: "No egress routing is claimed for this sandbox.",
} as const;

interface WorkspaceAIEgressActivityProps {
	workspaceId: string;
}

// Shows the retained egress audit trail for AI-confined execution. The
// attestation badge reports what the template declared: the platform records
// it but cannot verify an arbitrary sandbox script's routing coverage.
export const WorkspaceAIEgressActivity: FC<WorkspaceAIEgressActivityProps> = ({
	workspaceId,
}) => {
	const sessionsQuery = useQuery(workspaceAISandboxSessions(workspaceId));
	const sessions = sessionsQuery.data ?? [];

	if (sessionsQuery.isLoading) {
		return (
			<section className="flex flex-col gap-4">
				<Header />
				<div className="flex justify-center p-6">
					<Spinner loading />
				</div>
			</section>
		);
	}

	if (sessions.length === 0) {
		return (
			<section className="flex flex-col gap-4">
				<Header />
				<EmptyState
					message="No confinement sessions yet"
					description="Egress activity appears once a confined agent reports a session."
				/>
			</section>
		);
	}

	return (
		<section className="flex flex-col gap-4">
			<Header />
			{sessions.map((session) => (
				<SessionCard
					key={session.id}
					workspaceId={workspaceId}
					session={session}
				/>
			))}
		</section>
	);
};

const Header: FC = () => (
	<div className="flex flex-col gap-1">
		<h3 className="text-base font-semibold text-content-primary m-0">
			AI egress activity
		</h3>
		<span className="text-sm text-content-secondary">
			Network egress from AI-confined execution is default-deny. Every allowed
			and denied destination is recorded here.
		</span>
	</div>
);

interface SessionCardProps {
	workspaceId: string;
	session: AISandboxSession;
}

const SessionCard: FC<SessionCardProps> = ({ workspaceId, session }) => {
	const eventsQuery = useInfiniteQuery(
		infiniteAISandboxSessionNetworkEvents(workspaceId, session.id),
	);
	const events = eventsQuery.data?.pages.flat() ?? [];

	return (
		<div className="flex flex-col gap-3 rounded-lg border border-solid border-border p-4">
			<div className="flex flex-wrap items-center gap-3">
				<Badge variant={enforcementVariant[session.egress_enforcement]}>
					{session.egress_enforcement}
				</Badge>
				<span className="text-sm text-content-secondary">
					{enforcementHelp[session.egress_enforcement]}
				</span>
				<span className="text-xs text-content-secondary ml-auto">
					{formatDate(new Date(session.started_at))}
					{session.ended_at
						? ` to ${formatDate(new Date(session.ended_at))}`
						: " (open)"}
				</span>
			</div>

			{events.length === 0 && !eventsQuery.isLoading ? (
				<EmptyState message="No egress attempts recorded for this session" />
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Time</TableHead>
							<TableHead>Protocol</TableHead>
							<TableHead>Destination</TableHead>
							<TableHead>Action</TableHead>
							<TableHead>Policy</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{events.map((event) => (
							<TableRow key={event.id}>
								<TableCell>{formatDate(new Date(event.occurred_at))}</TableCell>
								<TableCell>{event.protocol}</TableCell>
								<TableCell>
									{event.host}:{event.port}
								</TableCell>
								<TableCell>
									<Badge
										variant={
											event.action === "denied" ? "destructive" : "default"
										}
										size="xs"
									>
										{event.action}
									</Badge>
								</TableCell>
								<TableCell>{event.policy_revision}</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}

			<div className="flex items-center gap-3">
				<Spinner
					loading={eventsQuery.isLoading || eventsQuery.isFetchingNextPage}
					size="sm"
				/>
				{eventsQuery.hasNextPage && (
					<Button
						variant="outline"
						size="sm"
						disabled={eventsQuery.isFetchingNextPage}
						onClick={() => eventsQuery.fetchNextPage()}
					>
						Load more
					</Button>
				)}
			</div>
		</div>
	);
};
